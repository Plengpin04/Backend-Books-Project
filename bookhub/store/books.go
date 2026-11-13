package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type Book struct {
	ISBN        string  `json:"isbn"`
	Title       string  `json:"title"`
	Author      *string `json:"author"`
	Year        *string `json:"year"`
	Publisher   *string `json:"publisher"`
	ImageURLS   *string `json:"imageUrlS"`
	ImageURLM   *string `json:"imageUrlM"`
	ImageURL    *string `json:"imageUrl"`
	Rating      float64 `json:"rating"`
	RatingCount int64   `json:"ratingCount"`
	Description string  `json:"description"`
}

type BookDetail struct {
	Book
	DescriptionSource string `json:"descriptionSource,omitempty"`
	DescriptionURL    string `json:"descriptionUrl,omitempty"`
}

const bookSelect = `SELECT b.isbn, b.title, b.author, b.publication_year, b.publisher,
		b.image_url_s, b.image_url_m, b.image_url_l,
		coalesce(rs.rating_average, 0)::float8,
		coalesce(rs.rating_count, 0),
		coalesce(b.description, '')
	FROM books b
	LEFT JOIN dataset_book_rating_stats rs ON rs.isbn = b.isbn `

func (s *Store) SearchBooks(ctx context.Context, q string) ([]Book, error) {
	rows, err := s.Pool.Query(ctx, bookSelect+`WHERE lower(b.title) LIKE '%' || lower($1) || '%'
		OR lower(coalesce(b.author, '')) LIKE '%' || lower($1) || '%'
		OR lower(coalesce(b.publisher, '')) LIKE '%' || lower($1) || '%'
		ORDER BY rs.rating_average DESC NULLS LAST, rs.rating_count DESC, b.title LIMIT 500`, q)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Book{}

	for rows.Next() {
		var b Book

		if err := rows.Scan(
			&b.ISBN,
			&b.Title,
			&b.Author,
			&b.Year,
			&b.Publisher,
			&b.ImageURLS,
			&b.ImageURLM,
			&b.ImageURL,
			&b.Rating,
			&b.RatingCount,
			&b.Description,
		); err != nil {
			return nil, err
		}

		out = append(out, b)
	}

	return out, rows.Err()
}

func (s *Store) GetBook(ctx context.Context, isbn string) (BookDetail, error) {
	var b BookDetail

	err := s.Pool.QueryRow(
		ctx,
		bookSelect+`WHERE b.isbn = $1`,
		isbn,
	).Scan(
		&b.ISBN,
		&b.Title,
		&b.Author,
		&b.Year,
		&b.Publisher,
		&b.ImageURLS,
		&b.ImageURLM,
		&b.ImageURL,
		&b.Rating,
		&b.RatingCount,
		&b.Description,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}

	return b, err
}
