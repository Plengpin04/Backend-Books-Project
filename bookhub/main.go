
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/joho/godotenv/autoload"

	"bookhub/auth"
	"bookhub/handler"
	"bookhub/mw"
	"bookhub/store"
)

func main() {
	secret := os.Getenv("JWT_SECRET")

	if len(secret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters")
	}

	databaseURL := os.Getenv("DATABASE_URL")

	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)

	if err != nil {
		log.Fatal(err)
	}

	defer pool.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal("database connection failed: ", err)
	}

	tickets := &auth.Issuer{
		Secret: []byte(secret),
		TTL:    time.Hour,
	}

	h := &handler.Handler{
		Store:   &store.Store{Pool: pool},
		Tickets: tickets,
	}

	login := func(next http.HandlerFunc) http.HandlerFunc {
		return mw.RequireLogin(tickets, next)
	}

	admin := func(next http.HandlerFunc) http.HandlerFunc {
		return login(mw.RequireRole("admin", next))
	}

	mux := http.NewServeMux()

	// Authentication
	mux.HandleFunc("POST /api/auth/register", h.Register)
	mux.HandleFunc("POST /api/auth/login", h.Login)

	// Books
	mux.HandleFunc("GET /api/books", h.SearchBooks)
	mux.HandleFunc("GET /api/books/{isbn}", h.GetBook)

	// Reviews
	mux.HandleFunc("GET /api/books/{isbn}/reviews", h.ListReviews)
	mux.HandleFunc(
		"POST /api/books/{isbn}/reviews",
		login(h.CreateReview),
	)

	// Admin
	mux.HandleFunc("GET /api/admin/stats", admin(h.AdminStats))
	mux.HandleFunc("GET /api/admin/members", admin(h.AdminMembers))
	mux.HandleFunc("PUT /api/admin/members/{id}/role", admin(h.AdminSetRole))
	mux.HandleFunc("GET /api/admin/books/{isbn}", admin(h.AdminGetBook))
	mux.HandleFunc("POST /api/admin/books", admin(h.AdminCreateBook))
	mux.HandleFunc("PUT /api/admin/books/{isbn}", admin(h.AdminUpdateBook))
	mux.HandleFunc("DELETE /api/admin/books/{isbn}", admin(h.AdminDeleteBook))

	origins := strings.Split(
		os.Getenv("ALLOWED_ORIGINS"),
		",",
	)

	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	log.Println("BookHub API listening on :" + port)

	log.Fatal(
		http.ListenAndServe(
			":"+port,
			mw.Logging(mw.CORS(origins, mux)),
		),
	)
}
