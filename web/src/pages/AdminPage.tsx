import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type User } from "../api/client";
import { useMe } from "../App";

export default function AdminPage() {
  const me = useMe();
  const queryClient = useQueryClient();

  const users = useQuery<User[]>({
    queryKey: ["adminUsers"],
    queryFn: () => api.get<User[]>("/api/admin/users"),
  });
  const settings = useQuery<{ allowRegistration: boolean }>({
    queryKey: ["adminSettings"],
    queryFn: () => api.get("/api/admin/settings"),
  });

  const setAdmin = useMutation({
    mutationFn: ({ id, isAdmin }: { id: string; isAdmin: boolean }) =>
      api.patch(`/api/admin/users/${id}`, { isAdmin }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["adminUsers"] }),
  });
  const deleteUser = useMutation({
    mutationFn: (id: string) => api.del(`/api/admin/users/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["adminUsers"] }),
  });
  const setRegistration = useMutation({
    mutationFn: (allow: boolean) => api.put("/api/admin/settings", { allowRegistration: allow }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["adminSettings"] }),
  });

  return (
    <div className="min-h-full bg-gray-50 dark:bg-gray-950">
      <header className="border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 px-6 py-4">
        <div className="mx-auto flex max-w-3xl items-center gap-4">
          <Link to="/projects" className="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:text-gray-100">
            ← Projects
          </Link>
          <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Administration</h1>
        </div>
      </header>

      <main className="mx-auto max-w-3xl space-y-8 px-6 py-8">
        <section>
          <h2 className="mb-3 text-base font-semibold text-gray-900 dark:text-gray-100">Registration</h2>
          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input
              type="checkbox"
              checked={settings.data?.allowRegistration ?? true}
              onChange={(e) => setRegistration.mutate(e.target.checked)}
            />
            Allow new users to register with email/password
          </label>
        </section>

        <section>
          <h2 className="mb-3 text-base font-semibold text-gray-900 dark:text-gray-100">Users</h2>
          <ul className="divide-y divide-gray-200 dark:divide-gray-800 rounded-md border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
            {users.data?.map((u) => (
              <li key={u.id} className="flex items-center justify-between px-4 py-3 text-sm">
                <div className="flex items-center gap-3">
                  <span
                    className="flex h-7 w-7 items-center justify-center rounded-full text-xs font-semibold text-white"
                    style={{ backgroundColor: u.color }}
                  >
                    {u.name[0]?.toUpperCase()}
                  </span>
                  <div>
                    <span className="font-medium text-gray-900 dark:text-gray-100">{u.name}</span>
                    <span className="ml-2 text-gray-400">{u.email}</span>
                    {u.isAdmin && (
                      <span className="ml-2 rounded bg-indigo-100 px-1.5 py-0.5 text-xs text-indigo-700">
                        admin
                      </span>
                    )}
                  </div>
                </div>
                {u.id !== me.data?.id && (
                  <div className="flex gap-3 text-xs">
                    <button
                      className="text-gray-500 dark:text-gray-400 hover:underline"
                      onClick={() => setAdmin.mutate({ id: u.id, isAdmin: !u.isAdmin })}
                    >
                      {u.isAdmin ? "Remove admin" : "Make admin"}
                    </button>
                    <button
                      className="text-red-500 hover:underline"
                      onClick={() => {
                        if (confirm(`Delete user ${u.email}? Their projects will be removed.`))
                          deleteUser.mutate(u.id);
                      }}
                    >
                      Delete
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </section>
      </main>
    </div>
  );
}
