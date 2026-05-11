-- +goose Up

UPDATE news_items
SET
  status = 'needs_review',
  published_at = NULL,
  updated_at = now()
WHERE source = 'instagram'
AND (
  source_media_id ~ '^[a-z]{2}_[A-Z]{2}$'
  OR reel_url ~ '/reel/[a-z]{2}_[A-Z]{2}/?$'
  OR category = 'Otro'
  OR title = 'Nueva publicación institucional'
  OR summary = 'Contenido publicado en la cuenta institucional de Instagram.'
  OR LOWER(title) ~ '^[0-9,.]+\s+likes?,\s+[0-9,.]+\s+comments?'
  OR LOWER(summary) ~ '^[0-9,.]+\s+likes?,\s+[0-9,.]+\s+comments?'
);

-- +goose Down

-- Intentionally irreversible: these rows are false positives or non-editorial
-- fallbacks and should not be republished automatically.
