-- Per-file permission: a locked file can only be edited by the project owner;
-- everyone else (including editors) gets read-only access to that file.
ALTER TABLE files ADD COLUMN locked boolean NOT NULL DEFAULT false;
