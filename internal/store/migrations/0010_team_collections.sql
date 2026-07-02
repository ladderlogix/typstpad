-- Collections can belong to a team (shared with all its members) instead of a
-- single user. team_id NULL = personal collection (unchanged behaviour).
ALTER TABLE collections ADD COLUMN team_id uuid REFERENCES teams(id) ON DELETE CASCADE;
CREATE INDEX collections_team ON collections(team_id);
