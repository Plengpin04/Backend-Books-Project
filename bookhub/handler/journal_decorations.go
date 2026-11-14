package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
)

const (
	maxJournalDecorationItems = 50
	minJournalDecorationScale = 0.1
	maxJournalDecorationScale = 3.0
)

type journalDecorationRequest struct {
	Type      string   `json:"type"`
	StickerID string   `json:"stickerId"`
	X         *float64 `json:"x"`
	Y         *float64 `json:"y"`
	Rotation  *float64 `json:"rotation"`
	Scale     *float64 `json:"scale"`
	ZIndex    *int     `json:"zIndex"`
}

func isBuiltInStickerID(id string) bool {
	for _, sticker := range journalBuiltInStickers {
		if sticker.ID == id {
			return true
		}
	}
	return false
}

func validateJournalDecorations(
	w http.ResponseWriter,
	raw json.RawMessage,
) bool {
	var items []json.RawMessage

	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		writeError(w, http.StatusBadRequest, "BAD_DECORATIONS",
			"ข้อมูลการตกแต่งต้องเป็น JSON Array")
		return false
	}

	if len(items) > maxJournalDecorationItems {
		writeError(w, http.StatusBadRequest, "TOO_MANY_DECORATIONS",
			"ตกแต่งได้ไม่เกิน 50 ชิ้นต่อบันทึก")
		return false
	}

	for _, item := range items {
		var decoration journalDecorationRequest

		decoder := json.NewDecoder(bytes.NewReader(item))
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&decoration); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_DECORATION_ITEM",
				"ข้อมูลสติกเกอร์ในรายการไม่ถูกต้อง")
			return false
		}

		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			writeError(w, http.StatusBadRequest, "BAD_DECORATION_ITEM",
				"ข้อมูลสติกเกอร์ต้องมี JSON Object เพียงหนึ่งชุด")
			return false
		}

		if decoration.Type != "builtin" {
			writeError(w, http.StatusBadRequest, "BAD_DECORATION_TYPE",
				"รองรับเฉพาะสติกเกอร์สำเร็จรูปเท่านั้น")
			return false
		}

		if decoration.StickerID == "" ||
			!isBuiltInStickerID(decoration.StickerID) {
			writeError(w, http.StatusBadRequest, "UNKNOWN_STICKER",
				"ไม่พบสติกเกอร์สำเร็จรูปที่เลือก")
			return false
		}

		if decoration.X == nil || decoration.Y == nil ||
			decoration.Rotation == nil ||
			decoration.Scale == nil ||
			decoration.ZIndex == nil {
			writeError(w, http.StatusBadRequest, "INCOMPLETE_DECORATION",
				"ข้อมูลตำแหน่งและรูปแบบสติกเกอร์ไม่ครบถ้วน")
			return false
		}

		if math.IsNaN(*decoration.X) || math.IsInf(*decoration.X, 0) ||
			*decoration.X < 0 || *decoration.X > 100 {
			writeError(w, http.StatusBadRequest, "BAD_DECORATION_POSITION",
				"ตำแหน่ง X ต้องอยู่ระหว่าง 0 ถึง 100")
			return false
		}

		if math.IsNaN(*decoration.Y) || math.IsInf(*decoration.Y, 0) ||
			*decoration.Y < 0 || *decoration.Y > 100 {
			writeError(w, http.StatusBadRequest, "BAD_DECORATION_POSITION",
				"ตำแหน่ง Y ต้องอยู่ระหว่าง 0 ถึง 100")
			return false
		}

		if math.IsNaN(*decoration.Rotation) ||
			math.IsInf(*decoration.Rotation, 0) ||
			*decoration.Rotation < -360 ||
			*decoration.Rotation > 360 {
			writeError(w, http.StatusBadRequest, "BAD_DECORATION_ROTATION",
				"องศาการหมุนต้องอยู่ระหว่าง -360 ถึง 360")
			return false
		}

		if math.IsNaN(*decoration.Scale) ||
			math.IsInf(*decoration.Scale, 0) ||
			*decoration.Scale < minJournalDecorationScale ||
			*decoration.Scale > maxJournalDecorationScale {
			writeError(w, http.StatusBadRequest, "BAD_DECORATION_SCALE",
				"ขนาดสติกเกอร์ต้องอยู่ระหว่าง 0.1 ถึง 3")
			return false
		}

		if *decoration.ZIndex < 0 || *decoration.ZIndex > 1000 {
			writeError(w, http.StatusBadRequest, "BAD_DECORATION_Z_INDEX",
				"ลำดับชั้นสติกเกอร์ต้องอยู่ระหว่าง 0 ถึง 1000")
			return false
		}
	}

	return true
}
