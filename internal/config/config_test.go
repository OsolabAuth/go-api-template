package config

import "testing"

func TestConfig(t *testing.T) {
	for _, tt := range []struct {
		name        string
		databaseURL string
		redisURL    string
		wantError   string
	}{
		{"missing database", "", "redis://localhost:6379/0", "DATABASE_URL must be set"},
		{"missing redis", "postgres://localhost/app", "", "REDIS_URL must be set"},
		{"both configured", "postgres://localhost/app", "redis://localhost:6379/2", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tt.databaseURL)
			t.Setenv("REDIS_URL", tt.redisURL)
			t.Setenv("DB_MAX_CONNS", "")
			cfg, err := Load()
			if tt.wantError != "" {
				if err == nil || err.Error() != tt.wantError {
					t.Fatalf("Load() error = %v, want %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DatabaseURL != tt.databaseURL || cfg.RedisURL != tt.redisURL {
				t.Fatalf("Load() URLs = %q, %q; want %q, %q", cfg.DatabaseURL, cfg.RedisURL, tt.databaseURL, tt.redisURL)
			}
		})
	}
}
