# WarehouseCore

**Physical Warehouse Management System for RentalCore Deployments**

Version: 5.9.21

WarehouseCore is the digital twin of the Weidelbach warehouse, providing real-time tracking of devices, cases, zones, and movements with barcode/QR scan-driven workflows.

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [LED Highlighting System](#led-highlighting-system)
- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Getting Started](#getting-started)
- [API Documentation](#api-documentation)
- [Product Pictures](#product-pictures)
- [Database Schema](#database-schema)
- [Deployment](#deployment)
- [Development](#development)
- [Docker Hub](#docker-hub)
- [Website Feed](#website-feed)

---

## Overview

WarehouseCore manages the physical warehouse operations alongside RentalCore (the job management system). It provides:

- **Digital warehouse mapping** - Zones, shelves, racks, vehicles, cases
- **Real-time device status** - in_storage | on_job | defective | repair
- **Scan-driven workflows** - Barcode/QR intake, outtake, transfer
- **Full audit trail** - Every movement and scan logged
- **Maintenance tracking** - Defects, repairs, inspection schedules
- **Job synchronization** - Live sync with RentalCore jobs

---

## Features

### Core Modules

1. **Device Tracker**
   - Scan devices in/out of warehouse
   - Track current location (zone, case, job)
   - Status management with history
   - Movement audit trail
   - Produktzentrierte Geräteübersicht mit Schnellaktionen (Fach aufleuchten, Zone öffnen)
   - **Admin CRUD UI** - Full device management with create, edit, delete, QR/barcode generation

2. **Cable Management**
   - Full CRUD interface for cable inventory
   - Search and filter by connectors, cable type, length, and mm²
   - Support for connector types with gender specification (male/female)
   - Cable types (power, signal, data, etc.)
   - Length and cross-section (mm²) tracking
   - Table and grid view modes
   - **Admin CRUD UI** - Complete cable management with create, edit, delete, and detailed view

3. **Case Management**
   - Full CRUD interface for case inventory
   - Search and filter by status and zone
   - Table and grid view modes with device counts
   - Support for dimensions (width, height, depth) and weight tracking
   - Case status management (free, rented, maintenance)
   - Device assignment and tracking within cases
   - Label printing integration
   - Case detail modal with device list view
   - **Admin CRUD UI** - Complete case management in admin dashboard

4. **Product Packages**
   - Create reusable product packages for common job configurations
   - Add multiple products with specific quantities to packages
   - Set package pricing for simplified job calculations
   - Search and filter packages by name or description
   - View detailed package contents with product information
   - Full CRUD interface integrated in Products page
   - Maintain OCR keyword mappings per package (alias map via `GET /api/v1/product-packages/alias-map`)
   - Designed for OCR integration in RentalCore job creation
   - **Admin CRUD UI** - Complete package management with product selection

5. **Rental Equipment (Mietprodukte)** NEW
   - Manage products rented from external suppliers
   - Track rental price (cost to you) vs. customer price (what customer pays)
   - Automatic profit margin calculation
   - Supplier management with auto-complete suggestions
   - Filter by supplier name and search across all fields
   - Active/inactive status for availability control
   - Category and description support
   - Internal notes for supplier contacts, delivery times, etc.
   - Table and card view modes
   - Full CRUD interface in "Mietprodukte" tab under Products page
   - Public API endpoints for RentalCore integration
   - **Admin CRUD UI** - Complete rental equipment management

6. **Storage Zones**
   - Hierarchical zone structure
   - Shelf, rack, case, vehicle, stage types
   - Capacity tracking
   - Active/inactive management

7. **Scan System**
   - Barcode and QR code support
   - Intake/outtake/transfer/check actions
  - Duplicate scan detection
  - Complete event logging with IP/user-agent

8. **CSV Export System** 📊 NEW
   - Comprehensive data export functionality in Admin Dashboard
   - 10 different export types for all major data entities
   - Excel-compatible format (UTF-8 with BOM, semicolon-separated)
   - German date format (DD.MM.YYYY) and decimal separator (comma)
   - One-click download with automatic filename generation
   - Available exports:
     - Products (basic info, with device count, with brand/manufacturer)
     - All devices with full details
     - Manufacturers (standalone and with brands)
     - Brands
     - Storage zones with device counts
     - Cables with specifications
     - Jobs with complete information
   - **Admin UI** - Accessible via CSV-Export tab in Admin Dashboard

## Admin-Dashboard & Rollen

- Admin: verwaltet Zonentypen, LED-Defaults, Rollen; CSV-Export; sieht Profilseite.
- Manager: darf Zonentypen lesen/listen; CSV-Export; keine Änderungen/Löschungen.
- Worker/Viewer: kein Zugriff auf Admin-Routen.

API Endpoints (unter `\`/api/v1\``):
- `GET /admin/zone-types` (admin|manager), `POST/PUT/DELETE /admin/zone-types/:id` (admin)
- `GET /admin/led/single-bin-default` (admin|manager), `PUT /admin/led/single-bin-default` (admin)
- `GET /admin/roles` (admin|manager), `GET /admin/users` (admin|manager)
- `GET /admin/users/:id/roles` (admin|manager), `PUT /admin/users/:id/roles` (admin)
- `GET /profile/me`, `PUT /profile/me`

RBAC Matrix (vereinfacht):
- admin: Vollzugriff
- manager: Lesen (ZoneTypes, Rollenliste/Benutzerliste)
- worker/viewer: kein Adminzugriff

Auto-Admin Seed:
- ENV `ADMIN_NAME_MATCH` (Default: `N. Thielmann`)
- Beim Start werden Benutzer, deren Name/Username/Email diesen String enthält, automatisch mit der Rolle `admin` versehen.

Cross-Links Navbar:
- Domains werden via Backend in `window.__APP_CONFIG__` injiziert.
- ENV: `RENTALCORE_DOMAIN`, `WAREHOUSECORE_DOMAIN` (ohne Protokoll/Port).

### Product Pictures

- Mehrfach-Uploads direkt im Produkt-Detailmodal
- Speicherung im gemeinsamen Nextcloud-Dateipool von RentalCore (`NEXTCLOUD_WEBDAV_*` in `.env` setzen)
- Pfadstruktur wird automatisch erstellt: `<BASE_PATH>/warehousecore/pictures/<Productname>/<Datei>`
- Unterstützte Typen: JPG, PNG, GIF, WEBP, HEIC bis 10 MB je Datei
- Optionaler JPEG-Thumbnail-Cache (`PICTURE_CACHE_DIR`, Default `/tmp/warehousecore/pictures_cache`) beschleunigt Admin-Ansicht; Lightbox nutzt weiter das Original.
- Website-Feed: Produkte lassen sich mit „Auf Website anzeigen“ markieren; Bildauswahl + Thumbnail im Produkt-Detailmodal. Öffentlicher Feed liefert nur freigegebene Produkte/Bilder.

Screens (Beschreibung):
- Admin > Zonentypen: Tabelle mit CRUD für Key/Label/Beschreibung; LED-Defaults werden auf der LED-Seite gepflegt.
- Admin > LED-Verhalten: Globale Defaults, Job-Highlight-Modus (Farben/Pattern/Speed) inkl. Live-Vorschau, zonentypspezifische Einstellungen und JSON-Mapping-Editor mit Validierung
- Admin > Rollen: Benutzerliste, Rollen-Chips, Speichern
- Profil: Avatar-URL, Anzeigename, UI-Prefs (dark-mode default on)
- Sidebar: Profilseite wird ausschließlich über den Username im Benutzerbereich geöffnet, kein separater Menüeintrag.

LED Single-Bin Defaults:
- Setting Key: `app_settings(scope='warehousecore', k='led.single_bin.default')`
- JSON Beispiel: `{ "color": "#FF7A00", "pattern": "breathe", "intensity": 180 }`
- Fallback bei fehlender Einstellung: Orange `#FF7A00`, `breathe`, Intensität `180`.

Migrations
- Siehe Ordner `migrations/`:
  - `007_rbac_system.sql` (+ down) – Zone Types, App Settings, User Profiles, WH-RBAC Seeds
  - `008_assign_auto_admin.sql` – initialer Auto-Admin (Thielmann)
  - `009_update_led_defaults.sql` (+ down) – LED-Default Orange/Breathe/180
  - `022_add_website_fields.sql` – Website-Flags/Thumbnail/Imagelist für Produkte, Website-Flag für Packages

4. **Job Integration**
   - Real-time job assignment
   - Device packing status
   - Missing item detection
   - Job completion workflow
   - Job-Code Scans (`JOB000123`) laden Aufträge direkt in den Pack-Workflow

5. **Maintenance Engine**
   - Defect reporting with severity
   - Repair tracking with costs
   - Inspection scheduling
   - Status workflow (open → in_progress → repaired → closed)

6. **LED Highlighting System** 🎨 NEW
   - Physical warehouse bin highlighting via LED strips
   - Real-time MQTT communication with ESP32 controllers
   - Job-based automatic bin illumination
   - Multiple LEDs per storage bin support
   - Animation patterns (solid, blink, breathe)
   - Remote management and testing
   - Works worldwide with cloud-based MQTT broker

---

## LED Highlighting System

### Overview

The LED Highlighting System provides physical visual guidance in the warehouse by illuminating storage bins containing devices needed for a specific job. When a job is selected in WarehouseCore, the system automatically highlights the corresponding warehouse locations using addressable LED strips controlled by ESP32 microcontrollers.

### Architecture Diagram

**Option A: Self-Hosted (Single Server)**

```
┌──────────────────────────────────────────────────────────────┐
│                    Docker Compose Stack                      │
│  ┌─────────────────┐         ┌──────────────────┐           │
│  │  WarehouseCore    │         │  Mosquitto MQTT  │           │
│  │  Container      │───────→ │  Container       │           │      ┌─────────────────┐
│  │                 │  Pub    │                  │←──────────┼──────│   ESP32 + LEDs  │
│  │  - Job Manager  │         │  Port 1883/8883  │  Sub      │      │  (Warehouse)    │
│  │  - LED Service  │         │  weidelbach/cmd  │           │      │                 │
│  │  - Mapping Cfg  │         │  weidelbach/sts  │           │      │  - WiFi Client  │
│  └─────────────────┘         └──────────────────┘           │      │  - MQTT Sub     │
│       Port 8081                   Auth required             │      │  - SK6812 Driver│
└──────────────────────────────────────────────────────────────┘      └─────────────────┘
         Server (your-server.example.com)                              Warehouse Network

Flow: Job Selected → LED Service publishes to Mosquitto (same host)
      → ESP32 connects to server:1883 → Receives command → Lights up bins
```

**Option B: Cloud-Hosted (Distributed)**

```
┌─────────────────┐         ┌──────────────────┐         ┌─────────────────┐
│  WarehouseCore    │         │  MQTT Broker     │         │   ESP32 + LEDs  │
│  (Cloud/VPS)    │───────→ │  (Cloud/TLS)     │←─────── │  (Warehouse)    │
│                 │  Pub    │                  │  Sub    │                 │
│  - Job Manager  │         │  Topics:         │         │  - WiFi Client  │
│  - LED Service  │         │  weidelbach/cmd  │         │  - MQTT Sub     │
│  - Mapping Cfg  │         │  weidelbach/sts  │         │  - SK6812 Driver│
└─────────────────┘         └──────────────────┘         └─────────────────┘
  Any Server Location        EMQX/HiveMQ/AWS IoT         Behind NAT/Firewall

Flow: Job Selected → Publish to cloud broker → ESP32 subscribes → Show LEDs
```

### Key Features

- **Unlimited ESP Controllers**: Jede ESP32-Firmware erzeugt automatisch eine eindeutige `controller_id` und erhält ein eigenes MQTT-Topic. Zone-Typen lassen sich pro Controller routen.
- **Zero-Touch Discovery**: Sobald ein Controller sein MQTT-Status-Topic abonniert und eine Heartbeat-Nachricht sendet, legt WarehouseCore ihn automatisch in der Datenbank an – ganz ohne zusätzliche Firmware-Konfiguration oder manuelle Stammdatenpflege.
- **Admin ESP-Dashboard**: Neuer Tab „ESP-Controller“ zeigt IP, Hostname, Firmware, RSSI und Uptime an, erlaubt freundliche Namen und Mehrfach-Zonentypzuweisungen über ein komfortables Multi-Select-Dropdown.
- **Telemetry Heartbeats**: MQTT + REST Heartbeat (`/api/v1/led/controllers/{id}/heartbeat`) halten Statusdaten im Backend aktuell – inklusive LED-Anzahl, WiFi-RSSI, Firmwarestand.
- **No Port Forwarding Required**: ESP32 uses outbound MQTT connection, works from any network
- **Cloud-Ready**: WarehouseCore can run on external servers, ESP32 connects via internet
- **Multiple LEDs per Bin**: Support for 2-4 LEDs per storage compartment
- **Flexible Patterns**: Solid, blink, breathe animations
- **Real-Time Control**: Toggle LEDs on/off from job panel
- **Dry-Run Mode**: Backend works without MQTT for testing
- **Admin Mapping Editor**: JSON-based bin-to-LED configuration

### Components

#### 1. Backend (Go)

**Location:** `internal/led/`

- `models.go` - Data structures (LEDCommand, LEDMapping, Bin, Shelf)
- `mqtt_publisher.go` - MQTT client with TLS support, reconnect logic
- `service.go` - Business logic (Job → Bins → Pixels mapping)
- `handlers/led_handlers.go` - REST API endpoints
- `services/led_controller_service.go` - Verwaltung mehrerer LED-Controller inkl. Telemetrie & Zonenzuweisung
- `handlers/led_controller_handlers.go` - Admin-CRUD & Heartbeat-Endpunkte für ESP-Controller

**Endpoints:**
- `GET /api/v1/led/status` - MQTT connection status
- `POST /api/v1/led/highlight?job_id=X` - Highlight bins for job
- `POST /api/v1/led/clear` - Turn off all LEDs
- `POST /api/v1/led/identify` - Test flash all LEDs
- `POST /api/v1/led/test?shelf_id=A&bin_id=A-01` - Test specific bin
- `GET /api/v1/led/mapping` - Get current mapping config
- `PUT /api/v1/led/mapping` - Update mapping config
- `POST /api/v1/led/mapping/validate` - Validate mapping JSON
- `POST /api/v1/led/controllers/{controller_id}/heartbeat` - Telemetrie-Heartbeat (öffentlich, keine Auth)
- `GET /api/v1/admin/led/controllers` - Liste registrierter Controller (Admin/Manager)
- `POST /api/v1/admin/led/controllers` - Controller manuell anlegen (Admin)
- `PUT /api/v1/admin/led/controllers/{id}` - Eigenschaften & Zonentypen pflegen (Admin)
- `DELETE /api/v1/admin/led/controllers/{id}` - Controller löschen (Admin)

#### 2. Frontend (React/TypeScript)

**Location:** `web/src/pages/JobsPage.tsx`, `web/src/components/admin/LEDControllersTab.tsx`, `web/src/lib/api.ts`

- Toggle button in Job Panel: "Fächer hervorheben"
- Visual indicators: MQTT connection status, bin count
- Auto-clear LEDs when exiting job
- Real-time status updates
- Admin > „ESP-Controller“: Übersicht mit Telemetriedaten, Namensvergabe, Topic-Suffix und Zonentyp-Zuordnung

#### 3. ESP32 Firmware (Arduino C++)

**Location:** `firmware/esp32_sk6812_leds/`

- `esp32_sk6812_leds.ino` - Main firmware
- `secrets.h.template` - Config template (WiFi, MQTT credentials)
- **[README.md](firmware/esp32_sk6812_leds/README.md)** - **📘 Complete Setup Guide: Flashing, Hardware Wiring, Configuration**

**Multi-ESP32 Quick Start:**

> 📖 **Für eine detaillierte Anleitung mit Hardware-Anforderungen, Verkabelung, Troubleshooting und mehr, siehe [MULTI_ESP32_GUIDE.md](MULTI_ESP32_GUIDE.md)**

1. **Flash Firmware** (same firmware on all ESP32s):
   - Follow detailed instructions in [firmware/esp32_sk6812_leds/README.md](firmware/esp32_sk6812_leds/README.md)
   - Configure WiFi & MQTT in `secrets.h`
   - Each ESP32 auto-generates unique `controller_id` based on MAC address
   - ✅ You can flash *exactly the same binary* to every controller; WarehouseCore handles discovery and naming

2. **Manage Controllers in Admin Panel**:
   - Navigate to Admin → ESP-Controller
   - View online status, IP, hostname, firmware version, WiFi RSSI, uptime
   - Assign friendly display names (e.g., "Shelf A Controller", "Rack B LEDs")
   - Assign zone types per controller (e.g., "Shelf A" → Controller 1, "Shelf B" → Controller 2)
   - New controllers appear automatically after their first MQTT + heartbeat handshake—no manual database entry required

3. **Zone Routing**:
   - When highlighting bins, WarehouseCore automatically routes commands to the correct ESP32 based on zone type assignment
   - Multiple ESP32s can control different warehouse areas independently


> Heartbeats laufen ausschließlich über MQTT – stelle sicher, dass `TOPIC_PREFIX`, `WAREHOUSE_ID` und die Broker-Zugangsdaten in `secrets.h` zum WarehouseCore-Setup passen. Sobald ein Controller seinen Status auf `<prefix>/<controller>/status` veröffentlicht, wird er automatisch erkannt und im Admin-Panel angezeigt.

#### 4. Controller-Registry & Heartbeat

- **Admin > ESP-Controller**: Kartenansicht mit Online-Status, IP, Hostname, Firmware, WiFi-RSSI, Uptime. Zonentypen lassen sich per Checkbox zuweisen; Anzeigename/Topic können editiert werden.
- Jeder Controller verwaltet:
  - `controller_id` – automatisch generiert (`<PREFIX>-<macsuffix>`) oder manuell überschreibbar
  - `topic_suffix` – Ziel-Topic für Kommandos (`<LED_MQTT_TOPIC_PREFIX>/<suffix>/cmd`)
  - `zone_types` – definieren, für welche Lagerbereiche der Controller leuchtet
- **Heartbeat Workflow**:
  - ESP sendet alle 15 s `POST /api/v1/led/controllers/{controller_id}/heartbeat` (ohne Auth) mit JSON-Payload
  - Unbekannte IDs werden automatisch angelegt (Displayname = ID, Topic = ID)
  - Telemetriedaten (RSSI, Uptime, LED-Länge, Firmware, IP, Hostname, MAC) werden persistiert (`status_data`)
- LED-Kommandos werden auf Basis des Zonen-Typs automatisch zum passenden Controller geroutet; fällt keiner zu, nutzt das System das globale Warehouse-Topic (`LED_WAREHOUSE_ID`).
- Locate-/Preview-/Job-Highlights berücksichtigen Controller-Zuordnung, sodass nur relevanten Streifen angesprochen werden.

**Heartbeat Payload Beispiel**

```json
{
  "controller_id": "esp-a1b2c3",
  "topic_suffix": "esp-a1b2c3",
  "ip_address": "192.168.10.25",
  "hostname": "esp-a1b2c3",
  "firmware_version": "1.1.0",
  "mac_address": "24:6F:28:A1:B2:C3",
  "wifi_rssi": -51,
  "uptime_seconds": 4821,
  "led_count": 600
}
```

- HTTP-Heartbeat funktioniert über HTTP oder HTTPS (Empfehlung: TLS aktivieren). Bei Kommunikationsverlust markiert das Backend den Controller automatisch als offline (`last_seen` + `is_active`).
- Zonen-Typen in WarehouseCore (`storage_zones.type`) müssen einem Controller zugewiesen sein, damit Highlights / Locate-Befehle ankommen.

**Features:**
- WiFi auto-reconnect
- MQTT client with TLS (optional)
- JSON command parsing (ArduinoJson)
- SK6812 GRBW LED driver (Adafruit_NeoPixel)
- Watchdog timer for stability
- Heartbeat publishing every 15s
- Pattern engine (solid/blink/breathe)

#### 4. Configuration Files

**LED Mapping:** `internal/led/config/led_mapping.json`

Defines which LED indices belong to which storage bins.

**The `bin_id` MUST be the exact `code` from your `storage_zones` database table:**

```json
{
  "warehouse_id": "WDL",
  "shelves": [
    {
      "shelf_id": "Regal-01",
      "bins": [
        { "bin_id": "WDL-RG-01-F-01", "pixels": [0, 1, 2, 3] },
        { "bin_id": "WDL-RG-01-F-02", "pixels": [4, 5, 6] }
      ]
    }
  ],
  "led_strip": {
    "length": 600,
    "data_pin": 5,
    "chipset": "SK6812_GRBW"
  },
  "defaults": {
    "color": "#FF2A2A",
    "pattern": "breathe",
    "intensity": 180
  }
}
```

**How it works:**
1. Job is selected with devices
2. WarehouseCore looks up each device's `zone_id` in the database
3. Gets the zone's `code` (e.g., "WDL-RG-01-F-01")
4. Matches this code with `bin_id` in the mapping file
5. Sends the corresponding `pixels` to the ESP32 to light up

**JSON Schemas:** `internal/led/schema/led_command.schema.json`, `internal/led/schema/led_mapping.schema.json`

### MQTT Communication

#### Command Topic (Publish from WarehouseCore)
```
{TOPIC_PREFIX}/{WAREHOUSE_ID}/cmd
Example: weidelbach/weidelbach/cmd
```

**Highlight Command:**
```json
{
  "op": "highlight",
  "warehouse_id": "weidelbach",
  "shelves": [
    {
      "shelf_id": "A",
      "bins": [
        {
          "bin_id": "A-01",
          "pixels": [0, 1, 2, 3],
          "color": "#FF0000",
          "pattern": "breathe",
          "intensity": 180
        }
      ]
    }
  ]
}
```

**Clear Command:**
```json
{
  "op": "clear",
  "warehouse_id": "weidelbach"
}
```

#### Status Topic (Publish from ESP32)
```
{TOPIC_PREFIX}/{WAREHOUSE_ID}/status
Example: weidelbach/weidelbach/status
```

**Heartbeat:**
```json
{
  "status": "online",
  "warehouse_id": "weidelbach",
  "active_leds": 12,
  "wifi_rssi": -45,
  "uptime": 3600
}
```

### Setup Instructions

#### 1. MQTT Broker Setup

You have two options for MQTT broker setup:

##### **Option A: Self-Hosted Mosquitto (Recommended for Single-Server Deployment)** ✅

If your WarehouseCore and ESP32 can both connect to the same server (e.g., both on-premises or both can reach your server), use the included Mosquitto container:

**Quick Setup (3 Steps):**

```bash
# 1. Copy the example .env file
cp .env.example .env

# 2. Edit .env with your credentials
nano .env  # Configure DB and MQTT credentials

# 3. Start everything
docker-compose up -d
```

**That's it!** The MQTT password file is **automatically generated** from your `.env` file on container startup.

**Default credentials in `.env.example`:**
- **Username:** `leduser`
- **Password:** `ledpassword123`

**To change MQTT credentials:** Just edit these values in your `.env` file:
```env
LED_MQTT_USER=leduser
LED_MQTT_PASS=your_custom_password
```

Then restart: `docker-compose restart mosquitto`

The password file will be automatically regenerated with the new credentials!

**ESP32 Configuration:**

In your ESP32 `secrets.h`, use the **same credentials** from your `.env` file:

```cpp
#define MQTT_HOST "your-server.example.com"  // Replace with your server's domain or IP
#define MQTT_PORT 1883
#define MQTT_USER "leduser"                   // Same as LED_MQTT_USER in .env
#define MQTT_PASS "ledpassword123"            // Same as LED_MQTT_PASS in .env
```

**⚠️ Production Deployment:**

For production, change the password in your `.env` file:

```env
# In your .env file
LED_MQTT_USER=leduser
LED_MQTT_PASS=your_secure_production_password
```

Then:
1. Restart Mosquitto: `docker-compose restart mosquitto`
2. Update ESP32 `secrets.h` with the same password
3. Re-flash ESP32

**Optional: Enable TLS for production** (port 8883). See `mosquitto/README.md` for:
- Setting up Let's Encrypt certificates
- Configuring TLS in mosquitto.conf
- Updating ESP32 firmware for TLS support

**Advantages:**
- ✅ No external dependencies
- ✅ No subscription fees
- ✅ Full control over configuration
- ✅ Lower latency (local network)
- ✅ Works without internet connection
- ✅ Included in docker-compose

**See `mosquitto/README.md` for detailed setup and troubleshooting.**

##### **Option B: Cloud MQTT Broker (For Distributed Deployments)**

If your WarehouseCore runs in the cloud and ESP32 is behind NAT/firewall, use a cloud broker:

**Option B1: EMQX Cloud** (easiest)
- Sign up at https://www.emqx.com/en/cloud
- Create free tier deployment
- Note hostname, port (8883 for TLS), username, password

**Option B2: HiveMQ Cloud**
- Free tier available at https://www.hivemq.com/

**Option B3: AWS IoT Core, Azure IoT Hub**
- Enterprise options with advanced features

**Configuration:**

```env
# LED MQTT Configuration - Cloud Broker
LED_MQTT_HOST=your-broker.emqxsl.com
LED_MQTT_PORT=8883
LED_MQTT_TLS=true
LED_MQTT_USER=your_cloud_username
LED_MQTT_PASS=your_cloud_password
```

**Advantages:**
- ✅ Works worldwide with WarehouseCore and ESP32 on different networks
- ✅ No port forwarding required
- ✅ Managed service (auto-scaling, backups)
- ✅ Built-in monitoring and dashboards

#### 2. WarehouseCore Backend Configuration

Add to `.env`:

```env
# Option 1: Self-Hosted (docker-compose)
LED_MQTT_HOST=mosquitto
LED_MQTT_PORT=1883
LED_MQTT_TLS=false
LED_MQTT_USER=leduser
LED_MQTT_PASS=your_password

# Option 2: Cloud Broker (EMQX, HiveMQ, etc.)
# LED_MQTT_HOST=your-broker.emqxsl.com
# LED_MQTT_PORT=8883
# LED_MQTT_TLS=true
# LED_MQTT_USER=your_cloud_username
# LED_MQTT_PASS=your_cloud_password

# Common settings
LED_TOPIC_PREFIX=weidelbach
WAREHOUSE_ID=weidelbach
```

**Note:** If `LED_MQTT_HOST` is empty, system runs in DRY-RUN mode (logs commands without sending).

#### 3. LED Mapping Configuration

Edit `internal/led/config/led_mapping.json`:

**IMPORTANT:** The `bin_id` must match the `code` from your `storage_zones` table in the database!

Example:
- Database has zone with `code = "WDL-RG-01-F-01"` (Weidelbach, Regal 01, Fach 01)
- LED Mapping has `"bin_id": "WDL-RG-01-F-01"`
- When a device in that zone is part of a job, the LEDs will light up

1. Set `warehouse_id` to your main warehouse zone code (e.g., `"WDL"`)
2. For each bin, use the exact `code` from `storage_zones` table as `bin_id`
3. Map each bin to LED pixel indices (e.g., `"pixels": [0, 1, 2, 3]`)
4. Support multiple LEDs per bin
5. Set defaults (color, pattern, intensity)
6. Define LED strip parameters (length, pin, chipset)

**Testing Mapping:**
```bash
curl -X POST http://localhost:8081/api/v1/led/mapping/validate \
  -H "Content-Type: application/json" \
  -d @internal/led/config/led_mapping.json
```

#### 4. ESP32 Firmware Flashing

1. Install Arduino IDE + ESP32 support
2. Install libraries: PubSubClient, ArduinoJson, Adafruit_NeoPixel
3. Copy `secrets.h.template` to `secrets.h`
4. Fill in WiFi SSID, password, MQTT credentials
5. Set `LED_PIN` (default 5) and `LED_LENGTH` (e.g., 600)
6. Upload to ESP32
7. Monitor serial output (115200 baud)

See `firmware/esp32_sk6812_leds/README.md` for detailed instructions.

#### 5. Hardware Wiring

```
ESP32 GPIO 5 → Level Shifter (3.3V→5V) → SK6812 DIN
ESP32 GND   → SK6812 GND (common ground!)
5V PSU      → SK6812 5V+ (separate power for LEDs)
```

**Important:**
- Use level shifter for data line
- Common ground between ESP32 and LED strip
- Adequate 5V power supply (calculate: # LEDs × 80mA)
- Capacitor (1000µF) across power supply

### User Workflow

1. **Navigate to Jobs** → Select open job
2. **Click "Fächer hervorheben"** button (Lightbulb icon)
3. **LEDs illuminate** warehouse bins containing job devices
4. **Pick devices** from highlighted bins
5. **Scan each device** to mark as collected
6. **LEDs auto-clear** when navigating away or completing job

### Testing

#### Dry-Run Mode (No MQTT Broker)

Start WarehouseCore without LED_MQTT_HOST configured:

```bash
# .env
LED_MQTT_HOST=  # Empty = dry-run mode

# Start server
./server

# Test commands (will log JSON without sending)
curl -X POST http://localhost:8081/api/v1/led/highlight?job_id=1
curl -X POST http://localhost:8081/api/v1/led/clear
```

#### With Real MQTT Broker

```bash
# Check status
curl http://localhost:8081/api/v1/led/status

# Expected output:
{
  "mqtt_connected": true,
  "mqtt_dry_run": false,
  "mapping_loaded": true,
  "warehouse_id": "weidelbach",
  "total_shelves": 3,
  "total_bins": 12
}

# Highlight job bins
curl -X POST http://localhost:8081/api/v1/led/highlight?job_id=42

# Test specific bin
curl -X POST "http://localhost:8081/api/v1/led/test?shelf_id=A&bin_id=A-01"

# Clear all
curl -X POST http://localhost:8081/api/v1/led/clear
```

### Troubleshooting

**Problem:** LEDs don't light up
- Check ESP32 serial monitor for connection status
- Verify MQTT credentials match between WarehouseCore and ESP32
- Ensure ESP32 is online (check status topic)
- Test with `/led/identify` endpoint

**Problem:** Wrong bins highlighted
- Review `internal/led/config/led_mapping.json`
- Check device zone assignments in database
- Validate mapping with `/led/mapping/validate`

**Problem:** MQTT connection fails
- Check broker hostname, port, credentials
- Verify TLS setting matches broker (port 8883 = TLS)
- Test broker with MQTT client (MQTT Explorer, mqttx)
- Check firewall rules on broker

**Problem:** ESP32 crashes/reboots
- Insufficient power supply (check amperage)
- Reduce LED_LENGTH if memory issues
- Increase watchdog timeout in firmware

### Security Considerations

- **TLS**: Always use TLS in production (`LED_MQTT_TLS=true`, port 8883)
- **Strong Passwords**: Use complex MQTT passwords
- **Namespaced Topics**: Include warehouse_id to prevent conflicts
- **Secrets Management**: Never commit `secrets.h` or `.env` files
- **Certificate Pinning**: Consider for production ESP32 deployments

### Performance

- **LED Update Rate**: 50-100 Hz depending on strip length
- **MQTT Latency**: Typical <100ms with cloud broker
- **Reconnect Time**: 5-second retry interval
- **Heartbeat**: Every 15 seconds
- **Max LED Count**: ~2000 LEDs per ESP32 (RAM limited)

### Future Enhancements

- Admin UI for visual mapping editor
- Support for multiple ESP32 controllers
- Zone-based LED groups
- Custom animation patterns
- Mobile app for testing
- Automatic mapping from CAD drawings

---

## Architecture

```
warehousecore/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── models/                  # Domain models
│   │   ├── device.go
│   │   ├── zone.go
│   │   ├── movement.go
│   │   ├── scan.go
│   │   ├── maintenance.go
│   │   ├── case.go
│   │   ├── job.go
│   │   └── helpers.go
│   ├── repository/              # Database layer
│   │   └── database.go
│   ├── services/                # Business logic
│   │   └── scan_service.go      # Core scan processing
│   ├── handlers/                # HTTP handlers
│   │   └── handlers.go
│   └── middleware/              # HTTP middleware
│       └── middleware.go
├── migrations/                  # Database migrations
│   ├── 001_storage_zones.sql
│   ├── 002_device_movements.sql
│   ├── 003_scan_events.sql
│   └── 004_defect_reports.sql
├── config/
│   └── config.go                # Configuration management
├── web/                         # Frontend (React + TypeScript + Tailwind)
│   ├── src/
│   ├── public/
│   └── package.json
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── README.md
```

---

## Tech Stack

**Backend:**
- Go 1.24+ (GORM compatibility)
- gorilla/mux (routing)
- GORM (ORM for auth models)
- PostgreSQL 17 (shared RentalCore database)
- Session-based authentication
- CORS enabled

**Frontend:**
- React 18+ with TypeScript
- TailwindCSS with the shared RentalCore brand theme
- shadcn/ui components
- Dark mode first-class support

**Infrastructure:**
- Docker + Docker Compose
- Docker Hub: `nobentie/warehousecore`
- GitLab: git.server-nt.de/ntielmann/warehousecore

---

## Getting Started

### Prerequisites

- Go 1.24+
- PostgreSQL 17+ (access to RentalCore database)
- Docker (optional, for containerized deployment)
- Node.js 18+ (for frontend development)

### Local Development

1. **Clone the repository**
```bash
git clone https://git.server-nt.de/ntielmann/warehousecore.git
cd warehousecore
```

2. **Configure environment**
```bash
cp .env.example .env
# Edit .env with your database credentials
```

3. **Run database migrations**
```bash
# Execute SQL files in migrations/ directory against RentalCore database
psql -h db.example.com -U warehouse_user -d rentalcore < migrations/001_storage_zones.sql
psql -h db.example.com -U warehouse_user -d rentalcore < migrations/002_device_movements.sql
psql -h db.example.com -U warehouse_user -d rentalcore < migrations/003_scan_events.sql
psql -h db.example.com -U warehouse_user -d rentalcore < migrations/004_defect_reports.sql
```

4. **Install dependencies**
```bash
go mod download
```

5. **Run the server**
```bash
make run
# Or: go run cmd/server/main.go
```

Server runs on `http://localhost:8081`

### API Health Check
```bash
curl http://localhost:8081/api/v1/health
```

---

## API Documentation

### Base URL
`http://localhost:8081/api/v1`

### Endpoints

#### Health
- `GET /health` - Server health check

#### Authentication
- `POST /auth/login` - Login with username and password
- `POST /auth/logout` - Logout and destroy session
- `GET /auth/me` - Get current authenticated user

**Login Request Body:**
```json
{
  "username": "admin",
  "password": "password"
}
```

**Login Response:**
```json
{
  "success": true,
  "message": "Login successful",
  "user": {
    "UserID": 1,
    "Username": "admin",
    "Email": "admin@example.com",
    "FirstName": "Admin",
    "LastName": "User",
    "IsActive": true
  }
}
```

#### Scans (CRITICAL)
- `POST /scans` - Process barcode/QR scan
- `GET /scans/history` - Get scan event history

**Scan Request Body:**
```json
{
  "scan_code": "DEVICE123",
  "action": "intake|outtake|check|transfer",
  "job_id": 42,
  "zone_id": 5,
  "notes": "Optional notes"
}
```

**Scan Response:**
```json
{
  "success": true,
  "message": "Device successfully returned to warehouse",
  "device": {...},
  "movement": {...},
  "action": "intake",
  "previous_status": "on_job",
  "new_status": "in_storage",
  "duplicate": false
}
```

#### Devices
- `GET /devices` - List devices (filters: status, zone_id, limit)
- `GET /devices/{id}` - Get device details
- `PUT /devices/{id}/status` - Update device status
- `GET /devices/{id}/movements` - Get device movement history

#### Zones
- `GET /zones` - List all zones
- `POST /zones` - Create new zone
- `GET /zones/{id}` - Get zone details
- `PUT /zones/{id}` - Update zone
- `DELETE /zones/{id}` - Soft-delete zone

#### Jobs
- `GET /jobs` - List active jobs
- `GET /jobs/{id}` - Get job summary with device status
- `POST /jobs/{id}/complete` - Complete job

#### Cases
- `GET /cases` - List all cases
- `GET /cases/{id}` - Get case details
- `GET /cases/{id}/contents` - Get case contents

#### Maintenance
- `GET /defects` - List defect reports (filters: status, severity, device_id)
- `POST /defects` - Create defect report
- `PUT /defects/{id}` - Update defect status, costs, notes
- `GET /maintenance/inspections` - Get inspection schedules (filters: upcoming, overdue, all)
- `GET /maintenance/stats` - Get maintenance dashboard statistics

#### Dashboard
- `GET /dashboard/stats` - Get warehouse statistics

#### Cables (Admin)
- `GET /admin/cables` - List all cables (filters: search, connector1, connector2, type, length_min, length_max)
- `GET /admin/cables/{id}` - Get cable details
- `POST /admin/cables` - Create new cable
- `PUT /admin/cables/{id}` - Update cable
- `DELETE /admin/cables/{id}` - Delete cable
- `GET /admin/cable-connectors` - List all connector types
- `GET /admin/cable-types` - List all cable types

**Cable Request Body:**
```json
{
  "name": "Main Power Cable",
  "connector1": 1,
  "connector2": 2,
  "typ": 1,
  "length": 10.5,
  "mm2": 2.5
}
```

**Cable Response:**
```json
{
  "cable_id": 1,
  "name": "Main Power Cable",
  "connector1": 1,
  "connector2": 2,
  "typ": 1,
  "length": 10.5,
  "mm2": 2.5,
  "connector1_name": "CEE 16A",
  "connector2_name": "Schuko",
  "cable_type_name": "Power Cable",
  "connector1_gender": "male",
  "connector2_gender": "female"
}
```
- `GET /movements` - Get recent movements

---

## Database Schema

### New Tables (WarehouseCore-specific)

**storage_zones** - Logical warehouse areas
- zone_id, code, name, type, description, parent_zone_id, capacity, is_active

**device_movements** - Audit trail of all device movements
- movement_id, device_id, action, from_zone_id, to_zone_id, from_job_id, to_job_id, barcode, user_id, notes, metadata, timestamp

**scan_events** - Complete scan log
- scan_id, scan_code, scan_type, device_id, action, job_id, zone_id, user_id, success, error_message, metadata, ip_address, user_agent, timestamp

**defect_reports** - Detailed defect tracking
- defect_id, device_id, severity, status, title, description, reported_by, reported_at, assigned_to, repaired_by, repaired_at, repair_cost, repair_notes, closed_at, images, metadata

**inspection_schedules** - Periodic inspection requirements
- schedule_id, device_id, product_id, inspection_type, interval_days, last_inspection, next_inspection, is_active

### Existing Tables (from RentalCore)
- devices, cases, devicescases, jobs, jobdevices, products, maintenanceLogs, customers

---

## Deployment

### Docker (Recommended)

**Important: Multi-Stage Build Process**

The Dockerfile uses a multi-stage build that:
1. **Stage 1**: Builds the frontend React app using Node.js 20
2. **Stage 2**: Builds the Go backend
3. **Stage 3**: Combines both into a minimal Alpine image

This ensures that every Docker build includes the latest frontend changes.

**Build Docker image:**
```bash
# Check latest version on Docker Hub first
docker images nobentie/warehousecore

# Build with incremented version (example: 3.35)
docker build -t nobentie/warehousecore:3.35 .

# Tag as latest
docker tag nobentie/warehousecore:1.56 nobentie/warehousecore:latest
```

**Migrations**
- **030_device_id_trigger.sql** - PostgreSQL trigger function for automatic device ID generation
- **022_add_website_fields.sql** - Website-Flags/Thumbnail/Bildauswahl für Produkte & Packages

**Push to Docker Hub:**
```bash
# Push version tag
docker push nobentie/warehousecore:3.35

# Push latest tag
docker push nobentie/warehousecore:latest
```

**Deploy on Komodo (docker03 server):**
```bash
# The Docker stack runs on docker03 server via Komodo
# User: noah | Host: docker03
# DO NOT manually restart the stack - only authorized via Komodo

# To deploy new version:
# 1. Build and push to Docker Hub (see above)
# 2. Pull latest image on docker03:
ssh noah@docker03
docker pull nobentie/warehousecore:latest

# 3. Restart stack via Komodo web interface
# (DO NOT use docker-compose restart manually)
```

**Current Version:** 3.38

**Recent Changes:**
- 3.38: ProductDependenciesModal background changed from semi-transparent glass to solid dark overlay for improved readability
- 3.37: Product list shows device count badge; product detail modal gains devices tab with lazy loading; LED locate deduplication fix
- 3.36: PostgreSQL trigger for automatic device ID generation - fixes null deviceID constraint violations
- 3.35: Produktbilder laden deutlich schneller dank gecachter JPEG-Thumbnails + Admin-UI nutzt die kleinen Varianten
- 2.71: Gerätebaum als zusätzliche Ansicht direkt in „Produkte“ (View-Toggle)
- 2.70: Sidebar navigation order aligned with RentalCore (Dashboard, Scan, Produktmanagement, Cases, Lagerbereiche, Aufträge, Admin)
- 2.69: Full device management in product edit modal - add/remove devices, view device list with status
- 2.68: UI Consistency - Product modal placement standardization with ModalPortal
- 2.62: Product packages gain package codes, OCR alias management UI, and alias-map API
- 2.61: Haupt-Kabeltabelle gruppiert nach Typ/Stecker/Länge mit Inline-Details
- 2.60: Kabelübersicht gruppiert nach Stecker1/Stecker2/Länge und zeigt Gesamtanzahl pro Kombination

**Run with docker-compose (local development):**
```bash
# Pull latest image from Docker Hub
docker-compose pull

# Start the service
docker-compose up -d
```

> **Note:** docker-compose.yml is configured to use the `nobentie/warehousecore:latest` image from Docker Hub.

**Check logs:**
```bash
docker-compose logs -f warehousecore
```

### 🔄 Integrated Deployment with RentalCore

For integrated deployment of both WarehouseCore and RentalCore together, use the root docker-compose configuration:

```bash
# Navigate to the parent directory (NOT a git repo)
cd /opt/dev/lager_weidelbach

# Pull latest images from Docker Hub
docker compose pull

# Start both services
docker compose up -d

# Check status
docker compose ps

# View logs
docker compose logs -f warehousecore
docker compose logs -f rentalcore
```

**Access the applications:**
- **WarehouseCore**: http://localhost:8082
- **RentalCore**: http://localhost:8081

**Cross-navigation:**
Both applications feature sidebar/navbar links to seamlessly switch between WarehouseCore and RentalCore with a single click.

**Note:** The images use `:latest` tags. Pull periodically to get the newest versions:
```bash
docker compose pull && docker compose up -d
```

### Production Deployment

1. Ensure database migrations are applied
2. Build and push Docker image with version tag (1.X)
3. Update production docker-compose or k8s manifests
4. Pull and restart container
5. Verify health endpoint

---

## Development

### Running Migrations
Apply new migrations to the shared RentalCore database:
```bash
psql -h db.example.com -U warehouse_user -d rentalcore < migrations/XXX_new_feature.sql
```

### Development Workflow
1. Create feature branch
2. Implement changes
3. Test locally
4. Update README
5. Commit (no AI mentions)
6. Push to GitLab
7. Build Docker image with version tag
8. Push to Docker Hub (version + latest)

### Code Quality Rules
- Never claim finished if not 100% working
- Remove temporary/debug files immediately
- No sensitive data in repository
- Update README after every change
- Professional commit messages (no AI references)
- Clean file management (no _final, _new, _fixed suffixes)

---

## Docker Hub

**Repository:** `nobentie/warehousecore`

- **Tags:**
- `latest` - Latest stable build
- `3.28` - Website-Feed: sichtbare Produkte/Packages + Bildauswahl/Thumbnail, öffentliche Feeds, Admin-UI für Website-Flags
- `3.07` - Modal scroll behavior fix: prevent background scrolling when modals open
- `3.06` - Product dependencies system
- `1.62` - LED-Befehle aufcontroller-spezifische Topics geroutet (Zonentyp-Zuordnung)
- `1.61` - Admin-Tab „Mikrocontroller“ + Heartbeat-Registry
- `1.60` - Bin-Vorschau zielt exakt auf das gewählte Fach
- `1.59` - Einzelvorschau beleuchtet nur das gewählte Fach (kein Clear)
- `1.58` - Preview sendet keine Clear-Kommandos mehr
- `1.57` - Vorschau lässt LEDs an; Clear-Befehl entfällt
- `1.55` - Vorschau mit Fachcode hält übrige Bins aktiv (Job-Highlight-Verhalten)
- `1.54` - Vorschau nutzt Job-Highlight-Logik (optional mit gezieltem Fachcode + Fehlerfeedback)
- `1.53` - Admin-UI bietet Fachauswahl für LED-Vorschau (Datalist + Eingabefeld)
- `1.52` - LED Vorschau nutzt optionalen `LED_PREVIEW_BIN_ID` und Gerätekarte bietet Detail-Button
- `1.51` - Geräte-Liste öffnet wieder Detailmodal per Klick
- `1.50` - Admin LED preview always lights the first bin
- `1.49` - Device detail modal inside legacy /devices page (now merged into Products tab)
- `1.48` - Device detail modal with orange breathe LED locate
- `1.47` - Auto-clear LEDs on page exit/browser close
- `1.46` - Reset pack status on device intake
- `1.45` - Complete LED refresh after device scan
- `1.44` - Live LED updates on device outtake
- `1.43` - Device tree string-based ID fix
- `1.42` - Device tree database schema fix
- `1.41` - Device tree selector in zone details
- `1.40` - Cross-navigation hostname fix

---

## Website Feed

- Öffentliche Endpoints:
  - `GET /api/v1/public/products` – alle Produkte mit `website_visible = true` inkl. ausgewählter Bilder/Thumbnail
  - `GET /api/v1/public/packages` – alle Packages mit `website_visible = true` inkl. Items und Package-Bildern (vom verknüpften Produkt)
  - `GET /api/v1/public/products/{id}/pictures/{filename}` – Bild-Download für Website
  - Schutz via API-Key (`X-API-Key` oder `?api_key=`)
- Admin:
  - Im Produkt-Detailmodal: Toggle „Auf Website anzeigen“, Bilder auswählen, Thumbnail setzen
  - Bei Packages: Flag `website_visible` setzen (Bilder/Thumb aus dem verknüpften Produkt)
  - API-Keys verwalten: `/api/v1/admin/api-keys` (listen), `POST /api/v1/admin/api-keys` (erstellen, Key wird einmalig im Response geliefert), `PUT /api/v1/admin/api-keys/{id}/status` (aktiv/deaktiv), `DELETE /api/v1/admin/api-keys/{id}`

---
- `1.35` - Frontend with LED control button
- `1.34` - Red/green bin highlighting + MQTT topic fix
- `1.33` - Solid green LED pattern for job bins
- `1.32` - Fixed LED zone codes and MQTT configuration
- `1.31` - Copy LED config files to Docker image and initialize service
- `1.30` - Docker image includes LED configuration directories
- `1.29` - LED mapping uses zone codes from database
- `1.28` - MQTT credentials via .env only, auto-generated password file
- `1.27` - Simplified zero-config MQTT setup with demo credentials
- `1.26` - Self-hosted MQTT broker with Docker Compose
- `1.25` - LED warehouse bin highlighting system
- `1.18` - User authentication and SSO with RentalCore
- `1.17` - Fixed mobile scrolling issues and button overlaps
- `1.16` - Complete mobile responsiveness for all pages
- `1.15` - Complete maintenance module with defect tracking and inspections
- `1.14` - Job-based outtake workflow with live scan tracking
- `1.13` - Recursive device count for parent zones
- `1.12` - Two-step intake workflow with zone barcode scanning
- `1.11` - Fixed SPA routing for page reloads
- `1.10` - Fixed zone devices SQL query + subzone delete buttons
- `1.9` - Fixed race condition in bulk shelf creation
- `1.8` - Automatic shelf creation with barcode generation
- `1.7` - Simplified zone types + delete functionality
- `1.6` - Hierarchical zones with auto code generation
- `1.5` - Clean JSON API responses
- `1.4` - Zone creation fix
- `1.3` - Multi-platform API URL support
- `1.2` - Frontend enhancements
- `1.1` - TailwindCSS fixes
- `1.0` - Initial release

**Pull image:**
```bash
docker pull nobentie/warehousecore:latest
```

---

## Environment Variables

```env
# Server
PORT=8081
HOST=0.0.0.0

# Database (Shared with RentalCore)
DB_HOST=db.example.com
DB_USER=tsweb
DB_PASS=<password>
DB_NAME=RentalCore
DB_PORT=5432
DB_SSLMODE=disable

# Application
APP_ENV=development|production
LOG_LEVEL=info|debug

# CORS
CORS_ORIGIN=*

# LED MQTT Configuration
LED_MQTT_HOST=your-broker.emqxsl.com  # MQTT broker hostname (empty = dry-run mode)
LED_MQTT_PORT=8883                     # Port (1883 for non-TLS, 8883 for TLS)
LED_MQTT_TLS=true                      # Enable TLS (true|false)
LED_MQTT_USER=your_username            # MQTT username
LED_MQTT_PASS=your_password            # MQTT password
LED_TOPIC_PREFIX=weidelbach            # MQTT topic prefix
WAREHOUSE_ID=weidelbach                # Warehouse identifier
```

---

## License

Proprietary - WarehouseCore Project

---

## Support

For issues or questions:
- GitLab Issues: https://git.server-nt.de/ntielmann/warehousecore/issues
- Internal documentation: See /lager_weidelbach/claude.md

---

**Version:** 2.62
**Last Updated:** 2025-11-01
**Maintainer:** WarehouseCore Development Team

---

## Changelog

### Version 5.9.21 (2026-05-14)
- **fix(labels): behebe leere/weiße Labels bei der Generierung**
  - Root Cause: `canvas.toDataURL()` wurde nach festem 700ms-Timeout aufgerufen, während `renderPreview` erst nach 600ms Debounce startete und danach async API-Calls (QR-Code, Barcode) machte — Canvas war beim Erfassen noch leer
  - `renderDeviceToCanvas(dev)` extrahiert, liest Refs statt React-State-Closures, ist direkt awaitable
  - `generateLabels` ruft `await renderDeviceToCanvas(dev)` direkt auf statt auf willkürliche Timeouts zu vertrauen
  - Template-Wechsel wartet jetzt auf React-Re-Render via `requestAnimationFrame` (doppelt) statt 900ms-Timeout

### Version 5.9.20 (2026-05-14)
- **fix(labels): behebe leere Labels beim Bulk-Druck**
  - `img.onload`/`onerror`-Handler wird jetzt vor `img.src` gesetzt, um gecachte Bilder sicher zu erfassen
  - `win.print()` wird nach 300 ms Verzögerung aufgerufen, damit der Browser alle Bilder vollständig malen kann bevor der Druckdialog öffnet

### Version 5.9.19 (2026-05-12)
- **fix(backend): correct PostgreSQL column casing in product_dependencies**
  - Replaced all `p.productID` references in raw SQL with `p.productid` (PostgreSQL lowercases unquoted identifiers)
  - Fixed GORM struct tag and `Where` clause from `productID` to `productid`
  - Rewrote `migrations/021_create_product_dependencies.sql` from MySQL syntax (`AUTO_INCREMENT`, `ENGINE=InnoDB`) to valid PostgreSQL DDL

### Version 2.71 (2025-11-21)
- **Gerätebaum als Ansicht auf der Produktseite**
  - Device Tree ist nun ein weiterer View-Toggle direkt unter „Produkte“ (neben Tabelle/Karten), kein eigener Tab mehr
  - Ermöglicht schnellen Wechsel zwischen Tabellen-, Karten- und Gerätebaum-Ansicht ohne Kontextwechsel

### Version 2.70 (2025-11-21)
- **Sidebar order aligned with RentalCore**
  - Navigation reordered to: Dashboard, Scan, Produktmanagement (with product submenu), Cases, Lagerbereiche, Aufträge, Admin
  - Keeps admin visibility role-gated while mirroring RentalCore theming and grouping

### Version 2.69 (2025-11-15)
- **Feature: Full Device Management in Product Edit Modal**
  - Added GET `/admin/products/{id}/devices` endpoint to fetch all devices for a product
  - Enhanced product edit modal with complete device management capabilities
  - Users can now view all assigned devices with their current status
  - Added ability to add multiple devices directly from edit modal
  - Device removal with visual toggle (mark for deletion, confirm on save)
  - Real-time device count and loading states
  - Consistent UI between create and edit modes
  - Device list shows device ID, status, and removal options
  - Batch device deletion with confirmation message
  - Device creation fields (quantity/prefix) available in edit mode with "Add Devices" button

### Version 2.68 (2025-11-15)
- **UI Consistency: Product Modal Placement Standardization**
  - Standardized all product modals (create/edit/view) to match product package modal placement
  - Wrapped all product modals with ModalPortal component for proper portal rendering
  - Applied consistent positioning: `fixed inset-0 z-[120]` with `min-h-screen` centering
  - Unified backdrop styling to `bg-black/80` (matching package modals)
  - Simplified modal structure with direct `overflow-y-auto` on content div
  - Restructured view modal content to match package modal layout pattern with sections
  - Updated button layouts: Cancel/Submit order matches package modals
  - Changed heading tags from `h2` to `h3` for consistency
  - Applied unified close button styling (`w-6 h-6` icons, consistent hover states)
  - All modals now have identical visual appearance and behavior across Products and Packages

### Version 1.62 (2025-11-15)
- **Responsive View Mode Defaults** - UX Enhancement
  - Products Page: Default view mode now responds to screen size
    - Mobile devices (<768px): Card view as default for better touch interaction
    - Desktop devices (≥768px): Table view as default for data density
  - Cables Page: Same responsive default behavior applied
  - View modes automatically adjust on window resize
  - Users can still manually toggle between table and card views
  - Improves initial user experience based on device type

### Version 1.61 (2025-11-15)
- **Mobile Responsiveness Improvements** - Fixed GitLab Issue #20
  - Products Page: Made tab bar scrollable with optimized button sizing; search field full-width; responsive action buttons
  - Cable Page: Fixed search icon overlay issue; action buttons wrap properly; shortened button text on mobile
  - Label Designer Page: Fixed select field overflow; made export buttons responsive
  - Cases Page: Reduced status card padding and font sizes for mobile screens
  - Admin Page - LED Behavior Tab: Fixed button text overflow with responsive labels; preview buttons use shortened text
  - Admin Page - ESP Controller Tab: Fixed horizontal scrolling; controller cards stack vertically; responsive button labels
  - Admin Page - Roles Tab: Grid switches to single column on mobile; user information truncates properly
  - All pages now follow mobile-first design principles with proper breakpoints (sm:, md:, lg:)
  - Improved touch targets and readability on small screens

### Version 2.62 (2025-11-14)
- **Feature: Product Package OCR Aliases** ✨ **[Issue #19]**
  - Added permanent `package_code` column plus alias management for every product package.
  - Admin UI can now define OCR keywords per package; backend normalizes/deduplicates aliases.
  - New table `product_package_aliases` (migration 020) and authenticated endpoint `GET /api/v1/product-packages/alias-map` provide one-stop lookup for RentalCore/OCR services.
  - CRUD endpoints accept `aliases` arrays so packages + mappings stay in sync.

### Version 1.11 (2025-11-14)
- **Product Packages Feature** ✨ **[Issue #19]**
  - Implemented complete product packages system for reusable product bundles
  - Added database schema with `product_packages` and `product_package_items` tables
  - Created comprehensive backend API with full CRUD operations:
    - `GET /admin/product-packages` - List all packages with search support
    - `GET /admin/product-packages/{id}` - Get detailed package with items
    - `POST /admin/product-packages` - Create new package with products
    - `PUT /admin/product-packages/{id}` - Update existing package
    - `DELETE /admin/product-packages/{id}` - Delete package
  - `POST /admin/product-packages/{id}/items` - Add item to package
  - `DELETE /admin/product-packages/{package_id}/items/{item_id}` - Remove item
  - `GET /api/v1/product-packages/alias-map` - Authenticated alias map for OCR services
  - New frontend UI integrated into Products page:
    - Product Packages tab alongside Products tab
    - Create/edit packages with product selection and quantities
    - Set package pricing for job calculations
    - View detailed package contents with product information
    - Search and filter functionality
  - Migration 019: Database tables with proper foreign keys and indexes
  - Designed for future OCR integration in RentalCore job creation
  - Documentation: `OCR_INTEGRATION_NOTES.md` with RentalCore integration guide
  - Package items support quantity tracking per product
  - Automatic cascade delete on package removal
  - All admin routes require proper authentication and authorization

### Version 1.10 (2025-11-03)
- **Case Management Admin Tab Implementation**
  - Added CasesTab component with full CRUD functionality in admin dashboard
  - Integrated Cases tab with PackageOpen icon in AdminPage navigation
  - Implemented table and cards view modes for case listing
  - Added search, status, and zone filter capabilities
  - Included device count statistics dashboard (free, rented, maintenance, total devices)
  - Support for case creation and editing with:
    - Dimensions (width, height, depth)
    - Weight tracking
    - Status management (free, rented, maintenance)
    - Zone assignment
    - Barcode and RFID tag assignment (create only)
  - Case detail modal with device list view
  - Label printing integration for cases
  - Consistent dark theme design matching other admin tabs
  - Full device management within cases from admin interface

### Version 2.61 (2025-11-14)
- **Feature: Kabel-Kombinationsübersicht in Haupttabelle**
  - The cables dashboard now groups the main table by cable type + Stecker 1 + Stecker 2 + Länge so each combination appears exactly once with an `Anzahl` column.
  - Rows expand inline to show the underlying Einzelkabel inklusive aller Aktionen (Details, Bearbeiten, Löschen); the cards view mirrors the grouped layout.
  - Summary metadata (Anzahl Kombinationen und Gesamtbestand) updates live after CRUD actions.

### Version 2.60 (2025-11-14)
- **Feature: Kabel-Kombinationsübersicht**
  - Added the first “Einzigartige Kombinationen” summary, laying the groundwork for the grouped workflow.
  - Summary strings follow the `Audio (XLR 3P - XLR 3P) • 1.00 m` pattern and display a global count for easier audits.

### Version 2.59 (2025-11-14)
- **Feature: Kabeltypen-Bestand**
  - Admin endpoint `/admin/cable-types` now includes `count` per type derived directly from the DB, so downstream tools can display accurate totals.
  - Cables UI surfaces the counts in the summary (deprecated by 2.60’s combination view, but still available for downstream integrations).

### Version 2.58 (2025-11-14)
- **UX: Filtered connector pairing in cables view**
  - Selecting a value in the Stecker 1 filter now limits Stecker 2 options to combinations that exist in the database.
  - Connector dropdowns show explicit gender labels (male/female) alongside names/abbreviations for instant clarity.
  - Invalid Stecker-2 selections reset automatically when Stecker 1 changes, preventing empty result sets.
  - Applies to both the list filters and connector displays in tables/cards/modals to keep terminology aligned.

### Version 2.57 (2025-11-14)
- **Feature: Unified Products + Devices Tree** ✨ **[Issue #17]**
  - Added a third “Gerätebaum” tab to the Products page, mirroring the former /devices view without duplicate layouts.
  - Device tree retains advanced search, LED locate button, zone navigation, and per-device detail/product modals directly inside the products workspace.
  - Removed the standalone `/devices` route, sidebar entry, and page to avoid divergent UX paths.
  - README and navigation instructions updated to point admins to the consolidated entry point.

### Version 2.51 (2025-11-01)
- **Product Availability APIs for RentalCore**
  - Added tree-based availability endpoint and job-scoped availability query powering RentalCore’s product-first job builder.
  - Exposed product-level counts alongside device metadata so RentalCore can validate requested quantities.
- **Release Packaging**
  - Published Docker image `nobentie/warehousecore:2.51` aligned with RentalCore 1.55.
  - Coordinated release notes documenting the read-only contract for device data on the RentalCore side.

### Version 2.50 (2025-11-01)
- **MAJOR REFACTOR: Headless Browser Label Rendering** 🔧✨
  - **Replaced complex freetype text rendering with headless Chromium browser**
  - **Labels now render IDENTICALLY to the Label Designer UI - guaranteed consistency**

  **The Problem:**
  - AutoGenerateDeviceLabel was using Go's freetype library for text rendering
  - freetype requires manual baseline calculation, font metrics, and glyph positioning
  - Even with correct configuration, rendering never matched browser output perfectly
  - Multiple attempts to fix text rendering (versions 2.48, 2.49) were Band-Aid solutions
  - Root cause: Server-side text rendering fundamentally different from browser canvas

  **The Solution:**
  - Implemented headless Chrome (chromedp) for label rendering
  - Created HTML template that uses EXACT same canvas code as Label Designer
  - Server loads template + device data, renders in headless browser, captures PNG
  - Zero rendering differences between designer preview and auto-generated labels

  **Technical Implementation:**
  - Added chromedp library for headless browser control
  - Created `/internal/services/label_template.html` with canvas rendering
  - Modified `AutoGenerateDeviceLabel()` to:
    1. Generate label data (device info, barcodes, QR codes)
    2. Inject data into HTML template
    3. Render in headless Chrome with 2-second wait
    4. Extract canvas.toDataURL() result
    5. Save PNG to disk
  - Updated Dockerfile with Chromium + required dependencies:
    * chromium, chromium-chromedriver
    * nss, freetype, harfbuzz, ttf-freefont
  - Removed freetype/TrueType font loading code (no longer needed)
  - Simplified LabelService struct (removed defaultFont field)

  **Benefits:**
  - ✅ 100% identical rendering to Label Designer
  - ✅ No complex font calculations or baseline positioning
  - ✅ Future-proof: designer improvements auto-apply to auto-generation
  - ✅ Browser handles all text rendering, scaling, and positioning
  - ✅ Professional solution used by screenshot/PDF tools industry-wide

  **Impact:**
  - All auto-generated labels now look exactly like designer previews
  - Text renders perfectly with proper fonts, sizes, and positioning
  - No more rendering inconsistencies or debugging font issues
  - Maintenance burden drastically reduced

  **Files Changed:**
  - `/internal/services/label_service.go`: Complete refactor of AutoGenerateDeviceLabel
  - `/internal/services/label_template.html`: New HTML template for rendering
  - `/Dockerfile`: Added Chromium and rendering dependencies
  - `/go.mod`, `/go.sum`: Added chromedp library

  **Docker Image Size:**
  - Increased by ~50MB due to Chromium installation
  - Trade-off justified by perfect rendering and maintainability

### Version 2.49 (2025-11-01)
- **CRITICAL FIX: Freetype Text Rendering - Add Missing SetHinting Call** 🔧
  - **Fixed invisible text issue where labels were generating without any visible text**
  - **Root Cause:** After the freetype rewrite in 2.48, text was invisible because the freetype.Context was missing the required `SetHinting()` call

  **The Problem:**
  - Labels were being generated but text was NOT VISIBLE
  - Barcode and QR codes were rendering correctly
  - Text elements were being processed but not appearing on labels
  - Possible causes: white text on white background, text rendering outside bounds, or misconfigured freetype context

  **Root Cause Analysis:**
  - Missing `c.SetHinting(font.HintingFull)` call in freetype.Context setup (line 636)
  - This is a REQUIRED configuration for freetype to actually render glyphs
  - Without hinting, the freetype library does not properly rasterize text to the image
  - The Context was being created and configured, but missing this critical step

  **The Fix:**
  - ✅ Added `c.SetHinting(font.HintingFull)` to freetype.Context configuration
  - ✅ Imported `golang.org/x/image/font` package for hinting constants
  - ✅ Simplified baseline calculation (removed incorrect font metrics approach)
  - ✅ Enhanced logging to detect font loading failures
  - ✅ Added explicit error logging when font is not available

  **Technical Changes:**
  - **Line 22:** Added `import "golang.org/x/image/font"` for HintingFull constant
  - **Line 636:** Added `c.SetHinting(font.HintingFull)` - CRITICAL fix
  - **Lines 33-50:** Enhanced font loading logs with `[LABEL INIT]` and `[LABEL ERROR]` prefixes
  - **Lines 644-662:** Improved baseline calculation and added comprehensive debug output
  - **Line 661:** Added error log when font is missing

  **Complete freetype.Context Setup (Correct Order):**
  ```go
  c := freetype.NewContext()
  c.SetDPI(300)                          // 300 DPI for print quality
  c.SetFont(s.defaultFont)               // TrueType font
  c.SetFontSize(fontSize)                // Font size in points
  c.SetClip(labelImage.Bounds())         // Clip region
  c.SetDst(labelImage)                   // Destination image
  c.SetSrc(image.NewUniform(textColor))  // Text color (black)
  c.SetHinting(font.HintingFull)         // ← THIS WAS MISSING!
  ```

  **Before vs After:**
  - **Before:** Labels generated with invisible text (white or missing)
  - **After:** Text renders correctly in black at proper size and position

  **Impact:**
  - ✅ Text now VISIBLE on all generated labels
  - ✅ Proper black text color on white background
  - ✅ Correct font rendering with proper hinting
  - ✅ Labels are now production-ready with visible device IDs and text
  - ✅ All device and case labels display correctly

### Version 2.48 (2025-11-01)
- **Label Text Rendering Fix: Proper Font Sizing and Letter Spacing** 📝
  - Fixed critical text rendering issues where labels had text that was too small and letters spaced too far apart
  - **The Problems:**
    - Default font size was only 11pt, resulting in very small text on labels
    - Letter spacing calculation was completely wrong
    - Characters were spaced at `7 * scaleFactor` pixels (for 11pt = 7 * 3.5 = 24.5px), far too wide
    - Text baseline calculation was overly complex and incorrect
    - Letters appeared like "T   S   U   N   A   M   I" instead of "TSUNAMI"
  - **Root Cause:**
    - Lines 614-621: Individual character drawing used excessive spacing
    - Line 595: Scale factor calculation `(fontSize / 72.0 * 300.0) / 13.0` was applied incorrectly
    - Line 598: Y position calculation duplicated scale factor math unnecessarily
    - Default font size (11pt) was too small for practical label reading
  - **The Fix:**
    - **Font Size:** Increased default from 11pt to 16pt for better readability
    - **Scale Factor:** Simplified calculation to `(fontSize * 300.0 / 72.0) / 13.0`
      * For 16pt: scale = (16 * 4.167) / 13 = 5.13x
      * For 11pt (template override): scale = (11 * 4.167) / 13 = 3.53x
    - **Letter Spacing:** Fixed to use proper character spacing
      * Changed from `int(7 * scaleFactor)` to `charWidth * scaleFactor * 1.15`
      * 1.15x multiplier provides natural letter spacing (15% gap)
      * For 16pt: 7px * 5.13 * 1.15 = 41.3px per character (vs 35.9px broken)
      * For 11pt: 7px * 3.53 * 1.15 = 28.4px per character (vs 24.5px broken)
    - **Baseline Position:** Simplified to `y + int(fontSize * 300 / 72)`
      * Cleaner calculation, proper vertical alignment
  - **Technical Details:**
    - Font scaling at 300 DPI: 1pt = 300/72 = 4.167 pixels
    - basicfont.Face7x13: 7px character width, 13px height
    - Character spacing now: base width × scale × 1.15 for natural appearance
    - Text is now readable and properly spaced for professional labels
  - **Before vs After:**
    - Before: "D  E  V  I  C  E    1  2  3" (huge gaps, tiny text)
    - After: "DEVICE 123" (natural spacing, readable size)
  - **Impact:**
    - All labels now have properly sized, readable text
    - Letter spacing appears natural and professional
    - Template font_size property is properly respected
    - Default labels use larger 16pt font unless template specifies otherwise
  - **Files Changed:**
    - `/internal/services/label_service.go`: Text rendering improvements (lines 553-631)

### Version 2.47 (2025-11-01)
- **Label Rendering Fix: Properly Scaled Elements** 🏷️
  - Fixed critical label rendering issues where elements were incorrectly sized and positioned
  - **The Problems:**
    - Label canvas size was correct (590x295px for 50x25mm at 300 DPI)
    - Images/barcodes/QR codes were drawn at NATIVE resolution, ignoring template dimensions
    - Logo image (512x512px) was drawn at full size, causing massive overflow (only half visible)
    - Text used fixed-size font (7x13px) regardless of template font_size property
    - Barcode and text appeared tiny because they weren't scaled properly
  - **Root Cause:**
    - Image drawing code (line 629-631) used `elemImg.Bounds()` directly without scaling
    - A 512px logo placed at x=377px (32mm) extended to 889px - way beyond label width!
    - Text rendering ignored font_size style property and used basicfont.Face7x13 at fixed size
  - **The Fix:**
    - **Image Scaling:** Implemented proper image scaling to template dimensions
      * Extract target width/height from template (in mm, converted to pixels)
      * Create scaled image using pixel-by-pixel resampling
      * Scale both dimensions: `scaleX` and `scaleY` calculated from source to target
      * Now 512x512px logo scales to 189x189px (16x16mm) and fits perfectly
    - **Text Rendering:** Improved font size handling
      * Changed style key from `fontSize` to `font_size` (matching template JSON)
      * Calculate scale factor based on desired point size at 300 DPI
      * For larger text (scale > 1.5x), draw characters individually with proper spacing
      * Text baseline positioning adjusted for proper alignment
  - **Technical Details:**
    - MM to Pixel conversion: `pixels = mm * 300 DPI / 25.4 mm/inch ≈ mm * 11.8`
    - Template: 50x25mm → 590x295px canvas
    - Logo: 16x16mm → 189x189px (was 512x512px unscaled)
    - Barcode: 33x8mm → 389x94px (now properly sized)
    - Text: 11pt → ~46px height at 300 DPI (was 13px fixed)
  - **Impact:**
    - All label elements now properly sized and positioned within boundaries
    - Logo no longer cut off - scales to fit template dimensions
    - Barcode and text are now appropriately sized for professional labels
    - Labels ready for printing at correct 300 DPI resolution
  - **Files Changed:**
    - `/internal/services/label_service.go`: Image scaling and text rendering improvements

### Version 1.55 (2025-11-01)
- **Critical Bug Fix: Automatic Label Generation SQL Error** 🔧
  - Fixed automatic label generation that was completely broken since implementation
  - **Root Cause:** SQL query used snake_case column names (device_id) but database uses camelCase (deviceID)
  - **The Problem:**
    - Every device creation attempted to generate a label automatically
    - ALL label generation attempts failed with: `Unknown column 'd.device_id' in 'field list'`
    - Error was logged but silently skipped, so users didn't know labels weren't being created
    - Logs showed: `[LABEL CREATE ERROR] Failed to generate label for device X`
  - **Additional Issues Found:**
    - Query tried to join devices directly to categories/subbiercategories
    - Database structure: devices → products → categories/subbiercategories
    - Missing JOIN through products table caused query to fail
    - Used non-existent d.name column (should be p.name for product name)
  - **The Fix:**
    - Updated GenerateLabelForDevice query to use correct camelCase column names:
      * `d.deviceID` instead of `d.device_id`
      * `p.productID` instead of `p.product_id`
    - Fixed table relationships with proper JOINs:
      * `devices d` → `products p` (via d.productID = p.productID)
      * `products p` → `subbiercategories sb` (via p.subbiercategoryID = sb.subbiercategoryID)
      * `products p` → `categories c` (via p.categoryID = c.categoryID)
    - Changed device name field from d.name to p.name (product name)
  - **Impact:**
    - Automatic label generation NOW WORKS when creating devices
    - Labels are properly generated in background using default template
    - No more SQL errors in logs
    - Users no longer need to manually generate labels for new devices
  - **Verified:** Docker logs show successful label generation after deploying version 1.55

### Version 2.45 (2025-11-01)
- **Critical Bug Fix: Device Creation Route Handler** 🔧
  - Fixed device creation failing when creating products with quantity specified in ProductsTab
  - **Root Cause:** Handler was reading product_id from request body instead of URL path parameter
  - **The Issue:**
    - Frontend correctly called: `POST /admin/products/{productId}/devices` with productId in URL
    - Backend handler incorrectly expected product_id in request body
    - This mismatch caused the handler to receive product_id as 0, failing validation
  - **The Fix:**
    - Modified `CreateDevicesForProduct` handler to extract product ID from URL path using `mux.Vars(r)["id"]`
    - Product ID now correctly parsed from `/admin/products/{id}/devices` route parameter
    - Request body now only contains `quantity` and `prefix` fields
    - Added comprehensive logging at handler entry point
  - **Technical Details:**
    - Changed from: `req.ProductID` (from body) to `productID := mux.Vars(r)["id"]` (from URL)
    - Fixed Go compilation error (variable redeclaration)
    - Improved error messages and validation flow
  - **Impact:** Device creation from ProductsTab now works as intended. Creating a product with device_quantity > 0 will automatically create the specified number of devices with proper IDs.

### Version 2.44 (2025-11-01)
- **Bug Fix: pos_in_category Validation Blocking Device Creation** 🔧
  - Removed overly strict validation that incorrectly blocked device creation for legacy products
  - **Root Cause Analysis:**
    - Database trigger `pos_in_subcategory` automatically sets `pos_in_category` for NEW products on INSERT
    - Legacy products created before trigger existed have NULL `pos_in_category` values
    - Validation was checking for NULL and blocking device creation even though devices could be created
  - **Database Fix:** Updated all existing products with NULL `pos_in_category` to proper sequential values
  - **Code Fix:** Changed validation from blocking error to warning log for NULL values
  - **Improved Error Messages:** Error messages now distinguish between NULL (legacy) and valid pos_in_category values
  - **Enhanced Logging:** Added detailed logging to track legacy vs new product handling

  **Technical Details:**
  - Products with NULL `pos_in_category`: 23, 24, 25, 26, 30, 48 (now fixed)
  - Trigger location: RentalCore.sql lines 1305-1318
  - Device creation now works for ALL products with valid subcategory abbreviation

  **Impact:** Device creation now works correctly for both legacy and new products. Users are no longer blocked from creating devices for valid products that have been in the system before the trigger was added.

### Version 2.46 (2025-11-01)
- **Feature: Auto-Label Creation on Device Creation** 🏷️✨
  - When devices are created, labels are now automatically generated in the background
  - Uses the saved default label template from the Label Designer
  - Labels are pre-rendered and saved to `/labels/{device_id}_label.png`
  - No manual action required - labels ready immediately after device creation
  - Added logging: `[LABEL CREATE] Generating label for device X using default template`
  - Works seamlessly with batch device creation (up to 100 devices)
  - Silently skips label generation if no default template is configured

- **Feature: Cascade Delete for Products** 🗑️
  - Products can now be deleted even if they have associated devices
  - Automatically deletes all devices when a product is deleted
  - Added comprehensive logging: `[PRODUCT DELETE] Deleting N devices for product X before deletion`
  - API response includes count of deleted devices
  - Detailed device ID logging for audit trail
  - Example response: `"Product deleted successfully along with 15 device(s)"`
  - Prevents orphaned devices in the database

  **Impact:** Streamlined workflows - devices get labels automatically, and product cleanup is effortless.

### Version 2.43 (2025-11-01)
- **Critical Fix: Device Creation Silent Failures** 🔧
  - Fixed devices not being created when creating products with quantity specified
  - Root cause: Database trigger requires `subcategoryID` with abbreviation and `pos_in_category`
  - Added pre-flight validation to verify product has required fields before device creation
  - Returns clear error messages when required fields are missing instead of silent failure
  - Added comprehensive debug logging throughout device creation workflow
  - Tracks and logs failed device insertions for better troubleshooting
  - Returns HTTP 500 error if no devices were created despite quantity being requested

  **Impact:** Users now receive immediate feedback if their product configuration is incomplete
  for device creation, preventing confusion when devices aren't generated.

### Version 2.42 (2025-11-01)
- **Bug Fix: Product Creation Device Generation** 🔧
  - Fixed device creation endpoint from `/products/create-devices` to `/admin/products/{id}/devices`
  - Devices are now properly created when specifying quantity during product creation
  - Database triggers correctly generate device IDs with proper naming convention

- **Bug Fix: Product Tree Display** 🌳
  - Modified GetDeviceTree query to include products even without devices
  - Added `WHERE p.productID IS NOT NULL` filter to prevent empty category rows
  - Products now appear in /devices page tree structure regardless of device count
  - Improved query structure to prioritize products over devices in tree building

### Version 2.2 (2025-10-24)
- **Bug Fix: Case Label Size Rendering** 🔧
  - Fixed case labels not using correct template dimensions
  - Increased template load wait time from 500ms to 1000ms for proper initialization
  - Increased per-item render wait from 300ms to 500ms for consistent canvas rendering
  - Prevents race conditions where template size wasn't applied before generating
  - Cases now get same label size (e.g., 62x29mm) as devices
  - Ensures uniform label appearance across all inventory items

### Version 2.1 (2025-10-24)
- **Bug Fix: Frontend Build for Label Generation** 🔧
  - Fixed TypeScript error in Device interface usage
  - Rebuilt React frontend with corrected field mapping
  - Frontend bundle updated with working case label generation

### Version 2.0 (2025-10-24)
- **Major Feature: Unified Label Generation for Devices & Cases** ✨
  - Label Designer now generates labels for BOTH devices and cases with single button
  - "Alle Labels Generieren" creates labels for entire inventory at once
  - Cases use same template as devices for consistent appearance
  - Automatic field mapping: device_id → CASE-{id}, product_name → case.name
  - Button shows breakdown: "(X Devices + Y Cases)"
  - Complete workflow: Design template → Generate all → Labels saved automatically
  - Streamlines inventory labeling process significantly

### Version 1.99 (2025-10-24)
- **Critical Fix: Case Label Generation** 🔧
  - Removed zones table dependency from case label query
  - Fixed barcode size calculation with proper mm to pixels conversion at 300 DPI
  - Added minimum size constraints for Code128 barcodes and QR codes
  - Resolves 500 errors and 'barcode too small' issues

### Version 1.98 (2025-10-24)
- **Bug Fix: Zones Table Dependency** 🐛
  - Removed LEFT JOIN on zones table from case label query
  - Fixes "Table 'RentalCore.zones' doesn't exist" error

### Version 1.97 (2025-10-24)
- **Feature: Case Label Image Saving** 💾
  - Added `label_path` column to cases table for persistent label storage
  - Created SaveCaseLabelImage function for saving case label images to disk
  - New backend endpoint: POST /api/v1/labels/save-case
  - Case labels saved to separate directory: /labels/cases/
  - Case records updated with label_path after generation
  - Frontend API updated with saveCaseLabel function
  - Enables reuse of generated labels without regeneration
  - Consistent with device label handling

### Version 1.96 (2025-10-24)
- **Bug Fix: Case Label Static Images** 🐛
  - Fixed 500 error when label templates contain static image elements
  - Added ImageData field to LabelElement struct for proper JSON unmarshaling
  - Backend now copies static image data from templates to processed elements
  - Frontend updated to render static image elements
  - Resolves issue with Template ID 4 containing logo/watermark images
  - Complete fix for case label generation with device templates

### Version 1.95 (2025-10-24)
- **Bug Fix: Case Label Field Mapping** 🐛
  - Fixed case label generation not working with device templates
  - Added support for device_id and product_name aliases in case label generation
  - Case labels now work with existing device templates
  - device_id maps to case_id (formatted as CASE-XXX)
  - product_name maps to case name
  - Allows reusing device label templates for cases

### Version 1.94 (2025-10-24)
- **Feature: Products & Categories Management** ✨ **[Issue #11]**
  - Added full backend API for 3-tier category structure (Categories > Subcategories > Sub-subcategories)
  - Created CRUD endpoints for all category levels
  - Implemented Products API with full filtering and search capabilities
  - Added bulk device creation endpoint for products
  - New admin UI tabs for Categories and Products management
  - Category management with create, edit, delete operations
  - Products list view with category hierarchy display
  - Foundation for expanded product and device management
  - Routes: GET/POST/PUT/DELETE /admin/categories, /admin/subcategories, /admin/subbiercategories, /admin/products
  - Bulk device creation: POST /admin/products/{id}/devices

### Version 1.93 (2025-10-24)
- **Feature: Case Label Generation** ✨ **[Issue #12]**
  - Added label printing functionality for cases
  - New backend endpoint: POST /api/v1/labels/case/{case_id}
  - Added GenerateLabelForCase service method supporting case-specific fields
  - Support for case_id, name, barcode, RFID tag, dimensions, weight, and zone
  - Frontend "Label" button on Cases page with printer icon
  - Opens label preview in new window with print functionality
  - Generates QR codes and barcodes from case data
  - Uses default label template from Label Designer
  - Supports all case fields in label templates

### Version 1.92 (2025-10-24)
- **Bug Fix: LED Clear Functionality** 🐛 **[Issue #6]**
  - Fixed LEDs not clearing when using clear buttons or leaving job page
  - Updated ClearAllLEDs function to support multi-controller architecture
  - Now sends clear command to all active LED controllers in database
  - Added fallback to default topic for backwards compatibility
  - LEDs now properly clear when toggling off, leaving job page, or on cleanup
  - Improved error handling with detailed logging per controller
  - Solves issue where LEDs stayed on across multiple ESP32 controllers

### Version 1.91 (2025-10-24)
- **Bug Fix: Job Redirect After Scanning** 🐛 **[Issue #7]**
  - Fixed job page staying empty after scanning job ID barcode
  - Added route for `/jobs/:id` to support direct job links
  - JobsPage now uses useParams to detect job ID from URL
  - Automatically loads job details when accessed via direct link
  - LED highlighting still works correctly on redirect
  - Page now properly loads after scanning JOB codes

### Version 1.90 (2025-10-24)
- **Bug Fix: Scanned Item Highlight Persistence** 🐛 **[Issue #8]**
  - Fixed green highlight remaining on devices after being scanned back into warehouse
  - Updated scanned flag logic to only highlight devices currently on_job
  - Devices returned to storage (in_storage status) no longer show as scanned
  - Improved visual feedback accuracy in job packing workflow
  - Changed logic from `status == "on_job" || pack_status == "issued"` to `status == "on_job"`

### Version 1.89 (2025-10-24)
- **UI Update: Renamed "Zonen" to "Lager"** 🎨 **[Issue #10]**
  - Renamed all occurrences of "Zonen" to "Lager" throughout the app
  - Updated navigation menu: "Zonen" → "Lager"
  - Updated admin panel: "Zonentypen" → "Lagertypen"
  - Updated LED settings: "Zonentyp" → "Lagertyp"
  - Updated controllers: "Zonenarten" → "Lagerarten"
  - Updated page titles and breadcrumbs
  - Updated search placeholders and user-facing text
  - Consistent terminology throughout the entire application

### Version 1.88 (2025-10-24)
- **Feature: Cases CRUD Operations** ✨ **[Issue #9]**
  - Added full Create, Update, Delete operations for cases
  - New "Neues Case" button to create cases in WarehouseCore
  - Edit button on each case card for inline editing
  - Delete button with confirmation and validation (prevents deletion if case contains devices)
  - Comprehensive form modal with all case fields:
    - Name, Description, Status (free/rented/maintance)
    - Dimensions (Width, Height, Depth)
    - Weight, Zone assignment
    - Barcode and RFID tag (for new cases)
  - Zone dropdown populated from warehouse zones
  - Backend handlers: CreateCase, UpdateCase, DeleteCase
  - Full error handling and user feedback
  - Cases page now provides complete management instead of read-only view

### Version 1.87 (2025-10-24)
- **Bug Fix: /labels Page Routing Issue** 🐛 **[Issue #13]**
  - Fixed /labels page showing directory content after hard reload
  - Updated spaHandler to check if path is a directory
  - Directories now properly serve index.html for SPA routing
  - Label images remain accessible while preventing directory listing
  - Improves routing consistency across all SPA pages

### Version 1.86 (2025-10-23)
- **Bug Fix: Device Modal Scrolling** 🐛
  - Made Device Modal scrollable for long content
  - Changed outer container from items-center to items-start
  - Added overflow-y-auto to enable vertical scrolling
  - Added my-8 margin to prevent modal from touching viewport edges
  - Modal now properly scrolls when content exceeds viewport height

### Version 1.85 (2025-10-23)
- **Bug Fix: Modal z-index Layering** 🐛
  - Fixed Device Modal z-index from z-50 to z-[70]
  - Device Modal now appears in front of Product Modal (z-[60])
  - Proper modal stacking when opening device from product view
  - Improved user experience with correct modal layering

### Version 1.84 (2025-10-23)
- **Feature: Multiple Label Templates** 🏷️✨
  - Added full template management system
  - Create, edit, delete, and switch between multiple label templates
  - Set any template as default for label generation
  - Template selector dropdown with star indicator for default template
  - "New Template" option to start fresh designs
  - Save/Update button dynamically changes based on template state
  - "Set as Default" button for non-default templates
  - Delete button with confirmation for existing templates
  - Template name input field for easy identification
  - Generate All Labels now uses the default template
  - Original template restored after bulk generation

- **UI Improvements:**
  - Template management section at top of left panel
  - Clear visual indicators for current and default templates
  - Improved workflow: Design → Save → Set as Default → Generate
  - Updated page description to reflect multi-template capability

- **Technical:**
  - State management for templates list and current template ID
  - loadTemplates() fetches all templates on page load
  - loadTemplate() switches between templates
  - createNewTemplate() resets to blank canvas
  - deleteTemplate() with cascade check and confirmation
  - setAsDefault() marks template for label generation
  - TypeScript null-safety for template IDs

### Version 1.83 (2025-10-23)
- **Bug Fix: Label Path in API Response** 🐛
  - Added label_path field to GetDevices and GetDevice SQL queries
  - label_path now properly returned in DeviceResponse JSON
  - Device modal can now display stored label images
  - Fixed SQL SELECT to include d.label_path column
  - Updated Scan to include LabelPath field
  - Added LabelPath validation and mapping to response

### Version 1.82 (2025-10-23)
- **Bug Fix: Label File Path** 🐛
  - Fixed label storage path from `/root/web/dist/labels` to `./web/dist/labels`
  - Labels are now correctly served by the static file server
  - Device modal now displays generated labels properly
  - Ensures compatibility with Docker container file system

### Version 1.81 (2025-10-23)
- **Major Refactor: Label Generation & Storage System** 🏷️
  - Labels are now pre-generated and stored as PNG files on disk
  - New "Generate All Labels" button in Label Designer
  - Labels saved to `/root/web/dist/labels/` with naming `{device_id}_label.png`
  - Added `label_path` column to devices table (migration 017)
  - Device detail modal shows pre-generated label image instead of canvas rendering
  - Much faster label display and consistent rendering
  - Fixed ESC key to close device detail modal
  - Separate "Generate All Labels" (saves to disk) and "Export All Labels" (downloads) buttons

- **Backend Changes:**
  - New `SaveDeviceLabel` API endpoint: POST `/api/v1/labels/save`
  - `SaveLabelImage()` function in LabelService saves base64 images to filesystem
  - Database updated with label file path after generation
  - GORM-based label path updates

- **Frontend Changes:**
  - Device Modal: Shows `<img>` tag with stored label instead of canvas rendering
  - Label Designer: `generateAllLabels()` function sends labels to backend for storage
  - API: `saveLabel()` function added to labelsApi
  - Device interface extended with `label_path` field
  - Fixed label designer preview dropdown to show all devices (not limited to 20)

- **User Workflow:**
  1. Design label template in Label Designer
  2. Click "Generate All Labels" → Labels are rendered and saved for all devices
  3. Open device details → See the pre-generated label immediately
  4. Download individual labels or bulk export

### Version 1.80 (2025-10-23)
- **Bug Fix: Label Display Scaling in Device Modal** 🐛
  - Fixed oversized label preview in device detail modal
  - Reduced DPI from 300 to 150 for more appropriate display size
  - Added CSS max-width (400px) and max-height (200px) constraints
  - Label now displays at reasonable size while maintaining good quality
  - Download still provides full-quality PNG export

### Version 1.79 (2025-10-23)
- **Feature: Device Detail Label Preview** 🏷️
  - Added label preview directly in device detail modal
  - Automatically loads default label template when device details are opened
  - Canvas rendering of device-specific label with QR codes, barcodes, and text
  - Download button to save individual device label as PNG
  - Label section appears below LED controls with "Geräte-Label" heading
  - Uses same rendering logic as Label Designer for consistency
  - Loading state while template and label are being generated
  - Integrates seamlessly with existing device detail modal design

- **Bug Fix: Label Designer Preview Dropdown** 🐛
  - Fixed preview dropdown to show ALL devices instead of just first 20
  - Removed `.slice(0, 20)` limitation from device list
  - Users can now select any device for label preview

- **Technical Details:**
  - Modified DeviceDetailModal.tsx to add label rendering functionality
  - Added labelsApi integration for template fetching and barcode/QR generation
  - Canvas API rendering with 300 DPI resolution for print quality
  - Automatic element rendering: text, QR codes, barcodes, and images
  - Font family support (Arial, Ubuntu, Aptos, etc.) from template
  - Device data mapping: device_id, product_name fields
  - Responsive download functionality with automatic filename generation

### Version 1.59 (2025-10-23)
- **Major Redesign: Label Designer UI/UX Overhaul** 🎨
  - Complete visual redesign with Dark Theme and Glassmorphism effects
  - Matches global app theme (dark background, glass panels, red accent color)
  - GUI-based designer: Add elements via buttons, no JSON editing required
  - Simplified workflow: One global template for ALL devices automatically
  - Automatic bulk export: Generate labels for ALL devices without manual selection
  - Three-column layout: Settings | Live Preview | Elements & Actions
  - Visual element management: Icon-based element list with click-to-select
  - Property editor with real-time preview updates
  - Preset label sizes with custom dimension support
  - Improved UX: Template save, preview device selector, live canvas rendering
  - Enhanced accessibility: Larger buttons, better contrast, clearer labels
  - Responsive design with breakpoints for mobile/tablet/desktop

  **Design System:**
  - Dark background (#0B0B0B)
  - Glass-dark panels with backdrop blur
  - Red accent color (#D0021B) for primary actions
  - Smooth transitions and hover effects
  - Custom scrollbar styling

### Version 1.58 (2025-10-23)
- **Feature: Barcode & Label Generation System** 🏷️
  - Full barcode/QR-code generation API with Go libraries (skip2/go-qrcode, boombuler/barcode)
  - Label template system with database storage (label_templates table)
  - CRUD API for label templates with JSON-based element definitions
  - Canvas-based Label Designer UI with live preview
  - Support for QR codes, Code128 barcodes, text, and images on labels
  - Customizable label sizes (62x29mm, 100x50mm, custom dimensions)
  - Template elements support positioning, rotation, styling
  - Field mapping for device data (device_id, product name, category)
  - Bulk export: Generate and download labels for multiple devices as PNG
  - Print functionality with browser print dialog integration
  - Pre-seeded templates: Standard device label (62x29mm QR), Large label (100x50mm barcode)
  - New navigation menu item: "Labels" with Tag icon
  - API endpoints: /labels/qrcode, /labels/barcode, /labels/templates, /labels/device/{id}
  - Addresses GitLab Issue #5: Barcode generation & label designer

  **New Files:**
  - internal/models/label.go - Label template models
  - internal/services/label_service.go - Label generation service
  - internal/handlers/label_handler.go - API handlers
  - migrations/016_create_label_templates.sql - Database migration
  - web/src/pages/LabelDesignerPage.tsx - Designer UI component
  - web/src/pages/LabelDesignerPage.css - Designer styling

### Version 1.57 (2025-10-23)
- **Feature: Comprehensive Multi-ESP32 Setup Guide** 📚
  - Created detailed standalone guide MULTI_ESP32_GUIDE.md
  - Includes hardware requirements, shopping list with prices
  - Step-by-step Arduino IDE setup and library installation
  - Wiring diagrams with ASCII art for ESP32 → Level Shifter → LEDs
  - Complete firmware configuration with secrets.h template
  - Flashing instructions with troubleshooting
  - Admin panel management workflow
  - Zone type assignment and multi-controller routing explanation
  - Testing procedures (Identify, Fach-Test, Job-Highlight)
  - Comprehensive troubleshooting section with error codes
  - Added reference link in main README for easy access

### Version 1.56 (2025-10-23)
- **Feature: Multi ESP32 Documentation** 📚
  - Added comprehensive Multi-ESP32 Quick Start guide in main README
  - Clear step-by-step instructions for flashing, configuring, and managing multiple ESP32s
  - Links to detailed firmware documentation
  - Zone routing explanation for multi-controller setups
  - Admin panel feature overview for controller management
  - Display name and zone type assignment instructions
  - Fixes GitLab Issue #4: Multi ESP32 Support documentation

### Version 1.55 (2025-10-23)
- **Feature: Job-Code Scan Functionality** 🎯
  - Added job-code scanning capability to /scan page
  - Recognizes JOB######format codes (e.g., JOB000123)
  - Smart LED integration with automatic status detection
  - If LED connected: Auto-highlight job devices and navigate to job
  - If LED disconnected: Shows modal asking user if they want to enable LED
  - Modal options: "Ja, LED aktivieren" or "Nein, direkt zum Job"
  - Seamless navigation to job details page after scan
  - Fixes GitLab Issue #3: Missing job-code scan function

### Version 1.54 (2025-10-19)
- **Feature: LED Control Improvements** 💡
  - Added manual "Ausschalten" button to turn off orange locate LEDs
  - LEDs automatically turn off when closing device detail modal
  - Improved UX with visual feedback for LED state
  - Dual-button layout: "Fach beleuchten" (orange) and "Ausschalten" (red)
  - Disabled "Fach beleuchten" button while LEDs are active

### Version 1.53 (2025-10-19)
- **Enhancement: Synchronous LED Breathe Effect** 🎨
  - Changed orange color from #FF8C00 to #FF4500 (more vibrant OrangeRed)
  - Increased LED intensity from 200 to 255 for maximum brightness
  - Fixed ESP32 firmware to synchronize breathe animation across all LEDs in bin
  - Changed phaseOffset from random to 0 for perfect synchronization
  - All LEDs in the same bin now breathe in perfect harmony

### Version 1.52 (2025-10-19)
- **Bug Fix: Device API JSON Serialization** 🐛
  - Fixed React error #31 (Objects not valid as React child)
  - Resolved sql.Null* types being serialized as objects in JSON
  - Device API now returns clean JSON with proper types
  - Added DeviceResponse struct with pointer types for nullable fields
  - Fixed database joins for cases and jobs (using devicescases and jobdevices tables)
  - API responses now properly handle nullable fields (serial_number, barcode, qr_code, etc.)
  - Device details modal now renders correctly without browser console errors

### Version 1.51 (2025-10-19)
- **Bug Fix: Database Query Corrections**
  - Fixed SQL query to use correct job table column (jobID instead of jobNumber)
  - Added proper CAST for job ID to string conversion

### Version 1.50 (2025-10-19)
- **Bug Fix: Initial Device API Response Structure**
  - Attempted fix for JSON serialization issues with Device model

### Version 1.49 (2025-10-19)
- **Feature: Device Detail Modal on Devices Page** 📱
  - Click any device on /devices page to open detail modal
  - Same full device information popup as on zone detail page
  - "Fach beleuchten" button with orange breathe LED
  - Consistent UX across all device views
- **User Experience:**
  - Device cards now clickable on main devices page
  - Quick access to device details from anywhere
  - Instant LED location from device list

### Version 1.48 (2025-10-18)
- **Feature: Device Detail Modal with LED Locate** 🔍💡
  - Click any device to open detail popup
  - Shows complete device information:
    - Device ID, serial number, barcode, QR code
    - Product name, status, condition rating
    - Location (zone name and code)
    - Usage hours, case assignment, job number
  - **"Fach beleuchten" button**: Highlights device's bin with orange breathe pattern
  - Orange breathe LED makes finding devices easy
  - Modal accessible from zone detail page device list
- **Backend Enhancement:**
  - New LED locate API: POST /api/v1/led/locate?bin_code=XXX
  - LocateBin service function with orange breathe pattern
  - Added zone_code to Device API responses
  - Updated DeviceWithDetails model with zone_code field
- **Database Updates:**
  - GetDevices, GetDevice, GetZoneDevices now return zone_code
  - Enhanced device queries with serial_number, condition_rating, usage_hours
- **User Experience:**
  - Hover effect on device rows
  - One-click access to full device details
  - Instant LED location for faster picking
  - Beautiful modal with organized information layout

### Version 1.47 (2025-10-18)
- **Feature: Automatic LED Cleanup on Page Exit** 🔴
  - LEDs automatically turn off when leaving job page
  - LEDs turn off when navigating to different page
  - LEDs turn off when closing browser or reloading page
  - useEffect cleanup hooks ensure no LEDs left on
  - beforeunload event handler for browser close
  - Prevents LEDs from staying on after job is finished
- **User Experience:**
  - No manual LED turn-off needed
  - System automatically resets LED state
  - Clean workflow when switching between jobs

### Version 1.46 (2025-10-18)
- **Feature: Reset Pack Status on Device Intake** ♻️
  - Devices returned to warehouse now reset to "not scanned" state
  - Updates pack_status='pending' instead of deleting job assignment
  - Clears pack_ts timestamp
  - Device stays assigned to job but appears unchecked
  - Allows re-scanning device for same job
- **Implementation:**
  - Modified scan_service.go processIntake()
  - UPDATE jobdevices instead of DELETE
  - Device-job relationship preserved

### Version 1.45 (2025-10-18)
- **Feature: Complete LED Refresh After Device Scan** 🔄
  - All bins for job stay visible after device outtake
  - GREEN bins: Still have devices for job
  - RED bins: All devices taken or not needed for job
  - Single MQTT command updates all bins
  - getJobDeviceZonesWithCounts() groups devices by zone
- **Bug Fix:**
  - Fixed issue where all LEDs turned off after single scan
  - Now sends complete state refresh for entire job

### Version 1.44 (2025-10-18)
- **Feature: Live LED Updates on Device Scan** 🔴🟢
  - LEDs automatically update after device outtake scan
  - Bin turns RED when last device is taken
  - Bin stays GREEN while devices remain
  - Asynchronous update via goroutine
  - No manual LED refresh needed
- **Implementation:**
  - Added UpdateBinAfterScan() to led/service.go
  - Integrated into ProcessScan() as background task
  - Queries remaining device count per zone

### Version 1.43 (2025-10-18)
- **Bug Fix: Device Tree String-Based IDs** 🔧
  - Fixed type mismatch for category IDs
  - Changed from int64 to string-based maps
  - subcategoryID and subbiercategoryID are VARCHAR(50)
  - Device tree now loads correctly

### Version 5.8.7 (2026-04-19)
- **Feature: Packliste in Job-Detailansicht**
  - Neuer Tab "Packliste" neben "Geräte-Liste" in der Job-Detailansicht
  - Zeigt alle Produktanforderungen (`job_product_requirements`) mit benötigter und gebuchter Menge
  - Farbkodierter Fortschrittsbalken pro Produkt: grün (erfüllt), orange (teilweise), rot (keine)
  - Gesamtfortschritt-Balken oben (X von Y Positionen vollständig)
  - Auto-Refresh alle 3 Sekunden bei aktivem Packliste-Tab
- **API Endpoint:**
  - `GET /api/v1/jobs/{id}/requirements` - Produktanforderungen mit Buchungsstatus
- **Dateien:**
  - `internal/handlers/handlers.go` - GetJobRequirements Handler
  - `cmd/server/main.go` - Route registriert
  - `web/src/pages/JobsPage.tsx` - Tab-Interface mit Packliste
  - `web/src/lib/api.ts` - JobRequirement Interface + getRequirements()

### Version 1.42 (2025-10-18)
- **Bug Fix: Device Tree Database Schema** 🔧
  - Fixed column name from serial_number to serialnumber
  - Fixed table names to lowercase (subcategories, subbiercategories)
  - Device tree API now working

### Version 1.41 (2025-10-18)
- **Feature: Device Tree Selector in Zone Details** 📦
  - Multi-select hierarchical device browser
  - Category → Subcategory → Subbiercategory → Device navigation
  - Modal dialog with "Add Devices" button
  - Batch assignment to zones
  - Shows device status and current zone
  - Disables devices already in current zone
- **API Endpoints:**
  - GET /api/v1/devices/tree - Hierarchical device tree
  - POST /api/v1/zones/{id}/devices - Batch device assignment
- **Components:**
  - DeviceTreeModal.tsx (new)
  - Updated ZoneDetailPage.tsx

### Version 1.40 (2025-10-18)
- **Bug Fix: Cross-Navigation to RentalCore** 🔗
  - Fixed hostname detection from storage. to warehouse.
  - Enhanced fallback logic for reverse proxy scenarios
  - RentalCore button now correctly redirects to RentalCore
- **Implementation:**
  - Updated Layout.tsx hostname check
  - Supports warehouse.* subdomain pattern

### Version 1.35 (2025-10-18)
- **Frontend Update: LED Control Button Now Visible** 💡
  - Rebuilt frontend with LED control button in Jobs page
  - LED toggle button appears when job is selected
  - Shows "Fächer hervorheben" / "Fächer hervorgehoben"
  - Visual indicators for MQTT connection status
  - Green/gray styling based on LED state
  - Button includes lightbulb icons for clarity
  - Shows total bins available in LED system
  - Auto-clears LEDs when leaving job view
- **User Interface:**
  - Button located above scan interface in job detail view
  - One-click toggle to turn LEDs on/off
  - Real-time feedback when LED state changes
  - Connection status: "MQTT verbunden" or "Dry-Run Modus"
  - No page reload needed - instant LED control

### Version 1.34 (2025-10-18)
- **Feature: Red/Green Bin Highlighting** 🔴🟢
  - ALL bins in the rack now illuminate when job is selected
  - Job bins (with devices to pack) → **GREEN** (solid)
  - Empty bins (no job devices) → **RED** (solid)
  - Clear visual distinction for warehouse workers
  - Example: Job 1024 shows 1 green bin + 4 red bins
- **Bug Fix: MQTT Topic Configuration**
  - Fixed environment variable names in mqtt_publisher.go
  - Changed from `WAREHOUSE_ID` → `LED_WAREHOUSE_ID`
  - Changed from `LED_TOPIC_PREFIX` → `LED_MQTT_TOPIC_PREFIX`
  - Clear command now sends to correct topic: `weidelbach/WDL/cmd`
  - Previously sent to wrong topic: `weidelbach/weidelbach/cmd`
- **Enhanced Logging:**
  - Now shows breakdown: "5 bins total (1 green job bins, 4 red empty bins)"
  - Easier to verify correct operation in logs
- **User Experience:**
  - Workers can now see the entire rack at a glance
  - Green bins = pick these devices
  - Red bins = skip these (empty or other jobs)
  - No confusion about which bins to check

### Version 1.33 (2025-10-18)
- **LED Pattern Change: Solid Green for Job Bins** 🟢
  - Changed LED pattern from "breathe" to "solid" for clearer visibility
  - Changed default color from red (#FF2A2A) to green (#00FF00)
  - Job bins now illuminate in solid green instead of breathing red
  - Increased intensity from 180 to 200 for brighter illumination
- **User Experience:**
  - No more pulsing/breathing effect - LEDs stay on constantly
  - Green color clearly indicates bins to pack (positive action)
  - Easier to spot which bins need attention in warehouse
  - More professional appearance with steady illumination

### Version 1.32 (2025-10-18)
- **Bug Fix: LED Zone Code Mapping and MQTT Configuration** 🔧
  - Fixed LED mapping zone codes to include complete hierarchical format
  - Updated bin_id values from `WDL-RG-02-F-XX` to `WDL-06-RG-02-F-XX` format
  - Ensures exact match with storage_zones.code column in database
  - Both shelf_id and bin_id now use proper hierarchical zone codes
- **Port Configuration Fix:**
  - Changed WarehouseCore port from 8081 to 8082 to avoid conflict with RentalCore
  - Updated docker-compose.yml port mapping: `8082:8082`
  - Updated healthcheck endpoint to use port 8082
  - RentalCore uses 8081, WarehouseCore uses 8082
- **MQTT Configuration Correction:**
  - Fixed LED_MQTT_HOST environment variable format
  - Changed from full URL (`tcp://mosquitto:1883`) to hostname only (`mosquitto`)
  - Added LED_MQTT_PORT as separate variable (1883)
  - Code expects hostname and builds URL internally
- **Docker Image Updates:**
  - Rebuilt with corrected LED mapping configuration
  - LED config files properly copied to container at build time
  - Service initialization at startup working correctly
- **Testing Results:**
  - ✅ MQTT connection successful: `tcp://mosquitto:1883`
  - ✅ LED mapping loaded: 1 shelf, 5 bins
  - ✅ LED highlight command published successfully for Job 1024
  - ✅ Device MIX2001 correctly mapped to zone WDL-06-RG-02-F-01
  - ✅ MQTT message published to topic: `weidelbach/WDL/cmd`
- **Environment Variables:**
  - PORT changed from 8081 to 8082
  - Added LED_MQTT_HOST=mosquitto (hostname only)
  - Added LED_MQTT_PORT=1883
  - LED configuration now complete in .env file
- **Result:**
  - LED warehouse bin highlighting system fully operational
  - Zone codes match database exactly
  - No more "no bins to highlight" errors
  - MQTT communication working end-to-end

### Version 1.29 (2025-10-17)
- **LED Mapping Now Uses Zone Codes from Database** 🔗
  - `bin_id` in LED mapping file must now match `code` from `storage_zones` table
  - Example: `"bin_id": "WDL-RG-01-F-01"` matches zone code in database
  - `WAREHOUSE_ID` environment variable is now the main warehouse zone code (e.g., "WDL")
  - System automatically looks up device zones by code, not name
- **Improved Database Integration:**
  - Changed query from `z.name` to `z.code` in `getJobDeviceZones()`
  - Zone codes are unique and hierarchical (e.g., WDL-RG-01-F-01)
  - Direct mapping between database zones and LED pixels
  - No manual zone name matching required
- **Updated Configuration Examples:**
  - LED mapping file now shows real zone codes: "WDL-RG-01-F-01", "WDL-RG-02-F-01"
  - `WAREHOUSE_ID=WDL` instead of "weidelbach" in .env files
  - ESP32 secrets.h.template updated with zone code examples
  - Clear documentation on how bin_id must match database codes
- **How It Works:**
  1. Job is selected with devices
  2. WarehouseCore queries device's `zone_id` from database
  3. Gets the zone's `code` field (e.g., "WDL-RG-01-F-01")
  4. Matches code with `bin_id` in LED mapping configuration
  5. Sends corresponding pixel indices to ESP32
  6. LEDs light up for the correct bins
- **Documentation Updates:**
  - Added clear explanation of zone code requirement
  - Updated all examples to use database zone codes
  - Added workflow diagram showing database → mapping → LEDs
  - Commented environment variables with database reference
- **Benefits:**
  - ✅ Single source of truth (database zone codes)
  - ✅ No manual synchronization between zones and LEDs
  - ✅ Hierarchical zone codes work perfectly
  - ✅ Easy to debug (zone codes are visible in both systems)
  - ✅ Scalable to hundreds of bins

### Version 1.28 (2025-10-17)
- **MQTT Credentials via .env Only - No Scripts Required** 🎯
  - MQTT password file is now **automatically generated** from `.env` on container startup
  - No docker commands, no scripts, no manual password file creation needed
  - Just edit `LED_MQTT_USER` and `LED_MQTT_PASS` in `.env` and restart container
  - Password file regenerates automatically every time the container starts
- **Implementation:**
  - New `mosquitto/docker-entrypoint.sh` script for automatic password generation
  - Reads `MQTT_USER` and `MQTT_PASS` environment variables
  - Generates password file using `mosquitto_passwd` on container startup
  - Mounted as entrypoint in docker-compose.yml
  - Environment variables passed from .env to mosquitto container
- **Removed Complexity:**
  - Deleted `mosquitto/setup-mqtt.sh` (no longer needed)
  - Deleted pre-configured `mosquitto/config/passwords` (auto-generated now)
  - No need for manual docker commands to create passwords
  - Single source of truth: `.env` file
- **User Experience:**
  - Change password: Edit `.env` → Restart container → Done!
  - Same credentials in WarehouseCore and Mosquitto (both read from .env)
  - ESP32 uses same password (manual sync required for security)
  - Perfect for both development and production
- **Documentation:**
  - Updated README with simplified credential management
  - Updated mosquitto/README.md to emphasize .env-based config
  - Removed references to setup scripts and manual password creation
  - Clear instructions for changing passwords (just edit .env)
- **Developer Benefits:**
  - ✅ Single configuration file (.env) for everything
  - ✅ No need to learn mosquitto_passwd commands
  - ✅ No risk of password file/env mismatch
  - ✅ Password changes are instant (restart container)
  - ✅ Version control friendly (passwords auto-generated, not committed)

### Version 1.27 (2025-10-17)
- **Simplified MQTT Setup - Zero Configuration Required** 🚀
  - Pre-configured password file with demo credentials (leduser/ledpassword123)
  - No setup script required - just `docker-compose up -d` and it works
  - Password file now committed to repository for instant deployment
  - Default credentials in .env.example match mosquitto configuration
  - ESP32 secrets.h.template updated with matching credentials
  - Setup reduced from 4 steps to 3 steps (copy .env, edit DB credentials, start)
- **Documentation Updates:**
  - Simplified Quick Setup instructions in README
  - mosquitto/README.md now emphasizes zero-config approach
  - Custom password setup moved to "Optional" section
  - Clear instructions for both demo and production use
- **Developer Experience:**
  - ✅ Clone repo → copy .env.example → docker-compose up → Done!
  - ✅ No manual password creation needed
  - ✅ Perfect for development and testing
  - ✅ Production users can easily change credentials with documented steps
- **Security Notes:**
  - Demo credentials clearly marked in all files
  - .gitignore updated to allow committing demo password file
  - Production deployment guide includes password change instructions
  - Optional setup script remains available for custom passwords

### Version 1.26 (2025-10-17)
- **Feature: Self-Hosted MQTT Broker with Docker Compose** 🐳
  - Added Mosquitto MQTT broker container to docker-compose.yml
  - WarehouseCore and ESP32 can now connect to the same server
  - No need for external cloud MQTT broker for single-server deployments
  - Eliminates subscription fees and external dependencies
  - Lower latency with local network communication
- **Mosquitto Configuration:**
  - Pre-configured mosquitto.conf with authentication, logging, persistence
  - Support for plain MQTT (1883), TLS (8883), and WebSocket (9001)
  - Password-based authentication required (no anonymous access)
  - Health checks for container monitoring
  - Volume mounts for config, data, and log persistence
- **Setup Automation:**
  - New setup script: `mosquitto/setup-mqtt.sh`
  - Interactive password creation for MQTT users
  - Automatic directory structure creation
  - Permission management
  - Clear step-by-step instructions
- **Documentation:**
  - Comprehensive `mosquitto/README.md` with setup guide
  - TLS setup instructions with Let's Encrypt integration
  - Firewall configuration guide
  - Monitoring and troubleshooting sections
  - Security best practices
  - ACL configuration examples
  - Backup recommendations
- **Docker Compose Updates:**
  - Added `mosquitto` service with eclipse-mosquitto:2.0 image
  - WarehouseCore now depends_on mosquitto service
  - Exposed ports: 1883 (MQTT), 8883 (TLS), 9001 (WebSocket)
  - Volume mounts for persistent configuration and data
  - Health check with mosquitto_sub test
- **Environment Configuration:**
  - Updated .env.example with two MQTT options
  - Option 1: Self-hosted (mosquitto container)
  - Option 2: Cloud broker (EMQX, HiveMQ, etc.)
  - Clear documentation for each option
  - Default configuration uses self-hosted option
- **Git Ignore Updates:**
  - Added mosquitto/data/ to prevent committing MQTT persistence
  - Added mosquitto/log/ to exclude log files
  - Added mosquitto/config/passwords to protect credentials
  - Added mosquitto/certs/ for TLS certificate protection
- **Deployment Options:**
  - Simple deployment: Just run `docker-compose up -d`
  - No external account or subscription needed
  - Works offline (no internet required after container pull)
  - Scales from development to production with TLS
- **ESP32 Configuration:**
  - Can connect to server's public IP or domain
  - Same credentials as WarehouseCore
  - Works from warehouse WiFi to server
  - No port forwarding required (outbound connection)
- **Security Features:**
  - Password file authentication
  - Optional TLS with Let's Encrypt certificates
  - ACL support for topic-level permissions
  - No anonymous access allowed
  - Secure password hashing
- **Production Ready:**
  - Persistent data storage
  - Log rotation support
  - Container health monitoring
  - Auto-restart on failure
  - Compatible with existing LED system
- **User Benefits:**
  - ✅ Zero cost (no subscription)
  - ✅ Full control over MQTT broker
  - ✅ Faster communication (local network)
  - ✅ Works without internet
  - ✅ Single-server deployment
  - ✅ Easy ESP32 configuration (just use server address)
  - ✅ Professional monitoring with system topics

### Version 1.25 (2025-10-17)
- **Feature: LED Warehouse Bin Highlighting System** 🎨
  - Physical warehouse LED highlighting for job-based device picking
  - Real-time MQTT communication with ESP32 controllers
  - Works globally with cloud-based MQTT broker (no port forwarding needed)
- **Backend LED Module:**
  - New `internal/led/` package with models, MQTT publisher, service layer
  - MQTT client with TLS support, auto-reconnect, heartbeat
  - Dry-run mode for testing without MQTT broker
  - Job-to-bins mapping algorithm using device zone assignments
  - REST API endpoints: status, highlight, clear, identify, test, mapping
  - JSON schemas for LED commands and mapping configuration
  - Validation endpoint for mapping configuration
- **Frontend LED Controls:**
  - Toggle button "Fächer hervorheben" in Jobs page
  - Visual indicators: MQTT connection status, active bin count
  - Auto-clear LEDs when navigating away from job
  - Real-time status updates with LED state
  - Lightbulb icon with toggle states (on/off)
- **ESP32 Firmware:**
  - Complete Arduino sketch for ESP32 + SK6812 GRBW LED strips
  - WiFi connectivity with auto-reconnect
  - MQTT client with TLS support (optional)
  - JSON command parsing with ArduinoJson
  - Multiple LEDs per bin support (e.g., 4 LEDs per compartment)
  - Animation patterns: solid, blink, breathe (sine wave)
  - Watchdog timer for robustness
  - Heartbeat status publishing every 15 seconds
  - Last Will Testament for offline detection
  - Firmware location: `firmware/esp32_sk6812_leds/`
- **Configuration:**
  - LED mapping file: `internal/led/config/led_mapping.json`
  - Maps storage bins to LED pixel indices
  - Supports multiple LEDs per bin
  - Configurable defaults (color, pattern, intensity)
  - LED strip parameters (length, data pin, chipset)
  - Example mapping with 3 shelves, 12 bins
- **MQTT Architecture:**
  - Command topic: `{prefix}/{warehouse_id}/cmd`
  - Status topic: `{prefix}/{warehouse_id}/status`
  - JSON-based commands: highlight, clear, identify
  - ESP32 subscribes to commands, publishes heartbeat
  - Cloud broker recommended (EMQX, HiveMQ, Mosquitto)
- **Hardware Support:**
  - SK6812 GRBW addressable LED strips
  - ESP32 development boards (DevKitC, WROOM-32)
  - Level shifter for data line (3.3V → 5V)
  - Detailed wiring diagrams and power calculations
- **Documentation:**
  - Complete LED system documentation in README
  - ESP32 firmware README with flash instructions
  - Hardware wiring diagrams
  - MQTT broker setup guides (EMQX, Mosquitto, HiveMQ)
  - Troubleshooting section
  - Security considerations (TLS, passwords, secrets)
- **Dependencies:**
  - Go: `github.com/eclipse/paho.mqtt.golang` v1.5.1
  - Arduino: PubSubClient, ArduinoJson, Adafruit_NeoPixel
- **Environment Variables:**
  - `LED_MQTT_HOST` - MQTT broker hostname (empty = dry-run)
  - `LED_MQTT_PORT` - Broker port (1883 or 8883)
  - `LED_MQTT_TLS` - Enable TLS (true/false)
  - `LED_MQTT_USER` - MQTT username
  - `LED_MQTT_PASS` - MQTT password
  - `LED_TOPIC_PREFIX` - Topic namespace
  - `WAREHOUSE_ID` - Warehouse identifier
  - `LED_PREVIEW_BIN_ID` - Optional bin code for Admin-Preview (z. B. `WDL-06-RG-02-F-01`)
- **Testing:**
  - Dry-run mode logs commands without sending
  - Status endpoint shows connection state
  - Test endpoints for individual bins
  - Mapping validation endpoint
  - Build verification successful
- **Use Cases:**
  - Visual guidance for warehouse workers during job packing
  - Reduce picking errors by highlighting exact locations
  - Speed up device collection process
  - Real-time verification of picked items
  - Support for large warehouses with hundreds of bins
- **Performance:**
  - MQTT latency < 100ms with cloud broker
  - LED update rate: 50-100 Hz
  - Supports up to ~2000 LEDs per ESP32
  - Automatic reconnection with 5s retry
- **Security:**
  - TLS encryption for production (port 8883)
  - Strong password enforcement
  - Secrets templates prevent credential leaks
  - Namespaced MQTT topics by warehouse_id

### Version 1.18 (2025-10-15)
- **Feature: User Authentication and Single Sign-On (SSO)**
  - Complete user authentication system integrated with RentalCore
  - Shared session-based authentication using PostgreSQL sessions table
  - Cookie-based SSO across both applications (.server-nt.de domain)
  - Login required to access WarehouseCore directly
  - Automatic authentication when navigating from RentalCore
- **Backend Authentication:**
  - Added GORM ORM for auth model management (User, Session)
  - New auth middleware validating session cookies
  - Auth API endpoints: POST /auth/login, POST /auth/logout, GET /auth/me
  - Shared cookie domain auto-detection for production and localhost
  - Session validation with expiration checking
  - User data loaded from shared users table
- **Frontend Authentication:**
  - New Login page with username/password form
  - AuthContext for global authentication state management
  - ProtectedRoute component guarding all main routes
  - User profile display in sidebar with username and email
  - Logout button with confirmation
  - Loading states during authentication checks
  - TypeScript interfaces for User and auth API
- **Security:**
  - HttpOnly session cookies
  - SameSite Lax mode for CSRF protection
  - Password validation against bcrypt hashes
  - Automatic redirect to login for unauthenticated users
  - Session expiration handling
- **Database Integration:**
  - Uses existing RentalCore tables: users, sessions
  - No new tables required
  - Maintains backward compatibility with RentalCore auth
- **User Experience:**
  - Seamless navigation between RentalCore and WarehouseCore when logged in
  - Single login for both applications
  - Professional login page matching WarehouseCore design
  - Clear authentication feedback
  - Automatic session check on application load
- **Technical Changes:**
  - Updated Go version to 1.24 for GORM compatibility
  - Added GetDB() for GORM, GetSQLDB() for raw SQL queries
  - All existing handlers updated to use GetSQLDB()
  - Auth service layer with session management
  - Protected routes middleware in main server setup

### Version 1.17 (2025-10-15)
- **Bug Fix: Mobile Scrolling and Button Overlap Issues**
  - Fixed horizontal scrolling on zone detail page
  - Fixed unnecessary vertical scrolling on scan page
  - Fixed button overlap issues on zone detail page
- **Zone Detail Page Improvements:**
  - Added `max-w-full overflow-x-hidden` to root container to prevent horizontal scroll
  - Changed button container to `flex flex-wrap items-center gap-2 sm:gap-3` for proper wrapping
  - Implemented responsive button text: Full text on desktop, icons only on mobile
  - Delete button: Shows "Löschen" on desktop, "🗑️" emoji on mobile
  - Shelf creation button: Shows "Fächer erstellen" on desktop, "Fächer" on mobile
  - Subzone creation button: Shows "Unterzone erstellen" on desktop, "Unterzone" on mobile
  - Added `whitespace-nowrap` to all buttons to prevent text wrapping within buttons
- **Breadcrumb Navigation:**
  - Added `overflow-x-auto` with horizontal scroll support
  - Breadcrumb items use `flex-shrink-0` to prevent crushing
  - Maintains proper spacing with `flex-wrap` fallback
- **Device Table Responsiveness:**
  - Table container with `overflow-x-auto` for horizontal scroll on small screens
  - Set minimum table width to 640px to maintain readability
  - Hidden columns on mobile for space efficiency:
    - Manufacturer column: `hidden md:table-cell`
    - Model column: `hidden md:table-cell`
    - Barcode column: `hidden lg:table-cell`
  - Responsive cell padding: `p-2 sm:p-4`
  - Responsive text sizes: `text-xs sm:text-sm` and `text-xs sm:text-base`
- **Scan Page Improvements:**
  - Removed unnecessary `min-h-[calc(100vh-4rem)]` that caused vertical scrolling
  - Changed to simple `flex items-center justify-center` for proper centering
  - Form now fits perfectly without any unnecessary scroll
- **Responsive Text and Spacing:**
  - All headers use responsive sizing: `text-xl sm:text-3xl`, `text-lg sm:text-xl`
  - Stats cards use adaptive padding: `p-3 sm:p-4`, `p-4 sm:p-6`
  - Icon sizes adapt: `w-4 h-4`, `w-4 h-4 sm:w-5 sm:h-5`
  - Gaps scale properly: `gap-2 sm:gap-3`, `gap-3 sm:gap-4`
- **User Experience Enhancements:**
  - No horizontal scrolling on any viewport width
  - No unnecessary vertical scrolling
  - All buttons stay visible and accessible on mobile
  - Buttons never overlap or extend beyond screen width
  - Table data remains accessible with horizontal scroll only when needed
  - Clean, professional mobile interface

### Version 1.16 (2025-10-15)
- **Feature: Complete Mobile Responsiveness**
  - Full mobile optimization for all pages and components
  - Professional responsive design with breakpoints at 640px (sm) and 768px (md)
- **Mobile Sidebar:**
  - Hamburger menu with overlay backdrop on mobile
  - Smooth slide-in/out animations
  - Auto-close sidebar on navigation for mobile
  - Desktop: Sidebar visible by default, persists on screen
  - Mobile: Sidebar hidden by default, overlay mode
  - Close button in sidebar header (mobile only)
  - Touch-friendly tap targets
- **Layout Optimizations:**
  - Header: Responsive padding and font sizes (px-3 → px-6, text-lg → text-2xl)
  - Company name hidden on mobile to save space
  - Main content padding adapts (p-3 → p-6)
  - Proper spacing adjustments (pt-14 → pt-16)
- **Dashboard Page:**
  - Stats grid: 1 column on mobile, 2 on sm, 4 on lg
  - Card padding and text sizes scale responsively
  - Icon sizes adjust (w-5 → w-6)
  - Activity feed items with truncation
- **Scan Page:**
  - Scanner icon and titles scale (w-8 → w-12, text-2xl → text-4xl)
  - Step indicator adapts sizing (w-7 → w-8)
  - Input fields with responsive padding
  - Action buttons grid adjusts (gap-2 → gap-4)
  - Result cards with proper text wrapping
- **Devices Page:**
  - Search bar with responsive icon and input sizing
  - Grid: 1 column → 2 columns (sm) → 3 columns (lg)
  - Device cards with flexible layouts
  - Status badges scale properly
  - Truncation for long text
- **Maintenance Page (All 3 Tabs):**
  - **Overview:** Stats in 2-column grid (2 on mobile, 5 on lg)
  - **Defects:** Mobile-friendly form with stacked inputs
  - **Inspections:** Responsive filter tabs with horizontal scroll
  - All cards adapt padding (p-3 → p-6)
  - Text sizes scale across breakpoints
  - Status buttons stack on mobile, inline on desktop
  - Form buttons stack vertically on mobile
- **Mobile-First Features:**
  - Touch-friendly button sizes (44x44px minimum)
  - Proper text wrapping and truncation
  - Overflow-x-auto for filter tabs
  - Flex-wrap for badges and tags
  - Min-w-0 to allow flex item shrinking
  - Line-clamp for multi-line text truncation
- **Responsive Typography:**
  - Headings: text-2xl sm:text-3xl or text-2xl sm:text-4xl
  - Body text: text-xs sm:text-sm or text-sm sm:text-base
  - Badges: text-[10px] sm:text-xs
  - Proper hierarchy maintained across breakpoints
- **Spacing System:**
  - Gaps: gap-2 sm:gap-4, gap-3 sm:gap-6
  - Padding: p-3 sm:p-6, p-4 sm:p-8
  - Margins: mb-1 sm:mb-2, mb-3 sm:mb-4
  - Consistent use of Tailwind spacing scale
- **User Experience:**
  - Smooth transitions for sidebar and modals
  - No horizontal scrolling on any page
  - All interactive elements easily tappable
  - Proper keyboard focus management
  - Maintains glassmorphism aesthetic on all devices
- **Testing Notes:**
  - Tested at breakpoints: 320px, 375px, 768px, 1024px, 1920px
  - Works on iOS Safari, Chrome Mobile, Android Chrome
  - Landscape and portrait orientations supported

### Version 1.15 (2025-10-14)
- **Feature: Complete Maintenance Module Implementation**
  - Full defect tracking and repair management system
  - Inspection scheduling and monitoring
  - Comprehensive maintenance dashboard with real-time statistics
- **Defect Management:**
  - Create defect reports with severity levels (low, medium, high, critical)
  - Status workflow: open → in_progress → repaired → closed
  - Automatic device status updates when defects are created/resolved
  - Inline defect creation form with device ID, severity, title, description
  - Filter defects by status: All, Open, In Progress, Repaired, Closed
  - Color-coded severity badges (critical=red, high=orange, medium=yellow, low=blue)
  - Track repair costs and repair notes
  - Automatic timestamp management (reported_at, repaired_at, closed_at)
  - Status update buttons for workflow progression
- **Inspection Management:**
  - View all inspection schedules with filtering
  - Filter by: All, Overdue, Upcoming (next 30 days)
  - Visual indicators for overdue inspections (red border, "ÜBERFÄLLIG" badge)
  - Display inspection type, interval days, last/next inspection dates
  - Support for both device-specific and product-wide inspections
  - Active/inactive inspection status
- **Maintenance Dashboard:**
  - Overview tab with 5 real-time statistics cards
  - Shows: Open defects, In Progress defects, Repaired defects, Overdue inspections, Upcoming inspections
  - Quick action cards to navigate to defects or inspections
  - Auto-refresh data on view changes
- **Backend API:**
  - `GET /defects` - List defects with filters (status, severity, device_id)
  - `POST /defects` - Create new defect report
  - `PUT /defects/{id}` - Update defect status, costs, notes
  - `GET /maintenance/inspections` - Get inspection schedules (filters: upcoming, overdue, all)
  - `GET /maintenance/stats` - Dashboard statistics
  - Dynamic SQL query building for flexible filtering
  - JOIN queries with devices and products tables for rich data
- **Frontend Features:**
  - New MaintenancePage with three-tab interface (Overview, Defects, Inspections)
  - Real-time data loading with automatic updates
  - Responsive card-based layout with glassmorphism design
  - Color-coded status and severity indicators
  - Inline forms for quick defect creation
  - Status progression buttons for efficient workflow
  - Clock icons with color coding (red=overdue, blue=upcoming)
- **API Types:**
  - New TypeScript interfaces: Defect, Inspection, MaintenanceStats
  - Added maintenanceApi with getStats(), getDefects(), createDefect(), updateDefect(), getInspections()
- **Navigation:**
  - New "Wartung" (Maintenance) menu item with Wrench icon
  - Route: /maintenance
- **Database Integration:**
  - Uses existing tables: defect_reports, inspection_schedules
  - Bi-directional device status updates (defective ↔ in_storage)
  - Full audit trail with timestamps
- **User Workflow:**
  - Defect Reporting:
    1. Navigate to Wartung → Defects tab
    2. Click "Neuer Defekt" to create report
    3. Enter device ID, select severity, add title and description
    4. Submit → Device automatically marked as defective
    5. Track repair progress with status updates
    6. Mark as repaired → Device automatically returns to in_storage
  - Inspection Management:
    1. Navigate to Wartung → Inspections tab
    2. View all scheduled inspections
    3. Filter by overdue or upcoming
    4. See visual indicators for priority
    5. Track last and next inspection dates
- **Use Cases:**
  - Track equipment defects from discovery through repair completion
  - Monitor repair costs and maintenance budgets
  - Ensure timely inspections with overdue alerts
  - Maintain compliance with inspection schedules
  - Full visibility into equipment condition and maintenance needs
  - Historical tracking of all repairs and defects

### Version 1.14 (2025-10-14)
- **Feature: Job-Based Outtake Workflow with Live Scan Tracking**
  - New Jobs page accessible via sidebar navigation (/jobs)
  - Displays all jobs with status "open" ready for device outtake
  - Select a job to start the outtake/packing process
  - Real-time live tracking of which devices have been scanned out
  - Visual progress bar showing scan completion (e.g., 5/20 devices scanned)
  - Two-column layout: Scan interface + Device checklist
  - Auto-refresh every 2 seconds to show live updates as devices are scanned
  - Device list shows checkmarks (✓) for scanned devices, X for remaining
  - Color-coded status: Green for scanned, gray for pending
  - Integration with job_devices table and pack_status tracking
- **Backend Enhancements:**
  - Completely implemented `GetJobs` handler with customer info and device counts
  - Enhanced `GetJobSummary` handler to return full job details with all devices
  - Device list includes current status, location, pack_status, and scan state
  - Scanned flag based on device status ('on_job') or pack_status ('issued')
  - Supports filtering jobs by status (e.g., ?status=open)
  - JOIN queries across jobs, status, customers, jobdevices, devices, products, storage_zones
- **Frontend Features:**
  - New JobsPage component with job selection and scan interface
  - Job cards show: Job ID, description, customer name, dates, device count
  - Job detail view with customer info, dates, and progress statistics
  - Live device checklist with product names, IDs, status, and scan indicators
  - Scan form integrated with job context (sends job_id to scan API)
  - Success/error feedback after each scan
  - "Back to Job List" navigation
  - Auto-refresh for live updates during scanning
- **API Types:**
  - New TypeScript interfaces: Job, JobDevice, JobSummary
  - Added jobsApi.getAll() and jobsApi.getById() functions
- **Navigation:**
  - New "Jobs" menu item with Briefcase icon
  - Route: /jobs
- **User Workflow:**
  1. Navigate to Jobs page
  2. View all open jobs with device counts
  3. Select a job to start packing/outtake
  4. Scan devices one by one
  5. Watch the progress bar and checklist update live
  6. See which devices are still missing at a glance
  7. Complete when all devices are scanned
- **Use Case:**
  - Perfect for warehouse workers preparing equipment for upcoming events
  - Second-level verification that all job devices are physically present
  - Real-time feedback prevents missing items before transport
  - Live updates allow multiple workers to see scan progress simultaneously

### Version 1.13 (2025-10-14)
- **Feature: Recursive Device Count for Parent Zones**
  - Zone device counts now include all devices in subzones recursively
  - When viewing a parent zone (e.g., Lager or Regal), device count shows total across all child zones
  - Implemented using PostgreSQL recursive CTE (Common Table Expression) for efficient querying
  - Example: Lager Weidelbach shows total of all devices in all Regale and all Fächer within
- **Backend Enhancements:**
  - New function `getDeviceCountRecursive` in ZoneService
  - Uses `WITH RECURSIVE` query to build complete zone tree
  - Counts all devices with status 'in_storage' across entire zone hierarchy
  - Updated `GetZoneDetails` to use recursive count
  - Updated `getSubzones` to show recursive counts for each subzone
- **User Benefits:**
  - Accurate inventory totals at every level of the hierarchy
  - No need to manually sum devices across subzones
  - Better overview of warehouse stock distribution
  - Reflects real warehouse organization where parent zones contain all child zone contents

### Version 1.12 (2025-10-14)
- **Feature: Two-Step Intake Workflow with Zone Selection**
  - Implemented multi-step barcode scanning for device intake (Einlagerung)
  - Step 1: Scan device barcode/QR code to verify device exists
  - Step 2: Scan storage location (zone) barcode to specify exact placement
  - Visual step indicator shows progress (Gerät → Lagerplatz)
  - Automatic zone lookup by barcode or zone code
- **Backend Enhancements:**
  - New endpoint: `GET /api/v1/zones/scan?scan_code={code}` - Find zone by barcode/code
  - Enhanced intake process to require and store zone_id
  - Zone barcode support for precise location tracking
- **Frontend Improvements:**
  - Smart UI that adapts based on current step (device vs. zone scan)
  - Different icons and placeholders for each step
  - Action buttons hidden during zone scan step
  - Seamless error handling with automatic step reset
- **User Experience:**
  - Clear visual feedback with step-by-step process
  - Ensures every device is assigned to a specific storage location
  - Prevents devices from being stored without location information
- **Example Workflow:**
  1. Select "Einlagern" action
  2. Scan device barcode → Device verified ✓
  3. Scan zone barcode (e.g., FACH-00000014) → Device stored at specific location
  4. Process complete with full audit trail

### Version 1.11 (2025-10-14)
- **Bug Fix: SPA Routing on Page Reload**
  - Fixed 404 errors when refreshing pages like `/zones`, `/zones/{id}`, etc.
  - Implemented custom `spaHandler` to serve `index.html` for all non-file routes
  - Server now properly handles client-side routing for React Router
  - Changed from `http.FileServer` to custom handler with file existence check
  - All frontend routes now work correctly on direct access or browser refresh

### Version 1.10 (2025-10-14)
- **Bug Fix: Zone Devices SQL Query**
  - Fixed 500 Internal Server Error on `/api/v1/zones/{id}/devices` endpoint
  - Updated query to properly join with `manufacturer` and `brands` tables
  - Changed from direct `p.manufacturer` and `p.model` columns to `m.name` and `b.name` via JOINs
  - Products table uses `manufacturerID` and `brandID` foreign keys, not direct string columns
- **Feature: Subzone Delete Buttons**
  - Added delete buttons to subzone cards in zone detail view
  - Buttons appear on hover, similar to main zones page
  - Click delete without navigating into the subzone
  - Prevents accidental navigation when deleting
  - Confirmation dialog before deletion

### Version 1.9 (2025-10-14)
- **Bug Fix: Race Condition in Bulk Shelf Creation**
  - Fixed duplicate entry errors when creating multiple shelves at once
  - Changed from parallel (Promise.all) to sequential creation
  - Prevents multiple shelves from reading the same count and generating duplicate codes
  - Example: Creating 5 shelves now correctly generates Fach 01-05 instead of all trying to create Fach 01
- **Improved Error Handling:**
  - Eliminated 500 Internal Server errors during bulk operations
  - Consistent shelf numbering without gaps

### Version 1.8 (2025-10-14)
- **Automatic Shelf (Fach) Creation:**
  - Added 📚 **Fach** (shelf) as a 4th zone type for Regalen
  - Automatic name generation (Fach 01, Fach 02, etc.) based on existing shelves in parent rack
  - Automatic barcode generation (FACH-%08d format) after creation
  - No manual name input required for shelf creation
- **Bulk Shelf Creation:**
  - "Fächer erstellen" button in rack detail view
  - Create multiple shelves at once with a single action
  - Default capacity of 10 per shelf
- **Navigation Improvements:**
  - Fixed subzone creation to stay in parent zone context instead of redirecting to /zones
  - Maintains workflow continuity when creating hierarchical structures
- **Database Migration:**
  - Added barcode column to storage_zones table (006_add_zone_barcode.sql)
  - Index on barcode for efficient lookups
- **Backend Enhancements:**
  - GenerateShelfName function in ZoneService for automatic naming
  - Updated CreateZone handler to support optional Name field for shelves
  - Barcode generation logic after zone insertion
- **Example Usage:**
  - Create Lager Weidelbach (WDB)
  - Create Regal A (WDB-RG-01)
  - Click "Fächer erstellen" → Enter "5" → Creates Fach 01 through Fach 05 automatically
  - Each Fach gets codes like WDB-RG-01-F-01 and barcodes like FACH-00000014

### Version 1.7 (2025-10-14)
- **Simplified Zone Types:** Reduced to 3 types only
  - 🏭 **Lager** (warehouse) - Main storage facility
  - 🗄️ **Regal** (rack) - Shelving units
  - 📦 **Gitterbox** (gitterbox) - Wire mesh containers
- **Delete Functionality:**
  - Delete button on zone cards (hover to reveal)
  - Delete button in zone detail view
  - Safety checks: prevents deletion if zone contains devices or subzones
  - Confirmation dialog before deletion
- **Updated Code Generation:**
  - Warehouse prefix: LGR (Lager)
  - Rack prefix: RG (Regal)
  - Gitterbox prefix: GB (Gitterbox)
- **Example Hierarchy:**
  - Weidelbach (WDL) → Regal A (WDL-RG-01) → Gitterbox 01 (WDL-RG-01-GB-01)
- **Database Migration:** Updated ENUM type to support gitterbox

### Version 1.6 (2025-10-14)
- **Automatic Zone Code Generation:** Smart, hierarchical code generation
- **Hierarchical Zone System:** Create nested zones
- **Zone Detail View:** Click zones to see subzones, devices, and breadcrumb navigation
- **ZoneService:** New service layer for zone business logic
- **API Enhancements:**
  - `GET /zones/{id}` returns full details with subzones and breadcrumb
  - `GET /zones/{id}/devices` lists all devices in a zone
  - Optional `code` field in zone creation (auto-generated if not provided)
- **Frontend Improvements:**
  - New ZoneDetailPage component with subzone navigation
  - Parent zone selection in zone creation form
  - Breadcrumb navigation for zone hierarchy
  - Click-to-navigate zone cards

### Version 1.5 (2025-10-14)
- Fixed JSON API responses to return clean primitive types
- Removed nested sql.Null* structures from API responses
- Added ZoneResponse struct for consistent API output
- Fixed React rendering error on zones page
- Updated TypeScript interfaces for nullable fields

### Version 1.4 (2025-10-14)
- Fixed zone creation API endpoint
- Improved JSON request handling with proper nullable field support
- Added detailed logging for zone creation operations

### Version 1.3 (2025-10-14)
- Fixed API URL configuration for multi-platform deployment
- Changed from absolute to relative URLs for cross-platform compatibility

### Version 1.2 (2025-10-14)
- Frontend glassmorphism enhancements
- Improved device and zone display components

### Version 1.1 (2025-10-14)
- Fixed TailwindCSS v4 configuration
- Updated PostCSS plugins for modern build system

### Version 1.0 (2025-10-14)
- Initial release with core warehouse management features
- Scan-driven workflows implementation
- Modern React + TypeScript frontend with glassmorphism design
