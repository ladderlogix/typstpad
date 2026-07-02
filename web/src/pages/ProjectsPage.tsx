import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Project } from "../api/client";
import { useMe } from "../App";

export default function ProjectsPage() {
  const me = useMe();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [query, setQuery] = useState("");
  const [showNew, setShowNew] = useState(false);

  const projects = useQuery<Project[]>({
    queryKey: ["projects", query],
    queryFn: () => api.get<Project[]>(`/api/projects?q=${encodeURIComponent(query)}`),
  });
  const templates = useQuery<Project[]>({
    queryKey: ["templates"],
    queryFn: () => api.get<Project[]>("/api/templates"),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/api/projects/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["projects"] }),
  });
  const duplicate = useMutation({
    mutationFn: (id: string) => api.post<Project>(`/api/projects/${id}/duplicate`, {}),
    onSuccess: (p) => navigate(`/p/${p.id}`),
  });

  async function logout() {
    await api.post("/api/auth/logout");
    queryClient.clear();
    navigate("/login");
  }

  return (
    <div className="min-h-full bg-gray-50">
      <header className="border-b border-gray-200 bg-white">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
          <h1 className="text-xl font-bold text-gray-900">TypstPad</h1>
          <div className="flex items-center gap-4 text-sm">
            {me.data?.isAdmin && (
              <Link to="/admin" className="text-gray-500 hover:text-gray-900">
                Admin
              </Link>
            )}
            <Link to="/settings" className="text-gray-500 hover:text-gray-900">
              Settings
            </Link>
            <span
              className="flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold text-white"
              style={{ backgroundColor: me.data?.color }}
              title={me.data?.email}
            >
              {me.data?.name?.[0]?.toUpperCase()}
            </span>
            <button onClick={logout} className="text-gray-500 hover:text-gray-900">
              Sign out
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-6 py-8">
        <div className="mb-6 flex items-center justify-between gap-4">
          <input
            type="search"
            placeholder="Search projects…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-64 rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
          />
          <button
            onClick={() => setShowNew(true)}
            className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700"
          >
            New project
          </button>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.data?.map((p) => (
            <div
              key={p.id}
              className="group rounded-lg border border-gray-200 bg-white p-4 shadow-sm transition hover:border-indigo-300 hover:shadow"
            >
              <Link to={`/p/${p.id}`} className="block">
                <h3 className="font-semibold text-gray-900">{p.name}</h3>
                <p className="mt-1 line-clamp-2 text-sm text-gray-500">{p.description || " "}</p>
                <p className="mt-2 text-xs text-gray-400">
                  {p.role} · updated {new Date(p.updatedAt).toLocaleString()}
                </p>
              </Link>
              <div className="mt-2 flex gap-3 text-xs text-gray-400 opacity-0 transition group-hover:opacity-100">
                <button className="hover:text-gray-700" onClick={() => duplicate.mutate(p.id)}>
                  Duplicate
                </button>
                {p.role === "owner" && (
                  <button
                    className="hover:text-red-600"
                    onClick={() => {
                      if (confirm(`Delete project "${p.name}"?`)) remove.mutate(p.id);
                    }}
                  >
                    Delete
                  </button>
                )}
              </div>
            </div>
          ))}
          {projects.data?.length === 0 && (
            <p className="col-span-full py-12 text-center text-gray-400">
              No projects yet — create one from a template.
            </p>
          )}
        </div>
      </main>

      {showNew && (
        <NewProjectDialog templates={templates.data ?? []} onClose={() => setShowNew(false)} />
      )}
    </div>
  );
}

function NewProjectDialog({ templates, onClose }: { templates: Project[]; onClose: () => void }) {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [templateId, setTemplateId] = useState<string>("");
  const [error, setError] = useState("");

  const create = useMutation({
    mutationFn: () =>
      api.post<Project>("/api/projects", { name: name || "Untitled project", templateId }),
    onSuccess: (p) => navigate(`/p/${p.id}`),
    onError: (err: Error) => setError(err.message),
  });

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30" onClick={onClose}>
      <div
        className="w-full max-w-lg rounded-xl bg-white p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-lg font-semibold text-gray-900">New project</h2>
        <input
          autoFocus
          placeholder="Project name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="mb-4 w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
        />
        <p className="mb-2 text-sm font-medium text-gray-700">Template</p>
        <div className="mb-4 grid max-h-64 grid-cols-2 gap-2 overflow-y-auto">
          <TemplateCard
            name="Blank"
            description="Empty document"
            selected={templateId === ""}
            onClick={() => setTemplateId("")}
          />
          {templates.map((t) => (
            <TemplateCard
              key={t.id}
              name={t.name}
              description={t.templateMeta?.description ?? t.description}
              selected={templateId === t.id}
              onClick={() => setTemplateId(t.id)}
            />
          ))}
        </div>
        {error && <p className="mb-2 text-sm text-red-600">{error}</p>}
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="rounded-md px-4 py-2 text-sm text-gray-600 hover:bg-gray-100">
            Cancel
          </button>
          <button
            onClick={() => create.mutate()}
            disabled={create.isPending}
            className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            Create
          </button>
        </div>
      </div>
    </div>
  );
}

function TemplateCard({
  name,
  description,
  selected,
  onClick,
}: {
  name: string;
  description?: string;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`rounded-lg border p-3 text-left text-sm transition ${
        selected ? "border-indigo-500 bg-indigo-50" : "border-gray-200 hover:border-gray-300"
      }`}
    >
      <span className="block font-medium text-gray-900">{name}</span>
      <span className="mt-0.5 line-clamp-2 block text-xs text-gray-500">{description}</span>
    </button>
  );
}
