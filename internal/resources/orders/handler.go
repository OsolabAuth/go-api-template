package orders

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
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	return r
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input CreateOrder
	if err := common.DecodeJSON(r, &input); err != nil {
		common.Error(w, r, http.StatusBadRequest, "invalid JSON body")
		return
	}
	input.CustomerName = strings.TrimSpace(input.CustomerName)
	if length := utf8.RuneCountInString(input.CustomerName); length == 0 || length > 200 || strings.ContainsRune(input.CustomerName, 0) {
		common.Error(w, r, http.StatusBadRequest, "customer_name must contain 1-200 characters without NUL")
		return
	}
	if len(input.Items) == 0 || len(input.Items) > 100 {
		common.Error(w, r, http.StatusBadRequest, "items must contain 1-100 entries")
		return
	}
	for i := range input.Items {
		input.Items[i].SKU = strings.TrimSpace(input.Items[i].SKU)
		if length := utf8.RuneCountInString(input.Items[i].SKU); length == 0 || length > 64 || strings.ContainsRune(input.Items[i].SKU, 0) {
			common.Error(w, r, http.StatusBadRequest, "sku must contain 1-64 characters without NUL")
			return
		}
		if input.Items[i].Quantity < 1 || input.Items[i].Quantity > 1000 {
			common.Error(w, r, http.StatusBadRequest, "quantity must be an integer between 1 and 1000")
			return
		}
	}
	order, err := h.repo.Create(r.Context(), input)
	if errors.Is(err, ErrDuplicateSKU) {
		common.Error(w, r, http.StatusConflict, ErrDuplicateSKU.Error())
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	common.JSON(w, http.StatusCreated, order)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		common.Error(w, r, http.StatusBadRequest, "invalid order id")
		return
	}
	order, err := h.repo.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		common.Error(w, r, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, order)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("order request failed", "error", err, "request_id", middleware.GetReqID(r.Context()))
	common.Error(w, r, http.StatusInternalServerError, "internal server error")
}
