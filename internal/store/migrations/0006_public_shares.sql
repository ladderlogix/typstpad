-- Public read-only share: an anonymous link that lets anyone view a project's
-- compiled PDF without an account. One link per project (toggle on/off). The
-- token is intentionally retrievable (it *is* the shareable URL), so it is
-- stored in plaintext rather than hashed like membership links.
CREATE TABLE public_shares (
  project_id uuid PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  token text UNIQUE NOT NULL,
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
