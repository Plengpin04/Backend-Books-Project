# BookHub Backend

Go REST API converted from the supplied MovieHub backend and adapted to the Kaggle Book Recommendation Dataset.

## Stack
- Go + `net/http`
- PostgreSQL (Supabase)
- `pgx`, JWT, bcrypt
- Render Web Service

## Dataset
Use `Books.csv` and `Ratings.csv` from the supplied Book Recommendation Dataset archive. `Users.csv` is not imported because the website's member accounts are managed by the existing registration/login system.

The supplied archive contains 271,360 books and 1,149,780 ratings. There are 118,644 rating rows whose ISBN is not present in `Books.csv`; `dataset_ratings` intentionally has no foreign key to `books` so these source rows are preserved. Rating `0` appears 716,109 times and represents an implicit interaction, not a zero-star review; it is excluded from `dataset_book_rating_stats` averages/counts. `Books.csv` has three malformed publication-year values (`DK Publishing Inc` and `Gallimard`), so `publication_year` is stored as text to preserve the original source values.

## Supabase setup
1. Create a Supabase PostgreSQL project.
2. Open SQL Editor and run `schema.sql`.
3. In Table Editor, import `Books.csv` into `books` and map:
   - `ISBN` -> `isbn`
   - `Book-Title` -> `title`
   - `Book-Author` -> `author`
   - `Year-Of-Publication` -> `publication_year` (text, to preserve the three malformed source values)
   - `Publisher` -> `publisher`
   - `Image-URL-S` -> `image_url_s`
   - `Image-URL-M` -> `image_url_m`
   - `Image-URL-L` -> `image_url_l`
4. Import `Ratings.csv` into `dataset_ratings` and map `User-ID` -> `user_id`, `ISBN` -> `isbn`, `Book-Rating` -> `rating`. Do not import an `id` column; it is generated.
5. `Users.csv` is not used by the backend.
6. Keep the Supabase database password and connection string private.

## Render setup
1. Create a new Web Service from this GitHub repository.
2. Select Go runtime; build command `go build -o bookhub .`; start command `./bookhub`.
3. Set `DATABASE_URL` to the Supabase PostgreSQL connection string (use the connection mode recommended by Supabase for external services, SSL enabled).
4. Set `JWT_SECRET` to a random secret of at least 32 characters.
5. Set `ALLOWED_ORIGINS` to the exact frontend origin(s), comma-separated. If no frontend origin is deployed yet, leave it empty; do not use `*` with authenticated requests.
6. Deploy and verify the service starts successfully in Render logs.

## Local run
```bash
cp .env.example .env
# edit DATABASE_URL and JWT_SECRET
 go run .
```

## API
- `POST /api/auth/register`
- `POST /api/auth/login`
- `GET /api/books?q=...`
- `GET /api/books/{isbn}`
- `GET /api/books/{isbn}/reviews`
- `GET /api/admin/stats` (admin token)
- `GET /api/admin/members` (admin token)
- `PUT /api/admin/members/{id}/role` (admin token)
- `GET /api/admin/books/{isbn}` (admin token)
- `POST /api/admin/books` (admin token)
- `PUT /api/admin/books/{isbn}` (admin token)
- `DELETE /api/admin/books/{isbn}` (admin token)

Book search returns up to 500 matching records per request. The API does not add recommendation algorithms or endpoints that were not present in the supplied backend.
