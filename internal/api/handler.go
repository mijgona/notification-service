package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/mijgona/notification-service/internal/storage"
)

type Handler struct {
	notifications *storage.Notifications
	attempts      *storage.Attempts
	log           *slog.Logger
}

func NewHandler(n *storage.Notifications, a *storage.Attempts, log *slog.Logger) *Handler {
	return &Handler{notifications: n, attempts: a, log: log}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/notifications", h.create)
	mux.HandleFunc("GET /v1/notifications/{id}", h.get)
	mux.HandleFunc("GET /healthz", h.health)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	n, created, err := h.notifications.CreateWithOutbox(r.Context(), storage.Notification{
		IdempotencyKey: req.IdempotencyKey,
		Channel:        req.Channel,
		Recipient:      req.Recipient,
		Subject:        req.Subject,
		Body:           req.Body,
	})
	if err != nil {
		h.log.Error("create notification", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":        n.ID,
		"status":    n.Status,
		"duplicate": !created,
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	n, err := h.notifications.GetByID(r.Context(), id)
	if err != nil {
		// TODO(mijgona): distinguish pgx.ErrNoRows (404) from real
		// database failures (500) — right now everything maps to 404.
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}

	attempts, err := h.attempts.ListByNotification(r.Context(), id)
	if err != nil {
		h.log.Error("list delivery attempts", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"notification": n,
		"attempts":     attempts,
	})
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
