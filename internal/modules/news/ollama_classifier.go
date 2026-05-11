package news

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type OllamaNewsClassifier struct {
	enabled  bool
	baseURL  string
	model    string
	client   *http.Client
	fallback NewsClassifier
}

func (c *OllamaNewsClassifier) Enabled() bool {
	return c.enabled && c.baseURL != "" && c.model != ""
}

type ollamaGenerateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Format  string         `json:"format"`
	Options map[string]any `json:"options,omitempty"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

type ollamaNewsResponse struct {
	IsNews     bool    `json:"isNews"`
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	Body       string  `json:"body"`
	Category   string  `json:"category"`
	Importance string  `json:"importance"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

func NewOllamaNewsClassifier(enabled bool, baseURL string, model string, fallback NewsClassifier) *OllamaNewsClassifier {
	return &OllamaNewsClassifier{
		enabled:  enabled,
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		model:    strings.TrimSpace(model),
		client:   &http.Client{Timeout: 75 * time.Second},
		fallback: fallback,
	}
}

func (c *OllamaNewsClassifier) Classify(ctx context.Context, reel ExtractedReel) (NewsClassification, error) {
	fallbackClassification, fallbackErr := c.fallbackClassification(ctx, reel)
	if isInvalidExtractedReel(reel) {
		return markClassificationAsRejected(fallbackClassification, "reel inválido o metadata insuficiente"), nil
	}

	if !c.enabled || c.baseURL == "" || c.model == "" {
		if fallbackErr != nil {
			return NewsClassification{}, fallbackErr
		}

		// Ollama no disponible: si el fallback dice que es noticia, publicar con fallback
		if fallbackClassification.IsNews {
			fallbackClassification.Reason = appendClassificationReason(
				fallbackClassification.Reason,
				"ollama no disponible; publicada con clasificacion local",
			)
			return fallbackClassification, nil
		}

		return markClassificationAsNeedsEditorialReview(
			fallbackClassification,
			"ollama no disponible: requiere clasificacion y redaccion editorial con IA antes de publicar",
		), nil
	}

	classification, err := c.classifyWithOllama(ctx, reel, fallbackClassification)
	if err != nil {
		if fallbackErr != nil {
			return NewsClassification{}, err
		}

		return markClassificationAsNeedsEditorialReview(
			fallbackClassification,
			fmt.Sprintf("ollama fallo; requiere redaccion editorial con IA antes de publicar: %v", err),
		), nil
	}

	return classification, nil
}

func appendClassificationReason(current string, extra string) string {
	current = strings.TrimSpace(current)
	extra = strings.TrimSpace(extra)

	if current == "" {
		return extra
	}
	if extra == "" {
		return current
	}

	return current + "; " + extra
}

func (c *OllamaNewsClassifier) ClassifyStrict(ctx context.Context, reel ExtractedReel) (NewsClassification, error) {
	fallbackClassification, fallbackErr := c.fallbackClassification(ctx, reel)
	if fallbackErr != nil {
		return NewsClassification{}, fallbackErr
	}

	if isInvalidExtractedReel(reel) {
		return markClassificationAsRejected(fallbackClassification, "reel inválido o metadata insuficiente"), nil
	}

	if !c.enabled || c.baseURL == "" || c.model == "" {
		return NewsClassification{}, fmt.Errorf("ollama no está habilitado o no está configurado")
	}

	return c.classifyWithOllama(ctx, reel, fallbackClassification)
}

func isInvalidExtractedReel(reel ExtractedReel) bool {
	if matched, _ := regexp.MatchString(`^[a-z]{2}_[A-Z]{2}$`, reel.SourceMediaID); matched {
		return true
	}

	caption := strings.TrimSpace(reel.Caption)
	if caption == "" {
		return true
	}

	if caption == "Contenido publicado en la cuenta institucional de Instagram." {
		return true
	}

	if matched, _ := regexp.MatchString(`(?i)^[0-9,.]+\s+likes?,\s+[0-9,.]+\s+comments?`, caption); matched {
		return true
	}

	return false
}

func markClassificationAsRejected(classification NewsClassification, reason string) NewsClassification {
	classification.Category = CategoryOther
	classification.Importance = ImportanceLow
	classification.Confidence = 0.10
	classification.IsNews = false
	classification.Reason = reason

	return classification
}

func markClassificationAsNeedsEditorialReview(classification NewsClassification, reason string) NewsClassification {
	classification.Category = CategoryOther
	classification.Importance = ImportanceLow
	classification.Confidence = minConfidence(classification.Confidence, 0.45)
	classification.IsNews = false
	classification.Reason = appendClassificationReason(classification.Reason, reason)

	return classification
}

func (c *OllamaNewsClassifier) fallbackClassification(ctx context.Context, reel ExtractedReel) (NewsClassification, error) {
	if c.fallback == nil {
		return NewsClassification{
			Category:   CategoryOther,
			Title:      buildTitle(reel.Caption),
			Summary:    buildSummary(reel.Caption),
			Body:       buildBody(reel.Caption, buildSummary(reel.Caption)),
			Importance: ImportanceLow,
			Confidence: 0.40,
			IsNews:     strings.TrimSpace(reel.Caption) != "",
			Reason:     "clasificación local mínima",
		}, nil
	}

	return c.fallback.Classify(ctx, reel)
}

func (c *OllamaNewsClassifier) classifyWithOllama(ctx context.Context, reel ExtractedReel, fallback NewsClassification) (NewsClassification, error) {
	const maxRetries = 2

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("ollama retry %d/%d para reel %s: %v", attempt, maxRetries, reel.SourceMediaID, lastErr)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		result, err := c.doOllamaRequest(ctx, reel, fallback)
		if err == nil {
			return result, nil
		}

		lastErr = err
	}

	return NewsClassification{}, fmt.Errorf("ollama fallo tras %d reintentos: %w", maxRetries+1, lastErr)
}

func (c *OllamaNewsClassifier) doOllamaRequest(ctx context.Context, reel ExtractedReel, fallback NewsClassification) (NewsClassification, error) {
	payload := ollamaGenerateRequest{
		Model:  c.model,
		Prompt: buildOllamaNewsPrompt(reel),
		Stream: false,
		Format: "json",
		Options: map[string]any{
			"temperature": 0.2,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return NewsClassification{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return NewsClassification{}, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return NewsClassification{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return NewsClassification{}, fmt.Errorf("ollama respondió status %d", response.StatusCode)
	}

	var generated ollamaGenerateResponse
	if err := json.NewDecoder(response.Body).Decode(&generated); err != nil {
		return NewsClassification{}, err
	}

	var parsed ollamaNewsResponse
	if err := json.Unmarshal([]byte(cleanModelJSON(generated.Response)), &parsed); err != nil {
		return NewsClassification{}, err
	}

	return normalizeOllamaNewsResponse(parsed, fallback), nil
}

func buildOllamaNewsPrompt(reel ExtractedReel) string {
	publishedAt := ""
	if reel.PublishedAt != nil {
		publishedAt = reel.PublishedAt.Format(time.RFC3339)
	}

	return fmt.Sprintf(`Actuá como editor periodístico institucional de una universidad.
Analizá la información extraída de Instagram y devolvé SOLO un JSON válido, sin markdown.

Objetivo editorial:
- Primero extrae mentalmente los datos verificables del caption: que ocurre, quien participa, cuando, donde, oferta/convocatoria, beneficios o relevancia institucional.
- Luego decide si corresponde publicar como noticia institucional.
- Si corresponde, redacta una noticia NUEVA. No maquilles el caption: conviertelo en una nota periodistica institucional.
- Genera un title original creado por vos. El title NO debe ser la primera frase del caption ni una copia del texto original.
- Redacta summary y body con tus propias palabras, usando solo datos del caption.
- Clasifica categoria e importancia.
- IDENTIDAD INSTITUCIONAL OBLIGATORIA: UNIMETA es la sigla de Corporacion Universitaria del Meta. PROHIBIDO escribir Universidad Nacional de Medellin, Universidad Nacional de Colombia, Universidad del Meta, Universidad Metropolitana del Meta o cualquier otra expansion. Si necesitas el nombre completo, escribe "Corporacion Universitaria del Meta (UNIMETA)".
Categorías válidas exactas:
["%s","%s","%s","%s","%s","%s","%s","%s","%s"]

Importancias válidas exactas:
["%s","%s","%s"]

Reglas:
- Tono editorial formal, sobrio e institucional. No uses emojis, hashtags, signos decorativos ni lenguaje publicitario.
- Cuando menciones la institucion, usa "UNIMETA" o "Corporacion Universitaria del Meta". Nunca inventes otro nombre institucional.
- Revisa ortografia, mayusculas y tildes. Evita escribir titulos completamente en mayusculas.
- isNews SOLO puede ser true si el contenido trata sobre al menos uno de estos temas:
  1. asuntos académicos: matrículas, inscripciones, clases, programas, admisiones, docentes o estudiantes;
  2. eventos institucionales: charlas, talleres, jornadas, ferias, congresos, conferencias o invitaciones con información concreta;
  3. investigación, tecnología, bienestar universitario o logros institucionales relevantes;
  4. comunicados, convocatorias, cambios importantes, reconocimientos o información de interés para la comunidad universitaria.
- isNews debe ser false SOLO si es puramente un meme irrelevante, trend musical sin contexto o contenido decorativo vacío.
- ACEPTA como noticia (isNews=true) los contenidos promocionales, invitaciones a estudiar, frases motivacionales de la universidad y publicidad institucional. Todo lo que promueva la oferta académica, la vida universitaria o invite a inscribirse DEBE ser considerado noticia.
- Si isNews es false, igual devolvé title/summary/body descriptivos breves, pero category debe ser "%s" e importance debe ser "%s".
- No inventes fechas, nombres, lugares ni datos que no esten en el texto.
- Si falta informacion, redacta de forma prudente usando solo lo disponible.
- NO copies frases completas del caption. Debes parafrasear y convertir el texto en noticia institucional.
- El body NO debe ser una transcripcion del caption: debe explicar que ocurre, quien participa, donde/cuando si aplica, y por que es relevante.
- El title debe ser original, informativo y editorial. Prohibido usar literalmente la primera linea del caption como title.
- Evita titulos promocionales como 'El momento es ahora', 'Todo empieza con una decision', 'Tu vocacion puede transformar vidas' o similares. Converti eso en informacion institucional concreta.
- title: maximo 90 caracteres, especifico, diferenciable, formal y directo; sin emojis, sin hashtags, sin signos de alarma repetidos y sin mayusculas sostenidas.
- El title NO debe ser genérico. Evitá títulos como "UNIMETA en FILBO 2026", "UNIMETA en la Feria Internacional del Libro de Bogotá", "Participación de UNIMETA en FILBO" o similares.
- Si el caption menciona libro, taller, conversatorio, lanzamiento, evento, invitación, programa, autor, expositor, stand, salón o tema central, el title DEBE incluir ese dato concreto.
- Para publicaciones de FILBO, diferenciá por actividad concreta: presentación/lanzamiento de libro, taller, conversatorio, invitación a evento, pregunta a visitantes, stand o tema académico.
- Ejemplos de title buenos:
  - "Presentación de 'Pedagogía y territorio' en FILBO 2026"
  - "Taller sobre metabolismo urbano y sostenibilidad en FILBO"
  - "UNIMETA invita a charla jurídica por sus 40 años"
  - "Visitantes comparten sus libros favoritos en el stand UNIMETA"
- Ejemplos de title malos:
  - "UNIMETA en FILBO 2026"
  - "UNIMETA participa en la Feria del Libro"
  - "Evento académico de UNIMETA"
- summary: máximo 220 caracteres.
- body: entre 1 y 3 párrafos breves.
- confidence debe ir entre 0 y 1.
- Respondé exactamente con estas claves:
{"isNews":true,"title":"","summary":"","body":"","category":"%s","importance":"%s","confidence":0.8,"reason":""}

Datos extraídos:
source: instagram
sourceMediaId: %s
reelUrl: %s
publishedAt: %s
caption:
%s`,
		CategoryAcademic,
		CategoryEvents,
		CategoryResearch,
		CategorySports,
		CategoryTechnology,
		CategoryCulture,
		CategoryWellbeing,
		CategoryInstitutional,
		CategoryOther,
		ImportanceHigh,
		ImportanceMedium,
		ImportanceLow,
		CategoryOther,
		ImportanceLow,
		CategoryInstitutional,
		ImportanceMedium,
		reel.SourceMediaID,
		reel.ReelURL,
		publishedAt,
		reel.Caption,
	)
}

func cleanModelJSON(raw string) string {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	return strings.TrimSpace(clean)
}

func improveGenericTitle(title string, fallback NewsClassification) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return title
	}

	if !isGenericInstitutionalTitle(title) {
		return title
	}

	sourceText := fallback.Body
	if strings.TrimSpace(sourceText) == "" {
		sourceText = fallback.Summary
	}
	if strings.TrimSpace(sourceText) == "" {
		return title
	}

	if specific := buildSpecificTitleFromText(sourceText); specific != "" {
		return specific
	}

	return title
}

func isGenericInstitutionalTitle(title string) bool {
	normalized := normalizeText(title)

	if strings.Contains(normalized, "unimeta") &&
		(strings.Contains(normalized, "filbo") || strings.Contains(normalized, "feria internacional del libro")) {
		specificMarkers := []string{
			"presentacion del libro",
			"lanzamiento del libro",
			"taller sobre",
			"conversatorio",
			"stand",
			"metabolismo urbano",
			"pedagogia y territorio",
			"imputacion",
			"libros favoritos",
		}

		for _, marker := range specificMarkers {
			if strings.Contains(normalized, normalizeText(marker)) {
				return false
			}
		}

		return true
	}

	genericPatterns := []string{
		"unimeta en filbo",
		"unimeta participa en filbo",
		"unimeta participa en la feria",
		"unimeta participa en feria",
		"unimeta en la feria internacional del libro",
		"unimeta en feria internacional del libro",
		"participacion de unimeta en filbo",
		"evento academico de unimeta",
		"unimeta participa en filbo 2026 con evento academico",
		"unimeta en la filbo",
	}

	for _, pattern := range genericPatterns {
		if strings.Contains(normalized, normalizeText(pattern)) {
			return true
		}
	}

	return false
}

func buildSpecificTitleFromText(text string) string {
	plainText := strings.Join(strings.Fields(cleanEditorialText(text)), " ")
	if plainText == "" {
		return ""
	}

	type titlePattern struct {
		Regex    *regexp.Regexp
		Template string
	}

	patterns := []titlePattern{
		{
			Regex:    regexp.MustCompile(`(?i)(?:lanzamiento|presentaci[oó]n)\s+del\s+libro\s+[“"']([^”"']+)[”"']`),
			Template: "Lanzamiento del libro %q en FILBO 2026",
		},
		{
			Regex:    regexp.MustCompile(`(?i)libro\s+[“"']([^”"']+)[”"']`),
			Template: "Presentación del libro %q en FILBO 2026",
		},
		{
			Regex:    regexp.MustCompile(`(?i)taller\s+(?:de|sobre)?\s*([^.,;]+)`),
			Template: "Taller sobre %s en FILBO 2026",
		},
		{
			Regex:    regexp.MustCompile(`(?i)conversatorio\s+[“"']([^”"']+)[”"']`),
			Template: "Conversatorio %q en UNIMETA",
		},
		{
			Regex:    regexp.MustCompile(`(?i)evento\s+[“"']?(UNIMETA\s+40\s+AÑOS[^”"',.;]+)`),
			Template: "%s en FILBO 2026",
		},
		{
			Regex:    regexp.MustCompile(`(?i)(imputaci[oó]n\s+(?:penal\s+)?a\s+los\s+dirigentes[^”"',.;]+)`),
			Template: "Charla jurídica sobre %s",
		},
	}

	for _, pattern := range patterns {
		match := pattern.Regex.FindStringSubmatch(plainText)
		if len(match) < 2 {
			continue
		}

		detail := strings.TrimSpace(match[1])
		detail = strings.Trim(detail, " .,:;")
		if detail == "" {
			continue
		}

		title := fmt.Sprintf(pattern.Template, detail)
		return formalizeTitle(truncateRunes(title, 90))
	}

	if strings.Contains(normalizeText(plainText), "stand") && strings.Contains(normalizeText(plainText), "libro favorito") {
		return "Visitantes comparten sus libros favoritos en el stand UNIMETA"
	}

	return ""
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}

	if max <= 3 {
		return string(runes[:max])
	}

	return string(runes[:max-3]) + "..."
}

func makeDistinctiveTitle(title string, sourceText string, previousTitles []string) string {
	title = improveGenericTitle(title, NewsClassification{
		Summary: sourceText,
		Body:    sourceText,
	})
	title = formalizeTitle(title)

	if title == "" {
		title = buildTitle(sourceText)
	}

	if !hasSimilarTitle(title, previousTitles) {
		return title
	}

	for _, candidate := range buildCreativeTitleCandidates(sourceText) {
		candidate = formalizeTitle(candidate)
		if candidate == "" {
			continue
		}

		if !hasSimilarTitle(candidate, previousTitles) {
			return candidate
		}
	}

	return formalizeTitle(title)
}

func hasSimilarTitle(title string, previousTitles []string) bool {
	for _, previous := range previousTitles {
		if areTitlesTooSimilar(title, previous) {
			return true
		}
	}

	return false
}

func areTitlesTooSimilar(left string, right string) bool {
	left = normalizeText(left)
	right = normalizeText(right)

	if left == "" || right == "" {
		return false
	}

	if left == right {
		return true
	}

	if isGenericInstitutionalTitle(left) && isGenericInstitutionalTitle(right) {
		return true
	}

	if strings.Contains(left, right) || strings.Contains(right, left) {
		shortest := len([]rune(left))
		longest := len([]rune(right))
		if shortest > longest {
			shortest, longest = longest, shortest
		}

		return float64(shortest)/float64(longest) >= 0.65
	}

	leftTokens := titleSimilarityTokens(left)
	rightTokens := titleSimilarityTokens(right)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return false
	}

	intersection := 0
	for token := range leftTokens {
		if rightTokens[token] {
			intersection++
		}
	}

	shorter := len(leftTokens)
	longer := len(rightTokens)
	if len(rightTokens) < shorter {
		shorter = len(rightTokens)
	}
	if len(leftTokens) > longer {
		longer = len(leftTokens)
	}

	if intersection == shorter && shorter <= 2 && longer-shorter >= 3 {
		return false
	}

	return float64(intersection)/float64(shorter) >= 0.72
}

func titleSimilarityTokens(normalizedTitle string) map[string]bool {
	stopwords := map[string]bool{
		"unimeta": true, "filbo": true, "feria": true, "internacional": true,
		"libro": true, "libros": true, "bogota": true, "participa": true,
		"participacion": true, "evento": true, "academico": true, "academica": true,
		"del": true, "de": true, "la": true, "el": true, "en": true, "con": true,
		"por": true, "para": true, "sobre": true, "sus": true, "los": true,
		"las": true, "un": true, "una": true, "y": true,
	}

	tokens := map[string]bool{}
	for _, token := range strings.Fields(normalizedTitle) {
		token = strings.Trim(token, " .,:;()[]{}'\"")
		if len([]rune(token)) < 4 || stopwords[token] {
			continue
		}

		tokens[token] = true
	}

	return tokens
}

func buildCreativeTitleCandidates(text string) []string {
	plainText := strings.Join(strings.Fields(cleanEditorialText(text)), " ")
	normalized := normalizeText(plainText)
	candidates := []string{}

	if quoted := firstQuotedText(plainText); quoted != "" {
		candidates = append(candidates,
			fmt.Sprintf("%s: una mirada académica desde UNIMETA", quoted),
			fmt.Sprintf("%s llega al espacio editorial de UNIMETA", quoted),
			fmt.Sprintf("El libro %q abre conversación en FILBO", quoted),
		)
	}

	switch {
	case strings.Contains(normalized, "pedagogia y territorio"):
		candidates = append(candidates,
			"Pedagogía y territorio: educación conectada con la región",
			"Janeth Vaca presenta una mirada territorial de la educación",
		)
	case strings.Contains(normalized, "metabolismo urbano") || strings.Contains(normalized, "paradojas termodinamicas"):
		candidates = append(candidates,
			"Ciudades sostenibles bajo la lupa académica de UNIMETA",
			"Metabolismo urbano: el debate ambiental que llevó UNIMETA a FILBO",
		)
	case strings.Contains(normalized, "imputacion") && strings.Contains(normalized, "dirigentes"):
		candidates = append(candidates,
			"Debate jurídico sobre estructuras criminales en FILBO",
			"UNIMETA abre conversación penal sobre dirigentes criminales",
		)
	case strings.Contains(normalized, "libro favorito") || strings.Contains(normalized, "stand"):
		candidates = append(candidates,
			"Lectores revelan sus favoritos en el stand de UNIMETA",
			"El stand de UNIMETA pregunta por los libros que dejan huella",
		)
	case strings.Contains(normalized, "turismo") && strings.Contains(normalized, "emprendimiento"):
		candidates = append(candidates,
			"Academia y empresarios dialogan sobre competitividad regional",
			"Turismo y emprendimiento se debaten con mirada regional",
		)
	}

	if specific := buildSpecificTitleFromText(plainText); specific != "" {
		candidates = append([]string{specific}, candidates...)
	}

	if headline := buildTitleFromSourceText(plainText); headline != "" {
		candidates = append(candidates, headline)
	}

	return candidates
}

func buildTitleFromSourceText(text string) string {
	text = cleanEditorialText(text)
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := regexp.MustCompile(`[.!?]`).Split(line, 2)
		title := strings.TrimSpace(parts[0])
		title = strings.Trim(title, " .,:;")
		if len([]rune(title)) < 18 {
			continue
		}

		return formalizeTitle(title)
	}

	return ""
}

func firstQuotedText(text string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`[“"]([^”"]+)[”"]`),
		regexp.MustCompile(`[']([^']+)[']`),
	}

	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) < 2 {
			continue
		}

		value := strings.TrimSpace(match[1])
		if value != "" {
			return value
		}
	}

	return ""
}

func normalizeOllamaNewsResponse(response ollamaNewsResponse, fallback NewsClassification) NewsClassification {
	title := strings.TrimSpace(response.Title)
	if title == "" {
		title = fallback.Title
	}
	title = improveGenericTitle(title, fallback)

	summary := strings.TrimSpace(response.Summary)
	if summary == "" {
		summary = fallback.Summary
	}

	body := strings.TrimSpace(response.Body)
	if body == "" {
		body = fallback.Body
	}

	category := strings.TrimSpace(response.Category)
	if !IsValidCategory(category) {
		category = fallback.Category
	}

	importance := strings.TrimSpace(response.Importance)
	if !IsValidImportance(importance) {
		importance = fallback.Importance
	}

	confidence := response.Confidence
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	if confidence == 0 {
		confidence = fallback.Confidence
	}

	reason := strings.TrimSpace(response.Reason)
	if reason == "" {
		reason = fallback.Reason
	}

	isNews := response.IsNews
	if category == CategoryOther {
		isNews = false
		importance = ImportanceLow
	}

	classification := NewsClassification{
		Category:   category,
		Title:      title,
		Summary:    summary,
		Body:       body,
		Importance: importance,
		Confidence: confidence,
		IsNews:     isNews,
		Reason:     reason,
	}

	classification = normalizeEditorialClassification(classification, fallback.Body+" "+fallback.Summary)
	if classification.IsNews && !hasEditorialRewriteQuality(classification, fallback) {
		return markClassificationAsNeedsEditorialReview(
			classification,
			"la respuesta de IA no reescribio suficientemente el caption original",
		)
	}

	return classification
}

func hasEditorialRewriteQuality(classification NewsClassification, fallback NewsClassification) bool {
	sourceText := cleanEditorialText(strings.TrimSpace(fallback.Body + " " + fallback.Summary))
	if sourceText == "" {
		return true
	}

	body := cleanEditorialText(classification.Body)
	summary := cleanEditorialText(classification.Summary)
	title := cleanEditorialText(classification.Title)

	if body == "" || summary == "" || title == "" {
		return false
	}

	if isCopiedOrPromotionalTitle(title, sourceText) {
		return false
	}

	if areTextsTooSimilarForEditorialRewrite(body, sourceText) {
		return false
	}

	if areTextsTooSimilarForEditorialRewrite(summary, sourceText) && len([]rune(summary)) > 80 {
		return false
	}

	firstSourceLine := firstMeaningfulLine(sourceText)
	if firstSourceLine != "" && areTitleTooCloseToCaptionLine(title, firstSourceLine) {
		return false
	}

	return true
}

func isCopiedOrPromotionalTitle(title string, sourceText string) bool {
	normalizedTitle := normalizeText(title)
	if normalizedTitle == "" {
		return true
	}

	promotionalTitles := []string{
		"el momento es ahora",
		"todo empieza con una decision",
		"tu vocacion puede transformar vidas",
		"convierte tu pasion por viajar en tu trabajo",
		"convierte tu pasion por el deporte en tu proyecto de vida",
	}
	for _, promotional := range promotionalTitles {
		if strings.Contains(normalizedTitle, normalizeText(promotional)) {
			return true
		}
	}

	for _, line := range strings.Split(sourceText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if areTitleTooCloseToCaptionLine(title, line) {
			return true
		}
	}

	return false
}

func areTitleTooCloseToCaptionLine(title string, line string) bool {
	title = normalizeText(title)
	line = normalizeText(line)
	if title == "" || line == "" {
		return false
	}

	if title == line {
		return true
	}

	if strings.Contains(line, title) && len([]rune(title)) >= 18 {
		return true
	}

	return areTextsTooSimilarForEditorialRewrite(title, line)
}

func areTextsTooSimilarForEditorialRewrite(left string, right string) bool {
	left = normalizeText(left)
	right = normalizeText(right)
	if left == "" || right == "" {
		return false
	}

	if left == right {
		return true
	}

	if strings.Contains(right, left) && len([]rune(left)) >= 80 {
		return true
	}

	leftTokens := editorialSimilarityTokens(left)
	rightTokens := editorialSimilarityTokens(right)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return false
	}

	intersection := 0
	for token := range leftTokens {
		if rightTokens[token] {
			intersection++
		}
	}

	shorter := len(leftTokens)
	if len(rightTokens) < shorter {
		shorter = len(rightTokens)
	}

	// Relajado de 0.82 a 0.90 para evitar rechazar noticias válidas
	// donde el LLM parafrasea pero mantiene datos concretos del caption
	return float64(intersection)/float64(shorter) >= 0.90
}

func editorialSimilarityTokens(text string) map[string]bool {
	stopwords := map[string]bool{
		"unimeta": true, "para": true, "como": true, "este": true, "esta": true,
		"estos": true, "estas": true, "desde": true, "donde": true, "cuando": true,
		"porque": true, "sobre": true, "entre": true, "tambien": true, "también": true,
		"todo": true, "toda": true, "todos": true, "todas": true, "with": true,
	}

	tokens := map[string]bool{}
	for _, token := range strings.Fields(text) {
		token = strings.Trim(token, " .,:;()[]{}'\"")
		if len([]rune(token)) < 5 || stopwords[token] {
			continue
		}

		tokens[token] = true
	}

	return tokens
}

func firstMeaningfulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}

	return ""
}
