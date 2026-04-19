package led

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gorm.io/gorm"

	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

//go:embed config/led_mapping.json
var defaultMappingData []byte

// Service handles LED-related business logic
type Service struct {
	mapping     *LEDMapping
	mappingPath string
	publisher   *Publisher
	mu          sync.RWMutex
}

type controllerCaches struct {
	zoneTypeIDs map[string]int
	controllers map[int]*models.LEDController
}

func newControllerCaches() *controllerCaches {
	return &controllerCaches{
		zoneTypeIDs: make(map[string]int),
		controllers: make(map[int]*models.LEDController),
	}
}

const defaultControllerKey = "__default__"

type highlightGroup struct {
	controller *models.LEDController
	shelves    map[string]*Shelf
}

func newHighlightGroup(controller *models.LEDController) *highlightGroup {
	return &highlightGroup{
		controller: controller,
		shelves:    make(map[string]*Shelf),
	}
}

func (g *highlightGroup) addBin(shelfID string, bin Bin) {
	shelf := g.shelves[shelfID]
	if shelf == nil {
		shelf = &Shelf{ShelfID: shelfID}
		g.shelves[shelfID] = shelf
	}
	shelf.Bins = append(shelf.Bins, bin)
}

func (g *highlightGroup) toCommand(warehouseID string) LEDCommand {
	cmd := LEDCommand{
		Op:          "highlight",
		WarehouseID: warehouseID,
	}
	for _, shelf := range g.shelves {
		binCopy := make([]Bin, len(shelf.Bins))
		copy(binCopy, shelf.Bins)
		cmd.Shelves = append(cmd.Shelves, Shelf{
			ShelfID: shelf.ShelfID,
			Bins:    binCopy,
		})
	}
	return cmd
}

func (g *highlightGroup) binCount() int {
	total := 0
	for _, shelf := range g.shelves {
		total += len(shelf.Bins)
	}
	return total
}

var (
	serviceInstance *Service
	serviceOnce     sync.Once
)

// GetService returns the singleton LED service instance
func GetService() *Service {
	serviceOnce.Do(func() {
		serviceInstance = NewService()
	})
	return serviceInstance
}

// NewService creates a new LED service
func NewService() *Service {
	s := &Service{
		publisher: GetPublisher(),
	}

	mappingPath := os.Getenv("LED_MAPPING_FILE")
	if mappingPath == "" {
		mappingPath = "internal/led/config/led_mapping.json"
	}

	if err := s.ensureMappingFile(mappingPath); err != nil {
		log.Printf("[LED] Failed to prepare mapping file: %v", err)
	}

	// Load mapping configuration
	if err := s.LoadMapping(mappingPath); err != nil {
		log.Printf("[LED] Failed to load mapping: %v", err)
	}

	return s
}

func (s *Service) getJobHighlightSettings() *models.LEDJobHighlightSettings {
	defaults := models.DefaultLEDJobHighlightSettings()

	gormDB := repository.GetDB()
	if gormDB == nil {
		return defaults
	}

	var setting models.AppSetting
	if err := gormDB.Where("scope = ? AND k = ?", "warehousecore", "led.job.highlight").First(&setting).Error; err != nil {
		return defaults
	}

	bytes, err := json.Marshal(setting.Value)
	if err != nil {
		return defaults
	}

	var cfg models.LEDJobHighlightSettings
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		return defaults
	}

	cfg.Normalize(defaults)
	return &cfg
}

// LoadMapping loads the LED mapping configuration from file
func (s *Service) LoadMapping(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mappingPath = filename

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read mapping file: %w", err)
	}

	var mapping LEDMapping
	if err := json.Unmarshal(data, &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping JSON: %w", err)
	}

	s.mapping = &mapping
	log.Printf("[LED] Loaded mapping: %d shelves, %d total bins", len(mapping.Shelves), s.countTotalBins())
	return nil
}

// SaveMapping saves the LED mapping configuration to file
func (s *Service) SaveMapping() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.mapping == nil {
		return fmt.Errorf("no mapping loaded")
	}
	if s.mappingPath == "" {
		return fmt.Errorf("mapping path not set")
	}

	data, err := json.MarshalIndent(s.mapping, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal mapping: %w", err)
	}

	if err := os.WriteFile(s.mappingPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write mapping file: %w", err)
	}

	return nil
}

// GetMapping returns a copy of the current mapping
func (s *Service) GetMapping() (*LEDMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.mapping == nil {
		return nil, fmt.Errorf("no mapping loaded")
	}

	// Return a copy to prevent external modifications
	mappingCopy := *s.mapping
	return &mappingCopy, nil
}

// UpdateMapping updates the mapping configuration
func (s *Service) UpdateMapping(mapping *LEDMapping) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mapping = mapping
	log.Printf("[LED] Mapping updated: %d shelves, %d bins", len(mapping.Shelves), s.countTotalBins())
	return nil
}

func (s *Service) ensureMappingFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create mapping directory: %w", err)
	}

	data := defaultMappingData
	if len(data) == 0 {
		// Fallback minimal mapping if embed fails for some reason
		data = []byte(`{"warehouse_id":"default","shelves":[],"led_strip":{"length":600,"data_pin":5,"chipset":"SK6812_GRBW"},"defaults":{"color":"#FF0000","pattern":"solid","intensity":200,"speed":1200}}`)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write default mapping: %w", err)
	}

	return nil
}

// HighlightJobBins highlights bins for devices in a specific job
func (s *Service) HighlightJobBins(jobID string) error {
	zoneCounts, err := s.getJobDeviceZonesWithCounts(jobID)
	if err != nil {
		return fmt.Errorf("failed to get job devices: %w", err)
	}
	if len(zoneCounts) == 0 {
		return fmt.Errorf("no devices found for job %s", jobID)
	}

	s.mu.RLock()
	mapping := s.mapping
	s.mu.RUnlock()
	if mapping == nil {
		return fmt.Errorf("no mapping loaded")
	}

	settings := s.getJobHighlightSettings()
	if settings == nil {
		settings = models.DefaultLEDJobHighlightSettings()
	} else {
		settings.Normalize(models.DefaultLEDJobHighlightSettings())
	}

	jobZones := make(map[string]bool)
	for zoneCode := range zoneCounts {
		jobZones[zoneCode] = true
	}

	caches := newControllerCaches()
	groups := make(map[string]*highlightGroup)

	for _, shelf := range mapping.Shelves {
		for _, binConfig := range shelf.Bins {
			hasJobDevice := jobZones[binConfig.BinID]

			appearance := settings.NonRequired
			if hasJobDevice {
				appearance = settings.Required
			} else if settings.Mode == "required_only" {
				continue
			}

			ledBin := Bin{
				BinID:     binConfig.BinID,
				Pixels:    binConfig.Pixels,
				Color:     appearance.Color,
				Pattern:   appearance.Pattern,
				Intensity: int(appearance.Intensity),
			}
			if appearance.Speed > 0 {
				ledBin.Speed = appearance.Speed
			}

			controller, err := s.resolveControllerForZone(binConfig.BinID, caches)
			if err != nil {
				return err
			}
			key := defaultControllerKey
			if controller != nil {
				key = controller.ControllerID
			}
			group := groups[key]
			if group == nil {
				group = newHighlightGroup(controller)
				groups[key] = group
			}
			group.addBin(shelf.ShelfID, ledBin)
		}
	}

	if len(groups) == 0 {
		return fmt.Errorf("no bins configured in LED mapping")
	}

	if settings.Mode == "required_only" {
		for _, group := range groups {
			if err := s.publishClearForGroup(group, mapping.WarehouseID); err != nil {
				log.Printf("[LED] Failed to clear LEDs for controller group: %v", err)
			}
		}
	}

	requiredBins := len(zoneCounts)
	totalBins := 0

	for _, group := range groups {
		cmd := group.toCommand(mapping.WarehouseID)
		var err error
		if group.controller != nil {
			err = s.publisher.PublishCommandToController(group.controller, cmd)
		} else {
			err = s.publisher.PublishCommand(cmd)
		}
		if err != nil {
			return err
		}
		totalBins += group.binCount()
	}

	if settings.Mode == "required_only" {
		log.Printf("[LED] Highlighting %d required bins only", requiredBins)
	} else {
		log.Printf("[LED] Highlighting %d bins total (%d required, %d non-required)", totalBins, requiredBins, totalBins-requiredBins)
	}

	return nil
}

// ClearAllLEDs turns off all LEDs (multi-controller aware)
func (s *Service) ClearAllLEDs() error {
	s.mu.RLock()
	mapping := s.mapping
	s.mu.RUnlock()

	// Get all active LED controllers from database
	db := repository.GetDB()
	if db == nil {
		// Fallback: send clear to default topic only
		log.Println("[LED] Database not available, clearing default controller only")
		return s.publisher.PublishClear()
	}

	var controllers []models.LEDController
	if err := db.Where("is_active = TRUE").Find(&controllers).Error; err != nil {
		log.Printf("[LED] Failed to query controllers: %v, clearing default only", err)
		return s.publisher.PublishClear()
	}

	// If no controllers configured, clear default
	if len(controllers) == 0 {
		log.Println("[LED] No active controllers found, clearing default controller")
		return s.publisher.PublishClear()
	}

	// Send clear command to all active controllers
	warehouseID := ""
	if mapping != nil {
		warehouseID = mapping.WarehouseID
	}

	clearCmd := LEDCommand{Op: "clear", WarehouseID: warehouseID}
	errCount := 0

	for _, controller := range controllers {
		if err := s.publisher.PublishCommandToController(&controller, clearCmd); err != nil {
			log.Printf("[LED] Failed to clear LEDs for controller %s: %v", controller.ControllerID, err)
			errCount++
		} else {
			log.Printf("[LED] Cleared LEDs for controller %s (%s)", controller.ControllerID, controller.DisplayName)
		}
	}

	// Also clear default topic for backwards compatibility
	if err := s.publisher.PublishClear(); err != nil {
		log.Printf("[LED] Failed to clear default topic: %v", err)
		errCount++
	}

	if errCount > 0 {
		return fmt.Errorf("failed to clear LEDs for %d controller(s)", errCount)
	}

	return nil
}

// IdentifyController sends identify command to test LEDs
func (s *Service) IdentifyController() error {
	return s.publisher.PublishIdentify()
}

// TestBin sends a test command for a specific bin
func (s *Service) TestBin(shelfID, binID string) error {
	s.mu.RLock()
	mapping := s.mapping
	s.mu.RUnlock()

	if mapping == nil {
		return fmt.Errorf("no mapping loaded")
	}

	// Find bin in mapping
	var pixels []int
	found := false
	for _, shelf := range mapping.Shelves {
		if shelf.ShelfID == shelfID {
			for _, bin := range shelf.Bins {
				if bin.BinID == binID {
					pixels = bin.Pixels
					found = true
					break
				}
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("bin %s not found in shelf %s", binID, shelfID)
	}

	// Create test command (blink pattern)
	cmd := LEDCommand{
		Op:          "highlight",
		WarehouseID: mapping.WarehouseID,
		Shelves: []Shelf{
			{
				ShelfID: shelfID,
				Bins: []Bin{
					{
						BinID:     binID,
						Pixels:    pixels,
						Color:     "#00FF00", // Green for test
						Pattern:   "blink",
						Intensity: 255,
					},
				},
			},
		},
	}

	return s.publishCommandForBin(binID, cmd, newControllerCaches())
}

// LocateBin highlights a single bin with configurable pattern to help locate it
func (s *Service) LocateBin(binCode string) error {
	s.mu.RLock()
	mapping := s.mapping
	s.mu.RUnlock()

	if mapping == nil {
		return fmt.Errorf("no mapping loaded")
	}

	// Find bin by code (binCode is the zone code like "WDL-06-RG-02-F-01")
	var pixels []int
	var shelfID string
	found := false
	for _, shelf := range mapping.Shelves {
		for _, bin := range shelf.Bins {
			if bin.BinID == binCode {
				pixels = bin.Pixels
				shelfID = shelf.ShelfID
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("bin with code %s not found in LED mapping", binCode)
	}

	// Get LED defaults from settings (with fallback to defaults)
	color := "#FF7A00"
	pattern := "breathe"
	intensity := uint8(180)

	// Try to load from app_settings
	gormDB := repository.GetDB()
	if gormDB != nil {
		var setting models.AppSetting
		if err := gormDB.Where("scope = ? AND k = ?", "warehousecore", "led.single_bin.default").First(&setting).Error; err == nil {
			// Parse JSON to defaults
			bytes, _ := json.Marshal(setting.Value)
			var defaults map[string]interface{}
			if err := json.Unmarshal(bytes, &defaults); err == nil {
				if c, ok := defaults["color"].(string); ok {
					color = c
				}
				if p, ok := defaults["pattern"].(string); ok {
					pattern = p
				}
				if i, ok := defaults["intensity"].(float64); ok {
					intensity = uint8(i)
				}
			}
		}
	}

	// Create locate command with configurable settings
	cmd := LEDCommand{
		Op:          "highlight",
		WarehouseID: mapping.WarehouseID,
		Shelves: []Shelf{
			{
				ShelfID: shelfID,
				Bins: []Bin{
					{
						BinID:     binCode,
						Pixels:    pixels,
						Color:     color,
						Pattern:   pattern,
						Intensity: int(intensity),
					},
				},
			},
		},
	}

	log.Printf("[LED] Locating bin %s with %s %s pattern (intensity: %d)", binCode, color, pattern, intensity)
	return s.publishCommandForBin(binCode, cmd, newControllerCaches())
}

// GetStatus returns the current LED system status
func (s *Service) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"mqtt_connected": s.publisher.IsConnected(),
		"mqtt_dry_run":   s.publisher.IsDryRun(),
		"mapping_loaded": s.mapping != nil,
		"warehouse_id":   "",
		"total_shelves":  0,
		"total_bins":     0,
	}

	if s.mapping != nil {
		status["warehouse_id"] = s.mapping.WarehouseID
		status["total_shelves"] = len(s.mapping.Shelves)
		status["total_bins"] = s.countTotalBins()
	}

	return status
}

// UpdateBinAfterScan refreshes ALL bins for a job after a device scan
// This ensures all bins show correct colors: GREEN (has devices) or RED (empty/complete)
func (s *Service) UpdateBinAfterScan(jobID string, zoneCode string) error {
	if jobID == "" {
		return nil
	}

	log.Printf("[LED] Refreshing all bins for job %s after scan in zone %s", jobID, zoneCode)

	// Get all device zones for this job with their current counts
	deviceZones, err := s.getJobDeviceZonesWithCounts(jobID)
	if err != nil {
		return fmt.Errorf("failed to get job device zones: %w", err)
	}

	s.mu.RLock()
	mapping := s.mapping
	s.mu.RUnlock()

	settings := s.getJobHighlightSettings()

	if mapping == nil {
		return fmt.Errorf("no mapping loaded")
	}

	if settings.Mode == "required_only" {
		if err := s.publisher.PublishClear(); err != nil {
			log.Printf("[LED] Failed to clear LEDs before required-only update: %v", err)
		}
	}

	// Build bins list with updated colors
	shelvesMap := make(map[string][]Bin)

	// Go through all bins in mapping
	for _, shelf := range mapping.Shelves {
		for _, bin := range shelf.Bins {
			// Check if this bin has devices for the job
			count, hasDevices := deviceZones[bin.BinID]

			hasRequired := hasDevices && count > 0

			if !hasRequired && settings.Mode == "required_only" {
				// Skip bins without pending devices when operating in required-only mode
				continue
			}

			appearance := settings.NonRequired
			if hasRequired {
				appearance = settings.Required
				log.Printf("[LED] Bin %s: REQUIRED (%d devices remaining)", bin.BinID, count)
			} else {
				log.Printf("[LED] Bin %s: NON-REQUIRED (complete or not needed)", bin.BinID)
			}

			ledBin := Bin{
				BinID:     bin.BinID,
				Pixels:    bin.Pixels,
				Color:     appearance.Color,
				Pattern:   appearance.Pattern,
				Intensity: int(appearance.Intensity),
			}
			if appearance.Speed > 0 {
				ledBin.Speed = appearance.Speed
			}

			shelvesMap[shelf.ShelfID] = append(shelvesMap[shelf.ShelfID], ledBin)
		}
	}

	// Convert map to shelves array
	shelves := []Shelf{}
	for shelfID, bins := range shelvesMap {
		shelves = append(shelves, Shelf{
			ShelfID: shelfID,
			Bins:    bins,
		})
	}

	// Send complete update for all bins
	if len(shelves) == 0 {
		log.Printf("[LED] No bins to update for job %s in mode %s", jobID, settings.Mode)
		return nil
	}

	cmd := LEDCommand{
		Op:          "highlight",
		WarehouseID: mapping.WarehouseID,
		Shelves:     shelves,
	}

	return s.publisher.PublishCommand(cmd)
}

// getJobDeviceZonesWithCounts returns a map of zone_code -> device count for a job
func (s *Service) getJobDeviceZonesWithCounts(jobID string) (map[string]int, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT z.code, COUNT(*) as device_count
		FROM job_devices jd
		JOIN devices d ON jd.deviceID = d.deviceID
		JOIN storage_zones z ON d.zone_id = z.zone_id
		WHERE jd.jobID = $1 AND d.status = 'in_storage' AND z.code IS NOT NULL
		GROUP BY z.code
	`

	rows, err := db.Query(query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	zoneCounts := make(map[string]int)
	for rows.Next() {
		var zoneCode string
		var count int
		if err := rows.Scan(&zoneCode, &count); err != nil {
			log.Printf("[LED] Error scanning zone count: %v", err)
			continue
		}
		zoneCounts[zoneCode] = count
		log.Printf("[LED] Zone %s has %d devices for job %s", zoneCode, count, jobID)
	}

	return zoneCounts, nil
}

// getJobDeviceZones retrieves device zone codes for a job from database
// Returns a map of deviceID -> zone code (e.g., "WDL-06-RG-01-F-01")
func (s *Service) getJobDeviceZones(jobID string) (map[string]string, error) {
	db := repository.GetSQLDB()

	// Query job devices with their zone CODE (not name!)
	// The zone code matches the bin_id in the LED mapping configuration
	query := `
		SELECT jd.deviceID, COALESCE(z.code, '') as zone_code
		FROM job_devices jd
		LEFT JOIN devices d ON jd.deviceID = d.deviceID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		WHERE jd.jobID = $1 AND d.status = 'in_storage' AND z.code IS NOT NULL
	`

	rows, err := db.Query(query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deviceZones := make(map[string]string)
	for rows.Next() {
		var deviceID, zoneCode string
		if err := rows.Scan(&deviceID, &zoneCode); err != nil {
			log.Printf("[LED] Error scanning device row: %v", err)
			continue
		}
		deviceZones[deviceID] = zoneCode
		log.Printf("[LED] Device %s is in zone %s", deviceID, zoneCode)
	}

	return deviceZones, nil
}

// PreviewAppearances highlights sample bins using the provided appearances for live preview
func (s *Service) PreviewAppearances(appearances []models.LEDAppearance, clearBefore bool, overrideBinID string) error {
	if len(appearances) == 0 {
		return fmt.Errorf("no appearances provided")
	}

	s.mu.RLock()
	mapping := s.mapping
	s.mu.RUnlock()

	if mapping == nil {
		return fmt.Errorf("no mapping loaded")
	}

	targetBinID := strings.TrimSpace(overrideBinID)
	if targetBinID == "" {
		targetBinID = strings.TrimSpace(os.Getenv("LED_PREVIEW_BIN_ID"))
	}

	if len(mapping.Shelves) == 0 {
		return fmt.Errorf("mapping contains no bins to preview")
	}

	primary := appearances[0]
	caches := newControllerCaches()

	if targetBinID != "" {
		for _, shelf := range mapping.Shelves {
			for _, bin := range shelf.Bins {
				if strings.EqualFold(bin.BinID, targetBinID) {
					intensity := clampIntensity(int(primary.Intensity))
					ledBin := Bin{
						BinID:     bin.BinID,
						Pixels:    bin.Pixels,
						Color:     primary.Color,
						Pattern:   primary.Pattern,
						Intensity: intensity,
					}
					if primary.Speed > 0 {
						ledBin.Speed = primary.Speed
					}

					cmd := LEDCommand{
						Op:          "highlight",
						WarehouseID: mapping.WarehouseID,
						Shelves: []Shelf{
							{
								ShelfID: shelf.ShelfID,
								Bins:    []Bin{ledBin},
							},
						},
					}

					if clearBefore {
						if err := s.publishCommandForBin(bin.BinID, LEDCommand{Op: "clear", WarehouseID: mapping.WarehouseID}, caches); err != nil {
							log.Printf("[LED] Failed to clear LEDs before preview: %v", err)
						}
					}

					if err := s.publishCommandForBin(bin.BinID, cmd, caches); err != nil {
						return err
					}

					log.Printf("[LED] Preview command sent for bin %s (shelf %s, %d pixels)", bin.BinID, shelf.ShelfID, len(bin.Pixels))
					return nil
				}
			}
		}

		return fmt.Errorf("preview bin '%s' not found in mapping", targetBinID)
	}

	appearanceCount := len(appearances)
	var secondary *models.LEDAppearance
	if len(appearances) > 1 {
		secondary = &appearances[1]
	}

	index := 0
	groups := make(map[string]*highlightGroup)

	for _, shelf := range mapping.Shelves {
		for _, binCfg := range shelf.Bins {
			appearance := appearances[index%appearanceCount]
			if index == 0 {
				appearance = primary
			} else if secondary != nil {
				appearance = *secondary
			}
			index++

			intensity := clampIntensity(int(appearance.Intensity))
			ledBin := Bin{
				BinID:     binCfg.BinID,
				Pixels:    binCfg.Pixels,
				Color:     appearance.Color,
				Pattern:   appearance.Pattern,
				Intensity: intensity,
			}
			if appearance.Speed > 0 {
				ledBin.Speed = appearance.Speed
			}

			controller, err := s.resolveControllerForZone(binCfg.BinID, caches)
			if err != nil {
				return err
			}
			key := defaultControllerKey
			if controller != nil {
				key = controller.ControllerID
			}
			group := groups[key]
			if group == nil {
				group = newHighlightGroup(controller)
				groups[key] = group
			}
			group.addBin(shelf.ShelfID, ledBin)
		}
	}

	if len(groups) == 0 {
		return fmt.Errorf("no bins available for preview")
	}

	if clearBefore {
		for _, group := range groups {
			if err := s.publishClearForGroup(group, mapping.WarehouseID); err != nil {
				log.Printf("[LED] Failed to clear LEDs before preview: %v", err)
			}
		}
	}

	for _, group := range groups {
		cmd := group.toCommand(mapping.WarehouseID)
		var err error
		if group.controller != nil {
			err = s.publisher.PublishCommandToController(group.controller, cmd)
		} else {
			err = s.publisher.PublishCommand(cmd)
		}
		if err != nil {
			return err
		}
		log.Printf("[LED] Preview command sent for %d shelves (%d bins total)", len(cmd.Shelves), group.binCount())
	}

	return nil
}

// countTotalBins counts total bins in current mapping (must be called with lock held)
func (s *Service) countTotalBins() int {
	if s.mapping == nil {
		return 0
	}
	count := 0
	for _, shelf := range s.mapping.Shelves {
		count += len(shelf.Bins)
	}
	return count
}

func (s *Service) publishCommandForBin(binID string, cmd LEDCommand, caches *controllerCaches) error {
	if caches == nil {
		caches = newControllerCaches()
	}
	controller, err := s.resolveControllerForZone(binID, caches)
	if err != nil {
		return err
	}
	if controller != nil {
		return s.publisher.PublishCommandToController(controller, cmd)
	}
	return s.publisher.PublishCommand(cmd)
}

func (s *Service) publishClearForGroup(group *highlightGroup, warehouseID string) error {
	clearCmd := LEDCommand{Op: "clear", WarehouseID: warehouseID}
	if group.controller != nil {
		return s.publisher.PublishCommandToController(group.controller, clearCmd)
	}
	return s.publisher.PublishCommand(clearCmd)
}

func (s *Service) resolveControllerForZone(zoneCode string, caches *controllerCaches) (*models.LEDController, error) {
	if zoneCode == "" {
		return nil, nil
	}
	zoneTypeID, err := s.lookupZoneTypeID(zoneCode, caches)
	if err != nil {
		return nil, err
	}
	if zoneTypeID <= 0 {
		return nil, nil
	}
	if controller, ok := caches.controllers[zoneTypeID]; ok {
		return controller, nil
	}
	db := repository.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialised")
	}

	var controller models.LEDController
	err = db.Preload("ZoneTypes").
		Joins("JOIN led_controller_zone_types lcz ON lcz.controller_id = led_controllers.id").
		Where("lcz.zone_type_id = ?", zoneTypeID).
		Order("led_controllers.id ASC").
		First(&controller).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			caches.controllers[zoneTypeID] = nil
			return nil, nil
		}
		return nil, err
	}

	caches.controllers[zoneTypeID] = &controller
	return &controller, nil
}

func (s *Service) lookupZoneTypeID(zoneCode string, caches *controllerCaches) (int, error) {
	if caches != nil {
		if cached, ok := caches.zoneTypeIDs[zoneCode]; ok {
			return cached, nil
		}
	}

	db := repository.GetDB()
	if db == nil {
		return 0, fmt.Errorf("database not initialised")
	}

	var result struct {
		ZoneTypeID sql.NullInt64
	}
	if err := db.Table("storage_zones").
		Select("zone_types.id AS zone_type_id").
		Joins("LEFT JOIN zone_types ON zone_types.key = storage_zones.type").
		Where("storage_zones.code = ?", zoneCode).
		Scan(&result).Error; err != nil {
		return 0, err
	}

	id := 0
	if result.ZoneTypeID.Valid {
		id = int(result.ZoneTypeID.Int64)
	}
	if caches != nil {
		caches.zoneTypeIDs[zoneCode] = id
	}
	return id, nil
}

func clampIntensity(value int) int {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return value
}

// Close cleanup resources
func (s *Service) Close() {
	if s.publisher != nil {
		s.publisher.Close()
	}
}
