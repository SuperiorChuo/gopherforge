-- +goose Up
-- Delete older duplicate rows before adding the user/provider unique key.
DELETE FROM oauth_bindings ob
USING oauth_bindings newer
WHERE newer.user_id = ob.user_id
  AND newer.provider = ob.provider
  AND newer.id > ob.id;

CREATE UNIQUE INDEX IF NOT EXISTS uk_oauth_bindings_user_provider ON oauth_bindings (user_id, provider);

-- +goose Down
DROP INDEX IF EXISTS uk_oauth_bindings_user_provider;
