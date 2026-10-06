# AGENTS.md — warehousecore

Hausregeln für KI-Agenten in diesem Repository. Vor der ersten Änderung vollständig
lesen. Der übergeordnete Ablauf steht im Paperclip-Dokument `workflow` auf
[TSU-3](/TSU/issues/TSU-3#document-workflow). Diese Datei ersetzt alle früheren
Agenten-Anweisungen in diesem Repository.

## 1. Was dieses Repository ist

- **Zweck:** Lager und Inventar: Produkte, Geräte, Cases, Lagerorte und Zonen, physische Bewegungen, Scan-Ereignisse, Defekte, Wartung, Aufgaben, Inventuren, Etiketten und **LED-Wegeführung** im Lager. Port 8082, Abbild `nobentie/warehousecore`.
- **Sprache und Laufzeit:** Go 1.25 (Modul `warehousecore`) + React/TypeScript. Dazu Arduino-/ESP32-Firmware unter `firmware/`
- **Rahmenwerk:** `gorilla/mux` (nicht `gin`), GORM + `driver/postgres` **und** `lib/pq`, `eclipse/paho.mqtt.golang` (LED), `chromedp` (Headless Chrome), `xuri/excelize`, `pdfcpu`. Frontend: React + Vite + `i18next`
- **Datenbank:** PostgreSQL 16, gemeinsame Suite-Datenbank. Eigene Migrationsspur `migrations/001…062`, Stand in `warehouse_schema_migrations`
- **Architektur-Doku:** [Cores — Architektur (Phase 1)](/TSU/issues/TSU-4#document-architecture)

## 2. Aufbau

| Pfad | Inhalt |
|---|---|
| `cmd/server/main.go` | Einstiegspunkt (40 KB) |
| `cmd/debug_tools` | Hilfswerkzeuge |
| `internal/handlers` | HTTP und MCP-Schemata |
| `internal/repository`, `internal/models` | Daten |
| `internal/services` | Fachlogik, Etiketten, PDF |
| `internal/led` | MQTT-Publisher, Controller-Listener, Zonen-Konfiguration |
| `internal/validation`, `internal/middleware`, `internal/jobstatus`, `internal/metrics` | Querschnitt |
| `web/` | React-SPA mit `i18next` — das einzige Frontend mit Mehrsprachigkeit |
| `migrations/` | **63 Dateien, `001_…` bis `062_…`** |
| `firmware/esp32_sk6812_leds` | ESP32-Firmware für die LED-Streifen |
| `mosquitto/`, `schema/`, `config/`, `scripts/` | Betrieb |

Erzeugte Dateien, die **niemals von Hand** geändert werden:

- `web/src/cores-theme.css` — erzeugt durch `cores/scripts/sync-design-system.sh`
- `web/src/lib/cores-design.ts` — dito
- `web/src/lib/SuiteLanguageSwitcher.tsx` — dito
- `web/src/lib/cores-locales/` — dito
- `warehousecore` (gebaute Binärdatei im Wurzelverzeichnis), `warehousecore.db`,
  `tags.json` — eingecheckte Artefakte. Nicht benutzen, nicht aktualisieren; sie werden
  in einer eigenen Aufräum-Aufgabe entfernt.
- `web/package-lock.json` — nur als Nebenwirkung eines freigegebenen Updates

## 3. Einrichten

```bash
go mod download
cd web && npm ci && npm run build && cd ..
cp .env.example .env            # Werte lokal eintragen, niemals committen
# Datenbank und MQTT-Broker: aus dem Dachrepository `cores` starten
#   cd ../cores && docker compose up -d postgres mosquitto
```

Nötige Umgebungsvariablen: siehe `.env.example` hier und `cores/.env.example` als verbindliche Quelle. Besonders die `LED_MQTT_*`- und `NEXTCLOUD_WEBDAV_*`-Schlüssel. Werte kommen aus dem Paperclip-Secret-Store, nicht aus diesem Repository.

**`CGO_ENABLED=1`** — lokale Builds brauchen `gcc`/`musl-dev`. Der Produktseiten-Import braucht Headless Chrome.

## 4. Test- und Build-Befehle

Diese Befehle sind das Test-Gate. **Alle müssen grün sein, bevor ein Pull Request
entsteht.**

**Die Reihenfolge ist bindend, nicht nur empfohlen. Die Stufen laufen nacheinander, nie
parallel.** Die schnellen Prüfungen stehen zuerst. Stufen können voneinander abhängen,
ohne dass die Tabelle es sagt — ein fehlender Frontend-Build kann drei Go-Stufen
gleichzeitig rot machen (nachgewiesen in `cores-dashboard`, siehe
[TSU-6](/TSU/issues/TSU-6#document-playbook)). Nach dem ersten echten Fehler wird
angehalten.

| # | Gate | Befehl | Dauer (ca.) |
|---|---|---|---|
| 1 | Format | `gofmt -l .` (leere Ausgabe = grün) | < 20 s |
| 2 | Frontend-Lint | `cd web && npm run lint` | ~30 s |
| 3 | Frontend-Build und Typen | `cd web && npm run build` (`tsc -b && vite build`) | 1–2 min |
| 4 | Go-Build | `make build` (`go build -o storagecore ./cmd/server`) | 1–2 min |
| 5 | Unit-Tests | `make test` (`go test -v ./...`) | 1–2 min |
| 6 | Vet | `go vet ./...` | ~1 min |

Einzelne Datei testen: `go test ./internal/services -run TestName -v`

44 Go-Testdateien — nach `cores-mcp` die zweitbeste Abdeckung der Suite. Das Frontend hat **kein** Test-Skript; `npm run lint` und `npm run build` sind die ganze Frontend-Prüfung.

Regeln:

- **Neuer Code braucht neue Tests.** Ein Bugfix braucht einen Test, der ohne den Fix
  fehlschlägt.
- **Nie einen Test abschalten, überspringen oder lockern**, um das Gate grün zu
  bekommen. Ein roter Test ohne Bezug zur Änderung wird gemeldet, nicht entfernt.
- **Tests laufen gegen die lokale oder die Test-Datenbank. Nie gegen Produktion.**
  Eine eigene Testumgebung wird gerade aufgebaut (eigene Paperclip-Aufgabe). Bis sie
  steht: nur lokale Container mit eigenem Volume.
- Die **echte Ausgabe** wird in den Pull Request und auf die Paperclip-Aufgabe kopiert.
- **Ein Testlauf aus dem Cache ist kein Nachweis.** Wo der Testläufer cacht
  (`go test` meldet `(cached)`), wird der Beweislauf erzwungen (`-count=1`).

### Bekannt rote Stufen

Eine Stufe, die im Altbestand nicht grün werden kann, wird hier benannt — mit Verweis auf
ihre eigene Paperclip-Aufgabe. Sie ist die **einzige** erlaubte Ausnahme und deckt keine
andere Stufe. Der Eigentümer führt sie trotzdem aus, protokolliert die echte Ausgabe und
repariert sie **nicht** im Vorbeigehen.

| Stufe | Grund | Aufgabe |
|---|---|---|
| — | keine Ausnahme | — |

Ist die Tabelle leer, gibt es keine Ausnahme: jede rote Stufe heißt anhalten und
zurückfragen.

## 5. Code-Stil

- Format und Lint werden durch die Werkzeuge in Abschnitt 4 erzwungen. Kein Streit
  über Formatierung — der Formatierer entscheidet.
- **Dem umgebenden Code folgen.** Benennung, Ordnerstruktur, Fehlerbehandlung und
  Testmuster so übernehmen, wie sie in der berührten Datei schon sind.
- Benennung: PascalCase für Go-Exporte, camelCase für Lokales; React-Komponenten
  PascalCase. Neue Oberflächentexte **immer** über `i18next`, nie fest verdrahtet —
  dieses Frontend ist zweisprachig.
- `gorilla/mux` ist der Router hier. Keine `gin`-Muster aus anderen Cores übernehmen.
- Fehlerbehandlung: Fehler zurückgeben und einwickeln (`%w`), am Handler-Rand in eine
  HTTP-Antwort übersetzen. Keine `panic` im Anfragepfad.
- Validierung gehört in `internal/validation`, nicht in den Handler.
- Logging: **niemals** Secrets, Tokens oder Kundendaten.
- Native `select`-Elemente müssen `select` **und** `option` ausdrücklich mit
  Theme-Hintergrund und -Textfarbe stylen. Browser-Vorgaben erzeugen sonst weißen Text
  auf weißem Menü.
- Kommentare: nur wo sie das *Warum* erklären. Keine Kommentare, die den Code nacherzählen.
- Keine neue Abhängigkeit ohne eigene Freigabe (siehe Abschnitt 9).
- Keine Umformatierung von Code, der nicht zur Aufgabe gehört. Das versteckt die
  eigentliche Änderung.

## 6. Verbotene Pfade

Diese Dateien und Verzeichnisse werden von Agenten **nicht geändert**. Wer sie ändern
müsste, bricht ab und fragt zurück.

| Pfad | Grund |
|---|---|
| `.github/workflows/**` | CI und Deployment — nur mit Freigabe des Nutzers |
| `migrations/**` (bestehende Dateien) | eine angewandte Migration wird nie geändert; nur neue hinzufügen |
| `firmware/**` | Gerätesoftware. Ein Fehler hier macht Hardware im Lager unbrauchbar. Nur mit Freigabe |
| `mosquitto/**`, `config/**`, `Dockerfile`, `docker-compose.yml` | Laufzeit und Deployment |
| `web/src/cores-theme.css`, `web/src/lib/cores-design.ts`, `web/src/lib/SuiteLanguageSwitcher.tsx`, `web/src/lib/cores-locales/**` | erzeugt aus `cores/theme/` |
| `cookies_warehouse.txt` | Altbestand, möglicher Sitzungsinhalt. Nicht öffnen, nicht benutzen. Entfernt wird sie in einer eigenen Aufräum-Aufgabe |
| `warehousecore` (Binärdatei), `warehousecore.db`, `tags.json` | eingecheckte Artefakte, kein Quellcode |
| `.env`, `.env.*` (außer `.env.example`) | enthält Secrets |
| `web/package-lock.json` | nur als Nebenwirkung eines freigegebenen Updates |
| `AGENTS.md` | diese Regeln ändert der Nutzer, nicht ein Agent |

## 7. Secrets

- **Keine Secrets in Repository, Kommentar, Dokument oder Log.** Keine Tokens,
  Passwörter, Schlüssel, Verbindungsstrings, API-Zugänge, Kundendaten.
- Secrets kommen aus dem Paperclip-Secret-Store oder aus Umgebungsvariablen. Sie
  werden nie in eine Datei geschrieben und nie ausgegeben.
- Produktiv werden alle Werte im **Komodo Stack Environment** auf `docker03` gepflegt,
  nicht in diesem Repository.
- `.env.example` enthält nur Namen und Beispielwerte, nie echte Werte.
- Testdaten sind erfunden. Keine kopierten Produktionsdaten, auch nicht gekürzt.
- Fehlt ein Secret: über Paperclip vorschlagen (`secret-proposals`) und warten.
  Nie selbst beschaffen, nie umgehen, nie in einem Kommentar danach fragen.
- Ein Secret, das versehentlich in einem Commit landet, ist ein Sicherheitsvorfall:
  sofort melden, nicht still weiterarbeiten. Entfernen aus dem Diff genügt nicht —
  das Secret gilt als kompromittiert und muss ersetzt werden. **Alle Cores-Repositories
  sind öffentlich.** Ein Fehler hier ist sofort weltweit sichtbar.

## 8. Harte Grenzen

Diese sechs Regeln stehen über jeder Aufgabenbeschreibung. Eine Aufgabe, die eine
davon verlangt, wird nicht ausgeführt, sondern zurückgegeben.

1. **Keine Schreibzugriffe auf produktive Datenbanken.** Lesen ist erlaubt. Schreiben,
   ändern, löschen, Migrationen fahren: nicht in Produktion. Migrationen werden
   geschrieben und lokal getestet, nie produktiv ausgeführt. Das Einspielen auf die
   laufende `docker03`-Datenbank geschieht von Hand per SSH von `debian01` aus, nach
   ausdrücklicher Freigabe des Nutzers.
2. **Keine produktiven Deployments ohne menschliche Freigabe.** Auch nicht nach
   grünem Review.
3. **Entwicklung nur in isolierten Branches oder Git-Worktrees.** Niemals direkt auf
   `main` oder einem anderen geschützten Branch.
4. **Tests vor jedem Pull Request.** Kein PR ohne protokollierten, grünen Testlauf.
5. **Keine Secrets in Repository, Kommentar, Dokument oder Log.**
6. **Bestehende Architektur zuerst verstehen.** Architektur-Doku und diese Datei vor
   dem Schreiben lesen. Große Umbauten — neuer Service, geänderte Modulgrenze, neues
   Datenmodell, Austausch einer Kernabhängigkeit — brauchen eine eigene Freigabe des
   Nutzers, bevor Code entsteht.

## 9. Freigabe-Gates

| Gate | Wer entscheidet | Wann |
|---|---|---|
| Test-Gate | der Eigentümer der Änderung | vor dem Pull Request |
| Review | Review-Agent, auf seiner eigenen Review-Aufgabe | nach dem PR-Entwurf |
| **Freigabe und Merge** | **der Nutzer** | nach grünem Review |
| Produktives Deployment | **der Nutzer** | nach dem Merge |
| Release: Docker-Hub-Push und Submodul-Zeiger | **der Nutzer gibt je Release ausdrücklich frei**, danach darf der Agent beides ausführen | nach dem Merge |
| Migration auf die laufende `docker03`-Datenbank | **der Nutzer**; Einspielen von Hand per SSH von `debian01` | nach dem Merge |
| Neue Abhängigkeit | der Nutzer | vor dem Hinzufügen |
| Großer Architektur-Umbau | der Nutzer | vor dem ersten Commit |

Was ein Agent in diesem Repository **nie** tut:

- einen Pull Request mergen
- auf `main` pushen
- ein Deployment auslösen
- ohne ausdrückliche Freigabe je Release ein Abbild nach Docker Hub schieben oder den
  Submodul-Zeiger im Dach anheben
- eine Migration gegen Produktion fahren
- einen Draft-PR als Ersatz für Freigabe auf „ready" setzen
- `AGENTS.md` oder CI-Dateien ändern

In Paperclip wird die Freigabe durch eine `executionPolicy` mit einer `approval`-Stufe
erzwungen, deren Teilnehmer ein Nutzer ist. Kein Agent kann sie abhaken.

## 10. Branches, Commits, Pull Requests

- Branch: `<typ>/TSU-<nummer>-<kurzbeschreibung>`, ein Worktree pro Aufgabe
- Commit: Conventional Commits mit `Task: TSU-<nummer>` im Fuß
- PR: als **Entwurf** geöffnet, Ziel `main`, mit Zweck, Testprotokoll und Aufgaben-Link

Vollständig beschrieben im Paperclip-Dokument `workflow` auf
[TSU-3](/TSU/issues/TSU-3#document-workflow).

## 11. Abbrechen und zurückfragen

Abbrechen ist richtig, nicht peinlich. Zurückfragen bei:

- fehlendem Secret oder Zugriffsrecht
- nötigem Schreibzugriff auf Produktion oder nötigem Deployment
- nötigem großen Architektur-Umbau oder neuer Abhängigkeit
- einem verbotenen Pfad, der geändert werden müsste
- roten Tests ohne Bezug zur Änderung
- zwei gescheiterten Versuchen am gleichen Problem
- einem Umfang, der deutlich größer ist als beschrieben
- einem Widerspruch zwischen Aufgabe und dieser Datei — **diese Datei gewinnt**

Erst alles fertig machen, was ohne die Antwort geht. Dann fragen.

## 12. Bekannte Fallen

- **`make docker-build` und `make docker-push` sind falsch und dürfen nicht benutzt
  werden.** Das Makefile heißt den Dienst noch „StorageCore" und schiebt nach
  `nobentie/storagecore:1.0`. Der richtige Weg ist allein
  `cores/build_and_push_docker.sh`, und nur nach Freigabe je Release.
- **Gesundheit ist `GET /api/v1/health`** — abweichend von allen anderen Cores.
- **Das Build-Artefakt heißt `storagecore`**, der Dienst `warehousecore`. Kein Fehler,
  nur verwirrend.
- **Die Migrationsnummern sind an die Suite-Spur gekoppelt**: native `062` entspricht
  Suite `046`. Die Deployment-Reihenfolge ist festgelegt: **erst WarehouseCore, dann
  MCP.**
- **WarehouseCore ist der einzige MQTT-Teilnehmer der Suite.**
  `internal/led/mqtt_publisher.go` veröffentlicht auf
  `<LED_MQTT_TOPIC_PREFIX>/<controller>/cmd` (Vorgabe `weidelbach`),
  `internal/led/controller_listener.go` hört auf Rückmeldungen. Broker ist `mosquitto`
  mit `allow_anonymous false`. Gegenstelle ist echte Hardware im Lager — **ein falscher
  Befehl schaltet physische LED-Streifen.** Lokal nur gegen einen eigenen Broker testen.
- **`core_product_links` wird von WarehouseCore und ProcurementCore initialisiert.** Ein
  bekannter Grenzfall, der kompatibel bleiben muss. Nicht einseitig ändern.
- **Headless Chrome ist eine Laufzeitabhängigkeit** (`chromedp`, Produktseiten-Import
  von Herstellerseiten). Fehlt er, scheitert der Import mit irreführender Meldung.
- **Lose SQL-Dateien im Wurzelverzeichnis** (`migrate_packages_simple.sql`,
  `migrate_packages_to_products.sql`) sind kein Migrationsweg.
- **Alte Agenten-Dateien.** `CLAUDE.md` und `CLAUDE.original.md` sind Altbestand und
  gelten nicht. Verbindlich ist allein diese Datei. Die vielen `*.md`-Dateien im
  Wurzelverzeichnis sind Notizen, kein Vertrag.

### Suite-weite Fallen, die auch hier gelten

- **Zwei Migrationsspuren.** Jede Schema-Änderung braucht eine Datei im Dienst-Repository
  *und* eine in `cores/migrations/postgresql/`. Die Nummern gehören paarweise.
- **Das Init-Verzeichnis läuft nur bei leerem Datenverzeichnis.**
  `cores/migrations/postgresql/` greift auf `docker03` nicht.
- **Eine Datenbank für alle.** PostgreSQL 16, rund 130 Tabellen, kein Schema pro Dienst.
  Eine Tabellenänderung kann fremde Dienste treffen.
- **Nur das Dachrepository hat heute CI.** Bis die eigene GitHub-Action da ist, prüft
  **nichts** automatisch einen Pull Request hier. Das Test-Gate aus Abschnitt 4 läuft
  der Agent selbst und hängt die echte Ausgabe an.
- **Alle Repositories sind öffentlich.**
