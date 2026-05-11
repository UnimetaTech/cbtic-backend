package news

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context, params ListParams) ([]NewsItem, int, error) {
	if params.Limit <= 0 || params.Limit > 50 {
		params.Limit = 12
	}

	if params.Offset < 0 {
		params.Offset = 0
	}

	if params.FeaturedLimit <= 0 {
		params.FeaturedLimit = 3
	}

	where := []string{"status = 'published'"}
	args := []any{params.FeaturedLimit}
	argIndex := 2

	where = append(where, "classified_at IS NOT NULL")
	where = append(where, "COALESCE(confidence, 0) > 0")
	where = append(where, "category <> 'Otro'")
	where = append(where, "title <> 'Nueva publicación institucional'")
	where = append(where, "summary <> 'Contenido publicado en la cuenta institucional de Instagram.'")
	where = append(where, "reel_url !~ '/reel/[a-z]{2}_[A-Z]{2}/?$'")
	where = append(where, "NOT (LOWER(title) ~ '^[0-9,.]+\\s+likes?,\\s+[0-9,.]+\\s+comments?')")
	where = append(where, "NOT (LOWER(summary) ~ '^[0-9,.]+\\s+likes?,\\s+[0-9,.]+\\s+comments?')")

	if params.Category != "" {
		where = append(where, fmt.Sprintf("category = $%d", argIndex))
		args = append(args, params.Category)
		argIndex++
	}

	filters := []string{"TRUE"}
	if params.Importance != "" {
		filters = append(filters, fmt.Sprintf("public_importance = $%d", argIndex))
		args = append(args, params.Importance)
		argIndex++
	}

	query := fmt.Sprintf(`
		WITH ranked_news AS (
			SELECT
				public_id,
				source,
				title,
				summary,
				COALESCE(body, '') AS body,
				COALESCE(thumbnail_url, '') AS thumbnail_url,
				reel_url,
				category,
				CASE
					WHEN ROW_NUMBER() OVER (ORDER BY COALESCE(published_at, created_at) DESC) <= $1 THEN 'alta'
					WHEN importance = 'alta' THEN 'media'
					ELSE importance
				END AS public_importance,
				COALESCE(published_at, created_at) AS published_at
			FROM news_items
			WHERE %s
		)
		SELECT
			public_id,
			title,
			summary,
			body,
			thumbnail_url,
			thumbnail_url,
			published_at,
			category,
			public_importance,
			source,
			reel_url,
			COUNT(*) OVER() AS total
		FROM ranked_news
		WHERE %s
		ORDER BY published_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(where, " AND "), strings.Join(filters, " AND "), argIndex, argIndex+1)

	args = append(args, params.Limit, params.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []NewsItem{}
	total := 0

	for rows.Next() {
		var item NewsItem

		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.Summary,
			&item.Body,
			&item.ImageURL,
			&item.ThumbnailURL,
			&item.PublishedAt,
			&item.Category,
			&item.Importance,
			&item.Source,
			&item.ReelURL,
			&total,
		); err != nil {
			return nil, 0, err
		}

		items = append(items, item)
	}

	return items, total, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, id string, featuredLimit int) (*NewsItem, error) {
	if featuredLimit <= 0 {
		featuredLimit = 3
	}

	query := `
		WITH ranked_news AS (
			SELECT
				public_id,
				source,
				title,
				summary,
				COALESCE(body, '') AS body,
				COALESCE(thumbnail_url, '') AS thumbnail_url,
				reel_url,
				category,
				CASE
					WHEN ROW_NUMBER() OVER (ORDER BY COALESCE(published_at, created_at) DESC) <= $1 THEN 'alta'
					WHEN importance = 'alta' THEN 'media'
					ELSE importance
				END AS public_importance,
				COALESCE(published_at, created_at) AS published_at
			FROM news_items
			WHERE status = 'published'
			AND classified_at IS NOT NULL
			AND COALESCE(confidence, 0) > 0
			AND category <> 'Otro'
			AND title <> 'Nueva publicación institucional'
			AND summary <> 'Contenido publicado en la cuenta institucional de Instagram.'
			AND reel_url !~ '/reel/[a-z]{2}_[A-Z]{2}/?$'
			AND NOT (LOWER(title) ~ '^[0-9,.]+\s+likes?,\s+[0-9,.]+\s+comments?')
			AND NOT (LOWER(summary) ~ '^[0-9,.]+\s+likes?,\s+[0-9,.]+\s+comments?')
		)
		SELECT
			public_id,
			title,
			summary,
			body,
			thumbnail_url,
			thumbnail_url,
			published_at,
			category,
			public_importance,
			source,
			reel_url
		FROM ranked_news
		WHERE public_id::text = $2
		LIMIT 1
	`

	var item NewsItem

	err := r.db.QueryRow(ctx, query, featuredLimit, id).Scan(
		&item.ID,
		&item.Title,
		&item.Summary,
		&item.Body,
		&item.ImageURL,
		&item.ThumbnailURL,
		&item.PublishedAt,
		&item.Category,
		&item.Importance,
		&item.Source,
		&item.ReelURL,
	)

	if err != nil {
		return nil, err
	}

	return &item, nil
}

func (r *Repository) ExistsBySourceMediaID(ctx context.Context, source string, sourceMediaID string) (bool, error) {
	var exists bool

	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM news_items
			WHERE source = $1
			AND source_media_id = $2
		)
	`, source, sourceMediaID).Scan(&exists)

	return exists, err
}

func (r *Repository) PublishNeedsReviewBySourceMediaID(ctx context.Context, source string, sourceMediaID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE news_items
		SET
			status = 'published',
			published_at = COALESCE(published_at, now()),
			updated_at = now()
		WHERE source = $1
		AND source_media_id = $2
		AND status = 'needs_review'
	`, source, sourceMediaID)

	return err
}

func (r *Repository) ListRecentTitles(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	rows, err := r.db.Query(ctx, `
		SELECT title
		FROM news_items
		WHERE COALESCE(title, '') <> ''
		ORDER BY COALESCE(published_at, created_at) DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	titles := []string{}
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, err
		}

		titles = append(titles, title)
	}

	return titles, rows.Err()
}

func (r *Repository) ListInstagramItemsForAIReprocessing(ctx context.Context, limit int) ([]AIReprocessCandidate, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id::text,
			source_media_id,
			reel_url,
			COALESCE(thumbnail_url, ''),
			COALESCE(caption, ''),
			COALESCE(published_at, created_at)
		FROM news_items
		WHERE source = 'instagram'
		AND COALESCE(caption, '') <> ''
		ORDER BY COALESCE(published_at, created_at) DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []AIReprocessCandidate{}
	for rows.Next() {
		var item AIReprocessCandidate
		var publishedAt time.Time

		if err := rows.Scan(
			&item.ID,
			&item.SourceMediaID,
			&item.ReelURL,
			&item.ThumbnailURL,
			&item.Caption,
			&publishedAt,
		); err != nil {
			return nil, err
		}

		item.PublishedAt = &publishedAt
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *Repository) ListInstagramTitleRepairCandidates(ctx context.Context, limit int) ([]TitleRepairCandidate, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id::text,
			COALESCE(title, ''),
			COALESCE(summary, ''),
			COALESCE(body, ''),
			COALESCE(caption, '')
		FROM news_items
		WHERE source = 'instagram'
		AND status = 'published'
		AND COALESCE(caption, '') <> ''
		AND COALESCE(title, '') <> ''
		ORDER BY COALESCE(published_at, created_at) DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TitleRepairCandidate{}
	for rows.Next() {
		var item TitleRepairCandidate
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.Summary,
			&item.Body,
			&item.Caption,
		); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *Repository) UpdateTitle(ctx context.Context, id string, title string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE news_items
		SET
			title = $2,
			updated_at = now()
		WHERE id = $1
	`, id, title)

	return err
}

func (r *Repository) UpdateFromAIClassification(ctx context.Context, id string, classification NewsClassification) error {
	status := NewsStatusNeedsReview
	if classification.IsNews {
		status = NewsStatusPublished
	}

	_, err := r.db.Exec(ctx, `
		UPDATE news_items
		SET
			title = $2,
			summary = $3,
			body = $4,
			category = $5,
			importance = $6,
			confidence = $7,
			status = $8,
			published_at = CASE
				WHEN $8 = 'published' THEN COALESCE(published_at, now())
				ELSE NULL
			END,
			classified_at = now(),
			updated_at = now()
		WHERE id = $1
	`, id,
		classification.Title,
		classification.Summary,
		classification.Body,
		classification.Category,
		classification.Importance,
		classification.Confidence,
		status,
	)

	return err
}

func (r *Repository) InsertFromReel(
	ctx context.Context,
	reel ExtractedReel,
	classification NewsClassification,
	status string,
) (bool, error) {
	now := time.Now()

	var publishedAt *time.Time
	if status == NewsStatusPublished {
		publishedAt = reel.PublishedAt
		if publishedAt == nil {
			publishedAt = &now
		}
	}

	commandTag, err := r.db.Exec(ctx, `
		INSERT INTO news_items (
			source,
			source_media_id,
			reel_url,
			thumbnail_url,
			caption,
			title,
			summary,
			body,
			category,
			importance,
			confidence,
			status,
			published_at,
			scraped_at,
			classified_at
		)
		VALUES (
			'instagram',
			$1,
			$2,
			NULLIF($3, ''),
			NULLIF($4, ''),
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11,
			$12,
			$13,
			$14
		)
		ON CONFLICT (source, source_media_id) DO NOTHING
	`,
		reel.SourceMediaID,
		reel.ReelURL,
		reel.ThumbnailURL,
		reel.Caption,
		classification.Title,
		classification.Summary,
		classification.Body,
		classification.Category,
		classification.Importance,
		classification.Confidence,
		status,
		publishedAt,
		now,
		now,
	)
	if err != nil {
		return false, err
	}

	return commandTag.RowsAffected() == 1, nil
}

func (r *Repository) UpdateExistingFromReel(
	ctx context.Context,
	reel ExtractedReel,
	classification NewsClassification,
	status string,
) error {
	now := time.Now()

	var publishedAt *time.Time
	if status == NewsStatusPublished {
		publishedAt = reel.PublishedAt
	}

	_, err := r.db.Exec(ctx, `
		UPDATE news_items
		SET
			reel_url = COALESCE(NULLIF($3, ''), reel_url),
			thumbnail_url = COALESCE(NULLIF($4, ''), thumbnail_url),
			caption = COALESCE(NULLIF($5, ''), caption),
			title = $6,
			summary = $7,
			body = $8,
			category = $9,
			importance = $10,
			confidence = $11,
			status = $12,
			published_at = CASE
				WHEN $12 = 'published' THEN COALESCE($13, published_at, $14)
				ELSE NULL
			END,
			scraped_at = $14,
			classified_at = $15,
			updated_at = now()
		WHERE source = $1
		AND source_media_id = $2
	`,
		"instagram",
		reel.SourceMediaID,
		reel.ReelURL,
		reel.ThumbnailURL,
		reel.Caption,
		classification.Title,
		classification.Summary,
		classification.Body,
		classification.Category,
		classification.Importance,
		classification.Confidence,
		status,
		publishedAt,
		now,
		now,
	)

	return err
}

func (r *Repository) InsertManual(ctx context.Context, input CreateManualNewsRequest) (*NewsItem, error) {
	now := time.Now()
	sourceMediaID := "manual-" + uuid.NewString()

	if input.Status == "" {
		input.Status = NewsStatusPublished
	}

	if input.Category == "" {
		input.Category = CategoryInstitutional
	}

	if input.Importance == "" {
		input.Importance = ImportanceMedium
	}

	if input.Body == "" {
		input.Body = input.Summary
	}

	if input.ReelURL == "" {
		input.ReelURL = "manual://" + sourceMediaID
	}

	var publishedAt *time.Time
	if input.Status == NewsStatusPublished {
		publishedAt = &now
	}

	query := `
		INSERT INTO news_items (
			source,
			source_media_id,
			reel_url,
			thumbnail_url,
			caption,
			title,
			summary,
			body,
			category,
			importance,
			confidence,
			status,
			published_at,
			scraped_at,
			classified_at
		)
		VALUES (
			'manual',
			$1,
			$2,
			NULLIF($3, ''),
			NULLIF($4, ''),
			$5,
			$6,
			$7,
			$8,
			$9,
			1.000,
			$10,
			$11,
			$12,
			$13
		)
		RETURNING
			public_id,
			source,
			title,
			summary,
			COALESCE(body, ''),
			COALESCE(thumbnail_url, ''),
			reel_url,
			category,
			importance,
			COALESCE(published_at, created_at)
	`

	var item NewsItem

	err := r.db.QueryRow(
		ctx,
		query,
		sourceMediaID,
		input.ReelURL,
		input.ImageURL,
		input.Summary,
		input.Title,
		input.Summary,
		input.Body,
		input.Category,
		input.Importance,
		input.Status,
		publishedAt,
		now,
		now,
	).Scan(
		&item.ID,
		&item.Source,
		&item.Title,
		&item.Summary,
		&item.Body,
		&item.ImageURL,
		&item.ReelURL,
		&item.Category,
		&item.Importance,
		&item.PublishedAt,
	)

	if err != nil {
		return nil, err
	}

	item.ThumbnailURL = item.ImageURL

	return &item, nil
}

func (r *Repository) ListAdmin(ctx context.Context) ([]AdminNewsItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			id::text,
			source,
			source_media_id,
			title,
			summary,
			COALESCE(body, ''),
			COALESCE(caption, ''),
			category,
			importance,
			COALESCE(confidence, 0),
			status,
			reel_url,
			COALESCE(thumbnail_url, ''),
			created_at::text
		FROM news_items
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []AdminNewsItem{}

	for rows.Next() {
		var item AdminNewsItem

		if err := rows.Scan(
			&item.ID,
			&item.Source,
			&item.SourceMediaID,
			&item.Title,
			&item.Summary,
			&item.Body,
			&item.Caption,
			&item.Category,
			&item.Importance,
			&item.Confidence,
			&item.Status,
			&item.ReelURL,
			&item.ImageURL,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *Repository) UpdateAdmin(ctx context.Context, id string, input UpdateAdminNewsRequest) (*AdminNewsItem, error) {
	query := `
		UPDATE news_items
		SET
			title = COALESCE(NULLIF($2, ''), title),
			summary = COALESCE(NULLIF($3, ''), summary),
			body = COALESCE(NULLIF($4, ''), body),
			thumbnail_url = COALESCE(NULLIF($5, ''), thumbnail_url),
			reel_url = COALESCE(NULLIF($6, ''), reel_url),
			category = COALESCE(NULLIF($7, ''), category),
			importance = COALESCE(NULLIF($8, ''), importance),
			status = COALESCE(NULLIF($9, ''), status),
			published_at = CASE
				WHEN NULLIF($9, '') = 'published' AND published_at IS NULL THEN now()
				WHEN NULLIF($9, '') IN ('draft', 'hidden', 'needs_review') THEN NULL
				ELSE published_at
			END,
			updated_at = now()
		WHERE id = $1
		RETURNING
			id::text,
			source,
			source_media_id,
			title,
			summary,
			COALESCE(body, ''),
			COALESCE(caption, ''),
			category,
			importance,
			COALESCE(confidence, 0),
			status,
			reel_url,
			COALESCE(thumbnail_url, ''),
			created_at::text
	`

	return scanAdminNewsItem(r.db.QueryRow(
		ctx,
		query,
		id,
		input.Title,
		input.Summary,
		input.Body,
		input.ImageURL,
		input.ReelURL,
		input.Category,
		input.Importance,
		input.Status,
	))
}

func (r *Repository) SetStatus(ctx context.Context, id string, status string) (*AdminNewsItem, error) {
	query := `
		UPDATE news_items
		SET
			status = $2,
			published_at = CASE
				WHEN $2 = 'published' AND published_at IS NULL THEN now()
				WHEN $2 IN ('draft', 'hidden', 'needs_review') THEN NULL
				ELSE published_at
			END,
			updated_at = now()
		WHERE id = $1
		RETURNING
			id::text,
			source,
			source_media_id,
			title,
			summary,
			COALESCE(body, ''),
			COALESCE(caption, ''),
			category,
			importance,
			COALESCE(confidence, 0),
			status,
			reel_url,
			COALESCE(thumbnail_url, ''),
			created_at::text
	`

	return scanAdminNewsItem(r.db.QueryRow(ctx, query, id, status))
}

func (r *Repository) StartJobRun(ctx context.Context, jobName string) (string, error) {
	var id string

	err := r.db.QueryRow(ctx, `
		INSERT INTO job_runs (job_name, status)
		VALUES ($1, 'running')
		RETURNING id::text
	`, jobName).Scan(&id)

	return id, err
}

func (r *Repository) FinishJobRun(ctx context.Context, id string, stats SyncStats) error {
	_, err := r.db.Exec(ctx, `
		UPDATE job_runs
		SET
			status = 'succeeded',
			finished_at = now(),
			processed_count = $2,
			extracted_count = $3,
			new_items_count = $4,
			duplicates_count = $5,
			classified_count = $6,
			inserted_count = $7,
			skipped_old_count = $8,
			skipped_not_news_count = $9,
			failed_items_count = $10,
			error_message = NULL
		WHERE id = $1
	`, id, stats.Inserted, stats.Extracted, stats.NewItems, stats.Duplicates, stats.Classified, stats.Inserted, stats.SkippedOld, stats.SkippedNotNews, stats.FailedItems)

	return err
}

func (r *Repository) FailJobRun(ctx context.Context, id string, stats SyncStats, errorMessage string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE job_runs
		SET
			status = 'failed',
			finished_at = now(),
			processed_count = $2,
			extracted_count = $3,
			new_items_count = $4,
			duplicates_count = $5,
			classified_count = $6,
			inserted_count = $7,
			skipped_old_count = $8,
			skipped_not_news_count = $9,
			failed_items_count = $10,
			error_message = NULLIF($11, '')
		WHERE id = $1
	`, id, stats.Inserted, stats.Extracted, stats.NewItems, stats.Duplicates, stats.Classified, stats.Inserted, stats.SkippedOld, stats.SkippedNotNews, stats.FailedItems, errorMessage)

	return err
}

func (r *Repository) ListJobRuns(ctx context.Context, limit int) ([]JobRun, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id::text,
			job_name,
			status,
			started_at,
			finished_at,
			COALESCE(extracted_count, 0),
			COALESCE(new_items_count, 0),
			COALESCE(duplicates_count, 0),
			COALESCE(classified_count, 0),
			COALESCE(inserted_count, 0),
			COALESCE(skipped_old_count, 0),
			COALESCE(skipped_not_news_count, 0),
			COALESCE(failed_items_count, 0),
			COALESCE(error_message, '')
		FROM job_runs
		WHERE job_name = $1
		ORDER BY started_at DESC
		LIMIT $2
	`, syncInstagramReelsJobName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []JobRun{}
	for rows.Next() {
		var run JobRun
		var finishedAt pgtype.Timestamptz

		if err := rows.Scan(
			&run.ID,
			&run.JobName,
			&run.Status,
			&run.StartedAt,
			&finishedAt,
			&run.ExtractedCount,
			&run.NewItemsCount,
			&run.DuplicatesCount,
			&run.ClassifiedCount,
			&run.InsertedCount,
			&run.SkippedOldCount,
			&run.SkippedNotNewsCount,
			&run.FailedItemsCount,
			&run.ErrorMessage,
		); err != nil {
			return nil, err
		}

		if finishedAt.Valid {
			run.FinishedAt = &finishedAt.Time
		}

		runs = append(runs, run)
	}

	return runs, rows.Err()
}

func scanAdminNewsItem(row interface {
	Scan(dest ...any) error
}) (*AdminNewsItem, error) {
	var item AdminNewsItem

	err := row.Scan(
		&item.ID,
		&item.Source,
		&item.SourceMediaID,
		&item.Title,
		&item.Summary,
		&item.Body,
		&item.Caption,
		&item.Category,
		&item.Importance,
		&item.Confidence,
		&item.Status,
		&item.ReelURL,
		&item.ImageURL,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &item, nil
}
