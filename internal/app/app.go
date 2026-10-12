package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"example.com/go-api-template/internal/common"
	"example.com/go-api-template/internal/config"
	"example.com/go-api-template/internal/resources/tasks"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Router(db *pgxpool.Pool, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set(middleware.RequestIDHeader, middleware.GetReqID(req.Context()))
			start := time.Now()
			response := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
			defer func() {
				status := response.Status()
				if status == 0 {
					status = http.StatusOK
				}
				logger.Info("http request", "method", req.Method, "path", req.URL.Path, "status", status, "duration_ms", time.Since(start).Milliseconds(), "request_id", middleware.GetReqID(req.Context()))
			}()
			next.ServeHTTP(response, req)
		})
	})
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) { common.JSON(w, 200, map[string]string{"status": "ok"}) })
	r.Get("/ready", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			common.Error(w, req, 503, "database unavailable")
			return
		}
		common.JSON(w, 200, map[string]string{"status": "ready"})
	})
	r.Mount("/api/v1/tasks", tasks.NewHandler(tasks.NewRepository(db), logger).Routes())
	return r
}
func Run(ctx context.Context, cfg config.Config, db *pgxpool.Pool, logger *slog.Logger) error {
	server := &http.Server{Addr: ":" + cfg.Port, Handler: Router(db, logger), ReadHeaderTimeout: cfg.ReadHeaderTimeout}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	logger.Info("server listening", "port", cfg.Port)
	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}
