package handlers

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type transferImportRow struct {
	RowNumber  int               `json:"row_number"`
	Values     map[string]string `json:"values"`
	Existing   map[string]string `json:"existing,omitempty"`
	Status     string            `json:"status"`
	MatchField string            `json:"match_field,omitempty"`
	Errors     []string          `json:"errors,omitempty"`
}

type transferImportPreview struct {
	Dataset        string              `json:"dataset"`
	Filename       string              `json:"filename"`
	MappedFields   []string            `json:"mapped_fields"`
	UnknownColumns []string            `json:"unknown_columns"`
	Rows           []transferImportRow `json:"rows"`
	NewRows        int                 `json:"new_rows"`
	Conflicts      int                 `json:"conflicts"`
	InvalidRows    int                 `json:"invalid_rows"`
}

type transferImportApplyRequest struct {
	Dataset        string              `json:"dataset"`
	Rows           []transferImportRow `json:"rows"`
	ConflictPolicy string              `json:"conflict_policy"`
	FieldPolicies  map[string]string   `json:"field_policies"`
}

type transferImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type transferInputError struct {
	message string
}

func (err *transferInputError) Error() string {
	return err.message
}

type transferDB interface {
	transferQueryer
	QueryRow(query string, args ...any) *sql.Row
}

func PreviewDataTransferImport(w http.ResponseWriter, r *http.Request) {
	dataset, ok := transferDatasetByKey(strings.TrimSpace(r.URL.Query().Get("dataset")))
	if !ok || !dataset.ImportSupported {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Dataset does not support imports"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxTransferFileBytes+(1<<20))
	if err := r.ParseMultipartForm(maxTransferFileBytes); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Import file is too large or invalid"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Select a CSV or XLSX file"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTransferFileBytes+1))
	if err != nil || len(data) > maxTransferFileBytes {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Import file is too large or unreadable"})
		return
	}

	records, err := parseTransferFile(header, data, r.URL.Query().Get("delimiter"))
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	preview, err := buildTransferPreview(r.Context(), repository.GetSQLDB(), dataset, header.Filename, records)
	if err != nil {
		var inputError *transferInputError
		if errors.As(err, &inputError) {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": inputError.Error()})
			return
		}
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to analyze import data"})
		return
	}
	respondJSON(w, http.StatusOK, preview)
}

func ApplyDataTransferImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	var request transferImportApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid import request"})
		return
	}
	dataset, ok := transferDatasetByKey(strings.TrimSpace(request.Dataset))
	if !ok || !dataset.ImportSupported {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Dataset does not support imports"})
		return
	}
	if len(request.Rows) == 0 || len(request.Rows) > maxTransferRows {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Import must contain 1 to %d rows", maxTransferRows)})
		return
	}
	policy := strings.ToLower(strings.TrimSpace(request.ConflictPolicy))
	if policy == "" {
		policy = "skip"
	}
	if policy != "skip" && policy != "merge" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Unknown conflict policy"})
		return
	}
	for key, value := range request.FieldPolicies {
		field, exists := dataset.fieldByKey(key)
		if !exists || !field.Writable || !validTransferFieldPolicy(value) {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Invalid policy for field %s", key)})
			return
		}
	}

	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start import"})
		return
	}
	defer tx.Rollback()

	result := transferImportResult{}
	for index, row := range request.Rows {
		rowNumber := row.RowNumber
		if rowNumber == 0 {
			rowNumber = index + 2
		}
		existingID, existing, _, err := findTransferExisting(r.Context(), tx, dataset, row.Values)
		if err != nil {
			respondJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": fmt.Sprintf("Row %d: %v", rowNumber, err)})
			return
		}
		if existingID != "" {
			if policy == "skip" {
				result.Skipped++
				continue
			}
			changed, err := updateTransferRow(r.Context(), tx, dataset, existingID, existing, row.Values, request.FieldPolicies)
			if err != nil {
				respondJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": fmt.Sprintf("Row %d: %v", rowNumber, err)})
				return
			}
			if changed {
				result.Updated++
			} else {
				result.Skipped++
			}
			continue
		}
		if _, err := insertTransferRow(r.Context(), tx, dataset, row.Values); err != nil {
			respondJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": fmt.Sprintf("Row %d: %v", rowNumber, err)})
			return
		}
		result.Created++
	}
	if err := recordDataTransferAudit(tx, r, dataset.Key, policy, result); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to record import audit"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit import"})
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func recordDataTransferAudit(tx *sql.Tx, r *http.Request, dataset, conflictPolicy string, result transferImportResult) error {
	var userID any
	if user, ok := middleware.GetUserFromContext(r); ok {
		userID = user.UserID
	}
	details, _ := json.Marshal(map[string]any{
		"dataset": dataset, "conflict_policy": conflictPolicy,
		"created": result.Created, "updated": result.Updated, "skipped": result.Skipped,
	})
	ipAddress := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		ipAddress = host
	}
	if len(ipAddress) > 45 {
		ipAddress = ipAddress[:45]
	}
	_, err := tx.Exec(`
		INSERT INTO audit_log (user_id,action,entity_type,entity_id,old_values,new_values,ip_address,user_agent)
		VALUES ($1,'data_transfer.import','data_transfer',$2,NULL,$3::jsonb,$4,$5)
	`, userID, dataset, string(details), ipAddress, r.UserAgent())
	return err
}

func validTransferFieldPolicy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "incoming", "existing", "if_empty":
		return true
	default:
		return false
	}
}

func parseTransferFile(header *multipart.FileHeader, data []byte, delimiterName string) ([][]string, error) {
	extension := strings.ToLower(filepath.Ext(header.Filename))
	if extension == ".xlsx" || (len(data) >= 4 && bytes.Equal(data[:4], []byte{'P', 'K', 3, 4})) {
		return parseTransferXLSX(data)
	}
	return parseTransferCSV(data, delimiterName)
}

func parseTransferCSV(data []byte, delimiterName string) ([][]string, error) {
	delimiter := transferDelimiter(delimiterName)
	if strings.TrimSpace(delimiterName) == "" || strings.EqualFold(strings.TrimSpace(delimiterName), "auto") {
		delimiter = detectTransferDelimiter(data)
	}
	reader := csv.NewReader(bufio.NewReader(bytes.NewReader(data)))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV file: %w", err)
	}
	return validateTransferRecords(records)
}

func detectTransferDelimiter(data []byte) rune {
	line := data
	if index := bytes.IndexByte(line, '\n'); index >= 0 {
		line = line[:index]
	}
	counts := map[rune]int{';': 0, ',': 0, '\t': 0}
	quoted := false
	for _, character := range string(line) {
		if character == '"' {
			quoted = !quoted
			continue
		}
		if !quoted {
			if _, ok := counts[character]; ok {
				counts[character]++
			}
		}
	}
	best := ';'
	for _, candidate := range []rune{',', '\t'} {
		if counts[candidate] > counts[best] {
			best = candidate
		}
	}
	return best
}

func parseTransferXLSX(data []byte) ([][]string, error) {
	workbook, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    64 << 20,
		UnzipXMLSizeLimit: 16 << 20,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid XLSX file: %w", err)
	}
	defer workbook.Close()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("XLSX file does not contain a worksheet")
	}
	iterator, err := workbook.Rows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to read XLSX worksheet: %w", err)
	}
	defer iterator.Close()
	records := make([][]string, 0)
	for iterator.Next() {
		row, err := iterator.Columns()
		if err != nil {
			return nil, fmt.Errorf("failed to read XLSX row: %w", err)
		}
		records = append(records, row)
		if len(records) > maxTransferRows+1 {
			return nil, fmt.Errorf("import exceeds the limit of %d rows", maxTransferRows)
		}
	}
	if err := iterator.Error(); err != nil {
		return nil, fmt.Errorf("failed to read XLSX worksheet: %w", err)
	}
	return validateTransferRecords(records)
}

func validateTransferRecords(records [][]string) ([][]string, error) {
	if len(records) < 2 {
		return nil, errors.New("import requires a header and at least one data row")
	}
	if len(records)-1 > maxTransferRows {
		return nil, fmt.Errorf("import exceeds the limit of %d rows", maxTransferRows)
	}
	if len(records[0]) == 0 || len(records[0]) > maxTransferColumns {
		return nil, fmt.Errorf("import must contain 1 to %d columns", maxTransferColumns)
	}
	return records, nil
}

func buildTransferPreview(ctx context.Context, db transferDB, dataset *transferDataset, filename string, records [][]string) (transferImportPreview, error) {
	preview := transferImportPreview{Dataset: dataset.Key, Filename: filename, Rows: make([]transferImportRow, 0, len(records)-1)}
	mapped := make([]*transferField, len(records[0]))
	seen := make(map[string]bool)
	for index, header := range records[0] {
		field, ok := dataset.resolveHeader(header)
		if !ok {
			if strings.TrimSpace(header) != "" {
				preview.UnknownColumns = append(preview.UnknownColumns, strings.TrimSpace(header))
			}
			continue
		}
		if seen[field.Key] {
			return preview, &transferInputError{message: fmt.Sprintf("field %s is mapped more than once", field.Label)}
		}
		seen[field.Key] = true
		mapped[index] = field
		preview.MappedFields = append(preview.MappedFields, field.Key)
	}
	if len(preview.MappedFields) == 0 {
		return preview, &transferInputError{message: "no columns match this dataset"}
	}

	for index, record := range records[1:] {
		values := make(map[string]string)
		nonEmpty := false
		for column, field := range mapped {
			if field == nil {
				continue
			}
			value := ""
			if column < len(record) {
				value = strings.TrimSpace(record[column])
			}
			values[field.Key] = value
			if value != "" {
				nonEmpty = true
			}
		}
		if !nonEmpty {
			continue
		}
		row := transferImportRow{RowNumber: index + 2, Values: values, Status: "new"}
		existingID, existing, matchField, err := findTransferExisting(ctx, db, dataset, values)
		if err != nil {
			return preview, err
		}
		if existingID != "" {
			row.Status = "conflict"
			row.Existing = existing
			row.MatchField = matchField
		}
		row.Errors = validateTransferRow(ctx, db, dataset, values, existingID == "")
		if len(row.Errors) > 0 {
			row.Status = "invalid"
			preview.InvalidRows++
		} else if row.Status == "conflict" {
			preview.Conflicts++
		} else {
			preview.NewRows++
		}
		preview.Rows = append(preview.Rows, row)
	}
	if len(preview.Rows) == 0 {
		return preview, &transferInputError{message: "import does not contain any data rows"}
	}
	return preview, nil
}

func findTransferExisting(ctx context.Context, db transferDB, dataset *transferDataset, values map[string]string) (string, map[string]string, string, error) {
	for _, match := range dataset.Matches {
		value := strings.TrimSpace(values[match.Field])
		if value == "" {
			continue
		}
		condition := dataset.Alias + "." + quoteTransferIdentifier(match.Column) + "::text=$1"
		if !match.Exact {
			condition = "LOWER(TRIM(" + dataset.Alias + "." + quoteTransferIdentifier(match.Column) + "::text))=LOWER(TRIM($1))"
		}
		rows, err := queryTransferRows(db, dataset, dataset.Fields, condition, []any{value})
		if err != nil {
			return "", nil, "", err
		}
		if len(rows) == 0 {
			continue
		}
		existing := make(map[string]string, len(dataset.Fields))
		for index, field := range dataset.Fields {
			existing[field.Key] = rows[0][index]
		}
		var id string
		if err := db.QueryRow("SELECT "+quoteTransferIdentifier(dataset.PrimaryKey)+"::text FROM "+quoteTransferIdentifier(dataset.Table)+" WHERE "+quoteTransferIdentifier(match.Column)+"::text=$1", value).Scan(&id); err != nil {
			if match.Exact || !errors.Is(err, sql.ErrNoRows) {
				return "", nil, "", err
			}
			if err := db.QueryRow("SELECT "+quoteTransferIdentifier(dataset.PrimaryKey)+"::text FROM "+quoteTransferIdentifier(dataset.Table)+" WHERE LOWER(TRIM("+quoteTransferIdentifier(match.Column)+"::text))=LOWER(TRIM($1)) LIMIT 1", value).Scan(&id); err != nil {
				return "", nil, "", err
			}
		}
		return id, existing, match.Field, nil
	}
	return "", nil, "", nil
}

func validateTransferRow(ctx context.Context, db transferDB, dataset *transferDataset, values map[string]string, isNew bool) []string {
	errorsFound := make([]string, 0)
	for _, field := range dataset.Fields {
		value, present := values[field.Key]
		if isNew && field.Required && (!present || strings.TrimSpace(value) == "") {
			errorsFound = append(errorsFound, field.Label+" ist erforderlich")
			continue
		}
		if !present || strings.TrimSpace(value) == "" || !field.Writable {
			continue
		}
		if _, _, err := convertTransferValue(ctx, db, field, value, false); err != nil {
			errorsFound = append(errorsFound, field.Label+": "+err.Error())
		}
	}
	return errorsFound
}

func insertTransferRow(ctx context.Context, tx *sql.Tx, dataset *transferDataset, values map[string]string) (string, error) {
	if validationErrors := validateTransferRow(ctx, tx, dataset, values, true); len(validationErrors) > 0 {
		return "", errors.New(strings.Join(validationErrors, "; "))
	}
	columns := make([]string, 0)
	placeholders := make([]string, 0)
	arguments := make([]any, 0)
	for _, field := range dataset.Fields {
		raw, present := values[field.Key]
		if !present || !field.Writable {
			continue
		}
		value, include, err := convertTransferValue(ctx, tx, field, raw, true)
		if err != nil {
			return "", fmt.Errorf("%s: %w", field.Label, err)
		}
		if !include {
			continue
		}
		columns = append(columns, quoteTransferIdentifier(field.Column))
		arguments = append(arguments, value)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(arguments)))
	}
	if len(columns) == 0 {
		return "", errors.New("row does not contain writable values")
	}
	query := "INSERT INTO " + quoteTransferIdentifier(dataset.Table) + " (" + strings.Join(columns, ",") + ") VALUES (" + strings.Join(placeholders, ",") + ") RETURNING " + quoteTransferIdentifier(dataset.PrimaryKey) + "::text"
	var id string
	if err := tx.QueryRowContext(ctx, query, arguments...).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func updateTransferRow(ctx context.Context, tx *sql.Tx, dataset *transferDataset, id string, existing, values map[string]string, policies map[string]string) (bool, error) {
	sets := make([]string, 0)
	arguments := make([]any, 0)
	for _, field := range dataset.Fields {
		raw, present := values[field.Key]
		if !present || !field.Writable || field.Column == dataset.PrimaryKey {
			continue
		}
		policy := strings.ToLower(strings.TrimSpace(policies[field.Key]))
		if policy == "" {
			policy = "incoming"
		}
		if policy == "existing" || (policy == "if_empty" && strings.TrimSpace(existing[field.Key]) != "") {
			continue
		}
		value, include, err := convertTransferValue(ctx, tx, field, raw, false)
		if err != nil {
			return false, fmt.Errorf("%s: %w", field.Label, err)
		}
		if !include {
			continue
		}
		arguments = append(arguments, value)
		sets = append(sets, quoteTransferIdentifier(field.Column)+"=$"+strconv.Itoa(len(arguments)))
	}
	if len(sets) == 0 {
		return false, nil
	}
	arguments = append(arguments, id)
	query := "UPDATE " + quoteTransferIdentifier(dataset.Table) + " SET " + strings.Join(sets, ",") + " WHERE " + quoteTransferIdentifier(dataset.PrimaryKey) + "::text=$" + strconv.Itoa(len(arguments))
	if _, err := tx.ExecContext(ctx, query, arguments...); err != nil {
		return false, err
	}
	return true, nil
}

func convertTransferValue(ctx context.Context, db transferDB, field transferField, raw string, inserting bool) (any, bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		if field.Required && inserting {
			return nil, false, errors.New("value is required")
		}
		if field.Required {
			return nil, false, nil
		}
		if inserting {
			return nil, false, nil
		}
		return nil, true, nil
	}
	if field.Relation != nil {
		var id any
		query := "SELECT " + quoteTransferIdentifier(field.Relation.IDColumn) + " FROM " + quoteTransferIdentifier(field.Relation.Table) + " WHERE " + quoteTransferIdentifier(field.Relation.IDColumn) + "::text=$1 OR LOWER(TRIM(" + quoteTransferIdentifier(field.Relation.NameColumn) + "))=LOWER(TRIM($1)) ORDER BY CASE WHEN " + quoteTransferIdentifier(field.Relation.IDColumn) + "::text=$1 THEN 0 ELSE 1 END LIMIT 1"
		if err := db.QueryRow(query, value).Scan(&id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, false, fmt.Errorf("unbekannte Referenz %q", value)
			}
			return nil, false, err
		}
		return id, true, nil
	}
	switch field.Type {
	case "integer":
		parsed, err := strconv.ParseInt(strings.ReplaceAll(value, " ", ""), 10, 64)
		if err != nil {
			return nil, false, errors.New("Ganzzahl erwartet")
		}
		return parsed, true, nil
	case "decimal":
		parsed, err := strconv.ParseFloat(normalizeTransferNumber(value), 64)
		if err != nil {
			return nil, false, errors.New("Zahl erwartet")
		}
		return parsed, true, nil
	case "boolean":
		parsed, err := parseTransferBool(value)
		if err != nil {
			return nil, false, err
		}
		return parsed, true, nil
	case "date":
		parsed, err := parseTransferDate(value)
		if err != nil {
			return nil, false, err
		}
		return parsed, true, nil
	default:
		return value, true, nil
	}
}

func normalizeTransferNumber(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) || character == '\'' {
			return -1
		}
		return character
	}, value)
	comma := strings.LastIndex(value, ",")
	dot := strings.LastIndex(value, ".")
	if comma >= 0 && dot >= 0 {
		if comma > dot {
			value = strings.ReplaceAll(value, ".", "")
			value = strings.ReplaceAll(value, ",", ".")
		} else {
			value = strings.ReplaceAll(value, ",", "")
		}
	} else if comma >= 0 {
		value = strings.ReplaceAll(value, ",", ".")
	}
	return value
}

func parseTransferBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "ja", "y", "j":
		return true, nil
	case "false", "0", "no", "nein", "n":
		return false, nil
	default:
		return false, errors.New("Wahr/Falsch oder Ja/Nein erwartet")
	}
}

func parseTransferDate(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", "02.01.2006", "02/01/2006", time.RFC3339} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errors.New("Datum als YYYY-MM-DD oder DD.MM.YYYY erwartet")
}
