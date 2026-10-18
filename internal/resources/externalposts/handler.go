package externalposts

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"example.com/go-api-template/internal/common"
	"example.com/go-api-template/internal/integrations/posts"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	client *posts.Client
	logger *slog.Logger
}

func NewHandler(client *posts.Client, logger *slog.Logger) *Handler {
	return &Handler{client: client, logger: logger}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/{id}", h.get)
	return r
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		common.Error(w, r, http.StatusBadRequest, "invalid external post id")
		return
	}
	post, err := h.client.Get(r.Context(), id)
	if errors.Is(err, posts.ErrNotFound) {
		common.Error(w, r, http.StatusNotFound, "external post not found")
		return
	}
	if err != nil {
		h.logger.Error("external post request failed", "error", err, "request_id", middleware.GetReqID(r.Context()))
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			common.Error(w, r, http.StatusGatewayTimeout, "external API timed out")
			return
		}
		common.Error(w, r, http.StatusBadGateway, "external API unavailable")
		return
	}
	common.JSON(w, http.StatusOK, post)
}
