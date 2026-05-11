package news

import (
	"context"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type NewsClassifier interface {
	Classify(ctx context.Context, reel ExtractedReel) (NewsClassification, error)
}

type SimpleNewsClassifier struct{}

const officialUNIMETAName = "Corporaci\u00F3n Universitaria del Meta"

func NewSimpleNewsClassifier() *SimpleNewsClassifier {
	return &SimpleNewsClassifier{}
}

type categoryRule struct {
	Category   string
	Importance string
	Keywords   []string
}

var categoryRules = []categoryRule{
	{
		Category:   CategoryAcademic,
		Importance: ImportanceHigh,
		Keywords: []string{
			"academico",
			"académico",
			"clase",
			"clases",
			"inscripcion",
			"inscripción",
			"inscripciones",
			"matricula",
			"matrícula",
			"matriculas",
			"matrículas",
			"admisiones",
			"convocatoria",
			"programa academico",
			"programa académico",
			"estudiantes",
			"docentes",
			"semestre",
			"pregrado",
			"posgrado",
		},
	},
	{
		Category:   CategoryEvents,
		Importance: ImportanceHigh,
		Keywords: []string{
			"evento",
			"eventos",
			"jornada",
			"feria",
			"encuentro",
			"congreso",
			"seminario",
			"conferencia",
			"foro",
			"charla",
			"taller",
			"participa",
			"te esperamos",
			"invitacion",
			"invitación",
		},
	},
	{
		Category:   CategoryResearch,
		Importance: ImportanceMedium,
		Keywords: []string{
			"investigacion",
			"investigación",
			"semillero",
			"semilleros",
			"proyecto de investigacion",
			"proyecto de investigación",
			"grupo de investigacion",
			"grupo de investigación",
			"publicacion",
			"publicación",
			"ponencia",
			"articulo",
			"artículo",
		},
	},
	{
		Category:   CategorySports,
		Importance: ImportanceMedium,
		Keywords: []string{
			"deporte",
			"deportes",
			"torneo",
			"campeonato",
			"partido",
			"seleccion",
			"selección",
			"futbol",
			"fútbol",
			"baloncesto",
			"voleibol",
			"competencia",
		},
	},
	{
		Category:   CategoryTechnology,
		Importance: ImportanceMedium,
		Keywords: []string{
			"tecnologia",
			"tecnología",
			"software",
			"programacion",
			"programación",
			"inteligencia artificial",
			"ia",
			"sistemas",
			"innovacion",
			"innovación",
			"desarrollo",
			"aplicacion",
			"aplicación",
			"plataforma",
			"digital",
		},
	},
	{
		Category:   CategoryCulture,
		Importance: ImportanceMedium,
		Keywords: []string{
			"cultura",
			"cultural",
			"danza",
			"musica",
			"música",
			"arte",
			"teatro",
			"literatura",
			"presentacion cultural",
			"presentación cultural",
		},
	},
	{
		Category:   CategoryWellbeing,
		Importance: ImportanceMedium,
		Keywords: []string{
			"bienestar",
			"salud",
			"acompañamiento",
			"acompanamiento",
			"psicologia",
			"psicología",
			"actividad fisica",
			"actividad física",
			"campaña",
			"prevencion",
			"prevención",
		},
	},
	{
		Category:   CategoryInstitutional,
		Importance: ImportanceMedium,
		Keywords: []string{
			"institucional",
			"universidad",
			"corporacion universitaria",
			"corporación universitaria",
			"unimeta",
			"cbtic",
			"rectoria",
			"rectoría",
			"vicerrectoria",
			"vicerrectoría",
			"facultad",
			"escuela",
			"comunidad universitaria",
		},
	},
}

func (c *SimpleNewsClassifier) Classify(ctx context.Context, reel ExtractedReel) (NewsClassification, error) {
	text := normalizeText(reel.Caption)

	bestCategory := CategoryOther
	bestImportance := ImportanceLow
	bestScore := 0

	for _, rule := range categoryRules {
		score := calculateScore(text, rule.Keywords)

		if score > bestScore {
			bestScore = score
			bestCategory = rule.Category
			bestImportance = rule.Importance
		}
	}

	confidence := calculateConfidence(bestScore)

	if bestScore == 0 {
		bestCategory = CategoryOther
		bestImportance = ImportanceLow
		confidence = 0.40
	}

	title := buildTitle(reel.Caption)
	summary := buildSummary(reel.Caption)

	classification := NewsClassification{
		Category:   bestCategory,
		Title:      title,
		Summary:    summary,
		Body:       buildBody(reel.Caption, summary),
		Importance: bestImportance,
		Confidence: confidence,
		IsNews:     isNewsworthy(bestScore, reel.Caption),
		Reason:     buildNewsworthinessReason(bestScore, reel.Caption),
	}

	return normalizeEditorialClassification(classification, reel.Caption), nil
}

func isNewsworthy(score int, caption string) bool {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return false
	}

	text := normalizeText(caption)
	if isPureAdvertisingText(text) {
		return false
	}

	if hasAdmissionsNewsValue(text) || hasEventNewsValue(text) || hasInstitutionalNewsValue(text) {
		return true
	}

	if score >= 3 {
		return true
	}

	for _, marker := range newsIntentMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}

	return false
}

func buildNewsworthinessReason(score int, caption string) string {
	if strings.TrimSpace(caption) == "" {
		return "caption vacio"
	}

	text := normalizeText(caption)
	if isPureAdvertisingText(text) {
		return "contenido principalmente publicitario sin valor noticioso suficiente"
	}

	if hasAdmissionsNewsValue(text) {
		return "contiene informacion academica verificable sobre inscripciones u oferta educativa"
	}

	if hasEventNewsValue(text) {
		return "contiene informacion concreta de evento, fecha, lugar o actividad institucional"
	}

	if hasInstitutionalNewsValue(text) {
		return "contiene informacion institucional relevante para la comunidad universitaria"
	}

	if score >= 3 {
		return "coincide con suficientes palabras clave institucionales"
	}

	for _, marker := range newsIntentMarkers {
		if strings.Contains(text, marker) {
			return "contiene marcador de intencion noticiosa"
		}
	}

	return "no alcanza senales minimas para considerarse noticia"
}
func calculateScore(text string, keywords []string) int {
	score := 0
	words := map[string]bool{}
	for _, word := range strings.Fields(text) {
		words[word] = true
	}

	for _, keyword := range keywords {
		normalizedKeyword := normalizeText(keyword)

		if strings.Contains(normalizedKeyword, " ") {
			if strings.Contains(text, normalizedKeyword) {
				score += 3
			}
			continue
		}

		if words[normalizedKeyword] {
			score += 1
		}
	}

	return score
}

func calculateConfidence(score int) float64 {
	switch {
	case score >= 5:
		return 0.90
	case score >= 3:
		return 0.82
	case score >= 2:
		return 0.76
	case score == 1:
		return 0.68
	default:
		return 0.40
	}
}

var newsIntentMarkers = []string{
	"inscripciones abiertas",
	"convocatoria",
	"comunicado",
	"participa",
	"te invitamos",
	"te esperamos",
	"nuevo programa",
	"proceso de admision",
	"proceso de admisión",
	"matriculas abiertas",
	"matrículas abiertas",
	"evento",
	"jornada",
	"taller",
	"conferencia",
}

func hasAdmissionsNewsValue(text string) bool {
	admissionMarkers := []string{
		"inscripciones abiertas",
		"matriculas abiertas",
		"matrículas abiertas",
		"admision",
		"admisión",
	}

	hasAdmission := false
	for _, marker := range admissionMarkers {
		if strings.Contains(text, normalizeText(marker)) {
			hasAdmission = true
			break
		}
	}
	if !hasAdmission {
		return false
	}

	concreteMarkers := []string{
		"2026", "semestre", "programa", "programas", "pregrado", "posgrado",
		"tecnico", "técnico", "especializacion", "especialización", "beca",
		"becas", "financiacion", "financiación", "fecha", "plazo",
	}
	for _, marker := range concreteMarkers {
		if strings.Contains(text, normalizeText(marker)) {
			return true
		}
	}

	return false
}

func hasEventNewsValue(text string) bool {
	eventMarkers := []string{
		"evento", "jornada", "charla", "taller", "conferencia", "congreso",
		"seminario", "foro", "feria", "encuentro", "precongreso", "capacitacion",
		"capacitación", "lanzamiento", "presentacion", "presentación",
	}

	hasEvent := false
	for _, marker := range eventMarkers {
		if strings.Contains(text, normalizeText(marker)) {
			hasEvent = true
			break
		}
	}
	if !hasEvent {
		return false
	}

	concreteMarkers := []string{
		"fecha", "hora", "lugar", "auditorio", "salon", "salón",
		"gran salon", "corferias", "mayo", "abril", "junio", "2026",
		"a m", "p m", "participacion", "participación", "ponente",
	}
	for _, marker := range concreteMarkers {
		if strings.Contains(text, normalizeText(marker)) {
			return true
		}
	}

	return false
}

func hasInstitutionalNewsValue(text string) bool {
	markers := []string{
		"nuevo programa", "nueva oferta", "convenio", "reconocimiento",
		"acreditacion", "acreditación", "investigacion", "investigación",
		"publicacion", "publicación", "convocatoria", "comunicado",
		"aniversario", "rectora", "escuela", "facultad",
	}

	for _, marker := range markers {
		if strings.Contains(text, normalizeText(marker)) {
			return true
		}
	}

	return false
}

func isPureAdvertisingText(text string) bool {
	promoMarkers := []string{
		"haz que pase",
		"cumple tus suenos",
		"cumple tus sueños",
		"tu futuro empieza",
		"no lo pienses",
		"da el paso",
		"quiero mas para mi vida",
		"quiero más para mi vida",
		"hazlo posible",
		"oportunidades no esperan",
	}

	promoCount := 0
	for _, marker := range promoMarkers {
		if strings.Contains(text, normalizeText(marker)) {
			promoCount++
		}
	}

	return promoCount >= 2 && !hasAdmissionsNewsValue(text) && !hasEventNewsValue(text) && !hasInstitutionalNewsValue(text)
}

func normalizeText(text string) string {
	text = repairMojibake(text)
	text = strings.ToLower(text)
	text = strings.TrimSpace(text)

	replacer := strings.NewReplacer(
		"á", "a",
		"é", "e",
		"í", "i",
		"ó", "o",
		"ú", "u",
		"ü", "u",
		"ñ", "n",
		"á", "a",
		"é", "e",
		"í", "i",
		"ó", "o",
		"ú", "u",
		"ü", "u",
		"ñ", "n",
		"#", " ",
		".", " ",
		",", " ",
		";", " ",
		":", " ",
		"!", " ",
		"¡", " ",
		"¡", " ",
		"?", " ",
		"¿", " ",
		"¿", " ",
		"\n", " ",
		"\t", " ",
	)

	text = replacer.Replace(text)

	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return strings.TrimSpace(text)
}

func buildTitle(caption string) string {
	caption = cleanEditorialText(caption)

	if caption == "" {
		return "Nueva publicación institucional"
	}

	parts := strings.Split(caption, "\n")
	firstLine := strings.TrimSpace(parts[0])

	if firstLine == "" {
		firstLine = caption
	}

	runes := []rune(firstLine)

	if len(runes) > 90 {
		return string(runes[:90]) + "..."
	}

	return formalizeTitle(firstLine)
}

func buildSummary(caption string) string {
	caption = cleanEditorialText(caption)

	if caption == "" {
		return "Contenido publicado en la cuenta institucional de Instagram."
	}

	runes := []rune(caption)

	if len(runes) > 220 {
		return string(runes[:220]) + "..."
	}

	return caption
}

func buildBody(caption string, summary string) string {
	caption = cleanEditorialText(caption)
	if caption == "" {
		return summary
	}

	return caption
}

func normalizeEditorialClassification(classification NewsClassification, sourceText string) NewsClassification {
	classification.Title = formalizeTitle(classification.Title)
	if classification.Title == "" {
		classification.Title = buildTitle(sourceText)
	}

	classification.Summary = cleanEditorialText(classification.Summary)
	if classification.Summary == "" {
		classification.Summary = buildSummary(sourceText)
	}

	classification.Body = cleanEditorialText(classification.Body)
	if classification.Body == "" {
		classification.Body = buildBody(sourceText, classification.Summary)
	}

	classification = enforceUNIMETAInstitutionalIdentity(classification)

	if !isNewsworthyForPublication(sourceText, classification) {
		classification.IsNews = false
		classification.Category = CategoryOther
		classification.Importance = ImportanceLow
		classification.Confidence = minConfidence(classification.Confidence, 0.45)
		classification.Reason = appendClassificationReason(
			classification.Reason,
			"no supera criterio editorial de noticia institucional",
		)
	}

	return classification
}

func enforceUNIMETAInstitutionalIdentity(classification NewsClassification) NewsClassification {
	classification.Title = replaceUNIMETAIdentityErrors(classification.Title)
	classification.Summary = replaceUNIMETAIdentityErrors(classification.Summary)
	classification.Body = replaceUNIMETAIdentityErrors(classification.Body)
	return classification
}

func replaceUNIMETAIdentityErrors(value string) string {
	value = repairMojibake(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\buniversidad\s+nacional\s+de\s+medell[ií]n\s*\(unimeta\)`),
		regexp.MustCompile(`(?i)\buniversidad\s+nacional\s+de\s+medell[ií]n\b`),
		regexp.MustCompile(`(?i)\buniversidad\s+nacional\s+de\s+colombia\s*\(unimeta\)`),
		regexp.MustCompile(`(?i)\buniversidad\s+nacional\s+de\s+colombia\b`),
		regexp.MustCompile(`(?i)\buniversidad\s+del\s+meta\s*\(unimeta\)`),
		regexp.MustCompile(`(?i)\buniversidad\s+del\s+meta\b`),
		regexp.MustCompile(`(?i)\buniversity\s+of\s+meta\s*\(unimeta\)`),
		regexp.MustCompile(`(?i)\buniversity\s+of\s+meta\b`),
		// Variantes genéricas que el LLM puede generar.
		regexp.MustCompile(`(?i)\buniversidad\s+nacional\s*\(unimeta\)`),
		regexp.MustCompile(`(?i)\buniversidad\s+metropolitana\s+del\s+meta\b`),
		regexp.MustCompile(`(?i)\buniversidad\s+metropolitana\s*\(unimeta\)`),
	}

	for _, pattern := range patterns {
		value = pattern.ReplaceAllString(value, officialUNIMETAName)
	}

	return strings.TrimSpace(value)
}

func isNewsworthyForPublication(sourceText string, classification NewsClassification) bool {
	if !classification.IsNews {
		return false
	}

	text := normalizeText(sourceText)
	if text == "" {
		text = normalizeText(classification.Title + " " + classification.Summary + " " + classification.Body)
	}

	if isPureAdvertisingText(text) {
		return false
	}

	return hasAdmissionsNewsValue(text) ||
		hasEventNewsValue(text) ||
		hasInstitutionalNewsValue(text) ||
		classification.Category == CategoryResearch ||
		classification.Category == CategoryTechnology ||
		classification.Category == CategoryWellbeing
}

func minConfidence(value float64, max float64) float64 {
	if value > max {
		return max
	}
	return value
}

func cleanEditorialText(value string) string {
	value = repairMojibake(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	lines := strings.Split(value, "\n")
	cleanLines := []string{}
	for _, line := range lines {
		line = repairMojibake(line)
		line = stripEmojiAndSymbols(line)
		line = stripHashtags(line)
		line = strings.TrimSpace(line)
		if line == "" || isSocialOnlyLine(line) {
			continue
		}
		cleanLines = append(cleanLines, line)
	}

	clean := strings.Join(cleanLines, "\n")
	clean = repairMojibake(clean)
	clean = normalizeWhitespace(clean)
	clean = strings.Trim(clean, " -–—|")
	return strings.TrimSpace(clean)
}

func stripEmojiAndSymbols(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == utf8.RuneError:
			return -1
		case r == 0xFE0F || r == 0x200D:
			return -1
		case unicode.In(r, unicode.So, unicode.Sk, unicode.Sm):
			return -1
		case r >= 0x1F000 && r <= 0x1FAFF:
			return -1
		default:
			return r
		}
	}, value)
}

func stripHashtags(value string) string {
	parts := strings.Fields(value)
	kept := []string{}
	for _, part := range parts {
		if strings.HasPrefix(part, "#") {
			continue
		}
		kept = append(kept, part)
	}

	return strings.Join(kept, " ")
}

func isSocialOnlyLine(value string) bool {
	normalized := normalizeText(value)
	return normalized == "" ||
		strings.HasPrefix(normalized, "www ") ||
		strings.HasPrefix(normalized, "http ") ||
		strings.HasPrefix(normalized, "https ")
}

func normalizeWhitespace(value string) string {
	value = regexp.MustCompile(`[ \t]+`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`\n{3,}`).ReplaceAllString(value, "\n\n")
	return strings.TrimSpace(value)
}

func formalizeTitle(value string) string {
	value = cleanEditorialText(value)
	hadEllipsis := hasTrailingEllipsis(value)
	value = cleanNewsTitleText(value)
	value = strings.Trim(value, " .,:;!\u00A1?\u00BF-\u2013\u2014|/*\u00B7\u2022")
	if value == "" {
		return ""
	}

	if isMostlyUpper(value) {
		value = strings.ToLower(value)
	}

	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	value = string(runes)
	value = fixKnownAcronyms(value)
	value = truncateRunes(value, 90)
	if hadEllipsis && !hasTrailingEllipsis(value) {
		runes := []rune(value)
		if len(runes) > 86 {
			value = string(runes[:86])
		}
		value = strings.TrimSpace(value) + " ..."
	}

	return value
}

func hasTrailingEllipsis(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasSuffix(value, "...") || strings.HasSuffix(value, "\u2026")
}

func cleanNewsTitleText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	value = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r):
			return r
		case isAllowedNewsTitlePunctuation(r):
			return r
		default:
			return -1
		}
	}, value)

	value = normalizeWhitespace(value)
	value = regexp.MustCompile(`\s+([,.:;!?])`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile("([\u00A1\u00BF])\\s+").ReplaceAllString(value, "$1")
	value = strings.Trim(value, " .,:;!\u00A1?\u00BF-\u2013\u2014|/*\u00B7\u2022")

	return strings.TrimSpace(value)
}

func isAllowedNewsTitlePunctuation(r rune) bool {
	switch r {
	case '.', ',', ':', ';', '!', '\u00A1', '?', '\u00BF',
		'\'', '"', '\u201C', '\u201D', '\u2018', '\u2019',
		'(', ')', '[', ']',
		'-', '\u2013', '\u2014',
		'/', '&', '+', '%', '\u00B0':
		return true
	default:
		return false
	}
}

func repairMojibake(value string) string {
	if value == "" {
		return ""
	}

	for i := 0; i < 3; i++ {
		repaired := replaceCommonMojibake(value)
		decoded, ok := decodeMojibakeOnce(repaired)
		if !ok {
			value = repaired
			break
		}
		value = decoded
	}

	return replaceCommonMojibake(value)
}

func replaceCommonMojibake(value string) string {
	replacer := strings.NewReplacer(
		"ÃƒÂ¡", "\u00E1",
		"ÃƒÂ©", "\u00E9",
		"ÃƒÂ­", "\u00ED",
		"ÃƒÂ³", "\u00F3",
		"ÃƒÂº", "\u00FA",
		"ÃƒÂ±", "\u00F1",
		"Ã¡", "\u00E1",
		"Ã©", "\u00E9",
		"Ã­", "\u00ED",
		"Ã³", "\u00F3",
		"Ãº", "\u00FA",
		"Ã¼", "\u00FC",
		"Ã±", "\u00F1",
		"Ã", "\u00C1",
		"Ã‰", "\u00C9",
		"Ã", "\u00CD",
		"Ã“", "\u00D3",
		"Ãš", "\u00DA",
		"Ã‘", "\u00D1",
		"Ã‚Â¡", "\u00A1",
		"Ã‚Â¿", "\u00BF",
		"Â¡", "\u00A1",
		"Â¿", "\u00BF",
		"Â°", "\u00B0",
		"Âº", "\u00BA",
		"Âª", "\u00AA",
		"Ã‚Â", "",
		"Â", "",
		"â€œ", "\u201C",
		"â€", "\u201D",
		"â€˜", "\u2018",
		"â€™", "\u2019",
		"â€“", "\u2013",
		"â€”", "\u2014",
		"â€¦", "\u2026",
		"â€¢", "",
	)

	return replacer.Replace(value)
}

func decodeMojibakeOnce(value string) (string, bool) {
	if !strings.ContainsAny(value, "ÃÂâ€Æ") {
		return value, false
	}

	bytes := make([]byte, 0, len(value))
	for _, r := range value {
		if r <= 0xFF {
			bytes = append(bytes, byte(r))
			continue
		}

		if b, ok := windows1252Byte(r); ok {
			bytes = append(bytes, b)
			continue
		}

		return value, false
	}

	if !utf8.Valid(bytes) {
		return value, false
	}

	decoded := string(bytes)
	return decoded, decoded != value
}

func windows1252Byte(r rune) (byte, bool) {
	switch r {
	case '\u20AC':
		return 0x80, true
	case '\u201A':
		return 0x82, true
	case '\u0192':
		return 0x83, true
	case '\u201E':
		return 0x84, true
	case '\u2026':
		return 0x85, true
	case '\u2020':
		return 0x86, true
	case '\u2021':
		return 0x87, true
	case '\u02C6':
		return 0x88, true
	case '\u2030':
		return 0x89, true
	case '\u0160':
		return 0x8A, true
	case '\u2039':
		return 0x8B, true
	case '\u0152':
		return 0x8C, true
	case '\u017D':
		return 0x8E, true
	case '\u2018':
		return 0x91, true
	case '\u2019':
		return 0x92, true
	case '\u201C':
		return 0x93, true
	case '\u201D':
		return 0x94, true
	case '\u2022':
		return 0x95, true
	case '\u2013':
		return 0x96, true
	case '\u2014':
		return 0x97, true
	case '\u02DC':
		return 0x98, true
	case '\u2122':
		return 0x99, true
	case '\u0161':
		return 0x9A, true
	case '\u203A':
		return 0x9B, true
	case '\u0153':
		return 0x9C, true
	case '\u017E':
		return 0x9E, true
	case '\u0178':
		return 0x9F, true
	default:
		return 0, false
	}
}

func isMostlyUpper(value string) bool {
	letters := 0
	upper := 0
	for _, r := range value {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.IsUpper(r) {
			upper++
		}
	}

	return letters > 0 && float64(upper)/float64(letters) >= 0.70
}

func fixKnownAcronyms(value string) string {
	replacements := map[string]string{
		"unimeta": "UNIMETA",
		"filbo":   "FILBO",
		"cbtic":   "CBTIC",
		"ia":      "IA",
	}

	words := strings.Fields(value)
	for index, word := range words {
		trimmed := strings.Trim(word, ".,:;()[]{}")
		if replacement, ok := replacements[strings.ToLower(trimmed)]; ok {
			words[index] = strings.Replace(word, trimmed, replacement, 1)
		}
	}

	return strings.Join(words, " ")
}
