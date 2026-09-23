package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"warehousecore/internal/repository"
)

func TestNormalizeTransferHeaderMatchesKeysAndLabels(t *testing.T) {
	dataset, ok := transferDatasetByKey("products")
	if !ok {
		t.Fatal("products dataset missing")
	}
	for _, header := range []string{"product_code", "Produktcode", "\ufeffProduktcode"} {
		field, found := dataset.resolveHeader(header)
		if !found || field.Key != "product_code" {
			t.Fatalf("header %q resolved to %#v, found=%v", header, field, found)
		}
	}
	field, found := dataset.resolveHeader("Höhe (cm)")
	if !found || field.Key != "height" {
		t.Fatalf("localized header resolved to %#v, found=%v", field, found)
	}
}

func TestParseTransferCSVDetectsDelimiterAndQuotes(t *testing.T) {
	records, err := parseTransferCSV([]byte("name;description\nMixer;\"Line 1; Line 2\"\n"), "auto")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"name", "description"}, {"Mixer", "Line 1; Line 2"}}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v, want %#v", records, want)
	}

	records, err = parseTransferCSV([]byte("name,website\nAcme,https://example.com\n"), "auto")
	if err != nil {
		t.Fatal(err)
	}
	if got := records[1][1]; got != "https://example.com" {
		t.Fatalf("comma separated value = %q", got)
	}
}

func TestTransferXLSXRoundTrip(t *testing.T) {
	fields := []transferField{
		{Key: "name", Label: "Name"},
		{Key: "website", Label: "Webseite"},
	}
	rows := [][]string{{"Acme", "https://example.com"}, {"Quoted; value", ""}}
	data, err := encodeTransferXLSX("Hersteller", fields, rows, false)
	if err != nil {
		t.Fatal(err)
	}
	records, err := parseTransferFile(&multipart.FileHeader{Filename: "manufacturers.xlsx"}, data, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || !reflect.DeepEqual(records[0], []string{"name", "website"}) ||
		!reflect.DeepEqual(records[1], rows[0]) || len(records[2]) != 1 || records[2][0] != rows[1][0] {
		t.Fatalf("records = %#v", records)
	}
}

func TestTransferValueParsers(t *testing.T) {
	for input, want := range map[string]string{
		"1.234,56": "1234.56",
		"1,234.56": "1234.56",
		"12,5":     "12.5",
	} {
		if got := normalizeTransferNumber(input); got != want {
			t.Errorf("normalizeTransferNumber(%q) = %q, want %q", input, got, want)
		}
	}
	if value, err := parseTransferBool("Ja"); err != nil || !value {
		t.Fatalf("parseTransferBool(Ja) = %v, %v", value, err)
	}
	if value, err := parseTransferBool("no"); err != nil || value {
		t.Fatalf("parseTransferBool(no) = %v, %v", value, err)
	}
	parsed, err := parseTransferDate("22.09.2026")
	if err != nil || parsed.Format(time.DateOnly) != "2026-09-22" {
		t.Fatalf("parseTransferDate = %v, %v", parsed, err)
	}
}

func TestResolveTransferFieldsRejectsUnknownField(t *testing.T) {
	dataset, _ := transferDatasetByKey("products")
	if _, err := resolveTransferFields(dataset, []string{"name", "drop_table"}); err == nil {
		t.Fatal("expected unknown field error")
	}
	fields, err := resolveTransferFields(dataset, []string{"name", "name", "barcode"})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{fields[0].Key, fields[1].Key}; !reflect.DeepEqual(got, []string{"name", "barcode"}) {
		t.Fatalf("resolved fields = %#v", got)
	}
}

func TestDataTransferCatalogAndCSVExportHandlers(t *testing.T) {
	catalogResponse := httptest.NewRecorder()
	GetDataTransferCatalog(catalogResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if catalogResponse.Code != http.StatusOK {
		t.Fatalf("catalog status = %d", catalogResponse.Code)
	}
	var catalog transferCatalogResponse
	if err := json.NewDecoder(catalogResponse.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Datasets) < 9 || catalog.MaxRows != maxTransferRows {
		t.Fatalf("unexpected catalog: datasets=%d maxRows=%d", len(catalog.Datasets), catalog.MaxRows)
	}

	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previousDB := repository.DB
	repository.DB = database
	defer func() { repository.DB = previousDB }()
	mock.ExpectQuery(`SELECT m.name AS "name", COALESCE(m.website,'') AS "website" FROM manufacturer m ORDER BY m.name`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "website"}).AddRow("Acme", "https://example.com"))

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"dataset":"manufacturers","fields":["name","website"],"format":"csv","delimiter":"comma","headers":"keys"}`))
	response := httptest.NewRecorder()
	ExportDataTransfer(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("export status = %d, body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	payload := bytes.TrimPrefix(response.Body.Bytes(), utf8BOM)
	reader := csv.NewReader(bytes.NewReader(payload))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"name", "website"}, {"Acme", "https://example.com"}}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v, want %#v", records, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertAndMergeTransferRowUseAllowlistedColumns(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	dataset, _ := transferDatasetByKey("manufacturers")

	mock.ExpectBegin()
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`INSERT INTO "manufacturer" ("name","website") VALUES ($1,$2) RETURNING "manufacturerid"::text`).
		WithArgs("Acme", "https://example.com").
		WillReturnRows(sqlmock.NewRows([]string{"manufacturerid"}).AddRow("7"))
	id, err := insertTransferRow(context.Background(), tx, dataset, map[string]string{"name": "Acme", "website": "https://example.com"})
	if err != nil || id != "7" {
		t.Fatalf("insertTransferRow = %q, %v", id, err)
	}
	mock.ExpectCommit()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	tx, err = database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE "manufacturer" SET "website"=$1 WHERE "manufacturerid"::text=$2`).
		WithArgs("https://new.example.com", "7").
		WillReturnResult(sqlmock.NewResult(0, 1))
	changed, err := updateTransferRow(
		context.Background(), tx, dataset, "7",
		map[string]string{"name": "Acme", "website": "https://example.com"},
		map[string]string{"name": "Renamed", "website": "https://new.example.com"},
		map[string]string{"name": "existing", "website": "incoming"},
	)
	if err != nil || !changed {
		t.Fatalf("updateTransferRow = %v, %v", changed, err)
	}
	mock.ExpectRollback()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
