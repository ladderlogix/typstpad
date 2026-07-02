export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: {} };
  if (body !== undefined) {
    if (body instanceof FormData) {
      init.body = body;
    } else {
      init.headers = { "Content-Type": "application/json" };
      init.body = JSON.stringify(body);
    }
  }
  const resp = await fetch(path, init);
  const ct = resp.headers.get("Content-Type") ?? "";
  if (!resp.ok) {
    let msg = resp.statusText;
    if (ct.includes("json")) {
      const data = await resp.json().catch(() => null);
      if (data?.error) msg = data.error;
    }
    throw new ApiError(resp.status, msg);
  }
  if (ct.includes("json")) {
    return resp.json();
  }
  return (await resp.text()) as unknown as T;
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  put: <T>(path: string, body?: unknown) => request<T>("PUT", path, body),
  patch: <T>(path: string, body?: unknown) => request<T>("PATCH", path, body),
  del: <T>(path: string) => request<T>("DELETE", path),
};

// ---- types mirrored from the Go API ----

export interface User {
  id: string;
  email: string;
  name: string;
  isAdmin: boolean;
  color: string;
  emailVerified?: boolean;
  hasPassword?: boolean;
}

export interface Project {
  id: string;
  name: string;
  description: string;
  ownerId: string;
  mainPath: string;
  isTemplate: boolean;
  templateMeta?: { description?: string; category?: string };
  createdAt: string;
  updatedAt: string;
  role?: "owner" | "editor" | "suggester" | "viewer";
  favorite?: boolean;
  deletedAt?: string;
}

export interface Collection {
  id: string;
  ownerId: string;
  name: string;
  createdAt: string;
  count: number;
  teamId?: string;
  teamName?: string;
  canManage?: boolean;
}

export interface FileEntry {
  id: string;
  projectId: string;
  path: string;
  kind: "text" | "asset";
  mime: string;
  size: number;
}

export interface Snapshot {
  id: string;
  projectId: string;
  kind: "auto" | "named" | "pre_restore";
  name?: string;
  createdAt: string;
}

export interface Suggestion {
  id: string;
  projectId: string;
  fileId: string;
  authorId: string;
  authorName: string;
  authorColor: string;
  type: "insert" | "delete" | "replace";
  anchorStart: string;
  anchorEnd?: string;
  insertedText?: string;
  deletedPreview?: string;
  status: "open" | "accepted" | "rejected";
  createdAt: string;
}

export interface Comment {
  id: string;
  projectId: string;
  fileId?: string;
  authorId: string;
  authorName: string;
  authorColor: string;
  parentId?: string;
  body: string;
  anchorStart?: string;
  anchorEnd?: string;
  status: "open" | "resolved";
  createdAt: string;
}

export interface Member {
  userId: string;
  email: string;
  name: string;
  color: string;
  role: string;
}

export interface Team {
  id: string;
  name: string;
  createdBy: string;
  createdAt: string;
  role?: "admin" | "member";
  memberCount?: number;
}

export interface ProjectTeam {
  teamId: string;
  teamName: string;
  role: string;
}

export interface ShareLink {
  id: string;
  projectId: string;
  role: string;
  createdAt: string;
  expiresAt?: string;
  token?: string;
}

export interface AuditEntry {
  id: string;
  actorEmail: string;
  action: string;
  target: string;
  detail: string;
  createdAt: string;
}

export interface Notification {
  id: string;
  type: "comment" | "mention" | "share";
  actorName?: string;
  projectId?: string;
  projectName?: string;
  summary: string;
  readAt?: string;
  createdAt: string;
}

export interface PublicShare {
  enabled: boolean;
  token?: string;
  url?: string;
}

export interface APIToken {
  id: string;
  name: string;
  scopes: string[];
  createdAt: string;
  lastUsedAt?: string;
  expiresAt?: string;
  token?: string;
}

export interface Diagnostic {
  severity: "error" | "warning";
  file: string;
  line: number;
  col: number;
  message: string;
}

export interface AuthConfig {
  oidcEnabled: boolean;
  allowRegistration: boolean;
  emailVerification: boolean;
  signupAllowlist: string;
}

export interface AdminSettings {
  allowRegistration: boolean;
  signupAllowlist: string;
  requireEmailVerification: boolean;
  smtpHost: string;
  smtpPort: number;
  smtpUsername: string;
  smtpPasswordSet: boolean;
  smtpFrom: string;
  smtpFromName: string;
  oidcIssuer: string;
  oidcClientId: string;
  oidcClientSecretSet: boolean;
  oidcScopes: string;
  emailVerificationActive: boolean;
  oidcActive: boolean;
}

export function roleAtLeast(role: string | undefined, min: "viewer" | "suggester" | "editor" | "owner"): boolean {
  const rank: Record<string, number> = { viewer: 1, suggester: 2, editor: 3, owner: 4 };
  return (rank[role ?? ""] ?? 0) >= rank[min];
}
