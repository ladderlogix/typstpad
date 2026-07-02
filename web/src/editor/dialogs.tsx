import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MergeView } from "@codemirror/merge";
import { EditorView, lineNumbers } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { api, type Member, type ShareLink, type Snapshot } from "../api/client";

export function Modal({ title, onClose, children, wide }: { title: string; onClose: () => void; children: React.ReactNode; wide?: boolean }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4" onClick={onClose}>
      <div
        className={`flex max-h-[85vh] w-full ${wide ? "max-w-4xl" : "max-w-lg"} flex-col rounded-xl bg-white shadow-xl`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-gray-100 px-5 py-3">
          <h2 className="text-base font-semibold text-gray-900">{title}</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-700">
            ✕
          </button>
        </div>
        <div className="flex-1 overflow-y-auto">{children}</div>
      </div>
    </div>
  );
}

// ---- Share dialog ----

export function ShareDialog({ projectId, isOwner, onClose }: { projectId: string; isOwner: boolean; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("editor");
  const [linkRole, setLinkRole] = useState("viewer");
  const [error, setError] = useState("");

  const members = useQuery<Member[]>({
    queryKey: ["members", projectId],
    queryFn: () => api.get(`/api/projects/${projectId}/members`),
  });
  const links = useQuery<ShareLink[]>({
    queryKey: ["links", projectId],
    queryFn: () => api.get(`/api/projects/${projectId}/links`),
    enabled: isOwner,
  });

  const addMember = useMutation({
    mutationFn: () => api.post(`/api/projects/${projectId}/members`, { email, role }),
    onSuccess: () => {
      setEmail("");
      setError("");
      queryClient.invalidateQueries({ queryKey: ["members", projectId] });
    },
    onError: (e: Error) => setError(e.message),
  });
  const updateMember = useMutation({
    mutationFn: ({ userId, role }: { userId: string; role: string }) =>
      api.patch(`/api/projects/${projectId}/members/${userId}`, { role }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["members", projectId] }),
  });
  const removeMember = useMutation({
    mutationFn: (userId: string) => api.del(`/api/projects/${projectId}/members/${userId}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["members", projectId] }),
  });
  const createLink = useMutation({
    mutationFn: () => api.post<ShareLink>(`/api/projects/${projectId}/links`, { role: linkRole }),
    onSuccess: (l) => {
      queryClient.invalidateQueries({ queryKey: ["links", projectId] });
      const url = `${window.location.origin}/join/${l.token}`;
      prompt("Share this link (shown once):", url);
    },
  });
  const revokeLink = useMutation({
    mutationFn: (id: string) => api.del(`/api/links/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["links", projectId] }),
  });

  return (
    <Modal title="Share project" onClose={onClose}>
      <div className="space-y-6 p-5">
        <section>
          <h3 className="mb-2 text-sm font-medium text-gray-700">People</h3>
          {isOwner && (
            <div className="mb-3 flex gap-2">
              <input
                placeholder="user@example.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="flex-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm focus:border-indigo-500 focus:outline-none"
              />
              <RoleSelect value={role} onChange={setRole} />
              <button
                onClick={() => addMember.mutate()}
                className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-700"
              >
                Invite
              </button>
            </div>
          )}
          {error && <p className="mb-2 text-xs text-red-600">{error}</p>}
          <ul className="divide-y divide-gray-100 rounded-md border border-gray-200">
            {members.data?.map((m) => (
              <li key={m.userId} className="flex items-center justify-between px-3 py-2 text-sm">
                <div className="flex items-center gap-2">
                  <span
                    className="flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold text-white"
                    style={{ backgroundColor: m.color }}
                  >
                    {m.name[0]?.toUpperCase()}
                  </span>
                  <span className="text-gray-900">{m.name}</span>
                  <span className="text-xs text-gray-400">{m.email}</span>
                </div>
                {isOwner && m.role !== "owner" ? (
                  <div className="flex items-center gap-2">
                    <RoleSelect value={m.role} onChange={(role) => updateMember.mutate({ userId: m.userId, role })} />
                    <button
                      className="text-xs text-red-500 hover:underline"
                      onClick={() => removeMember.mutate(m.userId)}
                    >
                      Remove
                    </button>
                  </div>
                ) : (
                  <span className="text-xs text-gray-500">{m.role}</span>
                )}
              </li>
            ))}
          </ul>
        </section>

        {isOwner && (
          <section>
            <h3 className="mb-2 text-sm font-medium text-gray-700">Share links</h3>
            <div className="mb-3 flex gap-2">
              <RoleSelect value={linkRole} onChange={setLinkRole} />
              <button
                onClick={() => createLink.mutate()}
                className="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
              >
                Create link
              </button>
            </div>
            <ul className="divide-y divide-gray-100 rounded-md border border-gray-200">
              {links.data?.map((l) => (
                <li key={l.id} className="flex items-center justify-between px-3 py-2 text-sm">
                  <span className="text-gray-600">
                    {l.role} link · created {new Date(l.createdAt).toLocaleDateString()}
                  </span>
                  <button className="text-xs text-red-500 hover:underline" onClick={() => revokeLink.mutate(l.id)}>
                    Revoke
                  </button>
                </li>
              ))}
              {links.data?.length === 0 && <li className="px-3 py-2 text-xs text-gray-400">No active links.</li>}
            </ul>
          </section>
        )}
      </div>
    </Modal>
  );
}

function RoleSelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-indigo-500 focus:outline-none"
    >
      <option value="viewer">Viewer</option>
      <option value="suggester">Suggester</option>
      <option value="editor">Editor</option>
    </select>
  );
}

// ---- History dialog ----

interface DiffFile {
  path: string;
  status: "added" | "removed" | "modified";
  old: string;
  new: string;
}

export function HistoryDialog({ projectId, canEdit, onClose }: { projectId: string; canEdit: boolean; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<Snapshot | null>(null);
  const [diffPath, setDiffPath] = useState<string | null>(null);

  const versions = useQuery<Snapshot[]>({
    queryKey: ["versions", projectId],
    queryFn: () => api.get(`/api/projects/${projectId}/versions`),
  });
  const diff = useQuery<DiffFile[]>({
    queryKey: ["diff", projectId, selected?.id],
    queryFn: () => api.get(`/api/projects/${projectId}/diff?from=${selected!.id}&to=current`),
    enabled: !!selected,
  });
  const createVersion = useMutation({
    mutationFn: (name: string) => api.post(`/api/projects/${projectId}/versions`, { name }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["versions", projectId] }),
  });
  const restore = useMutation({
    mutationFn: (id: string) => api.post(`/api/versions/${id}/restore`),
    onSuccess: () => {
      queryClient.invalidateQueries();
      onClose();
    },
    onError: (e: Error) => alert(e.message),
  });

  const diffFile = diff.data?.find((f) => f.path === diffPath) ?? diff.data?.[0];

  return (
    <Modal title="Version history" onClose={onClose} wide>
      <div className="flex h-[70vh]">
        <div className="w-64 shrink-0 overflow-y-auto border-r border-gray-100">
          {canEdit && (
            <button
              className="m-3 w-[calc(100%-1.5rem)] rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
              onClick={() => {
                const name = prompt("Version name:");
                if (name) createVersion.mutate(name);
              }}
            >
              + Save named version
            </button>
          )}
          <ul>
            {versions.data?.map((v) => (
              <li key={v.id}>
                <button
                  onClick={() => {
                    setSelected(v);
                    setDiffPath(null);
                  }}
                  className={`w-full px-4 py-2 text-left text-sm ${
                    selected?.id === v.id ? "bg-indigo-50 text-indigo-700" : "text-gray-700 hover:bg-gray-50"
                  }`}
                >
                  <span className="block font-medium">
                    {v.name ?? (v.kind === "auto" ? "Auto-save" : v.kind === "pre_restore" ? "Before restore" : v.kind)}
                  </span>
                  <span className="text-xs text-gray-400">{new Date(v.createdAt).toLocaleString()}</span>
                </button>
              </li>
            ))}
            {versions.data?.length === 0 && (
              <li className="px-4 py-6 text-center text-xs text-gray-400">
                No versions yet — they are created automatically as you edit.
              </li>
            )}
          </ul>
        </div>
        <div className="flex flex-1 flex-col overflow-hidden">
          {!selected ? (
            <p className="p-8 text-center text-sm text-gray-400">Select a version to compare with the current state.</p>
          ) : (
            <>
              <div className="flex items-center gap-2 border-b border-gray-100 px-4 py-2">
                <select
                  value={diffFile?.path ?? ""}
                  onChange={(e) => setDiffPath(e.target.value)}
                  className="rounded-md border border-gray-300 px-2 py-1 text-sm"
                >
                  {diff.data?.map((f) => (
                    <option key={f.path} value={f.path}>
                      {f.path} ({f.status})
                    </option>
                  ))}
                </select>
                {diff.data?.length === 0 && (
                  <span className="text-sm text-gray-400">No differences from current state.</span>
                )}
                {canEdit && (
                  <button
                    className="ml-auto rounded-md bg-amber-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-amber-700"
                    onClick={() => {
                      if (confirm("Restore the project to this version? Current state is snapshotted first."))
                        restore.mutate(selected.id);
                    }}
                  >
                    Restore this version
                  </button>
                )}
              </div>
              <div className="flex-1 overflow-auto">
                {diffFile && <DiffView key={selected.id + diffFile.path} oldText={diffFile.old} newText={diffFile.new} />}
              </div>
            </>
          )}
        </div>
      </div>
    </Modal>
  );
}

function DiffView({ oldText, newText }: { oldText: string; newText: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!ref.current) return;
    const view = new MergeView({
      a: {
        doc: oldText,
        extensions: [lineNumbers(), EditorView.editable.of(false), EditorState.readOnly.of(true), EditorView.lineWrapping],
      },
      b: {
        doc: newText,
        extensions: [lineNumbers(), EditorView.editable.of(false), EditorState.readOnly.of(true), EditorView.lineWrapping],
      },
      parent: ref.current,
      collapseUnchanged: { margin: 3, minSize: 4 },
    });
    return () => view.destroy();
  }, [oldText, newText]);
  return <div ref={ref} className="text-sm" />;
}

// ---- Suggest dialog ----

export function SuggestDialog({
  fileId,
  selection,
  selectedText,
  onClose,
}: {
  fileId: string;
  selection: { from: number; to: number };
  selectedText: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const isInsert = selection.from === selection.to;
  const [type, setType] = useState<"insert" | "delete" | "replace">(isInsert ? "insert" : "replace");
  const [text, setText] = useState(isInsert ? "" : selectedText);
  const [error, setError] = useState("");

  const create = useMutation({
    mutationFn: () =>
      api.post(`/api/files/${fileId}/suggestions`, {
        type,
        from: selection.from,
        to: selection.to,
        text: type === "delete" ? "" : text,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["suggestions", fileId] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <Modal title="Suggest a change" onClose={onClose}>
      <div className="space-y-3 p-5">
        {!isInsert && (
          <div className="flex gap-2 text-sm">
            <label className="flex items-center gap-1">
              <input type="radio" checked={type === "replace"} onChange={() => setType("replace")} /> Replace
            </label>
            <label className="flex items-center gap-1">
              <input type="radio" checked={type === "delete"} onChange={() => setType("delete")} /> Delete
            </label>
          </div>
        )}
        {!isInsert && (
          <p className="max-h-24 overflow-y-auto rounded bg-gray-50 p-2 text-xs text-gray-600">{selectedText}</p>
        )}
        {type !== "delete" && (
          <textarea
            autoFocus
            rows={5}
            placeholder={isInsert ? "Text to insert at the cursor…" : "Proposed replacement…"}
            value={text}
            onChange={(e) => setText(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-3 py-2 font-mono text-sm focus:border-indigo-500 focus:outline-none"
          />
        )}
        {error && <p className="text-sm text-red-600">{error}</p>}
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="rounded-md px-4 py-2 text-sm text-gray-600 hover:bg-gray-100">
            Cancel
          </button>
          <button
            onClick={() => create.mutate()}
            disabled={create.isPending || (type !== "delete" && !text)}
            className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            Submit suggestion
          </button>
        </div>
      </div>
    </Modal>
  );
}
