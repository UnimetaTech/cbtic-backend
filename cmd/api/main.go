package main

import (
	"context"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/cbtic/cbtic-backend/internal/config"
	"github.com/cbtic/cbtic-backend/internal/modules/exam"
	"github.com/cbtic/cbtic-backend/internal/modules/news"
	"github.com/cbtic/cbtic-backend/internal/platform/database"
	"github.com/cbtic/cbtic-backend/internal/platform/httpserver"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("error conectando a PostgreSQL: %v", err)
	}
	defer db.Close()

	newsRepo := news.NewRepository(db)
	newsService := news.NewService(newsRepo, cfg.NewsFeaturedLimit)

	extractor := news.NewInstagramHTMLReelExtractor(30)
	classifier := news.NewOllamaNewsClassifier(
		cfg.OllamaEnabled,
		cfg.OllamaBaseURL,
		cfg.OllamaModel,
		news.NewSimpleNewsClassifier(),
	)

	newsSyncer := news.NewSyncer(
		newsRepo,
		extractor,
		classifier,
		cfg.InstagramUsername,
		cfg.NewsRecentDays,
	)

	newsHandler := news.NewHandler(newsService, newsSyncer, cfg.NewsSyncSecret)

	examRepo := exam.NewRepository(db)
	examMailer := exam.NewMailer(exam.SMTPConfig{
		Host: cfg.SMTPHost,
		Port: cfg.SMTPPort,
		User: cfg.SMTPUser,
		Pass: cfg.SMTPPass,
		From: cfg.SMTPFrom,
	})
	examService := exam.NewService(examRepo, examMailer, cfg.ExamTeacherEmail)
	examHandler := exam.NewHandler(examService)

	router := httpserver.NewRouter(cfg, newsHandler, examHandler)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("CBTIC Backend escuchando en puerto %s", cfg.Port)
	startNewsSyncScheduler(context.Background(), newsSyncer, cfg.NewsSyncIntervalMinutes, cfg.NewsSyncOnStartup)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("error iniciando servidor: %v", err)
	}
}

func startNewsSyncScheduler(ctx context.Context, syncer *news.Syncer, intervalMinutes int, runOnStartup bool) {
	var running atomic.Bool

	run := func(trigger string) {
		if !running.CompareAndSwap(false, true) {
			log.Printf("news sync omitido (%s): ya hay una sincronizacion en curso", trigger)
			return
		}
		defer running.Store(false)

		stats, err := syncer.Sync(ctx)
		if err != nil {
			log.Printf("news sync fallo (%s): %v", trigger, err)
			return
		}

		log.Printf("news sync finalizado (%s): %+v", trigger, stats)
	}

	if runOnStartup {
		go run("startup")
	}

	if intervalMinutes <= 0 {
		log.Print("news sync scheduler periodico deshabilitado")
		return
	}

	interval := time.Duration(intervalMinutes) * time.Minute

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("news sync scheduler activo cada %s", interval)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run("ticker")
			}
		}
	}()
}
