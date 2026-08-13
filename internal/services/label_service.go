package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/skip2/go-qrcode"

	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

type LabelService struct{}

func NewLabelService() *LabelService {
	log.Printf("[LABEL INIT] Label service initialized (using headless browser rendering)")
	return &LabelService{}
}

// GenerateQRCode generates a QR code and returns it as base64-encoded PNG
func (s *LabelService) GenerateQRCode(content string, size int) (string, error) {
	if size == 0 {
		size = 256 // default size
	}

	// Generate QR code
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", fmt.Errorf("failed to generate QR code: %w", err)
	}

	// Convert to PNG bytes
	pngBytes, err := qr.PNG(size)
	if err != nil {
		return "", fmt.Errorf("failed to convert QR code to PNG: %w", err)
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(pngBytes)
	return fmt.Sprintf("data:image/png;base64,%s", encoded), nil
}

// GenerateBarcode generates a Code128 barcode and returns it as base64-encoded PNG
func (s *LabelService) GenerateBarcode(content string, width, height int) (string, error) {
	if width == 0 {
		width = 300
	}
	if height == 0 {
		height = 100
	}

	// Ensure minimum dimensions for barcode library (needs at least 123px width)
	if width < 123 {
		width = 123
	}
	if height < 1 {
		height = 1
	}

	// Generate Code128 barcode
	bc, err := code128.Encode(content)
	if err != nil {
		return "", fmt.Errorf("failed to generate barcode: %w", err)
	}

	// Scale barcode
	scaled, err := barcode.Scale(bc, width, height)
	if err != nil {
		return "", fmt.Errorf("failed to scale barcode: %w", err)
	}

	// Convert to PNG
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, scaled); err != nil {
		return "", fmt.Errorf("failed to encode barcode to PNG: %w", err)
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return fmt.Sprintf("data:image/png;base64,%s", encoded), nil
}

// GetAllTemplates retrieves all label templates
func (s *LabelService) GetAllTemplates() ([]models.LabelTemplate, error) {
	db := repository.GetDB()
	var templates []models.LabelTemplate

	if err := db.Order("is_default DESC, name ASC").Find(&templates).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch templates: %w", err)
	}

	return templates, nil
}

// GetTemplateByID retrieves a specific template
func (s *LabelService) GetTemplateByID(id int) (*models.LabelTemplate, error) {
	db := repository.GetDB()
	var template models.LabelTemplate

	if err := db.Where("id = ?", id).First(&template).Error; err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}

	return &template, nil
}

// CreateTemplate creates a new label template
func (s *LabelService) CreateTemplate(template *models.LabelTemplate) error {
	db := repository.GetDB()
	template.Name = strings.TrimSpace(template.Name)
	if template.TargetType == "" {
		template.TargetType = LabelTargetDevice
	}
	if !ValidLabelTargetType(template.TargetType) {
		return fmt.Errorf("unsupported target type %q", template.TargetType)
	}
	if template.Revision < 1 {
		template.Revision = 1
	}

	// Validate template JSON
	var elements []models.LabelElement
	if err := json.Unmarshal([]byte(template.TemplateJSON), &elements); err != nil {
		return fmt.Errorf("invalid template JSON: %w", err)
	}

	// If this is set as default, unset other defaults
	if template.IsDefault {
		if err := db.Model(&models.LabelTemplate{}).Where("target_type = ? AND is_default = ?", template.TargetType, true).Update("is_default", false).Error; err != nil {
			return fmt.Errorf("failed to unset other defaults: %w", err)
		}
	}

	if err := db.Create(template).Error; err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}

	return nil
}

// UpdateTemplate updates an existing label template
func (s *LabelService) UpdateTemplate(id int, updates map[string]interface{}) error {
	db := repository.GetDB()
	var current models.LabelTemplate
	if err := db.First(&current, id).Error; err != nil {
		return fmt.Errorf("template not found: %w", err)
	}

	// Validate template JSON if provided
	if templateJSON, ok := updates["template_json"].(string); ok {
		var elements []models.LabelElement
		if err := json.Unmarshal([]byte(templateJSON), &elements); err != nil {
			return fmt.Errorf("invalid template JSON: %w", err)
		}
	}

	targetType := current.TargetType
	if requestedType, ok := updates["target_type"].(string); ok {
		requestedType = strings.TrimSpace(requestedType)
		if !ValidLabelTargetType(requestedType) {
			return fmt.Errorf("unsupported target type %q", requestedType)
		}
		targetType = requestedType
	}

	allowed := map[string]interface{}{}
	for _, key := range []string{"name", "description", "width", "height", "template_json", "is_default", "target_type"} {
		if value, ok := updates[key]; ok {
			allowed[key] = value
		}
	}
	if _, designChanged := allowed["template_json"]; designChanged {
		allowed["revision"] = current.Revision + 1
	} else if _, widthChanged := allowed["width"]; widthChanged {
		allowed["revision"] = current.Revision + 1
	} else if _, heightChanged := allowed["height"]; heightChanged {
		allowed["revision"] = current.Revision + 1
	}

	// Defaults are unique within each target type.
	if isDefault, ok := allowed["is_default"].(bool); ok && isDefault {
		if err := db.Model(&models.LabelTemplate{}).Where("target_type = ? AND is_default = ? AND id != ?", targetType, true, id).Update("is_default", false).Error; err != nil {
			return fmt.Errorf("failed to unset other defaults: %w", err)
		}
	}

	if len(allowed) == 0 {
		return nil
	}
	if err := db.Model(&models.LabelTemplate{}).Where("id = ?", id).Updates(allowed).Error; err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}

	return nil
}

// DeleteTemplate deletes a label template
func (s *LabelService) DeleteTemplate(id int) error {
	db := repository.GetDB()

	result := db.Where("id = ?", id).Delete(&models.LabelTemplate{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete template: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("template not found")
	}

	return nil
}

// GenerateLabelForDevice generates a complete label for a device
func (s *LabelService) GenerateLabelForDevice(deviceID string, templateID int) (map[string]interface{}, error) {
	// Get template
	template, err := s.GetTemplateByID(templateID)
	if err != nil {
		return nil, err
	}

	// Parse template elements
	var elements []models.LabelElement
	if err := json.Unmarshal([]byte(template.TemplateJSON), &elements); err != nil {
		return nil, fmt.Errorf("invalid template JSON: %w", err)
	}

	// Get device data
	db := repository.GetDB()
	var device struct {
		DeviceID    string `json:"device_id"`
		ProductName string `json:"product_name"`
		Subcategory string `json:"subcategory"`
		Category    string `json:"category"`
	}

	query := `
		SELECT
			d.deviceID as device_id,
			COALESCE(p.name, '') as product_name,
			COALESCE(sb.name, '') as subcategory,
			COALESCE(c.name, '') as category
		FROM devices d
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN subbiercategories sb ON p.subbiercategoryID = sb.subbiercategoryID
		LEFT JOIN categories c ON p.categoryID = c.categoryID
		WHERE d.deviceID = $1
	`

	if err := db.Raw(query, deviceID).Scan(&device).Error; err != nil {
		return nil, fmt.Errorf("device not found: %w", err)
	}

	// Process elements and generate barcodes/QR codes
	processedElements := make([]map[string]interface{}, 0, len(elements))
	for _, elem := range elements {
		processed := map[string]interface{}{
			"type":     elem.Type,
			"x":        elem.X,
			"y":        elem.Y,
			"width":    elem.Width,
			"height":   elem.Height,
			"rotation": elem.Rotation,
			"style":    elem.Style,
		}

		// Resolve content from field names
		content := elem.Content
		switch elem.Content {
		case "device_id":
			content = device.DeviceID
		case "device_name", "product_name", "name":
			// Support multiple aliases for product name
			content = device.ProductName
		case "product", "subcategory":
			content = device.Subcategory
		case "category":
			content = device.Category
		}

		processed["content"] = content

		// Generate barcode/QR code if needed, or copy static image data
		if elem.Type == "qrcode" {
			// Convert mm to pixels at 300 DPI (300 pixels per inch, 25.4mm per inch)
			// pixels = mm * 300 / 25.4 ≈ mm * 11.8
			sizePixels := int(elem.Width * 11.8)
			if sizePixels < 100 {
				sizePixels = 100
			}
			qrData, err := s.GenerateQRCode(content, sizePixels)
			if err != nil {
				return nil, err
			}
			processed["image_data"] = qrData
		} else if elem.Type == "barcode" {
			// Convert mm to pixels at 300 DPI
			widthPixels := int(elem.Width * 11.8)
			heightPixels := int(elem.Height * 11.8)
			if widthPixels < 123 {
				widthPixels = 123 // Minimum for Code128
			}
			if heightPixels < 50 {
				heightPixels = 50
			}
			barcodeData, err := s.GenerateBarcode(content, widthPixels, heightPixels)
			if err != nil {
				return nil, err
			}
			processed["image_data"] = barcodeData
		} else if elem.Type == "image" && elem.ImageData != "" {
			// Copy static image data from template
			processed["image_data"] = elem.ImageData
		}

		processedElements = append(processedElements, processed)
	}

	return map[string]interface{}{
		"template": template,
		"elements": processedElements,
		"device":   device,
	}, nil
}

// GenerateLabelForCase generates a complete label for a case
func (s *LabelService) GenerateLabelForCase(caseID int, templateID int) (map[string]interface{}, error) {
	// Get template
	template, err := s.GetTemplateByID(templateID)
	if err != nil {
		return nil, err
	}

	// Parse template elements
	var elements []models.LabelElement
	if err := json.Unmarshal([]byte(template.TemplateJSON), &elements); err != nil {
		return nil, fmt.Errorf("invalid template JSON: %w", err)
	}

	// Get case data
	db := repository.GetDB()
	var caseData struct {
		CaseID      int      `json:"case_id"`
		Name        string   `json:"name"`
		Description *string  `json:"description"`
		Barcode     *string  `json:"barcode"`
		RFIDTag     *string  `json:"rfid_tag"`
		Width       *float64 `json:"width"`
		Height      *float64 `json:"height"`
		Depth       *float64 `json:"depth"`
		Weight      *float64 `json:"weight"`
		Status      string   `json:"status"`
		ZoneName    *string  `json:"zone_name"`
	}

	query := `
		SELECT
			c.caseID as case_id,
			c.name,
			c.description,
			c.barcode,
			c.rfid_tag,
			c.width,
			c.height,
			c.depth,
			c.weight,
			c.status
		FROM cases c
		WHERE c.caseID = $1
	`

	if err := db.Raw(query, caseID).Scan(&caseData).Error; err != nil {
		return nil, fmt.Errorf("case not found: %w", err)
	}

	// Process elements and generate barcodes/QR codes
	processedElements := make([]map[string]interface{}, 0, len(elements))
	for _, elem := range elements {
		processed := map[string]interface{}{
			"type":     elem.Type,
			"x":        elem.X,
			"y":        elem.Y,
			"width":    elem.Width,
			"height":   elem.Height,
			"rotation": elem.Rotation,
			"style":    elem.Style,
		}

		// Resolve content from field names
		content := elem.Content
		switch elem.Content {
		case "case_id", "device_id": // Support both case_id and device_id (for compatibility with device templates)
			content = fmt.Sprintf("CASE-%d", caseData.CaseID)
		case "name", "product_name": // Support both name and product_name
			content = caseData.Name
		case "description":
			if caseData.Description != nil {
				content = *caseData.Description
			}
		case "barcode":
			if caseData.Barcode != nil {
				content = *caseData.Barcode
			} else {
				content = fmt.Sprintf("CASE-%d", caseData.CaseID) // fallback
			}
		case "rfid_tag":
			if caseData.RFIDTag != nil {
				content = *caseData.RFIDTag
			}
		case "dimensions":
			if caseData.Width != nil && caseData.Height != nil && caseData.Depth != nil {
				content = fmt.Sprintf("%.1fx%.1fx%.1f cm", *caseData.Width, *caseData.Height, *caseData.Depth)
			}
		case "weight":
			if caseData.Weight != nil {
				content = fmt.Sprintf("%.1f kg", *caseData.Weight)
			}
		case "zone_name":
			if caseData.ZoneName != nil {
				content = *caseData.ZoneName
			}
		case "status":
			content = caseData.Status
		}

		processed["content"] = content

		// Generate barcode/QR code if needed, or copy static image data
		if elem.Type == "qrcode" {
			// Convert mm to pixels at 300 DPI (300 pixels per inch, 25.4mm per inch)
			// pixels = mm * 300 / 25.4 ≈ mm * 11.8
			sizePixels := int(elem.Width * 11.8)
			if sizePixels < 100 {
				sizePixels = 100
			}
			qrData, err := s.GenerateQRCode(content, sizePixels)
			if err != nil {
				return nil, err
			}
			processed["image_data"] = qrData
		} else if elem.Type == "barcode" {
			// Convert mm to pixels at 300 DPI
			widthPixels := int(elem.Width * 11.8)
			heightPixels := int(elem.Height * 11.8)
			if widthPixels < 123 {
				widthPixels = 123 // Minimum for Code128
			}
			if heightPixels < 50 {
				heightPixels = 50
			}
			barcodeData, err := s.GenerateBarcode(content, widthPixels, heightPixels)
			if err != nil {
				return nil, err
			}
			processed["image_data"] = barcodeData
		} else if elem.Type == "image" && elem.ImageData != "" {
			// Copy static image data from template
			processed["image_data"] = elem.ImageData
		}

		processedElements = append(processedElements, processed)
	}

	return map[string]interface{}{
		"template": template,
		"elements": processedElements,
		"case":     caseData,
	}, nil
}

// SaveLabelImage saves a base64-encoded label image to disk and updates the device record
func (s *LabelService) SaveLabelImage(deviceID string, base64Image string) (string, error) {
	// Remove base64 prefix if present
	if len(base64Image) > 22 && base64Image[:22] == "data:image/png;base64," {
		base64Image = base64Image[22:]
	}

	// Decode base64
	imageData, err := base64.StdEncoding.DecodeString(base64Image)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 image: %w", err)
	}

	// Create labels directory if it doesn't exist
	labelsDir := "./web/dist/labels"
	if err := os.MkdirAll(labelsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create labels directory: %w", err)
	}

	// Save file
	filename := fmt.Sprintf("%s_label.png", deviceID)
	filePath := filepath.Join(labelsDir, filename)

	if err := os.WriteFile(filePath, imageData, 0644); err != nil {
		return "", fmt.Errorf("failed to write label file: %w", err)
	}

	// Update device record with label path
	labelPath := fmt.Sprintf("/labels/%s", filename)
	db := repository.GetDB()
	result := db.Exec("UPDATE devices SET label_path = $1 WHERE deviceID = $2", labelPath, deviceID)
	if result.Error != nil {
		return "", fmt.Errorf("failed to update device label path: %w", result.Error)
	}

	return labelPath, nil
}

// SaveCaseLabelImage saves a base64-encoded label image to disk and updates the case record
func (s *LabelService) SaveCaseLabelImage(caseID int, base64Image string) (string, error) {
	// Remove base64 prefix if present
	if len(base64Image) > 22 && base64Image[:22] == "data:image/png;base64," {
		base64Image = base64Image[22:]
	}

	// Decode base64
	imageData, err := base64.StdEncoding.DecodeString(base64Image)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 image: %w", err)
	}

	// Create labels/cases directory if it doesn't exist
	labelsDir := "./web/dist/labels/cases"
	if err := os.MkdirAll(labelsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create labels directory: %w", err)
	}

	// Save file
	filename := fmt.Sprintf("CASE-%d_label.png", caseID)
	filePath := filepath.Join(labelsDir, filename)

	if err := os.WriteFile(filePath, imageData, 0644); err != nil {
		return "", fmt.Errorf("failed to write label file: %w", err)
	}

	// Update case record with label path
	labelPath := fmt.Sprintf("/labels/cases/%s", filename)
	db := repository.GetDB()
	result := db.Exec("UPDATE cases SET label_path = $1 WHERE caseID = $2", labelPath, caseID)
	if result.Error != nil {
		return "", fmt.Errorf("failed to update case label path: %w", result.Error)
	}

	return labelPath, nil
}

// AutoGenerateDeviceLabel automatically generates a label for a new device using the default template
// This uses a headless Chrome browser to render labels identically to the Label Designer UI
func (s *LabelService) AutoGenerateDeviceLabel(deviceID string) error {
	// Get default template
	db := repository.GetDB()
	var template models.LabelTemplate
	if err := db.Where("target_type = ? AND is_default = ?", LabelTargetDevice, true).First(&template).Error; err != nil {
		// No default template, skip label generation silently
		log.Printf("[LABEL DEBUG] No default template found, skipping auto-generation for device: %s", deviceID)
		return nil
	}

	log.Printf("[LABEL DEBUG] === Generating label for device: %s ===", deviceID)
	log.Printf("[LABEL DEBUG] Template: %s (ID: %d)", template.Name, template.ID)
	log.Printf("[LABEL DEBUG] Label size: %.1fmm x %.1fmm", template.Width, template.Height)

	// Generate label data using the template (includes device data, barcodes, QR codes)
	labelData, err := s.GenerateLabelForDevice(deviceID, template.ID)
	if err != nil {
		return fmt.Errorf("failed to generate label data: %w", err)
	}

	// Convert label data to JSON for embedding in HTML
	labelDataJSON, err := json.Marshal(labelData)
	if err != nil {
		return fmt.Errorf("failed to marshal label data: %w", err)
	}

	// Load HTML template
	htmlTemplatePath := "./internal/services/label_template.html"
	htmlTemplate, err := os.ReadFile(htmlTemplatePath) // FIXED: replaced deprecated ioutil.ReadFile
	if err != nil {
		return fmt.Errorf("failed to load HTML template: %w", err)
	}

	// Replace placeholder with actual label data
	htmlContent := strings.Replace(string(htmlTemplate), "{{LABEL_DATA_JSON}}", string(labelDataJSON), 1)

	// Render label using headless Chrome
	base64PNG, err := s.renderLabelWithHeadlessBrowser(htmlContent)
	if err != nil {
		return fmt.Errorf("failed to render label with headless browser: %w", err)
	}

	// Save the label
	base64String := fmt.Sprintf("data:image/png;base64,%s", base64PNG)
	_, err = s.SaveLabelImage(deviceID, base64String)
	if err != nil {
		return fmt.Errorf("failed to save label: %w", err)
	}

	log.Printf("[LABEL DEBUG] Label generated successfully for device: %s", deviceID)
	return nil
}

// renderLabelWithHeadlessBrowser uses chromedp to render the label HTML and capture as PNG
func (s *LabelService) renderLabelWithHeadlessBrowser(htmlContent string) (string, error) {
	labels, err := s.renderLabelsWithHeadlessBrowser([]string{htmlContent})
	if err != nil {
		return "", err
	}
	return labels[0], nil
}

// renderLabelsWithHeadlessBrowser reuses one Chromium process for a complete
// batch. Starting Chromium dominates render time, so this is substantially
// faster than rendering every label in an isolated browser process.
func (s *LabelService) renderLabelsWithHeadlessBrowser(htmlContents []string) ([]string, error) {
	if len(htmlContents) == 0 {
		return []string{}, nil
	}
	timeout := max(60*time.Second, time.Duration(len(htmlContents))*10*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create chromedp context with options optimized for Docker
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Headless,
		chromedp.Flag("disable-dev-shm-usage", true), // Critical for Docker - prevents shared memory issues
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("disable-features", "VizDisplayCompositor"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	// Disable verbose logging to reduce noise
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	labels := make([]string, 0, len(htmlContents))
	for index, htmlContent := range htmlContents {
		var dataURL string
		err := chromedp.Run(taskCtx,
			chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(htmlContent))),
			chromedp.WaitVisible("canvas", chromedp.ByQuery),
			chromedp.Poll(`window.labelRendered === true`, nil, chromedp.WithPollingInterval(25*time.Millisecond)),
			chromedp.Evaluate(`document.getElementById('canvas').toDataURL('image/png')`, &dataURL),
		)
		if err != nil {
			return nil, fmt.Errorf("render label %d/%d: %w", index+1, len(htmlContents), err)
		}
		if !strings.HasPrefix(dataURL, "data:image/png;base64,") {
			preview := dataURL
			if len(preview) > 50 {
				preview = preview[:50]
			}
			return nil, fmt.Errorf("label %d returned unexpected data URL format: %q", index+1, preview)
		}
		labels = append(labels, dataURL[22:])
	}
	return labels, nil
}

func (s *LabelService) renderHTMLToPDF(htmlContent string, widthMM, heightMM float64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Headless,
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-setuid-sandbox", true),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	var pdfData []byte
	err := chromedp.Run(taskCtx,
		chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(htmlContent))),
		chromedp.Poll(`window.pdfReady === true`, nil, chromedp.WithPollingInterval(25*time.Millisecond)),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdfData, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(widthMM / 25.4).
				WithPaperHeight(heightMM / 25.4).
				WithMarginTop(0).
				WithMarginBottom(0).
				WithMarginLeft(0).
				WithMarginRight(0).
				WithPreferCSSPageSize(true).
				Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("render label PDF: %w", err)
	}
	if len(pdfData) < 5 || string(pdfData[:5]) != "%PDF-" {
		return nil, fmt.Errorf("label PDF renderer returned invalid data")
	}
	return pdfData, nil
}
