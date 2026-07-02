import { Navigate, Route, Routes, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api, type User } from "./api/client";
import LoginPage from "./pages/LoginPage";
import ProjectsPage from "./pages/ProjectsPage";
import EditorPage from "./pages/EditorPage";
import JoinPage from "./pages/JoinPage";
import SettingsPage from "./pages/SettingsPage";
import AdminPage from "./pages/AdminPage";
import TeamsPage from "./pages/TeamsPage";

export function useMe() {
  return useQuery<User>({
    queryKey: ["me"],
    queryFn: () => api.get<User>("/api/auth/me"),
    retry: false,
  });
}

function RequireAuth({ children }: { children: React.ReactNode }) {
  const me = useMe();
  if (me.isLoading) {
    return <div className="flex h-full items-center justify-center text-gray-400">Loading…</div>;
  }
  if (me.isError) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<Navigate to="/projects" replace />} />
      <Route
        path="/projects"
        element={
          <RequireAuth>
            <ProjectsPage />
          </RequireAuth>
        }
      />
      <Route
        path="/p/:projectId"
        element={
          <RequireAuth>
            <EditorPageWrapper />
          </RequireAuth>
        }
      />
      <Route
        path="/join/:token"
        element={
          <RequireAuth>
            <JoinPage />
          </RequireAuth>
        }
      />
      <Route
        path="/teams"
        element={
          <RequireAuth>
            <TeamsPage />
          </RequireAuth>
        }
      />
      <Route
        path="/settings"
        element={
          <RequireAuth>
            <SettingsPage />
          </RequireAuth>
        }
      />
      <Route
        path="/admin"
        element={
          <RequireAuth>
            <AdminPage />
          </RequireAuth>
        }
      />
    </Routes>
  );
}

function EditorPageWrapper() {
  const { projectId } = useParams();
  // Remount the editor completely when switching projects.
  return <EditorPage key={projectId} projectId={projectId!} />;
}
