package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// journalTestPool only permits the disposable local Docker test database.
// It never falls back to DATABASE_URL.
func journalTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))

	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping PostgreSQL integration test")
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("invalid TEST_DATABASE_URL: %v", err)
	}

	databaseName := strings.TrimPrefix(parsed.Path, "/")

	if parsed.Hostname() != "localhost" &&
		parsed.Hostname() != "127.0.0.1" {
		t.Fatalf("refusing non-local test database host: %q", parsed.Hostname())
	}

	if parsed.Port() != "55432" {
		t.Fatalf("refusing test database port %q; expected 55432", parsed.Port())
	}

	if databaseName != "bookhub_test" {
		t.Fatalf("refusing database %q; expected bookhub_test", databaseName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("connect to test database: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func createJournalTestMember(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	email string,
) int64 {
	t.Helper()

	var memberID int64

	err := pool.QueryRow(ctx, `
		INSERT INTO members (
			email,
			password_hash,
			display_name
		)
		VALUES ($1, $2, $3)
		RETURNING id
	`, email, "integration-test-hash", "Journal Test").Scan(&memberID)

	if err != nil {
		t.Fatalf("create test member %q: %v", email, err)
	}

	return memberID
}

func journalJSONEqual(t *testing.T, actual, expected []byte) {
	t.Helper()

	var actualValue any
	var expectedValue any

	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("decode actual JSON %q: %v", actual, err)
	}

	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		t.Fatalf("decode expected JSON %q: %v", expected, err)
	}

	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatalf(
			"JSON mismatch:\nactual:   %s\nexpected: %s",
			actual,
			expected,
		)
	}
}

func TestJournalStoreIntegration(t *testing.T) {
	pool := journalTestPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	s := &Store{Pool: pool}

	stamp := time.Now().UnixNano()

	memberA := createJournalTestMember(
		t,
		ctx,
		pool,
		fmt.Sprintf("journal-test-a-%d@example.test", stamp),
	)

	memberB := createJournalTestMember(
		t,
		ctx,
		pool,
		fmt.Sprintf("journal-test-b-%d@example.test", stamp),
	)

	bookISBN := "J" + strconv.FormatInt(stamp, 10)

	_, err := pool.Exec(ctx, `
		INSERT INTO books (isbn, title)
		VALUES ($1, $2)
	`, bookISBN, "Journal Integration Test Book")
	if err != nil {
		t.Fatalf("create test book: %v", err)
	}

	// Clean up only rows created by this test.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(
			cleanupCtx,
			`DELETE FROM members WHERE id = ANY($1)`,
			[]int64{memberA, memberB},
		)

		_, _ = pool.Exec(
			cleanupCtx,
			`DELETE FROM books WHERE isbn = $1`,
			bookISBN,
		)
	})

	t.Run("create and read entry", func(t *testing.T) {
		isbn := bookISBN
		decorations := []byte(`[{"type":"sticker","id":"flower"}]`)

		created, err := s.CreateJournalEntry(
			ctx,
			memberA,
			&isbn,
			"2026-11-14",
			"My first test entry",
			"Happy",
			"A lovely reading day",
			"Keep reading",
			decorations,
		)
		if err != nil {
			t.Fatalf("CreateJournalEntry: %v", err)
		}

		if created.ID <= 0 {
			t.Fatalf("expected generated entry ID, got %d", created.ID)
		}

		if created.BookISBN == nil || *created.BookISBN != bookISBN {
			t.Fatalf("unexpected book ISBN: %#v", created.BookISBN)
		}

		if created.EntryDate != "2026-11-14" {
			t.Fatalf("unexpected entry date: %q", created.EntryDate)
		}

		if created.Title != "My first test entry" {
			t.Fatalf("unexpected title: %q", created.Title)
		}

		journalJSONEqual(t, created.Decorations, decorations)

		fetched, err := s.GetJournalEntry(ctx, memberA, created.ID)
		if err != nil {
			t.Fatalf("GetJournalEntry: %v", err)
		}

		if fetched.ID != created.ID {
			t.Fatalf("expected entry ID %d, got %d", created.ID, fetched.ID)
		}

		if fetched.Content != "A lovely reading day" {
			t.Fatalf("unexpected content: %q", fetched.Content)
		}

		// Another member must not be able to read this entry.
		_, err = s.GetJournalEntry(ctx, memberB, created.ID)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound for another member, got %v", err)
		}

		itemsB, err := s.ListJournalEntries(ctx, memberB)
		if err != nil {
			t.Fatalf("ListJournalEntries for member B: %v", err)
		}

		if len(itemsB) != 0 {
			t.Fatalf("member B should see no entries, got %d", len(itemsB))
		}

		itemsA, err := s.ListJournalEntries(ctx, memberA)
		if err != nil {
			t.Fatalf("ListJournalEntries for member A: %v", err)
		}

		if len(itemsA) != 1 || itemsA[0].ID != created.ID {
			t.Fatalf("unexpected entries for member A: %#v", itemsA)
		}

		t.Run("another member cannot update or delete entry", func(t *testing.T) {
			_, err := s.UpdateJournalEntry(
				ctx,
				memberB,
				created.ID,
				nil,
				"",
				"Unauthorized edit",
				"",
				"",
				"",
				[]byte("[]"),
			)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("expected ErrNotFound on unauthorized update, got %v", err)
			}

			err = s.DeleteJournalEntry(ctx, memberB, created.ID)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("expected ErrNotFound on unauthorized delete, got %v", err)
			}

			after, err := s.GetJournalEntry(ctx, memberA, created.ID)
			if err != nil {
				t.Fatalf("owner should still be able to read entry: %v", err)
			}

			if after.Title != "My first test entry" {
				t.Fatalf("entry was unexpectedly modified: %q", after.Title)
			}
		})

		t.Run("owner can update and delete entry", func(t *testing.T) {
			updatedDecorations := []byte(`[{"type":"tape","color":"pink"}]`)

			updated, err := s.UpdateJournalEntry(
				ctx,
				memberA,
				created.ID,
				&isbn,
				"2026-11-13",
				"Updated journal title",
				"Calm",
				"Updated reading thoughts",
				"Another quote",
				updatedDecorations,
			)
			if err != nil {
				t.Fatalf("UpdateJournalEntry: %v", err)
			}

			if updated.Title != "Updated journal title" {
				t.Fatalf("unexpected updated title: %q", updated.Title)
			}

			if updated.EntryDate != "2026-11-13" {
				t.Fatalf("unexpected updated date: %q", updated.EntryDate)
			}

			journalJSONEqual(t, updated.Decorations, updatedDecorations)

			if err := s.DeleteJournalEntry(ctx, memberA, created.ID); err != nil {
				t.Fatalf("DeleteJournalEntry: %v", err)
			}

			_, err = s.GetJournalEntry(ctx, memberA, created.ID)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("expected ErrNotFound after deletion, got %v", err)
			}
		})
	})

	t.Run("settings are private to each member", func(t *testing.T) {
		defaultSettings, err := s.GetJournalSettings(ctx, memberA)
		if err != nil {
			t.Fatalf("GetJournalSettings for new member: %v", err)
		}

		journalJSONEqual(t, defaultSettings, []byte(`{}`))

		settings := []byte(`{
			"theme": "cozy",
			"paper": "cream",
			"accentColor": "#E8A6A1",
			"stickers": ["flower", "heart"]
		}`)

		saved, err := s.UpdateJournalSettings(ctx, memberA, settings)
		if err != nil {
			t.Fatalf("UpdateJournalSettings: %v", err)
		}

		journalJSONEqual(t, saved, settings)

		loaded, err := s.GetJournalSettings(ctx, memberA)
		if err != nil {
			t.Fatalf("GetJournalSettings after update: %v", err)
		}

		journalJSONEqual(t, loaded, settings)

		memberBSettings, err := s.GetJournalSettings(ctx, memberB)
		if err != nil {
			t.Fatalf("GetJournalSettings for member B: %v", err)
		}

		journalJSONEqual(t, memberBSettings, []byte(`{}`))

		settingsB := []byte(`{"theme":"minimal","paper":"white"}`)

		_, err = s.UpdateJournalSettings(ctx, memberB, settingsB)
		if err != nil {
			t.Fatalf("UpdateJournalSettings for member B: %v", err)
		}

		memberAAfterBUpdate, err := s.GetJournalSettings(ctx, memberA)
		if err != nil {
			t.Fatalf("reload member A settings: %v", err)
		}

		journalJSONEqual(t, memberAAfterBUpdate, settings)
	})
}
