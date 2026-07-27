# WarehouseCore

**Lagerverwaltung und Inventarmanagement im Cores-Ökosystem — Geräte-Tracking, Zonenverwaltung, LED-Bin-Highlighting, Etikettendruck und Barcode-Scanning.**

---

## Features

- **Geräteverwaltung** — Vollständiges Inventory-Tracking mit Hierarchiebaum, Statusverfolgung, Bewegungsprotokoll und Defekterfassung
- **Zonenmanagement** — Physische Lagerzonen mit Barcode-Kennzeichnung, Gerätezuweisung, Produktbestand und Lagerplatz-Optimierung
- **LED-Bin-Highlighting** — Echtzeit-Steuerung von LED-Streifen via MQTT zur visuellen Hervorhebung von Pick-Positionen. Unterstützt Selbsthosting (Mosquitto) und Cloud-Broker
- **Etikettendruck** — Chromium-Headless-basiertes Rendering von Geräte- und Case-Labels als PNG. QR-Code- und Barcode-Generierung mit anpassbaren Templates
- **Barcode-Scanning** — Scan-Endpunkt für schnelle Geräteidentifikation und Warenbewegungen im Lager
- **Job-Picklisten** — Automatische Picklist-Generierung für Mietaufträge mit Scan-Bestätigung und Abschluss-Workflow
- **Case-Management** — Transportkisten-Verwaltung mit Inhaltsverfolgung, QR/Barcode-Labels und Gerätezuweisung
- **Wartungsmanagement** — Defekt-Tracking, Inspektionshistorie und Wartungsstatistiken
- **Öffentliche Produktseite** — Ungeschützte API für getrennte Produkt- und Paketlisten, jeweils mit Website-Freigabe und Bildern
- **Produktpakete** — Pakete unabhängig von normalen Produkten verwalten, Produkte mit Mengen zuweisen und eigene Paketbilder pflegen
- **Role-Based Access** — Feingranulares Rollensystem mit Admin-Bereich für Benutzer-, Kategorie- und LED-Konfiguration

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
