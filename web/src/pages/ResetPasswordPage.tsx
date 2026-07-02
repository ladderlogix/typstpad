import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { api } from "../api/client";

export default function ResetPasswordPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const token = params.get("token") ?? "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const card = "w-full max-w-sm rounded-xl border border-gray-200 bg-white p-8 shadow-sm dark:border-gray-800 dark:bg-gray-900";
  const input =
    "w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-indigo-500 focus:outline-none dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100";

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (password !== confirm) {
      setError("Passwords don't match.");
      return;
    }
    setBusy(true);
    try {
      await api.post("/api/auth/reset-password", { token, password });
      navigate("/login?reset=success");
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex h-full items-center justify-center">
      <div className={card}>
        <h1 className="mb-4 text-center text-xl font-bold text-gray-900 dark:text-gray-100">Choose a new password</h1>
        {!token ? (
          <p className="text-center text-sm text-red-600 dark:text-red-400">This reset link is missing its token.</p>
        ) : (
          <form onSubmit={submit} className="space-y-3">
            <input type="password" required placeholder="New password" value={password} onChange={(e) => setPassword(e.target.value)} className={input} />
            <input type="password" required placeholder="Confirm new password" value={confirm} onChange={(e) => setConfirm(e.target.value)} className={input} />
            {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
            <button type="submit" disabled={busy} className="w-full rounded-md bg-indigo-600 px-3 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50">
              Reset password
            </button>
          </form>
        )}
        <Link to="/login" className="mt-4 block text-center text-sm text-indigo-600 hover:underline dark:text-indigo-400">
          Back to sign in
        </Link>
      </div>
    </div>
  );
}
