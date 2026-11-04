package messages

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/aa228058/tgqf/backend/internal/auth"
)

func parseGroupCatalogueDelimited(reader io.Reader, delimiter rune) ([]GroupCatalogueImportRecord, error) {
	csvReader := csv.NewReader(reader)
	csvReader.Comma = delimiter
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true
	header, err := csvReader.Read()
	if err != nil { return nil, errors.New("CSV header is required") }
	columns := make(map[string]int, len(header))
	for i, name := range header {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" { continue }
		if _, exists := columns[key]; exists { return nil, errors.New("duplicate CSV column: " + key) }
		columns[key] = i
	}
	idColumn, ok := columns["telegram_chat_id"]
	if !ok { idColumn, ok = columns["chat_id"] }
	if !ok { return nil, errors.New("CSV requires telegram_chat_id or chat_id column") }
	typeColumn, typeOK := columns["chat_type"]
	titleColumn, titleOK := columns["title"]
	if !typeOK || !titleOK { return nil, errors.New("CSV requires chat_type and title columns") }
	records := make([]GroupCatalogueImportRecord, 0)
	for rowNo := 2; ; rowNo++ {
		row, readErr := csvReader.Read()
		if readErr == io.EOF { break }
		if readErr != nil { return nil, errors.New("invalid CSV data") }
		blank := true
		for _, cell := range row { if strings.TrimSpace(cell) != "" { blank = false; break } }
		if blank { continue }
		if len(records) >= 10000 { return nil, errors.New("CSV exceeds 10000 data rows") }
		if len(row) != len(header) { return nil, errors.New("CSV row " + strconv.Itoa(rowNo) + " has a column count mismatch") }
		get := func(index int) string { if index < 0 || index >= len(row) { return "" }; return strings.TrimSpace(row[index]) }
		chatID, parseErr := strconv.ParseInt(get(idColumn), 10, 64)
		if parseErr != nil || chatID == 0 { return nil, errors.New("CSV row " + strconv.Itoa(rowNo) + " has an invalid telegram_chat_id") }
		item := GroupCatalogueImportRecord{ChatID:chatID,ChatType:get(typeColumn),Title:get(titleColumn)}
		if index, exists := columns["username"]; exists { item.Username = get(index) }
		if index, exists := columns["member_count"]; exists && get(index) != "" {
			item.MemberCount, parseErr = strconv.Atoi(get(index))
			if parseErr != nil { return nil, errors.New("CSV row " + strconv.Itoa(rowNo) + " has an invalid member_count") }
		}
		if index, exists := columns["is_verified"]; exists && get(index) != "" {
			item.IsVerified, parseErr = strconv.ParseBool(get(index))
			if parseErr != nil { return nil, errors.New("CSV row " + strconv.Itoa(rowNo) + " has an invalid is_verified") }
		}
		if _, normalizeErr := normalizeGroupCatalogueImportRecord(item); normalizeErr != nil {
			return nil, errors.New("CSV row " + strconv.Itoa(rowNo) + ": " + normalizeErr.Error())
		}
		records = append(records,item)
	}
	if len(records) == 0 { return nil, errors.New("CSV contains no group records") }
	return records,nil
}

func parseGroupCatalogueCSV(reader io.Reader) ([]GroupCatalogueImportRecord, error) {
	return parseGroupCatalogueDelimited(reader, ',')
}

func parseGroupCatalogueTXT(reader io.Reader) ([]GroupCatalogueImportRecord, error) {
	return parseGroupCatalogueDelimited(reader, '\\t')
}

func (h *Handler) importGroupCatalogueTXT(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user.Role != "tenant_admin" {
		messageJSON(w,http.StatusForbidden,map[string]string{"error":"tenant_admin_required"})
		return
	}
	r.Body = http.MaxBytesReader(w,r.Body,2<<20)
	records, err := parseGroupCatalogueTXT(r.Body)
	if err != nil {
		messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()})
		return
	}
	inserted, skipped, err := h.service.ImportGroupCatalogue(r.Context(),user.TenantID,records)
	if err != nil {
		messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()})
		return
	}
	messageJSON(w,http.StatusOK,map[string]any{"total":len(records),"inserted":inserted,"skipped_existing":skipped,"mode":"insert_only","shared_catalogue":true})
}

func (h *Handler) importGroupCatalogueCSV(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user.Role != "tenant_admin" {
		messageJSON(w,http.StatusForbidden,map[string]string{"error":"tenant_admin_required"})
		return
	}
	r.Body = http.MaxBytesReader(w,r.Body,2<<20)
	records, err := parseGroupCatalogueCSV(r.Body)
	if err != nil {
		messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()})
		return
	}
	inserted, skipped, err := h.service.ImportGroupCatalogue(r.Context(),user.TenantID,records)
	if err != nil {
		messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()})
		return
	}
	messageJSON(w,http.StatusOK,map[string]any{
		"total":len(records),"inserted":inserted,"skipped_existing":skipped,
		"mode":"insert_only","shared_catalogue":true,
	})
}
