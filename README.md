# WarehouseCore

**Lagerverwaltung und Inventarmanagement im Cores-Ökosystem — Geräte-Tracking, Zonenverwaltung, LED-Bin-Highlighting, Etikettendruck und Barcode-Scanning.**

---

## Features

- **Geräteverwaltung** — Vollständiges Inventory-Tracking mit Hierarchiebaum, Statusverfolgung, Bewegungsprotokoll und Defekterfassung
- **Sichere Produktstammdaten** — Explizite Produkttypen und Trackingarten, serverseitige Plausibilitätsprüfungen, revisionssichere Archivierung statt kaskadierendem Löschen und Audit-Protokoll für Stammdatenänderungen
- **Konsistente Mengenbestände** — Lagerzonen sind die führende Bestandsquelle; Produktsummen werden automatisch aus `product_locations` synchronisiert und verteilte Bestände ausschließlich über Lager- und Scanabläufe korrigiert
- **Zonenmanagement** — Physische Lagerzonen mit Barcode-Kennzeichnung, Gerätezuweisung, Produktbestand und Lagerplatz-Optimierung
- **LED-Bin-Highlighting** — Echtzeit-Steuerung von LED-Streifen via MQTT zur visuellen Hervorhebung von Pick-Positionen. Unterstützt Selbsthosting (Mosquitto) und Cloud-Broker
- **Label Studio & Direktdruck** — Visueller Designer für Geräte-, Kabel-, Case- und Zonenlabels mit direktem Verschieben/Skalieren über Ziehpunkte und einpassbarer 25–200-%-Vorschau. Dauerhaft gespeicherte PDF-Master, schneller PDF-Download/Browserdruck aus dem Cache und protokollierter Zebra-ZPL-Direktdruck über TCP
- **Geführtes Barcode-Scanning** — Klare Abläufe „Job → Artikel“ für Ausgaben und „Artikel → Lagerplatz“ für Einlagerungen, ein echtes Mengenfeld für Zubehör/Verbrauchsmaterial sowie nachvollziehbare Bewegungs- und Scanprotokolle
- **Eindeutige Gerätestatus** — `on_job` bezeichnet nur aktive Ausgaben. Nach Jobabschluss bleibt ein nicht eingebuchtes Gerät als „Rückgabe offen“ sichtbar; alte Datensätze ohne Job oder Lagerplatz werden als „Standort ungeklärt“ ausgewiesen.
- **Hybrides Kabelinventar** — Kabel als normale Produkte mit strukturierten Anschlüssen, Länge und Querschnitt verwalten. Wahlweise gemeinsamer Artikelbarcode mit Mengenbestand je Lagerzone oder individueller Barcode je physischem Kabel
- **Job-Picklisten** — Automatische Picklist-Generierung für Mietaufträge mit Scan-Bestätigung und Abschluss-Workflow
- **Case-Management** — Transportkisten-Verwaltung mit Inhaltsverfolgung, QR/Barcode-Labels und Gerätezuweisung
- **Wartungsmanagement** — Defekt-Tracking, Inspektionshistorie und Wartungsstatistiken
- **Öffentliche Produktseite** — Ungeschützte API für getrennte Produkt- und Paketlisten, jeweils mit Website-Freigabe und Bildern
- **Produktpakete** — Pakete unabhängig von normalen Produkten verwalten, Produkte mit Mengen zuweisen und eigene Paketbilder pflegen
- **Role-Based Access** — Feingranulares Rollensystem mit Admin-Bereich für Benutzer-, Kategorie- und LED-Konfiguration
- **Installierbare Mobile-App (PWA)** — Standalone-Modus mit WarehouseCore-App-Icon, Safe-Area-Unterstützung, großen Touch-Zielen, App-Tabbar und Drawer-Navigation; auf iPhone/iPad über Safari → Teilen → „Zum Home-Bildschirm“ installieren
- **Zentrales Branding** — Live geladene Varianten für Bildmarke, Sidebar, Login, Browser-Tab und dynamisches PWA-Manifest über `/api/v1/branding`

---

## Tech-Stack

| Schicht         | Technologie                                             |
|-----------------|---------------------------------------------------------|
| Backend         | Go 1.24, gorilla/mux, GORM, PostgreSQL 16               |
| Frontend        | React 19, TypeScript, Vite 7, Zustand 5, i18next        |
| Styling         | Tailwind CSS 4, PostCSS, Tailwind Merge                 |
| Auth            | JWT (golang-jwt/jwt/v5), bcrypt                         |
| MQTT            | Eclipse Paho MQTT (eclipse/paho.mqtt.golang)            |
| Barcodes/Labels | boombuler/barcode, skip2/go-qrcode, Chromium headless   |
| Bildverarbeitung| chai2010/webp, disintegration/imaging                   |
| Container       | Docker (Multi-Stage: Node 20 + Go 1.24 + Alpine + Chromium) |

---

## Schnellstart

### Docker

```bash
docker run -d \
  --name warehousecore \
  -e DB_HOST=postgres \
  -e DB_USER=warehouse_user \
  -e DB_PASS=*** \
  -e DB_NAME=rentalcore \
  -e DB_PORT=5432 \
  -e SESSION_SECRET=your-3...here \
  -e LED_MQTT_HOST=mosquitto \
  -e LED_MQTT_USER=leduser \
  -e LED_MQTT_PASS=ledpassword123 \
  -e WAREHOUSE_ID=MAIN \
  -p 8081:8081 \
  nobentie/warehousecore:latest
```

### docker-compose (Auszug)

```yaml
warehousecore:
  image: nobentie/warehousecore:latest
  ports:
    - "8082:8081"
  environment:
    DB_HOST: postgres
    DB_USER: warehouse_user
    DB_PASS: ${DB_PASS}
    DB_NAME: rentalcore
    DB_PORT: 5432
    SESSION_SECRET: ${SESSION_SECRET}
    CORES_JWT_SECRET: ${CORES_JWT_SECRET}
    LED_MQTT_HOST: mosquitto
    LED_MQTT_USER: leduser
    LED_MQTT_PASS: ledpassword123
    WAREHOUSE_ID: MAIN
    APP_ENV: production
  depends_on:
    - postgres
    - mosquitto
  volumes:
    - warehouse_uploads:/app/uploads
```

---

## API-Endpunkte

### Auth & Health

| Methode | Pfad                          | Beschreibung                              |
|---------|-------------------------------|-------------------------------------------|
| `POST`  | `/api/v1/auth/login`          | Benutzer-Login                            |
| `POST`  | `/api/v1/auth/logout`         | Session beenden                           |
| `GET`   | `/api/v1/auth/me`             | Aktuellen Benutzer abrufen (🔒)            |
| `POST`  | `/api/v1/auth/change-password`| Passwort ändern (🔒)                      |
| `GET`   | `/api/v1/health`              | Health Check (öffentlich)                  |

### Geräte & Scans

| Methode | Pfad                                    | Beschreibung                              |
|---------|-----------------------------------------|-------------------------------------------|
| `GET`   | `/api/v1/devices`                       | Alle Geräte auflisten (🔒)                |
| `GET`   | `/api/v1/devices/tree`                  | Geräte-Hierarchiebaum (🔒)                |
| `GET`   | `/api/v1/devices/:id`                   | Gerätedetails (🔒)                        |
| `PUT`   | `/api/v1/devices/:id/status`            | Gerätestatus aktualisieren (🔒)           |
| `GET`   | `/api/v1/devices/:id/movements`         | Bewegungsprotokoll (🔒)                   |
| `POST`  | `/api/v1/scans`                         | Gerät scannen (🔒)                        |
| `GET`   | `/api/v1/scans/history`                 | Scan-Historie (🔒)                        |

`POST /api/v1/scans` akzeptiert `scan_code`, `action`, optional `job_id`, `zone_id` und `quantity`. `job_id` bezeichnet ausschließlich den echten Zieljob; Mengen werden nicht mehr über dieses Feld transportiert. Eine Ausgabe benötigt einen offenen Job, eine Einlagerung einen bestätigten Lagerplatz.

### Zonen

| Methode  | Pfad                              | Beschreibung                              |
|----------|-----------------------------------|-------------------------------------------|
| `GET`    | `/api/v1/zones`                   | Alle Zonen auflisten (🔒)                 |
| `POST`   | `/api/v1/zones`                   | Zone erstellen (🔒 Admin)                 |
| `GET`    | `/api/v1/zones/scan`              | Zone per Barcode finden (🔒)              |
| `GET`    | `/api/v1/zones/:id`               | Zonendetails (🔒)                         |
| `PUT`    | `/api/v1/zones/:id`               | Zone aktualisieren (🔒 Admin)             |
| `DELETE` | `/api/v1/zones/:id`               | Zone löschen (🔒 Admin)                   |
| `GET`    | `/api/v1/zones/:id/devices`       | Geräte in Zone (🔒)                       |
| `POST`   | `/api/v1/zones/:id/devices`       | Geräte zu Zone zuweisen (🔒 Admin)        |
| `GET`    | `/api/v1/zones/:id/products`      | Produkte in Zone (🔒)                     |

### Produkte

| Methode  | Pfad                                  | Beschreibung                                      |
|----------|---------------------------------------|---------------------------------------------------|
| `GET`    | `/api/v1/admin/products`              | Aktive Produkte auflisten; Statusfilter möglich (🔒) |
| `POST`   | `/api/v1/admin/products`              | Typisiertes Produkt erstellen (🔒 Admin)          |
| `PUT`    | `/api/v1/admin/products/:id`          | Produktstammdaten aktualisieren (🔒 Admin)         |
| `DELETE` | `/api/v1/admin/products/:id`          | Produkt sicher archivieren (🔒 Admin)              |
| `PUT`    | `/api/v1/admin/products/:id/restore`  | Archiviertes Produkt wiederherstellen (🔒 Admin)  |

`GET /api/v1/admin/products` akzeptiert `lifecycle_status=active|archived|all`. Mengenbestände werden aus `product_locations` berechnet; bei auf Lagerzonen verteiltem Bestand erfolgen Korrekturen über Zonen- oder Scanabläufe.

### Produktpakete

| Methode  | Pfad                                                        | Beschreibung                         |
|----------|-------------------------------------------------------------|--------------------------------------|
| `GET`    | `/api/v1/admin/product-packages`                            | Produktpakete auflisten (🔒)         |
| `POST`   | `/api/v1/admin/product-packages`                            | Produktpaket erstellen (🔒 Admin)    |
| `GET`    | `/api/v1/admin/product-packages/:id`                        | Paketdetails und Positionen (🔒)     |
| `PUT`    | `/api/v1/admin/product-packages/:id`                        | Paket und Mengen aktualisieren (🔒)  |
| `POST`   | `/api/v1/admin/product-packages/:id/pictures`               | Paketbilder hochladen (🔒 Admin)     |
| `PUT`    | `/api/v1/admin/product-packages/:id/website`                | Website-Freigabe/Bilder setzen (🔒)  |
| `GET`    | `/api/v1/public/packages`                                   | Sichtbare Pakete öffentlich abrufen  |

### Kabelinventar

| Methode  | Pfad                                                   | Beschreibung                                      |
|----------|--------------------------------------------------------|---------------------------------------------------|
| `GET`    | `/api/v1/admin/cables`                                 | Kabelprodukte und Bestand auflisten (🔒 Admin/Manager) |
| `POST`   | `/api/v1/admin/cables`                                 | Kabelprodukt mit Trackingart anlegen (🔒 Admin)   |
| `GET`    | `/api/v1/admin/cables/:id`                             | Zonenbestand oder einzelne Exemplare (🔒 Admin/Manager) |
| `PUT`    | `/api/v1/admin/cables/:id`                             | Kabelspezifikation aktualisieren (🔒 Admin)        |
| `PUT`    | `/api/v1/admin/cables/:id/stock`                       | Mengenbestand einer Lagerzone setzen (🔒 Admin)   |
| `POST`   | `/api/v1/admin/cables/:id/units`                       | Einzelne Kabelexemplare erzeugen (🔒 Admin)        |
| `DELETE` | `/api/v1/admin/cables/:id/units/:device_id`            | Unbenutztes Kabelexemplar löschen (🔒 Admin)       |

Vorhandene Einträge aus der bisherigen `cables`-Tabelle werden beim ersten Start nach dem Update anhand von Kabeltyp, Anschlüssen, Länge und Querschnitt gruppiert. Die bisherigen Zeilen bleiben als unveränderte Legacy-Daten erhalten. Neue Kabel sind über `products` direkt für Jobs, Pakete, Lagerzonen und Scanabläufe verfügbar.

### Jobs, Cases & Labels

| Methode  | Pfad                                        | Beschreibung                              |
|----------|---------------------------------------------|-------------------------------------------|
| `GET`    | `/api/v1/jobs`                              | Job-Liste (🔒)                            |
| `GET`    | `/api/v1/jobs/:id`                          | Job-Zusammenfassung (🔒)                  |
| `GET`    | `/api/v1/jobs/:id/requirements`             | Job-Anforderungen (🔒)                    |
| `GET`    | `/api/v1/jobs/:id/picklist`                 | Pickliste (🔒)                            |
| `POST`   | `/api/v1/jobs/:id/picklist/scan`            | Gerät zur Pickliste scannen (🔒)          |
| `POST`   | `/api/v1/jobs/:id/complete`                 | Job abschließen (🔒)                      |
| `GET`    | `/api/v1/cases`                             | Kistenliste (🔒)                          |
| `POST`   | `/api/v1/cases`                             | Kiste erstellen (🔒)                      |
| `GET`    | `/api/v1/cases/:id`                         | Kistendetails (🔒)                        |
| `PUT`    | `/api/v1/cases/:id`                         | Kiste aktualisieren (🔒)                  |
| `DELETE` | `/api/v1/cases/:id`                         | Kiste löschen (🔒)                        |
| `GET`    | `/api/v1/cases/:id/contents`                | Kisteninhalt (🔒)                         |
| `POST`   | `/api/v1/cases/:id/devices`                 | Geräte in Kiste (🔒)                      |
| `DELETE` | `/api/v1/cases/:id/devices/:device_id`      | Gerät aus Kiste entfernen (🔒)            |

### LED-Steuerung

| Methode | Pfad                                | Beschreibung                              |
|---------|-------------------------------------|-------------------------------------------|
| `GET`   | `/api/v1/led/status`                | LED-Status abrufen (🔒)                   |
| `POST`  | `/api/v1/led/highlight`             | Job-Bins hervorheben (🔒)                 |
| `POST`  | `/api/v1/led/clear`                 | Alle LEDs löschen (🔒)                    |
| `POST`  | `/api/v1/led/identify`              | LEDs identifizieren (🔒)                  |
| `POST`  | `/api/v1/led/test`                  | Bin testen (🔒)                           |
| `POST`  | `/api/v1/led/locate`                | Bin orten (🔒)                            |

### Labels & Druck

| Methode  | Pfad                                    | Beschreibung                              |
|----------|-----------------------------------------|-------------------------------------------|
| `POST`   | `/api/v1/labels/qrcode`                 | QR-Code generieren (🔒)                   |
| `POST`   | `/api/v1/labels/barcode`                | Barcode generieren (🔒)                   |
| `GET`    | `/api/v1/labels/templates`              | Label-Templates auflisten (🔒)            |
| `POST`   | `/api/v1/labels/templates`              | Template erstellen (🔒)                   |
| `PUT`    | `/api/v1/labels/templates/:id`          | Template aktualisieren (🔒)               |
| `DELETE` | `/api/v1/labels/templates/:id`          | Template löschen (🔒)                     |
| `POST`   | `/api/v1/labels/device/:device_id`      | Geräte-Label generieren (🔒)              |
| `POST`   | `/api/v1/labels/case/:case_id`          | Kisten-Label generieren (🔒)              |
| `POST`   | `/api/v1/labels/save`                   | Geräte-Label speichern (🔒)               |
| `POST`   | `/api/v1/labels/save-case`              | Kisten-Label speichern (🔒)               |
| `GET`    | `/api/v1/labels/targets`                | Druckbare Ziele nach Typ auflisten (🔒)   |
| `GET`    | `/api/v1/labels/fields/:target_type`    | Datenfelder eines Labeltyps (🔒)          |
| `POST`   | `/api/v1/labels/render`                 | Label serverseitig rendern/speichern; das Druckcenter zeigt dabei Fortschritt und Ergebnis direkt an (🔒) |
| `POST`   | `/api/v1/labels/render-batch`           | Bis zu 250 Labels mit einem gemeinsamen Browserprozess rendern (🔒) |
| `POST`   | `/api/v1/labels/pdf`                    | Auswahl als maßhaltige Mehrseiten-PDF herunterladen (🔒) |
| `GET`    | `/api/v1/labels/printers`               | Druckerprofile auflisten (🔒)             |
| `POST`   | `/api/v1/labels/printers`               | Zebra-Netzwerkdrucker anlegen (🔒)        |
| `PUT`    | `/api/v1/labels/printers/:id`           | Druckerprofil aktualisieren (🔒)          |
| `DELETE` | `/api/v1/labels/printers/:id`           | Druckerprofil löschen (🔒)                |
| `POST`   | `/api/v1/labels/print`                  | Labels direkt als ZPL drucken (🔒)        |
| `GET`    | `/api/v1/labels/print-jobs`             | Druckaufträge und Fehler abrufen (🔒)     |

Das Label Studio verwendet für Geräte, Kabel, Cases und Lagerzonen jeweils getrennte Templates und ein eigenes Standardtemplate. Im Kabelbereich werden ausschließlich Datensätze aus `cable_products` angeboten; normale Produkte erscheinen dort nicht. Bei „Neu erzeugen“ wird pro Ziel ein maßhaltiges, einseitiges PDF als Master gespeichert und der vorherige Stand überschrieben; PNG-Dateien werden nicht gespeichert. Export, Browserdruck und ZPL-Direktdruck verwenden diesen Cache und rendern nur fehlende oder veraltete Labels neu. Ein Label gilt als veraltet, sobald sich sein Quelldatensatz, das gewählte Template oder dessen Revision ändert. Der PDF-Export führt die gespeicherten Master entsprechend Auswahl und Kopien zu einer Mehrseiten-PDF zusammen. Für Direktdruck wird im Studio ein aktiver Zebra-kompatibler Netzwerkdrucker mit IP/Hostname, TCP-Port (üblicherweise `9100`) und Auflösung (`203`, `300` oder `600` DPI) hinterlegt.

Die Migration `034_label_studio_and_direct_print.sql` ergänzt Templates um Zieltyp und Revision und legt `label_assets`, `label_printers` sowie `label_print_jobs` an. `035_cable_labels_and_pdf_export.sql` benennt das Standardtemplate konsistent für Kabel. `036_pdf_label_cache.sql` verwirft alte PNG-Cachepfade; die Dateien werden beim Start gezielt aus dem Label-Cache entfernt und bei Bedarf als PDF neu erzeugt. Die Migrationen werden beim WarehouseCore-Start idempotent angewendet.

Die Migration `038_device_status_lifecycle.sql` trennt Jobabschluss und physische Rückgabe. Ausgegebene Geräte abgeschlossener oder stornierter Jobs wechseln zu `return_pending`, bis ein Einlagerungsscan Lagerplatz und Rückgabe bestätigt. Nicht belegbare alte `on_job`-Werte werden je nach bekanntem Lagerkontext zu `in_storage` oder `location_unknown` normalisiert.

🔒 = Authentifizierung via `session_id` Cookie erforderlich

---

## Umgebungsvariablen

| Variable                | Beschreibung                                          | Standard               |
|-------------------------|-------------------------------------------------------|------------------------|
| `PORT`                  | Server-Port                                           | `8081`                 |
| `HOST`                  | Server-Host                                           | `0.0.0.0`              |
| `DB_HOST`               | PostgreSQL-Host                                       | `localhost`            |
| `DB_USER`               | Datenbank-Benutzer                                    | –                      |
| `DB_PASS`               | Datenbank-Passwort                                    | –                      |
| `DB_NAME`               | Datenbank-Name (Shared mit RentalCore)                | `rentalcore`           |
| `DB_PORT`               | Datenbank-Port                                        | `5432`                 |
| `APP_ENV`               | Umgebung (`development`/`production`)                 | `development`          |
| `LOG_LEVEL`             | Log-Level                                             | `info`                 |
| `CORS_ORIGIN`           | CORS-Origin                                           | `http://localhost:3000`|
| `SESSION_SECRET`        | Session-Secret                                        | –                      |
| `CORES_JWT_SECRET`      | JWT-Secret (Cores-weit identisch)                     | –                      |
| `ADMIN_NAME_MATCH`      | Auto-Admin bei Namensmatch                            | `Admin`                |
| `LED_MQTT_HOST`         | MQTT-Broker-Host                                      | `mosquitto`            |
| `LED_MQTT_PORT`         | MQTT-Broker-Port                                      | `1883`                 |
| `LED_MQTT_TLS`          | MQTT TLS aktivieren                                   | `false`                |
| `LED_MQTT_USER`         | MQTT-Benutzer                                         | `leduser`              |
| `LED_MQTT_PASS`         | MQTT-Passwort                                         | –                      |
| `LED_TOPIC_PREFIX`      | MQTT-Topic-Präfix                                     | `warehousecore`        |
| `WAREHOUSE_ID`          | Lagerzonen-Code (z. B. `MAIN`)                        | `MAIN`                 |
| `RENTALCORE_DOMAIN`     | RentalCore-Domain für Cross-Navigation                | –                      |
| `WAREHOUSECORE_DOMAIN`  | Eigene öffentliche Domain für Cross-Navigation        | –                      |

---

[Quellcode](https://github.com/nbt4/warehousecore) | [Monorepo](https://github.com/nbt4/cores) | `nobentie/warehousecore:latest`
