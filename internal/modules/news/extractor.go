package news

import "context"

type ReelExtractor interface {
	GetLatestReels(ctx context.Context, username string) ([]ExtractedReel, error)
}
