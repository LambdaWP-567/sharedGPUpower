-- +migrate Up

CREATE TABLE IF NOT EXISTS users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    zitadel_id    TEXT        NOT NULL UNIQUE,
    email         TEXT        NOT NULL UNIQUE,
    display_name  TEXT,
    avatar_url    TEXT,
    role          TEXT        NOT NULL DEFAULT 'user',
    status        TEXT        NOT NULL DEFAULT 'pending',
    api_key       TEXT        NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS user_id         UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'pending';

ALTER TABLE token_ledger
    ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id);

ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id);

CREATE INDEX IF NOT EXISTS users_api_key_idx   ON users(api_key);
CREATE INDEX IF NOT EXISTS agents_user_id_idx  ON agents(user_id);
CREATE INDEX IF NOT EXISTS agents_approval_idx ON agents(approval_status);

-- +migrate Down
ALTER TABLE jobs         DROP COLUMN IF EXISTS user_id;
ALTER TABLE token_ledger DROP COLUMN IF EXISTS user_id;
ALTER TABLE agents       DROP COLUMN IF EXISTS approval_status;
ALTER TABLE agents       DROP COLUMN IF EXISTS user_id;
DROP TABLE IF EXISTS users;
