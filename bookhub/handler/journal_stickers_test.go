package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bookhub/auth"
	"bookhub/mw"
)

func TestJournalStickerCatalogIntegrity(t *testing.T) {
	if len(journalStickerCategories) == 0 {
		t.Fatal("sticker categories must not be empty")
	}

	if len(journalBuiltInStickers) == 0 {
		t.Fatal("built-in stickers must not be empty")
	}

	categories := make(map[string]bool)

	for _, category := range journalStickerCategories {
		if category.ID == "" || category.Name == "" {
			t.Fatalf("invalid category: %+v", category)
		}

		if categories[category.ID] {
			t.Fatalf("duplicate category ID: %s", category.ID)
		}

		categories[category.ID] = true
	}

	stickerIDs := make(map[string]bool)
	assetKeys := make(map[string]bool)

	for _, sticker := range journalBuiltInStickers {
		if sticker.ID == "" || sticker.Name == "" || sticker.AssetKey == "" {
			t.Fatalf("invalid sticker: %+v", sticker)
		}

		if stickerIDs[sticker.ID] {
			t.Fatalf("duplicate sticker ID: %s", sticker.ID)
		}

		if assetKeys[sticker.AssetKey] {
			t.Fatalf("duplicate asset key: %s", sticker.AssetKey)
		}

		if !categories[sticker.Category] {
			t.Fatalf(
				"sticker %s references unknown category %s",
				sticker.ID,
				sticker.Category,
			)
		}

		stickerIDs[sticker.ID] = true
		assetKeys[sticker.AssetKey] = true
	}
}

func TestGetJournalStickersRequiresLogin(t *testing.T) {
	issuer := &auth.Issuer{
		Secret: []byte("journal-sticker-test-secret"),
		TTL:    time.Hour,
	}

	handler := &Handler{}

	mux := http.NewServeMux()
	mux.HandleFunc(
		"GET /api/journal/stickers",
		mw.RequireLogin(issuer, handler.GetJournalStickers),
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/journal/stickers",
		nil,
	)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusUnauthorized,
			rec.Code,
		)
	}
}

func TestGetJournalStickersReturnsCatalog(t *testing.T) {
	issuer := &auth.Issuer{
		Secret: []byte("journal-sticker-test-secret"),
		TTL:    time.Hour,
	}

	token, err := issuer.Issue(123, "member")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	handler := &Handler{}

	mux := http.NewServeMux()
	mux.HandleFunc(
		"GET /api/journal/stickers",
		mw.RequireLogin(issuer, handler.GetJournalStickers),
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/journal/stickers",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusOK,
			rec.Code,
			rec.Body.String(),
		)
	}

	var response struct {
		Version    int                      `json:"version"`
		Categories []journalStickerCategory `json:"categories"`
		Items      []journalSticker         `json:"items"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.Version != 1 {
		t.Fatalf("expected catalog version 1, got %d", response.Version)
	}

	if len(response.Categories) != len(journalStickerCategories) {
		t.Fatalf("unexpected category count: %d", len(response.Categories))
	}

	if len(response.Items) != len(journalBuiltInStickers) {
		t.Fatalf("unexpected sticker count: %d", len(response.Items))
	}
}
