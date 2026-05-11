package news

import (
	"context"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type InstagramHTMLReelExtractor struct {
	client   *http.Client
	maxReels int
}

func NewInstagramHTMLReelExtractor(maxReels int) *InstagramHTMLReelExtractor {
	if maxReels <= 0 {
		maxReels = 10
	}

	return &InstagramHTMLReelExtractor{
		maxReels: maxReels,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *InstagramHTMLReelExtractor) GetLatestReels(ctx context.Context, username string) ([]ExtractedReel, error) {
	username = sanitizeInstagramUsername(username)

	if username == "" {
		return nil, fmt.Errorf("INSTAGRAM_TARGET_USERNAME no esta configurado")
	}

	apiReels, err := e.getLatestReelsFromProfileAPI(ctx, username)
	if err == nil && len(apiReels) > 0 {
		log.Printf("Instagram username: %s", username)
		log.Printf("Items encontrados desde profile API: %d", len(apiReels))
	} else if err != nil {
		log.Printf("No se pudo leer profile API de Instagram para %s: %v", username, err)
	}

	cacheBuster := time.Now().UnixMilli()
	profileURLs := []string{
		fmt.Sprintf("https://www.instagram.com/%s/reels/?_t=%d", url.PathEscape(username), cacheBuster),
		fmt.Sprintf("https://www.instagram.com/%s/?_t=%d", url.PathEscape(username), cacheBuster),
	}

	htmlCandidates := []instagramMediaCandidate{}
	seenHTMLCandidates := map[string]bool{}

	for _, profileURL := range profileURLs {
		htmlBody, err := e.fetchHTML(ctx, profileURL)
		if err != nil {
			log.Printf("No se pudo leer %s: %v", profileURL, err)
			continue
		}

		normalizedHTML := normalizeInstagramHTML(htmlBody)
		foundMedia := extractInstagramMediaCandidates(normalizedHTML)

		log.Printf("URL analizada: %s", profileURL)
		log.Printf("Tamano HTML recibido: %d caracteres", len(htmlBody))
		log.Printf("Contiene /reel/: %v", strings.Contains(normalizedHTML, "/reel/"))
		log.Printf("Contiene /p/: %v", strings.Contains(normalizedHTML, "/p/"))
		log.Printf("Ocurrencias de shortcode: %d", strings.Count(normalizedHTML, "shortcode"))
		log.Printf("Ocurrencias de code: %d", strings.Count(normalizedHTML, `"code"`))
		log.Printf("Publicaciones encontradas en esta URL: %d", len(foundMedia))

		for _, candidate := range foundMedia {
			if seenHTMLCandidates[candidate.Shortcode] {
				continue
			}

			seenHTMLCandidates[candidate.Shortcode] = true
			htmlCandidates = append(htmlCandidates, candidate)
		}
	}

	log.Printf("Instagram username: %s", username)
	log.Printf("Publicaciones encontradas desde HTML: %d", len(htmlCandidates))

	htmlReels := []ExtractedReel{}
	seenHTMLReels := map[string]bool{}

	for _, candidate := range htmlCandidates {
		if seenHTMLReels[candidate.Shortcode] {
			continue
		}

		reel, err := e.fetchReelMetadata(ctx, candidate.Shortcode, candidate.URL)
		if err != nil {
			fallbackURL := alternateInstagramMediaURL(candidate.Shortcode, candidate.URL)
			if fallbackURL == "" {
				log.Printf("No se pudo leer metadata de Instagram %s: %v", candidate.URL, err)
				continue
			}

			reel, err = e.fetchReelMetadata(ctx, candidate.Shortcode, fallbackURL)
			if err != nil {
				log.Printf("No se pudo leer metadata de Instagram %s ni %s: %v", candidate.URL, fallbackURL, err)
				continue
			}
		}

		seenHTMLReels[candidate.Shortcode] = true
		htmlReels = append(htmlReels, reel)
	}

	reels := mergeExtractedInstagramReels(apiReels, htmlReels, e.maxReels)

	log.Printf("Publicaciones totales combinadas: %d", len(reels))

	if len(reels) == 0 {
		return []ExtractedReel{}, nil
	}

	return reels, nil
}
func (e *InstagramHTMLReelExtractor) fetchReelMetadata(ctx context.Context, shortcode string, reelURL string) (ExtractedReel, error) {
	htmlBody, err := e.fetchHTML(ctx, reelURL)
	if err != nil {
		return ExtractedReel{}, err
	}

	normalizedHTML := normalizeInstagramHTML(htmlBody)

	// Important: read meta tags from the raw HTML, not from normalizedHTML.
	// Instagram often encodes quotes inside og:description as &quot;. If we
	// unescape the whole document before reading the attribute, those quotes
	// become literal " characters and the attribute parser truncates the value
	// to just "N likes, N comments ...:".
	description := extractMetaContent(htmlBody, "og:description")
	title := extractMetaContent(htmlBody, "og:title")
	imageURL := firstLikelyInstagramImageURL(
		extractMetaContent(htmlBody, "og:image"),
		extractMetaContent(htmlBody, "og:image:secure_url"),
		extractMetaContent(htmlBody, "twitter:image"),
		extractMetaContent(htmlBody, "twitter:image:src"),
		extractInstagramImageURLFromHTML(normalizedHTML),
		extractInstagramImageURLFromHTML(htmlBody),
		instagramMediaImageURL(shortcode),
	)
	publishedAt := extractPublishedAtForShortcode(normalizedHTML, shortcode)
	if publishedAt == nil {
		publishedAt = extractPublishedAt(normalizedHTML)
	}

	caption := extractInstagramCaptionFromHTML(htmlBody)

	if caption == "" {
		caption = extractInstagramCaptionFromHTML(normalizedHTML)
	}

	if caption == "" {
		caption = extractInstagramCaption(description)
	}

	if caption == "" {
		caption = extractInstagramCaption(title)
	}

	if caption == "" {
		caption = strings.TrimSpace(description)
	}

	if caption == "" {
		caption = strings.TrimSpace(title)
	}

	return ExtractedReel{
		SourceMediaID: shortcode,
		ReelURL:       reelURL,
		ThumbnailURL:  imageURL,
		Caption:       caption,
		PublishedAt:   publishedAt,
	}, nil
}

func (e *InstagramHTMLReelExtractor) getLatestReelsFromProfileAPI(ctx context.Context, username string) ([]ExtractedReel, error) {
	apiURL := fmt.Sprintf("https://www.instagram.com/api/v1/users/web_profile_info/?username=%s&_t=%d", url.QueryEscape(username), time.Now().UnixMilli())

	body, err := e.fetchInstagramURL(ctx, apiURL, "application/json,text/plain,*/*")
	if err != nil {
		return nil, err
	}

	return extractReelsFromProfileAPIJSON(body, e.maxReels)
}

type instagramProfileAPIResponse struct {
	Data struct {
		User struct {
			TimelineMedia instagramProfileMediaConnection `json:"edge_owner_to_timeline_media"`
			ReelsMedia    instagramProfileMediaConnection `json:"edge_felix_video_timeline"`
		} `json:"user"`
	} `json:"data"`
}

type instagramProfileMediaConnection struct {
	Edges []instagramProfileMediaEdge `json:"edges"`
}

type instagramProfileMediaEdge struct {
	Node instagramProfileMediaNode `json:"node"`
}

type instagramProfileMediaNode struct {
	TypeName       string                        `json:"__typename"`
	Shortcode      string                        `json:"shortcode"`
	IsVideo        bool                          `json:"is_video"`
	TakenAt        int64                         `json:"taken_at_timestamp"`
	ThumbnailSrc   string                        `json:"thumbnail_src"`
	ThumbnailURL   string                        `json:"thumbnail_url"`
	DisplayURL     string                        `json:"display_url"`
	DisplayItems   []instagramProfileImageItem   `json:"display_resources"`
	ThumbnailItems []instagramProfileImageItem   `json:"thumbnail_resources"`
	ImageVersions  instagramProfileImageVersions `json:"image_versions2"`
	Caption        instagramProfileCaption       `json:"caption"`
	LegacyCaptions instagramLegacyCaptionEdges   `json:"edge_media_to_caption"`
}

type instagramProfileImageVersions struct {
	Candidates []instagramProfileImageItem `json:"candidates"`
}

type instagramProfileImageItem struct {
	Src    string `json:"src"`
	URL    string `json:"url"`
	Width  int    `json:"config_width"`
	Height int    `json:"config_height"`
}

type instagramProfileCaption struct {
	Text string `json:"text"`
}

type instagramLegacyCaptionEdges struct {
	Edges []struct {
		Node struct {
			Text string `json:"text"`
		} `json:"node"`
	} `json:"edges"`
}

func extractReelsFromProfileAPIJSON(raw string, limit int) ([]ExtractedReel, error) {
	var response instagramProfileAPIResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 10
	}

	nodes := []instagramProfileMediaNode{}
	seen := map[string]bool{}

	appendNodes := func(edges []instagramProfileMediaEdge) {
		for _, edge := range edges {
			node := edge.Node
			shortcode := strings.TrimSpace(node.Shortcode)
			if !isLikelyInstagramShortcode(shortcode) || seen[shortcode] {
				continue
			}

			seen[shortcode] = true
			nodes = append(nodes, node)
		}
	}

	appendNodes(response.Data.User.ReelsMedia.Edges)
	appendNodes(response.Data.User.TimelineMedia.Edges)

	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].TakenAt > nodes[j].TakenAt
	})

	items := []ExtractedReel{}
	for _, node := range nodes {
		if len(items) >= limit {
			break
		}

		caption := cleanInstagramCaptionCandidate(node.Caption.Text)
		if caption == "" && len(node.LegacyCaptions.Edges) > 0 {
			caption = cleanInstagramCaptionCandidate(node.LegacyCaptions.Edges[0].Node.Text)
		}
		if caption == "" || isInstagramMetadataOnlyCaption(caption) {
			continue
		}

		thumbnailURL := bestInstagramProfileThumbnailURL(node)

		var publishedAt *time.Time
		if node.TakenAt > 0 {
			parsed := time.Unix(node.TakenAt, 0).UTC()
			publishedAt = &parsed
		}

		items = append(items, ExtractedReel{
			SourceMediaID: strings.TrimSpace(node.Shortcode),
			ReelURL:       instagramMediaURL(node.TypeName, node.IsVideo, strings.TrimSpace(node.Shortcode)),
			ThumbnailURL:  thumbnailURL,
			Caption:       caption,
			PublishedAt:   publishedAt,
		})
	}

	return items, nil
}

func bestInstagramProfileThumbnailURL(node instagramProfileMediaNode) string {
	candidates := []string{
		node.ThumbnailSrc,
		node.ThumbnailURL,
		node.DisplayURL,
	}

	candidates = append(candidates, bestInstagramProfileImageItemURL(node.DisplayItems))
	candidates = append(candidates, bestInstagramProfileImageItemURL(node.ThumbnailItems))
	candidates = append(candidates, bestInstagramProfileImageItemURL(node.ImageVersions.Candidates))
	candidates = append(candidates, instagramMediaImageURL(strings.TrimSpace(node.Shortcode)))

	for _, candidate := range candidates {
		candidate = cleanInstagramURLCandidate(candidate)
		if candidate == "" {
			continue
		}

		if isLikelyInstagramImageURL(candidate) || isInstagramMediaImageEndpoint(candidate) {
			return candidate
		}
	}

	return ""
}

func bestInstagramProfileImageItemURL(items []instagramProfileImageItem) string {
	if len(items) == 0 {
		return ""
	}

	bestURL := ""
	bestArea := -1
	for _, item := range items {
		candidate := firstNonEmptyString(item.Src, item.URL)
		if candidate == "" {
			continue
		}

		area := item.Width * item.Height
		if bestURL == "" || area > bestArea {
			bestURL = candidate
			bestArea = area
		}
	}

	return bestURL
}

func instagramMediaURL(typeName string, isVideo bool, shortcode string) string {
	if isVideo || typeName == "GraphVideo" {
		return fmt.Sprintf("https://www.instagram.com/reel/%s/", shortcode)
	}

	return fmt.Sprintf("https://www.instagram.com/p/%s/", shortcode)
}

func instagramMediaImageURL(shortcode string) string {
	shortcode = strings.TrimSpace(shortcode)
	if !isLikelyInstagramShortcode(shortcode) {
		return ""
	}

	return fmt.Sprintf("https://www.instagram.com/p/%s/media/?size=l", url.PathEscape(shortcode))
}

func alternateInstagramMediaURL(shortcode string, mediaURL string) string {
	switch {
	case strings.Contains(mediaURL, "/reel/"):
		return fmt.Sprintf("https://www.instagram.com/p/%s/", shortcode)
	case strings.Contains(mediaURL, "/p/"):
		return fmt.Sprintf("https://www.instagram.com/reel/%s/", shortcode)
	default:
		return ""
	}
}

type instagramMediaCandidate struct {
	Shortcode string
	URL       string
}

func limitExtractedReels(reels []ExtractedReel, limit int) []ExtractedReel {
	if limit <= 0 || len(reels) <= limit {
		return reels
	}

	return reels[:limit]
}

func mergeExtractedInstagramReels(apiReels []ExtractedReel, htmlReels []ExtractedReel, limit int) []ExtractedReel {
	reels := []ExtractedReel{}
	indexBySourceMediaID := map[string]int{}

	appendReel := func(reel ExtractedReel) {
		sourceMediaID := strings.TrimSpace(reel.SourceMediaID)
		if sourceMediaID == "" {
			return
		}

		reel.SourceMediaID = sourceMediaID
		if existingIndex, exists := indexBySourceMediaID[sourceMediaID]; exists {
			fillMissingReelFields(&reels[existingIndex], reel)
			return
		}

		indexBySourceMediaID[sourceMediaID] = len(reels)
		reels = append(reels, reel)
	}

	// La API de perfil trae taken_at_timestamp/display_url/thumbnail_src del
	// nodo real. El HTML de la página puede incluir JSON de posts relacionados
	// y devolver fecha o imagen equivocada para el mismo shortcode.
	for _, reel := range apiReels {
		appendReel(reel)
	}

	for _, reel := range htmlReels {
		appendReel(reel)
	}

	return limitExtractedReels(reels, limit)
}

func fillMissingReelFields(target *ExtractedReel, fallback ExtractedReel) {
	if target == nil {
		return
	}

	if strings.TrimSpace(target.ReelURL) == "" {
		target.ReelURL = strings.TrimSpace(fallback.ReelURL)
	}

	if strings.TrimSpace(target.ThumbnailURL) == "" {
		target.ThumbnailURL = strings.TrimSpace(fallback.ThumbnailURL)
	}

	if strings.TrimSpace(target.Caption) == "" {
		target.Caption = strings.TrimSpace(fallback.Caption)
	}

	// Solo rellenar PublishedAt si el target no tiene fecha.
	// La fecha del API (target) es la fuente autoritativa; la fecha del
	// HTML (fallback) puede provenir de posts relacionados embebidos en
	// la misma página, así que nunca debe sobreescribir la del API.
	if target.PublishedAt == nil && fallback.PublishedAt != nil {
		target.PublishedAt = fallback.PublishedAt
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}

func firstLikelyInstagramImageURL(values ...string) string {
	for _, value := range values {
		value = cleanInstagramURLCandidate(value)
		if value == "" {
			continue
		}

		if isLikelyInstagramImageURL(value) || isInstagramMediaImageEndpoint(value) {
			return value
		}
	}

	return ""
}
func (e *InstagramHTMLReelExtractor) fetchHTML(ctx context.Context, pageURL string) (string, error) {
	return e.fetchInstagramURL(ctx, pageURL, "text/html,application/xhtml+xml")
}

func (e *InstagramHTMLReelExtractor) fetchInstagramURL(ctx context.Context, pageURL string, accept string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "es-CO,es;q=0.9,en;q=0.8")
	req.Header.Set("X-IG-App-ID", "936619743392459")

	res, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("Instagram respondiÃ³ con estado HTTP %d para %s", res.StatusCode, pageURL)
	}

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return string(bodyBytes), nil
}

func sanitizeInstagramUsername(username string) string {
	username = strings.TrimSpace(username)
	username = strings.TrimPrefix(username, "@")
	username = strings.TrimSuffix(username, "/")

	if strings.Contains(username, "instagram.com") {
		parts := strings.Split(username, "instagram.com/")
		if len(parts) > 1 {
			username = parts[1]
		}

		username = strings.Trim(username, "/")
		username = strings.Split(username, "/")[0]
	}

	return username
}

func normalizeInstagramHTML(htmlBody string) string {
	htmlBody = stdhtml.UnescapeString(htmlBody)

	replacer := strings.NewReplacer(
		`\/`, `/`,
		`\u002F`, `/`,
		`\u003D`, `=`,
		`\u0026`, `&`,
		`\u003C`, `<`,
		`\u003E`, `>`,
		`&amp;`, `&`,
	)

	return replacer.Replace(htmlBody)
}

func extractInstagramMediaURLs(htmlBody string) map[string]string {
	mediaURLs := map[string]string{}
	for _, candidate := range extractInstagramMediaCandidates(htmlBody) {
		if mediaURLs[candidate.Shortcode] == "" {
			mediaURLs[candidate.Shortcode] = candidate.URL
		}
	}

	return mediaURLs
}

func extractInstagramMediaCandidates(htmlBody string) []instagramMediaCandidate {
	urlPatterns := []*regexp.Regexp{
		// Enlaces directos normales para Reels y publicaciones del feed.
		regexp.MustCompile(`https://www\.instagram\.com/(?:reel|p)/([A-Za-z0-9_-]+)/?`),
		regexp.MustCompile(`https://instagram\.com/(?:reel|p)/([A-Za-z0-9_-]+)/?`),
		regexp.MustCompile(`/(?:reel|p)/([A-Za-z0-9_-]+)/?`),

		// JSON normal embebido.
		regexp.MustCompile(`"url"\s*:\s*"https://www\.instagram\.com/(?:reel|p)/([A-Za-z0-9_-]+)/?"`),
		regexp.MustCompile(`"permalink"\s*:\s*"https://www\.instagram\.com/(?:reel|p)/([A-Za-z0-9_-]+)/?"`),
	}

	shortcodePatterns := []*regexp.Regexp{
		regexp.MustCompile(`"shortcode"\s*:\s*"([A-Za-z0-9_-]+)"`),
		regexp.MustCompile(`"code"\s*:\s*"([A-Za-z0-9_-]+)"`),

		// JSON escapado dentro de scripts.
		regexp.MustCompile(`\\"shortcode\\"\s*:\s*\\"([A-Za-z0-9_-]+)\\"`),
		regexp.MustCompile(`\\"code\\"\s*:\s*\\"([A-Za-z0-9_-]+)\\"`),

		// Algunos payloads usan nombres alternos.
		regexp.MustCompile(`"media_shortcode"\s*:\s*"([A-Za-z0-9_-]+)"`),
		regexp.MustCompile(`\\"media_shortcode\\"\s*:\s*\\"([A-Za-z0-9_-]+)\\"`),
	}

	seen := map[string]bool{}
	candidates := []instagramMediaCandidate{}

	appendCandidate := func(shortcode string, mediaURL string) {
		shortcode = strings.TrimSpace(shortcode)
		if !isLikelyInstagramShortcode(shortcode) || seen[shortcode] {
			return
		}

		seen[shortcode] = true
		candidates = append(candidates, instagramMediaCandidate{
			Shortcode: shortcode,
			URL:       mediaURL,
		})
	}

	for _, pattern := range urlPatterns {
		matches := pattern.FindAllStringSubmatch(htmlBody, -1)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			shortcode := strings.TrimSpace(match[1])
			appendCandidate(shortcode, instagramMediaURLFromMatchedPath(match[0], shortcode))
		}
	}

	for _, pattern := range shortcodePatterns {
		matches := pattern.FindAllStringSubmatch(htmlBody, -1)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			shortcode := strings.TrimSpace(match[1])
			appendCandidate(shortcode, fmt.Sprintf("https://www.instagram.com/reel/%s/", shortcode))
		}
	}

	return candidates
}
func instagramMediaURLFromMatchedPath(matched string, shortcode string) string {
	if strings.Contains(matched, "/p/") {
		return fmt.Sprintf("https://www.instagram.com/p/%s/", shortcode)
	}

	return fmt.Sprintf("https://www.instagram.com/reel/%s/", shortcode)
}
func isLikelyInstagramShortcode(value string) bool {
	if value == "" {
		return false
	}

	// Los shortcodes de Instagram suelen ser cadenas cortas.
	// Dejamos un rango amplio para no excluir cÃ³digos vÃ¡lidos.
	if len(value) < 5 || len(value) > 30 {
		return false
	}

	for _, char := range value {
		isLetter := char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z'
		isNumber := char >= '0' && char <= '9'
		isAllowedSymbol := char == '_' || char == '-'

		if !isLetter && !isNumber && !isAllowedSymbol {
			return false
		}
	}

	// Evita falsos positivos muy comunes.
	if matched, _ := regexp.MatchString(`^[a-z]{2}_[A-Z]{2}$`, value); matched {
		return false
	}

	invalidValues := map[string]bool{
		"Polaris":   true,
		"Instagram": true,
		"Pagelet":   true,
		"profile":   true,
		"reels":     true,
		"static":    true,
		"es_LA":     true,
		"en_US":     true,
		"pt_BR":     true,
	}

	if invalidValues[value] {
		return false
	}

	return true
}

func extractMetaContent(htmlBody string, propertyName string) string {
	escapedProperty := regexp.QuoteMeta(propertyName)

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<meta[^>]+(?:property|name)=["']` + escapedProperty + `["'][^>]+content=["']([^"']*)["'][^>]*>`),
		regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']*)["'][^>]+(?:property|name)=["']` + escapedProperty + `["'][^>]*>`),
	}

	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(htmlBody)

		if len(match) >= 2 {
			return stdhtml.UnescapeString(strings.TrimSpace(match[1]))
		}
	}

	return ""
}

func extractInstagramCaption(value string) string {
	value = strings.TrimSpace(stdhtml.UnescapeString(value))
	if value == "" {
		return ""
	}

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)^\s*[\d,.]+\s+likes?,\s+[\d,.]+\s+comments?\s+-\s+[^:]+:\s*"(.+)"\s*$`),
		regexp.MustCompile(`(?is)^\s*[\d,.]+\s+me gusta,\s+[\d,.]+\s+comentarios?\s+-\s+[^:]+:\s*"(.+)"\s*$`),
		regexp.MustCompile(`(?is)^\s*[\d,.]+\s+likes?,\s+[\d,.]+\s+comments?\s+-\s+[^:]+:\s*(.+)\s*$`),
		regexp.MustCompile(`(?is)^\s*[\d,.]+\s+me gusta,\s+[\d,.]+\s+comentarios?\s+-\s+[^:]+:\s*(.+)\s*$`),
	}

	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(value)
		if len(match) < 2 {
			continue
		}

		caption := strings.TrimSpace(match[1])
		caption = strings.Trim(caption, `"`)
		caption = strings.TrimSpace(caption)
		if caption != "" {
			return caption
		}
	}

	return ""
}

func extractInstagramCaptionFromHTML(htmlBody string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)"edge_media_to_caption"\s*:\s*\{\s*"edges"\s*:\s*\[\s*\{\s*"node"\s*:\s*\{\s*"text"\s*:\s*"((?:\\.|[^"\\])*)"`),
		regexp.MustCompile(`(?is)"caption"\s*:\s*\{[^{}]*"text"\s*:\s*"((?:\\.|[^"\\])*)"`),
		regexp.MustCompile(`(?is)"caption_text"\s*:\s*"((?:\\.|[^"\\])*)"`),
		regexp.MustCompile(`(?is)"accessibility_caption"\s*:\s*"((?:\\.|[^"\\])*)"`),
		regexp.MustCompile(`(?is)\\"edge_media_to_caption\\"\s*:\s*\{\s*\\"edges\\"\s*:\s*\[\s*\{\s*\\"node\\"\s*:\s*\{\s*\\"text\\"\s*:\s*\\"((?:\\\\.|[^\\"\\\\])*)\\"`),
		regexp.MustCompile(`(?is)\\"caption\\"\s*:\s*\{[^\{\}]*\\"text\\"\s*:\s*\\"((?:\\\\.|[^\\"\\\\])*)\\"`),
		regexp.MustCompile(`(?is)\\"caption_text\\"\s*:\s*\\"((?:\\\\.|[^\\"\\\\])*)\\"`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(htmlBody, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			caption := cleanInstagramCaptionCandidate(match[1])
			if caption == "" || isInstagramMetadataOnlyCaption(caption) {
				continue
			}

			return caption
		}
	}

	return ""
}

func cleanInstagramCaptionCandidate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	value = strings.ReplaceAll(value, `\\n`, `\n`)
	value = strings.ReplaceAll(value, `\\u`, `\u`)

	if unquoted, err := strconv.Unquote(`"` + value + `"`); err == nil {
		value = unquoted
	}

	value = stdhtml.UnescapeString(value)
	value = strings.ReplaceAll(value, `\n`, "\n")
	value = strings.ReplaceAll(value, `\"`, `"`)
	value = strings.ReplaceAll(value, `\/`, `/`)

	lines := []string{}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

func isInstagramMetadataOnlyCaption(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}

	if matched, _ := regexp.MatchString(`(?i)^[\d,.]+\s+likes?,\s+[\d,.]+\s+comments?\s*-`, value); matched {
		return true
	}

	if matched, _ := regexp.MatchString(`(?i)^[\d,.]+\s+me gusta,\s+[\d,.]+\s+comentarios?\s*-`, value); matched {
		return true
	}

	return false
}

func extractPublishedAt(htmlBody string) *time.Time {
	candidates := []string{
		// En Instagram el timestamp numérico del media es más confiable que
		// datePublished/uploadDate: esas claves también pueden aparecer en JSON
		// de posts relacionados embebidos en la misma página.
		extractJSONDateValue(htmlBody, "taken_at_timestamp"),
		extractMetaContent(htmlBody, "article:published_time"),
		extractMetaContent(htmlBody, "video:release_date"),
		extractMetaContent(htmlBody, "datePublished"),
		extractJSONDateValue(htmlBody, "uploadDate"),
		extractJSONDateValue(htmlBody, "datePublished"),
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}

		if parsed, ok := parseInstagramDate(candidate); ok {
			return &parsed
		}
	}

	return nil
}

func extractPublishedAtForShortcode(htmlBody string, shortcode string) *time.Time {
	shortcode = strings.TrimSpace(shortcode)
	if shortcode == "" {
		return nil
	}

	indexes := shortcodeIndexes(htmlBody, shortcode)
	for _, index := range indexes {
		start := index - 5000
		if start < 0 {
			start = 0
		}

		end := index + len(shortcode) + 5000
		if end > len(htmlBody) {
			end = len(htmlBody)
		}

		window := htmlBody[start:end]
		for _, candidate := range []string{
			extractJSONDateValue(window, "taken_at_timestamp"),
			extractJSONDateValue(window, "taken_at"),
			extractJSONDateValue(window, "created_time"),
			extractJSONDateValue(window, "datePublished"),
			extractJSONDateValue(window, "uploadDate"),
		} {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}

			if parsed, ok := parseInstagramDate(candidate); ok {
				return &parsed
			}
		}
	}

	return nil
}

func shortcodeIndexes(value string, shortcode string) []int {
	indexes := []int{}
	offset := 0

	for {
		index := strings.Index(value[offset:], shortcode)
		if index < 0 {
			return indexes
		}

		absoluteIndex := offset + index
		indexes = append(indexes, absoluteIndex)
		offset = absoluteIndex + len(shortcode)
	}
}

func extractInstagramImageURLFromHTML(htmlBody string) string {
	return firstLikelyInstagramImageURL(
		extractJSONImageValue(htmlBody, "thumbnail_url"),
		extractJSONImageValue(htmlBody, "thumbnailUrl"),
		extractJSONImageValue(htmlBody, "thumbnail_src"),
		extractJSONImageValue(htmlBody, "display_url"),
		extractJSONImageValue(htmlBody, "contentUrl"),
		extractResourceImageURL(htmlBody, "display_resources"),
		extractResourceImageURL(htmlBody, "thumbnail_resources"),
		extractImageVersionsCandidateURL(htmlBody),
	)
}

func extractJSONImageValue(htmlBody string, key string) string {
	value := extractJSONDateValue(htmlBody, key)
	if !isLikelyInstagramImageURL(value) {
		return ""
	}

	return value
}

func extractResourceImageURL(htmlBody string, key string) string {
	escapedKey := regexp.QuoteMeta(key)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)"` + escapedKey + `"\s*:\s*\[.*?"src"\s*:\s*"([^"]+)"`),
		regexp.MustCompile(`(?is)\\"` + escapedKey + `\\"\s*:\s*\[.*?\\"src\\"\s*:\s*\\"([^\\"]+)\\"`),
	}

	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(htmlBody)
		if len(match) < 2 {
			continue
		}

		value := cleanInstagramURLCandidate(match[1])
		if isLikelyInstagramImageURL(value) {
			return value
		}
	}

	return ""
}

func extractImageVersionsCandidateURL(htmlBody string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)"image_versions2"\s*:\s*\{.*?"url"\s*:\s*"([^"]+)"`),
		regexp.MustCompile(`(?is)\\"image_versions2\\"\s*:\s*\{.*?\\"url\\"\s*:\s*\\"([^\\"]+)\\"`),
	}

	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(htmlBody)
		if len(match) < 2 {
			continue
		}

		value := cleanInstagramURLCandidate(match[1])
		if isLikelyInstagramImageURL(value) {
			return value
		}
	}

	return ""
}

func extractJSONDateValue(htmlBody string, key string) string {
	escapedKey := regexp.QuoteMeta(key)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`"` + escapedKey + `"\s*:\s*"([^"]+)"`),
		regexp.MustCompile(`"` + escapedKey + `"\s*:\s*\[\s*"([^"]+)"`),
		regexp.MustCompile(`"` + escapedKey + `"\s*:\s*([0-9]+)`),
		regexp.MustCompile(`\\"` + escapedKey + `\\"\s*:\s*\\"([^\\"]+)\\"`),
		regexp.MustCompile(`\\"` + escapedKey + `\\"\s*:\s*\[\s*\\"([^\\"]+)\\"`),
	}

	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(htmlBody)
		if len(match) >= 2 {
			return cleanInstagramURLCandidate(match[1])
		}
	}

	return ""
}

func parseInstagramDate(value string) (time.Time, bool) {
	if unixSeconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if unixSeconds > 9999999999 {
			unixSeconds = unixSeconds / 1000
		}

		return time.Unix(unixSeconds, 0).UTC(), true
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
	}

	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), true
		}
	}

	return time.Time{}, false
}

func cleanInstagramURLCandidate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	value = stdhtml.UnescapeString(value)
	value = strings.ReplaceAll(value, `\/`, `/`)
	value = strings.ReplaceAll(value, `\u0026`, `&`)
	value = strings.ReplaceAll(value, `\\u0026`, `&`)
	value = strings.ReplaceAll(value, `\u003D`, `=`)
	value = strings.ReplaceAll(value, `\\u003D`, `=`)

	if unquoted, err := strconv.Unquote(`"` + strings.ReplaceAll(value, `"`, `\"`) + `"`); err == nil {
		value = unquoted
	}

	return strings.TrimSpace(value)
}

func isInstagramMediaImageEndpoint(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}

	host := strings.ToLower(parsed.Host)
	path := strings.ToLower(strings.TrimRight(parsed.Path, "/"))

	return (host == "instagram.com" || host == "www.instagram.com") &&
		strings.Contains(path, "/p/") &&
		strings.HasSuffix(path, "/media")
}

func isLikelyInstagramImageURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}

	host := strings.ToLower(parsed.Host)
	path := strings.ToLower(parsed.Path)

	if strings.Contains(host, "cdninstagram.com") || strings.Contains(host, "fbcdn.net") {
		return true
	}

	if isInstagramMediaImageEndpoint(value) {
		return true
	}

	return strings.HasSuffix(path, ".jpg") ||
		strings.HasSuffix(path, ".jpeg") ||
		strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".webp")
}
