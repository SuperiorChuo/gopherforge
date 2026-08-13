-- +goose Up

-- Public avatar URLs carry a random capability token. This is intentional:
-- browser img tags cannot attach Authorization headers, while generic signed
-- /uploads URLs expire and therefore cannot be stored as a profile field.
ALTER TABLE public.files
  ADD COLUMN IF NOT EXISTS purpose varchar(32) NOT NULL DEFAULT 'general',
  ADD COLUMN IF NOT EXISTS public_token varchar(64);

CREATE UNIQUE INDEX IF NOT EXISTS ux_files_public_token
  ON public.files (public_token)
  WHERE public_token IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_files_tenant_user_purpose_created
  ON public.files (tenant_id, user_id, purpose, created_at DESC);

-- +goose Down

DROP INDEX IF EXISTS public.idx_files_tenant_user_purpose_created;
DROP INDEX IF EXISTS public.ux_files_public_token;
ALTER TABLE public.files
  DROP COLUMN IF EXISTS public_token,
  DROP COLUMN IF EXISTS purpose;
