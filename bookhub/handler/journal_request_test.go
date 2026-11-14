package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJournalRequestRequiresJSONObject(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantOK    bool
		wantTitle string
	}{
		{
			name:      "valid object",
			body:      `{"title":"Reading notes"}`,
			wantOK:    true,
			wantTitle: "Reading notes",
		},
		{
			name: "reject null",
			body: `null`,
		},
		{
			name: "reject array",
			body: `[]`,
		},
		{
			name: "reject string",
			body: `"journal"`,
		},
		{
			name: "reject number",
			body: `123`,
		},
		{
			name: "reject empty body",
			body: "",
		},
		{
			name: "reject malformed JSON",
			body: `{"title":`,
		},
		{
			name: "reject a second JSON document",
			body: `{} {}`,
		},
		{
			name: "reject unknown fields",
			body: `{"unexpected":"field"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(
				http.MethodPost,
				"/api/journal",
				strings.NewReader(tt.body),
			)

			got, ok := decodeJournalRequest(recorder, request)
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v; response=%s", tt.wantOK, ok, recorder.Body.String())
			}

			if tt.wantOK {
				if got.Title != tt.wantTitle {
					t.Fatalf("expected title %q, got %q", tt.wantTitle, got.Title)
				}
				return
			}

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected HTTP %d, got %d", http.StatusBadRequest, recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), "BAD_JSON") {
				t.Fatalf("expected BAD_JSON response, got %s", recorder.Body.String())
			}
		})
	}
}
