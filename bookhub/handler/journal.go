package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"bookhub/mw"
	"bookhub/store"
)

const (
	maxJournalContent     = 20000
	maxJournalFeeling     = 2000
	maxJournalQuote       = 5000
	maxJournalDecorations = 1024 * 1024
)

type journalEntryRequest struct {
	BookISBN      *string         `json:"bookIsbn"`
	EntryDate     string          `json:"entryDate"`
	Title         string          `json:"title"`
	Feeling       string          `json:"feeling"`
	Content       string          `json:"content"`
	FavoriteQuote string          `json:"favoriteQuote"`
	Decorations   json.RawMessage `json:"decorations"`
}

func decodeJournalRequest(
	w http.ResponseWriter,
	r *http.Request,
) (journalEntryRequest, bool) {
	var req journalEntryRequest

	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_JSON",
			"ข้อมูล JSON ไม่ถูกต้อง",
		)
		return req, false
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_JSON",
			"กรุณาส่ง JSON เพียงหนึ่งชุด",
		)
		return req, false
	}

	return req, true
}

func validateJournalRequest(
	w http.ResponseWriter,
	req *journalEntryRequest,
) bool {
	req.Title = strings.TrimSpace(req.Title)
	req.Feeling = strings.TrimSpace(req.Feeling)
	req.Content = strings.TrimSpace(req.Content)
	req.FavoriteQuote = strings.TrimSpace(req.FavoriteQuote)
	req.EntryDate = strings.TrimSpace(req.EntryDate)

	if utf8.RuneCountInString(req.Title) > 150 {
		writeError(
			w,
			http.StatusBadRequest,
			"TITLE_TOO_LONG",
			"ชื่อบันทึกต้องไม่เกิน 150 ตัวอักษร",
		)
		return false
	}

	if utf8.RuneCountInString(req.Feeling) > maxJournalFeeling {
		writeError(
			w,
			http.StatusBadRequest,
			"FEELING_TOO_LONG",
			"ความรู้สึกต้องไม่เกิน 2000 ตัวอักษร",
		)
		return false
	}

	if utf8.RuneCountInString(req.Content) > maxJournalContent {
		writeError(
			w,
			http.StatusBadRequest,
			"CONTENT_TOO_LONG",
			"เนื้อหาต้องไม่เกิน 20000 ตัวอักษร",
		)
		return false
	}

	if utf8.RuneCountInString(req.FavoriteQuote) > maxJournalQuote {
		writeError(
			w,
			http.StatusBadRequest,
			"QUOTE_TOO_LONG",
			"คำคมต้องไม่เกิน 5000 ตัวอักษร",
		)
		return false
	}

	if req.EntryDate != "" {
		if _, err := time.Parse("2006-01-02", req.EntryDate); err != nil {
			writeError(
				w,
				http.StatusBadRequest,
				"BAD_DATE",
				"วันที่ต้องอยู่ในรูปแบบ YYYY-MM-DD",
			)
			return false
		}
	}

	if req.BookISBN != nil {
		isbn := strings.TrimSpace(*req.BookISBN)

		if len(isbn) > 20 {
			writeError(
				w,
				http.StatusBadRequest,
				"BAD_BOOK_ISBN",
				"รหัสหนังสือไม่ถูกต้อง",
			)
			return false
		}

		if isbn == "" {
			req.BookISBN = nil
		} else {
			isbn = strings.ToUpper(isbn)
			req.BookISBN = &isbn
		}
	}

	if len(req.Decorations) == 0 {
		req.Decorations = json.RawMessage("[]")
	}

	if len(req.Decorations) > maxJournalDecorations {
		writeError(
			w,
			http.StatusBadRequest,
			"DECORATIONS_TOO_LARGE",
			"ข้อมูลการตกแต่งมีขนาดใหญ่เกินไป",
		)
		return false
	}

	var decorations []json.RawMessage

	if err := json.Unmarshal(req.Decorations, &decorations); err != nil || decorations == nil {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_DECORATIONS",
			"ข้อมูลการตกแต่งต้องเป็น JSON Array",
		)
		return false
	}

	return true
}

func journalMemberID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	memberID, ok := mw.MemberID(r.Context())

	if !ok {
		writeError(
			w,
			http.StatusUnauthorized,
			"NO_TOKEN",
			"กรุณาเข้าสู่ระบบก่อนใช้งาน Reading Journal",
		)
		return 0, false
	}

	return memberID, true
}

func journalEntryID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil || id <= 0 {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_JOURNAL_ID",
			"รหัสบันทึกไม่ถูกต้อง",
		)
		return 0, false
	}

	return id, true
}

func (h *Handler) ListJournalEntries(w http.ResponseWriter, r *http.Request) {
	memberID, ok := journalMemberID(w, r)
	if !ok {
		return
	}

	items, err := h.Store.ListJournalEntries(r.Context(), memberID)
	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *Handler) CreateJournalEntry(w http.ResponseWriter, r *http.Request) {
	memberID, ok := journalMemberID(w, r)
	if !ok {
		return
	}

	req, ok := decodeJournalRequest(w, r)
	if !ok {
		return
	}

	if !validateJournalRequest(w, &req) {
		return
	}

	entry, err := h.Store.CreateJournalEntry(
		r.Context(),
		memberID,
		req.BookISBN,
		req.EntryDate,
		req.Title,
		req.Feeling,
		req.Content,
		req.FavoriteQuote,
		req.Decorations,
	)

	if errors.Is(err, store.ErrNotFound) {
		writeError(
			w,
			http.StatusNotFound,
			"BOOK_NOT_FOUND",
			"ไม่พบหนังสือที่เลือก",
		)
		return
	}

	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) GetJournalEntry(w http.ResponseWriter, r *http.Request) {
	memberID, ok := journalMemberID(w, r)
	if !ok {
		return
	}

	entryID, ok := journalEntryID(w, r)
	if !ok {
		return
	}

	entry, err := h.Store.GetJournalEntry(
		r.Context(),
		memberID,
		entryID,
	)

	if errors.Is(err, store.ErrNotFound) {
		writeError(
			w,
			http.StatusNotFound,
			"JOURNAL_NOT_FOUND",
			"ไม่พบบันทึกนี้",
		)
		return
	}

	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) UpdateJournalEntry(w http.ResponseWriter, r *http.Request) {
	memberID, ok := journalMemberID(w, r)
	if !ok {
		return
	}

	entryID, ok := journalEntryID(w, r)
	if !ok {
		return
	}

	req, ok := decodeJournalRequest(w, r)
	if !ok {
		return
	}

	if !validateJournalRequest(w, &req) {
		return
	}

	entry, err := h.Store.UpdateJournalEntry(
		r.Context(),
		memberID,
		entryID,
		req.BookISBN,
		req.EntryDate,
		req.Title,
		req.Feeling,
		req.Content,
		req.FavoriteQuote,
		req.Decorations,
	)

	if errors.Is(err, store.ErrNotFound) {
		writeError(
			w,
			http.StatusNotFound,
			"JOURNAL_NOT_FOUND",
			"ไม่พบบันทึกหรือหนังสือที่เลือก",
		)
		return
	}

	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) DeleteJournalEntry(w http.ResponseWriter, r *http.Request) {
	memberID, ok := journalMemberID(w, r)
	if !ok {
		return
	}

	entryID, ok := journalEntryID(w, r)
	if !ok {
		return
	}

	err := h.Store.DeleteJournalEntry(
		r.Context(),
		memberID,
		entryID,
	)

	if errors.Is(err, store.ErrNotFound) {
		writeError(
			w,
			http.StatusNotFound,
			"JOURNAL_NOT_FOUND",
			"ไม่พบบันทึกนี้",
		)
		return
	}

	if err != nil {
		internal(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
