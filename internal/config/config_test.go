package config

import "testing"

func TestConfigRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing database url error")
	}
}

func TestExternalAPIURL(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "")
	t.Setenv("DATABASE_URL", "postgres://test")
	for _, tt := range []struct{ value, want string }{
		{"", "https://jsonplaceholder.typicode.com"},
		{"http://127.0.0.1:9000", "http://127.0.0.1:9000"},
	} {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("EXTERNAL_API_URL", tt.value)
			cfg, err := Load()
			if err != nil || cfg.ExternalAPIURL != tt.want {
				t.Fatalf("Load() = %+v, %v; want external API %q", cfg, err, tt.want)
			}
		})
	}
}
