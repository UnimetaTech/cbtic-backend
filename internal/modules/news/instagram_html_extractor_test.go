package news

import (
	"testing"
	"time"
)

func TestExtractInstagramMediaURLsIncludesFeedPostsAndReels(t *testing.T) {
	html := `
		<a href="/p/ABC123xyz_-/">post</a>
		<a href="https://www.instagram.com/reel/REEL456abc/">reel</a>
	`

	got := extractInstagramMediaURLs(html)

	if got["ABC123xyz_-"] != "https://www.instagram.com/p/ABC123xyz_-/" {
		t.Fatalf("feed post URL = %q", got["ABC123xyz_-"])
	}

	if got["REEL456abc"] != "https://www.instagram.com/reel/REEL456abc/" {
		t.Fatalf("reel URL = %q", got["REEL456abc"])
	}
}

func TestSyncerTreatsMissingInstagramDateAsRecent(t *testing.T) {
	syncer := &Syncer{recentWindow: 7 * 24 * time.Hour}
	reel := ExtractedReel{SourceMediaID: "ABC123xyz", Caption: "Convocatoria academica"}

	if !syncer.isRecent(reel, time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("isRecent() = false for missing Instagram date, want true")
	}
}

func TestExtractInstagramMediaCandidatesPreservesHTMLOrder(t *testing.T) {
	html := `
		<a href="/p/NEW111abc/">new post</a>
		<a href="/reel/OLD222abc/">old reel</a>
	`

	got := extractInstagramMediaCandidates(html)
	if len(got) != 2 {
		t.Fatalf("len(candidates) = %d, want 2", len(got))
	}

	if got[0].Shortcode != "NEW111abc" || got[0].URL != "https://www.instagram.com/p/NEW111abc/" {
		t.Fatalf("first candidate = %+v", got[0])
	}

	if got[1].Shortcode != "OLD222abc" || got[1].URL != "https://www.instagram.com/reel/OLD222abc/" {
		t.Fatalf("second candidate = %+v", got[1])
	}
}

func TestMergeExtractedInstagramReelsPrefersProfileAPIFields(t *testing.T) {
	apiPublishedAt := time.Date(2026, 5, 11, 15, 0, 0, 0, time.UTC)
	htmlPublishedAt := time.Date(2026, 5, 9, 15, 0, 0, 0, time.UTC)

	got := mergeExtractedInstagramReels(
		[]ExtractedReel{
			{
				SourceMediaID: "POST123abc",
				ReelURL:       "https://www.instagram.com/p/POST123abc/",
				ThumbnailURL:  "https://scontent.cdninstagram.com/new.jpg",
				Caption:       "Caption desde API",
				PublishedAt:   &apiPublishedAt,
			},
		},
		[]ExtractedReel{
			{
				SourceMediaID: "POST123abc",
				ReelURL:       "https://www.instagram.com/p/POST123abc/",
				ThumbnailURL:  "",
				Caption:       "Caption desde HTML",
				PublishedAt:   &htmlPublishedAt,
			},
		},
		10,
	)

	if len(got) != 1 {
		t.Fatalf("len(reels) = %d, want 1", len(got))
	}

	if got[0].ThumbnailURL != "https://scontent.cdninstagram.com/new.jpg" {
		t.Fatalf("ThumbnailURL = %q", got[0].ThumbnailURL)
	}

	if got[0].Caption != "Caption desde API" {
		t.Fatalf("Caption = %q", got[0].Caption)
	}

	if got[0].PublishedAt == nil || !got[0].PublishedAt.Equal(apiPublishedAt) {
		t.Fatalf("PublishedAt = %v, want %v", got[0].PublishedAt, apiPublishedAt)
	}
}

func TestMergeExtractedInstagramReelsFillsMissingAPIFieldsFromHTML(t *testing.T) {
	htmlPublishedAt := time.Date(2026, 5, 11, 16, 0, 0, 0, time.UTC)

	got := mergeExtractedInstagramReels(
		[]ExtractedReel{
			{
				SourceMediaID: "POST123abc",
				ReelURL:       "https://www.instagram.com/p/POST123abc/",
			},
		},
		[]ExtractedReel{
			{
				SourceMediaID: "POST123abc",
				ReelURL:       "https://www.instagram.com/p/POST123abc/",
				ThumbnailURL:  "https://scontent.cdninstagram.com/fallback.jpg",
				Caption:       "Caption fallback desde HTML",
				PublishedAt:   &htmlPublishedAt,
			},
		},
		10,
	)

	if len(got) != 1 {
		t.Fatalf("len(reels) = %d, want 1", len(got))
	}

	if got[0].ThumbnailURL != "https://scontent.cdninstagram.com/fallback.jpg" {
		t.Fatalf("ThumbnailURL = %q", got[0].ThumbnailURL)
	}

	if got[0].Caption != "Caption fallback desde HTML" {
		t.Fatalf("Caption = %q", got[0].Caption)
	}

	if got[0].PublishedAt == nil || !got[0].PublishedAt.Equal(htmlPublishedAt) {
		t.Fatalf("PublishedAt = %v, want %v", got[0].PublishedAt, htmlPublishedAt)
	}
}

func TestExtractReelsFromProfileAPIJSONIncludesTimelinePostImageAndDate(t *testing.T) {
	raw := `{
		"data": {
			"user": {
				"edge_felix_video_timeline": { "edges": [] },
				"edge_owner_to_timeline_media": {
					"edges": [
						{
							"node": {
								"__typename": "GraphImage",
								"shortcode": "POST123abc",
								"is_video": false,
								"taken_at_timestamp": 1778500800,
								"display_url": "https://scontent.cdninstagram.com/post.jpg",
								"caption": { "text": "Publicacion institucional" }
							}
						}
					]
				}
			}
		}
	}`

	got, err := extractReelsFromProfileAPIJSON(raw, 10)
	if err != nil {
		t.Fatalf("extractReelsFromProfileAPIJSON() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(reels) = %d, want 1", len(got))
	}

	if got[0].ReelURL != "https://www.instagram.com/p/POST123abc/" {
		t.Fatalf("ReelURL = %q", got[0].ReelURL)
	}

	if got[0].ThumbnailURL != "https://scontent.cdninstagram.com/post.jpg" {
		t.Fatalf("ThumbnailURL = %q", got[0].ThumbnailURL)
	}

	wantPublishedAt := time.Unix(1778500800, 0).UTC()
	if got[0].PublishedAt == nil || !got[0].PublishedAt.Equal(wantPublishedAt) {
		t.Fatalf("PublishedAt = %v, want %v", got[0].PublishedAt, wantPublishedAt)
	}
}

func TestExtractReelsFromProfileAPIJSONUsesImageVersionFallback(t *testing.T) {
	raw := `{
		"data": {
			"user": {
				"edge_felix_video_timeline": {
					"edges": [
						{
							"node": {
								"__typename": "GraphVideo",
								"shortcode": "REEL123abc",
								"is_video": true,
								"taken_at_timestamp": 1778500800,
								"image_versions2": {
									"candidates": [
										{ "url": "https://scontent.cdninstagram.com/small.jpg", "config_width": 320, "config_height": 320 },
										{ "url": "https://scontent.cdninstagram.com/large.jpg", "config_width": 1080, "config_height": 1080 }
									]
								},
								"caption": { "text": "Convocatoria institucional con fecha y lugar" }
							}
						}
					]
				},
				"edge_owner_to_timeline_media": { "edges": [] }
			}
		}
	}`

	got, err := extractReelsFromProfileAPIJSON(raw, 10)
	if err != nil {
		t.Fatalf("extractReelsFromProfileAPIJSON() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(reels) = %d, want 1", len(got))
	}

	if got[0].ThumbnailURL != "https://scontent.cdninstagram.com/large.jpg" {
		t.Fatalf("ThumbnailURL = %q", got[0].ThumbnailURL)
	}
}

func TestExtractReelsFromProfileAPIJSONFallsBackToInstagramMediaEndpoint(t *testing.T) {
	raw := `{
		"data": {
			"user": {
				"edge_felix_video_timeline": {
					"edges": [
						{
							"node": {
								"__typename": "GraphImage",
								"shortcode": "POST123abc",
								"is_video": false,
								"caption": { "text": "Convocatoria institucional con fecha y lugar" }
							}
						}
					]
				},
				"edge_owner_to_timeline_media": { "edges": [] }
			}
		}
	}`

	got, err := extractReelsFromProfileAPIJSON(raw, 10)
	if err != nil {
		t.Fatalf("extractReelsFromProfileAPIJSON() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(reels) = %d, want 1", len(got))
	}

	want := "https://www.instagram.com/p/POST123abc/media/?size=l"
	if got[0].ThumbnailURL != want {
		t.Fatalf("ThumbnailURL = %q, want %q", got[0].ThumbnailURL, want)
	}
}

func TestExtractInstagramImageURLFromHTMLReadsThumbnailURL(t *testing.T) {
	html := `{"thumbnail_url":"https:\/\/scontent.cdninstagram.com\/thumb.jpg?stp=dst-jpg_e35\u0026ccb=7-5"}`

	got := extractInstagramImageURLFromHTML(normalizeInstagramHTML(html))
	want := "https://scontent.cdninstagram.com/thumb.jpg?stp=dst-jpg_e35&ccb=7-5"

	if got != want {
		t.Fatalf("image URL = %q, want %q", got, want)
	}
}

func TestExtractInstagramImageURLFromHTMLUsesJSONLDThumbnailURL(t *testing.T) {
	html := `<script type="application/ld+json">{"thumbnailUrl":["https:\/\/scontent.cdninstagram.com\/jsonld.jpg?foo=1\u0026bar=2"]}</script>`

	got := extractInstagramImageURLFromHTML(normalizeInstagramHTML(html))
	want := "https://scontent.cdninstagram.com/jsonld.jpg?foo=1&bar=2"

	if got != want {
		t.Fatalf("image URL = %q, want %q", got, want)
	}
}

func TestParseInstagramDateTreatsUnixMilliseconds(t *testing.T) {
	got, ok := parseInstagramDate("1778500800000")
	if !ok {
		t.Fatal("parseInstagramDate() ok = false")
	}

	want := time.Unix(1778500800, 0).UTC()
	if !got.Equal(want) {
		t.Fatalf("parseInstagramDate() = %v, want %v", got, want)
	}
}
