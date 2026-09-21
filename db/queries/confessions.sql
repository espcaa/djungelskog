-- name: CreateConfession :one
INSERT INTO confessions (text, reply_key, post_channel)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetConfessionByID :one
SELECT * FROM confessions
WHERE id = $1;

-- name: GetConfessionByReplyKey :one
SELECT * FROM confessions
WHERE reply_key = $1;

-- name: SetConfessionReviewTs :one
UPDATE confessions
SET review_ts = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: AcceptConfession :one
UPDATE confessions
SET status = 'accepted', post_ts = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteConfession :exec
DELETE FROM confessions
WHERE id = $1;