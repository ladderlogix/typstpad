import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { api, type Project } from "../api/client";

export default function JoinPage() {
  const { token } = useParams();
  const navigate = useNavigate();
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .post<Project>(`/api/links/${token}/join`)
      .then((p) => navigate(`/p/${p.id}`, { replace: true }))
      .catch((err) => setError(err.message));
  }, [token]);

  return (
    <div className="flex h-full items-center justify-center text-gray-500">
      {error ? `Could not join: ${error}` : "Joining project…"}
    </div>
  );
}
