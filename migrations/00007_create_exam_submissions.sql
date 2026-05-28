-- +goose Up

CREATE TABLE exam_submissions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  exam_id TEXT NOT NULL,
  email TEXT NOT NULL,
  score NUMERIC(6, 2) NOT NULL,
  max_score NUMERIC(6, 2) NOT NULL,
  status TEXT NOT NULL,
  answers JSONB NOT NULL DEFAULT '{}'::jsonb,
  breakdown JSONB NOT NULL DEFAULT '{}'::jsonb,
  client_ip TEXT,
  user_agent TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Un solo intento por estudiante y examen (case-insensitive sobre el correo).
CREATE UNIQUE INDEX exam_submissions_exam_email_unique
ON exam_submissions (exam_id, lower(email));

CREATE INDEX exam_submissions_created_at_idx
ON exam_submissions (created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS exam_submissions;
