import { useState } from "react";
import { useNavigate } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type AuthConfig } from "../api/client";

export default function LoginPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const config = useQuery<AuthConfig>({
    queryKey: ["authConfig"],
    queryFn: () => api.get<AuthConfig>("/api/auth/config"),
  });

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      if (mode === "login") {
        await api.post("/api/auth/login", { email, password });
      } else {
        await api.post("/api/auth/register", { email, name, password });
      }
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      navigate("/projects");
    } catch (err: any) {
      setError(err.message ?? "failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex h-full items-center justify-center bg-gray-50">
      <div className="w-full max-w-sm rounded-xl border border-gray-200 bg-white p-8 shadow-sm">
        <h1 className="mb-1 text-center text-2xl font-bold text-gray-900">TypstPad</h1>
        <p className="mb-6 text-center text-sm text-gray-500">
          Collaborative Typst editing, self-hosted.
        </p>

        <form onSubmit={submit} className="space-y-3">
          <input
            type="email"
            required
            placeholder="Email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
          />
          {mode === "register" && (
            <input
              type="text"
              placeholder="Display name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
            />
          )}
          <input
            type="password"
            required
            placeholder="Password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
          />
          {error && <p className="text-sm text-red-600">{error}</p>}
          <button
            type="submit"
            disabled={busy}
            className="w-full rounded-md bg-indigo-600 px-3 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {mode === "login" ? "Sign in" : "Create account"}
          </button>
        </form>

        {config.data?.oidcEnabled && (
          <a
            href="/api/auth/oidc/login"
            className="mt-3 block w-full rounded-md border border-gray-300 px-3 py-2 text-center text-sm font-medium text-gray-700 hover:bg-gray-50"
          >
            Sign in with SSO
          </a>
        )}

        <p className="mt-4 text-center text-sm text-gray-500">
          {mode === "login" ? (
            <>
              No account?{" "}
              <button className="text-indigo-600 hover:underline" onClick={() => setMode("register")}>
                Register
              </button>
            </>
          ) : (
            <>
              Have an account?{" "}
              <button className="text-indigo-600 hover:underline" onClick={() => setMode("login")}>
                Sign in
              </button>
            </>
          )}
        </p>
      </div>
    </div>
  );
}
