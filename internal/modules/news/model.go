package news

import "time"

type NewsItem struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	Summary      string    `json:"summary"`
	Body         string    `json:"body"`
	ImageURL     string    `json:"imageUrl"`
	ThumbnailURL string    `json:"thumbnailUrl"`
	PublishedAt  time.Time `json:"publishedAt"`
	Category     string    `json:"category"`
	Importance   string    `json:"importance"`
	Source       string    `json:"source"`
	ReelURL      string    `json:"reelUrl"`
}

type ListParams struct {
	Category      string
	Importance    string
	Limit         int
	Offset        int
	FeaturedLimit int
}

type ListResponse struct {
	Data       []NewsItem `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type ExtractedReel struct {
	SourceMediaID string
	ReelURL       string
	ThumbnailURL  string
	Caption       string
	PublishedAt   *time.Time
}

type NewsClassification struct {
	Category   string
	Title      string
	Summary    string
	Body       string
	Importance string
	Confidence float64
	IsNews     bool
	Reason     string
}

type SyncStats struct {
	Extracted      int `json:"extracted"`
	NewItems       int `json:"newItems"`
	Duplicates     int `json:"duplicates"`
	Classified     int `json:"classified"`
	Inserted       int `json:"inserted"`
	SkippedOld     int `json:"skippedOld"`
	SkippedNotNews int `json:"skippedNotNews"`
	FailedItems    int `json:"failedItems"`
}

type AIReprocessCandidate struct {
	ID            string
	SourceMediaID string
	ReelURL       string
	ThumbnailURL  string
	Caption       string
	PublishedAt   *time.Time
}

type TitleRepairCandidate struct {
	ID      string
	Title   string
	Summary string
	Body    string
	Caption string
}

type AIReprocessStats struct {
	Scanned   int `json:"scanned"`
	Processed int `json:"processed"`
	Published int `json:"published"`
	Rejected  int `json:"rejected"`
	Failed    int `json:"failed"`
}

type CreateManualNewsRequest struct {
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Body       string `json:"body"`
	ImageURL   string `json:"imageUrl"`
	ReelURL    string `json:"reelUrl"`
	Category   string `json:"category"`
	Importance string `json:"importance"`
	Status     string `json:"status"`
}

type UpdateAdminNewsRequest struct {
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Body       string `json:"body"`
	ImageURL   string `json:"imageUrl"`
	ReelURL    string `json:"reelUrl"`
	Category   string `json:"category"`
	Importance string `json:"importance"`
	Status     string `json:"status"`
}

type AdminNewsItem struct {
	ID            string  `json:"id"`
	Source        string  `json:"source"`
	SourceMediaID string  `json:"sourceMediaId"`
	Title         string  `json:"title"`
	Summary       string  `json:"summary"`
	Body          string  `json:"body"`
	Caption       string  `json:"caption"`
	Category      string  `json:"category"`
	Importance    string  `json:"importance"`
	Confidence    float64 `json:"confidence"`
	Status        string  `json:"status"`
	ReelURL       string  `json:"reelUrl"`
	ImageURL      string  `json:"imageUrl"`
	CreatedAt     string  `json:"createdAt"`
}

type JobRun struct {
	ID                  string     `json:"id"`
	JobName             string     `json:"jobName"`
	Status              string     `json:"status"`
	StartedAt           time.Time  `json:"startedAt"`
	FinishedAt          *time.Time `json:"finishedAt"`
	ExtractedCount      int        `json:"extractedCount"`
	NewItemsCount       int        `json:"newItemsCount"`
	DuplicatesCount     int        `json:"duplicatesCount"`
	ClassifiedCount     int        `json:"classifiedCount"`
	InsertedCount       int        `json:"insertedCount"`
	SkippedOldCount     int        `json:"skippedOldCount"`
	SkippedNotNewsCount int        `json:"skippedNotNewsCount"`
	FailedItemsCount    int        `json:"failedItemsCount"`
	ErrorMessage        string     `json:"errorMessage"`
}
