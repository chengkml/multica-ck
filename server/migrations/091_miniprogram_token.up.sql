CREATE TABLE miniprogram_token (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    openid_hash TEXT NOT NULL,
    appid TEXT NOT NULL,
    unionid_hash TEXT,
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_miniprogram_token_openid_hash ON miniprogram_token(openid_hash);
CREATE INDEX idx_miniprogram_token_user ON miniprogram_token(user_id, revoked);
