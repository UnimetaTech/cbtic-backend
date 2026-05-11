-- +goose Up

CREATE SEQUENCE IF NOT EXISTS news_items_public_id_seq;

ALTER TABLE news_items
  ADD COLUMN IF NOT EXISTS public_id BIGINT;

UPDATE news_items
SET public_id = nextval('news_items_public_id_seq')
WHERE public_id IS NULL;

SELECT setval(
  'news_items_public_id_seq',
  GREATEST(COALESCE(MAX(public_id), 0), 1),
  COALESCE(MAX(public_id), 0) > 0
)
FROM news_items;

ALTER TABLE news_items
  ALTER COLUMN public_id SET DEFAULT nextval('news_items_public_id_seq'),
  ALTER COLUMN public_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS news_items_public_id_unique
ON news_items (public_id);

-- +goose Down

DROP INDEX IF EXISTS news_items_public_id_unique;

ALTER TABLE news_items
  DROP COLUMN IF EXISTS public_id;

DROP SEQUENCE IF EXISTS news_items_public_id_seq;
