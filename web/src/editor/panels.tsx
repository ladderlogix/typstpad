import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type Comment, type Suggestion } from "../api/client";

// ---- Suggestions panel ----

export function SuggestionsPanel({
  fileId,
  suggestions,
  canResolve,
  meId,
  onJump,
}: {
  fileId: string | null;
  suggestions: Suggestion[];
  canResolve: boolean;
  meId?: string;
  onJump: (s: Suggestion) => void;
}) {
  const queryClient = useQueryClient();
  const resolve = useMutation({
    mutationFn: ({ id, action }: { id: string; action: "accept" | "reject" }) =>
      api.post(`/api/suggestions/${id}/${action}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["suggestions", fileId] }),
    onError: (e: Error) => alert(e.message),
  });

  if (!suggestions.length) {
    return <Empty text="No open suggestions for this file. Select text and click “Suggest” to propose a change." />;
  }
  return (
    <ul className="space-y-2 overflow-y-auto p-3">
      {suggestions.map((s) => (
        <li key={s.id} className="rounded-lg border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-3 text-sm shadow-sm">
          <button className="mb-1 flex w-full items-center gap-2 text-left" onClick={() => onJump(s)}>
            <span
              className="inline-block h-2.5 w-2.5 rounded-full"
              style={{ backgroundColor: s.authorColor }}
            />
            <span className="font-medium text-gray-900 dark:text-gray-100">{s.authorName}</span>
            <span className="text-xs text-gray-400">
              {s.type} · {new Date(s.createdAt).toLocaleTimeString()}
            </span>
          </button>
          {s.deletedPreview && (
            <p className="mb-1 rounded bg-red-50 px-2 py-1 text-xs text-red-700 line-through">
              {truncate(s.deletedPreview, 160)}
            </p>
          )}
          {s.insertedText !== undefined && s.insertedText !== null && (
            <p className="mb-1 rounded bg-green-50 px-2 py-1 text-xs text-green-700">
              {truncate(s.insertedText, 160)}
            </p>
          )}
          <div className="mt-2 flex gap-2">
            {canResolve && (
              <button
                className="rounded bg-green-600 px-2 py-1 text-xs font-medium text-white hover:bg-green-700"
                onClick={() => resolve.mutate({ id: s.id, action: "accept" })}
              >
                Accept
              </button>
            )}
            {(canResolve || s.authorId === meId) && (
              <button
                className="rounded border border-gray-300 px-2 py-1 text-xs text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800"
                onClick={() => resolve.mutate({ id: s.id, action: "reject" })}
              >
                Reject
              </button>
            )}
          </div>
        </li>
      ))}
    </ul>
  );
}

// ---- Comments panel ----

export function CommentsPanel({
  projectId,
  comments,
  activeFileId,
  meId,
  onJump,
}: {
  projectId: string;
  comments: Comment[];
  activeFileId: string | null;
  meId?: string;
  onJump: (c: Comment) => void;
}) {
  const queryClient = useQueryClient();
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["comments", projectId] });
  const reply = useMutation({
    mutationFn: ({ parentId, body }: { parentId: string; body: string }) =>
      api.post(`/api/projects/${projectId}/comments`, { parentId, body }),
    onSuccess: invalidate,
  });
  const resolve = useMutation({
    mutationFn: (id: string) => api.post(`/api/comments/${id}/resolve`),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/api/comments/${id}`),
    onSuccess: invalidate,
  });

  const threads = comments.filter((c) => !c.parentId);
  const replies = (id: string) => comments.filter((c) => c.parentId === id);

  if (!threads.length) {
    return <Empty text="No comments yet. Select text and click “Comment” to start a thread." />;
  }
  return (
    <ul className="space-y-2 overflow-y-auto p-3">
      {threads.map((c) => (
        <li
          key={c.id}
          className={`rounded-lg border bg-white dark:bg-gray-900 p-3 text-sm shadow-sm ${
            c.status === "resolved" ? "border-gray-100 dark:border-gray-800 opacity-60" : "border-gray-200 dark:border-gray-800"
          }`}
        >
          <button className="flex w-full items-center gap-2 text-left" onClick={() => onJump(c)}>
            <span className="inline-block h-2.5 w-2.5 rounded-full" style={{ backgroundColor: c.authorColor }} />
            <span className="font-medium text-gray-900 dark:text-gray-100">{c.authorName}</span>
            <span className="text-xs text-gray-400">{new Date(c.createdAt).toLocaleString()}</span>
            {c.fileId === activeFileId && c.anchorStart && (
              <span className="text-[10px] text-amber-600">anchored</span>
            )}
          </button>
          <p className="mt-1 whitespace-pre-wrap text-gray-700 dark:text-gray-300">{c.body}</p>
          {replies(c.id).map((r) => (
            <div key={r.id} className="mt-2 border-l-2 border-gray-100 dark:border-gray-800 pl-3">
              <span className="text-xs font-medium text-gray-900 dark:text-gray-100">{r.authorName}</span>
              <p className="whitespace-pre-wrap text-xs text-gray-600 dark:text-gray-400">{r.body}</p>
            </div>
          ))}
          <div className="mt-2 flex items-center gap-2">
            <button
              className="text-xs text-gray-500 dark:text-gray-400 hover:underline"
              onClick={() => {
                const body = prompt("Reply:");
                if (body) reply.mutate({ parentId: c.id, body });
              }}
            >
              Reply
            </button>
            <button className="text-xs text-gray-500 dark:text-gray-400 hover:underline" onClick={() => resolve.mutate(c.id)}>
              {c.status === "resolved" ? "Reopen" : "Resolve"}
            </button>
            {c.authorId === meId && (
              <button className="text-xs text-red-500 hover:underline" onClick={() => remove.mutate(c.id)}>
                Delete
              </button>
            )}
          </div>
        </li>
      ))}
    </ul>
  );
}

function Empty({ text }: { text: string }) {
  return <p className="p-4 text-center text-xs text-gray-400">{text}</p>;
}

function truncate(s: string, n: number) {
  return s.length > n ? s.slice(0, n) + "…" : s;
}
