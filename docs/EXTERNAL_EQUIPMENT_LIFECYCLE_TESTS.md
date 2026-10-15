# Fremdmietkatalog-Lebenszyklus: Prüfprotokoll

2026-10-15. Ausschließlich synthetische Daten in lokalem PostgreSQL16,
Datenbank `cores_catalog_lifecycle_test`, eigener Compose-Stack und frisches
Volume mit kanonischen Root-Migrationen. Kein produktiver Fachdatentest.

## Vollständige Go-Prüfung

`go test -count=1 ./...` mit den passenden isolierten Testdatenbankvariablen.

```text
?   	warehousecore/cmd/debug_tools	[no test files]
?   	warehousecore/cmd/server	[no test files]
?   	warehousecore/config	[no test files]
ok  	warehousecore/internal/handlers	46.904s
ok  	warehousecore/internal/jobstatus	0.005s
?   	warehousecore/internal/led	[no test files]
?   	warehousecore/internal/metrics	[no test files]
?   	warehousecore/internal/middleware	[no test files]
?   	warehousecore/internal/models	[no test files]
?   	warehousecore/internal/repository	[no test files]
ok  	warehousecore/internal/services	5.137s
ok  	warehousecore/internal/services/storage	0.021s
ok  	warehousecore/internal/validation	0.004s
```

## Race

`go test -race -count=1 ./internal/handlers` mit den passenden isolierten Testdatenbankvariablen.

```text
ok  	warehousecore/internal/handlers	64.173s
```

`go vet ./...` und Serverbuild erfolgreich. Frontendtests:4Tests,4erfolgreich. Native vollständige Editor-Payloads mit unveränderten, geänderten und leeren Zusatzfeldern geprüft.

Die Tests prüfen Teiländerungen, Nullpreise, exakte Version/Kontextbindung,
Archivierung/Wiederherstellung, offene Jobreferenzen aus beiden Tabellen,
historische Preissnapshots, Zusatzfelder, Lieferanten/Duplikate, Rollback bei
Auditfehlern, dauerhaftes Replay und Rechteentzug. Rental führt echte Zwei-
Verbindungs-Rennen aus: Archivierung gegen beide Referenzquellen/Reopen sowie
Reopen ohne bisherige Referenzen gegen die erste neue Referenz, jeweils beide
Reihenfolgen. Die bestehende echte Child-Versionstrigger-Kette wird gegen den
Owner-Sperrplan geprüft. Schema-Reapplication ist idempotent; SQL-Kopien stimmen.

Unabhängiges Review abgeschlossen ohne verbleibende konkrete Findings.
Gefundene MVCC-Reopen-Lücke, umgekehrte Child-/Parent-Sperrreihenfolge und
Native-Editor-Zusatzfeld-Randfälle behoben und mit Regressionen gesichert.
Suite-CI führt die neuen Owner-/Native-/MCP-PostgreSQL-Tests mit Race aus.

Frontend: `npm ci` → `npm test` → `npm run build` erfolgreich. Keine UI-Quelländerung.
