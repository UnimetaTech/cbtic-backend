package main

import (
	"context"
	"log"
	"time"

	"github.com/cbtic/cbtic-backend/internal/config"
	"github.com/cbtic/cbtic-backend/internal/modules/news"
	"github.com/cbtic/cbtic-backend/internal/platform/database"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("error conectando a PostgreSQL: %v", err)
	}
	defer db.Close()

	newsRepo := news.NewRepository(db)
	extractor := news.NewInstagramHTMLReelExtractor(30)
	classifier := news.NewOllamaNewsClassifier(
		cfg.OllamaEnabled,
		cfg.OllamaBaseURL,
		cfg.OllamaModel,
		news.NewSimpleNewsClassifier(),
	)

	syncer := news.NewSyncer(
		newsRepo,
		extractor,
		classifier,
		cfg.InstagramUsername,
		cfg.NewsRecentDays,
	)

	stats, err := syncer.Sync(ctx)
	if err != nil {
		log.Fatalf("error ejecutando sync-instagram-reels: %v", err)
	}

	log.Printf("sync-instagram-reels finalizado: %+v", stats)
}
