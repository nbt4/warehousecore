package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lib/pq"

	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

// DeviceAdminService provides administrative CRUD helpers for warehouse devices.
type DeviceAdminService struct {
	db           *sql.DB
	labelService *LabelService
}

// NewDeviceAdminService constructs a device admin service using the global repositories.
func NewDeviceAdminService() *DeviceAdminService {
	return &DeviceAdminService{
		db:           repository.GetSQLDB(),
		labelService: NewLabelService(),
	}
}

// CreateDevices inserts one or multiple devices and returns their hydrated representations.
func (s *DeviceAdminService) CreateDevices(ctx context.Context, input *models.DeviceCreateInput) ([]*models.DeviceWithDetails, error) {
	if input == nil {
		return nil, errors.New("input cannot be nil")
	}
	if input.ProductID <= 0 {
		return nil, errors.New("product_id is required")
	}

	if input.Quantity <= 0 {
		input.Quantity = 1
	}

	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "free"
	}

	autoGenerateLabel := true
	if input.AutoGenerateLabel != nil {
		autoGenerateLabel = *input.AutoGenerateLabel
	}

	regenerateCodes := false
	if input.RegenerateCodes != nil {
		regenerateCodes = *input.RegenerateCodes
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	createdIDs := make([]string, 0, input.Quantity)
	providedBarcode := input.Barcode != nil && strings.TrimSpace(*input.Barcode) != ""
	providedQRCode := input.QRCode != nil && strings.TrimSpace(*input.QRCode) != ""

	for i := 0; i < input.Quantity; i++ {
		serialValue := serialForIndex(input.SerialNumber, input.StartingSerial, input.IncrementSerial, i)

		_, err := tx.ExecContext(ctx, `
			INSERT INTO devices (
				productID, serialnumber, status, current_location, zone_id,
				condition_rating, usage_hours, purchaseDate, lastmaintenance, nextmaintenance,
				notes, barcode, qr_code
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		`,
			input.ProductID,
			nullableString(serialValue),
			status,
			nullableString(trimPtr(input.CurrentLocation)),
			nullableInt(input.ZoneID),
			nullableFloat(input.ConditionRating),
			nullableFloat(input.UsageHours),
			parseDatePtr(input.PurchaseDate),
			parseDatePtr(input.LastMaintenance),
			parseDatePtr(input.NextMaintenance),
			nullableText(input.Notes),
			nullableString(trimPtr(input.Barcode)),
			nullableString(trimPtr(input.QRCode)),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert device: %w", err)
		}

		var deviceID string
		err = tx.QueryRowContext(ctx, `
			SELECT deviceID
			FROM devices
			WHERE productID = $1
			ORDER BY deviceID DESC
			LIMIT 1
		`, input.ProductID).Scan(&deviceID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch device id: %w", err)
		}

		if err := s.ensureDeviceCodes(ctx, tx, deviceID, providedBarcode, providedQRCode, regenerateCodes); err != nil {
			return nil, err
		}

		createdIDs = append(createdIDs, deviceID)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit device creation: %w", err)
	}

	devices := make([]*models.DeviceWithDetails, 0, len(createdIDs))
	for _, id := range createdIDs {
		device, err := s.FetchDevice(ctx, id)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}

	// Trigger async label generation after commit
	if autoGenerateLabel || input.LabelTemplateID != nil {
		templateID := input.LabelTemplateID
		for i := range createdIDs {
			deviceID := createdIDs[i]
			go func(id string) {
				if err := s.generateLabelForDevice(id, templateID); err != nil {
					log.Printf("[DEVICE LABEL] Failed to generate label for %s: %v", id, err)
				}
			}(deviceID)
		}
	}

	return devices, nil
}

// UpdateDevice updates an existing device and returns the updated record.
func (s *DeviceAdminService) UpdateDevice(ctx context.Context, deviceID string, input *models.DeviceUpdateInput) (*models.DeviceWithDetails, error) {
	if deviceID == "" {
		return nil, errors.New("deviceID required")
	}
	if input == nil {
		return nil, errors.New("input cannot be nil")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	setClauses := make([]string, 0, 12)
	args := make([]interface{}, 0, 12)

	if input.ProductID.Set {
		setClauses = append(setClauses, fmt.Sprintf("productID = $%d", len(args)+1))
		if input.ProductID.Valid {
			args = append(args, input.ProductID.Value)
		} else {
			args = append(args, nil)
		}
	}

	if input.Status.Set {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", len(args)+1))
		if input.Status.Valid {
			args = append(args, strings.TrimSpace(input.Status.Value))
		} else {
			args = append(args, nil)
		}
	}

	if input.SerialNumber.Set {
		setClauses = append(setClauses, fmt.Sprintf("serialnumber = $%d", len(args)+1))
		if input.SerialNumber.Valid {
			args = append(args, nullableStringPtr(&input.SerialNumber.Value))
		} else {
			args = append(args, nil)
		}
	}

	if input.Barcode.Set {
		setClauses = append(setClauses, fmt.Sprintf("barcode = $%d", len(args)+1))
		if input.Barcode.Valid {
			args = append(args, nullableStringPtr(&input.Barcode.Value))
		} else {
			args = append(args, nil)
		}
	}

	if input.QRCode.Set {
		setClauses = append(setClauses, fmt.Sprintf("qr_code = $%d", len(args)+1))
		if input.QRCode.Valid {
			args = append(args, nullableStringPtr(&input.QRCode.Value))
		} else {
			args = append(args, nil)
		}
	}

	if input.CurrentLocation.Set {
		setClauses = append(setClauses, fmt.Sprintf("current_location = $%d", len(args)+1))
		if input.CurrentLocation.Valid {
			args = append(args, nullableStringPtr(&input.CurrentLocation.Value))
		} else {
			args = append(args, nil)
		}
	}

	if input.ZoneID.Set {
		setClauses = append(setClauses, fmt.Sprintf("zone_id = $%d", len(args)+1))
		if input.ZoneID.Valid {
			id := input.ZoneID.Value
			args = append(args, &id)
		} else {
			args = append(args, nil)
		}
	}

	if input.ConditionRating.Set {
		setClauses = append(setClauses, fmt.Sprintf("condition_rating = $%d", len(args)+1))
		if input.ConditionRating.Valid {
			args = append(args, input.ConditionRating.Value)
		} else {
			args = append(args, nil)
		}
	}

	if input.UsageHours.Set {
		setClauses = append(setClauses, fmt.Sprintf("usage_hours = $%d", len(args)+1))
		if input.UsageHours.Valid {
			args = append(args, input.UsageHours.Value)
		} else {
			args = append(args, nil)
		}
	}

	if input.PurchaseDate.Set {
		setClauses = append(setClauses, fmt.Sprintf("purchaseDate = $%d", len(args)+1))
		if input.PurchaseDate.Valid {
			args = append(args, parseDateValue(input.PurchaseDate.Value))
		} else {
			args = append(args, nil)
		}
	}

	if input.LastMaintenance.Set {
		setClauses = append(setClauses, fmt.Sprintf("lastmaintenance = $%d", len(args)+1))
		if input.LastMaintenance.Valid {
			args = append(args, parseDateValue(input.LastMaintenance.Value))
		} else {
			args = append(args, nil)
		}
	}

	if input.NextMaintenance.Set {
		setClauses = append(setClauses, fmt.Sprintf("nextmaintenance = $%d", len(args)+1))
		if input.NextMaintenance.Valid {
			args = append(args, parseDateValue(input.NextMaintenance.Value))
		} else {
			args = append(args, nil)
		}
	}

	if input.Notes.Set {
		setClauses = append(setClauses, fmt.Sprintf("notes = $%d", len(args)+1))
		if input.Notes.Valid {
			args = append(args, nullableStringPtr(&input.Notes.Value))
		} else {
			args = append(args, nil)
		}
	}

	if len(setClauses) == 0 {
		return nil, errors.New("no fields provided for update")
	}

	query := fmt.Sprintf("UPDATE devices SET %s WHERE deviceID = $%d", strings.Join(setClauses, ", "), len(args)+1)
	args = append(args, deviceID)
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("failed to update device: %w", err)
	}

	shouldResetCodes := input.RegenerateCodes.Set && input.RegenerateCodes.Valid && input.RegenerateCodes.Value
	if shouldResetCodes {
		if err := s.resetDeviceCodes(ctx, tx, deviceID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit device update: %w", err)
	}

	if shouldResetCodes {
		log.Printf("[DEVICE] Regenerated codes for %s", deviceID)
	}

	if input.RegenerateLabel.Set && input.RegenerateLabel.Valid && input.RegenerateLabel.Value {
		templateID := input.LabelTemplateID.Ptr()
		go func() {
			if err := s.generateLabelForDevice(deviceID, templateID); err != nil {
				log.Printf("[DEVICE LABEL] Failed to regenerate label for %s: %v", deviceID, err)
			}
		}()
	}

	return s.FetchDevice(ctx, deviceID)
}

// DeleteDevice removes a device and its label file if no dependencies exist.
func (s *DeviceAdminService) DeleteDevice(ctx context.Context, deviceID string) error {
	if deviceID == "" {
		return errors.New("deviceID required")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var labelPath sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT label_path FROM devices WHERE deviceID = $1`, deviceID).Scan(&labelPath)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to load device: %w", err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM devices WHERE deviceID = $1`, deviceID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" { // foreign_key_violation
			return fmt.Errorf("device %s is still linked to cases, jobs, or history entries", deviceID)
		}
		return fmt.Errorf("failed to delete device: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return repository.ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete: %w", err)
	}

	if labelPath.Valid {
		fullPath := filepath.Join("web", "dist", strings.TrimPrefix(labelPath.String, "/"))
		if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("[DEVICE] Failed to remove label %s: %v", fullPath, err)
		}
	}

	return nil
}

// RegenerateLabel allows manual regeneration of a device label using the default or provided template.
func (s *DeviceAdminService) RegenerateLabel(deviceID string, templateID *int) error {
	if deviceID == "" {
		return errors.New("deviceID required")
	}
	return s.generateLabelForDevice(deviceID, templateID)
}

// FetchDevice loads a device with related metadata for API responses.
func (s *DeviceAdminService) FetchDevice(ctx context.Context, deviceID string) (*models.DeviceWithDetails, error) {
	var device models.DeviceWithDetails
	err := s.db.QueryRowContext(ctx, `
		SELECT d.deviceID, d.productID, d.serialnumber, d.barcode, d.qr_code, d.status,
		       d.current_location, d.zone_id,
		       d.condition_rating, d.usage_hours, d.purchaseDate, d.lastmaintenance, d.nextmaintenance,
		       d.notes, d.label_path,
		       COALESCE(p.name, '') AS product_name,
		       COALESCE(cat.name, '') AS product_category,
		       COALESCE(z.name, '') AS zone_name,
		       COALESCE(z.code, '') AS zone_code,
		       dc.caseID,
		       COALESCE(c.name, '') AS case_name,
		       jd.jobID,
		       COALESCE(j.job_code, '') AS job_number
		FROM devices d
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN categories cat ON p.categoryID = cat.categoryID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		LEFT JOIN devicescases dc ON d.deviceID = dc.deviceID
		LEFT JOIN cases c ON dc.caseID = c.caseID
		LEFT JOIN job_devices jd ON d.deviceID = jd.deviceID
		LEFT JOIN jobs j ON jd.jobID = j.jobID
		WHERE d.deviceID = $1
		LIMIT 1
	`, deviceID).Scan(
		&device.DeviceID,
		&device.ProductID,
		&device.SerialNumber,
		&device.Barcode,
		&device.QRCode,
		&device.Status,
		&device.CurrentLocation,
		&device.ZoneID,
		&device.ConditionRating,
		&device.UsageHours,
		&device.PurchaseDate,
		&device.LastMaintenance,
		&device.NextMaintenance,
		&device.Notes,
		&device.LabelPath,
		&device.ProductName,
		&device.ProductCategory,
		&device.ZoneName,
		&device.ZoneCode,
		&device.CaseID,
		&device.CaseName,
		&device.CurrentJobID,
		&device.JobNumber,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load device %s: %w", deviceID, err)
	}

	return &device, nil
}

func (s *DeviceAdminService) ensureDeviceCodes(ctx context.Context, tx *sql.Tx, deviceID string, hadBarcode bool, hadQR bool, regenerate bool) error {
	if regenerate || !hadBarcode || !hadQR {
		barcode := ""
		qr := ""
		if regenerate || !hadBarcode {
			barcode = deviceID
		}
		if regenerate || !hadQR {
			qr = fmt.Sprintf("QR-%s", deviceID)
		}

		columns := make([]string, 0, 2)
		args := make([]interface{}, 0, 2)

		if regenerate || !hadBarcode {
			columns = append(columns, fmt.Sprintf("barcode = $%d", len(args)+1))
			args = append(args, barcode)
		}
		if regenerate || !hadQR {
			columns = append(columns, fmt.Sprintf("qr_code = $%d", len(args)+1))
			args = append(args, qr)
		}
		if len(columns) == 0 {
			return nil
		}
		_, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE devices SET %s WHERE deviceID = $%d", strings.Join(columns, ", "), len(args)+1), append(args, deviceID)...)
		if err != nil {
			return fmt.Errorf("failed to backfill device codes: %w", err)
		}
	}
	return nil
}

func (s *DeviceAdminService) resetDeviceCodes(ctx context.Context, tx *sql.Tx, deviceID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE devices
		SET barcode = $1, qr_code = $2
		WHERE deviceID = $3
	`, deviceID, fmt.Sprintf("QR-%s", deviceID), deviceID)
	if err != nil {
		return fmt.Errorf("failed to regenerate device codes: %w", err)
	}
	return nil
}

func (s *DeviceAdminService) generateLabelForDevice(deviceID string, templateID *int) error {
	if templateID == nil || *templateID <= 0 {
		return s.labelService.AutoGenerateDeviceLabel(deviceID)
	}
	_, err := s.labelService.RenderTargetLabel(LabelTargetDevice, deviceID, *templateID, true)
	return err
}

// Helper conversions ---------------------------------------------------------

func serialForIndex(base *string, starting *int, increment bool, index int) *string {
	if base == nil || strings.TrimSpace(*base) == "" {
		return nil
	}
	value := strings.TrimSpace(*base)
	if !increment {
		return &value
	}
	start := 1
	if starting != nil && *starting > 0 {
		start = *starting
	}
	serial := fmt.Sprintf("%s-%02d", value, start+index)
	return &serial
}

func nullableString(value *string) interface{} {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullableStringPtr(value *string) interface{} {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullableText(value *string) interface{} {
	if value == nil {
		return nil
	}
	if strings.TrimSpace(*value) == "" {
		return nil
	}
	return *value
}

func nullableInt(value *int) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat(value *float64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func parseDatePtr(value *string) interface{} {
	if value == nil {
		return nil
	}
	return parseDateValue(*value)
}

func parseDateValue(value string) interface{} {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return nil
	}
	return t
}

func trimPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}
