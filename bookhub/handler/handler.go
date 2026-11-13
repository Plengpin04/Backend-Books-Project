package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"bookhub/auth"
	"bookhub/store"
)

type Handler struct {
	Store   *store.Store
	Tickets *auth.Issuer
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func internal(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)

	writeError(
		w,
		http.StatusInternalServerError,
		"INTERNAL",
		"ระบบขัดข้อง กรุณาลองใหม่",
	)
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)

	return id, err == nil && id > 0
}

func pathISBN(r *http.Request) (string, bool) {
	isbn := strings.TrimSpace(r.PathValue("isbn"))
	isbn = strings.ToUpper(isbn)

	if !validISBN(isbn) {
		return "", false
	}

	return isbn, true
}

func (h *Handler) SearchBooks(w http.ResponseWriter, r *http.Request) {
	items, err := h.Store.SearchBooks(
		r.Context(),
		r.URL.Query().Get("q"),
	)

	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *Handler) GetBook(w http.ResponseWriter, r *http.Request) {
	isbn, ok := pathISBN(r)

	if !ok {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_ISBN",
			"ISBN ไม่ถูกต้อง",
		)
		return
	}

	b, err := h.Store.GetBook(r.Context(), isbn)

	if errors.Is(err, store.ErrNotFound) {
		writeError(
			w,
			http.StatusNotFound,
			"BOOK_NOT_FOUND",
			"ไม่พบหนังสือเล่มนี้",
		)
		return
	}

	if err != nil {
		internal(w, err)
		return
	}

	// ถ้า Supabase มีเรื่องย่อที่เราใส่ไว้เองแล้ว
	// ให้ใช้ข้อมูลนั้นก่อน
	if strings.TrimSpace(b.Description) != "" {
		b.DescriptionSource = "Moony Books"
	} else {
		// ถ้ายังไม่มี ให้ลองค้นจาก API ภายนอก
		description, source, sourceURL := lookupBookSynopsis(
			r.Context(),
			isbn,
		)

		b.Description = description
		b.DescriptionSource = source
		b.DescriptionURL = sourceURL
	}

	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) ListReviews(w http.ResponseWriter, r *http.Request) {
	isbn, ok := pathISBN(r)

	if !ok {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_ISBN",
			"ISBN ไม่ถูกต้อง",
		)
		return
	}

	items, err := h.Store.ListReviews(r.Context(), isbn)

	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}
