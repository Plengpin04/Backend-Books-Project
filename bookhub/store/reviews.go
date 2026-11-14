
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type ReviewAuthor struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"displayName"`
}

type Review struct {
	ID        int64        `json:"id"`
	MemberID  int64        `json:"memberId"`
	BookISBN  string       `json:"bookIsbn"`
	Text      string       `json:"text"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
	Member    ReviewAuthor `json:"member"`
	Score     *int16       `json:"score"`
}

// ListReviews ดึงรีวิวทั้งหมดของหนังสือ เรียงจากใหม่ไปเก่า
func (s *Store) ListReviews(
	ctx context.Context,
	isbn string,
) ([]Review, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT
			r.id,
			r.member_id,
			r.book_isbn,
			r.text,
			r.created_at,
			r.updated_at,
			m.display_name,
			v.score
		FROM reviews r
		JOIN members m ON m.id = r.member_id
		LEFT JOIN votes v
			ON v.member_id = r.member_id
			AND v.book_isbn = r.book_isbn
		WHERE r.book_isbn = $1
		ORDER BY r.created_at DESC
	`, isbn)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Review{}

	for rows.Next() {
		var review Review

		if err := rows.Scan(
			&review.ID,
			&review.MemberID,
			&review.BookISBN,
			&review.Text,
			&review.CreatedAt,
			&review.UpdatedAt,
			&review.Member.DisplayName,
			&review.Score,
		); err != nil {
			return nil, err
		}

		review.Member.ID = review.MemberID
		out = append(out, review)
	}

	return out, rows.Err()
}

// CreateReview สร้างหรือแก้ไขรีวิวเดิมของสมาชิก
// คะแนนจากหน้าเว็บ 1-5 ดาว ถูกแปลงเป็นคะแนน 2-10 ในตาราง votes
func (s *Store) CreateReview(
	ctx context.Context,
	memberID int64,
	isbn string,
	reviewText string,
	score int16,
) (Review, error) {
	var review Review

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return review, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// ตรวจสอบว่ามีหนังสือเล่มนี้จริง
	var exists int

	err = tx.QueryRow(ctx, `
		SELECT 1
		FROM books
		WHERE isbn = $1
	`, isbn).Scan(&exists)

	if errors.Is(err, pgx.ErrNoRows) {
		return review, ErrNotFound
	}

	if err != nil {
		return review, err
	}

	// บันทึกรีวิว ถ้ามีรีวิวเดิมให้แก้ไขรายการเดิม
	err = tx.QueryRow(ctx, `
		INSERT INTO reviews (
			member_id,
			book_isbn,
			text
		)
		VALUES ($1, $2, $3)
		ON CONFLICT (member_id, book_isbn)
		DO UPDATE SET
			text = EXCLUDED.text,
			updated_at = now()
		RETURNING id
	`, memberID, isbn, reviewText).Scan(&review.ID)

	if err != nil {
		return review, err
	}

	// บันทึกคะแนน 1-10 โดยแปลงจากคะแนน 1-5 ดาว
	err = tx.QueryRow(ctx, `
		INSERT INTO votes (
			member_id,
			book_isbn,
			score
		)
		VALUES ($1, $2, $3)
		ON CONFLICT (member_id, book_isbn)
		DO UPDATE SET
			score = EXCLUDED.score,
			updated_at = now()
		RETURNING score
	`, memberID, isbn, score).Scan(&review.Score)

	if err != nil {
		return review, err
	}

	// ดึงข้อมูลรีวิวพร้อมชื่อสมาชิกกลับไปให้ Frontend
	err = tx.QueryRow(ctx, `
		SELECT
			r.id,
			r.member_id,
			r.book_isbn,
			r.text,
			r.created_at,
			r.updated_at,
			m.display_name
		FROM reviews r
		JOIN members m ON m.id = r.member_id
		WHERE r.id = $1
	`, review.ID).Scan(
		&review.ID,
		&review.MemberID,
		&review.BookISBN,
		&review.Text,
		&review.CreatedAt,
		&review.UpdatedAt,
		&review.Member.DisplayName,
	)

	if err != nil {
		return review, err
	}

	review.Member.ID = review.MemberID

	if err := tx.Commit(ctx); err != nil {
		return review, err
	}

	return review, nil
}
