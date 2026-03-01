-- name: GetMessage :one
SELECT message FROM guestbook WHERE id = 1 LIMIT 1;

-- name: UpsertMessage :exec
INSERT INTO guestbook (id, message) 
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET message = excluded.message;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, verification_token)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ? LIMIT 1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expiry)
VALUES (?, ?, ?);

-- name: GetSession :one
SELECT sessions.*, users.email FROM sessions
JOIN users ON sessions.user_id = users.id
WHERE token_hash = ? AND expiry > CURRENT_TIMESTAMP LIMIT 1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: VerifyUser :one
UPDATE users 
SET verified_at = CURRENT_TIMESTAMP, verification_token = NULL
WHERE verification_token = ? AND verified_at IS NULL
RETURNING id;