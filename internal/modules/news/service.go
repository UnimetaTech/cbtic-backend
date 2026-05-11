package news

import (
	"context"
	"strings"
)

type Service struct {
	repo          *Repository
	featuredLimit int
}

func NewService(repo *Repository, featuredLimit int) *Service {
	if featuredLimit <= 0 {
		featuredLimit = 3
	}

	return &Service{
		repo:          repo,
		featuredLimit: featuredLimit,
	}
}

func (s *Service) List(ctx context.Context, params ListParams) (ListResponse, error) {
	if params.Limit <= 0 || params.Limit > 50 {
		params.Limit = 12
	}

	if params.Offset < 0 {
		params.Offset = 0
	}

	params.FeaturedLimit = s.featuredLimit

	items, total, err := s.repo.List(ctx, params)
	if err != nil {
		return ListResponse{}, err
	}

	return ListResponse{
		Data: items,
		Pagination: Pagination{
			Limit:  params.Limit,
			Offset: params.Offset,
			Total:  total,
		},
	}, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*NewsItem, error) {
	return s.repo.GetByID(ctx, id, s.featuredLimit)
}

func (s *Service) CreateManual(ctx context.Context, input CreateManualNewsRequest) (*NewsItem, error) {
	input = normalizeManualNewsInput(input)
	return s.repo.InsertManual(ctx, input)
}

func (s *Service) ListAdmin(ctx context.Context) ([]AdminNewsItem, error) {
	return s.repo.ListAdmin(ctx)
}

func (s *Service) UpdateAdmin(ctx context.Context, id string, input UpdateAdminNewsRequest) (*AdminNewsItem, error) {
	input = normalizeUpdateNewsInput(input)
	return s.repo.UpdateAdmin(ctx, id, input)
}

func (s *Service) Publish(ctx context.Context, id string) (*AdminNewsItem, error) {
	return s.repo.SetStatus(ctx, id, NewsStatusPublished)
}

func (s *Service) Hide(ctx context.Context, id string) (*AdminNewsItem, error) {
	return s.repo.SetStatus(ctx, id, NewsStatusHidden)
}

func (s *Service) ListSyncRuns(ctx context.Context, limit int) ([]JobRun, error) {
	return s.repo.ListJobRuns(ctx, limit)
}

func normalizeManualNewsInput(input CreateManualNewsRequest) CreateManualNewsRequest {
	input.Title = strings.TrimSpace(input.Title)
	input.Summary = strings.TrimSpace(input.Summary)
	input.Body = strings.TrimSpace(input.Body)
	input.ImageURL = strings.TrimSpace(input.ImageURL)
	input.ReelURL = strings.TrimSpace(input.ReelURL)
	input.Category = strings.TrimSpace(input.Category)
	input.Importance = strings.TrimSpace(input.Importance)
	input.Status = strings.TrimSpace(input.Status)

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

	return input
}

func normalizeUpdateNewsInput(input UpdateAdminNewsRequest) UpdateAdminNewsRequest {
	input.Title = strings.TrimSpace(input.Title)
	input.Summary = strings.TrimSpace(input.Summary)
	input.Body = strings.TrimSpace(input.Body)
	input.ImageURL = strings.TrimSpace(input.ImageURL)
	input.ReelURL = strings.TrimSpace(input.ReelURL)
	input.Category = strings.TrimSpace(input.Category)
	input.Importance = strings.TrimSpace(input.Importance)
	input.Status = strings.TrimSpace(input.Status)

	return input
}
