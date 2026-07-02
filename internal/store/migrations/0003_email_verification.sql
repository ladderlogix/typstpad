-- Email verification for local accounts. Existing users are grandfathered
-- as verified so enabling SMTP doesn't lock anyone out.
ALTER TABLE users ADD COLUMN email_verified boolean NOT NULL DEFAULT false;
UPDATE users SET email_verified = true;

CREATE TABLE email_verifications (
  token_hash bytea PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_verifications_user ON email_verifications(user_id);
