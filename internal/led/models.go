package led

// LEDCommand represents a command to be sent to ESP32 LED controllers via MQTT
type LEDCommand struct {
	Op          string  `json:"op"`           // "highlight", "clear", "identify", "config"
	WarehouseID string  `json:"warehouse_id"` // e.g., "weidelbach"
	Shelves     []Shelf `json:"shelves,omitempty"`
	LedCount    *int    `json:"led_count,omitempty"` // For "config" op - number of LEDs
	DataPin     *int    `json:"data_pin,omitempty"`  // For "config" op - GPIO pin number
	Chipset     *string `json:"chipset,omitempty"`   // For "config" op - LED chipset type
}

// Shelf represents a shelf with bins to highlight
type Shelf struct {
	ShelfID string `json:"shelf_id"` // e.g., "A", "B"
	Bins    []Bin  `json:"bins"`
}

// Bin represents a storage bin with LED pixel mappings
type Bin struct {
	BinID     string `json:"bin_id"`          // e.g., "A-01"
	Pixels    []int  `json:"pixels"`          // LED indices
	Color     string `json:"color"`           // Hex color, e.g., "#FF0000"
	Pattern   string `json:"pattern"`         // "solid", "blink", "breathe"
	Intensity int    `json:"intensity"`       // 0-255
	Speed     int    `json:"speed,omitempty"` // Animation speed in milliseconds
}

// LEDMapping represents the configuration file that maps bins to LED indices
type LEDMapping struct {
	WarehouseID string         `json:"warehouse_id"`
	Shelves     []ShelfConfig  `json:"shelves"`
	LEDStrip    LEDStripConfig `json:"led_strip"`
	Defaults    DefaultConfig  `json:"defaults"`
}

// ShelfConfig represents shelf configuration in mapping file
type ShelfConfig struct {
	ShelfID string      `json:"shelf_id"`
	Bins    []BinConfig `json:"bins"`
}

// BinConfig represents bin configuration in mapping file
type BinConfig struct {
	BinID  string `json:"bin_id"`
	Pixels []int  `json:"pixels"`
}

// LEDStripConfig represents LED strip hardware configuration
type LEDStripConfig struct {
	Length  int    `json:"length"`
	DataPin int    `json:"data_pin"`
	Chipset string `json:"chipset"`
}

// DefaultConfig represents default LED appearance settings
type DefaultConfig struct {
	Color     string `json:"color"`
	Pattern   string `json:"pattern"`
	Intensity int    `json:"intensity"`
	Speed     int    `json:"speed,omitempty"`
}
