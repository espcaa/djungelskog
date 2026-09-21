-- +goose Up
ALTER TABLE confessions RENAME COLUMN channel_id TO post_channel;

ALTER TABLE confessions DROP COLUMN user_id;
ALTER TABLE confessions DROP COLUMN secret_hash;
ALTER TABLE confessions DROP COLUMN message_ts;

ALTER TABLE confessions ADD COLUMN reply_key TEXT;
ALTER TABLE confessions ADD COLUMN review_ts TEXT;
ALTER TABLE confessions ADD COLUMN post_ts TEXT;

UPDATE confessions SET reply_key = '' WHERE reply_key IS NULL;
ALTER TABLE confessions ALTER COLUMN reply_key SET NOT NULL;

DROP INDEX IF EXISTS idx_confessions_secret_hash;
DROP INDEX IF EXISTS idx_confessions_user_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_confessions_reply_key ON confessions (reply_key);

-- +goose Down
ALTER TABLE confessions ADD COLUMN channel_id TEXT NOT NULL DEFAULT '';
ALTER TABLE confessions ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE confessions ADD COLUMN secret_hash TEXT NOT NULL DEFAULT '';

ALTER TABLE confessions DROP COLUMN reply_key;
ALTER TABLE confessions DROP COLUMN review_ts;
ALTER TABLE confessions DROP COLUMN post_ts;

DROP INDEX IF EXISTS idx_confessions_reply_key;
CREATE INDEX IF NOT EXISTS idx_confessions_secret_hash ON confessions (secret_hash);
CREATE INDEX IF NOT EXISTS idx_confessions_user_id ON confessions (user_id);
