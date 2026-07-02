import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type FileEntry } from "../api/client";

interface Props {
  projectId: string;
  files: FileEntry[];
  activeFileId: string | null;
  mainPath: string;
  canEdit: boolean;
  onSelect: (f: FileEntry) => void;
}

export default function FileTree({ projectId, files, activeFileId, mainPath, canEdit, onSelect }: Props) {
  const queryClient = useQueryClient();
  const [dragOver, setDragOver] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["files", projectId] });

  const createFile = useMutation({
    mutationFn: (path: string) => api.post(`/api/projects/${projectId}/files`, { path, content: "" }),
    onSuccess: invalidate,
    onError: (e: Error) => alert(e.message),
  });
  const renameFile = useMutation({
    mutationFn: ({ id, path }: { id: string; path: string }) => api.patch(`/api/files/${id}`, { path }),
    onSuccess: invalidate,
    onError: (e: Error) => alert(e.message),
  });
  const deleteFile = useMutation({
    mutationFn: (id: string) => api.del(`/api/files/${id}`),
    onSuccess: invalidate,
    onError: (e: Error) => alert(e.message),
  });

  async function uploadFiles(fileList: FileList | File[]) {
    for (const file of Array.from(fileList)) {
      const form = new FormData();
      form.append("file", file);
      form.append("path", file.name);
      try {
        await api.post(`/api/projects/${projectId}/files`, form);
      } catch (e: any) {
        alert(`${file.name}: ${e.message}`);
      }
    }
    invalidate();
  }

  return (
    <div
      className={`flex h-full flex-col border-r border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 ${dragOver ? "bg-indigo-50" : ""}`}
      onDragOver={(e) => {
        if (!canEdit) return;
        e.preventDefault();
        setDragOver(true);
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={(e) => {
        if (!canEdit) return;
        e.preventDefault();
        setDragOver(false);
        if (e.dataTransfer.files.length) uploadFiles(e.dataTransfer.files);
      }}
    >
      <div className="flex items-center justify-between border-b border-gray-100 dark:border-gray-800 px-3 py-2">
        <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">Files</span>
        {canEdit && (
          <div className="flex gap-1">
            <button
              title="New file"
              className="rounded px-1.5 text-sm text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 hover:text-gray-700 dark:text-gray-300"
              onClick={() => {
                const path = prompt("New file path (e.g. chapter1.typ):");
                if (path) createFile.mutate(path);
              }}
            >
              +
            </button>
            <button
              title="Upload image/asset"
              className="rounded px-1.5 text-sm text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 hover:text-gray-700 dark:text-gray-300"
              onClick={() => inputRef.current?.click()}
            >
              ⬆
            </button>
            <input
              ref={inputRef}
              type="file"
              multiple
              hidden
              onChange={(e) => e.target.files && uploadFiles(e.target.files)}
            />
          </div>
        )}
      </div>
      <ul className="flex-1 overflow-y-auto py-1">
        {files.map((f) => (
          <li key={f.id} className="group">
            <button
              onClick={() => onSelect(f)}
              className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm ${
                f.id === activeFileId ? "bg-indigo-50 font-medium text-indigo-700" : "text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800"
              }`}
            >
              <span className="text-xs">{f.kind === "asset" ? "🖼" : "📄"}</span>
              <span className="flex-1 truncate">{f.path}</span>
              {f.path === mainPath && <span className="text-[10px] text-gray-400">main</span>}
              {canEdit && f.path !== mainPath && (
                <span className="hidden gap-1 group-hover:flex">
                  {f.kind === "text" && (
                    <span
                      title="Rename"
                      className="cursor-pointer text-xs text-gray-400 hover:text-gray-700 dark:text-gray-300"
                      onClick={(e) => {
                        e.stopPropagation();
                        const path = prompt("New path:", f.path);
                        if (path && path !== f.path) renameFile.mutate({ id: f.id, path });
                      }}
                    >
                      ✎
                    </span>
                  )}
                  <span
                    title="Delete"
                    className="cursor-pointer text-xs text-gray-400 hover:text-red-600"
                    onClick={(e) => {
                      e.stopPropagation();
                      if (confirm(`Delete ${f.path}?`)) deleteFile.mutate(f.id);
                    }}
                  >
                    ×
                  </span>
                </span>
              )}
            </button>
          </li>
        ))}
      </ul>
      {canEdit && (
        <p className="border-t border-gray-100 dark:border-gray-800 px-3 py-2 text-[11px] text-gray-400">
          Drag &amp; drop images to upload
        </p>
      )}
    </div>
  );
}
