package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"bookhub/mw"
)

const maxJournalSettingsSize = 64 * 1024

type journalSettingsRequest struct {
	Settings json.RawMessage `json:"settings"`
}

func decodeJournalSettingsRequest(
	w http.ResponseWriter,
	r *http.Request,
) (json.RawMessage, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJournalSettingsSize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var req journalSettingsRequest

	if err := decoder.Decode(&req); err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_JSON",
			"ข้อมูล JSON ไม่ถูกต้อง",
		)
		return nil, false
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_JSON",
			"กรุณาส่ง JSON เพียงหนึ่งชุด",
		)
		return nil, false
	}

	settings := bytes.TrimSpace(req.Settings)

	if len(settings) == 0 || !json.Valid(settings) {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_SETTINGS",
			"กรุณาระบุข้อมูล settings ที่ถูกต้อง",
		)
		return nil, false
	}

	var object map[string]json.RawMessage

	if err := json.Unmarshal(settings, &object); err != nil || object == nil {
		writeError(
			w,
			http.StatusBadRequest,
			"BAD_SETTINGS",
			"ข้อมูล settings ต้องเป็น JSON Object",
		)
		return nil, false
	}

	return json.RawMessage(settings), true
}

// GetJournalSettings returns the current member's journal customization.
func (h *Handler) GetJournalSettings(
	w http.ResponseWriter,
	r *http.Request,
) {
	memberID, ok := mw.MemberID(r.Context())
	if !ok {
		writeError(
			w,
			http.StatusUnauthorized,
			"NO_TOKEN",
			"กรุณาเข้าสู่ระบบก่อนใช้งาน Reading Journal",
		)
		return
	}

	settings, err := h.Store.GetJournalSettings(
		r.Context(),
		memberID,
	)
	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"settings": settings,
	})
}

// UpdateJournalSettings saves the current member's journal customization.
func (h *Handler) UpdateJournalSettings(
	w http.ResponseWriter,
	r *http.Request,
) {
	memberID, ok := mw.MemberID(r.Context())
	if !ok {
		writeError(
			w,
			http.StatusUnauthorized,
			"NO_TOKEN",
			"กรุณาเข้าสู่ระบบก่อนใช้งาน Reading Journal",
		)
		return
	}

	settings, ok := decodeJournalSettingsRequest(w, r)
	if !ok {
		return
	}

	saved, err := h.Store.UpdateJournalSettings(
		r.Context(),
		memberID,
		settings,
	)
	if err != nil {
		internal(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"settings": saved,
	})
}
