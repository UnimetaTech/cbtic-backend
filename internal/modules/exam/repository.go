package exam

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrAlreadySubmitted indica que el estudiante ya presentó este examen.
var ErrAlreadySubmitted = errors.New("el estudiante ya presentó este examen")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) HasSubmitted(ctx context.Context, examID, email string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM exam_submissions
			WHERE exam_id = $1 AND lower(email) = lower($2)
		)`, examID, email).Scan(&exists)
	return exists, err
}

type InsertParams struct {
	ExamID    string
	Email     string
	Score     float64
	MaxScore  float64
	Status    string
	Answers   AnswersPayload
	Breakdown GradeResult
	ClientIP  string
	UserAgent string
}

// Insert persiste la presentación. El índice único (exam_id, lower(email)) es la
// barrera real contra dobles intentos: si ya existe, devolvemos ErrAlreadySubmitted.
func (r *Repository) Insert(ctx context.Context, p InsertParams) (*Submission, error) {
	answersJSON, err := json.Marshal(p.Answers)
	if err != nil {
		return nil, err
	}
	breakdownJSON, err := json.Marshal(p.Breakdown)
	if err != nil {
		return nil, err
	}

	var (
		id        string
		createdAt time.Time
	)
	err = r.db.QueryRow(ctx, `
		INSERT INTO exam_submissions
			(exam_id, email, score, max_score, status, answers, breakdown, client_ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`,
		p.ExamID, p.Email, p.Score, p.MaxScore, p.Status,
		answersJSON, breakdownJSON, nullable(p.ClientIP), nullable(p.UserAgent),
	).Scan(&id, &createdAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAlreadySubmitted
		}
		return nil, err
	}

	return &Submission{
		ID:        id,
		ExamID:    p.ExamID,
		Email:     p.Email,
		Score:     p.Score,
		MaxScore:  p.MaxScore,
		Status:    p.Status,
		CreatedAt: createdAt,
	}, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
