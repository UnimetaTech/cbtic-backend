package news

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

type Syncer struct {
	repo           *Repository
	extractor      ReelExtractor
	classifier     NewsClassifier
	targetUsername string
	recentWindow   time.Duration
}

type strictNewsClassifier interface {
	ClassifyStrict(ctx context.Context, reel ExtractedReel) (NewsClassification, error)
}

type classifierReadiness interface {
	Enabled() bool
}

func NewSyncer(
	repo *Repository,
	extractor ReelExtractor,
	classifier NewsClassifier,
	targetUsername string,
	recentDays int,
) *Syncer {
	if recentDays <= 0 {
		recentDays = 7
	}

	return &Syncer{
		repo:           repo,
		extractor:      extractor,
		classifier:     classifier,
		targetUsername: targetUsername,
		recentWindow:   time.Duration(recentDays) * 24 * time.Hour,
	}
}

func (s *Syncer) Sync(ctx context.Context) (SyncStats, error) {
	if s.targetUsername == "" {
		return SyncStats{}, fmt.Errorf("INSTAGRAM_TARGET_USERNAME no está configurado")
	}

	stats := SyncStats{}

	jobRunID, err := s.repo.StartJobRun(ctx, syncInstagramReelsJobName)
	if err != nil {
		return stats, err
	}

	repairedTitles, err := s.RepairSimilarTitles(ctx, 100)
	if err != nil {
		log.Printf("news title repair fallo: %v", err)
	} else if repairedTitles > 0 {
		log.Printf("news title repair corrigio %d titulos existentes", repairedTitles)
	}

	reels, err := s.extractor.GetLatestReels(ctx, s.targetUsername)
	if err != nil {
		_ = s.repo.FailJobRun(ctx, jobRunID, stats, err.Error())
		return stats, err
	}

	stats.Extracted = len(reels)

	titleHistory := s.loadTitleHistory(ctx)

	for _, reel := range reels {
		if !s.isRecent(reel, time.Now()) {
			stats.SkippedOld++
			continue
		}

		exists, err := s.repo.ExistsBySourceMediaID(ctx, "instagram", reel.SourceMediaID)
		if err != nil {
			stats.FailedItems++
			continue
		}

		if exists {
			stats.Duplicates++

			classification, err := s.classifier.Classify(ctx, reel)
			if err != nil {
				stats.FailedItems++
				continue
			}

			stats.Classified++
			titleHistory = s.makeClassificationTitleDistinct(&classification, reel.Caption, titleHistory)

			if !classification.IsNews {
				stats.SkippedNotNews++
				if err := s.repo.UpdateExistingFromReel(ctx, reel, classification, NewsStatusNeedsReview); err != nil {
					stats.FailedItems++
				}
				continue
			}

			if err := s.repo.UpdateExistingFromReel(ctx, reel, classification, statusFromClassification(classification)); err != nil {
				stats.FailedItems++
			}
			continue
		}

		stats.NewItems++

		classification, err := s.classifier.Classify(ctx, reel)
		if err != nil {
			// Clasificación falló — insertar con datos mínimos como needs_review
			// para que no se pierda y pueda ser reprocesada después
			log.Printf("clasificacion fallo para reel %s, insertando como needs_review: %v", reel.SourceMediaID, err)
			minimalClassification := NewsClassification{
				Category:   CategoryOther,
				Title:      buildTitle(reel.Caption),
				Summary:    buildSummary(reel.Caption),
				Body:       buildBody(reel.Caption, buildSummary(reel.Caption)),
				Importance: ImportanceLow,
				Confidence: 0.10,
				IsNews:     false,
				Reason:     fmt.Sprintf("clasificacion fallo: %v", err),
			}
			titleHistory = s.makeClassificationTitleDistinct(&minimalClassification, reel.Caption, titleHistory)
			if inserted, insertErr := s.repo.InsertFromReel(ctx, reel, minimalClassification, NewsStatusNeedsReview); insertErr != nil {
				stats.FailedItems++
			} else if inserted {
				stats.Inserted++
			}
			continue
		}

		stats.Classified++
		titleHistory = s.makeClassificationTitleDistinct(&classification, reel.Caption, titleHistory)

		if !classification.IsNews {
			stats.SkippedNotNews++
			// Insertar como needs_review en vez de descartar, para que
			// el admin o el reprocesamiento con IA pueda rescatarla
			if inserted, insertErr := s.repo.InsertFromReel(ctx, reel, classification, NewsStatusNeedsReview); insertErr != nil {
				stats.FailedItems++
			} else if inserted {
				stats.Inserted++
			}
			continue
		}

		inserted, err := s.repo.InsertFromReel(ctx, reel, classification, statusFromClassification(classification))
		if err != nil {
			stats.FailedItems++
			continue
		}

		if inserted {
			stats.Inserted++
		} else {
			stats.Duplicates++
		}
	}

	if stats.FailedItems > 0 {
		_ = s.repo.FailJobRun(ctx, jobRunID, stats, "uno o más reels fallaron durante la sincronización")
		return stats, nil
	}

	if err := s.repo.FinishJobRun(ctx, jobRunID, stats); err != nil {
		return stats, err
	}

	return stats, nil
}

func (s *Syncer) RepairSimilarTitles(ctx context.Context, limit int) (int, error) {
	items, err := s.repo.ListInstagramTitleRepairCandidates(ctx, limit)
	if err != nil {
		return 0, err
	}

	titleHistory := []string{}
	repaired := 0

	for _, item := range items {
		sourceText := bestTitleSourceText(item)
		if sourceText == "" {
			titleHistory = append(titleHistory, item.Title)
			continue
		}

		needsRepair := isGenericInstitutionalTitle(item.Title) || hasSimilarTitle(item.Title, titleHistory)
		if !needsRepair {
			titleHistory = append(titleHistory, item.Title)
			continue
		}

		repairedTitle := makeDistinctiveTitle(item.Title, sourceText, titleHistory)
		if strings.TrimSpace(repairedTitle) == "" {
			titleHistory = append(titleHistory, item.Title)
			continue
		}

		if areTitlesTooSimilar(repairedTitle, item.Title) && !isGenericInstitutionalTitle(item.Title) {
			titleHistory = append(titleHistory, item.Title)
			continue
		}

		if repairedTitle != item.Title {
			if err := s.repo.UpdateTitle(ctx, item.ID, repairedTitle); err != nil {
				return repaired, err
			}

			repaired++
		}

		titleHistory = append(titleHistory, repairedTitle)
	}

	return repaired, nil
}

func bestTitleSourceText(item TitleRepairCandidate) string {
	for _, value := range []string{item.Caption, item.Body, item.Summary} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}

func (s *Syncer) ReprocessExistingWithAI(ctx context.Context, limit int) (AIReprocessStats, error) {
	items, err := s.repo.ListInstagramItemsForAIReprocessing(ctx, limit)
	if err != nil {
		return AIReprocessStats{}, err
	}

	stats := AIReprocessStats{
		Scanned: len(items),
	}

	titleHistory := s.loadTitleHistory(ctx)

	for _, item := range items {
		reel := ExtractedReel{
			SourceMediaID: item.SourceMediaID,
			ReelURL:       item.ReelURL,
			ThumbnailURL:  item.ThumbnailURL,
			Caption:       item.Caption,
			PublishedAt:   item.PublishedAt,
		}

		classification, err := s.classifyExistingItemWithAI(ctx, reel)
		if err != nil {
			stats.Failed++
			continue
		}
		titleHistory = s.makeClassificationTitleDistinct(&classification, reel.Caption, titleHistory)

		if err := s.repo.UpdateFromAIClassification(ctx, item.ID, classification); err != nil {
			stats.Failed++
			continue
		}

		stats.Processed++
		if classification.IsNews {
			stats.Published++
		} else {
			stats.Rejected++
		}
	}

	return stats, nil
}

func statusFromClassification(classification NewsClassification) string {
	if classification.IsNews {
		return NewsStatusPublished
	}

	return NewsStatusNeedsReview
}

func (s *Syncer) ClassifierReady() bool {
	readiness, ok := s.classifier.(classifierReadiness)
	if !ok {
		return true
	}

	return readiness.Enabled()
}

func (s *Syncer) classifyExistingItemWithAI(ctx context.Context, reel ExtractedReel) (NewsClassification, error) {
	if classifier, ok := s.classifier.(strictNewsClassifier); ok {
		return classifier.ClassifyStrict(ctx, reel)
	}

	return s.classifier.Classify(ctx, reel)
}

func (s *Syncer) loadTitleHistory(ctx context.Context) []string {
	titles, err := s.repo.ListRecentTitles(ctx, 100)
	if err != nil {
		return []string{}
	}

	return titles
}

func (s *Syncer) makeClassificationTitleDistinct(classification *NewsClassification, sourceText string, titleHistory []string) []string {
	if classification == nil || !classification.IsNews {
		return titleHistory
	}

	classification.Title = makeDistinctiveTitle(classification.Title, sourceText, titleHistory)
	return append(titleHistory, classification.Title)
}

func (s *Syncer) isRecent(reel ExtractedReel, now time.Time) bool {
	if reel.PublishedAt == nil {
		return true
	}

	cutoff := now.Add(-s.recentWindow)
	return !reel.PublishedAt.Before(cutoff) && !reel.PublishedAt.After(now.Add(24*time.Hour))
}
