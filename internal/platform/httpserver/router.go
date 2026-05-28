package httpserver

import (
	"net/http"

	"github.com/cbtic/cbtic-backend/internal/config"
	"github.com/cbtic/cbtic-backend/internal/modules/exam"
	"github.com/cbtic/cbtic-backend/internal/modules/news"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func NewRouter(cfg config.Config, newsHandler *news.Handler, examHandler *exam.Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{
			cfg.CorsOrigin,
			"http://localhost:5173",
		},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/api", func(r chi.Router) {
		newsHandler.Routes(r)
		examHandler.Routes(r)

		// Futuro:
		// projectsHandler.Routes(r)
		// usersHandler.Routes(r)
		// chatbotHandler.Routes(r)
	})

	return r
}
