package services

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func TestValidLabelTargetType(t *testing.T) {
	t.Parallel()
	for _, targetType := range []string{LabelTargetDevice, LabelTargetProduct, LabelTargetCase, LabelTargetZone} {
		if !ValidLabelTargetType(targetType) {
			t.Fatalf("expected %q to be valid", targetType)
		}
	}
	if ValidLabelTargetType("job") {
		t.Fatal("unexpected unsupported label target")
	}
}

func TestCableLabelFields(t *testing.T) {
	t.Parallel()
	fields := LabelFields(LabelTargetProduct)
	for _, expected := range []string{"cable_id", "cable_type", "connector_a", "connector_b", "length_m", "tracking_mode"} {
		found := false
		for _, field := range fields {
			if field.Key == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected cable label field %q", expected)
		}
	}
}

func TestPNGBytesToPDF(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	pdfData, err := pngBytesToPDF(encoded.Bytes(), 51, 25)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdfData, []byte("%PDF-")) {
		t.Fatalf("expected PDF data, got %d bytes", len(pdfData))
	}
	pages, err := api.PageCount(bytes.NewReader(pdfData), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if pages != 1 {
		t.Fatalf("expected one PDF page, got %d", pages)
	}
	dimensions, err := api.PageDims(bytes.NewReader(pdfData), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if len(dimensions) != 1 || math.Abs(dimensions[0].Width-51*72/25.4) > 0.1 || math.Abs(dimensions[0].Height-25*72/25.4) > 0.1 {
		t.Fatalf("unexpected PDF dimensions: %+v", dimensions)
	}

	merged, err := mergeCachedLabelPDFs([]cachedLabelPDF{{PDF: pdfData}, {PDF: pdfData}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	pages, err = api.PageCount(bytes.NewReader(merged), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if pages != 4 {
		t.Fatalf("expected four merged PDF pages, got %d", pages)
	}
	perTarget, err := mergeCachedLabelPDFItems(
		[]cachedLabelPDF{{Target: LabelTarget{ID: "A"}, PDF: pdfData}, {Target: LabelTarget{ID: "B"}, PDF: pdfData}},
		[]LabelPrintItem{{TargetID: "A", Copies: 4}, {TargetID: "B", Copies: 2}},
	)
	if err != nil {
		t.Fatal(err)
	}
	pages, err = api.PageCount(bytes.NewReader(perTarget), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if pages != 6 {
		t.Fatalf("expected six per-target PDF pages, got %d", pages)
	}
}

func TestNormalizeLabelPrintItems(t *testing.T) {
	t.Parallel()

	legacy, err := normalizeLabelPrintItems([]string{"A", "B"}, 2, nil, 250, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 2 || legacy[0].Copies != 2 || legacy[1].TargetID != "B" {
		t.Fatalf("unexpected legacy items: %+v", legacy)
	}

	items, err := normalizeLabelPrintItems(nil, 0, []LabelPrintItem{{TargetID: " A ", Copies: 4}, {TargetID: "B", Copies: 2}}, 250, 500)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].TargetID != "A" || items[0].Copies != 4 || items[1].Copies != 2 {
		t.Fatalf("unexpected normalized items: %+v", items)
	}

	for name, requested := range map[string][]LabelPrintItem{
		"duplicate": {{TargetID: "A", Copies: 1}, {TargetID: "A", Copies: 2}},
		"zero":      {{TargetID: "A", Copies: 0}},
		"too many":  {{TargetID: "A", Copies: 501}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeLabelPrintItems(nil, 0, requested, 250, 500); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestBuildA4LabelSheetHTML(t *testing.T) {
	t.Parallel()
	images := []labelSheetImage{
		{DataURL: "data:image/png;base64,QQ==", Copies: 4},
		{DataURL: "data:image/png;base64,Qg==", Copies: 2},
	}
	html, layout, err := buildA4LabelSheetHTML(images, 51, 25, 210, 297, 3, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Columns != 4 || layout.Rows != 11 || layout.Capacity != 44 || layout.Pages != 1 {
		t.Fatalf("unexpected A4 layout: %+v", layout)
	}
	if strings.Count(html, `<div class="label`) != 6 {
		t.Fatalf("expected six label cells")
	}
	if strings.Count(html, "data:image/png;base64,") != 4 {
		t.Fatalf("expected each unique image once in CSS and once in preload data")
	}
	if !strings.Contains(html, "outline: 0.2mm dashed") || !strings.Contains(html, "window.pdfReady=true") {
		t.Fatalf("expected guides and PDF readiness in sheet HTML")
	}
	multiPageHTML, multiPageLayout, err := buildA4LabelSheetHTML([]labelSheetImage{{DataURL: images[0].DataURL, Copies: 45}}, 51, 25, 210, 297, 3, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if multiPageLayout.Pages != 2 || strings.Count(multiPageHTML, `<section class="sheet">`) != 2 {
		t.Fatalf("expected a two-page sheet, got %+v", multiPageLayout)
	}

	if _, _, err := buildA4LabelSheetHTML(images, 220, 25, 210, 297, 0, 0, 0, false); err == nil {
		t.Fatal("expected oversized label to be rejected")
	}
}

func TestRenderLabelPDFWithChromium(t *testing.T) {
	if os.Getenv("WAREHOUSECORE_CHROME_TEST") != "1" {
		t.Skip("set WAREHOUSECORE_CHROME_TEST=1 to run the Chromium integration test")
	}
	html := `<!doctype html><html><body><div>Label</div><script>window.pdfReady = true;</script></body></html>`
	// A real multi-label export can easily exceed the size Chromium accepts in
	// a data URL. Keep this document deliberately large to cover that failure.
	html += "<!--" + strings.Repeat("x", 8<<20) + "-->"
	pdfData, err := (&LabelService{}).renderHTMLToPDF(html, 51, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfData) < 5 || string(pdfData[:5]) != "%PDF-" {
		t.Fatalf("expected PDF data, got %d bytes", len(pdfData))
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 40, 20))); err != nil {
		t.Fatal(err)
	}
	sheetHTML, layout, err := buildA4LabelSheetHTML([]labelSheetImage{{
		DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), Copies: 6,
	}}, 51, 25, 210, 297, 3, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	sheetPDF, err := (&LabelService{}).RenderHTMLToPDF(sheetHTML, 210, 297)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := api.PageCount(bytes.NewReader(sheetPDF), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if pages != layout.Pages || pages != 1 {
		t.Fatalf("expected one A4 sheet page, got %d", pages)
	}
	dimensions, err := api.PageDims(bytes.NewReader(sheetPDF), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if len(dimensions) != 1 || math.Abs(dimensions[0].Width-210*72/25.4) > 0.5 || math.Abs(dimensions[0].Height-297*72/25.4) > 0.5 {
		t.Fatalf("unexpected A4 dimensions: %+v", dimensions)
	}
}

func TestSendRawPrinterData(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	received := make(chan string, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			received <- ""
			return
		}
		defer connection.Close()
		payload, _ := io.ReadAll(connection)
		received <- string(payload)
	}()

	address := listener.Addr().(*net.TCPAddr)
	if err := sendRawPrinterData("127.0.0.1", address.Port, []byte("^XA^XZ\n")); err != nil {
		t.Fatal(err)
	}
	if actual := <-received; actual != "^XA^XZ\n" {
		t.Fatalf("unexpected printer payload %q", actual)
	}
}

func TestEncodePNGAsZPL(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.Black)
	img.Set(1, 0, color.White)
	img.Set(0, 1, color.White)
	img.Set(1, 1, color.Black)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}

	zpl, err := encodePNGAsZPL(encoded.Bytes(), 51, 25, 203, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"^XA", "^PW408", "^LL200", "^GFA,", "^PQ3", "^XZ"} {
		if !strings.Contains(zpl, expected) {
			t.Errorf("expected ZPL to contain %q, got %q", expected, zpl)
		}
	}
}
