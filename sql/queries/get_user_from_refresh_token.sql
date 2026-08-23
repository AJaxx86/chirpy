-- name: GetUserFromRefreshToken :one
SELECT
	users.id AS user_id,
	users.email,
	refresh_tokens.token,
	refresh_tokens.expires_at,
	refresh_tokens.revoked_at
FROM refresh_tokens
JOIN users ON refresh_tokens.user_id = users.id
WHERE refresh_tokens.token = $1;
