import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Notification } from "./api/client";

const ICON: Record<string, string> = { comment: "💬", mention: "@", share: "📄" };

// Bell with an unread badge and a dropdown feed. The unread count polls every
// 30s (notifications are per-user, so there's no project SSE channel for them).
export default function NotificationBell() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);

  const unread = useQuery<{ count: number }>({
    queryKey: ["notif-unread"],
    queryFn: () => api.get("/api/notifications/unread-count"),
    refetchInterval: 30000,
  });
  const list = useQuery<{ items: Notification[]; unread: number }>({
    queryKey: ["notifs"],
    queryFn: () => api.get("/api/notifications"),
    enabled: open,
  });
  const markAll = useMutation({
    mutationFn: () => api.post("/api/notifications/read"),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["notif-unread"] });
      queryClient.invalidateQueries({ queryKey: ["notifs"] });
    },
  });
  const markOne = useMutation({
    mutationFn: (id: string) => api.post(`/api/notifications/${id}/read`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["notif-unread"] }),
  });

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const count = unread.data?.count ?? 0;

  function openItem(n: Notification) {
    if (!n.readAt) markOne.mutate(n.id);
    setOpen(false);
    if (n.projectId) navigate(`/p/${n.projectId}`);
  }

  return (
    <div ref={wrapRef} className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        className="relative flex h-8 w-8 items-center justify-center rounded-full text-lg text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
        title="Notifications"
        aria-label={`Notifications${count ? `, ${count} unread` : ""}`}
      >
        🔔
        {count > 0 && (
          <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-semibold text-white">
            {count > 9 ? "9+" : count}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 z-50 mt-2 w-80 overflow-hidden rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-800 dark:bg-gray-900">
          <div className="flex items-center justify-between border-b border-gray-100 px-3 py-2 dark:border-gray-800">
            <span className="text-sm font-medium text-gray-700 dark:text-gray-300">Notifications</span>
            {count > 0 && (
              <button onClick={() => markAll.mutate()} className="text-xs text-indigo-600 hover:underline dark:text-indigo-400">
                Mark all read
              </button>
            )}
          </div>
          <div className="max-h-96 overflow-y-auto">
            {list.data?.items.length === 0 && (
              <p className="px-3 py-8 text-center text-sm text-gray-400">You're all caught up.</p>
            )}
            {list.data?.items.map((n) => (
              <button
                key={n.id}
                onClick={() => openItem(n)}
                className={`flex w-full items-start gap-2 px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-gray-800 ${
                  n.readAt ? "" : "bg-indigo-50/40 dark:bg-indigo-950/30"
                }`}
              >
                <span className="mt-0.5 text-sm">{ICON[n.type] ?? "•"}</span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm text-gray-700 dark:text-gray-300">{n.summary}</span>
                  <span className="block text-xs text-gray-400">{new Date(n.createdAt).toLocaleString()}</span>
                </span>
                {!n.readAt && <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-indigo-500" />}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
