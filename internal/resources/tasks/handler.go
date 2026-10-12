package tasks

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"example.com/go-api-template/internal/common"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	repo   *Repository
	logger *slog.Logger
}

func NewHandler(repo *Repository, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, logger: logger}
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
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		common.Error(w, r, http.StatusBadRequest, "invalid query string")
		return
	}
	var afterID int64
	if values, present := query["after_id"]; present {
		if len(values) != 1 {
			common.Error(w, r, http.StatusBadRequest, "after_id must be a single non-negative integer")
			return
		}
		afterID, err = strconv.ParseInt(values[0], 10, 64)
		if err != nil || afterID < 0 {
			common.Error(w, r, http.StatusBadRequest, "after_id must be a single non-negative integer")
			return
		}
	}
	limit := 100
	if values, present := query["limit"]; present {
		if len(values) != 1 {
			common.Error(w, r, http.StatusBadRequest, "limit must be a single integer between 1 and 100")
			return
		}
		limit, err = strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > 100 {
			common.Error(w, r, http.StatusBadRequest, "limit must be a single integer between 1 and 100")
			return
		}
	}
	data, err := h.repo.List(r.Context(), afterID, limit)
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
	task, err := h.repo.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		common.Error(w, r, 404, "task not found")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	common.JSON(w, 200, task)
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(chi.URLParam(r, "id"))
	if !ok {
		common.Error(w, r, 400, "invalid task id")
		return
	}
	err := h.repo.Delete(r.Context(), id)
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
