-- +goose Up

ALTER TABLE job_runs
  ADD COLUMN IF NOT EXISTS skipped_old_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS skipped_not_news_count INTEGER DEFAULT 0;

-- +goose Down

ALTER TABLE job_runs
  DROP COLUMN IF EXISTS skipped_not_news_count,
  DROP COLUMN IF EXISTS skipped_old_count;
