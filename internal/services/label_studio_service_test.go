package services

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"os"
	"strings"
	"testing"
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

func TestBuildLabelPDFHTML(t *testing.T) {
	t.Parallel()
	html := buildLabelPDFHTML([]string{"data:image/png;base64,AAA", "data:image/png;base64,BBB"}, 51, 25, 2)
	if pages := strings.Count(html, `<div class="label-page">`); pages != 4 {
		t.Fatalf("expected 4 PDF pages, got %d", pages)
	}
	for _, expected := range []string{"size: 51.0000mm 25.0000mm", "data:image/png;base64,AAA", "data:image/png;base64,BBB", "window.pdfReady = true"} {
		if !strings.Contains(html, expected) {
			t.Errorf("expected PDF HTML to contain %q", expected)
		}
	}
}

func TestRenderLabelPDFWithChromium(t *testing.T) {
	if os.Getenv("WAREHOUSECORE_CHROME_TEST") != "1" {
		t.Skip("set WAREHOUSECORE_CHROME_TEST=1 to run the Chromium integration test")
	}
	const onePixelPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	html := buildLabelPDFHTML([]string{onePixelPNG}, 51, 25, 1)
	pdfData, err := (&LabelService{}).renderHTMLToPDF(html, 51, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfData) < 5 || string(pdfData[:5]) != "%PDF-" {
		t.Fatalf("expected PDF data, got %d bytes", len(pdfData))
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
