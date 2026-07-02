import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type AuthConfig } from "../api/client";
import { ThemeToggle } from "../theme";

export default function LoginPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [params] = useSearchParams();
  const [mode, setMode] = useState<"login" | "register" | "forgot">("login");
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [forgotSent, setForgotSent] = useState(false);
  // When verification is required: after register, or after a login attempt on
  // an unverified account, prompt to check email / resend.
  const [pendingEmail, setPendingEmail] = useState("");
  const [resent, setResent] = useState(false);

  const config = useQuery<AuthConfig>({
    queryKey: ["authConfig"],
    queryFn: () => api.get<AuthConfig>("/api/auth/config"),
  });

  const verifyBanner = params.get("verify"); // success | invalid

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setResent(false);
    setBusy(true);
    try {
      if (mode === "forgot") {
        await api.post("/api/auth/forgot-password", { email });
        setForgotSent(true);
        return;
      }
      if (mode === "login") {
        await api.post("/api/auth/login", { email, password });
        await queryClient.invalidateQueries({ queryKey: ["me"] });
        navigate("/projects");
      } else {
        const res = await api.post<{ emailVerificationRequired?: boolean; email?: string }>(
          "/api/auth/register",
          { email, name, password }
        );
        if (res.emailVerificationRequired) {
          setPendingEmail(res.email ?? email);
        } else {
          await queryClient.invalidateQueries({ queryKey: ["me"] });
          navigate("/projects");
        }
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 403 && /verify your email/i.test(err.message)) {
        setPendingEmail(email);
      } else {
        setError(err instanceof Error ? err.message : "failed");
      }
    } finally {
      setBusy(false);
    }
  }

  async function resend() {
    await api.post("/api/auth/resend-verification", { email: pendingEmail });
    setResent(true);
  }

  const card = "w-full max-w-sm rounded-xl border border-gray-200 bg-white p-8 shadow-sm dark:border-gray-800 dark:bg-gray-900";
  const input =
    "w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-indigo-500 focus:outline-none dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100";

  if (pendingEmail) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className={card}>
          <h1 className="mb-2 text-center text-xl font-bold text-gray-900 dark:text-gray-100">Check your email</h1>
          <p className="mb-4 text-center text-sm text-gray-500 dark:text-gray-400">
            We sent a verification link to <span className="font-medium">{pendingEmail}</span>. Click it to activate
            your account, then sign in.
          </p>
          {resent ? (
            <p className="mb-3 text-center text-sm text-green-600 dark:text-green-400">Verification email resent.</p>
          ) : (
            <button
              onClick={resend}
              className="mb-3 w-full rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-200 dark:hover:bg-gray-800"
            >
              Resend verification email
            </button>
          )}
          <button
            onClick={() => {
              setPendingEmail("");
              setMode("login");
            }}
            className="w-full text-center text-sm text-indigo-600 hover:underline dark:text-indigo-400"
          >
            Back to sign in
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="relative flex h-full items-center justify-center">
      <ThemeToggle className="absolute right-4 top-4 text-lg" />
      <div className={card}>
        <h1 className="mb-1 text-center text-2xl font-bold text-gray-900 dark:text-gray-100">TypstPad</h1>
        <p className="mb-6 text-center text-sm text-gray-500 dark:text-gray-400">
          Collaborative Typst editing, self-hosted.
        </p>

        {verifyBanner === "success" && (
          <p className="mb-4 rounded-md bg-green-50 px-3 py-2 text-center text-sm text-green-700 dark:bg-green-950 dark:text-green-300">
            Email verified — you can sign in now.
          </p>
        )}
        {verifyBanner === "invalid" && (
          <p className="mb-4 rounded-md bg-amber-50 px-3 py-2 text-center text-sm text-amber-700 dark:bg-amber-950 dark:text-amber-300">
            That verification link is invalid or expired. Sign in to resend.
          </p>
        )}
        {params.get("reset") === "success" && (
          <p className="mb-4 rounded-md bg-green-50 px-3 py-2 text-center text-sm text-green-700 dark:bg-green-950 dark:text-green-300">
            Password updated — sign in with your new password.
          </p>
        )}

        {mode === "forgot" && forgotSent ? (
          <p className="rounded-md bg-green-50 px-3 py-2 text-center text-sm text-green-700 dark:bg-green-950 dark:text-green-300">
            If an account exists for {email}, a reset link is on its way.
          </p>
        ) : (
          <form onSubmit={submit} className="space-y-3">
            <input type="email" required placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} className={input} />
            {mode === "register" && (
              <input type="text" placeholder="Display name" value={name} onChange={(e) => setName(e.target.value)} className={input} />
            )}
            {mode !== "forgot" && (
              <input type="password" required placeholder="Password" value={password} onChange={(e) => setPassword(e.target.value)} className={input} />
            )}
            {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
            <button
              type="submit"
              disabled={busy}
              className="w-full rounded-md bg-indigo-600 px-3 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
            >
              {mode === "login" ? "Sign in" : mode === "register" ? "Create account" : "Send reset link"}
            </button>
          </form>
        )}

        {mode === "login" && (
          <button
            className="mt-2 block w-full text-center text-xs text-gray-500 hover:underline dark:text-gray-400"
            onClick={() => {
              setMode("forgot");
              setForgotSent(false);
              setError("");
            }}
          >
            Forgot password?
          </button>
        )}
        {mode === "forgot" && (
          <button
            className="mt-3 block w-full text-center text-sm text-indigo-600 hover:underline dark:text-indigo-400"
            onClick={() => {
              setMode("login");
              setForgotSent(false);
            }}
          >
            Back to sign in
          </button>
        )}

        {mode !== "forgot" && config.data?.oidcEnabled && (
          <a
            href="/api/auth/oidc/login"
            className="mt-3 block w-full rounded-md border border-gray-300 px-3 py-2 text-center text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-200 dark:hover:bg-gray-800"
          >
            Sign in with SSO
          </a>
        )}

        {mode === "register" && config.data?.signupAllowlist && (
          <p className="mt-3 text-center text-xs text-gray-400">
            Sign-ups limited to: {config.data.signupAllowlist}
          </p>
        )}

        {mode !== "forgot" && (
        <p className="mt-4 text-center text-sm text-gray-500 dark:text-gray-400">
          {mode === "login" ? (
            <>
              No account?{" "}
              <button className="text-indigo-600 hover:underline dark:text-indigo-400" onClick={() => setMode("register")}>
                Register
              </button>
            </>
          ) : (
            <>
              Have an account?{" "}
              <button className="text-indigo-600 hover:underline dark:text-indigo-400" onClick={() => setMode("login")}>
                Sign in
              </button>
            </>
          )}
        </p>
        )}
      </div>
    </div>
  );
}
