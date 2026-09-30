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
SET status = 'accepted', post_ts = $2, post_thread_ts = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteConfession :exec
DELETE FROM confessions
WHERE id = $1;

-- name: RejectConfession :one
UPDATE confessions
SET status = 'rejected', updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UndoConfession :one
UPDATE confessions
SET status = 'pending', post_ts = NULL, post_thread_ts = NULL, updated_at = now()
WHERE id = $1
RETURNING *;
