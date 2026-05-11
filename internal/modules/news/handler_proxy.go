package news

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ProxyImage descarga una imagen desde Instagram/Facebook CDN del lado del
// servidor y la devuelve al cliente. Esto evita los bloqueos por CORP
// (Cross-Origin Resource Policy) y tokens expirados que ocurren cuando el
// navegador intenta cargar esas URLs directamente.
//
// Si el CDN rechaza la petición (403, redirect a login, devuelve HTML),
// intenta con el endpoint público /p/{shortcode}/media/?size=l como fallback.
func (h *Handler) ProxyImage(w http.ResponseWriter, r *http.Request) {
	imageURL := r.URL.Query().Get("url")
	if imageURL == "" {
		writeError(w, http.StatusBadRequest, "Parámetro url es obligatorio")
		return
	}

	parsed, err := url.Parse(imageURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "URL inválida")
		return
	}

	host := strings.ToLower(parsed.Host)
	if !isAllowedImageProxyHost(host) {
		writeError(w, http.StatusForbidden, "Host no permitido para proxy de imágenes")
		return
	}

	// Intentar con la URL original del CDN primero.
	if h.tryServeImage(w, r, imageURL) {
		return
	}

	// Fallback: si la URL del CDN falló (403, login redirect, etc.),
	// intentar con el endpoint público /p/{shortcode}/media/?size=l.
	shortcode := extractShortcodeFromImageContext(imageURL, r.URL.Query().Get("shortcode"))
	if shortcode != "" {
		fallbackURL := fmt.Sprintf("https://www.instagram.com/p/%s/media/?size=l", url.PathEscape(shortcode))
		log.Printf("image proxy: CDN fallback para shortcode %s", shortcode)

		if h.tryServeImage(w, r, fallbackURL) {
			return
		}
	}

	writeError(w, http.StatusBadGateway, "No se pudo descargar la imagen desde Instagram")
}

// tryServeImage intenta descargar la imagen y servirla al cliente.
// Devuelve true si pudo servir la imagen correctamente; false si falló.
func (h *Handler) tryServeImage(w http.ResponseWriter, r *http.Request, imageURL string) bool {
	client := &http.Client{
		Timeout: h.imageClient.Timeout,
		// No seguir redirects automáticamente para detectar login redirects.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Si redirige a una página de login, abortar.
			if strings.Contains(req.URL.Path, "/accounts/login") ||
				strings.Contains(req.URL.Host, "facebook.com") {
				return http.ErrUseLastResponse
			}
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, imageURL, nil)
	if err != nil {
		return false
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-CO,es;q=0.9,en;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	// Verificar que la respuesta es realmente una imagen y no HTML (login page).
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/json") {
		return false
	}

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)
	return true
}

// extractShortcodeFromImageContext intenta extraer el shortcode del reel/post
// asociado a la imagen, ya sea del query param explícito o de la URL del CDN.
func extractShortcodeFromImageContext(imageURL string, explicitShortcode string) string {
	if explicitShortcode != "" && isLikelyInstagramShortcode(explicitShortcode) {
		return explicitShortcode
	}

	// Intentar extraer el shortcode de la URL de la imagen.
	// Las URLs del CDN no contienen el shortcode directamente, pero las
	// URLs del endpoint /p/{shortcode}/media/ sí.
	shortcodePattern := regexp.MustCompile(`/p/([A-Za-z0-9_-]+)/`)
	match := shortcodePattern.FindStringSubmatch(imageURL)
	if len(match) >= 2 && isLikelyInstagramShortcode(match[1]) {
		return match[1]
	}

	return ""
}

// isAllowedImageProxyHost verifica que el host sea de Instagram/Facebook para
// evitar que el proxy se use como open relay.
func isAllowedImageProxyHost(host string) bool {
	allowedSuffixes := []string{
		"cdninstagram.com",
		"fbcdn.net",
		"instagram.com",
		"facebook.com",
	}

	for _, suffix := range allowedSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}

	return false
}

// proxyListResponseImages reemplaza las URLs directas de CDN por URLs del
// proxy del backend en toda la respuesta de listado público.
func proxyListResponseImages(r *http.Request, response *ListResponse) {
	for i := range response.Data {
		proxyNewsItemImages(r, &response.Data[i])
	}
}

// proxyNewsItemImages reemplaza las URLs de imagen de un NewsItem individual.
func proxyNewsItemImages(r *http.Request, item *NewsItem) {
	if item == nil {
		return
	}

	item.ImageURL = buildProxiedImageURL(r, item.ImageURL, item.ReelURL)
	item.ThumbnailURL = buildProxiedImageURL(r, item.ThumbnailURL, item.ReelURL)
}

// proxyAdminNewsItemImages reemplaza las URLs de imagen en la lista admin.
func proxyAdminNewsItemImages(r *http.Request, items []AdminNewsItem) {
	for i := range items {
		items[i].ImageURL = buildProxiedImageURL(r, items[i].ImageURL, items[i].ReelURL)
	}
}

// buildProxiedImageURL convierte una URL directa de CDN de Instagram/Facebook
// en una URL del proxy del backend (/api/news/image?url=...&shortcode=...).
// Si la URL no es de un host permitido, la devuelve sin modificar.
func buildProxiedImageURL(r *http.Request, originalURL string, reelURL string) string {
	originalURL = strings.TrimSpace(originalURL)
	if originalURL == "" {
		return ""
	}

	parsed, err := url.Parse(originalURL)
	if err != nil {
		return originalURL
	}

	host := strings.ToLower(parsed.Host)
	if !isAllowedImageProxyHost(host) {
		return originalURL
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	// Extraer shortcode del reelURL para pasarlo como fallback hint.
	shortcode := extractShortcodeFromReelURL(reelURL)

	proxyBase := fmt.Sprintf("%s://%s/api/news/image", scheme, r.Host)
	proxyURL := fmt.Sprintf("%s?url=%s", proxyBase, url.QueryEscape(originalURL))
	if shortcode != "" {
		proxyURL += "&shortcode=" + url.QueryEscape(shortcode)
	}

	return proxyURL
}

// extractShortcodeFromReelURL extrae el shortcode de una URL de reel/post de
// Instagram (ej: https://www.instagram.com/p/DYISK3_j64-/ → DYISK3_j64-).
func extractShortcodeFromReelURL(reelURL string) string {
	reelURL = strings.TrimSpace(reelURL)
	if reelURL == "" {
		return ""
	}

	pattern := regexp.MustCompile(`(?:instagram\.com)/(?:p|reel)/([A-Za-z0-9_-]+)`)
	match := pattern.FindStringSubmatch(reelURL)
	if len(match) >= 2 && isLikelyInstagramShortcode(match[1]) {
		return match[1]
	}

	return ""
}
