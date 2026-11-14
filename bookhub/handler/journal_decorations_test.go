package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func decorationJSON(
	stickerID string,
	x, y, rotation, scale float64,
	zIndex int,
) string {
	return `{
		"type":"builtin",
		"stickerId":"` + stickerID + `",
		"x":` + jsonNumber(x) + `,
		"y":` + jsonNumber(y) + `,
		"rotation":` + jsonNumber(rotation) + `,
		"scale":` + jsonNumber(scale) + `,
		"zIndex":` + jsonNumber(float64(zIndex)) + `
	}`
}

func jsonNumber(value float64) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func TestJournalDecorationValidation(t *testing.T) {
	valid := decorationJSON("flower-daisy", 50, 50, 0, 1, 1)

	tests := []struct {
		name    string
		input   string
		wantOK  bool
		wantErr string
	}{
		{
			name:   "valid built-in sticker",
			input:  "[" + valid + "]",
			wantOK: true,
		},
		{
			name:    "unknown sticker",
			input:   "[" + strings.Replace(valid, "flower-daisy", "unknown-sticker", 1) + "]",
			wantErr: "UNKNOWN_STICKER",
		},
		{
			name:    "upload is not supported yet",
			input:   "[" + strings.Replace(valid, `"builtin"`, `"upload"`, 1) + "]",
			wantErr: "BAD_DECORATION_TYPE",
		},
		{
			name:    "x below minimum",
			input:   "[" + decorationJSON("flower-daisy", -1, 50, 0, 1, 1) + "]",
			wantErr: "BAD_DECORATION_POSITION",
		},
		{
			name:    "y above maximum",
			input:   "[" + decorationJSON("flower-daisy", 50, 101, 0, 1, 1) + "]",
			wantErr: "BAD_DECORATION_POSITION",
		},
		{
			name:    "rotation outside range",
			input:   "[" + decorationJSON("flower-daisy", 50, 50, 361, 1, 1) + "]",
			wantErr: "BAD_DECORATION_ROTATION",
		},
		{
			name:    "scale below minimum",
			input:   "[" + decorationJSON("flower-daisy", 50, 50, 0, 0.05, 1) + "]",
			wantErr: "BAD_DECORATION_SCALE",
		},
		{
			name:    "scale above maximum",
			input:   "[" + decorationJSON("flower-daisy", 50, 50, 0, 4, 1) + "]",
			wantErr: "BAD_DECORATION_SCALE",
		},
		{
			name:    "zIndex outside range",
			input:   "[" + decorationJSON("flower-daisy", 50, 50, 0, 1, 1001) + "]",
			wantErr: "BAD_DECORATION_Z_INDEX",
		},
		{
			name:    "unknown field",
			input:   `[{"type":"builtin","stickerId":"flower-daisy","x":50,"y":50,"rotation":0,"scale":1,"zIndex":1,"url":"https://example.com/x.png"}]`,
			wantErr: "BAD_DECORATION_ITEM",
		},
		{
			name:    "missing required field",
			input:   `[{"type":"builtin","stickerId":"flower-daisy","x":50,"y":50,"scale":1,"zIndex":1}]`,
			wantErr: "INCOMPLETE_DECORATION",
		},
		{
			name:    "not an array",
			input:   `{"type":"builtin"}`,
			wantErr: "BAD_DECORATIONS",
		},
		{
			name:    "null array",
			input:   `null`,
			wantErr: "BAD_DECORATIONS",
		},
		{
			name:    "null item",
			input:   `[null]`,
			wantErr: "BAD_DECORATION_TYPE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			ok := validateJournalDecorations(
				rec,
				json.RawMessage(tt.input),
			)

			if ok != tt.wantOK {
				t.Fatalf("expected valid=%v, got %v", tt.wantOK, ok)
			}

			if tt.wantErr != "" && !strings.Contains(rec.Body.String(), tt.wantErr) {
				t.Fatalf(
					"expected error %q in response, got %s",
					tt.wantErr,
					rec.Body.String(),
				)
			}
		})
	}
}

func TestJournalDecorationMaximumItems(t *testing.T) {
	item := decorationJSON("flower-daisy", 50, 50, 0, 1, 1)

	items := make([]string, maxJournalDecorationItems+1)
	for i := range items {
		items[i] = item
	}

	input := "[" + strings.Join(items, ",") + "]"

	rec := httptest.NewRecorder()
	ok := validateJournalDecorations(rec, json.RawMessage(input))

	if ok {
		t.Fatal("expected too many decorations to be rejected")
	}

	if !strings.Contains(rec.Body.String(), "TOO_MANY_DECORATIONS") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}
