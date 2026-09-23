package services

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

const (
	a4WidthMM  = 210.0
	a4HeightMM = 297.0
)

type labelSheetImage struct {
	DataURL string
	Copies  int
}

type labelSheetLayout struct {
	Columns  int
	Rows     int
	Capacity int
	Pages    int
}

func normalizeLabelPrintItems(targetIDs []string, copies int, requested []LabelPrintItem, maxTargets, maxTotal int) ([]LabelPrintItem, error) {
	items := make([]LabelPrintItem, 0, max(len(requested), len(targetIDs)))
	if len(requested) > 0 {
		items = append(items, requested...)
	} else {
		if copies < 1 || copies > 1000 {
			return nil, fmt.Errorf("Copies must be between 1 and 1000.")
		}
		for _, targetID := range targetIDs {
			items = append(items, LabelPrintItem{TargetID: targetID, Copies: copies})
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("At least one label target is required.")
	}
	if len(items) > maxTargets {
		return nil, fmt.Errorf("At most %d label targets are allowed.", maxTargets)
	}

	seen := make(map[string]bool, len(items))
	total := 0
	for index := range items {
		items[index].TargetID = strings.TrimSpace(items[index].TargetID)
		if items[index].TargetID == "" {
			return nil, fmt.Errorf("Label target IDs must not be empty.")
		}
		if seen[items[index].TargetID] {
			return nil, fmt.Errorf("Label target %q is listed more than once.", items[index].TargetID)
		}
		seen[items[index].TargetID] = true
		if items[index].Copies < 1 || items[index].Copies > 1000 {
			return nil, fmt.Errorf("Copies for target %q must be between 1 and 1000.", items[index].TargetID)
		}
		total += items[index].Copies
		if total > maxTotal {
			return nil, fmt.Errorf("At most %d labels are allowed per request.", maxTotal)
		}
	}
	return items, nil
}

func labelPrintTargetIDs(items []LabelPrintItem) []string {
	ids := make([]string, len(items))
	for index, item := range items {
		ids[index] = item.TargetID
	}
	return ids
}

func (s *LabelService) exportTargetsA4Sheet(request LabelPDFRequest, items []LabelPrintItem) ([]byte, error) {
	orientation := strings.ToLower(strings.TrimSpace(request.Orientation))
	if orientation == "" {
		orientation = "portrait"
	}
	pageWidth, pageHeight := a4WidthMM, a4HeightMM
	if orientation == "landscape" {
		pageWidth, pageHeight = pageHeight, pageWidth
	} else if orientation != "portrait" {
		return nil, fmt.Errorf("The A4 orientation must be portrait or landscape.")
	}
	_, template, err := s.targetAndTemplate(request.TargetType, items[0].TargetID, request.TemplateID)
	if err != nil {
		return nil, err
	}
	if _, _, err := buildA4LabelSheetHTML([]labelSheetImage{{DataURL: "data:image/png;base64,AA==", Copies: 1}},
		template.Width, template.Height, pageWidth, pageHeight, request.MarginMM, request.HorizontalGapMM, request.VerticalGapMM, request.ShowGuides); err != nil {
		return nil, err
	}

	results, err := s.RenderTargetLabels(LabelBatchRequest{
		TargetType: request.TargetType, TargetIDs: labelPrintTargetIDs(items), TemplateID: request.TemplateID,
		Save: true, IncludeImage: true,
	})
	if err != nil {
		return nil, err
	}
	if len(results) != len(items) || len(results) == 0 || results[0].Template == nil {
		return nil, fmt.Errorf("label renderer returned incomplete sheet data")
	}

	images := make([]labelSheetImage, len(results))
	for index, result := range results {
		if result.Target.ID != items[index].TargetID || !strings.HasPrefix(result.ImageData, "data:image/png;base64,") {
			return nil, fmt.Errorf("label renderer returned invalid data for target %q", items[index].TargetID)
		}
		images[index] = labelSheetImage{DataURL: result.ImageData, Copies: items[index].Copies}
	}

	html, _, err := buildA4LabelSheetHTML(images, results[0].Template.Width, results[0].Template.Height, pageWidth, pageHeight,
		request.MarginMM, request.HorizontalGapMM, request.VerticalGapMM, request.ShowGuides)
	if err != nil {
		return nil, err
	}
	return s.RenderHTMLToPDF(html, pageWidth, pageHeight)
}

func buildA4LabelSheetHTML(images []labelSheetImage, labelWidth, labelHeight, pageWidth, pageHeight, margin, gapX, gapY float64, showGuides bool) (string, labelSheetLayout, error) {
	if labelWidth <= 0 || labelHeight <= 0 {
		return "", labelSheetLayout{}, fmt.Errorf("label dimensions must be positive")
	}
	if margin < 0 || margin > 50 || gapX < 0 || gapX > 50 || gapY < 0 || gapY > 50 {
		return "", labelSheetLayout{}, fmt.Errorf("Sheet margins and gaps must be between 0 and 50 mm.")
	}
	availableWidth := pageWidth - 2*margin
	availableHeight := pageHeight - 2*margin
	columns := int(math.Floor((availableWidth + gapX + 1e-9) / (labelWidth + gapX)))
	rows := int(math.Floor((availableHeight + gapY + 1e-9) / (labelHeight + gapY)))
	if columns < 1 || rows < 1 {
		return "", labelSheetLayout{}, fmt.Errorf("The label size does not fit the A4 sheet with the selected settings.")
	}

	total := 0
	dataURLs := make([]string, len(images))
	for index, image := range images {
		if image.Copies < 1 || !strings.HasPrefix(image.DataURL, "data:image/png;base64,") {
			return "", labelSheetLayout{}, fmt.Errorf("invalid label sheet image at position %d", index+1)
		}
		total += image.Copies
		dataURLs[index] = image.DataURL
	}
	if total == 0 {
		return "", labelSheetLayout{}, fmt.Errorf("at least one label is required for an A4 sheet")
	}
	capacity := columns * rows
	layout := labelSheetLayout{Columns: columns, Rows: rows, Capacity: capacity, Pages: (total + capacity - 1) / capacity}
	encodedSources, err := json.Marshal(dataURLs)
	if err != nil {
		return "", labelSheetLayout{}, fmt.Errorf("encode label sheet images: %w", err)
	}

	var html strings.Builder
	fmt.Fprintf(&html, `<!doctype html><html><head><meta charset="utf-8"><style>
@page { size: %.4fmm %.4fmm; margin: 0; }
* { box-sizing: border-box; }
html, body { margin: 0; padding: 0; background: #fff; }
.sheet { width: %.4fmm; height: %.4fmm; padding: %.4fmm; display: grid; grid-template-columns: repeat(%d, %.4fmm); grid-auto-rows: %.4fmm; column-gap: %.4fmm; row-gap: %.4fmm; align-content: start; justify-content: start; break-after: page; overflow: hidden; }
.sheet:last-child { break-after: auto; }
.label { width: %.4fmm; height: %.4fmm; background-position: center; background-repeat: no-repeat; background-size: 100%% 100%%; print-color-adjust: exact; -webkit-print-color-adjust: exact; }
`, pageWidth, pageHeight, pageWidth, pageHeight, margin, columns, labelWidth, labelHeight, gapX, gapY, labelWidth, labelHeight)
	if showGuides {
		html.WriteString(".label { outline: 0.2mm dashed rgba(0,0,0,.45); outline-offset: -0.2mm; }\n")
	}
	for index, image := range images {
		fmt.Fprintf(&html, ".label-%d { background-image: url('%s'); }\n", index, image.DataURL)
	}
	html.WriteString("</style></head><body>")
	position := 0
	for imageIndex, image := range images {
		for range image.Copies {
			if position%capacity == 0 {
				if position > 0 {
					html.WriteString("</section>")
				}
				html.WriteString(`<section class="sheet">`)
			}
			fmt.Fprintf(&html, `<div class="label label-%d" aria-hidden="true"></div>`, imageIndex)
			position++
		}
	}
	html.WriteString("</section><script>window.pdfReady=false;const sources=")
	html.Write(encodedSources)
	html.WriteString(`;Promise.all(sources.map(source=>new Promise((resolve,reject)=>{const image=new Image();image.onload=resolve;image.onerror=reject;image.src=source;}))).then(()=>document.fonts.ready).then(()=>{window.pdfReady=true;});</script></body></html>`)
	return html.String(), layout, nil
}
