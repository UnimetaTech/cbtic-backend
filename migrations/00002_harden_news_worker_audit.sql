-- +goose Up

ALTER TABLE job_runs
  ADD COLUMN IF NOT EXISTS extracted_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS new_items_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS duplicates_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS classified_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS inserted_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS skipped_old_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS skipped_not_news_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS failed_items_count INTEGER DEFAULT 0;

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'news_items_status_check'
  ) THEN
    ALTER TABLE news_items
      ADD CONSTRAINT news_items_status_check
      CHECK (status IN ('published', 'draft', 'hidden', 'needs_review'));
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'news_items_importance_check'
  ) THEN
    ALTER TABLE news_items
      ADD CONSTRAINT news_items_importance_check
      CHECK (importance IN ('alta', 'media', 'baja'));
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'news_items_category_check'
  ) THEN
    ALTER TABLE news_items
      ADD CONSTRAINT news_items_category_check
      CHECK (category IN (
        'Académico',
        'Eventos',
        'Investigación',
        'Deportes',
        'Tecnología',
        'Cultura',
        'Bienestar',
        'Institucional',
        'Otro'
      ));
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'job_runs_status_check'
  ) THEN
    ALTER TABLE job_runs
      ADD CONSTRAINT job_runs_status_check
      CHECK (status IN ('running', 'succeeded', 'failed'));
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down

ALTER TABLE job_runs
  DROP CONSTRAINT IF EXISTS job_runs_status_check;

ALTER TABLE news_items
  DROP CONSTRAINT IF EXISTS news_items_category_check,
  DROP CONSTRAINT IF EXISTS news_items_importance_check,
  DROP CONSTRAINT IF EXISTS news_items_status_check;

ALTER TABLE job_runs
  DROP COLUMN IF EXISTS failed_items_count,
  DROP COLUMN IF EXISTS skipped_not_news_count,
  DROP COLUMN IF EXISTS skipped_old_count,
  DROP COLUMN IF EXISTS inserted_count,
  DROP COLUMN IF EXISTS classified_count,
  DROP COLUMN IF EXISTS duplicates_count,
  DROP COLUMN IF EXISTS new_items_count,
  DROP COLUMN IF EXISTS extracted_count;