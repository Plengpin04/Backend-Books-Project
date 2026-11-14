package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// JournalEntry represents one private reading journal entry.
type JournalEntry struct {
	ID            int64           `json:"id"`
	BookISBN      *string         `json:"bookIsbn"`
	EntryDate     string          `json:"entryDate"`
	Title         string          `json:"title"`
	Feeling       string          `json:"feeling"`
	Content       string          `json:"content"`
	FavoriteQuote string          `json:"favoriteQuote"`
	Decorations   json.RawMessage `json:"decorations"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

func normalizeJournalBookISBN(isbn *string) *string {
	if isbn == nil {
		return nil
	}

	value := strings.ToUpper(strings.TrimSpace(*isbn))
	if value == "" {
		return nil
	}

	return &value
}

func normalizeJournalDecorations(data []byte) []byte {
	if len(data) == 0 {
		return []byte("[]")
	}

	return data
}

func scanJournalEntry(row pgx.Row) (JournalEntry, error) {
	var entry JournalEntry
	var bookISBN string
	var feeling string
	var favoriteQuote string
	var decorations []byte

	err := row.Scan(
		&entry.ID,
		&bookISBN,
		&entry.EntryDate,
		&entry.Title,
		&feeling,
		&entry.Content,
		&favoriteQuote,
		&decorations,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)
	if err != nil {
		return entry, err
	}

	if bookISBN != "" {
		entry.BookISBN = &bookISBN
	}

	entry.Feeling = feeling
	entry.FavoriteQuote = favoriteQuote

	if len(decorations) == 0 {
		decorations = []byte("[]")
	}

	entry.Decorations = json.RawMessage(decorations)

	return entry, nil
}

const journalEntryColumns = `
	id,
	COALESCE(book_isbn, ''),
	to_char(entry_date, 'YYYY-MM-DD'),
	title,
	COALESCE(feeling, ''),
	content,
	COALESCE(favorite_quote, ''),
	decorations,
	created_at,
	updated_at
`

// ListJournalEntries returns entries belonging to one member.
func (s *Store) ListJournalEntries(
	ctx context.Context,
	memberID int64,
) ([]JournalEntry, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+journalEntryColumns+`
		FROM journal_entries
		WHERE member_id = $1
		ORDER BY entry_date DESC, id DESC
	`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []JournalEntry{}

	for rows.Next() {
		var entry JournalEntry
		var bookISBN string
		var feeling string
		var favoriteQuote string
		var decorations []byte

		err := rows.Scan(
			&entry.ID,
			&bookISBN,
			&entry.EntryDate,
			&entry.Title,
			&feeling,
			&entry.Content,
			&favoriteQuote,
			&decorations,
			&entry.CreatedAt,
			&entry.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if bookISBN != "" {
			entry.BookISBN = &bookISBN
		}

		entry.Feeling = feeling
		entry.FavoriteQuote = favoriteQuote

		if len(decorations) == 0 {
			decorations = []byte("[]")
		}
		entry.Decorations = json.RawMessage(decorations)

		items = append(items, entry)
	}

	return items, rows.Err()
}

// GetJournalEntry returns one entry owned by the given member.
func (s *Store) GetJournalEntry(
	ctx context.Context,
	memberID int64,
	entryID int64,
) (JournalEntry, error) {
	entry, err := scanJournalEntry(s.Pool.QueryRow(ctx, `
		SELECT `+journalEntryColumns+`
		FROM journal_entries
		WHERE id = $1 AND member_id = $2
	`, entryID, memberID))

	if err == pgx.ErrNoRows {
		return JournalEntry{}, ErrNotFound
	}

	return entry, err
}

func (s *Store) validateJournalBook(
	ctx context.Context,
	isbn *string,
) error {
	if isbn == nil {
		return nil
	}

	var exists bool

	err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM books WHERE isbn = $1
		)
	`, *isbn).Scan(&exists)

	if err != nil {
		return err
	}

	if !exists {
		return ErrNotFound
	}

	return nil
}

// CreateJournalEntry creates a private entry for one member.
func (s *Store) CreateJournalEntry(
	ctx context.Context,
	memberID int64,
	bookISBN *string,
	entryDate string,
	title string,
	feeling string,
	content string,
	favoriteQuote string,
	decorations []byte,
) (JournalEntry, error) {
	bookISBN = normalizeJournalBookISBN(bookISBN)

	if err := s.validateJournalBook(ctx, bookISBN); err != nil {
		return JournalEntry{}, err
	}

	decorations = normalizeJournalDecorations(decorations)

	return scanJournalEntry(s.Pool.QueryRow(ctx, `
		INSERT INTO journal_entries (
			member_id,
			book_isbn,
			entry_date,
			title,
			feeling,
			content,
			favorite_quote,
			decorations
		)
		VALUES (
			$1,
			$2,
			CASE
				WHEN $3 = '' THEN CURRENT_DATE
				ELSE $3::date
			END,
			$4,
			NULLIF($5, ''),
			$6,
			NULLIF($7, ''),
			$8::jsonb
		)
		RETURNING `+journalEntryColumns,
		memberID,
		bookISBN,
		entryDate,
		title,
		feeling,
		content,
		favoriteQuote,
		decorations,
	))
}

// UpdateJournalEntry updates an entry only if it belongs to the member.
func (s *Store) UpdateJournalEntry(
	ctx context.Context,
	memberID int64,
	entryID int64,
	bookISBN *string,
	entryDate string,
	title string,
	feeling string,
	content string,
	favoriteQuote string,
	decorations []byte,
) (JournalEntry, error) {
	bookISBN = normalizeJournalBookISBN(bookISBN)

	if err := s.validateJournalBook(ctx, bookISBN); err != nil {
		return JournalEntry{}, err
	}

	decorations = normalizeJournalDecorations(decorations)

	entry, err := scanJournalEntry(s.Pool.QueryRow(ctx, `
		UPDATE journal_entries
		SET
			book_isbn = $3,
			entry_date = CASE
				WHEN $4 = '' THEN entry_date
				ELSE $4::date
			END,
			title = $5,
			feeling = NULLIF($6, ''),
			content = $7,
			favorite_quote = NULLIF($8, ''),
			decorations = $9::jsonb,
			updated_at = now()
		WHERE id = $1 AND member_id = $2
		RETURNING `+journalEntryColumns,
		entryID,
		memberID,
		bookISBN,
		entryDate,
		title,
		feeling,
		content,
		favoriteQuote,
		decorations,
	))

	if err == pgx.ErrNoRows {
		return JournalEntry{}, ErrNotFound
	}

	return entry, err
}

// DeleteJournalEntry deletes an entry only if it belongs to the member.
func (s *Store) DeleteJournalEntry(
	ctx context.Context,
	memberID int64,
	entryID int64,
) error {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM journal_entries
		WHERE id = $1 AND member_id = $2
	`, entryID, memberID)

	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
