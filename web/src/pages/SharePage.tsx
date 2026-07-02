import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { api } from "../api/client";

// Public, no-auth page: renders a shared project's server-compiled PDF in an
// iframe. Anyone with the link can view; nobody can edit.
export default function SharePage() {
  const { token } = useParams();
  const [name, setName] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const pdfURL = `/api/public/${token}/pdf`;

  useEffect(() => {
    let cancelled = false;
    api
      .get<{ name: string }>(`/api/public/${token}`)
      .then((d) => !cancelled && setName(d.name))
      .catch((e: Error) => !cancelled && setError(e.message || "This link is invalid or has been turned off."));
    return () => {
      cancelled = true;
    };
  }, [token]);

  if (error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 bg-gray-50 text-center dark:bg-gray-950">
        <p className="text-lg font-semibold text-gray-800 dark:text-gray-100">Link unavailable</p>
        <p className="max-w-sm text-sm text-gray-500 dark:text-gray-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col bg-gray-100 dark:bg-gray-950">
      <header className="flex items-center gap-3 border-b border-gray-200 bg-white px-4 py-2 dark:border-gray-800 dark:bg-gray-900">
        <span className="text-lg font-bold text-gray-900 dark:text-gray-100">TypstPad</span>
        <span className="truncate text-sm text-gray-500 dark:text-gray-400">
          {name ?? "Loading…"}
        </span>
        <span className="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-gray-500 dark:bg-gray-800 dark:text-gray-400">
          read-only
        </span>
        <a
          href={pdfURL}
          download
          className="ml-auto rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-indigo-700"
        >
          Download PDF
        </a>
      </header>
      <iframe title={name ?? "Shared document"} src={pdfURL} className="min-h-0 flex-1 border-0" />
    </div>
  );
}
