package handler

import "net/http"

// journalStickerCategory describes a built-in sticker category.
type journalStickerCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// journalSticker describes one built-in sticker.
// AssetKey is a stable key that the frontend maps to its bundled image.
type journalSticker struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	AssetKey string `json:"assetKey"`
	Preview  string `json:"preview"`
}

var journalStickerCategories = []journalStickerCategory{
	{ID: "flowers", Name: "Flowers"},
	{ID: "cozy", Name: "Cozy"},
	{ID: "reading", Name: "Reading"},
	{ID: "love", Name: "Love"},
	{ID: "nature", Name: "Nature"},
	{ID: "seasonal", Name: "Seasonal"},
}

var journalBuiltInStickers = []journalSticker{
	// Flowers
	{ID: "flower-daisy", Name: "Daisy", Category: "flowers", AssetKey: "flower-daisy", Preview: "🌼"},
	{ID: "flower-tulip", Name: "Tulip", Category: "flowers", AssetKey: "flower-tulip", Preview: "🌷"},
	{ID: "flower-sunflower", Name: "Sunflower", Category: "flowers", AssetKey: "flower-sunflower", Preview: "🌻"},
	{ID: "flower-cherry", Name: "Cherry Blossom", Category: "flowers", AssetKey: "flower-cherry", Preview: "🌸"},

	// Cozy
	{ID: "cozy-coffee", Name: "Coffee", Category: "cozy", AssetKey: "cozy-coffee", Preview: "☕"},
	{ID: "cozy-tea", Name: "Tea", Category: "cozy", AssetKey: "cozy-tea", Preview: "🍵"},
	{ID: "cozy-candle", Name: "Candle", Category: "cozy", AssetKey: "cozy-candle", Preview: "🕯️"},
	{ID: "cozy-cloud", Name: "Cloud", Category: "cozy", AssetKey: "cozy-cloud", Preview: "☁️"},

	// Reading
	{ID: "reading-book", Name: "Book", Category: "reading", AssetKey: "reading-book", Preview: "📖"},
	{ID: "reading-books", Name: "Books", Category: "reading", AssetKey: "reading-books", Preview: "📚"},
	{ID: "reading-bookmark", Name: "Bookmark", Category: "reading", AssetKey: "reading-bookmark", Preview: "🔖"},
	{ID: "reading-letter", Name: "Love Letter", Category: "reading", AssetKey: "reading-letter", Preview: "💌"},

	// Love
	{ID: "love-pink-heart", Name: "Pink Heart", Category: "love", AssetKey: "love-pink-heart", Preview: "💗"},
	{ID: "love-sparkle-heart", Name: "Sparkling Heart", Category: "love", AssetKey: "love-sparkle-heart", Preview: "💖"},
	{ID: "love-ribbon", Name: "Ribbon", Category: "love", AssetKey: "love-ribbon", Preview: "🎀"},
	{ID: "love-star", Name: "Star", Category: "love", AssetKey: "love-star", Preview: "⭐"},

	// Nature
	{ID: "nature-butterfly", Name: "Butterfly", Category: "nature", AssetKey: "nature-butterfly", Preview: "🦋"},
	{ID: "nature-leaf", Name: "Leaf", Category: "nature", AssetKey: "nature-leaf", Preview: "🍃"},
	{ID: "nature-mushroom", Name: "Mushroom", Category: "nature", AssetKey: "nature-mushroom", Preview: "🍄"},
	{ID: "nature-moon", Name: "Moon", Category: "nature", AssetKey: "nature-moon", Preview: "🌙"},

	// Seasonal
	{ID: "seasonal-christmas", Name: "Christmas Tree", Category: "seasonal", AssetKey: "seasonal-christmas", Preview: "🎄"},
	{ID: "seasonal-pumpkin", Name: "Pumpkin", Category: "seasonal", AssetKey: "seasonal-pumpkin", Preview: "🎃"},
	{ID: "seasonal-snowflake", Name: "Snowflake", Category: "seasonal", AssetKey: "seasonal-snowflake", Preview: "❄️"},
	{ID: "seasonal-gift", Name: "Gift", Category: "seasonal", AssetKey: "seasonal-gift", Preview: "🎁"},
}

// GetJournalStickers returns the built-in sticker catalog.
// The catalog is static Go data and does not access the database.
func (h *Handler) GetJournalStickers(
	w http.ResponseWriter,
	r *http.Request,
) {
	if _, ok := journalMemberID(w, r); !ok {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":    1,
		"categories": journalStickerCategories,
		"items":      journalBuiltInStickers,
	})
}
