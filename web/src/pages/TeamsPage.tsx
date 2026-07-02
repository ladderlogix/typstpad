import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Member, type Team } from "../api/client";
import { useMe } from "../App";

export default function TeamsPage() {
  const [newName, setNewName] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const teams = useQuery<Team[]>({
    queryKey: ["teams"],
    queryFn: () => api.get<Team[]>("/api/teams"),
  });

  const create = useMutation({
    mutationFn: (name: string) => api.post<Team>("/api/teams", { name }),
    onSuccess: (t) => {
      setNewName("");
      queryClient.invalidateQueries({ queryKey: ["teams"] });
      setSelected(t.id);
    },
    onError: (e: Error) => alert(e.message),
  });

  return (
    <div className="min-h-full bg-gray-50">
      <header className="border-b border-gray-200 bg-white px-6 py-4">
        <div className="mx-auto flex max-w-4xl items-center gap-4">
          <Link to="/projects" className="text-sm text-gray-500 hover:text-gray-900">
            ← Projects
          </Link>
          <h1 className="text-lg font-semibold text-gray-900">Teams</h1>
        </div>
      </header>

      <main className="mx-auto grid max-w-4xl grid-cols-[16rem_1fr] gap-6 px-6 py-8">
        <div>
          <div className="mb-3 flex gap-2">
            <input
              placeholder="New team name"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && newName && create.mutate(newName)}
              className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm focus:border-indigo-500 focus:outline-none"
            />
            <button
              onClick={() => newName && create.mutate(newName)}
              className="shrink-0 rounded-md bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-indigo-700"
            >
              +
            </button>
          </div>
          <ul className="divide-y divide-gray-100 rounded-md border border-gray-200 bg-white">
            {teams.data?.map((t) => (
              <li key={t.id}>
                <button
                  onClick={() => setSelected(t.id)}
                  className={`w-full px-3 py-2 text-left text-sm ${
                    selected === t.id ? "bg-indigo-50 text-indigo-700" : "text-gray-700 hover:bg-gray-50"
                  }`}
                >
                  <span className="block font-medium">{t.name}</span>
                  <span className="text-xs text-gray-400">
                    {t.memberCount} member{t.memberCount === 1 ? "" : "s"} · {t.role}
                  </span>
                </button>
              </li>
            ))}
            {teams.data?.length === 0 && (
              <li className="px-3 py-6 text-center text-xs text-gray-400">
                No teams yet. Create one to share projects with a group at once.
              </li>
            )}
          </ul>
        </div>

        <div>
          {selected ? (
            <TeamDetail teamId={selected} onDeleted={() => setSelected(null)} />
          ) : (
            <p className="pt-10 text-center text-sm text-gray-400">
              Select a team, or create one. Share a project with a team from its Share dialog and
              every member gets access automatically.
            </p>
          )}
        </div>
      </main>
    </div>
  );
}

function TeamDetail({ teamId, onDeleted }: { teamId: string; onDeleted: () => void }) {
  const me = useMe();
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [error, setError] = useState("");

  const team = useQuery<Team>({ queryKey: ["team", teamId], queryFn: () => api.get(`/api/teams/${teamId}`) });
  const members = useQuery<Member[]>({
    queryKey: ["teamMembers", teamId],
    queryFn: () => api.get(`/api/teams/${teamId}/members`),
  });
  const isAdmin = team.data?.role === "admin";
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["teamMembers", teamId] });
    queryClient.invalidateQueries({ queryKey: ["teams"] });
    queryClient.invalidateQueries({ queryKey: ["team", teamId] });
  };

  const add = useMutation({
    mutationFn: () => api.post(`/api/teams/${teamId}/members`, { email, role }),
    onSuccess: () => {
      setEmail("");
      setError("");
      invalidate();
    },
    onError: (e: Error) => setError(e.message),
  });
  const changeRole = useMutation({
    mutationFn: (v: { userId: string; role: string }) => api.patch(`/api/teams/${teamId}/members/${v.userId}`, { role: v.role }),
    onSuccess: invalidate,
    onError: (e: Error) => alert(e.message),
  });
  const removeMember = useMutation({
    mutationFn: (userId: string) => api.del(`/api/teams/${teamId}/members/${userId}`),
    onSuccess: invalidate,
    onError: (e: Error) => alert(e.message),
  });
  const rename = useMutation({
    mutationFn: (name: string) => api.patch(`/api/teams/${teamId}`, { name }),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: () => api.del(`/api/teams/${teamId}`),
    onSuccess: () => {
      invalidate();
      onDeleted();
    },
    onError: (e: Error) => alert(e.message),
  });

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-5">
      <div className="mb-4 flex items-center justify-between">
        <h2
          className="text-base font-semibold text-gray-900"
          onDoubleClick={() => {
            if (!isAdmin) return;
            const name = prompt("Team name:", team.data?.name);
            if (name) rename.mutate(name);
          }}
        >
          {team.data?.name ?? "…"}
        </h2>
        {isAdmin && (
          <button
            className="text-xs text-red-500 hover:underline"
            onClick={() => {
              if (confirm(`Delete team "${team.data?.name}"? Projects shared with it lose that access.`))
                remove.mutate();
            }}
          >
            Delete team
          </button>
        )}
      </div>

      {isAdmin && (
        <div className="mb-3 flex gap-2">
          <input
            placeholder="user@example.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="flex-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm focus:border-indigo-500 focus:outline-none"
          />
          <select
            value={role}
            onChange={(e) => setRole(e.target.value)}
            className="rounded-md border border-gray-300 px-2 py-1.5 text-sm"
          >
            <option value="member">Member</option>
            <option value="admin">Admin</option>
          </select>
          <button
            onClick={() => add.mutate()}
            className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-700"
          >
            Add
          </button>
        </div>
      )}
      {error && <p className="mb-2 text-xs text-red-600">{error}</p>}

      <ul className="divide-y divide-gray-100 rounded-md border border-gray-200">
        {members.data?.map((m) => (
          <li key={m.userId} className="flex items-center justify-between px-3 py-2 text-sm">
            <div className="flex items-center gap-2">
              <span
                className="flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold text-white"
                style={{ backgroundColor: m.color }}
              >
                {m.name[0]?.toUpperCase()}
              </span>
              <span className="text-gray-900">{m.name}</span>
              <span className="text-xs text-gray-400">{m.email}</span>
            </div>
            {isAdmin ? (
              <div className="flex items-center gap-2">
                <select
                  value={m.role}
                  onChange={(e) => changeRole.mutate({ userId: m.userId, role: e.target.value })}
                  className="rounded-md border border-gray-300 px-2 py-1 text-xs"
                >
                  <option value="member">Member</option>
                  <option value="admin">Admin</option>
                </select>
                <button className="text-xs text-red-500 hover:underline" onClick={() => removeMember.mutate(m.userId)}>
                  Remove
                </button>
              </div>
            ) : (
              <span className="text-xs text-gray-500">
                {m.role}
                {m.userId === me.data?.id && (
                  <button
                    className="ml-2 text-red-500 hover:underline"
                    onClick={() => removeMember.mutate(m.userId)}
                  >
                    Leave
                  </button>
                )}
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
