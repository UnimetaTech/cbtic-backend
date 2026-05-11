-- +goose Up

UPDATE news_items
SET
  status = 'published',
  published_at = COALESCE(published_at, now()),
  updated_at = now()
WHERE source = 'instagram'
AND status = 'needs_review';

-- +goose Down

-- Intentionally irreversible: once an automatically discovered news item is
-- published, rolling it back to needs_review could hide content unexpectedly.
