import { useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Collection, type Project } from "../api/client";
import { useMe } from "../App";
import { ThemeToggle } from "../theme";

export default function ProjectsPage() {
  const me = useMe();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [query, setQuery] = useState("");
  const [showNew, setShowNew] = useState(false);
  const [activeCollection, setActiveCollection] = useState<string>(""); // "" = all

  const projects = useQuery<Project[]>({
    queryKey: ["projects", query, activeCollection],
    queryFn: () =>
      api.get<Project[]>(
        `/api/projects?q=${encodeURIComponent(query)}${activeCollection ? `&collection=${activeCollection}` : ""}`
      ),
  });
  const templates = useQuery<Project[]>({
    queryKey: ["templates"],
    queryFn: () => api.get<Project[]>("/api/templates"),
  });
  const collections = useQuery<Collection[]>({
    queryKey: ["collections"],
    queryFn: () => api.get<Collection[]>("/api/collections"),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/api/projects/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["projects"] }),
  });
  const duplicate = useMutation({
    mutationFn: (id: string) => api.post<Project>(`/api/projects/${id}/duplicate`, {}),
    onSuccess: (p) => navigate(`/p/${p.id}`),
  });
  const newCollection = useMutation({
    mutationFn: (name: string) => api.post<Collection>("/api/collections", { name }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["collections"] }),
  });
  const deleteCollection = useMutation({
    mutationFn: (id: string) => api.del(`/api/collections/${id}`),
    onSuccess: () => {
      setActiveCollection("");
      queryClient.invalidateQueries({ queryKey: ["collections"] });
    },
  });
  const [projectMenu, setProjectMenu] = useState<Project | null>(null);

  const importInput = useRef<HTMLInputElement>(null);
  const importProject = useMutation({
    mutationFn: (file: File) => {
      const form = new FormData();
      form.append("file", file);
      return api.post<Project>("/api/projects/import", form);
    },
    onSuccess: (p) => navigate(`/p/${p.id}`),
    onError: (e: Error) => alert(`Import failed: ${e.message}`),
  });

  async function logout() {
    await api.post("/api/auth/logout");
    queryClient.clear();
    navigate("/login");
  }

  return (
    <div className="min-h-full bg-gray-50 dark:bg-gray-950">
      <header className="border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
          <h1 className="text-xl font-bold text-gray-900 dark:text-gray-100">TypstPad</h1>
          <div className="flex items-center gap-4 text-sm">
            <ThemeToggle className="text-base" />
            <Link to="/templates" className="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:text-gray-100">
              Templates
            </Link>
            <Link to="/teams" className="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:text-gray-100">
              Teams
            </Link>
            {me.data?.isAdmin && (
              <Link to="/admin" className="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:text-gray-100">
                Admin
              </Link>
            )}
            <Link to="/settings" className="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:text-gray-100">
              Settings
            </Link>
            <span
              className="flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold text-white"
              style={{ backgroundColor: me.data?.color }}
              title={me.data?.email}
            >
              {me.data?.name?.[0]?.toUpperCase()}
            </span>
            <button onClick={logout} className="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:text-gray-100">
              Sign out
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto grid max-w-6xl grid-cols-[14rem_1fr] gap-6 px-6 py-8">
        <aside>
          <button
            onClick={() => setActiveCollection("")}
            className={`mb-1 w-full rounded-md px-3 py-1.5 text-left text-sm ${
              activeCollection === ""
                ? "bg-indigo-50 font-medium text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300"
                : "text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
            }`}
          >
            All projects
          </button>
          <p className="mb-1 mt-4 px-3 text-xs font-semibold uppercase tracking-wide text-gray-400">Collections</p>
          {collections.data?.map((c) => (
            <div key={c.id} className="group flex items-center">
              <button
                onClick={() => setActiveCollection(c.id)}
                className={`flex-1 truncate rounded-md px-3 py-1.5 text-left text-sm ${
                  activeCollection === c.id
                    ? "bg-indigo-50 font-medium text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300"
                    : "text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
                }`}
              >
                {c.name} <span className="text-xs text-gray-400">{c.count}</span>
              </button>
              <button
                className="hidden px-1 text-xs text-gray-400 hover:text-red-600 group-hover:block"
                title="Delete collection"
                onClick={() => {
                  if (confirm(`Delete collection "${c.name}"? (projects are not deleted)`)) deleteCollection.mutate(c.id);
                }}
              >
                x
              </button>
            </div>
          ))}
          <button
            onClick={() => {
              const name = prompt("New collection name:");
              if (name) newCollection.mutate(name);
            }}
            className="mt-1 w-full rounded-md px-3 py-1.5 text-left text-sm text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
          >
            + New collection
          </button>
        </aside>

        <div>
        <div className="mb-6 flex items-center justify-between gap-4">
          <input
            type="search"
            placeholder="Search projects…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-64 rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
          />
          <div className="flex items-center gap-2">
            <input
              ref={importInput}
              type="file"
              accept=".zip,application/zip"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) importProject.mutate(f);
                e.target.value = "";
              }}
            />
            <button
              onClick={() => importInput.current?.click()}
              disabled={importProject.isPending}
              className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
              title="Import a project from a .zip of Typst source and assets"
            >
              {importProject.isPending ? "Importing…" : "Import ZIP"}
            </button>
            <button
              onClick={() => setShowNew(true)}
              className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700"
            >
              New project
            </button>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.data?.map((p) => (
            <div
              key={p.id}
              className="group rounded-lg border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-4 shadow-sm transition hover:border-indigo-300 hover:shadow"
            >
              <Link to={`/p/${p.id}`} className="block">
                <h3 className="font-semibold text-gray-900 dark:text-gray-100">{p.name}</h3>
                <p className="mt-1 line-clamp-2 text-sm text-gray-500 dark:text-gray-400">{p.description || " "}</p>
                <p className="mt-2 text-xs text-gray-400">
                  {p.role} · updated {new Date(p.updatedAt).toLocaleString()}
                </p>
              </Link>
              <div className="mt-2 flex gap-3 text-xs text-gray-400 opacity-0 transition group-hover:opacity-100">
                <button className="hover:text-gray-700 dark:hover:text-gray-200" onClick={() => setProjectMenu(p)}>
                  Organize
                </button>
                <button className="hover:text-gray-700 dark:hover:text-gray-200" onClick={() => duplicate.mutate(p.id)}>
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
        </div>
      </main>

      {showNew && (
        <NewProjectDialog templates={templates.data ?? []} onClose={() => setShowNew(false)} />
      )}
      {projectMenu && (
        <OrganizeDialog project={projectMenu} collections={collections.data ?? []} onClose={() => setProjectMenu(null)} />
      )}
    </div>
  );
}

function OrganizeDialog({
  project,
  collections,
  onClose,
}: {
  project: Project;
  collections: Collection[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const membership = useQuery<string[]>({
    queryKey: ["projectCollections", project.id],
    queryFn: () => api.get<string[]>(`/api/projects/${project.id}/collections`),
  });
  const toggle = useMutation({
    mutationFn: ({ collectionId, member }: { collectionId: string; member: boolean }) =>
      member
        ? api.del(`/api/collections/${collectionId}/projects/${project.id}`)
        : api.post(`/api/collections/${collectionId}/projects`, { projectId: project.id }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projectCollections", project.id] });
      queryClient.invalidateQueries({ queryKey: ["collections"] });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
  const publish = useMutation({
    mutationFn: (v: { name: string; description: string; category: string }) =>
      api.post(`/api/projects/${project.id}/publish-template`, v),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["templates"] });
      alert("Published to the template gallery.");
      onClose();
    },
    onError: (e: Error) => alert(e.message),
  });
  const inSet = new Set(membership.data ?? []);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30" onClick={onClose}>
      <div className="w-full max-w-sm rounded-xl bg-white p-6 shadow-xl dark:bg-gray-900" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-1 text-lg font-semibold text-gray-900 dark:text-gray-100">Organize</h2>
        <p className="mb-4 truncate text-sm text-gray-500 dark:text-gray-400">{project.name}</p>

        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400">Collections</p>
        <div className="mb-4 space-y-1">
          {collections.length === 0 && (
            <p className="text-sm text-gray-400">No collections yet — create one from the sidebar.</p>
          )}
          {collections.map((c) => (
            <label key={c.id} className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input
                type="checkbox"
                checked={inSet.has(c.id)}
                onChange={() => toggle.mutate({ collectionId: c.id, member: inSet.has(c.id) })}
              />
              {c.name}
            </label>
          ))}
        </div>

        {project.role === "owner" && (
          <div className="border-t border-gray-100 pt-3 dark:border-gray-800">
            <button
              onClick={() => {
                const name = prompt("Template name:", project.name);
                if (name === null) return;
                const description = prompt("Short description:", project.description) ?? "";
                const category = prompt("Category (e.g. Academic, Business):", "Community") ?? "Community";
                publish.mutate({ name: name || project.name, description, category });
              }}
              className="text-sm text-indigo-600 hover:underline dark:text-indigo-400"
            >
              Publish as template →
            </button>
          </div>
        )}

        <div className="mt-4 flex justify-end">
          <button onClick={onClose} className="rounded-md px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800">
            Done
          </button>
        </div>
      </div>
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
        className="w-full max-w-lg rounded-xl bg-white dark:bg-gray-900 p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-lg font-semibold text-gray-900 dark:text-gray-100">New project</h2>
        <input
          autoFocus
          placeholder="Project name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="mb-4 w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
        />
        <p className="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">Template</p>
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
          <button onClick={onClose} className="rounded-md px-4 py-2 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800">
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
        selected ? "border-indigo-500 bg-indigo-50" : "border-gray-200 dark:border-gray-800 hover:border-gray-300"
      }`}
    >
      <span className="block font-medium text-gray-900 dark:text-gray-100">{name}</span>
      <span className="mt-0.5 line-clamp-2 block text-xs text-gray-500 dark:text-gray-400">{description}</span>
    </button>
  );
}
