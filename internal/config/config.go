package config

import (
	"os"
	"strconv"
)

type Config struct {
	AppEnv                  string
	Port                    string
	DatabaseURL             string
	CorsOrigin              string
	InstagramUsername       string
	OpenAIAPIKey            string
	OpenAIModel             string
	NewsSyncSecret          string
	NewsRecentDays          int
	NewsSyncIntervalMinutes int
	NewsSyncOnStartup       bool
	NewsFeaturedLimit       int
	OllamaEnabled           bool
	OllamaBaseURL           string
	OllamaModel             string
}

func Load() Config {
	recentDays, err := strconv.Atoi(getEnv("NEWS_RECENT_DAYS", "90"))
	if err != nil || recentDays <= 0 {
		recentDays = 90
	}

	syncIntervalMinutes, err := strconv.Atoi(getEnv("NEWS_SYNC_INTERVAL_MINUTES", "60"))
	if err != nil || syncIntervalMinutes < 0 {
		syncIntervalMinutes = 60
	}

	featuredLimit, err := strconv.Atoi(getEnv("NEWS_FEATURED_LIMIT", "3"))
	if err != nil || featuredLimit < 0 {
		featuredLimit = 3
	}

	return Config{
		AppEnv:                  getEnv("APP_ENV", "development"),
		Port:                    getEnv("PORT", "8080"),
		DatabaseURL:             getEnv("DATABASE_URL", ""),
		CorsOrigin:              getEnv("CORS_ORIGIN", "http://localhost:5173"),
		InstagramUsername:       getEnv("INSTAGRAM_TARGET_USERNAME", ""),
		OpenAIAPIKey:            getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:             getEnv("OPENAI_MODEL", "gpt-4.1-mini"),
		NewsSyncSecret:          getEnv("NEWS_SYNC_SECRET", ""),
		NewsRecentDays:          recentDays,
		NewsSyncIntervalMinutes: syncIntervalMinutes,
		NewsSyncOnStartup:       getEnv("NEWS_SYNC_ON_STARTUP", "true") == "true",
		NewsFeaturedLimit:       featuredLimit,
		OllamaEnabled:           getEnv("OLLAMA_ENABLED", "false") == "true",
		OllamaBaseURL:           getEnv("OLLAMA_BASE_URL", ""),
		OllamaModel:             getEnv("OLLAMA_MODEL", "qwen2.5:7b-instruct-q4_K_M"),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
