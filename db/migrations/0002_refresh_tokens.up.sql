-- 0002_refresh_tokens.up.sql
BEGIN;

CREATE TABLE refresh_tokens (
    id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash             TEXT NOT NULL UNIQUE,
    user_id                BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at             TIMESTAMPTZ NOT NULL,
    revoked_at             TIMESTAMPTZ,
    replaced_by_token_hash TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT refresh_tokens_hash_not_blank CHECK (btrim(token_hash) <> '')
);

-- فهرس على user_id للاستعلام "أبطِل كل جلسات المستخدم"
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);

-- ملاحظة: لا نضيف refresh_tokens_hash_idx — UNIQUE constraint يوفر فهرسًا.

COMMIT;
