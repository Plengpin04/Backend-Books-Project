package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"bookhub/auth"
	"bookhub/mw"
	"bookhub/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	journalHTTPTestSecret = "journal-http-integration-test-secret-32-bytes-minimum"
	journalHTTPTestDBPort = "55432"
	journalHTTPTestDBName = "bookhub_test"
)

type journalHTTPTestEnvironment struct {
	pool    *pgxpool.Pool
	server  http.Handler
	issuer  *auth.Issuer
	memberA int64
	memberB int64
	tokenA  string
	tokenB  string
}

type journalHTTPResponse struct {
	recorder *httptest.ResponseRecorder
}

func journalHTTPTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping HTTP integration test")
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("invalid TEST_DATABASE_URL: %v", err)
	}

	if parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		t.Fatalf("refusing to use non-local test database host: %q", parsed.Hostname())
	}

	if parsed.Port() != journalHTTPTestDBPort {
		t.Fatalf(
			"refusing to use unexpected database port %q; expected %s",
			parsed.Port(),
			journalHTTPTestDBPort,
		)
	}

	if strings.TrimPrefix(parsed.Path, "/") != journalHTTPTestDBName {
		t.Fatalf(
			"refusing to use database %q; expected %s",
			parsed.Path,
			journalHTTPTestDBName,
		)
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("could not connect to test database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("could not ping test database: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func createJournalHTTPTestMember(t *testing.T, pool *pgxpool.Pool, label string) int64 {
	t.Helper()

	email := fmt.Sprintf(
		"journal-http-%s-%d@example.test",
		label,
		time.Now().UnixNano(),
	)

	var memberID int64

	err := pool.QueryRow(
		context.Background(),
		`
			INSERT INTO members (
				email,
				password_hash,
				display_name
			)
			VALUES ($1, $2, $3)
			RETURNING id
		`,
		email,
		"test-password-hash",
		"Journal HTTP Test "+label,
	).Scan(&memberID)

	if err != nil {
		t.Fatalf("could not create test member %s: %v", label, err)
	}

	return memberID
}

func newJournalHTTPTestEnvironment(t *testing.T) *journalHTTPTestEnvironment {
	t.Helper()

	pool := journalHTTPTestPool(t)

	memberA := createJournalHTTPTestMember(t, pool, "A")
	memberB := createJournalHTTPTestMember(t, pool, "B")

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// ลบเฉพาะข้อมูลของสมาชิกทดสอบที่สร้างใน Test นี้
		for _, memberID := range []int64{memberA, memberB} {
			_, _ = pool.Exec(
				ctx,
				`DELETE FROM journal_entries WHERE member_id = $1`,
				memberID,
			)

			_, _ = pool.Exec(
				ctx,
				`DELETE FROM journal_settings WHERE member_id = $1`,
				memberID,
			)

			_, _ = pool.Exec(
				ctx,
				`DELETE FROM members WHERE id = $1`,
				memberID,
			)
		}
	})

	issuer := &auth.Issuer{
		Secret: []byte(journalHTTPTestSecret),
		TTL:    time.Hour,
	}

	tokenA, err := issuer.Issue(memberA, "member")
	if err != nil {
		t.Fatalf("could not issue token for member A: %v", err)
	}

	tokenB, err := issuer.Issue(memberB, "member")
	if err != nil {
		t.Fatalf("could not issue token for member B: %v", err)
	}

	h := &Handler{
		Store:   &store.Store{Pool: pool},
		Tickets: issuer,
	}

	login := func(next http.HandlerFunc) http.HandlerFunc {
		return mw.RequireLogin(issuer, next)
	}

	mux := http.NewServeMux()

	// ใช้ Route เดียวกับ main.go
	mux.HandleFunc("GET /api/journal", login(h.ListJournalEntries))
	mux.HandleFunc("POST /api/journal", login(h.CreateJournalEntry))
	mux.HandleFunc("GET /api/journal/{id}", login(h.GetJournalEntry))
	mux.HandleFunc("PUT /api/journal/{id}", login(h.UpdateJournalEntry))
	mux.HandleFunc("DELETE /api/journal/{id}", login(h.DeleteJournalEntry))

	mux.HandleFunc(
		"GET /api/journal/settings",
		login(h.GetJournalSettings),
	)
	mux.HandleFunc(
		"PUT /api/journal/settings",
		login(h.UpdateJournalSettings),
	)

	mux.HandleFunc(
		"GET /api/journal/stickers",
		login(h.GetJournalStickers),
	)

	return &journalHTTPTestEnvironment{
		pool:    pool,
		server:  mux,
		issuer:  issuer,
		memberA: memberA,
		memberB: memberB,
		tokenA:  tokenA,
		tokenB:  tokenB,
	}
}

func (env *journalHTTPTestEnvironment) request(
	t *testing.T,
	method string,
	path string,
	token string,
	body string,
) journalHTTPResponse {
	t.Helper()

	req := httptest.NewRequest(
		method,
		path,
		strings.NewReader(body),
	)

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	req.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	env.server.ServeHTTP(recorder, req)

	return journalHTTPResponse{
		recorder: recorder,
	}
}

func assertJournalHTTPStatus(
	t *testing.T,
	response journalHTTPResponse,
	expected int,
) {
	t.Helper()

	if response.recorder.Code != expected {
		t.Fatalf(
			"expected HTTP status %d, got %d; response: %s",
			expected,
			response.recorder.Code,
			response.recorder.Body.String(),
		)
	}
}

func decodeJournalHTTPBody[T any](
	t *testing.T,
	response journalHTTPResponse,
) T {
	t.Helper()

	var result T

	if err := json.Unmarshal(response.recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf(
			"could not decode response JSON: %v; body: %s",
			err,
			response.recorder.Body.String(),
		)
	}

	return result
}

func TestJournalHTTPIntegration(t *testing.T) {
	env := newJournalHTTPTestEnvironment(t)

	validEntry := `{
		"entryDate": "2026-11-14",
		"title": "My first journal entry",
		"feeling": "Happy",
		"content": "Today I started my reading journal.",
		"favoriteQuote": "A book is a dream.",
		"decorations": []
	}`

	// ใช้รูปแบบ Built-in Sticker ตาม API ใหม่
	updatedEntry := `{
		"entryDate": "2026-11-14",
		"title": "Updated journal entry",
		"feeling": "Inspired",
		"content": "I updated this entry.",
		"favoriteQuote": "Keep reading.",
		"decorations": [
			{
				"type": "builtin",
				"stickerId": "flower-daisy",
				"x": 50,
				"y": 50,
				"rotation": 0,
				"scale": 1,
				"zIndex": 1
			}
		]
	}`

	var entryID int64

	t.Run("authentication", func(t *testing.T) {
		t.Run("rejects request without token", func(t *testing.T) {
			response := env.request(
				t,
				http.MethodGet,
				"/api/journal",
				"",
				"",
			)

			assertJournalHTTPStatus(t, response, http.StatusUnauthorized)
		})

		t.Run("rejects invalid token", func(t *testing.T) {
			response := env.request(
				t,
				http.MethodGet,
				"/api/journal",
				"not-a-valid-token",
				"",
			)

			assertJournalHTTPStatus(t, response, http.StatusUnauthorized)
		})

		t.Run("rejects expired token", func(t *testing.T) {
			expiredIssuer := &auth.Issuer{
				Secret: []byte(journalHTTPTestSecret),
				TTL:    -time.Minute,
			}

			expiredToken, err := expiredIssuer.Issue(env.memberA, "member")
			if err != nil {
				t.Fatalf("could not create expired token: %v", err)
			}

			response := env.request(
				t,
				http.MethodGet,
				"/api/journal",
				expiredToken,
				"",
			)

			assertJournalHTTPStatus(t, response, http.StatusUnauthorized)
		})
	})

	t.Run("create entry", func(t *testing.T) {
		response := env.request(
			t,
			http.MethodPost,
			"/api/journal",
			env.tokenA,
			validEntry,
		)

		assertJournalHTTPStatus(t, response, http.StatusCreated)

		created := decodeJournalHTTPBody[struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		}](t, response)

		if created.ID <= 0 {
			t.Fatalf("expected a positive journal entry ID, got %d", created.ID)
		}

		if created.Title != "My first journal entry" {
			t.Fatalf("unexpected created title: %q", created.Title)
		}

		entryID = created.ID
	})

	t.Run("read own entry", func(t *testing.T) {
		if entryID <= 0 {
			t.Fatal("entry was not created")
		}

		response := env.request(
			t,
			http.MethodGet,
			"/api/journal/"+strconv.FormatInt(entryID, 10),
			env.tokenA,
			"",
		)

		assertJournalHTTPStatus(t, response, http.StatusOK)

		result := decodeJournalHTTPBody[struct {
			ID      int64  `json:"id"`
			Title   string `json:"title"`
			Content string `json:"content"`
		}](t, response)

		if result.ID != entryID {
			t.Fatalf("expected entry ID %d, got %d", entryID, result.ID)
		}

		if result.Content != "Today I started my reading journal." {
			t.Fatalf("unexpected entry content: %q", result.Content)
		}
	})

	t.Run("member isolation", func(t *testing.T) {
		entryPath := "/api/journal/" + strconv.FormatInt(entryID, 10)

		t.Run("other member cannot read entry", func(t *testing.T) {
			response := env.request(
				t,
				http.MethodGet,
				entryPath,
				env.tokenB,
				"",
			)

			assertJournalHTTPStatus(t, response, http.StatusNotFound)
		})

		t.Run("other member cannot update entry", func(t *testing.T) {
			// ส่งข้อมูล Decoration ที่ถูกต้อง เพื่อให้ทดสอบ ownership
			// ไม่ใช่ถูกปฏิเสธจาก Validation ก่อน
			response := env.request(
				t,
				http.MethodPut,
				entryPath,
				env.tokenB,
				updatedEntry,
			)

			assertJournalHTTPStatus(t, response, http.StatusNotFound)
		})

		t.Run("other member cannot delete entry", func(t *testing.T) {
			response := env.request(
				t,
				http.MethodDelete,
				entryPath,
				env.tokenB,
				"",
			)

			assertJournalHTTPStatus(t, response, http.StatusNotFound)
		})

		t.Run("other member list does not contain entry", func(t *testing.T) {
			response := env.request(
				t,
				http.MethodGet,
				"/api/journal",
				env.tokenB,
				"",
			)

			assertJournalHTTPStatus(t, response, http.StatusOK)

			result := decodeJournalHTTPBody[struct {
				Items []json.RawMessage `json:"items"`
			}](t, response)

			if len(result.Items) != 0 {
				t.Fatalf(
					"expected member B to have no entries, got %d",
					len(result.Items),
				)
			}
		})
	})

	t.Run("update own entry", func(t *testing.T) {
		entryPath := "/api/journal/" + strconv.FormatInt(entryID, 10)

		response := env.request(
			t,
			http.MethodPut,
			entryPath,
			env.tokenA,
			updatedEntry,
		)

		assertJournalHTTPStatus(t, response, http.StatusOK)

		result := decodeJournalHTTPBody[struct {
			Title string `json:"title"`
		}](t, response)

		if result.Title != "Updated journal entry" {
			t.Fatalf("unexpected updated title: %q", result.Title)
		}

		// ตรวจสอบว่า Decoration ถูกบันทึกและอ่านกลับจากฐานข้อมูลได้
		readResponse := env.request(
			t,
			http.MethodGet,
			entryPath,
			env.tokenA,
			"",
		)

		assertJournalHTTPStatus(t, readResponse, http.StatusOK)

		readResult := decodeJournalHTTPBody[struct {
			Decorations []journalDecorationRequest `json:"decorations"`
		}](t, readResponse)

		if len(readResult.Decorations) != 1 {
			t.Fatalf(
				"expected 1 saved decoration, got %d",
				len(readResult.Decorations),
			)
		}

		decoration := readResult.Decorations[0]

		if decoration.Type != "builtin" {
			t.Fatalf("unexpected decoration type: %q", decoration.Type)
		}

		if decoration.StickerID != "flower-daisy" {
			t.Fatalf("unexpected sticker ID: %q", decoration.StickerID)
		}

		if decoration.X == nil || *decoration.X != 50 {
			t.Fatalf("unexpected decoration X position: %v", decoration.X)
		}

		if decoration.Y == nil || *decoration.Y != 50 {
			t.Fatalf("unexpected decoration Y position: %v", decoration.Y)
		}

		if decoration.Rotation == nil || *decoration.Rotation != 0 {
			t.Fatalf("unexpected decoration rotation: %v", decoration.Rotation)
		}

		if decoration.Scale == nil || *decoration.Scale != 1 {
			t.Fatalf("unexpected decoration scale: %v", decoration.Scale)
		}

		if decoration.ZIndex == nil || *decoration.ZIndex != 1 {
			t.Fatalf("unexpected decoration zIndex: %v", decoration.ZIndex)
		}
	})

	t.Run("settings", func(t *testing.T) {
		t.Run("new member receives default settings", func(t *testing.T) {
			response := env.request(
				t,
				http.MethodGet,
				"/api/journal/settings",
				env.tokenA,
				"",
			)

			assertJournalHTTPStatus(t, response, http.StatusOK)

			result := decodeJournalHTTPBody[struct {
				Settings json.RawMessage `json:"settings"`
			}](t, response)

			if string(result.Settings) != "{}" {
				t.Fatalf("expected default settings {}, got %s", result.Settings)
			}
		})

		t.Run("member can save settings", func(t *testing.T) {
			body := `{
				"settings": {
					"theme": "pink",
					"paper": "grid",
					"backgroundColor": "#fff0f5"
				}
			}`

			response := env.request(
				t,
				http.MethodPut,
				"/api/journal/settings",
				env.tokenA,
				body,
			)

			assertJournalHTTPStatus(t, response, http.StatusOK)

			result := decodeJournalHTTPBody[struct {
				Settings map[string]any `json:"settings"`
			}](t, response)

			if result.Settings["theme"] != "pink" {
				t.Fatalf("settings were not saved: %#v", result.Settings)
			}
		})

		t.Run("settings remain private to each member", func(t *testing.T) {
			responseB := env.request(
				t,
				http.MethodGet,
				"/api/journal/settings",
				env.tokenB,
				"",
			)

			assertJournalHTTPStatus(t, responseB, http.StatusOK)

			resultB := decodeJournalHTTPBody[struct {
				Settings json.RawMessage `json:"settings"`
			}](t, responseB)

			if string(resultB.Settings) != "{}" {
				t.Fatalf(
					"member B should receive default settings, got %s",
					resultB.Settings,
				)
			}

			responseA := env.request(
				t,
				http.MethodGet,
				"/api/journal/settings",
				env.tokenA,
				"",
			)

			assertJournalHTTPStatus(t, responseA, http.StatusOK)

			resultA := decodeJournalHTTPBody[struct {
				Settings map[string]any `json:"settings"`
			}](t, responseA)

			if resultA.Settings["theme"] != "pink" {
				t.Fatalf("member A settings changed unexpectedly: %#v", resultA.Settings)
			}
		})
	})

	t.Run("validation", func(t *testing.T) {
		tests := []struct {
			name string
			body string
		}{
			{
				name: "malformed JSON",
				body: `{"title":`,
			},
			{
				name: "extra JSON document",
				body: validEntry + ` {}`,
			},
			{
				name: "invalid date",
				body: `{
					"entryDate": "14-11-2026",
					"title": "Bad date",
					"decorations": []
				}`,
			},
			{
				name: "decorations must be an array",
				body: `{
					"entryDate": "2026-11-14",
					"title": "Bad decorations",
					"decorations": {}
				}`,
			},
			{
				name: "reject client supplied member ID",
				body: `{
					"member_id": 999999,
					"entryDate": "2026-11-14",
					"title": "Forged owner",
					"decorations": []
				}`,
			},
			{
				name: "reject unknown sticker ID",
				body: `{
					"entryDate": "2026-11-14",
					"title": "Unknown sticker",
					"decorations": [
						{
							"type": "builtin",
							"stickerId": "not-a-real-sticker",
							"x": 50,
							"y": 50,
							"rotation": 0,
							"scale": 1,
							"zIndex": 1
						}
					]
				}`,
			},
			{
				name: "reject uploaded sticker before upload support",
				body: `{
					"entryDate": "2026-11-14",
					"title": "Upload sticker",
					"decorations": [
						{
							"type": "upload",
							"stickerId": "my-upload",
							"x": 50,
							"y": 50,
							"rotation": 0,
							"scale": 1,
							"zIndex": 1
						}
					]
				}`,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				response := env.request(
					t,
					http.MethodPost,
					"/api/journal",
					env.tokenA,
					test.body,
				)

				assertJournalHTTPStatus(t, response, http.StatusBadRequest)
			})
		}
	})

	t.Run("invalid entry ID", func(t *testing.T) {
		response := env.request(
			t,
			http.MethodGet,
			"/api/journal/not-a-number",
			env.tokenA,
			"",
		)

		assertJournalHTTPStatus(t, response, http.StatusBadRequest)
	})

	t.Run("delete own entry", func(t *testing.T) {
		entryPath := "/api/journal/" + strconv.FormatInt(entryID, 10)

		response := env.request(
			t,
			http.MethodDelete,
			entryPath,
			env.tokenA,
			"",
		)

		assertJournalHTTPStatus(t, response, http.StatusNoContent)

		if response.recorder.Body.Len() != 0 {
			t.Fatalf(
				"expected empty response body after delete, got %q",
				response.recorder.Body.String(),
			)
		}

		getResponse := env.request(
			t,
			http.MethodGet,
			entryPath,
			env.tokenA,
			"",
		)

		assertJournalHTTPStatus(t, getResponse, http.StatusNotFound)
	})
}
