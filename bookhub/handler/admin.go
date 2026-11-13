
package handler

import (
	"bookhub/mw"
	"bookhub/store"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

func (h *Handler) AdminStats(w http.ResponseWriter, r *http.Request) {
	st, err := h.Store.Stats(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) AdminMembers(w http.ResponseWriter, r *http.Request) {
	items, err := h.Store.ListMembers(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type roleReq struct {
	Role string `json:"role"`
}

func (h *Handler) AdminSetRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "BAD_ID", "เลขสมาชิกไม่ถูกต้อง")
		return
	}

	var req roleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "BAD_JSON", "ข้อมูล JSON ไม่ถูกต้อง")
		return
	}

	if req.Role != "member" && req.Role != "admin" {
		writeError(w, 400, "BAD_ROLE", "role ต้องเป็น member หรือ admin")
		return
	}

	if me, _ := mw.MemberID(r.Context()); me == id {
		writeError(w, 400, "SELF_ROLE", "ไม่สามารถเปลี่ยนบทบาทของตัวเองได้")
		return
	}

	m, err := h.Store.UpdateMemberRole(r.Context(), id, req.Role)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "MEMBER_NOT_FOUND", "ไม่พบสมาชิก")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, 200, m)
}

func cleanPtr(p *string) *string {
	if p == nil {
		return nil
	}

	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}

	return &v
}

func validateBook(in *store.BookInput) (string, string) {
	in.Title = strings.TrimSpace(in.Title)
	in.Author = cleanPtr(in.Author)
	in.Year = cleanPtr(in.Year)
	in.Publisher = cleanPtr(in.Publisher)
	in.ImageURLS = cleanPtr(in.ImageURLS)
	in.ImageURLM = cleanPtr(in.ImageURLM)
	in.ImageURL = cleanPtr(in.ImageURL)
	in.Description = strings.TrimSpace(in.Description)

	if in.Title == "" || utf8.RuneCountInString(in.Title) > 500 {
		return "BAD_TITLE", "ต้องระบุชื่อหนังสือไม่เกิน 500 ตัวอักษร"
	}

	if utf8.RuneCountInString(in.Description) > 10000 {
		return "BAD_DESCRIPTION", "เรื่องย่อต้องไม่เกิน 10000 ตัวอักษร"
	}

	for _, u := range []*string{in.ImageURLS, in.ImageURLM, in.ImageURL} {
		if u != nil &&
			!strings.HasPrefix(strings.ToLower(*u), "http://") &&
			!strings.HasPrefix(strings.ToLower(*u), "https://") {
			return "BAD_IMAGE_URL", "URL รูปภาพต้องขึ้นต้นด้วย http:// หรือ https://"
		}
	}

	return "", ""
}

func AdminBookID(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

// รองรับทั้ง ISBN จริง และรหัสภายในของ Moony Books
func validBookID(id string) bool {
	id = strings.ToUpper(strings.TrimSpace(id))

	if validISBN(id) {
		return true
	}

	// รูปแบบรหัสหนังสือภายใน เช่น MOONY0001
	if !strings.HasPrefix(id, "MOONY") {
		return false
	}

	number := strings.TrimPrefix(id, "MOONY")

	if len(number) < 4 || len(id) > 20 {
		return false
	}

	for _, c := range number {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}

func (h *Handler) AdminGetBook(w http.ResponseWriter, r *http.Request) {
	isbn, ok := pathISBN(r)
	if !ok {
		writeError(w, 400, "BAD_ISBN", "รหัสหนังสือไม่ถูกต้อง")
		return
	}

	b, err := h.Store.AdminGetBook(r.Context(), isbn)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "BOOK_NOT_FOUND", "ไม่พบหนังสือ")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, 200, b)
}

type bookReq struct {
	ISBN string `json:"isbn"`
	store.BookInput
}

func (h *Handler) AdminCreateBook(w http.ResponseWriter, r *http.Request) {
	var req bookReq

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "BAD_JSON", "ข้อมูล JSON ไม่ถูกต้อง")
		return
	}

	req.ISBN = AdminBookID(req.ISBN)

	if !validBookID(req.ISBN) {
		writeError(w, 400, "BAD_ISBN", "รหัสหนังสือไม่ถูกต้อง กรุณาใช้ ISBN จริง หรือรหัส MOONY ตามด้วยตัวเลขอย่างน้อย 4 หลัก")
		return
	}

	if code, msg := validateBook(&req.BookInput); code != "" {
		writeError(w, 400, code, msg)
		return
	}

	b, err := h.Store.CreateBook(r.Context(), req.ISBN, req.BookInput)

	if errors.Is(err, store.ErrDuplicateISBN) {
		writeError(w, 409, "BOOK_EXISTS", "มีรหัสหนังสือนี้แล้ว กรุณาใช้รหัสอื่น")
		return
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		writeError(w, 409, "BOOK_EXISTS", "มีรหัสหนังสือนี้แล้ว กรุณาใช้รหัสอื่น")
		return
	}

	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, 201, b)
}

func (h *Handler) AdminUpdateBook(w http.ResponseWriter, r *http.Request) {
	isbn, ok := pathISBN(r)
	if !ok {
		writeError(w, 400, "BAD_ISBN", "รหัสหนังสือไม่ถูกต้อง")
		return
	}

	var req bookReq

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "BAD_JSON", "ข้อมูล JSON ไม่ถูกต้อง")
		return
	}

	if code, msg := validateBook(&req.BookInput); code != "" {
		writeError(w, 400, code, msg)
		return
	}

	b, err := h.Store.UpdateBook(r.Context(), isbn, req.BookInput)

	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "BOOK_NOT_FOUND", "ไม่พบหนังสือ")
		return
	}

	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, 200, b)
}

func (h *Handler) AdminDeleteBook(w http.ResponseWriter, r *http.Request) {
	isbn, ok := pathISBN(r)
	if !ok {
		writeError(w, 400, "BAD_ISBN", "รหัสหนังสือไม่ถูกต้อง")
		return
	}

	err := h.Store.DeleteBook(r.Context(), isbn)

	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "BOOK_NOT_FOUND", "ไม่พบหนังสือ")
		return
	}

	if err != nil {
		internal(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func validISBN(isbn string) bool {
	if len(isbn) < 10 || len(isbn) > 20 {
		return false
	}

	for _, c := range isbn {
		if !(c >= '0' && c <= '9') && c != 'X' {
			return false
		}
	}

	return true
}
