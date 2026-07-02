import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type AdminSettings, type User } from "../api/client";
import { useMe } from "../App";
import { ThemeToggle } from "../theme";

export default function AdminPage() {
  return (
    <div className="min-h-full bg-gray-50 dark:bg-gray-950">
      <header className="border-b border-gray-200 bg-white px-6 py-4 dark:border-gray-800 dark:bg-gray-900">
        <div className="mx-auto flex max-w-3xl items-center gap-4">
          <Link to="/projects" className="text-sm text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">
            ← Projects
          </Link>
          <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Administration</h1>
          <ThemeToggle className="ml-auto text-base" />
        </div>
      </header>
      <main className="mx-auto max-w-3xl space-y-8 px-6 py-8">
        <ServerSettings />
        <Users />
      </main>
    </div>
  );
}

const card = "rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900";
const input =
  "w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm focus:border-indigo-500 focus:outline-none dark:border-gray-700 dark:bg-gray-800";
const label = "block text-xs font-medium text-gray-500 dark:text-gray-400";

function Field({ label: l, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className={label}>{l}</span>
      <div className="mt-1">{children}</div>
    </label>
  );
}

function ServerSettings() {
  const queryClient = useQueryClient();
  const { data } = useQuery<AdminSettings>({
    queryKey: ["adminSettings"],
    queryFn: () => api.get("/api/admin/settings"),
  });
  const [form, setForm] = useState<Partial<AdminSettings> & { smtpPassword?: string; oidcClientSecret?: string }>({});
  const [saved, setSaved] = useState("");
  const [error, setError] = useState("");

  // Seed the editable form from server values once loaded.
  useEffect(() => {
    if (data) setForm({ ...data, smtpPassword: "", oidcClientSecret: "" });
  }, [data]);

  const save = useMutation({
    mutationFn: () => api.put<AdminSettings>("/api/admin/settings", form),
    onSuccess: (res) => {
      setForm({ ...res, smtpPassword: "", oidcClientSecret: "" });
      queryClient.invalidateQueries({ queryKey: ["adminSettings"] });
      queryClient.invalidateQueries({ queryKey: ["authConfig"] });
      setSaved("Saved");
      setError("");
      setTimeout(() => setSaved(""), 2500);
    },
    onError: (e: Error) => {
      setError(e.message);
      setSaved("");
    },
  });

  const set = (patch: Partial<typeof form>) => setForm((f) => ({ ...f, ...patch }));
  if (!data) return <section className={card}>Loading…</section>;

  return (
    <section className={card}>
      <h2 className="mb-4 text-base font-semibold text-gray-900 dark:text-gray-100">Server settings</h2>

      {/* Registration + allowlist */}
      <div className="space-y-3">
        <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input type="checkbox" checked={!!form.allowRegistration} onChange={(e) => set({ allowRegistration: e.target.checked })} />
          Allow new users to register with email/password
        </label>
        <Field label="Sign-up allowlist (domains or exact emails, comma-separated; empty = anyone)">
          <input className={input} value={form.signupAllowlist ?? ""} onChange={(e) => set({ signupAllowlist: e.target.value })} placeholder="ics.red, purdue.edu, vert3x.io" />
        </Field>
      </div>

      {/* Email / SMTP */}
      <h3 className="mb-2 mt-6 flex items-center gap-2 text-sm font-semibold text-gray-800 dark:text-gray-200">
        Email verification (SMTP / Amazon SES)
        <StatusPill on={data.emailVerificationActive} onText="active" offText="off" />
      </h3>
      <div className="grid grid-cols-2 gap-3">
        <Field label="SMTP host"><input className={input} value={form.smtpHost ?? ""} onChange={(e) => set({ smtpHost: e.target.value })} placeholder="email-smtp.us-east-1.amazonaws.com" /></Field>
        <Field label="SMTP port"><input className={input} value={String(form.smtpPort ?? "")} onChange={(e) => set({ smtpPort: Number(e.target.value) as any })} placeholder="587" /></Field>
        <Field label="SMTP username"><input className={input} value={form.smtpUsername ?? ""} onChange={(e) => set({ smtpUsername: e.target.value })} /></Field>
        <Field label={`SMTP password ${data.smtpPasswordSet ? "(set — leave blank to keep)" : ""}`}>
          <input className={input} type="password" value={form.smtpPassword ?? ""} onChange={(e) => set({ smtpPassword: e.target.value })} placeholder={data.smtpPasswordSet ? "••••••••" : ""} />
        </Field>
        <Field label="From address"><input className={input} value={form.smtpFrom ?? ""} onChange={(e) => set({ smtpFrom: e.target.value })} placeholder="noreply@ics.red" /></Field>
        <Field label="From name"><input className={input} value={form.smtpFromName ?? ""} onChange={(e) => set({ smtpFromName: e.target.value })} /></Field>
      </div>
      <label className="mt-3 flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input type="checkbox" checked={!!form.requireEmailVerification} onChange={(e) => set({ requireEmailVerification: e.target.checked })} />
        Require email verification before sign-in (needs SMTP configured)
      </label>

      {/* OIDC / SSO */}
      <h3 className="mb-2 mt-6 flex items-center gap-2 text-sm font-semibold text-gray-800 dark:text-gray-200">
        Single sign-on (OIDC)
        <StatusPill on={data.oidcActive} onText="active" offText="off" />
      </h3>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Issuer URL"><input className={input} value={form.oidcIssuer ?? ""} onChange={(e) => set({ oidcIssuer: e.target.value })} placeholder="https://team.cloudflareaccess.com" /></Field>
        <Field label="Scopes"><input className={input} value={form.oidcScopes ?? ""} onChange={(e) => set({ oidcScopes: e.target.value })} placeholder="openid profile email" /></Field>
        <Field label="Client ID"><input className={input} value={form.oidcClientId ?? ""} onChange={(e) => set({ oidcClientId: e.target.value })} /></Field>
        <Field label={`Client secret ${data.oidcClientSecretSet ? "(set — leave blank to keep)" : ""}`}>
          <input className={input} type="password" value={form.oidcClientSecret ?? ""} onChange={(e) => set({ oidcClientSecret: e.target.value })} placeholder={data.oidcClientSecretSet ? "••••••••" : ""} />
        </Field>
      </div>
      <p className="mt-2 text-xs text-gray-400">
        Redirect URI to register with your IdP: <code className="rounded bg-gray-100 px-1 dark:bg-gray-800">{window.location.origin}/api/auth/oidc/callback</code>
      </p>

      <div className="mt-6 flex items-center gap-3">
        <button
          onClick={() => save.mutate()}
          disabled={save.isPending}
          className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
        >
          Save settings
        </button>
        {saved && <span className="text-sm text-green-600 dark:text-green-400">{saved}</span>}
        {error && <span className="text-sm text-red-600 dark:text-red-400">{error}</span>}
      </div>
    </section>
  );
}

function StatusPill({ on, onText, offText }: { on: boolean; onText: string; offText: string }) {
  return (
    <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${on ? "bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300" : "bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400"}`}>
      {on ? onText : offText}
    </span>
  );
}

function Users() {
  const me = useMe();
  const queryClient = useQueryClient();
  const users = useQuery<User[]>({ queryKey: ["adminUsers"], queryFn: () => api.get("/api/admin/users") });

  const setAdmin = useMutation({
    mutationFn: ({ id, isAdmin }: { id: string; isAdmin: boolean }) => api.patch(`/api/admin/users/${id}`, { isAdmin }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["adminUsers"] }),
  });
  const deleteUser = useMutation({
    mutationFn: (id: string) => api.del(`/api/admin/users/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["adminUsers"] }),
  });

  return (
    <section>
      <h2 className="mb-3 text-base font-semibold text-gray-900 dark:text-gray-100">Users</h2>
      <ul className="divide-y divide-gray-200 rounded-md border border-gray-200 bg-white dark:divide-gray-800 dark:border-gray-800 dark:bg-gray-900">
        {users.data?.map((u) => (
          <li key={u.id} className="flex items-center justify-between px-4 py-3 text-sm">
            <div className="flex items-center gap-3">
              <span className="flex h-7 w-7 items-center justify-center rounded-full text-xs font-semibold text-white" style={{ backgroundColor: u.color }}>
                {u.name[0]?.toUpperCase()}
              </span>
              <div>
                <span className="font-medium text-gray-900 dark:text-gray-100">{u.name}</span>
                <span className="ml-2 text-gray-400">{u.email}</span>
                {u.isAdmin && <span className="ml-2 rounded bg-indigo-100 px-1.5 py-0.5 text-xs text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300">admin</span>}
                {!u.emailVerified && <span className="ml-2 rounded bg-amber-100 px-1.5 py-0.5 text-xs text-amber-700 dark:bg-amber-950 dark:text-amber-300">unverified</span>}
              </div>
            </div>
            {u.id !== me.data?.id && (
              <div className="flex gap-3 text-xs">
                <button className="text-gray-500 hover:underline dark:text-gray-400" onClick={() => setAdmin.mutate({ id: u.id, isAdmin: !u.isAdmin })}>
                  {u.isAdmin ? "Remove admin" : "Make admin"}
                </button>
                <button
                  className="text-red-500 hover:underline"
                  onClick={() => {
                    if (confirm(`Delete user ${u.email}? Their projects will be removed.`)) deleteUser.mutate(u.id);
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
  );
}
