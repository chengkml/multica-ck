-- name: CreateMiniprogramToken :one
INSERT INTO miniprogram_token (user_id, openid_hash, appid, unionid_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetMiniprogramTokenByOpenidHash :one
SELECT * FROM miniprogram_token
WHERE openid_hash = $1
  AND revoked = FALSE
  AND (expires_at IS NULL OR expires_at > now());

-- name: ListMiniprogramTokensByUser :many
SELECT * FROM miniprogram_token
WHERE user_id = $1
  AND revoked = FALSE
ORDER BY created_at DESC;

-- name: RevokeMiniprogramToken :one
UPDATE miniprogram_token
SET revoked = TRUE
WHERE id = $1 AND user_id = $2
RETURNING openid_hash;

-- name: UpdateMiniprogramTokenLastUsed :exec
UPDATE miniprogram_token
SET last_used_at = now()
WHERE id = $1;

-- name: DeleteExpiredMiniprogramTokens :exec
DELETE FROM miniprogram_token
WHERE expires_at <= now();
