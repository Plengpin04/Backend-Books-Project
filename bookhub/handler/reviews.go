
package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"bookhub/mw"
	"bookhub/store"
)

type createReviewRequest struct {
	Text  string `json:"text"`
	Score int    `json:"score"`
}

func (h *Handler) CreateReview(w http.ResponseWriter, r *http.Request) {
	memberID, ok := mw.MemberID(r.Context())

	if !ok {
		writeError(
			w,
			http.StatusUnauthorized,
			"NO_TOKEN",
			"กรุณาเข้าสู่ระบบก่อนเขียนรีวิว",
		)
		return
	}

	isbn, ok := pathISBN(r)

	if !ok {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_ISBN",
			"รหัสหนังสือไม่ถูกต้อง",
		)
		return
	}

	var req createReviewRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_JSON",
			"ข้อมูล JSON ไม่ถูกต้อง",
		)
		return
	}

	req.Text = strings.TrimSpace(req.Text)

	textLength := utf8.RuneCountInString(req.Text)

	if textLength < 10 {
		writeError(
			w,
			http.StatusBadRequest,
			"REVIEW_TOO_SHORT",
			"รีวิวต้องมีอย่างน้อย 10 ตัวอักษร",
		)
		return
	}

	if textLength > 5000 {
		writeError(
			w,
			http.StatusBadRequest,
			"REVIEW_TOO_LONG",
			"รีวิวต้องไม่เกิน 5000 ตัวอักษร",
		)
		return
	}

	if req.Score < 1 || req.Score > 5 {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_SCORE",
			"คะแนนต้องอยู่ระหว่าง 1 ถึง 5 ดาว",
		)
		return
	}

	// แปลงคะแนน 1-5 ดาว เป็นคะแนน 2-10
	score := int16(req.Score * 2)

	review, err := h.Store.CreateReview(
		r.Context(),
		memberID,
		isbn,
		req.Text,
		score,
	)

	if err == store.ErrNotFound {
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

	writeJSON(w, http.StatusOK, review)
}
