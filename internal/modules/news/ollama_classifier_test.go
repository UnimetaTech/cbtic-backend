package news

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestImproveGenericTitleFromBookCaption(t *testing.T) {
	fallback := NewsClassification{Body: `UNIMETA vivio un encuentro en la Feria Internacional del Libro de Bogota 2026 con el lanzamiento del libro "Pedagogia y territorio: Un metodo propio desde la investigacion formativa".`}

	got := improveGenericTitle("UNIMETA en la Feria Internacional del Libro de Bogota", fallback)

	if got == "UNIMETA en la Feria Internacional del Libro de Bogota" || !strings.Contains(normalizeText(got), "pedagogia") {
		t.Fatalf("improveGenericTitle() = %q, want specific book title", got)
	}
}

func TestImproveGenericTitleFromWorkshopCaption(t *testing.T) {
	fallback := NewsClassification{Body: `UNIMETA realizo el Taller de paradojas termodinamicas: Metabolismo urbano y sostenibilidad, como parte del lanzamiento del libro.`}

	got := improveGenericTitle("UNIMETA participa en Feria Internacional del Libro de Bogota", fallback)

	if !strings.Contains(normalizeText(got), "taller") || !strings.Contains(normalizeText(got), "metabolismo") {
		t.Fatalf("improveGenericTitle() = %q, want workshop-specific title", got)
	}
}

func TestImproveGenericTitleFromLegalEventCaption(t *testing.T) {
	fallback := NewsClassification{Body: `Este 1 de mayo te invitamos a ser parte del evento "UNIMETA 40 ANOS: Imputacion penal a los dirigentes en estructuras criminales", un espacio academico para el analisis juridico.`}

	got := improveGenericTitle("UNIMETA participa en FILBO 2026 con evento academico", fallback)

	if !strings.Contains(normalizeText(got), "imputacion") {
		t.Fatalf("improveGenericTitle() = %q, want legal-event title", got)
	}
}

func TestImproveGenericTitleKeepsSpecificTitle(t *testing.T) {
	fallback := NewsClassification{Body: "Contenido de apoyo"}

	got := improveGenericTitle("Taller sobre metabolismo urbano y sostenibilidad en FILBO", fallback)
	want := "Taller sobre metabolismo urbano y sostenibilidad en FILBO"

	if got != want {
		t.Fatalf("improveGenericTitle() = %q, want %q", got, want)
	}
}

func TestMakeDistinctiveTitleAvoidsSimilarFILBOTitles(t *testing.T) {
	previous := []string{
		"UNIMETA en la Feria Internacional del Libro de Bogota",
		"UNIMETA participa en Feria Internacional del Libro de Bogota",
		"UNIMETA participa en FILBO 2026 con evento academico",
	}
	sourceText := `UNIMETA vivio un encuentro en la Feria Internacional del Libro de Bogota 2026 con el lanzamiento del libro "Pedagogia y territorio: Un metodo propio desde la investigacion formativa".`

	got := makeDistinctiveTitle("UNIMETA en la Feria Internacional del Libro de Bogota", sourceText, previous)

	if areTitlesTooSimilar(got, previous[0]) || areTitlesTooSimilar(got, previous[1]) || areTitlesTooSimilar(got, previous[2]) {
		t.Fatalf("makeDistinctiveTitle() = %q is still too similar to previous titles", got)
	}
}

func TestMakeDistinctiveTitleUsesAlternativeWhenSpecificStillSimilar(t *testing.T) {
	previous := []string{`Lanzamiento del libro "Pedagogia y territorio: Un metodo propio desde la investigacion formativa" en FILBO 2026`}
	sourceText := `UNIMETA vivio un encuentro en la Feria Internacional del Libro de Bogota 2026 con el lanzamiento del libro "Pedagogia y territorio: Un metodo propio desde la investigacion formativa".`

	got := makeDistinctiveTitle("UNIMETA en FILBO 2026", sourceText, previous)

	if got == "UNIMETA en FILBO 2026" || hasSimilarTitle(got, previous) {
		t.Fatalf("makeDistinctiveTitle() = %q, want alternative distinct title", got)
	}
}

func TestAreTitlesTooSimilarDetectsGenericFILBOVariants(t *testing.T) {
	left := "UNIMETA en la Feria Internacional del Libro de Bogota"
	right := "UNIMETA participa en Feria Internacional del Libro de Bogota"

	if !areTitlesTooSimilar(left, right) {
		t.Fatalf("areTitlesTooSimilar(%q, %q) = false, want true", left, right)
	}
}

func TestAreTitlesTooSimilarAllowsDifferentAngles(t *testing.T) {
	left := "Pedagogia y territorio: educacion conectada con la region"
	right := "Debate juridico sobre estructuras criminales en FILBO"

	if areTitlesTooSimilar(left, right) {
		t.Fatalf("areTitlesTooSimilar(%q, %q) = true, want false", left, right)
	}
}

func TestMakeDistinctiveTitleFallsBackToCaptionHeadline(t *testing.T) {
	previous := []string{"UNIMETA abre inscripciones", "Inscripciones abiertas en UNIMETA"}
	sourceText := "Inscripciones abiertas para programas de pregrado con beneficios de matricula hasta el 15 de mayo."

	got := makeDistinctiveTitle("UNIMETA abre inscripciones", sourceText, previous)
	want := "Inscripciones abiertas para programas de pregrado con beneficios de matricula hasta el ..."

	if got != want {
		t.Fatalf("makeDistinctiveTitle() = %q, want %q", got, want)
	}
}

func TestMakeDistinctiveTitleRemovesDecorativeSymbolsAndMojibake(t *testing.T) {
	previous := []string{"UNIMETA participa en FILBO 2026"}
	sourceText := "\U0001F4DA Presentaci\u00C3\u00B3n del libro \"Pedagog\u00C3\u00ADa y territorio\" en FILBO 2026 \u2728 #UNIMETA"

	got := makeDistinctiveTitle("\u2728 UNIMETA participa en FILBO 2026", sourceText, previous)

	if strings.ContainsAny(got, "\U0001F4DA\u2728#") ||
		strings.Contains(got, "Ã") ||
		strings.Contains(got, "Â") ||
		strings.Contains(got, "â€") {
		t.Fatalf("title contains decorative or mojibake noise: %q", got)
	}

	if !strings.Contains(normalizeText(got), "pedagogia") {
		t.Fatalf("title = %q, want book-specific clean title", got)
	}
}

func TestOllamaClassifierRequiresReviewWhenOllamaFails(t *testing.T) {
	classifier := NewOllamaNewsClassifier(true, "http://127.0.0.1:1", "missing-model", NewSimpleNewsClassifier())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	got, err := classifier.Classify(ctx, ExtractedReel{
		SourceMediaID: "DX7ZYDlpKiO",
		ReelURL:       "https://www.instagram.com/p/DX7ZYDlpKiO/",
		Caption:       "Inscripciones abiertas para programas academicos de UNIMETA.",
	})

	if err != nil {
		t.Fatalf("Classify() error = %v, want review fallback without error", err)
	}

	if got.IsNews {
		t.Fatalf("Classify().IsNews = true, want false until AI rewrites editorially")
	}

	if !strings.Contains(got.Reason, "ollama fallo") {
		t.Fatalf("Classify().Reason = %q, want ollama fallback reason", got.Reason)
	}
}
func TestEditorialNormalizationRemovesEmojisAndFormalizesTitle(t *testing.T) {
	classification, err := NewSimpleNewsClassifier().Classify(context.Background(), ExtractedReel{
		SourceMediaID: "DX7ZD8tNL39",
		Caption:       "\U0001F6A8 Â¡INSCRIPCIONES ABIERTAS 2026-2! \U0001F6A8\nTu proxima meta comienza aqui \U0001F447\n\u2714\ufe0f 14 programas de pregrado\n#UNIMETA #Inscripciones",
	})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	if strings.ContainsAny(classification.Title, "\U0001F6A8\U0001F447\u2714\ufe0f#") {
		t.Fatalf("title contains social/emoji noise: %q", classification.Title)
	}

	if classification.Title != "Inscripciones abiertas 2026-2" {
		t.Fatalf("title = %q, want formal title", classification.Title)
	}

	if !classification.IsNews {
		t.Fatalf("inscriptions with concrete offer should be news: reason=%q", classification.Reason)
	}
}

func TestFormalizeTitleRemovesDecorativeSymbols(t *testing.T) {
	got := formalizeTitle("\U0001F6A8 #UNIMETA \u2192 INSCRIPCIONES ABIERTAS 2026-2 \u2705")
	want := "Inscripciones abiertas 2026-2"

	if got != want {
		t.Fatalf("formalizeTitle() = %q, want %q", got, want)
	}
}

func TestEditorialGateRejectsPureAdvertising(t *testing.T) {
	classification, err := NewSimpleNewsClassifier().Classify(context.Background(), ExtractedReel{
		SourceMediaID: "PROMO123",
		Caption:       "Tu futuro empieza hoy. No lo pienses tanto, da el paso y cumple tus sueÃƒÆ’Ã‚Â±os con UNIMETA. Haz que pase.",
	})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	if classification.IsNews {
		t.Fatalf("pure advertising classified as news: %+v", classification)
	}
}

func TestOllamaResponseCopiedFromCaptionRequiresReview(t *testing.T) {
	fallback := NewsClassification{
		Category:   CategoryAcademic,
		Title:      "Todo empieza con una decision",
		Summary:    "Inscripciones abiertas para programas academicos de UNIMETA con beneficios de matricula.",
		Body:       "Inscripciones abiertas para programas academicos de UNIMETA con beneficios de matricula. Hoy es el dia perfecto para empezar.",
		Importance: ImportanceMedium,
		Confidence: 0.8,
		IsNews:     true,
		Reason:     "fallback",
	}

	got := normalizeOllamaNewsResponse(ollamaNewsResponse{
		IsNews:     true,
		Title:      "Todo empieza con una decision",
		Summary:    fallback.Summary,
		Body:       fallback.Body,
		Category:   CategoryAcademic,
		Importance: ImportanceMedium,
		Confidence: 0.9,
		Reason:     "noticia academica",
	}, fallback)

	if got.IsNews {
		t.Fatalf("copied caption should require review, got published classification: %+v", got)
	}

	if !strings.Contains(got.Reason, "no reescribio") {
		t.Fatalf("reason = %q, want rewrite quality reason", got.Reason)
	}
}

func TestOllamaResponseParaphrasedCanPublish(t *testing.T) {
	fallback := NewsClassification{
		Category:   CategoryAcademic,
		Title:      "Todo empieza con una decision",
		Summary:    "Todo empieza con una decision. Inscripciones abiertas para programas academicos de UNIMETA con beneficios de matricula.",
		Body:       "Todo empieza con una decision. Inscripciones abiertas para programas academicos de UNIMETA con beneficios de matricula. Hoy es el dia perfecto para empezar.",
		Importance: ImportanceMedium,
		Confidence: 0.8,
		IsNews:     true,
		Reason:     "fallback",
	}

	got := normalizeOllamaNewsResponse(ollamaNewsResponse{
		IsNews:     true,
		Title:      "UNIMETA abre inscripciones para el periodo 2026-2",
		Summary:    "La instituciÃƒÂ³n habilitÃƒÂ³ su proceso de admisiÃƒÂ³n para programas acadÃƒÂ©micos del periodo 2026-2.",
		Body:       "La Universidad del Meta informÃƒÂ³ la apertura de inscripciones para el periodo 2026-2. La convocatoria incluye programas acadÃƒÂ©micos y beneficios institucionales para nuevos aspirantes.",
		Category:   CategoryAcademic,
		Importance: ImportanceMedium,
		Confidence: 0.9,
		Reason:     "apertura de inscripciones con informacion concreta",
	}, fallback)

	if !got.IsNews {
		t.Fatalf("paraphrased response should publish, got: %+v", got)
	}
}
