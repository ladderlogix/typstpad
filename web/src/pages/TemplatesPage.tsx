import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Project } from "../api/client";
import { useMe } from "../App";
import { ThemeToggle } from "../theme";

export default function TemplatesPage() {
  const me = useMe();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [category, setCategory] = useState<string>("All");

  const templates = useQuery<Project[]>({
    queryKey: ["templates"],
    queryFn: () => api.get<Project[]>("/api/templates"),
  });

  const use = useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) =>
      api.post<Project>(`/api/templates/${id}/use`, { name }),
    onSuccess: (p) => navigate(`/p/${p.id}`),
    onError: (e: Error) => alert(e.message),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/api/templates/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["templates"] }),
    onError: (e: Error) => alert(e.message),
  });

  const categories = useMemo(() => {
    const set = new Set<string>();
    templates.data?.forEach((t) => set.add(t.templateMeta?.category || "Other"));
    return ["All", ...Array.from(set).sort()];
  }, [templates.data]);

  const shown = (templates.data ?? []).filter(
    (t) => category === "All" || (t.templateMeta?.category || "Other") === category
  );

  return (
    <div className="min-h-full bg-gray-50 dark:bg-gray-950">
      <header className="border-b border-gray-200 bg-white px-6 py-4 dark:border-gray-800 dark:bg-gray-900">
        <div className="mx-auto flex max-w-5xl items-center gap-4">
          <Link to="/projects" className="text-sm text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">
            ← Projects
          </Link>
          <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Template gallery</h1>
          <ThemeToggle className="ml-auto text-base" />
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-6 py-8">
        <div className="mb-6 flex flex-wrap gap-2">
          {categories.map((c) => (
            <button
              key={c}
              onClick={() => setCategory(c)}
              className={`rounded-full px-3 py-1 text-sm ${
                category === c
                  ? "bg-indigo-600 text-white"
                  : "bg-white text-gray-600 hover:bg-gray-100 dark:bg-gray-900 dark:text-gray-300 dark:hover:bg-gray-800"
              }`}
            >
              {c}
            </button>
          ))}
        </div>

        <div className="grid grid-cols-2 gap-5 sm:grid-cols-3 lg:grid-cols-4">
          {shown.map((t) => (
            <TemplateCard
              key={t.id}
              t={t}
              canDelete={t.ownerId === me.data?.id || !!me.data?.isAdmin}
              onUse={() => {
                const name = prompt(`New project name:`, t.name);
                if (name) use.mutate({ id: t.id, name });
              }}
              onDelete={() => {
                if (confirm(`Remove template "${t.name}" from the gallery?`)) remove.mutate(t.id);
              }}
            />
          ))}
          {templates.data?.length === 0 && (
            <p className="col-span-full py-12 text-center text-gray-400">No templates yet.</p>
          )}
        </div>
      </main>
    </div>
  );
}

function TemplateCard({
  t,
  canDelete,
  onUse,
  onDelete,
}: {
  t: Project;
  canDelete: boolean;
  onUse: () => void;
  onDelete: () => void;
}) {
  const [imgFailed, setImgFailed] = useState(false);
  return (
    <div className="group flex flex-col overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm transition hover:border-indigo-300 hover:shadow-md dark:border-gray-800 dark:bg-gray-900">
      <div className="relative aspect-[3/4] overflow-hidden border-b border-gray-100 bg-gray-100 dark:border-gray-800 dark:bg-gray-800">
        {!imgFailed ? (
          <img
            src={`/api/templates/${t.id}/thumbnail.png`}
            alt={t.name}
            loading="lazy"
            onError={() => setImgFailed(true)}
            className="h-full w-full bg-white object-contain object-top"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-3xl text-gray-300 dark:text-gray-600">📄</div>
        )}
        <div className="absolute inset-0 flex items-center justify-center gap-2 bg-black/40 opacity-0 transition group-hover:opacity-100">
          <button onClick={onUse} className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-indigo-700">
            Use template
          </button>
          <Link
            to={`/p/${t.id}`}
            className="rounded-md bg-white/90 px-3 py-1.5 text-sm font-medium text-gray-800 hover:bg-white"
          >
            Preview
          </Link>
        </div>
      </div>
      <div className="flex flex-1 flex-col p-3">
        <div className="flex items-center justify-between">
          <h3 className="truncate font-semibold text-gray-900 dark:text-gray-100">{t.name}</h3>
          {t.templateMeta?.category && (
            <span className="ml-1 shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400">
              {t.templateMeta.category}
            </span>
          )}
        </div>
        <p className="mt-1 line-clamp-2 flex-1 text-xs text-gray-500 dark:text-gray-400">
          {t.templateMeta?.description || t.description}
        </p>
        {canDelete && (
          <button onClick={onDelete} className="mt-2 self-start text-[11px] text-red-500 hover:underline">
            Remove
          </button>
        )}
      </div>
    </div>
  );
}
