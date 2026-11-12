package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

var ErrDuplicateISBN = errors.New("duplicate ISBN")

type TopBook struct {
	ISBN        string  `json:"isbn"`
	Title       string  `json:"title"`
	ImageURL    *string `json:"imageUrl"`
	VoteCount   int64   `json:"voteCount"`
	VoteAverage float64 `json:"voteAverage"`
}
type Stats struct {
	Books         int64     `json:"books"`
	Members       int64     `json:"members"`
	Admins        int64     `json:"admins"`
	Votes         int64     `json:"votes"`
	WishlistItems int64     `json:"wishlistItems"`
	NewestMembers []Member  `json:"newestMembers"`
	TopVoted      []TopBook `json:"topVoted"`
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM books),(SELECT count(*) FROM members),(SELECT count(*) FROM members WHERE role='admin'),(SELECT count(*) FROM votes),(SELECT count(*) FROM wishlist_items)`).Scan(&st.Books, &st.Members, &st.Admins, &st.Votes, &st.WishlistItems)
	if err != nil {
		return st, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,email,display_name,role,created_at FROM members ORDER BY created_at DESC,id DESC LIMIT 5`)
	if err != nil {
		return st, err
	}
	st.NewestMembers = []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role, &m.CreatedAt); err != nil {
			rows.Close()
			return st, err
		}
		st.NewestMembers = append(st.NewestMembers, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return st, err
	}
	rows, err = s.Pool.Query(ctx, `SELECT b.isbn,b.title,b.image_url_l,vs.vote_count,vs.vote_average::float8 FROM book_vote_stats vs JOIN books b ON b.isbn=vs.book_isbn ORDER BY vs.vote_average DESC,vs.vote_count DESC,b.title LIMIT 5`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	st.TopVoted = []TopBook{}
	for rows.Next() {
		var b TopBook
		if err := rows.Scan(&b.ISBN, &b.Title, &b.ImageURL, &b.VoteCount, &b.VoteAverage); err != nil {
			return st, err
		}
		st.TopVoted = append(st.TopVoted, b)
	}
	return st, rows.Err()
}

type BookInput struct {
	Title     string  `json:"title"`
	Author    *string `json:"author"`
	Year      *string `json:"year"`
	Publisher *string `json:"publisher"`
	ImageURLS *string `json:"imageUrlS"`
	ImageURLM *string `json:"imageUrlM"`
	ImageURL  *string `json:"imageUrl"`
}
type Impact struct {
	Reviews  int64 `json:"reviews"`
	Votes    int64 `json:"votes"`
	Wishlist int64 `json:"wishlist"`
}
type AdminBook struct {
	ISBN string `json:"isbn"`
	BookInput
	Rating    float64   `json:"rating"`
	UpdatedAt time.Time `json:"updatedAt"`
	Impact    Impact    `json:"impact"`
}

func (s *Store) AdminGetBook(ctx context.Context, isbn string) (AdminBook, error) {
	var b AdminBook
	err := s.Pool.QueryRow(ctx, `SELECT b.isbn,b.title,b.author,b.publication_year,b.publisher,b.image_url_s,b.image_url_m,b.image_url_l,coalesce(rs.rating_average,0)::float8,b.updated_at,(SELECT count(*) FROM reviews WHERE book_isbn=b.isbn),(SELECT count(*) FROM votes WHERE book_isbn=b.isbn),(SELECT count(*) FROM wishlist_items WHERE book_isbn=b.isbn) FROM books b LEFT JOIN dataset_book_rating_stats rs ON rs.isbn=b.isbn WHERE b.isbn=$1`, isbn).Scan(&b.ISBN, &b.Title, &b.Author, &b.Year, &b.Publisher, &b.ImageURLS, &b.ImageURLM, &b.ImageURL, &b.Rating, &b.UpdatedAt, &b.Impact.Reviews, &b.Impact.Votes, &b.Impact.Wishlist)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}
func (s *Store) CreateBook(ctx context.Context, isbn string, in BookInput) (AdminBook, error) {
	_, err := s.Pool.Exec(ctx, `INSERT INTO books(isbn,title,author,publication_year,publisher,image_url_s,image_url_m,image_url_l) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, isbn, in.Title, in.Author, in.Year, in.Publisher, in.ImageURLS, in.ImageURLM, in.ImageURL)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return AdminBook{}, ErrDuplicateISBN
	}
	if err != nil {
		return AdminBook{}, err
	}
	return s.AdminGetBook(ctx, isbn)
}
func (s *Store) UpdateBook(ctx context.Context, isbn string, in BookInput) (AdminBook, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE books SET title=$2,author=$3,publication_year=$4,publisher=$5,image_url_s=$6,image_url_m=$7,image_url_l=$8,updated_at=now() WHERE isbn=$1`, isbn, in.Title, in.Author, in.Year, in.Publisher, in.ImageURLS, in.ImageURLM, in.ImageURL)
	if err != nil {
		return AdminBook{}, err
	}
	if tag.RowsAffected() == 0 {
		return AdminBook{}, ErrNotFound
	}
	return s.AdminGetBook(ctx, isbn)
}
func (s *Store) DeleteBook(ctx context.Context, isbn string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM books WHERE isbn=$1`, isbn)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
