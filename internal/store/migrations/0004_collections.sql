-- Personal collections: a per-user organization layer over projects (like
-- folders/labels). A project can live in multiple collections; collections are
-- private to their owner and don't affect sharing.
CREATE TABLE collections (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX collections_owner ON collections(owner_id);

CREATE TABLE project_collections (
  collection_id uuid NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  added_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (collection_id, project_id)
);
CREATE INDEX project_collections_project ON project_collections(project_id);
