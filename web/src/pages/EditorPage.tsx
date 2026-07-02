import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { HocuspocusProvider } from "@hocuspocus/provider";
import * as Y from "yjs";
import type { EditorView } from "@codemirror/view";
import { api, roleAtLeast, type Comment, type FileEntry, type Project, type Suggestion } from "../api/client";
import { useMe } from "../App";
import CodeEditor from "../editor/CodeEditor";
import PreviewPane, { type PreviewHandle } from "../editor/PreviewPane";
import FileTree from "../editor/FileTree";
import { SuggestionsPanel, CommentsPanel } from "../editor/panels";
import { ShareDialog, HistoryDialog, SuggestDialog } from "../editor/dialogs";
import { TypstClient, type WorkerDiagnostic } from "../editor/typst/compilerClient";
import { resolveAnchor } from "../editor/annotations";
import { createSuggestMode, type SuggestModeController } from "../editor/suggestMode";

interface CollabSession {
  fileId: string;
  doc: Y.Doc;
  ytext: Y.Text;
  provider: HocuspocusProvider;
  suggest: SuggestModeController;
}

interface PresenceUser {
  name: string;
  color: string;
}

export default function EditorPage({ projectId }: { projectId: string }) {
  const me = useMe();
  const queryClient = useQueryClient();

  const project = useQuery<Project>({
    queryKey: ["project", projectId],
    queryFn: () => api.get(`/api/projects/${projectId}`),
  });
  const files = useQuery<FileEntry[]>({
    queryKey: ["files", projectId],
    queryFn: () => api.get(`/api/projects/${projectId}/files`),
  });

  const [activeFileId, setActiveFileId] = useState<string | null>(null);
  const activeFile = files.data?.find((f) => f.id === activeFileId) ?? null;
  const canEdit = roleAtLeast(project.data?.role, "editor");
  const canSuggest = roleAtLeast(project.data?.role, "suggester");

  // Pick the main file once files are known.
  useEffect(() => {
    if (activeFileId || !files.data || !project.data) return;
    const main = files.data.find((f) => f.path === project.data.mainPath);
    const firstText = files.data.find((f) => f.kind === "text");
    if (main ?? firstText) setActiveFileId((main ?? firstText)!.id);
  }, [files.data, project.data, activeFileId]);

  const suggestions = useQuery<Suggestion[]>({
    queryKey: ["suggestions", activeFileId],
    queryFn: () => api.get(`/api/files/${activeFileId}/suggestions`),
    enabled: !!activeFileId && activeFile?.kind === "text",
  });
  const comments = useQuery<Comment[]>({
    queryKey: ["comments", projectId],
    queryFn: () => api.get(`/api/projects/${projectId}/comments`),
  });

  // ---- live events ----
  useEffect(() => {
    const es = new EventSource(`/api/projects/${projectId}/events`);
    es.onmessage = (ev) => {
      try {
        const event = JSON.parse(ev.data);
        switch (event.type) {
          case "files.changed":
            queryClient.invalidateQueries({ queryKey: ["files", projectId] });
            break;
          case "suggestions.changed":
            queryClient.invalidateQueries({ queryKey: ["suggestions"] });
            break;
          case "comments.changed":
            queryClient.invalidateQueries({ queryKey: ["comments", projectId] });
            break;
          case "versions.changed":
            queryClient.invalidateQueries({ queryKey: ["versions", projectId] });
            break;
          case "doc.stored":
            // another (non-open) file changed server-side → refresh its content
            if (event.payload?.fileId && event.payload.fileId !== activeFileIdRef.current) {
              staleFilesRef.current.add(event.payload.fileId);
              scheduleSync();
            }
            break;
        }
      } catch {
        /* ignore malformed events */
      }
    };
    return () => es.close();
  }, [projectId]);

  // ---- collab session for the active text file ----
  const [session, setSession] = useState<CollabSession | null>(null);
  const [synced, setSynced] = useState(false);
  const [presence, setPresence] = useState<PresenceUser[]>([]);
  const activeFileIdRef = useRef<string | null>(null);
  activeFileIdRef.current = activeFileId;

  useEffect(() => {
    if (!activeFile || activeFile.kind !== "text" || !me.data) {
      setSession(null);
      return;
    }
    const doc = new Y.Doc();
    setSynced(false);
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const provider = new HocuspocusProvider({
      url: `${proto}//${window.location.host}/collab`,
      name: activeFile.id,
      document: doc,
      token: async () => {
        const resp = await api.post<{ token: string }>(`/api/files/${activeFile.id}/collab-token`);
        return resp.token;
      },
      onSynced: () => setSynced(true),
    });
    provider.setAwarenessField("user", {
      name: me.data.name,
      color: me.data.color,
      colorLight: me.data.color + "55",
    });
    const awarenessListener = () => {
      const states = provider.awareness ? Array.from(provider.awareness.getStates().values()) : [];
      setPresence(states.map((s: any) => s.user).filter(Boolean));
    };
    provider.awareness?.on("change", awarenessListener);
    awarenessListener();
    const ytext = doc.getText("content");
    const suggest = createSuggestMode({
      ydoc: doc,
      ytext,
      fileId: activeFile.id,
      onRecordsChanged: () =>
        queryClient.invalidateQueries({ queryKey: ["suggestions", activeFile.id] }),
    });
    setSuggestModeOn(false);
    setSession({ fileId: activeFile.id, doc, ytext, provider, suggest });
    return () => {
      void suggest.flush();
      provider.awareness?.off("change", awarenessListener);
      provider.destroy();
      doc.destroy();
      setSession(null);
    };
  }, [activeFile?.id, activeFile?.kind, me.data?.id]);

  const [suggestModeOn, setSuggestModeOn] = useState(false);
  function toggleSuggestMode() {
    if (!session) return;
    const next = !suggestModeOn;
    session.suggest.setEnabled(next);
    setSuggestModeOn(next);
  }

  // ---- compile pipeline ----
  const typstRef = useRef<TypstClient | null>(null);
  const contentsRef = useRef(new Map<string, string>()); // path → text
  const syncedAssetsRef = useRef(new Set<string>()); // file ids
  const staleFilesRef = useRef(new Set<string>()); // file ids needing refetch
  const compileTimer = useRef<number | null>(null);
  const [svg, setSvg] = useState<string>();
  const [diagnostics, setDiagnostics] = useState<WorkerDiagnostic[]>([]);
  const [compiling, setCompiling] = useState(false);
  const [workerError, setWorkerError] = useState("");

  useEffect(() => {
    const client = new TypstClient();
    client.onFatal = (msg) => setWorkerError(msg);
    typstRef.current = client;
    return () => {
      client.destroy();
      typstRef.current = null;
    };
  }, []);

  const runCompile = useCallback(async () => {
    const typst = typstRef.current;
    const mainPath = project.data?.mainPath;
    if (!typst || !mainPath) return;
    setCompiling(true);
    try {
      const out = await typst.compile(mainPath);
      setDiagnostics(out.diagnostics);
      if (out.svg !== undefined) setSvg(out.svg); // keep last good render on error
    } finally {
      setCompiling(false);
    }
  }, [project.data?.mainPath]);

  const scheduleCompile = useCallback(() => {
    if (compileTimer.current) window.clearTimeout(compileTimer.current);
    compileTimer.current = window.setTimeout(() => runCompile(), 400);
  }, [runCompile]);

  // Full project sync into the worker (initial + on file-tree changes).
  const scheduleSync = useCallback(async () => {
    const typst = typstRef.current;
    if (!typst || !files.data) return;
    const textUpdates: Record<string, string> = {};
    for (const f of files.data) {
      if (f.kind === "text") {
        const isActive = f.id === activeFileIdRef.current;
        if (isActive) continue; // handled by the ytext observer
        if (!contentsRef.current.has(f.path) || staleFilesRef.current.has(f.id)) {
          try {
            const content = await api.get<string>(`/api/files/${f.id}/content`);
            contentsRef.current.set(f.path, content);
            textUpdates[f.path] = content;
            staleFilesRef.current.delete(f.id);
          } catch {
            /* file may have been deleted */
          }
        }
      } else if (!syncedAssetsRef.current.has(f.id)) {
        try {
          const resp = await fetch(`/api/assets/${f.id}`);
          if (resp.ok) {
            const buf = await resp.arrayBuffer();
            typst.sync(undefined, { [f.path]: buf });
            syncedAssetsRef.current.add(f.id);
          }
        } catch {
          /* ignore */
        }
      }
    }
    if (Object.keys(textUpdates).length) typst.sync(textUpdates);
    scheduleCompile();
  }, [files.data, scheduleCompile]);

  useEffect(() => {
    scheduleSync();
  }, [files.data]);

  // Active file: mirror the live ytext into the worker.
  useEffect(() => {
    if (!session || !synced || !activeFile) return;
    const push = () => {
      const text = session.ytext.toString();
      contentsRef.current.set(activeFile.path, text);
      typstRef.current?.sync({ [activeFile.path]: text });
      scheduleCompile();
    };
    push();
    session.ytext.observe(push);
    return () => session.ytext.unobserve(push);
  }, [session, synced, activeFile?.path, scheduleCompile]);

  // ---- scroll sync (approximate: document-fraction based) ----
  const previewRef = useRef<PreviewHandle>(null);
  const [syncEnabled, setSyncEnabled] = useState(true);
  const syncEnabledRef = useRef(syncEnabled);
  syncEnabledRef.current = syncEnabled;
  const syncThrottle = useRef<number | null>(null);

  const handleCursorFraction = useCallback((fraction: number) => {
    if (!syncEnabledRef.current || syncThrottle.current) return;
    syncThrottle.current = window.setTimeout(() => {
      syncThrottle.current = null;
    }, 250);
    previewRef.current?.scrollToFraction(fraction);
  }, []);

  const handleJumpToFraction = useCallback((fraction: number) => {
    const view = viewRef.current;
    if (!view) return;
    const line = view.state.doc.line(Math.max(1, Math.min(view.state.doc.lines, Math.round(fraction * (view.state.doc.lines - 1)) + 1)));
    view.dispatch({ selection: { anchor: line.from }, scrollIntoView: true });
    view.focus();
  }, []);

  // ---- actions ----
  const viewRef = useRef<EditorView | null>(null);
  const [showShare, setShowShare] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [suggestSel, setSuggestSel] = useState<{ from: number; to: number; text: string } | null>(null);
  const [sidePanel, setSidePanel] = useState<"suggestions" | "comments" | null>(null);
  const [showErrors, setShowErrors] = useState(true);

  function currentSelection() {
    const view = viewRef.current;
    if (!view) return null;
    const { from, to } = view.state.selection.main;
    return { from, to, text: view.state.sliceDoc(from, to) };
  }

  async function addComment() {
    const sel = currentSelection();
    const body = prompt("Comment:");
    if (!body) return;
    await api.post(`/api/projects/${projectId}/comments`, {
      body,
      fileId: sel && sel.from !== sel.to ? activeFileId : undefined,
      from: sel && sel.from !== sel.to ? sel.from : undefined,
      to: sel && sel.from !== sel.to ? sel.to : undefined,
    });
    queryClient.invalidateQueries({ queryKey: ["comments", projectId] });
    setSidePanel("comments");
  }

  async function exportPDF() {
    const typst = typstRef.current;
    if (!typst || !project.data) return;
    const { data, diagnostics } = await typst.pdf(project.data.mainPath);
    if (!data) {
      setDiagnostics(diagnostics);
      alert("PDF export failed — fix the compile errors first.");
      return;
    }
    const blob = new Blob([data], { type: "application/pdf" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${project.data.name}.pdf`;
    a.click();
    URL.revokeObjectURL(url);
  }

  function jumpTo(pos: number | null) {
    const view = viewRef.current;
    if (view && pos !== null) {
      view.dispatch({ selection: { anchor: Math.min(pos, view.state.doc.length) }, scrollIntoView: true });
      view.focus();
    }
  }

  function jumpToDiagnostic(d: WorkerDiagnostic) {
    const view = viewRef.current;
    if (!view || !activeFile) return;
    if (d.file && d.file !== activeFile.path) {
      const target = files.data?.find((f) => f.path === d.file);
      if (target) setActiveFileId(target.id);
      return;
    }
    try {
      const line = view.state.doc.line(Math.min(d.line, view.state.doc.lines));
      jumpTo(Math.min(line.from + d.col - 1, line.to));
    } catch {
      /* out of range */
    }
  }

  const errorCount = diagnostics.filter((d) => d.severity === "error").length;
  const annotationData = useMemo(
    () => ({ suggestions: suggestions.data ?? [], comments: (comments.data ?? []).filter((c) => c.fileId === activeFileId) }),
    [suggestions.data, comments.data, activeFileId]
  );

  if (project.isError) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-gray-500">
        <p>Project not found or you don't have access.</p>
        <Link to="/projects" className="text-indigo-600 hover:underline">
          Back to projects
        </Link>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {/* top bar */}
      <header className="flex items-center gap-3 border-b border-gray-200 bg-white px-3 py-2">
        <Link to="/projects" className="text-sm text-gray-400 hover:text-gray-700" title="Back to projects">
          ←
        </Link>
        <h1
          className="max-w-xs truncate text-sm font-semibold text-gray-900"
          title={project.data?.name}
          onDoubleClick={async () => {
            if (!canEdit) return;
            const name = prompt("Project name:", project.data?.name);
            if (name) {
              await api.patch(`/api/projects/${projectId}`, { name });
              queryClient.invalidateQueries({ queryKey: ["project", projectId] });
            }
          }}
        >
          {project.data?.name ?? "…"}
        </h1>
        <span className="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-gray-500">
          {project.data?.role}
        </span>

        <div className="ml-2 flex -space-x-1.5">
          {presence.slice(0, 6).map((u, i) => (
            <span
              key={i}
              title={u.name}
              className="flex h-6 w-6 items-center justify-center rounded-full border-2 border-white text-[10px] font-semibold text-white"
              style={{ backgroundColor: u.color }}
            >
              {u.name?.[0]?.toUpperCase()}
            </span>
          ))}
        </div>

        <div className="ml-auto flex items-center gap-1.5 text-sm">
          {canEdit && activeFile?.kind === "text" && (
            <ToolbarButton
              onClick={toggleSuggestMode}
              active={suggestModeOn}
              title="Suggesting mode: your typing becomes tracked changes for review instead of direct edits"
            >
              {suggestModeOn ? "✓ Suggesting" : "Suggesting"}
            </ToolbarButton>
          )}
          {canSuggest && activeFile?.kind === "text" && (
            <ToolbarButton
              onClick={() => {
                const sel = currentSelection();
                if (sel) setSuggestSel(sel);
              }}
              title="Propose a tracked change for the selection"
            >
              Suggest
            </ToolbarButton>
          )}
          {canSuggest && <ToolbarButton onClick={addComment}>Comment</ToolbarButton>}
          <ToolbarButton
            onClick={() => setSidePanel(sidePanel === "suggestions" ? null : "suggestions")}
            active={sidePanel === "suggestions"}
          >
            Suggestions{suggestions.data?.length ? ` (${suggestions.data.length})` : ""}
          </ToolbarButton>
          <ToolbarButton
            onClick={() => setSidePanel(sidePanel === "comments" ? null : "comments")}
            active={sidePanel === "comments"}
          >
            Comments{comments.data?.filter((c) => c.status === "open" && !c.parentId).length
              ? ` (${comments.data.filter((c) => c.status === "open" && !c.parentId).length})`
              : ""}
          </ToolbarButton>
          <ToolbarButton onClick={() => setShowHistory(true)}>History</ToolbarButton>
          <ToolbarButton onClick={() => setShowShare(true)}>Share</ToolbarButton>
          <button
            onClick={exportPDF}
            className="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-indigo-700"
          >
            Export PDF
          </button>
        </div>
      </header>

      {/* main area */}
      <div className="flex min-h-0 flex-1">
        <div className="w-56 shrink-0">
          <FileTree
            projectId={projectId}
            files={files.data ?? []}
            activeFileId={activeFileId}
            mainPath={project.data?.mainPath ?? "main.typ"}
            canEdit={canEdit}
            onSelect={(f) => f.kind === "text" && setActiveFileId(f.id)}
          />
        </div>

        <div className="flex min-w-0 flex-1 flex-col border-r border-gray-200">
          {session && synced && activeFile ? (
            <CodeEditor
              key={session.fileId}
              ydoc={session.doc}
              ytext={session.ytext}
              awareness={session.provider.awareness!}
              readOnly={!canEdit}
              annotations={annotationData}
              callbacks={{
                onSelectSuggestion: () => setSidePanel("suggestions"),
                onSelectComment: () => setSidePanel("comments"),
              }}
              onViewReady={(v) => (viewRef.current = v)}
              onCursorFraction={handleCursorFraction}
              extraExtensions={[session.suggest.extension]}
            />
          ) : (
            <div className="flex flex-1 items-center justify-center text-sm text-gray-400">
              {activeFile ? "Connecting to document…" : "Select a file"}
            </div>
          )}

          {/* diagnostics */}
          <div className="border-t border-gray-200 bg-white">
            <button
              className="flex w-full items-center gap-2 px-3 py-1.5 text-xs text-gray-500 hover:bg-gray-50"
              onClick={() => setShowErrors(!showErrors)}
            >
              <span className={errorCount ? "font-medium text-red-600" : "text-green-600"}>
                {errorCount ? `● ${errorCount} error${errorCount > 1 ? "s" : ""}` : "● compiles cleanly"}
              </span>
              {diagnostics.length > errorCount && (
                <span className="text-amber-600">{diagnostics.length - errorCount} warnings</span>
              )}
              <span className="ml-auto">{showErrors ? "▾" : "▸"}</span>
            </button>
            {showErrors && diagnostics.length > 0 && (
              <ul className="max-h-32 overflow-y-auto border-t border-gray-100 text-xs">
                {diagnostics.map((d, i) => (
                  <li key={i}>
                    <button
                      className="flex w-full gap-2 px-3 py-1 text-left hover:bg-gray-50"
                      onClick={() => jumpToDiagnostic(d)}
                    >
                      <span className={d.severity === "error" ? "text-red-600" : "text-amber-600"}>
                        {d.severity}
                      </span>
                      <span className="text-gray-400">
                        {d.file}:{d.line}:{d.col}
                      </span>
                      <span className="min-w-0 flex-1 truncate text-gray-700">{d.message}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {workerError && (
              <p className="border-t border-gray-100 px-3 py-1 text-xs text-red-600">
                Preview engine failed: {workerError}
              </p>
            )}
          </div>
        </div>

        <div className="min-w-0 flex-1">
          <PreviewPane
            ref={previewRef}
            svg={svg}
            compiling={compiling}
            onJumpToFraction={handleJumpToFraction}
            syncEnabled={syncEnabled}
            onToggleSync={() => setSyncEnabled((s) => !s)}
          />
        </div>

        {sidePanel && (
          <div className="flex w-80 shrink-0 flex-col border-l border-gray-200 bg-gray-50">
            <div className="border-b border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700">
              {sidePanel === "suggestions" ? "Suggestions" : "Comments"}
            </div>
            {sidePanel === "suggestions" ? (
              <SuggestionsPanel
                fileId={activeFileId}
                suggestions={suggestions.data ?? []}
                canResolve={canEdit}
                meId={me.data?.id}
                onJump={(s) => session && jumpTo(resolveAnchor(session.doc, s.anchorStart))}
              />
            ) : (
              <CommentsPanel
                projectId={projectId}
                comments={comments.data ?? []}
                activeFileId={activeFileId}
                meId={me.data?.id}
                onJump={(c) => session && c.anchorStart && jumpTo(resolveAnchor(session.doc, c.anchorStart))}
              />
            )}
          </div>
        )}
      </div>

      {showShare && (
        <ShareDialog projectId={projectId} isOwner={project.data?.role === "owner"} onClose={() => setShowShare(false)} />
      )}
      {showHistory && <HistoryDialog projectId={projectId} canEdit={canEdit} onClose={() => setShowHistory(false)} />}
      {suggestSel && activeFileId && (
        <SuggestDialog
          fileId={activeFileId}
          selection={suggestSel}
          selectedText={suggestSel.text}
          onClose={() => setSuggestSel(null)}
        />
      )}
    </div>
  );
}

function ToolbarButton({
  children,
  onClick,
  title,
  active,
}: {
  children: React.ReactNode;
  onClick: () => void;
  title?: string;
  active?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      title={title}
      className={`rounded-md px-2.5 py-1.5 text-xs font-medium ${
        active ? "bg-indigo-100 text-indigo-700" : "text-gray-600 hover:bg-gray-100"
      }`}
    >
      {children}
    </button>
  );
}
