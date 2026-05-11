-- +goose Up

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE news_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source TEXT NOT NULL,
  source_media_id TEXT NOT NULL,
  reel_url TEXT NOT NULL,
  thumbnail_url TEXT,
  caption TEXT,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  body TEXT,
  category TEXT NOT NULL,
  importance TEXT NOT NULL,
  confidence NUMERIC(4, 3),
  status TEXT NOT NULL,
  published_at TIMESTAMPTZ,
  scraped_at TIMESTAMPTZ NOT NULL,
  classified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX news_items_source_media_unique
ON news_items (source, source_media_id);

CREATE UNIQUE INDEX news_items_reel_url_unique
ON news_items (reel_url);

CREATE INDEX news_items_status_published_idx
ON news_items (status, published_at DESC);

CREATE INDEX news_items_category_idx
ON news_items (category);

CREATE TABLE job_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  job_name TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ,
  processed_count INTEGER DEFAULT 0,
  error_message TEXT
);

-- +goose Down

DROP TABLE IF EXISTS job_runs;
DROP TABLE IF EXISTS news_items;