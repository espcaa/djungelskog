-- +goose Up
ALTER TABLE confessions ADD COLUMN post_thread_ts TEXT;

-- +goose Down
ALTER TABLE confessions DROP COLUMN post_thread_ts;
