import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type APIToken } from "../api/client";
import { useMe } from "../App";
import { ThemeToggle } from "../theme";

export default function SettingsPage() {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [created, setCreated] = useState<APIToken | null>(null);

  const tokens = useQuery<APIToken[]>({
    queryKey: ["tokens"],
    queryFn: () => api.get<APIToken[]>("/api/tokens"),
  });

  const create = useMutation({
    mutationFn: () => api.post<APIToken>("/api/tokens", { name }),
    onSuccess: (t) => {
      setCreated(t);
      setName("");
      queryClient.invalidateQueries({ queryKey: ["tokens"] });
    },
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/api/tokens/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["tokens"] }),
  });

  return (
    <div className="min-h-full bg-gray-50 dark:bg-gray-950">
      <header className="border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 px-6 py-4">
        <div className="mx-auto flex max-w-3xl items-center gap-4">
          <Link to="/projects" className="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:text-gray-100">
            ← Projects
          </Link>
          <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Settings</h1>
          <ThemeToggle className="ml-auto text-base" />
        </div>
      </header>

      <main className="mx-auto max-w-3xl space-y-8 px-6 py-8">
        <ProfileSection />
        <section>
          <h2 className="mb-2 text-base font-semibold text-gray-900 dark:text-gray-100">API tokens</h2>
          <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
            Tokens let scripts, the <code className="rounded bg-gray-100 dark:bg-gray-800 px-1">typstpad</code> CLI and AI
            agents (via MCP) access your projects. Treat them like passwords.
          </p>

          <div className="mb-4 flex gap-2">
            <input
              placeholder="Token name, e.g. claude-mcp"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-64 rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
            />
            <button
              onClick={() => create.mutate()}
              disabled={!name || create.isPending}
              className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
            >
              Create token
            </button>
          </div>

          {created?.token && (
            <div className="mb-4 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm">
              <p className="mb-1 font-medium text-amber-800">
                Copy this token now — it won't be shown again:
              </p>
              <code className="block break-all rounded bg-white dark:bg-gray-900 p-2 text-xs">{created.token}</code>
            </div>
          )}

          <ul className="divide-y divide-gray-200 dark:divide-gray-800 rounded-md border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
            {tokens.data?.map((t) => (
              <li key={t.id} className="flex items-center justify-between px-4 py-3 text-sm">
                <div>
                  <span className="font-medium text-gray-900 dark:text-gray-100">{t.name}</span>
                  <span className="ml-2 text-xs text-gray-400">
                    {t.scopes.join(", ")} · created {new Date(t.createdAt).toLocaleDateString()}
                    {t.lastUsedAt && ` · last used ${new Date(t.lastUsedAt).toLocaleString()}`}
                  </span>
                </div>
                <button onClick={() => remove.mutate(t.id)} className="text-xs text-red-500 hover:underline">
                  Revoke
                </button>
              </li>
            ))}
            {tokens.data?.length === 0 && (
              <li className="px-4 py-3 text-sm text-gray-400">No tokens yet.</li>
            )}
          </ul>
        </section>

        <section>
          <h2 className="mb-2 text-base font-semibold text-gray-900 dark:text-gray-100">Connect an AI agent (MCP)</h2>
          <p className="mb-2 text-sm text-gray-500 dark:text-gray-400">
            Point any MCP-capable agent (e.g. Claude Code) at this server, using a token from above:
          </p>
          <pre className="overflow-x-auto rounded-md bg-gray-900 p-4 text-xs text-gray-100">
{`# Remote (streamable HTTP):
claude mcp add typstpad ${window.location.origin}/api/mcp \\
  --transport http --header "Authorization: Bearer tfp_..."

# Or via the CLI binary (stdio):
typstpad mcp --url ${window.location.origin} --token tfp_...`}
          </pre>
        </section>
      </main>
    </div>
  );
}

const cardCls = "rounded-md border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900";
const inputCls =
  "w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm focus:border-indigo-500 focus:outline-none dark:border-gray-700 dark:bg-gray-800";

function ProfileSection() {
  const me = useMe();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [color, setColor] = useState("");
  const [seeded, setSeeded] = useState(false);
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [pwMsg, setPwMsg] = useState("");
  const [profMsg, setProfMsg] = useState("");

  // Seed the form once from the loaded user.
  if (me.data && !seeded) {
    setName(me.data.name);
    setColor(me.data.color);
    setSeeded(true);
  }

  const saveProfile = useMutation({
    mutationFn: () => api.patch("/api/auth/me", { name, color }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["me"] });
      setProfMsg("Saved");
      setTimeout(() => setProfMsg(""), 2000);
    },
    onError: (e: Error) => setProfMsg(e.message),
  });
  const changePw = useMutation({
    mutationFn: () => api.post("/api/auth/change-password", { currentPassword: cur, newPassword: next }),
    onSuccess: () => {
      setCur("");
      setNext("");
      setPwMsg("Password changed");
      setTimeout(() => setPwMsg(""), 2500);
    },
    onError: (e: Error) => setPwMsg(e.message),
  });

  return (
    <section className={cardCls}>
      <h2 className="mb-3 text-base font-semibold text-gray-900 dark:text-gray-100">Profile</h2>
      <div className="flex items-end gap-3">
        <label className="flex-1">
          <span className="text-xs font-medium text-gray-500 dark:text-gray-400">Display name</span>
          <input className={`mt-1 ${inputCls}`} value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label>
          <span className="text-xs font-medium text-gray-500 dark:text-gray-400">Color</span>
          <input type="color" className="mt-1 h-9 w-12 rounded border border-gray-300 dark:border-gray-700" value={color} onChange={(e) => setColor(e.target.value)} />
        </label>
        <button onClick={() => saveProfile.mutate()} className="h-9 rounded-md bg-indigo-600 px-4 text-sm font-medium text-white hover:bg-indigo-700">
          Save
        </button>
        {profMsg && <span className="pb-2 text-sm text-green-600 dark:text-green-400">{profMsg}</span>}
      </div>

      {me.data?.hasPassword && (
        <>
          <h3 className="mb-2 mt-6 text-sm font-semibold text-gray-800 dark:text-gray-200">Change password</h3>
          <div className="flex items-end gap-3">
            <label className="flex-1">
              <span className="text-xs font-medium text-gray-500 dark:text-gray-400">Current password</span>
              <input type="password" className={`mt-1 ${inputCls}`} value={cur} onChange={(e) => setCur(e.target.value)} />
            </label>
            <label className="flex-1">
              <span className="text-xs font-medium text-gray-500 dark:text-gray-400">New password</span>
              <input type="password" className={`mt-1 ${inputCls}`} value={next} onChange={(e) => setNext(e.target.value)} />
            </label>
            <button
              onClick={() => changePw.mutate()}
              disabled={!cur || next.length < 8}
              className="h-9 rounded-md bg-indigo-600 px-4 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
            >
              Update
            </button>
          </div>
          {pwMsg && <p className="mt-2 text-sm text-green-600 dark:text-green-400">{pwMsg}</p>}
        </>
      )}
    </section>
  );
}
