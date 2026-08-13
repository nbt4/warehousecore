package services

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"gorm.io/gorm"

	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

const (
	LabelTargetDevice  = "device"
	LabelTargetProduct = "product"
	LabelTargetCase    = "case"
	LabelTargetZone    = "zone"
)

var safeLabelFilename = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type LabelFieldDefinition struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type LabelTarget struct {
	TargetType string            `json:"target_type"`
	ID         string            `json:"id"`
	Code       string            `json:"code"`
	Name       string            `json:"name"`
	Subtitle   string            `json:"subtitle"`
	LabelPath  string            `json:"label_path,omitempty"`
	IsStale    bool              `json:"is_stale"`
	Fields     map[string]string `json:"fields,omitempty"`
	UpdatedAt  time.Time         `json:"-"`
}

type LabelRenderResult struct {
	Target    LabelTarget           `json:"target"`
	Template  *models.LabelTemplate `json:"template"`
	Elements  []map[string]any      `json:"elements"`
	ImageData string                `json:"image_data,omitempty"`
	LabelPath string                `json:"label_path,omitempty"`
}

type LabelPrintRequest struct {
	TargetType string   `json:"target_type"`
	TargetIDs  []string `json:"target_ids"`
	TemplateID int      `json:"template_id"`
	PrinterID  int      `json:"printer_id"`
	Copies     int      `json:"copies"`
}

func ValidLabelTargetType(targetType string) bool {
	switch targetType {
	case LabelTargetDevice, LabelTargetProduct, LabelTargetCase, LabelTargetZone:
		return true
	default:
		return false
	}
}

func LabelFields(targetType string) []LabelFieldDefinition {
	common := []LabelFieldDefinition{{Key: "code", Label: "Code / Barcode"}, {Key: "name", Label: "Name"}}
	switch targetType {
	case LabelTargetDevice:
		return append(common,
			LabelFieldDefinition{Key: "device_id", Label: "Geräte-ID"},
			LabelFieldDefinition{Key: "product_name", Label: "Produktname"},
			LabelFieldDefinition{Key: "serial_number", Label: "Seriennummer"},
			LabelFieldDefinition{Key: "barcode", Label: "Barcode"},
			LabelFieldDefinition{Key: "status", Label: "Status"},
			LabelFieldDefinition{Key: "zone_code", Label: "Zonen-Code"},
			LabelFieldDefinition{Key: "zone_name", Label: "Zonenname"},
			LabelFieldDefinition{Key: "category", Label: "Kategorie"},
		)
	case LabelTargetProduct:
		return append(common,
			LabelFieldDefinition{Key: "product_id", Label: "Produkt-ID"},
			LabelFieldDefinition{Key: "product_name", Label: "Produktname"},
			LabelFieldDefinition{Key: "generic_barcode", Label: "Artikelbarcode"},
			LabelFieldDefinition{Key: "stock_quantity", Label: "Bestand"},
			LabelFieldDefinition{Key: "unit", Label: "Einheit"},
			LabelFieldDefinition{Key: "category", Label: "Kategorie"},
			LabelFieldDefinition{Key: "description", Label: "Beschreibung"},
		)
	case LabelTargetCase:
		return append(common,
			LabelFieldDefinition{Key: "case_id", Label: "Case-ID"},
			LabelFieldDefinition{Key: "barcode", Label: "Barcode"},
			LabelFieldDefinition{Key: "status", Label: "Status"},
			LabelFieldDefinition{Key: "zone_code", Label: "Zonen-Code"},
			LabelFieldDefinition{Key: "zone_name", Label: "Zonenname"},
			LabelFieldDefinition{Key: "dimensions", Label: "Abmessungen"},
			LabelFieldDefinition{Key: "weight", Label: "Gewicht"},
			LabelFieldDefinition{Key: "description", Label: "Beschreibung"},
		)
	case LabelTargetZone:
		return append(common,
			LabelFieldDefinition{Key: "zone_id", Label: "Zonen-ID"},
			LabelFieldDefinition{Key: "zone_code", Label: "Zonen-Code"},
			LabelFieldDefinition{Key: "barcode", Label: "Barcode"},
			LabelFieldDefinition{Key: "type", Label: "Zonentyp"},
			LabelFieldDefinition{Key: "location", Label: "Standort"},
			LabelFieldDefinition{Key: "capacity", Label: "Kapazität"},
			LabelFieldDefinition{Key: "description", Label: "Beschreibung"},
		)
	default:
		return nil
	}
}

func (s *LabelService) ListTargets(targetType, search string, limit int) ([]LabelTarget, error) {
	if !ValidLabelTargetType(targetType) {
		return nil, fmt.Errorf("unsupported target type %q", targetType)
	}
	if limit <= 0 || limit > 1000 {
		limit = 250
	}

	var query string
	switch targetType {
	case LabelTargetDevice:
		query = `SELECT d.deviceid::text, COALESCE(NULLIF(d.barcode, ''), d.deviceid), COALESCE(p.name, d.deviceid),
			COALESCE(d.serialnumber, z.name, ''), COALESCE(a.label_path, d.label_path, ''),
			COALESCE(d.updated_at, d.created_at), a.generated_at, a.source_updated_at,
			COALESCE(a.template_revision, 0), COALESCE(t.revision, 0)
		FROM devices d LEFT JOIN products p ON p.productid = d.productid
		LEFT JOIN storage_zones z ON z.zone_id = d.zone_id
		LEFT JOIN label_assets a ON a.target_type = 'device' AND a.target_id = d.deviceid
		LEFT JOIN label_templates t ON t.target_type = 'device' AND t.is_default
		WHERE ($1 = '' OR d.deviceid ILIKE '%' || $1 || '%' OR COALESCE(d.barcode, '') ILIKE '%' || $1 || '%' OR COALESCE(p.name, '') ILIKE '%' || $1 || '%')
		ORDER BY p.name NULLS LAST, d.deviceid LIMIT $2`
	case LabelTargetProduct:
		query = `SELECT p.productid::text, COALESCE(NULLIF(p.generic_barcode, ''), 'PROD-' || LPAD(p.productid::text, 6, '0')), p.name,
			COALESCE(c.name, ''), COALESCE(a.label_path, ''), COALESCE(p.updated_at, p.created_at), a.generated_at, a.source_updated_at,
			COALESCE(a.template_revision, 0), COALESCE(t.revision, 0)
		FROM products p LEFT JOIN categories c ON c.categoryid = p.categoryid
		LEFT JOIN label_assets a ON a.target_type = 'product' AND a.target_id = p.productid::text
		LEFT JOIN label_templates t ON t.target_type = 'product' AND t.is_default
		WHERE ($1 = '' OR p.name ILIKE '%' || $1 || '%' OR COALESCE(p.generic_barcode, '') ILIKE '%' || $1 || '%')
		ORDER BY p.name LIMIT $2`
	case LabelTargetCase:
		query = `SELECT c.caseid::text, COALESCE(NULLIF(c.barcode, ''), 'CASE-' || c.caseid::text), c.name,
			COALESCE(z.name, c.status, ''), COALESCE(a.label_path, c.label_path, ''), COALESCE(c.updated_at, c.created_at), a.generated_at, a.source_updated_at,
			COALESCE(a.template_revision, 0), COALESCE(t.revision, 0)
		FROM cases c LEFT JOIN storage_zones z ON z.zone_id = c.zone_id
		LEFT JOIN label_assets a ON a.target_type = 'case' AND a.target_id = c.caseid::text
		LEFT JOIN label_templates t ON t.target_type = 'case' AND t.is_default
		WHERE ($1 = '' OR c.name ILIKE '%' || $1 || '%' OR COALESCE(c.barcode, '') ILIKE '%' || $1 || '%')
		ORDER BY c.name LIMIT $2`
	case LabelTargetZone:
		query = `SELECT z.zone_id::text, COALESCE(NULLIF(z.barcode, ''), z.code), z.name, z.code,
			COALESCE(a.label_path, z.label_url, ''), COALESCE(z.updated_at, z.created_at), a.generated_at, a.source_updated_at,
			COALESCE(a.template_revision, 0), COALESCE(t.revision, 0)
		FROM storage_zones z
		LEFT JOIN label_assets a ON a.target_type = 'zone' AND a.target_id = z.zone_id::text
		LEFT JOIN label_templates t ON t.target_type = 'zone' AND t.is_default
		WHERE ($1 = '' OR z.name ILIKE '%' || $1 || '%' OR z.code ILIKE '%' || $1 || '%' OR COALESCE(z.barcode, '') ILIKE '%' || $1 || '%')
		ORDER BY z.code LIMIT $2`
	}

	rows, err := repository.GetSQLDB().Query(query, strings.TrimSpace(search), limit)
	if err != nil {
		return nil, fmt.Errorf("list label targets: %w", err)
	}
	defer rows.Close()

	targets := make([]LabelTarget, 0)
	for rows.Next() {
		var target LabelTarget
		var generatedAt, sourceUpdatedAt sql.NullTime
		var assetRevision, templateRevision int
		if err := rows.Scan(&target.ID, &target.Code, &target.Name, &target.Subtitle, &target.LabelPath, &target.UpdatedAt, &generatedAt, &sourceUpdatedAt, &assetRevision, &templateRevision); err != nil {
			return nil, fmt.Errorf("scan label target: %w", err)
		}
		target.TargetType = targetType
		target.IsStale = target.LabelPath == "" || assetRevision < templateRevision || !sourceUpdatedAt.Valid || target.UpdatedAt.After(sourceUpdatedAt.Time)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate label targets: %w", err)
	}
	return targets, nil
}

func (s *LabelService) GetTarget(targetType, targetID string) (LabelTarget, error) {
	if !ValidLabelTargetType(targetType) {
		return LabelTarget{}, fmt.Errorf("unsupported target type %q", targetType)
	}

	var query string
	switch targetType {
	case LabelTargetDevice:
		query = `SELECT jsonb_build_object(
			'device_id', d.deviceid, 'code', COALESCE(NULLIF(d.barcode, ''), d.deviceid),
			'barcode', COALESCE(NULLIF(d.barcode, ''), d.deviceid), 'name', COALESCE(p.name, d.deviceid),
			'product_name', COALESCE(p.name, ''), 'serial_number', COALESCE(d.serialnumber, ''),
			'status', COALESCE(d.status, ''), 'zone_code', COALESCE(z.code, ''), 'zone_name', COALESCE(z.name, ''),
			'category', COALESCE(c.name, ''))::text,
			COALESCE(d.updated_at, d.created_at), COALESCE(a.label_path, d.label_path, '')
		FROM devices d LEFT JOIN products p ON p.productid = d.productid
		LEFT JOIN categories c ON c.categoryid = p.categoryid LEFT JOIN storage_zones z ON z.zone_id = d.zone_id
		LEFT JOIN label_assets a ON a.target_type = 'device' AND a.target_id = d.deviceid WHERE d.deviceid = $1`
	case LabelTargetProduct:
		query = `SELECT jsonb_build_object(
			'product_id', p.productid::text, 'code', COALESCE(NULLIF(p.generic_barcode, ''), 'PROD-' || LPAD(p.productid::text, 6, '0')),
			'barcode', COALESCE(NULLIF(p.generic_barcode, ''), 'PROD-' || LPAD(p.productid::text, 6, '0')),
			'generic_barcode', COALESCE(NULLIF(p.generic_barcode, ''), 'PROD-' || LPAD(p.productid::text, 6, '0')),
			'name', p.name, 'product_name', p.name, 'stock_quantity', COALESCE(p.stock_quantity::text, '0'),
			'unit', COALESCE(ct.abbreviation, ''), 'category', COALESCE(c.name, ''), 'description', COALESCE(p.description, ''))::text,
			COALESCE(p.updated_at, p.created_at), COALESCE(a.label_path, '')
		FROM products p LEFT JOIN count_types ct ON ct.count_type_id = p.count_type_id
		LEFT JOIN categories c ON c.categoryid = p.categoryid
		LEFT JOIN label_assets a ON a.target_type = 'product' AND a.target_id = p.productid::text WHERE p.productid::text = $1`
	case LabelTargetCase:
		query = `SELECT jsonb_build_object(
			'case_id', 'CASE-' || c.caseid::text, 'code', COALESCE(NULLIF(c.barcode, ''), 'CASE-' || c.caseid::text),
			'barcode', COALESCE(NULLIF(c.barcode, ''), 'CASE-' || c.caseid::text), 'name', c.name,
			'status', COALESCE(c.status, ''), 'zone_code', COALESCE(z.code, ''), 'zone_name', COALESCE(z.name, ''),
			'dimensions', CONCAT_WS(' × ', c.width::text, c.height::text, c.depth::text),
			'weight', CASE WHEN c.weight IS NULL THEN '' ELSE c.weight::text || ' kg' END,
			'description', COALESCE(c.description, ''))::text,
			COALESCE(c.updated_at, c.created_at), COALESCE(a.label_path, c.label_path, '')
		FROM cases c LEFT JOIN storage_zones z ON z.zone_id = c.zone_id
		LEFT JOIN label_assets a ON a.target_type = 'case' AND a.target_id = c.caseid::text WHERE c.caseid::text = $1`
	case LabelTargetZone:
		query = `SELECT jsonb_build_object(
			'zone_id', z.zone_id::text, 'code', COALESCE(NULLIF(z.barcode, ''), z.code),
			'barcode', COALESCE(NULLIF(z.barcode, ''), z.code), 'name', z.name, 'zone_code', z.code,
			'type', z.type::text, 'location', COALESCE(z.location, ''), 'capacity', COALESCE(z.capacity::text, ''),
			'description', COALESCE(z.description, ''))::text,
			COALESCE(z.updated_at, z.created_at), COALESCE(a.label_path, z.label_url, '')
		FROM storage_zones z LEFT JOIN label_assets a ON a.target_type = 'zone' AND a.target_id = z.zone_id::text WHERE z.zone_id::text = $1`
	}

	var fieldsJSON string
	var target LabelTarget
	if err := repository.GetSQLDB().QueryRow(query, targetID).Scan(&fieldsJSON, &target.UpdatedAt, &target.LabelPath); err != nil {
		if err == sql.ErrNoRows {
			return LabelTarget{}, fmt.Errorf("%s %q not found", targetType, targetID)
		}
		return LabelTarget{}, fmt.Errorf("load label target: %w", err)
	}
	if err := json.Unmarshal([]byte(fieldsJSON), &target.Fields); err != nil {
		return LabelTarget{}, fmt.Errorf("decode label target fields: %w", err)
	}
	target.TargetType = targetType
	target.ID = targetID
	target.Code = target.Fields["code"]
	target.Name = target.Fields["name"]
	return target, nil
}

func (s *LabelService) defaultTemplate(targetType string) (*models.LabelTemplate, error) {
	var template models.LabelTemplate
	if err := repository.GetDB().Where("target_type = ? AND is_default = ?", targetType, true).First(&template).Error; err != nil {
		return nil, fmt.Errorf("no default template configured for %s", targetType)
	}
	return &template, nil
}

func (s *LabelService) GenerateLabelForTarget(targetType, targetID string, templateID int) (*LabelRenderResult, error) {
	target, err := s.GetTarget(targetType, targetID)
	if err != nil {
		return nil, err
	}
	var template *models.LabelTemplate
	if templateID > 0 {
		template, err = s.GetTemplateByID(templateID)
	} else {
		template, err = s.defaultTemplate(targetType)
	}
	if err != nil {
		return nil, err
	}
	if template.TargetType != targetType {
		return nil, fmt.Errorf("template %d belongs to %s labels", template.ID, template.TargetType)
	}

	var elements []models.LabelElement
	if err := json.Unmarshal([]byte(template.TemplateJSON), &elements); err != nil {
		return nil, fmt.Errorf("invalid template JSON: %w", err)
	}
	processed := make([]map[string]any, 0, len(elements))
	for _, element := range elements {
		content := element.Content
		if value, ok := target.Fields[element.Content]; ok {
			content = value
		}
		item := map[string]any{
			"type": element.Type, "x": element.X, "y": element.Y, "width": element.Width,
			"height": element.Height, "rotation": element.Rotation, "style": element.Style, "content": content,
		}
		switch element.Type {
		case "qrcode":
			size := max(100, int(element.Width*300/25.4))
			item["image_data"], err = s.GenerateQRCode(content, size)
		case "barcode":
			width := max(123, int(element.Width*300/25.4))
			height := max(50, int(element.Height*300/25.4))
			item["image_data"], err = s.GenerateBarcode(content, width, height)
		case "image":
			item["image_data"] = element.ImageData
		}
		if err != nil {
			return nil, err
		}
		processed = append(processed, item)
	}
	return &LabelRenderResult{Target: target, Template: template, Elements: processed}, nil
}

func (s *LabelService) RenderTargetLabel(targetType, targetID string, templateID int, save bool) (*LabelRenderResult, error) {
	result, err := s.GenerateLabelForTarget(targetType, targetID, templateID)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{"template": result.Template, "elements": result.Elements, "target": result.Target})
	if err != nil {
		return nil, fmt.Errorf("encode label render data: %w", err)
	}
	htmlTemplate, err := os.ReadFile("./internal/services/label_template.html")
	if err != nil {
		return nil, fmt.Errorf("load label renderer: %w", err)
	}
	htmlContent := strings.Replace(string(htmlTemplate), "{{LABEL_DATA_JSON}}", string(payload), 1)
	base64PNG, err := s.renderLabelWithHeadlessBrowser(htmlContent)
	if err != nil {
		return nil, err
	}
	result.ImageData = "data:image/png;base64," + base64PNG
	if save {
		result.LabelPath, err = s.saveTargetLabel(result, base64PNG)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *LabelService) saveTargetLabel(result *LabelRenderResult, base64PNG string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(base64PNG)
	if err != nil {
		return "", fmt.Errorf("decode rendered label: %w", err)
	}
	safeID := safeLabelFilename.ReplaceAllString(result.Target.ID, "_")
	dir := filepath.Join("web", "dist", "labels", result.Target.TargetType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create label directory: %w", err)
	}
	filename := fmt.Sprintf("%s_r%d.png", safeID, result.Template.Revision)
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0644); err != nil {
		return "", fmt.Errorf("save rendered label: %w", err)
	}
	path := "/labels/" + result.Target.TargetType + "/" + filename
	db := repository.GetSQLDB()
	_, err = db.Exec(`INSERT INTO label_assets
		(target_type, target_id, template_id, template_revision, source_updated_at, label_path, generated_at)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (target_type, target_id) DO UPDATE SET
		template_id = EXCLUDED.template_id, template_revision = EXCLUDED.template_revision,
		source_updated_at = EXCLUDED.source_updated_at, label_path = EXCLUDED.label_path, generated_at = CURRENT_TIMESTAMP`,
		result.Target.TargetType, result.Target.ID, result.Template.ID, result.Template.Revision, result.Target.UpdatedAt, path)
	if err != nil {
		return "", fmt.Errorf("record label asset: %w", err)
	}
	switch result.Target.TargetType {
	case LabelTargetDevice:
		_, err = db.Exec(`UPDATE devices SET label_path = $1 WHERE deviceid = $2`, path, result.Target.ID)
	case LabelTargetCase:
		_, err = db.Exec(`UPDATE cases SET label_path = $1 WHERE caseid::text = $2`, path, result.Target.ID)
	case LabelTargetZone:
		_, err = db.Exec(`UPDATE storage_zones SET label_url = $1 WHERE zone_id::text = $2`, path, result.Target.ID)
	}
	if err != nil {
		return "", fmt.Errorf("update target label path: %w", err)
	}
	return path, nil
}

func (s *LabelService) ListPrinters() ([]models.LabelPrinter, error) {
	printers := make([]models.LabelPrinter, 0)
	if err := repository.GetDB().Order("is_default DESC, name ASC").Find(&printers).Error; err != nil {
		return nil, fmt.Errorf("list label printers: %w", err)
	}
	return printers, nil
}

func (s *LabelService) SavePrinter(printer *models.LabelPrinter) error {
	printer.Name = strings.TrimSpace(printer.Name)
	printer.Address = strings.TrimSpace(printer.Address)
	if printer.Name == "" || printer.Address == "" {
		return fmt.Errorf("name and address are required")
	}
	if printer.Driver == "" {
		printer.Driver = "zpl_tcp"
	}
	if printer.Driver != "zpl_tcp" {
		return fmt.Errorf("unsupported printer driver %q", printer.Driver)
	}
	if printer.Port == 0 {
		printer.Port = 9100
	}
	if printer.Port < 1 || printer.Port > 65535 {
		return fmt.Errorf("printer port must be between 1 and 65535")
	}
	if printer.DPI == 0 {
		printer.DPI = 203
	}
	if printer.DPI != 203 && printer.DPI != 300 && printer.DPI != 600 {
		return fmt.Errorf("printer DPI must be 203, 300 or 600")
	}
	db := repository.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		if printer.IsDefault {
			if err := tx.Model(&models.LabelPrinter{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if printer.ID == 0 {
			return tx.Create(printer).Error
		}
		return tx.Model(&models.LabelPrinter{}).Where("id = ?", printer.ID).Updates(map[string]any{
			"name": printer.Name, "driver": printer.Driver, "address": printer.Address, "port": printer.Port,
			"dpi": printer.DPI, "is_default": printer.IsDefault, "is_active": printer.IsActive, "updated_at": time.Now(),
		}).Error
	})
}

func (s *LabelService) DeletePrinter(id int) error {
	result := repository.GetDB().Delete(&models.LabelPrinter{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("printer not found")
	}
	return nil
}

func (s *LabelService) ListPrintJobs(limit int) ([]models.LabelPrintJob, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	jobs := make([]models.LabelPrintJob, 0)
	err := repository.GetDB().Table("label_print_jobs j").
		Select("j.*, COALESCE(p.name, '') AS printer_name").
		Joins("LEFT JOIN label_printers p ON p.id = j.printer_id").
		Order("j.created_at DESC").Limit(limit).Scan(&jobs).Error
	return jobs, err
}

func (s *LabelService) PrintTargets(request LabelPrintRequest) ([]models.LabelPrintJob, error) {
	if !ValidLabelTargetType(request.TargetType) || len(request.TargetIDs) == 0 {
		return nil, fmt.Errorf("target type and at least one target are required")
	}
	if request.Copies <= 0 || request.Copies > 1000 {
		return nil, fmt.Errorf("copies must be between 1 and 1000")
	}
	var printer models.LabelPrinter
	query := repository.GetDB().Where("is_active = ?", true)
	if request.PrinterID > 0 {
		query = query.Where("id = ?", request.PrinterID)
	} else {
		query = query.Where("is_default = ?", true)
	}
	if err := query.First(&printer).Error; err != nil {
		return nil, fmt.Errorf("active printer not found")
	}

	jobs := make([]models.LabelPrintJob, 0, len(request.TargetIDs))
	for _, targetID := range request.TargetIDs {
		templateID := request.TemplateID
		printerID := printer.ID
		job := models.LabelPrintJob{TargetType: request.TargetType, TargetID: targetID, TemplateID: &templateID, PrinterID: &printerID, Copies: request.Copies, Status: "queued", PrinterName: printer.Name}
		if err := repository.GetDB().Create(&job).Error; err != nil {
			return jobs, fmt.Errorf("create print job: %w", err)
		}
		now := time.Now()
		job.Status = "printing"
		job.StartedAt = &now
		repository.GetDB().Model(&models.LabelPrintJob{}).Where("id = ?", job.ID).Updates(map[string]any{"status": job.Status, "started_at": now})

		result, renderErr := s.RenderTargetLabel(request.TargetType, targetID, request.TemplateID, true)
		if renderErr == nil {
			var pngData []byte
			pngData, renderErr = base64.StdEncoding.DecodeString(strings.TrimPrefix(result.ImageData, "data:image/png;base64,"))
			if renderErr == nil {
				var zpl string
				zpl, renderErr = encodePNGAsZPL(pngData, result.Template.Width, result.Template.Height, printer.DPI, request.Copies)
				if renderErr == nil {
					renderErr = sendRawPrinterData(printer.Address, printer.Port, []byte(zpl))
				}
			}
			job.LabelPath = result.LabelPath
		}
		finished := time.Now()
		job.CompletedAt = &finished
		if renderErr != nil {
			job.Status = "failed"
			job.ErrorMessage = renderErr.Error()
		} else {
			job.Status = "completed"
		}
		repository.GetDB().Model(&models.LabelPrintJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status": job.Status, "label_path": job.LabelPath, "error_message": job.ErrorMessage, "completed_at": finished,
		})
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func encodePNGAsZPL(pngData []byte, widthMM, heightMM float64, dpi, copies int) (string, error) {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return "", fmt.Errorf("decode label PNG: %w", err)
	}
	width := max(1, int(widthMM*float64(dpi)/25.4+0.5))
	height := max(1, int(heightMM*float64(dpi)/25.4+0.5))
	resized := imaging.Resize(img, width, height, imaging.Lanczos)
	bytesPerRow := (width + 7) / 8
	raster := make([]byte, bytesPerRow*height)
	for y := range height {
		for x := range width {
			r, g, b, a := resized.At(x, y).RGBA()
			gray := (299*r + 587*g + 114*b) / 1000
			if a > 0x4000 && gray < 0x8000 {
				raster[y*bytesPerRow+x/8] |= 1 << (7 - uint(x%8))
			}
		}
	}
	hexData := strings.ToUpper(hex.EncodeToString(raster))
	return fmt.Sprintf("^XA^PW%d^LL%d^LH0,0^FO0,0^GFA,%d,%d,%d,%s^FS^PQ%d^XZ\n", width, height, len(raster), len(raster), bytesPerRow, hexData, copies), nil
}

func sendRawPrinterData(address string, port int, payload []byte) error {
	endpoint := net.JoinHostPort(address, strconv.Itoa(port))
	connection, err := net.DialTimeout("tcp", endpoint, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect to printer %s: %w", endpoint, err)
	}
	defer connection.Close()
	if err := connection.SetWriteDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return err
	}
	if _, err := connection.Write(payload); err != nil {
		return fmt.Errorf("send label to printer: %w", err)
	}
	return nil
}
