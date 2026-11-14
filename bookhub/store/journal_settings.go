package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// GetJournalSettings returns the settings belonging to one member.
// Members without saved settings receive an empty JSON object.
func (s *Store) GetJournalSettings(
	ctx context.Context,
	memberID int64,
) (json.RawMessage, error) {
	var settings []byte

	err := s.Pool.QueryRow(ctx, `
		SELECT settings
		FROM journal_settings
		WHERE member_id = $1
	`, memberID).Scan(&settings)

	if err == pgx.ErrNoRows {
		return json.RawMessage(`{}`), nil
	}

	if err != nil {
		return nil, err
	}

	if len(settings) == 0 {
		return json.RawMessage(`{}`), nil
	}

	return json.RawMessage(settings), nil
}

// UpdateJournalSettings creates or replaces one member's settings.
func (s *Store) UpdateJournalSettings(
	ctx context.Context,
	memberID int64,
	settings json.RawMessage,
) (json.RawMessage, error) {
	var saved []byte

	err := s.Pool.QueryRow(ctx, `
		INSERT INTO journal_settings (
			member_id,
			settings,
			updated_at
		)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (member_id)
		DO UPDATE SET
			settings = EXCLUDED.settings,
			updated_at = now()
		RETURNING settings
	`, memberID, []byte(settings)).Scan(&saved)

	if err != nil {
		return nil, err
	}

	return json.RawMessage(saved), nil
}
