import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type APIToken } from "../api/client";

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
    <div className="min-h-full bg-gray-50">
      <header className="border-b border-gray-200 bg-white px-6 py-4">
        <div className="mx-auto flex max-w-3xl items-center gap-4">
          <Link to="/projects" className="text-sm text-gray-500 hover:text-gray-900">
            ← Projects
          </Link>
          <h1 className="text-lg font-semibold text-gray-900">Settings</h1>
        </div>
      </header>

      <main className="mx-auto max-w-3xl space-y-8 px-6 py-8">
        <section>
          <h2 className="mb-2 text-base font-semibold text-gray-900">API tokens</h2>
          <p className="mb-4 text-sm text-gray-500">
            Tokens let scripts, the <code className="rounded bg-gray-100 px-1">typstpad</code> CLI and AI
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
              <code className="block break-all rounded bg-white p-2 text-xs">{created.token}</code>
            </div>
          )}

          <ul className="divide-y divide-gray-200 rounded-md border border-gray-200 bg-white">
            {tokens.data?.map((t) => (
              <li key={t.id} className="flex items-center justify-between px-4 py-3 text-sm">
                <div>
                  <span className="font-medium text-gray-900">{t.name}</span>
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
          <h2 className="mb-2 text-base font-semibold text-gray-900">Connect an AI agent (MCP)</h2>
          <p className="mb-2 text-sm text-gray-500">
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
