package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port              string
	DatabaseURL       string
	DBMaxConns        int32
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
}

func Load() (Config, error) {
	cfg := Config{Port: env("PORT", "8080"), DatabaseURL: os.Getenv("DATABASE_URL"), DBMaxConns: 5, ReadHeaderTimeout: 5 * time.Second, ShutdownTimeout: 10 * time.Second}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL must be set")
	}
	if s := os.Getenv("DB_MAX_CONNS"); s != "" {
		n, err := strconv.ParseInt(s, 10, 32)
		if err != nil || n < 1 {
			return Config{}, errors.New("DB_MAX_CONNS must be a positive integer")
		}
		cfg.DBMaxConns = int32(n)
	}
	return cfg, nil
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
