package news

const (
	NewsStatusPublished   = "published"
	NewsStatusDraft       = "draft"
	NewsStatusHidden      = "hidden"
	NewsStatusNeedsReview = "needs_review"
)

const (
	ImportanceHigh   = "alta"
	ImportanceMedium = "media"
	ImportanceLow    = "baja"
)

const (
	CategoryAcademic      = "Académico"
	CategoryEvents        = "Eventos"
	CategoryResearch      = "Investigación"
	CategorySports        = "Deportes"
	CategoryTechnology    = "Tecnología"
	CategoryCulture       = "Cultura"
	CategoryWellbeing     = "Bienestar"
	CategoryInstitutional = "Institucional"
	CategoryOther         = "Otro"
)

const syncInstagramReelsJobName = "sync-instagram-reels"

func IsValidNewsStatus(status string) bool {
	switch status {
	case NewsStatusPublished, NewsStatusDraft, NewsStatusHidden, NewsStatusNeedsReview:
		return true
	default:
		return false
	}
}

func IsValidImportance(importance string) bool {
	switch importance {
	case ImportanceHigh, ImportanceMedium, ImportanceLow:
		return true
	default:
		return false
	}
}

func IsValidCategory(category string) bool {
	switch category {
	case CategoryAcademic,
		CategoryEvents,
		CategoryResearch,
		CategorySports,
		CategoryTechnology,
		CategoryCulture,
		CategoryWellbeing,
		CategoryInstitutional,
		CategoryOther:
		return true
	default:
		return false
	}
}
