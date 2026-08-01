package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/store"
)

// newBulkServices wires a BulkService on top of a URLService sharing the
// same full-schema store, matching how the handler layer constructs them.
func newBulkServices(t *testing.T) (*BulkService, *URLService, *store.Store) {
	t.Helper()
	st := newFullSchemaStore(t)
	urlSvc := NewURLService(st)
	bulkSvc := NewBulkService(st, urlSvc)
	return bulkSvc, urlSvc, st
}

// TestExportCSVEmpty covers the boundary condition of exporting for a user
// with no URLs: the result must still be a valid CSV containing only the
// header row, never an error.
func TestExportCSVEmpty(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	data, err := bulkSvc.ExportCSV(ctx, 1)
	if err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}

	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatalf("exported CSV is not parseable: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected only the header row, got %d rows", len(records))
	}
	want := []string{"short_code", "destination_url", "title", "clicks", "created_at", "expires_at"}
	for i, col := range want {
		if records[0][i] != col {
			t.Errorf("header[%d] = %q, want %q", i, records[0][i], col)
		}
	}
}

// TestExportJSONEmpty mirrors TestExportCSVEmpty for the JSON export path.
func TestExportJSONEmpty(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	data, err := bulkSvc.ExportJSON(ctx, 1)
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}
	if strings.TrimSpace(string(data)) != "null" {
		t.Errorf("expected JSON null for an empty result set, got %q", data)
	}
}

// TestExportCSVAndJSONHappyPath covers the round trip: seed a couple of
// URLs for a user, then confirm both export formats surface them with the
// documented columns and are scoped to the requesting user only.
func TestExportCSVAndJSONHappyPath(t *testing.T) {
	bulkSvc, urlSvc, _ := newBulkServices(t)
	ctx := context.Background()

	if _, err := urlSvc.CreateURLForUser(ctx, 7, &model.CreateURLRequest{LongURL: "https://example.com/one", CustomCode: "one"}); err != nil {
		t.Fatalf("seed CreateURLForUser failed: %v", err)
	}
	if _, err := urlSvc.CreateURLForUser(ctx, 7, &model.CreateURLRequest{LongURL: "https://example.com/two", CustomCode: "two"}); err != nil {
		t.Fatalf("seed CreateURLForUser failed: %v", err)
	}
	// Owned by a different user — must not appear in user 7's export.
	if _, err := urlSvc.CreateURLForUser(ctx, 8, &model.CreateURLRequest{LongURL: "https://example.com/other", CustomCode: "other"}); err != nil {
		t.Fatalf("seed CreateURLForUser failed: %v", err)
	}

	csvData, err := bulkSvc.ExportCSV(ctx, 7)
	if err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(csvData)).ReadAll()
	if err != nil {
		t.Fatalf("exported CSV is not parseable: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected header + 2 data rows, got %d rows", len(records))
	}

	jsonData, err := bulkSvc.ExportJSON(ctx, 7)
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(jsonData, &rows); err != nil {
		t.Fatalf("exported JSON is not parseable: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 exported rows for user 7, got %d", len(rows))
	}
	for _, r := range rows {
		code, _ := r["short_code"].(string)
		if code != "one" && code != "two" {
			t.Errorf("unexpected short_code %q leaked into user 7's export", code)
		}
	}
}

// TestImportCSVHappyPath covers a well-formed CSV with a header row and
// url/custom_code/title columns.
func TestImportCSVHappyPath(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	csvData := "url,custom_code,title\n" +
		"https://example.com/a,codea,Title A\n" +
		"https://example.com/b,codeb,Title B\n"

	success, errRows, err := bulkSvc.ImportCSV(ctx, 1, []byte(csvData))
	if err != nil {
		t.Fatalf("ImportCSV failed: %v", err)
	}
	if success != 2 {
		t.Errorf("success = %d, want 2", success)
	}
	if len(errRows) != 0 {
		t.Errorf("errorRows = %v, want empty", errRows)
	}
}

// TestImportCSVWithoutHeader covers a header-less CSV (no recognizable
// url/destination_url/long_url first column) — every row is treated as
// data.
func TestImportCSVWithoutHeader(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	csvData := "https://example.com/a,codea\nhttps://example.com/b,codeb\n"

	success, errRows, err := bulkSvc.ImportCSV(ctx, 1, []byte(csvData))
	if err != nil {
		t.Fatalf("ImportCSV failed: %v", err)
	}
	if success != 2 {
		t.Errorf("success = %d, want 2", success)
	}
	if len(errRows) != 0 {
		t.Errorf("errorRows = %v, want empty", errRows)
	}
}

// TestImportCSVEmpty covers the empty-input boundary: no rows, no header,
// zero successes, no error.
func TestImportCSVEmpty(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	success, errRows, err := bulkSvc.ImportCSV(ctx, 1, []byte(""))
	if err != nil {
		t.Fatalf("ImportCSV on empty input failed: %v", err)
	}
	if success != 0 {
		t.Errorf("success = %d, want 0", success)
	}
	if len(errRows) != 0 {
		t.Errorf("errorRows = %v, want empty", errRows)
	}
}

// TestImportCSVSkipsBlankURLRows covers rows with an empty first column —
// they must be silently skipped rather than attempted/errored.
func TestImportCSVSkipsBlankURLRows(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	csvData := "url,custom_code,title\n" +
		",skipme,Should Be Skipped\n" +
		"https://example.com/kept,keepme,Kept\n"

	success, errRows, err := bulkSvc.ImportCSV(ctx, 1, []byte(csvData))
	if err != nil {
		t.Fatalf("ImportCSV failed: %v", err)
	}
	if success != 1 {
		t.Errorf("success = %d, want 1", success)
	}
	if len(errRows) != 0 {
		t.Errorf("errorRows = %v, want empty", errRows)
	}
}

// TestImportCSVInvalidURLGoesToErrorRows covers the per-row error path: a
// malformed URL must not stop the batch, but must be reported back to the
// caller instead of silently counted as a success.
func TestImportCSVInvalidURLGoesToErrorRows(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	csvData := "url,custom_code,title\n" +
		"not-a-valid-url,badone,Bad\n" +
		"https://example.com/good,goodone,Good\n"

	success, errRows, err := bulkSvc.ImportCSV(ctx, 1, []byte(csvData))
	if err != nil {
		t.Fatalf("ImportCSV failed: %v", err)
	}
	if success != 1 {
		t.Errorf("success = %d, want 1", success)
	}
	if len(errRows) != 1 {
		t.Fatalf("errorRows = %v, want 1 entry", errRows)
	}
	if !strings.Contains(errRows[0], "not-a-valid-url") {
		t.Errorf("errorRows[0] = %q, want it to reference the failing URL", errRows[0])
	}
}

// TestImportCSVDuplicateCustomCodeGoesToErrorRows covers the resource
// conflict error path: re-importing the same custom_code must not create
// a duplicate short link, and must surface an error for that row.
func TestImportCSVDuplicateCustomCodeGoesToErrorRows(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	first := "url,custom_code\nhttps://example.com/first,dupcode\n"
	success, errRows, err := bulkSvc.ImportCSV(ctx, 1, []byte(first))
	if err != nil {
		t.Fatalf("first ImportCSV failed: %v", err)
	}
	if success != 1 || len(errRows) != 0 {
		t.Fatalf("first import: success=%d errRows=%v, want success=1 errRows=empty", success, errRows)
	}

	second := "url,custom_code\nhttps://example.com/second,dupcode\n"
	success, errRows, err = bulkSvc.ImportCSV(ctx, 1, []byte(second))
	if err != nil {
		t.Fatalf("second ImportCSV failed: %v", err)
	}
	if success != 0 {
		t.Errorf("second import success = %d, want 0", success)
	}
	if len(errRows) != 1 {
		t.Fatalf("second import errRows = %v, want 1 entry", errRows)
	}
}

// TestImportCSVExceedsByteLimit covers the 5 MB size boundary.
func TestImportCSVExceedsByteLimit(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	oversized := bytes.Repeat([]byte("a"), bulkMaxBytes+1)
	success, errRows, err := bulkSvc.ImportCSV(ctx, 1, oversized)
	if err == nil {
		t.Fatal("ImportCSV with oversized input: got nil error, want an error")
	}
	if success != 0 || errRows != nil {
		t.Errorf("ImportCSV with oversized input returned success=%d errRows=%v, want zero values", success, errRows)
	}
}

// TestImportCSVExceedsRowLimit covers the 10,000 row boundary.
func TestImportCSVExceedsRowLimit(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	var buf bytes.Buffer
	buf.WriteString("url\n")
	for i := 0; i < bulkMaxRows+1; i++ {
		buf.WriteString("https://example.com/row\n")
	}

	success, errRows, err := bulkSvc.ImportCSV(ctx, 1, buf.Bytes())
	if err == nil {
		t.Fatal("ImportCSV exceeding row limit: got nil error, want an error")
	}
	if success != 0 || errRows != nil {
		t.Errorf("ImportCSV exceeding row limit returned success=%d errRows=%v, want zero values", success, errRows)
	}
}

// TestImportCSVMalformedCSV covers the parse-error path (unbalanced quotes).
func TestImportCSVMalformedCSV(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	malformed := "url,custom_code\n\"unterminated,quote\n"
	success, errRows, err := bulkSvc.ImportCSV(ctx, 1, []byte(malformed))
	if err == nil {
		t.Fatal("ImportCSV with malformed CSV: got nil error, want an error")
	}
	if success != 0 || errRows != nil {
		t.Errorf("ImportCSV with malformed CSV returned success=%d errRows=%v, want zero values", success, errRows)
	}
}

// TestImportJSONHappyPath covers a well-formed JSON array import.
func TestImportJSONHappyPath(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	rows := []BulkRow{
		{URL: "https://example.com/j1", CustomCode: "j1code", Title: "JSON One"},
		{URL: "https://example.com/j2", CustomCode: "j2code"},
	}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	success, errRows, err := bulkSvc.ImportJSON(ctx, 1, data)
	if err != nil {
		t.Fatalf("ImportJSON failed: %v", err)
	}
	if success != 2 {
		t.Errorf("success = %d, want 2", success)
	}
	if len(errRows) != 0 {
		t.Errorf("errorRows = %v, want empty", errRows)
	}
}

// TestImportJSONEmptyArray covers the empty-input boundary for JSON.
func TestImportJSONEmptyArray(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	success, errRows, err := bulkSvc.ImportJSON(ctx, 1, []byte("[]"))
	if err != nil {
		t.Fatalf("ImportJSON on empty array failed: %v", err)
	}
	if success != 0 {
		t.Errorf("success = %d, want 0", success)
	}
	if len(errRows) != 0 {
		t.Errorf("errorRows = %v, want empty", errRows)
	}
}

// TestImportJSONInvalidJSON covers the parse-error path for malformed JSON.
func TestImportJSONInvalidJSON(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	success, errRows, err := bulkSvc.ImportJSON(ctx, 1, []byte("{not valid json"))
	if err == nil {
		t.Fatal("ImportJSON with invalid JSON: got nil error, want an error")
	}
	if success != 0 || errRows != nil {
		t.Errorf("ImportJSON with invalid JSON returned success=%d errRows=%v, want zero values", success, errRows)
	}
}

// TestImportJSONExceedsByteLimit covers the 5 MB size boundary for JSON.
func TestImportJSONExceedsByteLimit(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	oversized := bytes.Repeat([]byte("a"), bulkMaxBytes+1)
	success, errRows, err := bulkSvc.ImportJSON(ctx, 1, oversized)
	if err == nil {
		t.Fatal("ImportJSON with oversized input: got nil error, want an error")
	}
	if success != 0 || errRows != nil {
		t.Errorf("ImportJSON with oversized input returned success=%d errRows=%v, want zero values", success, errRows)
	}
}

// TestImportJSONExceedsRowLimit covers the 10,000 row boundary for JSON.
func TestImportJSONExceedsRowLimit(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	rows := make([]BulkRow, bulkMaxRows+1)
	for i := range rows {
		rows[i] = BulkRow{URL: "https://example.com/row"}
	}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	success, errRows, err := bulkSvc.ImportJSON(ctx, 1, data)
	if err == nil {
		t.Fatal("ImportJSON exceeding row limit: got nil error, want an error")
	}
	if success != 0 || errRows != nil {
		t.Errorf("ImportJSON exceeding row limit returned success=%d errRows=%v, want zero values", success, errRows)
	}
}

// TestImportJSONSkipsBlankURLRows mirrors the CSV blank-row skip behavior
// for the JSON import path.
func TestImportJSONSkipsBlankURLRows(t *testing.T) {
	bulkSvc, _, _ := newBulkServices(t)
	ctx := context.Background()

	rows := []BulkRow{
		{URL: "   "},
		{URL: "https://example.com/kept", CustomCode: "keptjson"},
	}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	success, errRows, err := bulkSvc.ImportJSON(ctx, 1, data)
	if err != nil {
		t.Fatalf("ImportJSON failed: %v", err)
	}
	if success != 1 {
		t.Errorf("success = %d, want 1", success)
	}
	if len(errRows) != 0 {
		t.Errorf("errorRows = %v, want empty", errRows)
	}
}
