package tasks

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"example.com/go-api-template/internal/common"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	repo   *Repository
	redis  *redis.Client
	logger *slog.Logger
}

func NewHandler(repo *Repository, redisClient *redis.Client, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, redis: redisClient, logger: logger}
}
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Delete("/{id}", h.delete)
	return r
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	data, err := h.repo.List(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	common.JSON(w, 200, data)
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var payload CreateTask
	if err := common.DecodeJSON(r, &payload); err != nil {
		common.Error(w, r, 400, "invalid JSON body")
		return
	}
	payload.Title = strings.TrimSpace(payload.Title)
	if length := utf8.RuneCountInString(payload.Title); length == 0 || length > 200 {
		common.Error(w, r, 400, "title must contain 1-200 characters")
		return
	}
	task, err := h.repo.Create(r.Context(), payload.Title)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	common.JSON(w, 201, task)
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(chi.URLParam(r, "id"))
	if !ok {
		common.Error(w, r, 400, "invalid task id")
		return
	}
	if task, ok := h.cachedTask(r.Context(), id); ok {
		common.JSON(w, http.StatusOK, task)
		return
	}
	task, err := h.repo.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		common.Error(w, r, 404, "task not found")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.cacheTask(r.Context(), task)
	common.JSON(w, 200, task)
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(chi.URLParam(r, "id"))
	if !ok {
		common.Error(w, r, 400, "invalid task id")
		return
	}
	err := h.repo.Delete(r.Context(), id)
	if err == nil || errors.Is(err, ErrNotFound) {
		h.invalidateTask(r.Context(), id)
	}
	if errors.Is(err, ErrNotFound) {
		common.Error(w, r, 404, "task not found")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("task request failed", "error", err, "request_id", middleware.GetReqID(r.Context()))
	common.Error(w, r, http.StatusInternalServerError, "internal server error")
}
func parseID(value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0
}
