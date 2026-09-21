-- +goose Up
CREATE TABLE IF NOT EXISTS confessions (
    id          BIGSERIAL PRIMARY KEY,
    channel_id  TEXT NOT NULL,
    message_ts  TEXT,
    text        TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    secret_hash TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_confessions_secret_hash ON confessions (secret_hash);
CREATE INDEX IF NOT EXISTS idx_confessions_user_id ON confessions (user_id);
CREATE INDEX IF NOT EXISTS idx_confessions_status ON confessions (status);

-- +goose Down
DROP TABLE IF EXISTS confessions;