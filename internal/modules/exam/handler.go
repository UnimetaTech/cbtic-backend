package exam

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 64 * 1024 // 64 KiB: el payload de respuestas es pequeño.

// Solo se aceptan correos institucionales de la universidad.
const institutionalDomain = "@unimeta.edu.co"

var emailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]{2,}$`)

func isInstitutionalEmail(email string) bool {
	return emailRegex.MatchString(email) && strings.HasSuffix(email, institutionalDomain)
}

type Handler struct {
	service  *Service
	limiter  *rateLimiter
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
		// Hasta 5 envíos por minuto por IP: corta scripts que intenten
		// inundar el endpoint sin molestar el uso normal.
		limiter: newRateLimiter(5),
	}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/exams/{examId}/status", h.Status)
	r.Post("/exams/{examId}/submit", h.Submit)
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	examID, ok := resolveExamID(chi.URLParam(r, "examId"))
	if !ok {
		writeError(w, http.StatusNotFound, "Examen no encontrado")
		return
	}

	email := normalizeEmail(r.URL.Query().Get("email"))
	if !isInstitutionalEmail(email) {
		writeError(w, http.StatusBadRequest, "Debes usar tu correo institucional @unimeta.edu.co")
		return
	}

	submitted, err := h.service.AlreadySubmitted(r.Context(), examID, email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error consultando el estado del examen")
		return
	}

	writeJSON(w, http.StatusOK, StatusResponse{Submitted: submitted})
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	examID, ok := resolveExamID(chi.URLParam(r, "examId"))
	if !ok {
		writeError(w, http.StatusNotFound, "Examen no encontrado")
		return
	}

	if !h.limiter.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Demasiados intentos. Espera un momento.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req SubmitRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido o demasiado grande")
		return
	}

	req.Email = normalizeEmail(req.Email)
	if !isInstitutionalEmail(req.Email) {
		writeError(w, http.StatusBadRequest, "Debes usar tu correo institucional @unimeta.edu.co")
		return
	}

	req.Status = strings.TrimSpace(req.Status)
	if !IsValidStatus(req.Status) {
		req.Status = StatusCompleted
	}

	resp, err := h.service.Submit(r.Context(), SubmitInput{
		ExamID:    examID,
		Email:     req.Email,
		Status:    req.Status,
		Answers:   req.Answers,
		ClientIP:  clientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		if errors.Is(err, ErrAlreadySubmitted) {
			writeError(w, http.StatusConflict, "Este correo ya presentó el examen. Solo se permite un intento.")
			return
		}
		writeError(w, http.StatusInternalServerError, "No se pudo registrar el examen")
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

func resolveExamID(raw string) (string, bool) {
	switch strings.TrimSpace(raw) {
	case ExamVariableComplejaID:
		return ExamVariableComplejaID, true
	default:
		return "", false
	}
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func clientIP(r *http.Request) string {
	// chi/middleware.RealIP ya resuelve X-Forwarded-For en RemoteAddr.
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	return host
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}
