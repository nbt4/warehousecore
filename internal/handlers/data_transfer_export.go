package handlers

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"warehousecore/internal/repository"
)

type transferExportRequest struct {
	Dataset   string   `json:"dataset"`
	Fields    []string `json:"fields"`
	Format    string   `json:"format"`
	Delimiter string   `json:"delimiter"`
	Headers   string   `json:"headers"`
}

func GetDataTransferCatalog(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, transferCatalogResponse{
		Datasets:   transferDatasets,
		Formats:    []string{"csv", "xlsx"},
		Delimiters: []string{"semicolon", "comma", "tab"},
		MaxRows:    maxTransferRows,
	})
}

func ExportDataTransfer(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request transferExportRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid export request"})
		return
	}

	dataset, ok := transferDatasetByKey(strings.TrimSpace(request.Dataset))
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Unknown dataset"})
		return
	}
	fields, err := resolveTransferFields(dataset, request.Fields)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	rows, err := queryTransferRows(repository.GetSQLDB(), dataset, fields, "", nil)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to export dataset"})
		return
	}

	labels := request.Headers != "keys"
	format := strings.ToLower(strings.TrimSpace(request.Format))
	if format == "" {
		format = "csv"
	}
	var data []byte
	var contentType, extension string
	switch format {
	case "csv":
		data, err = encodeTransferCSV(fields, rows, transferDelimiter(request.Delimiter), labels)
		contentType = "text/csv; charset=utf-8"
		extension = "csv"
	case "xlsx":
		data, err = encodeTransferXLSX(dataset.Label, fields, rows, labels)
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		extension = "xlsx"
	default:
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported export format"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to encode export"})
		return
	}

	filename := fmt.Sprintf("%s_%s.%s", dataset.Key, time.Now().Format("2006-01-02"), extension)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func resolveTransferFields(dataset *transferDataset, keys []string) ([]transferField, error) {
	if len(keys) == 0 {
		fields := make([]transferField, 0, len(dataset.Fields))
		for _, field := range dataset.Fields {
			if field.DefaultSelected {
				fields = append(fields, field)
			}
		}
		if len(fields) == 0 {
			return append([]transferField(nil), dataset.Fields...), nil
		}
		return fields, nil
	}
	seen := make(map[string]bool, len(keys))
	fields := make([]transferField, 0, len(keys))
	for _, key := range keys {
		if seen[key] {
			continue
		}
		field, ok := dataset.fieldByKey(key)
		if !ok {
			return nil, fmt.Errorf("unknown field %q", key)
		}
		seen[key] = true
		fields = append(fields, *field)
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("select at least one field")
	}
	return fields, nil
}

type transferQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func queryTransferRows(db transferQueryer, dataset *transferDataset, fields []transferField, extraWhere string, args []any) ([][]string, error) {
	rows, err := db.Query(dataset.selectQuery(fields, extraWhere), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([][]string, 0)
	for rows.Next() {
		values := make([]sql.NullString, len(fields))
		destinations := make([]any, len(fields))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}
		row := make([]string, len(fields))
		for i, value := range values {
			if value.Valid {
				row[i] = value.String
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func transferDelimiter(name string) rune {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "comma", ",":
		return ','
	case "tab", "\\t":
		return '\t'
	default:
		return ';'
	}
}

func encodeTransferCSV(fields []transferField, rows [][]string, delimiter rune, labels bool) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.Write(utf8BOM)
	writer := csv.NewWriter(&buffer)
	writer.Comma = delimiter

	headers := make([]string, len(fields))
	for i, field := range fields {
		if labels {
			headers[i] = field.Label
		} else {
			headers[i] = field.Key
		}
	}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func encodeTransferXLSX(datasetLabel string, fields []transferField, rows [][]string, labels bool) ([]byte, error) {
	workbook := excelize.NewFile()
	defer workbook.Close()
	sheet := "Data"
	if err := workbook.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}

	headerStyle, err := workbook.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"374151"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}

	for column, field := range fields {
		cell, _ := excelize.CoordinatesToCellName(column+1, 1)
		header := field.Key
		if labels {
			header = field.Label
		}
		if err := workbook.SetCellValue(sheet, cell, header); err != nil {
			return nil, err
		}
		_ = workbook.SetCellStyle(sheet, cell, cell, headerStyle)
		width := float64(len([]rune(header)) + 4)
		if width < 14 {
			width = 14
		}
		if width > 36 {
			width = 36
		}
		columnName, _ := excelize.ColumnNumberToName(column + 1)
		_ = workbook.SetColWidth(sheet, columnName, columnName, width)
	}
	for rowIndex, row := range rows {
		for column, value := range row {
			cell, _ := excelize.CoordinatesToCellName(column+1, rowIndex+2)
			if err := workbook.SetCellValue(sheet, cell, value); err != nil {
				return nil, err
			}
		}
	}
	if len(fields) > 0 {
		lastColumn, _ := excelize.ColumnNumberToName(len(fields))
		_ = workbook.AutoFilter(sheet, "A1:"+lastColumn+"1", []excelize.AutoFilterOptions{})
		_ = workbook.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	}
	workbook.SetDocProps(&excelize.DocProperties{Title: datasetLabel, Creator: "Cores"})
	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
