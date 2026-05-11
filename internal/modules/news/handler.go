package news

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service        *Service
	syncer         *Syncer
	newsSyncSecret string
	imageClient    *http.Client
}

func NewHandler(service *Service, syncer *Syncer, newsSyncSecret string) *Handler {
	return &Handler{
		service:        service,
		syncer:         syncer,
		newsSyncSecret: newsSyncSecret,
		imageClient:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/news", h.List)
	r.Get("/news/image", h.ProxyImage)
	r.Get("/news/{id}", h.GetByID)

	r.Get("/admin/news", h.ListAdmin)
	r.Post("/admin/news", h.CreateManual)
	r.Patch("/admin/news/{id}", h.UpdateAdmin)
	r.Post("/admin/news/{id}/publish", h.Publish)
	r.Post("/admin/news/{id}/hide", h.Hide)
	r.Post("/admin/news/sync", h.SyncManually)
	r.Post("/admin/news/ai/reprocess", h.ReprocessWithAI)
	r.Get("/admin/news/sync/runs", h.ListSyncRuns)
	r.Get("/admin/news/sync/runs/latest", h.GetLatestSyncRun)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if !h.syncer.ClassifierReady() {
		writeError(w, http.StatusServiceUnavailable, "Las noticias todavía no están listas: Ollama no está configurado")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	params := ListParams{
		Category:   r.URL.Query().Get("category"),
		Importance: r.URL.Query().Get("importance"),
		Limit:      limit,
		Offset:     offset,
	}

	response, err := h.service.List(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error consultando noticias")
		return
	}

	proxyListResponseImages(r, &response)
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	if !h.syncer.ClassifierReady() {
		writeError(w, http.StatusServiceUnavailable, "Las noticias todavía no están listas: Ollama no está configurado")
		return
	}

	id := chi.URLParam(r, "id")

	item, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Noticia no encontrada")
		return
	}

	proxyNewsItemImages(r, item)
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateManual(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "No autorizado")
		return
	}

	var input CreateManualNewsRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	input = normalizeManualNewsInput(input)

	if validationMessage := validateManualNewsInput(input); validationMessage != "" {
		writeError(w, http.StatusBadRequest, validationMessage)
		return
	}

	item, err := h.service.CreateManual(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error creando noticia")
		return
	}

	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) UpdateAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "No autorizado")
		return
	}

	var input UpdateAdminNewsRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	input = normalizeUpdateNewsInput(input)
	if validationMessage := validateUpdateNewsInput(input); validationMessage != "" {
		writeError(w, http.StatusBadRequest, validationMessage)
		return
	}

	item, err := h.service.UpdateAdmin(r.Context(), chi.URLParam(r, "id"), input)
	if err != nil {
		writeError(w, http.StatusNotFound, "Noticia no encontrada o no actualizable")
		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	h.setAdminStatus(w, r, NewsStatusPublished)
}

func (h *Handler) Hide(w http.ResponseWriter, r *http.Request) {
	h.setAdminStatus(w, r, NewsStatusHidden)
}

func (h *Handler) setAdminStatus(w http.ResponseWriter, r *http.Request, status string) {
	if !h.isAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "No autorizado")
		return
	}

	var (
		item *AdminNewsItem
		err  error
	)

	switch status {
	case NewsStatusPublished:
		item, err = h.service.Publish(r.Context(), chi.URLParam(r, "id"))
	case NewsStatusHidden:
		item, err = h.service.Hide(r.Context(), chi.URLParam(r, "id"))
	default:
		writeError(w, http.StatusBadRequest, "Status inválido")
		return
	}

	if err != nil {
		writeError(w, http.StatusNotFound, "Noticia no encontrada")
		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) SyncManually(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "No autorizado")
		return
	}

	stats, err := h.syncer.Sync(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) ReprocessWithAI(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "No autorizado")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	stats, err := h.syncer.ReprocessExistingWithAI(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "No autorizado")
		return
	}

	items, err := h.service.ListAdmin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error consultando noticias")
		return
	}

	proxyAdminNewsItemImages(r, items)
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  items,
		"total": len(items),
	})
}

func (h *Handler) ListSyncRuns(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "No autorizado")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := h.service.ListSyncRuns(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error consultando ejecuciones del worker")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  runs,
		"total": len(runs),
	})
}

func (h *Handler) GetLatestSyncRun(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "No autorizado")
		return
	}

	runs, err := h.service.ListSyncRuns(r.Context(), 1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error consultando última ejecución del worker")
		return
	}

	if len(runs) == 0 {
		writeError(w, http.StatusNotFound, "No hay ejecuciones registradas")
		return
	}

	writeJSON(w, http.StatusOK, runs[0])
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{
		"error": message,
	})
}

func (h *Handler) isAuthorized(r *http.Request) bool {
	if h.newsSyncSecret == "" {
		return false
	}

	authHeader := r.Header.Get("Authorization")
	expected := "Bearer " + h.newsSyncSecret

	if len(authHeader) != len(expected) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(authHeader), []byte(expected)) == 1
}

func validateManualNewsInput(input CreateManualNewsRequest) string {
	if input.Title == "" {
		return "El campo title es obligatorio"
	}

	if input.Summary == "" {
		return "El campo summary es obligatorio"
	}

	if !IsValidNewsStatus(input.Status) {
		return "Status inválido. Use published, draft, hidden o needs_review"
	}

	if !IsValidImportance(input.Importance) {
		return "Importance inválida. Use alta, media o baja"
	}

	if !IsValidCategory(input.Category) {
		return "Category inválida"
	}

	if input.ImageURL != "" && !isValidHTTPURL(input.ImageURL) {
		return "imageUrl debe ser una URL HTTP(S) válida"
	}

	if input.ReelURL != "" && !isValidReelURL(input.ReelURL) {
		return "reelUrl debe ser una URL HTTP(S) válida o manual://..."
	}

	return ""
}

func validateUpdateNewsInput(input UpdateAdminNewsRequest) string {
	if input.Status != "" && !IsValidNewsStatus(input.Status) {
		return "Status inválido. Use published, draft, hidden o needs_review"
	}

	if input.Importance != "" && !IsValidImportance(input.Importance) {
		return "Importance inválida. Use alta, media o baja"
	}

	if input.Category != "" && !IsValidCategory(input.Category) {
		return "Category inválida"
	}

	if input.ImageURL != "" && !isValidHTTPURL(input.ImageURL) {
		return "imageUrl debe ser una URL HTTP(S) válida"
	}

	if input.ReelURL != "" && !isValidReelURL(input.ReelURL) {
		return "reelUrl debe ser una URL HTTP(S) válida o manual://..."
	}

	return ""
}

func isValidHTTPURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}

	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func isValidReelURL(raw string) bool {
	if strings.HasPrefix(raw, "manual://") {
		return true
	}

	return isValidHTTPURL(raw)
}
