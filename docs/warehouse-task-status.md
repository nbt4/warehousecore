# Statuswechsel von Lageraufgaben

`PATCH /api/v1/warehouse/tasks/{id}/status` verarbeitet die Statuswerte
`open`, `in_progress`, `done` und `cancelled`. Das Frontend sendet beim Starten
`in_progress` und beim Abschließen `done`. Ein erfolgreicher Statuswechsel
aktualisiert `updated_at`; bei `done` wird zusätzlich `completed_at` gesetzt,
bei den anderen Statuswerten wird es geleert.

## PostgreSQL-Typkonflikt

Migration `039_warehouse_operations_and_handling_units.sql` definiert
`warehouse_tasks.status` als `VARCHAR(20)`. Der Handler verwendet denselben
SQL-Parameter sowohl für die Zuweisung an diese Spalte als auch für den Vergleich
mit `done` in einem `CASE`. Ohne explizite Typangabe kann PostgreSQL 16 diese
Abfrage mit `inconsistent types deduced for parameter $1` ablehnen; der Handler
antwortet dann mit HTTP 400, obwohl der angefragte Status gültig ist.

Beide Verwendungen des Parameters sind deshalb ausdrücklich als `$1::text`
typisiert. Der API-Vertrag und die Status-Constraint bleiben erhalten.

## Regressionstest

`TestUpdateWarehouseTaskStatusPostgres` in
`internal/handlers/warehouse_task_status_test.go` ruft den HTTP-Handler gegen eine
echte PostgreSQL-Datenbank auf. Die Testtabelle verwendet `VARCHAR(20)` und dieselbe
Status-Constraint wie Migration 039: Eine vereinfachte `TEXT`-Spalte würde den
Typkonflikt verdecken.

Der Test prüft Statuswechsel, Änderungs- und Abschlusszeitstempel sowie HTTP 404
für eine fehlende Aufgabe und HTTP 400 für ungültige IDs, ungültiges JSON und
unzulässige Statuswerte. Abgewiesene Anfragen dürfen die Aufgabe nicht verändern.

Die Umgebungsvariable `WAREHOUSE_TEST_DATABASE_URL` muss auf eine lokale,
wegwerfbare Datenbank mit dem Namenssuffix `_test` zeigen. Der Test verwendet ein
eigenes Schema und entfernt es nach dem Lauf.

```sh
go test -count=1 ./internal/handlers -run '^TestUpdateWarehouseTaskStatusPostgres$' -v
```

Der Nachweislauf am 11. Oktober 2026 verwendete einen isolierten
`postgres:16-alpine`-Container ohne Netzwerkzugriff. Die Verbindung erfolgte über
einen lokalen Unix-Socket. Ohne Fix scheiterten alle vier Statuswechsel mit
HTTP 400 und `pq: inconsistent types deduced for parameter $1`. Mit Fix bestanden
alle vier Statuswechsel sowie die Fehlerfälle. Beide Läufe verwendeten `-count=1`.

## Erster Prüflauf vom 11. Oktober 2026

Die Gates wurden nacheinander ausgeführt. Der Nutzer hat die Fortsetzung trotz
der bestehenden Format- und Frontend-Lintfehler ausdrücklich freigegeben.
Diese Dateien und Regeln wurden für den Fix nicht verändert.

| Gate | Befehl | Ergebnis |
|---|---|---|
| Format | `gofmt -l .` | Neun bestehende Dateien gemeldet; beide Go-Dateien des Fixes sind formatiert |
| Frontend-Lint | `cd web && npm run lint` | 49 bestehende Fehler, drei Warnungen |
| Frontend-Build | `cd web && npm run build` | Exit 0 |
| Go-Build | `CGO_ENABLED=1 make build` | Exit 0 |
| Regression ohne Fix | `go test -count=1 ./internal/handlers -run '^TestUpdateWarehouseTaskStatusPostgres$' -v` | Erwarteter Exit 1, HTTP 400 für alle vier Statuswerte |
| Regression mit Fix | derselbe Befehl | Exit 0 |
| Go-Tests | `CGO_ENABLED=1 GOFLAGS=-count=1 make test` | Exit 0, einschließlich der PostgreSQL-Integrationstests |
| Vet | `CGO_ENABLED=1 go vet ./...` | Exit 0, leere Ausgabe |

Die beiden Regressionen und der vollständige Go-Testlauf erhielten dieselbe
lokale `WAREHOUSE_TEST_DATABASE_URL`. Der vorhandene optionale
`TestRenderLabelPDFWithChromium` wurde vom Testläufer ausgelassen, da
`WAREHOUSECORE_CHROME_TEST` nicht aktiviert war. Es wurden keine Tests oder
Prüfregeln für diesen Fix deaktiviert oder gelockert.

Auszug aus der tatsächlichen Ausgabe ohne Fix:

```text
=== RUN   TestUpdateWarehouseTaskStatusPostgres/in_progress
    warehouse_task_status_test.go:64: status 400 want 200: {"error":"pq: inconsistent types deduced for parameter $1"}
=== RUN   TestUpdateWarehouseTaskStatusPostgres/done
    warehouse_task_status_test.go:64: status 400 want 200: {"error":"pq: inconsistent types deduced for parameter $1"}
```

Auszug aus der tatsächlichen Ausgabe des vollständigen Go-Testlaufs mit Fix:

```text
--- PASS: TestUpdateWarehouseTaskStatusPostgres (0.07s)
    --- PASS: TestUpdateWarehouseTaskStatusPostgres/in_progress (0.00s)
    --- PASS: TestUpdateWarehouseTaskStatusPostgres/done (0.00s)
    --- PASS: TestUpdateWarehouseTaskStatusPostgres/cancelled (0.00s)
    --- PASS: TestUpdateWarehouseTaskStatusPostgres/open (0.00s)
PASS
ok  	warehousecore/internal/handlers	32.019s
```

## Vollständige Korrektur der Release-Gates

Auf Wunsch des Nutzers wurden die neun Formatfehler und alle 49 Lintfehler sowie
alle drei Hook-Warnungen behoben. Branch und Commits verwenden ausdrücklich keine
TSU-Referenz. Der Handler und die Health-Version sind für WarehouseCore `5.9.114`
vorbereitet.

Formular-, Rollen-, Gerätebaum- und Labeldaten sind ausdrücklich typisiert.
Unbekannte Fehler werden in `web/src/lib/api-errors.ts` geprüft, bevor Meldungen
in die Oberfläche gelangen. Bestehende serverseitige Meldungsprioritäten und
explizite Fallbacks bleiben erhalten. Vier Frontendtests prüfen diese Prioritäten
und fehlerhafte Antwortformen mit den vorhandenen TypeScript- und Node-Werkzeugen;
es wurde keine Abhängigkeit hinzugefügt.

Der Auth-Provider und `useAuth` verwenden weiterhin denselben Context aus
`auth-context.ts`; der Hook ist für React Fast Refresh in einem eigenen Modul.
Die Ladefunktionen für Lagerplätze und LED-Mapping sind stabile Callbacks. Die
LED-Zielauswahl verwendet den aktuellen Zustand, und der bestehende Packlisten-
Refresh lädt beim Tabwechsel bereits die Anforderungen; der doppelte Ladeeffekt
wurde entfernt.

Alle abschließenden Prüfungen liefen nacheinander und bestanden:

| Prüfung | Ergebnis |
|---|---|
| `gofmt -l .` | Leere Ausgabe |
| `npm run lint` | Exit 0, keine Fehler oder Warnungen |
| `npm run build` | Exit 0 |
| `CGO_ENABLED=1 make build` | Exit 0 |
| `npm test` | Vier Tests bestanden, keine ausgelassen |
| `CGO_ENABLED=1 GOFLAGS=-count=1 make test` | Exit 0, PostgreSQL-Regression und sämtliche PostgreSQL-Integrationstests bestanden |
| `CGO_ENABLED=1 go vet ./...` | Exit 0, leere Ausgabe |

Der vorhandene optionale Chromium-Test bleibt im normalen Testlauf ohne Opt-in.
Die Prüfregeln und bestehenden Tests wurden für die Korrektur nicht gelockert.

Auszug aus dem abschließenden Go-Testlauf:

```text
--- PASS: TestUpdateWarehouseTaskStatusPostgres (0.07s)
ok  	warehousecore/internal/handlers	32.008s
ok  	warehousecore/internal/jobstatus	0.008s
ok  	warehousecore/internal/services	0.227s
ok  	warehousecore/internal/validation	0.004s
```

Der reguläre Release benötigt als nächste Schritte Draft-PR, separates Review und
menschlichen Merge. Die Freigabe zum anschließenden Produktiveinsatz liegt vor.
