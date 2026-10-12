package config

import (
	"testing"
	"time"
)

func TestConfigRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_MAX_CONNS", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing database url error")
	}
}

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		name         string
		port         string
		maxConns     string
		wantPort     string
		wantMaxConns int32
	}{
		{name: "defaults", wantPort: "8080", wantMaxConns: 5},
		{name: "overrides", port: "9090", maxConns: "12", wantPort: "9090", wantMaxConns: 12},
		{name: "minimum connections", maxConns: "1", wantPort: "8080", wantMaxConns: 1},
		{name: "maximum connections", maxConns: "2147483647", wantPort: "8080", wantMaxConns: 2147483647},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const databaseURL = "postgres://localhost/test"
			t.Setenv("DATABASE_URL", databaseURL)
			t.Setenv("PORT", tc.port)
			t.Setenv("DB_MAX_CONNS", tc.maxConns)
			got, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			want := Config{
				Port:              tc.wantPort,
				DatabaseURL:       databaseURL,
				DBMaxConns:        tc.wantMaxConns,
				ReadHeaderTimeout: 5 * time.Second,
				ShutdownTimeout:   10 * time.Second,
			}
			if got != want {
				t.Fatalf("config = %+v, want %+v", got, want)
			}
		})
	}
}

func TestLoadRejectsInvalidMaxConns(t *testing.T) {
	for _, value := range []string{"0", "-1", "many", "1.5", "2147483648"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/test")
			t.Setenv("DB_MAX_CONNS", value)
			if _, err := Load(); err == nil {
				t.Fatalf("DB_MAX_CONNS = %q: expected an error", value)
			}
		})
	}
}
