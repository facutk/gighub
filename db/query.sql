-- name: GetMessage :one
SELECT message FROM guestbook WHERE id = 1 LIMIT 1;

-- name: UpsertMessage :exec
INSERT INTO guestbook (id, message) 
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET message = excluded.message;