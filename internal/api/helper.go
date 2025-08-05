package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"2025-07-30/internal/logger"
	"2025-07-30/internal/model"
)

type httpError struct {
	StatusCode int
	StatusMsg  string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.StatusMsg)
}

type helper struct {
	ctx context.Context
	log *slog.Logger
	r   *http.Request
	w   http.ResponseWriter
}

func newHelper(w http.ResponseWriter, r *http.Request, op string) *helper {
	ctx := r.Context()
	return &helper{
		ctx: ctx,
		log: logger.FromContext(ctx).With("op", op),
		w:   w,
		r:   r,
	}
}

func (h *helper) Ctx() context.Context {
	return h.ctx
}

func (h *helper) Logger() *slog.Logger {
	return h.log
}

func (h *helper) WriteError(err error) {
	httpErr := h.mapError(err)
	http.Error(h.w, httpErr.StatusMsg, httpErr.StatusCode)
}

func (h *helper) mapError(err error) *httpError {
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		return httpErr
	}

	switch {
	case errors.Is(err, model.ErrTaskNotFound):
		return &httpError{http.StatusNotFound, err.Error()}
	case errors.Is(err, model.ErrMaxFilesExceeded):
		return &httpError{http.StatusConflict, err.Error()}
	case errors.Is(err, model.ErrServerBusy):
		return &httpError{http.StatusServiceUnavailable, err.Error()}
	case errors.Is(err, model.ErrServerCancelled):
		return &httpError{http.StatusServiceUnavailable, err.Error()}
	}

	h.log.Warn("unhandled error has been detected", "error", err)
	return &httpError{500, "internal error"}
}

func (h *helper) WriteResponse(resp any, statusCode int) {
	h.w.Header().Add("content-type", "application/json")
	h.w.WriteHeader(statusCode)
	err := json.NewEncoder(h.w).Encode(resp)
	if err != nil {
		h.log.Error("write respose failed", "error", err)
	}
}

func (h *helper) GetID() (int64, error) {
	s := h.r.PathValue("id")
	if s == "" {
		return 0, &httpError{http.StatusBadRequest, "id is required"}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, &httpError{http.StatusBadRequest, "id must be integer"}
	}
	if v <= 0 {
		return 0, &httpError{http.StatusBadRequest, "id must be > 0"}
	}
	return v, nil
}

func (h *helper) ReadRequest(req any) error {
	body, err := io.ReadAll(h.r.Body)
	if err != nil {
		msg := "can't read request body"
		h.log.Error(msg, "error", err)
		return &httpError{http.StatusInternalServerError, msg}
	}

	if err := json.Unmarshal(body, req); err != nil {
		msg := "can't parse request body"
		h.log.Error(msg, "error", err)
		return &httpError{http.StatusBadRequest, msg}
	}

	return nil
}
