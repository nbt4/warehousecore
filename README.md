# WarehouseCore

## Produkt-Batch und Hersteller-URL — Warehouse 5.9.107 / MCP 1.5.52

Der Katalog umfasst 395 Werkzeuge: 105 Abfragen, 145 Vorschauen und 145
Ausführungen. `warehouse.products.prepare_bulk_create/bulk_create` verarbeitet
1–100 vollständige Produktentwürfe in einer Transaktion. `cores.entities.schema`
für `warehouse.product_batches` beschreibt alle erlaubten Anlagefelder.

Zuerst je Produkt `warehouse.products.prepare_create` für Fuzzy-Auflösung und
Rückfragen nutzen; anschließend dessen native `draft`-Felder in `products`
übernehmen. Vorhandene Stammdaten mit ID referenzieren. `*_name_input` bestätigt
die Anlage eines fehlenden exakten Stammdatensatzes ausdrücklich; neue Kategorien
benötigen `category_abbreviation_input`. Gemeinsame Stammdaten werden einmal
angelegt, widersprüchliche Definitionen blockiert. Unvollständige empfohlene
Angaben erfordern pro Produkt `accept_incomplete`, ähnliche Artikel eine explizite
Prüfung mit `allow_similar_product`; bestehende eindeutige Kennungen einschließlich
archivierter Artikel werden niemals dupliziert.

Die reine Batch-Vorschau zeigt alle Produkte, Referenzen, Stammdatenpläne,
Datenlücken und gemeinsame Lagerkapazität. Aktuelle aktive Administratorrechte,
Create-Scope, unveränderter `expected_context`, `confirm_creation`, exakte
`CREATE WAREHOUSE PRODUCT BATCH ...`-Phrase und Idempotenzschlüssel sind Pflicht.
Preise benötigen zusätzlich ausdrücklich `cores:warehouse:financial`. Produkte,
Hersteller, Marke, Kategoriehierarchie, optionale Procurement-Zuordnung,
Anfangsbestand/Geräte (zusammen höchstens 1000), Audit und dauerhafter Beleg
werden zusammen gespeichert. Ein Fehler rollt alle Datensätze zurück. Dieselbe
Anfrage mit demselben Schlüssel liefert auch nach Neustart den ursprünglichen
Beleg; aktuelle Rechte werden vor jeder Wiederholung erneut im Eigentümer geprüft.
`dry_run` und Vorbereitung erzeugen weder Datensätze noch IDs/Audits/Belege.

`warehouse.products.prepare_create` akzeptiert nun `product_url` einer öffentlichen
HTTP(S)-Produktseite. Warehouse liest begrenzte HTML-/Schema.org-Produktdaten ohne
Shop-Login, Bilddownload oder Warenkorbzugriff. Interne Ziele, Zugangsdaten,
abweichende Ports und zu viele Weiterleitungen sind gesperrt. Erkannte Maße werden
nur mit bekannter Einheit in kg/cm umgerechnet. Mehrdeutige Produktseiten werden
abgewiesen. Ausdrücklich eingegebene Werte haben Vorrang; Text bleibt ungeprüfte
Geschäftsdaten, keine Anweisung. Quelle und ein `creation_input` ohne URL werden
zurückgegeben. Dieser eingefrorene Entwurf wird erneut geprüft und bestätigt;
`create` mit noch gesetzter URL wird abgewiesen, damit die Anlage keine veränderte
Produktseite neu einliest. Für gemeinsame Anlage die geprüften nativen Entwürfe
an die Batch-Vorschau übergeben. Es gibt keine generischen Datei-/HTTP-/SQL-Tools.

Keine neue Schema-Migration. Warehouse zuerst ausrollen, dann MCP. Die übrigen
Anforderungen der Eltern-Issues #4 und #5 bleiben offen.

## Materialanforderungen — Rental 5.3.120 / Warehouse 5.9.106 / MCP 1.5.39

`rental.requirements.prepare_create/create` und `prepare_update/update` nutzen
nun eine geschlossene Rental-Owner-API mit vollständigem Mengenvorschlag.
Neu sind `get`, `search`, `audit_history` und `prepare_archive/archive`,
`prepare_restore/restore`: 353 Werkzeuge (99 Abfragen / 127 Vorschauen /
127 Ausführungen).

`quantity` ist die Gesamtmenge, `manual_quantity` die zusätzlich geplante
manuelle Menge. Positionsanteile werden aus den bestehenden Produktpositionen
berechnet und bleiben servergesteuert. Entweder Gesamt- oder manuelle Menge
angeben; bei beiden müssen die Werte übereinstimmen. Eine manuelle Menge 0
ist erlaubt, wenn eine positive Positionsmenge bestehen bleibt. Job-/Produkt-
Identität ist unveränderlich; andere Identitäten erhalten separate geprüfte Zeilen.
Exakte Zeilen-, Job- und Kontextversion, aktuelle Admin-/Aktionsrechte sowie
Vorschau und deren Bestätigungsphrase sind Pflicht. Aktive Bearbeiter und
Änderungen an Positionen, Geräten oder Produktreferenzen stoppen die Ausführung.
Native Job-Historie, Audit und dauerhafter Beleg werden atomar gespeichert;
Wiederholungen prüfen aktuelle Rechte im Zielservice.

Archive erhalten die ursprünglichen Mengen und die Zeilen-ID. Positionen und
noch zugeordnete Geräte blockieren sie. Archivierte Anforderungen zählen nicht
mehr für aktive Bedarfe, Packlisten und Produkt-/Beziehungs-Abhängigkeiten.
Restore erhält alle Felder und prüft Job, aktives Produkt und Positionsquellen.
Es gibt keine Bestandsbewegungen, Preisänderungen oder externen Nachrichten.
Native manuelle/Positions-Workflows archivieren entfernte Zeilen ebenfalls;
späteres Auswählen stellt dieselbe Identität innerhalb der Geschäftstransaktion
wieder her. Packlisten berücksichtigen zusätzliche manuelle Mengen auch neben
kommerziellen Positionen und erweitern deren Zubehör mit der gesamten Menge.

Rental `049` / Root `035` ergänzt Archivzeitpunkt, exakte monotone Zeilenversionen,
Schutz aller Schreiber und einen Index für aktive Anforderungen. Root `032` und
die Warehouse-Initialisierung behandeln archivierte Materialanforderungen als
historische Referenzen. Rental zuerst ausrollen, danach Warehouse und MCP;
frische/aktualisierte Datenbank und tatsächlichen Streamable-HTTP-Verkehr prüfen.

## Produktbeziehungen — WarehouseCore 5.9.105 / MCP 1.5.35

`warehouse.product_relations` ergänzt `search`, `get`, redigierte
`audit_history` und Vorschau-/Ausführungspaare für `create`, `update`, `archive`
und `restore`. Der Katalog enthält 315 Werkzeuge
(89 Abfragen / 113 Vorschauen / 113 Ausführungen).

Der Eigentümer bietet vollständige Feldpflege für `relation_type` (`required`,
`recommended`, `compatible`, `consumes`, `alternative`, `included`),
`assignment_scope` (`product`, `device`, `case`), `default_quantity` und `notes`.
Anlage standardisiert recommended/product/ein Stück; Teilupdates erhalten
weggelassene Felder, ein expliziter leerer Notizstring leert auf null.
Mengen sind positiv, höchstens 99999999.99, mit maximal zwei Nachkommastellen.
`is_optional` wird bei Anlage/Pflege aus der Beziehungsart abgeleitet.
Produktendpunkte und Beziehungs-ID bleiben unveränderlich. Eine weitere
Beziehungsart desselben Produktpaars ist eine geprüfte Änderung desselben
Datensatzes; identische Duplikate werden nicht angelegt.

Vorschauen zeigen sämtliche Beziehungsfelder, beide Produktversionen, den
vollständigen Diff, betroffene aktive Jobs und Geschäftseffekte. Ausführung
benötigt tatsächliche Adminrechte, create/update/archive-Scope, `confirm_change`,
Idempotenz, vollständigen SHA-256-`expected_context`, bei bestehenden Beziehungen
die genaue `expected_updated_at` sowie die recordgebundene Vorschauphrase.
Archiv/Restore erhält sämtliche Felder und Historie; Restore verlangt aktive
Produkte. Pflichtbeziehungen dürfen keine Zyklen bilden; reziproke
Kompatibilität/Alternativen bleiben möglich. Aktive Jobs des Quellprodukts oder
seiner Vorfahren in der Packlisten-Hierarchie blockieren Änderungen.

`warehouse.products.prepare_link_relation/link_relation` bleibt erhalten und
verwendet denselben Eigentümer. `expected_updated_at` bleibt die Quellprodukt-
Version; die neue Vorschau liefert zusätzlich `expected_relation_updated_at`
und `expected_context`, die bei Bestätigung ebenfalls zu übernehmen sind.
Archivierte Beziehungen ausdrücklich wiederherstellen. Lebenszyklusaktionen
ersetzen keine separate Metadatenpflege.

Warehouse `059` / Umbrella `032` erhalten Historie auch bei alten Core-Schreibern,
versionieren jede Beziehungsänderung und beide Produktendpunkte, und schützen
alle Produkt-Metadatenupdates mit monotonen Mikrosekundenversionen. Auch das
Archivieren eines in aktiven Jobs indirekt benötigten Produkts ist blockiert.
Alte UI-DELETE-Aktionen für Beziehungen sind gesperrt; stattdessen den neuen
geführten Archivpfad verwenden. Ein Archiv wird nicht physisch gelöscht.
Normale Warehouse-/Rental-Vorschläge,
Scanner und rekursive Packlisten verwenden ausschließlich aktive Beziehungen
und Produkte. RentalCore 5.3.116 integriert diesen Filter; neue gemeinsame
Installationen und Upgrades benötigen die aktuelle Warehouse-Schema-Version.

Beziehung, Produktversionen, vollständiger Vorher/Nachher-Audit und dauerhafter
Replay sind atomar. Auditfehler lassen keine Teiländerung zurück; derselbe
Schlüssel kann erneut versucht werden. Erfolgreiche Wiederholung ist auch nach
Neustart identisch. Geschäftsbestand und bestehende Jobanforderungen bleiben
unverändert; die Beziehung steuert Vorschläge und zukünftige Packlisten-
Expansion. Historien schließen alte `product.relation.link`-Audits ein und
geben weder Notizen noch rohe Audit-JSONs aus. Issues #4/#5 bleiben bis zum
Abschluss aller Bereiche der Completion-Liste offen.


## Kategorie-Lifecycle — WarehouseCore 5.9.104 / Cores MCP 1.5.34

Alle drei Ebenen (`warehouse.categories`, `warehouse.subcategories`,
`warehouse.third_categories`) bieten `prepare_archive/archive`,
`prepare_restore/restore` und redigierte `audit_history`. Der Katalog enthält
304 Werkzeuge (86 Abfragen / 109 Vorschauen / 109 Ausführungen).

Archivierung erhält IDs, Namen, Abkürzungen, Elternzuordnung und historische
Produktbeziehungen. Aktive Produkte oder Unterkategorien blockieren sie,
auch über die gesamte untergeordnete Hierarchie. Zuerst Produkte und untere
Ebenen archivieren. Restore verlangt aktive Eltern und passende, eindeutige
Stammdaten; zuerst die Hauptkategorie, dann Unterkategorie und dritte Ebene
wiederherstellen. Restore ändert ausschließlich den Lifecycle. Archivierte
Datensätze vor Metadatenpflege wiederherstellen. Normale Core-Auswahllisten
bieten aktive Hierarchien; MCP-Auflösung zeigt Archive und verlangt deren
Wiederherstellung statt stiller Neuanlage.

Vorschauen zeigen sämtliche gespeicherten Felder, Lifecycle-Diff, Eltern und
aktive/historische Abhängigkeiten. Ausführung benötigt tatsächliche aktuelle
Warehouse-Adminrechte, archive-Scope, `confirm_lifecycle`, Idempotenz, die genaue
`expected_updated_at` und `expected_dependencies` aus der letzten Vorschau sowie
`ARCHIVE|RESTORE WAREHOUSE CATEGORY|SUBCATEGORY|THIRD_CATEGORY <ID>`.
Hauptkategorie-IDs sind kanonische positive Integer-Strings; beide unteren
Ebenen behalten ihre exakten String-IDs (höchstens 50 Zeichen).

Der SHA-256-Abhängigkeitskontext bindet auch archivierte Produkte,
Unterkategorien und Elternversionen. Änderungen nach der Vorschau verlangen
neue Prüfung. Eigentümer-API `/api/v1/admin/mcp/{entity}/{archive|restore}` friert
alle beteiligten Tabellen während der Validierung ein. Lifecycle, vollständiger
Vorher/Nachher-MCP/AI-Audit und dauerhafter Replay sind eine Transaktion.
Auditfehler rollen alles zurück; derselbe Schlüssel kann anschließend erneut
versucht werden. Erfolgreiche Wiederholung bleibt auch nach Neustart identisch.

Warehouse `058` / Umbrella `031` schützen auch bestehende UI-Schreiber gegen
Archivbearbeitung, Referenzen auf inaktive Hierarchien, widersprüchliche aktive
Produktpfade und das Löschen referenzierter Historie. Die ausdrücklich erlaubte
Löschung unbenutzter Kategorien bleibt als getrennte delete-Scope-Aktion mit
Version, Abhängigkeitsprüfung und recordgebundener Bestätigung erhalten.
Die neuen Historien liefern ausschließlich ausgewählte Metadaten und keine
rohen Audit-JSONs. Vollständige Race-/PostgreSQL-Tests, Vet/Build und frische
Streamable-HTTP-Prüfung gehören zur Release-Verifikation. Issues #4/#5 bleiben
bis zum Abschluss sämtlicher Bereiche der Completion-Liste offen.


## Geführte MCP-Inventur — WarehouseCore 5.9.103 / Cores MCP 1.5.33

Die Implementierung ergänzt `warehouse.inventory_counts` mit `search`,
`get`, redigierter `audit_history` und neun benannten Vorschau-/Ausführungspaaren:
`create`, `update`, `set_lines`, `review`, `return_for_counting`, `approve`, `cancel`,
`archive`, `restore`. Der Katalog enthält damit 289 Werkzeuge
(83 Abfragen / 103 Vorschauen / 103 Ausführungen).

Alle Änderungen erfordern aktuelle Adminrechte, passenden Aktionsscope,
`confirm_change`, Idempotenz und den vollständigen `expected_context` aus der
Vorschau. Bestehende Zählungen zusätzlich die exakte `expected_updated_at`;
Zeilen und Ereignisse versionieren ihre Zählung bei allen Schreibern.
Anlage benötigt create, Archiv/Restore archive, übrige Pflege update.
**Freigabe benötigt ausdrücklich `cores:warehouse:approve`; create/update und
Legacy `cores:write` erteilen diese Berechtigung nicht.** Die bestehende
OAuth-Freigabe muss diesen Scope ausdrücklich anfordern und gewähren.
Review, Freigabe, Storno und Lifecycle benötigen außerdem die recordgebundene
Phrase `<OPERATION> WAREHOUSE INVENTORY COUNT <ID>`.

`set_lines` ersetzt 1–100 eindeutige Mengen statt Scans zu addieren. Eine Zeile
nennt `item_type`, exakten `item_key` und entweder `counted_quantity` oder
`clear_counted`. Mengen sind 0–9999999.999 mit maximal drei Nachkommastellen;
Geräte und Cases ausschließlich 0 oder 1. Eine Zählung enthält maximal 1000
Zeilen und einen vollständigen Kontext unter zwei MiB. Der Lagerplatz bleibt
unveränderlich. `blind_count` (Standard true) und Arbeitsnotizen sind Teilupdates;
Notizen werden durch einen expliziten leeren String geleert.

Blinde Zählungen verbergen Sollmengen und Differenzen bis zur bestätigten
Prüfphase, einschließlich der bisherigen Varianzabfrage und verfrühter
Freigabeversuche. Review setzt fehlende Zeilen nur nach ausdrücklicher
`mark_uncounted_zero`-Bestätigung auf null Stück; ansonsten müssen alle Zeilen
gezählt sein. `return_for_counting` erlaubt Korrekturen vor der Freigabe und
benötigt einen Grund. Zählen/Review/Korrektur buchen keinen Bestand.

Die Freigabe prüft den unveränderten Startbestand und den aktuellen vollständigen
Kontext, aktive Artikel/Case-Inhalte, verfügbare Ziel-/Quellhierarchien, Aufträge,
Reservierungen, Packzuordnungen, Wartung/Defekte und Lagerprofile. Ihre Vorschau
zeigt jede Differenz, Geräte-/Case-Lageränderung und projizierte Stück-, Gewichts-
und Volumenbelegung. Produkt-/Case-Maße sind Zentimeter, Gewicht Kilogramm.
Gepackte Cases belegen ihren äußeren Raum; Gewichte enthalten alle verschachtelten
Cases, Geräte und Mengenartikel. Fehlende Maße/Gewichte blockieren gesetzte
physische Limits. Nur das bestehende Kapazitätsmodell `item_count` ist erlaubt.
Ein geänderter Startbestand verlangt Storno und eine neue Zählung.

Erst die gesonderte bestätigte Freigabe schreibt Mengenbestände, Gerätebewegungen,
Case-Ereignisse und ein Differenzjournal. Gepackte Inhalte bleiben gepackt und
folgen der Wurzelposition; Gerätezustände (`condition_status`) sowie Pack-/Jobzuordnungen bleiben erhalten.
Ein unerwarteter Artikel mit null gezählten Stück bleibt an seinem Quellort.
Bestand, globale Mengensummen, Zählung, Lagertermin, Ereignisse, Vorher/Nachher-
Audits und dauerhafter Replay sind eine Transaktion. Storno benötigt einen Grund
und löst ausschließlich den Zählstatus. Nur abgeschlossene/stornierte Zählungen
können archiviert werden. Restore erhält den terminalen Status und sämtliche
Historie; eine erneute Inventur ist eine neue Zählung.

Warehouse-Startup/Migration `057` und Umbrella `030` ergänzen das kanonische
Schema und schützen Versionen, Zeilen, Archive und unveränderliche Startbestände.
Die bisherige UI kann eine MCP-Zählung lesen und zählen; ihre alte, ungeprüfte
Freigabe ist für solche Zählungen blockiert. Dafür den neuen geprüften MCP-Pfad
verwenden. Bestehende UI-Zählungen ohne Startbaseline benötigen für MCP-Abgleich
Storno und Neuanlage. Normale Zähllisten blenden Archive aus.

Gezielte Race-Tests prüfen Autorisierung, Feldweiterleitung, Dry-run und
Wiederholung nach Auditfehlern. Der PostgreSQL-Integrationstest prüft Konflikte,
blinde Zählung, physische Freigabefolgen, vollständigen Rollback bei der letzten
Auditbuchung, identische Wiederholung und Archive/Restore. Vollständige Go-Tests,
Vet/Build und eine neue Datenbank mit dem echten MCP-Endpunkt gehören zur
Release-Prüfung. Die Parent-Issues #4 und #5 bleiben für weitere Bereiche offen.

## Atomare MCP-Lageraufgaben — WarehouseCore 5.9.102 / Cores MCP 1.5.32

`warehouse.tasks` unterstützt vollständige Anlage und Teilupdates, `start`,
`complete`, `cancel`, `reopen`, `archive`, `restore` samt `prepare_*`, `search`
und redigierter `audit_history`. Das Schema beschreibt Typ, Priorität 0–100
(Standard 50), Quelle/Ziel, Case, Gerät, Produkt/Menge, Job, Zuständigkeit,
Termin und Arbeitsnotizen. `clear_fields` leert optionale Werte ausdrücklich;
mindestens eine fachliche Referenz bleibt erforderlich. Geräte-/Produktbezug
muss zusammenpassen; Seriengeräte haben bei angegebener Menge genau ein Stück.
Mengen besitzen höchstens drei Nachkommastellen, Termine eine explizite Zeitzone.

Admin und create/update/archive-Scope, vollständige Vorschau/Diff und Idempotenz
sind erforderlich. Die Anlage behält `confirm_creation`; übrige Aktionen verwenden
`confirm_change` und die genaue `expected_updated_at`. Alle bestätigten Aktionen
verlangen die vollständige `expected_references` aus der Vorschau, einschließlich
ersetzter Verknüpfungen. Aktive Arbeit prüft aktive Geräte/Produkte/Cases/Zonen,
offene Jobs und aktive Zuständigkeiten. Änderungen an referenzierten Datensätzen
oder Ereignissen lassen eine ältere Vorschau scheitern.

Abschluss, Storno, Wiederöffnung und Lifecycle benötigen die Phrase
`COMPLETE|CANCEL|REOPEN|ARCHIVE|RESTORE WAREHOUSE TASK <ID>`; Storno/Wiederöffnung
zusätzlich einen Grund. Nur terminale Aufgaben können archiviert werden. Restore
erhält terminalen Status und Historie, auch wenn Stammdaten inzwischen archiviert
sind; Wiederöffnung prüft aktive Referenzen erneut. Normale Aufgabenlisten blenden
Archive aus. Arbeitsnotizen und Ereignisgründe werden in der Historienabfrage
redigiert; Actor, Herkunft, Zeitpunkt, Zustände und Versionen bleiben abrufbar.

Aufgabe, Ereignis, Referenzversionen, Vorher/Nachher-Audits und dauerhafter Replay
sind atomar. **Aufgabenabschluss quittiert Arbeit und bucht keinen Lagerbestand.**
Physische Geräte-/Case-/Mengenbewegungen bleiben eigene bestätigte Werkzeuge.
Migration Warehouse `056` / Umbrella `029` schützt Archive und versioniert alle
Aufgaben-/Ereignisschreiber sowie betroffene Geräte, Cases, Produkte, Zonen und
Jobs. Umbrella-Neuinstallationen erhalten das kanonische Aufgabenschema.
Fehlgeschlagene Aufgaben-/Wartungsaufrufe lassen sich mit demselben Schlüssel
wiederholen; der atomare Owner-Replay schützt auch nach Transportfehlern.

268 Tools: 80 Abfragen, 94 Vorschauen, 94 Ausführungen. Keine neue Konfiguration.
Inventur und weitere Anforderungen von #4/#5 bleiben im Abschlusscheck offen.

## MCP-Wartungsaufträge und Defekte — WarehouseCore 5.9.101 / Cores MCP 1.5.31

`warehouse.maintenance_orders` und `warehouse.defects` unterstützen `search`,
`prepare_create`/`create`, `prepare_update`/`update`, `prepare_transition`/`transition`,
`prepare_complete`/`complete`, `prepare_cancel`/`cancel`, `prepare_reopen`/`reopen`,
`prepare_archive`/`archive`, `prepare_restore`/`restore` und redigierte
`audit_history` einschließlich Ereignissen. Defektaktionen verwenden kanonische
`order_id`; `legacy_defect_id` ist eine getrennte historische Referenz. Schemas
beschreiben Gerät/Plan, Typ, Priorität, Titel, Beschreibung, Termin/Zeit,
Zuständigkeit, Ergebnis/Abschlussbericht und optionale genaue Dezimalkosten.
Teilupdates erhalten ausgelassene Felder; `clear_fields` leert optionale Werte.
Gerät, Typ und Planbezug bleiben unveränderlich.

Admin/create/update/archive-Scope, vollständige Vorschau mit Diff, genaue
Auftrags-/Geräteversion sowie bei Planbezug die genaue Planversion,
`confirm_change` und Idempotenz sind erforderlich. `transition` bildet den
Core-Statusgraphen ab; Abschluss setzt begonnene Arbeit, Ergebnis und Bericht
voraus. Abschluss, Storno, Wiederöffnung und Lifecycle verlangen zusätzlich
`COMPLETE|CANCEL|REOPEN|ARCHIVE|RESTORE WAREHOUSE MAINTENANCE ORDER <ID>`.
Storno/Wiederöffnung benötigen einen Grund. Nur terminale Aufträge lassen sich
archivieren; Restore erhält Status und Historie, Wiederöffnung erfolgt separat.

Die Vorschau zeigt Gerätezustand/-termine, Planfortschreibung, andere offene
Aufträge und migrierte Defekte. Auftrag, Ereignis, alle Geräte-/Plan-/Legacy-
Folgen, Vorher/Nachher-Audits und dauerhafter Replay bilden eine Transaktion.
Storno eines wiederkehrenden Auftrags überspringt den Zyklus; Abschluss setzt
den nächsten Plantermin auf das gewählte Datum oder heute plus Intervall.
Manuelle Sperre/Ausmusterung und physischer Lagerstatus bleiben erhalten.
Migration Warehouse `055` / Umbrella `028` schützt Archive und versioniert
Ereignisse und Abhängigkeiten bei allen Schreibern; die normale Auftragsliste
blendet Archive aus. Historische Legacy-Zeilen und IDs werden erhalten.

Wartungskosten erfordern **zusätzlich ausdrücklich** `cores:warehouse:financial`.
Legacy `cores:write` erteilt diesen Scope nicht. `cost_amount` ist eine genaue
Dezimalzeichenfolge bis `9999999999.99`; Kostenlesen erfolgt über
`warehouse.maintenance_orders.financial_get`. Ohne Financial-Scope enthalten
Vorschauen, Ergebnisse und Wiederholungen keine Kosten. Flexible Projektionen,
Filter, Sortierung und Aggregate auf Wartungskosten prüfen denselben Scope;
Standardabfragen und Geräte-Wartungshistorien lassen Kosten weg.
OAuth bietet angefragte Wartungskosten als unabhängigen, standardmäßig gesperrten
Select an, auch bei Read-only-Konfiguration. Ein Clientwunsch allein gewährt
keinen Kostenzugriff. Änderungen benötigen weiterhin den Aktionsscope und
Schreibmodus. Bestehende Tokens erhalten keinen zusätzlichen Scope automatisch. Aktuelle
Adminrechte werden vor jeder authentifizierten MCP-Anfrage erneut gelesen;
ein Rechteentzug sperrt auch zuvor gecachte Antworten.

252 Tools: 78 Abfragen, 87 Vorschauen, 87 Ausführungen. Keine neue Umgebungsvariable.
Read-only-Zugriff bleibt unverändert schreibfrei. Race-Tests, saubere Datenbank,
MCP-HTTP-Kontrollen sowie DE/EN, Light/Dark, responsive Tastaturbedienung sind geprüft.
Weitere Anforderungen von #4/#5 bleiben im Abschlusscheck offen.

## Atomare MCP-Wartungspläne — WarehouseCore 5.9.100 / Cores MCP 1.5.30

`warehouse.maintenance_plans` bietet `search`, `prepare_create`/`create`,
`prepare_update`/`update`, `prepare_archive`/`archive`, `prepare_restore`/`restore`
und redigierte `audit_history`. Alle fachlichen Planfelder sind im Schema
`warehouse.maintenance_plans` beschrieben. Updates ergänzen nur angegebene
Felder; `clear_fields=["instructions"]` leert Arbeitsanweisungen ausdrücklich.
Gerätezuordnung und historische Abschlüsse bleiben erhalten.

Die Vorschau zeigt Plan und Gerät, Diff, Duplikate, offene/historische Aufträge,
den neuen Gerätetermin und den Entwurf eines gegebenenfalls fälligen Auftrags.
Admin und create/update/archive-Scope, `confirm_change`, `idempotency_key`,
exakte `expected_device_updated_at` und bei bestehenden Plänen zusätzlich
`expected_updated_at` sind erforderlich. Lifecycle verlangt die Phrase
`ARCHIVE|RESTORE WAREHOUSE MAINTENANCE PLAN <ID>` und keine offenen Aufträge.
Restore benötigt ein aktives, nicht ausgemustertes Gerät und aktives Produkt.
Vorschau und `dry_run` schreiben nichts.

Plan, synchronisierter nächster Gerätetermin, automatisch fälliger geplanter
Auftrag samt Ereignis, Vorher/Nachher-Audits und dauerhafter Replay werden gemeinsam
gespeichert oder zurückgerollt. Zustand und Lagerort des Geräts werden erhalten.
Migration Warehouse `054` / Umbrella `027` versioniert alle Plan-/Auftragsschreiber;
Auftragsänderungen machen auch die Planvorschau ungültig. Neue Umbrella-Datenbanken
erhalten das kanonische Wartungsschema. Neustarts erzeugen bei vorhandenen
benutzerdefinierten Plänen keine zusätzlichen Standardpläne.

215 Tools: 73 Abfragen, 71 Vorschauen, 71 Ausführungen. Keine neue Konfiguration.
Manuelle Auftrags-/Defektprozesse und übrige Anforderungen von #4/#5 sind weiter
im [Abschlusscheck](https://github.com/nbt4/cores-mcp/blob/main/docs/ISSUE_COMPLETION.md) offen.

## MCP-Hersteller und Marken — WarehouseCore 5.9.99 / Cores MCP 1.5.29

Hersteller und Marken unterstützen `prepare_archive`/`archive`,
`prepare_restore`/`restore` und redigierte `audit_history`. Warehouse-Admin,
`cores:warehouse:archive` (oder Legacy `cores:write`), vollständige Vorschau,
exakte Version, `confirm_lifecycle`, Idempotenz und
`ARCHIVE|RESTORE WAREHOUSE MANUFACTURER|BRAND <ID>` sind erforderlich.

Aktive Produkte sperren beide Stammdatenarchive; aktive Marken sperren das
Herstellerarchiv zusätzlich. Historische Produktbeziehungen, Felder und IDs
bleiben erhalten. Restore prüft Identität und aktiven Hersteller der Marke.
Migration Warehouse `053` / Umbrella `026` schützt diese Regeln auch bei
bestehenden Schreibpfaden. Archivierte Datensätze sind vor Bearbeitung zu
restaurieren. Aktive Produkt-/Markenzuordnungen dürfen nicht auf archivierte
Stammdaten zeigen. Normale Auswahllisten zeigen aktive Hersteller und Marken;
MCP-Auflösung zeigt archivierte Identitäten mit Status für Duplikatprüfung.
Mutation, vollständiger Vorher/Nachher-Audit und dauerhafter Replay sind atomar.
Die Historientools liefern keine Roh-JSON, Website, IP oder User-Agent.

205 Tools: 71 Abfragen, 67 Vorschauen, 67 Ausführungen. Keine neue Konfiguration.
Der übrige Umfang von #4/#5 bleibt im [Abschlusscheck](https://github.com/nbt4/cores-mcp/blob/main/docs/ISSUE_COMPLETION.md) dokumentiert.

## Atomare MCP-Gerätestapel — WarehouseCore 5.9.98 / Cores MCP 1.5.28

`warehouse.devices.prepare_bulk_create` und `warehouse.devices.bulk_create`
verarbeiten 1–100 vollständige Geräteentwürfe. Die Vorschau prüft aktive,
einzeln geführte Produkte, alle Metadaten, reservierte Serien-/Scan-Kennungen,
Duplikate innerhalb des Stapels sowie die gesamte Belegung jedes Lagerplatzes.
Lagerprofil und vollständige Hierarchie werden geprüft. Vorschauen erzeugen
keine Geräte, Kennungen, Audits oder Replay-Belege.

Ausführung benötigt Warehouse-Admin, `cores:warehouse:create` (oder Legacy
`cores:write`), `confirm_creation`, `idempotency_key` und die exakte
`confirmation_text` aus der Vorschau. Die zusätzliche Phrase enthält Anzahl
und Fingerprint aller normalisierten Gerätefelder; Änderungen benötigen eine
neue Vorschau und Bestätigung. `dry_run` verhindert die Ausführung.
Geräte, Kennungen, ein Vorher/Nachher-Audit pro Gerät und dauerhafter Replay-Beleg
werden gemeinsam gespeichert oder vollständig zurückgerollt. Keine Etiketten-
oder Dateierzeugung. Automatische IDs werden erst bei Ausführung vergeben.
`cores.entities.schema` liefert das Schema `warehouse.device_batches` inklusive
aller Gerätefelder. `warehouse.devices.audit_history` zeigt auch Stapelereignisse.

195 Tools: 69 Abfragen, 63 Vorschauen, 63 Ausführungen. Keine neue Konfiguration.
Die Issues #4/#5 bleiben bis zum vollständigen [Abschlusscheck](https://github.com/nbt4/cores-mcp/blob/main/docs/ISSUE_COMPLETION.md) offen.

## MCP-Cases — WarehouseCore 5.9.97 / Cores MCP 1.5.27

Cases unterstützen prepare_create/create, prepare_update/update,
prepare_archive/archive, prepare_restore/restore und redigierte audit_history.
warehouse.case_models.search liefert vorhandene Modelle. Vollständige Vorschau,
Diff, Warehouse-Admin und create/update/archive-Scope, explizite confirm_change,
Idempotenz und exakte Mikrosekunden-Version sind erforderlich; Lifecycle verlangt
zusätzlich ARCHIVE|RESTORE WAREHOUSE CASE <ID>. Keine Bestandsbewegung oder
endgültige Löschung. Nullbare Felder sind über clear_fields ausdrücklich leerbar.

Migration Warehouse 052 / Umbrella 025 versioniert Metadaten, Inhalte, Templates
und verschachtelte Cases aller Schreibpfade. Archivierte Cases behalten IDs,
Metadaten und Vorlagen; aktive Inhalte, Jobs, Aufgaben und Lagerabläufe sperren.
Scannerkennungen werden deaktiviert und bleiben reserviert. Restore prüft
Modell, Lagerhierarchie, Profil, Kapazität und aktive Template-Produkte erneut.
Mutation, Vorher/Nachher-Audit mit MCP/AI und dauerhafter Replay-Beleg sind atomar.

193 Tools: 69 Abfragen, 62 Vorschauen, 62 Ausführungen. Keine neue Konfiguration.
Vollständiger Restumfang von #4/#5: [MCP-Abschlusscheck](https://github.com/nbt4/cores-mcp/blob/main/docs/ISSUE_COMPLETION.md).


## Release 5.9.96 – MCP-Lagerplatz-Lebenszyklus

Lagerplätze können über `warehouse.locations.prepare_archive`/`archive` und
`prepare_restore`/`restore` archiviert und wiederhergestellt werden. Die Vorschau
zeigt alle Metadaten, exakte Version, Betriebsstatus-Diff und Abhängigkeiten.
Warehouse-Admin und `cores:warehouse:archive` (oder Legacy `cores:write`) sind
nötig. Ausführung verlangt `expected_updated_at`, `idempotency_key`,
`confirm_lifecycle=true` und exakt `ARCHIVE|RESTORE WAREHOUSE LOCATION <ID>`.
Dry-run ist möglich.

Aktive Geräte (auch auf Jobs), aktive Nachfahren, Cases am Platz oder mit diesem
Heimatplatz, jede Mengenzeile mit Bestand sowie offene Aufgaben und Inventuren
sperren beide Aktionen. Unbekannte Aufgaben-/Inventurstatus sperren ebenfalls;
entgegengesetzte Bestandszeilen heben die Sperre nicht auf. Inventur-Betriebsstatus
sperrt Archivierung. Historische abgeschlossene Vorgänge und archivierte Geräte
bleiben erhalten. Keine Bestandsbewegung, Löschung oder Kaskade auf Kinder.

Restore prüft alle gespeicherten Felder, Identität und die gesamte Elternhierarchie
auf Aktivität, fehlende Knoten und Zyklen. Bei einer unveränderten, protokollierten
MCP-Archivierung wird `available`, `blocked` oder `maintenance` aus dem Vorzustand
wiederhergestellt. Ältere Archive ohne passenden Versionsbeleg oder später
bearbeitete Archive werden als `blocked` aktiviert und benötigen eine bewusste
Betriebsfreigabe im WarehouseCore. Beide Zustandswerte werden im Diff gezeigt.
ID, Code, Scan-Code, Metadaten, Hierarchie, Inventurplanung und Historie bleiben.

`warehouse.locations.audit_history` liefert Administratoren redigierte Ereignisse
mit Nutzer, Herkunft, Zeitpunkt, Ergebnisversion und Statuswechsel. Beschreibungen,
Roh-JSON, IP und User-Agent werden ausgeschlossen. Anlage und Bearbeitung schreiben
jetzt ebenfalls die Ergebnisversion in den Audit. `cores.entities.schema` für
`warehouse.locations` enthält `lifecycle_fields`. WarehouseCore prüft erneut und
speichert Mutation, Audit und Replay atomar. Migration `046` / Umbrella `019`
versioniert bereits alle Lagerplatz-Schreiber; keine neue Migration erforderlich.

Die neuen geführten Admin-Endpunkte sind
`POST /api/v1/admin/warehouse/locations/{id}/archive` und `/restore`.
Die bestehende Archiv-Route nutzt bei MCP-Herkunft denselben strengen Pfad.


## Release 5.9.95 – MCP-Paket-Lebenszyklus

Cores MCP `1.5.25` kann Produktpakete archivieren und wiederherstellen. Die
geführten Admin-Endpunkte `POST /api/v1/admin/product-packages/{id}/archive`
und `/restore` brauchen MCP-Herkunft, Warehouse-Admin, exakte Paket-/Inhaltsversion,
Idempotenzschlüssel, `confirm_lifecycle=true` und
`ARCHIVE|RESTORE WAREHOUSE PACKAGE <ID>`.

Aktive Jobs und nicht freigegebene Reservierungen sperren beide Aktionen.
Geschlossene Jobhistorie bleibt erhalten; Jobs ohne Status gelten als offen.
Restore prüft alle gespeicherten Felder und Bestandteile sowie aktive Produkte
und eindeutigen Namen. Paket-ID, Code, Preise und Inhaltszeilen bleiben erhalten.
Beide Aktionen deaktivieren `website_visible`; Veröffentlichung erfordert danach
bewusst einen separaten Update. Keine Produkte oder Bestände werden bewegt.

Mutation, Audit und dauerhafter Replay-Beleg werden atomar gespeichert. Die
vorhandenen Versionstrigger aus Migration `050` / Root `023` gelten auch hier;
es ist keine neue Migration erforderlich. Audit-Historie ist im MCP redigiert.
Auch Geräte-Abhängigkeiten berücksichtigen jetzt Jobs ohne gesetzten Status.


## Release 5.9.94 – MCP-Gerätepflege und Lebenszyklus

Cores MCP `1.5.24` kann einzelne Geräte vollständig anlegen und ihre Metadaten
bearbeiten. `product_id` muss auf ein aktives, einzeln verfolgtes Produkt zeigen.
Seriennummer, Barcode und QR-Wert sind auch gegenüber archivierten Geräten
reserviert; ausgelassene Scan-Kennungen werden bei der Anlage erzeugt. Ein
optionaler Lagerplatz wird auf Verfügbarkeit, Hierarchie und Kapazität geprüft.
Physischer Zustand und Betriebszustand werden bei Metadaten-Updates bewahrt.

Archivieren und Wiederherstellen verwenden die bestehenden Admin-Gerätepfade
mit MCP-Transaktion, exakter `expected_updated_at`-Version, Idempotenzschlüssel,
`confirm_lifecycle=true` und `ARCHIVE|RESTORE WAREHOUSE DEVICE <ID>`.
Aktive Jobs, Picklisten, Reservierungen, Cases, Komponenten, Aufgaben, Defekte,
Wartungsaufträge und Wartungspläne sperren beide Aktionen. Historie bleibt;
alle Scan-Kennungen werden deaktiviert bzw. reaktiviert. Restore prüft außerdem
Produkt und Lagerkapazität erneut. Geräte werden dabei nicht endgültig gelöscht.

`POST /api/v1/admin/devices/{id}/revert-update` setzt ausschließlich die eigene
letzte unveränderte `device.update`-Änderung mit Herkunft `MCP/AI` zurück. Es
braucht Audit-ID, Version, `confirm_revert=true` und
`REVERT WAREHOUSE DEVICE <ID> UPDATE <AUDIT-ID>`. Jede spätere Geräteänderung
oder jeder spätere Geräte-Audit sperrt diesen Rückweg. Referenzen und Identität
werden erneut geprüft. Revert erhält einen eigenen Audit und Replay-Beleg.

Migration `051_warehouse_device_version` (Umbrella `024`) versioniert alle
Geräte-Schreiber und hält Scan-Kennungen am Archivstatus. Mutation, Audit und
dauerhafter Replay-Beleg werden in einer Transaktion gespeichert. Die
redigierte MCP-Gerätehistorie gibt keine Notizinhalte, IPs oder Roh-JSON aus.


## Release 5.9.93 – Geführte MCP-Produktpakete

Die MCP-Pfade der Admin-Endpunkte `/product-packages` (POST) und
`/product-packages/{id}` (PUT) verwalten Name, Beschreibung, EUR-Preis,
Kategorie, Website-Sichtbarkeit, Aliasse und vollständige Produktlisten mit
Mengen und optionalen Bestandteilen. Adminrechte und Idempotenzschlüssel sind
nötig; Updates verlangen die exakte Version des vorbereiteten Pakets.

Pakete enthalten 1–200 verschiedene aktive Produkte mit ganzzahligen Mengen
von 1 bis 1000000. Preis ist nichtnegativ mit höchstens zwei Nachkommastellen.
Code und ID sind unveränderlich. Bereits in Jobs verwendete Pakete schützen
Preis und Zusammensetzung auch für vergangene Jobs; Metadaten bleiben änderbar.
Es entstehen keine Lagerbuchungen oder spiegelnden Produkte. Metadaten-Updates
erhalten die IDs der Inhaltszeilen. Nullable Beschreibung, Preis und Kategorie
lassen sich über den MCP-Entwurf ausdrücklich leeren.

Paket, Inhalte, vollständiger Vorher/Nachher-Audit und dauerhafter
Wiederholungsbeleg werden atomar gespeichert. Migration `050` versioniert
Metadaten und jede Einfügung, Änderung oder Entfernung einer Produktzeile,
auch über UI-/Importpfade. Der Start installiert Schema und Trigger idempotent.
Cores MCP `1.5.23` liefert die vier geführten Werkzeuge mit Schema-Discovery,
Dublettenprüfung, vollständiger Vorschau/Diff, expliziter Bestätigung und Dry-run.


## Release 5.9.92 – MCP-Kategoriepflege und kontrolliertes Entfernen

Die Admin-Endpunkte für Haupt-, Unter- und dritte Kategorien erlauben MCP/KI
versionsgesicherte Änderungen von Name, Abkürzung und Elternzuordnung.
Kategorienamen müssen 1–100 Zeichen enthalten, Abkürzungen höchstens 10;
auf Hauptebene ist eine Abkürzung erforderlich. IDs bleiben erhalten.
Elternwechsel werden gesperrt, wenn Produktzuordnungen einschließlich der
Nachfahren widersprüchlich würden. Duplikate sind pro Elternknoten gesperrt.

Die benannten DELETE-Endpunkte entfernen über MCP/KI nur ungenutzte Kategorien
ohne Produkte oder Kinder. Adminrechte, exakte `expected_updated_at`,
`confirm_delete=true`, die datensatzgebundene Phrase in `confirmation_text`
und `Idempotency-Key` sind erforderlich. Zusätzliche Fremdschlüssel aus
Erweiterungstabellen sperren das Entfernen bis zur gesonderten Prüfung.
Es gibt kein Cascade, keine automatische Umzuordnung und kein MCP-Undo.
Änderung bzw. Löschung, Vorher/Nachher-Audit und dauerhafter Wiederholungsbeleg
werden atomar gespeichert; Audit bleibt nach der Löschung erhalten.

Migration `049_warehouse_category_version` wird beim Start installiert und
versioniert alle drei Kategorieebenen auch bei normalen UI-/Importänderungen.
Cores MCP 1.5.22 nutzt für Änderungen `cores:warehouse:update`, für Entfernen
`cores:warehouse:delete`; `cores:write` bleibt kompatibel.

## Release 5.9.91 – Robuster Stammdatenstart und Markenidentität

Die historische Kategorieübersetzung lässt vorhandene englische und deutsche
Kategorien mit ihren IDs und Zuordnungen bestehen. Ein Neustart scheitert damit
nicht an gleichnamigen Übersetzungszielen; Kategorien werden nicht gelöscht.
Migration `048_brand_manufacturer_identity` gleicht die eindeutige Markenidentität
an die API an: Name und Hersteller bilden zusammen die Identität. Namen ohne
Hersteller bleiben ebenfalls eindeutig. Unterschiedliche Hersteller dürfen
jeweils dieselbe Markenbezeichnung führen. PostgreSQL 15 oder neuer ist nötig;
der Suite-Stack verwendet PostgreSQL 16.

## Release 5.9.90 – Versionsgesicherte MCP-Hersteller- und Markenpflege

`PUT /api/v1/admin/manufacturers/{id}` und `/brands/{id}` prüfen für MCP/KI
Warehouse-Adminrechte, die exakte `expected_updated_at`, Namen, Websites,
Herstellerreferenzen und Dubletten. Herstellerzuordnungen einer Marke können
nur geändert werden, wenn ihre Produkte bereits zum vorgeschlagenen Hersteller
passen. Eine leere Website wird entfernt; ungenutzte Marken können ausdrücklich
ohne Hersteller geführt werden. Namensänderungen gelten in verknüpften Anzeigen.

Änderung, Vorher/Nachher-Audit und dauerhafter Wiederholungsbeleg werden atomar
gespeichert. Wiederholte identische Aufrufe erzeugen keine zusätzliche Änderung;
veraltete Versionen und abweichende Wiederholungen werden zurückgewiesen.
Die idempotente Migration `047_warehouse_master_version` installiert beim Start
Versionstrigger für `manufacturer` und `brands`; dadurch machen auch normale
Oberflächen- und Importänderungen eine alte Vorschau ungültig.

## Release 5.9.89 – Versionsgesicherte MCP-Lagerplatzpflege

`PUT /api/v1/admin/warehouse/locations/{id}` prüft für MCP/KI die exakte
`expected_updated_at`-Version, Administratorrechte, Code-/Scan-Code-Duplikate,
Elternhierarchie und Bestandsgrenzen. Ein belegter Platz darf kein reiner
Strukturbereich werden; die Kapazität darf seine Belegung nicht unterschreiten.
Die Belegung entspricht der Lagerplatzansicht: aktive Geräte im Lager, Cases
und Mengenbestand. Ausgegebene oder archivierte Geräte zählen nicht mit.
Archivierte Plätze und Änderungen des Betriebsstatus sind ausgeschlossen.
Änderung, Vorher/Nachher-Audit und dauerhafter Idempotenzbeleg werden atomar
gespeichert; unveränderte Felder erzeugen keinen neuen Vorgang. Eine reine
Metadatenänderung verschiebt den Zähltermin nicht. Migration 046 aktualisiert
`updated_at` bei jeder Datenbankänderung, auch aus dem Frontend oder einer
Inventur, damit alte MCP-Vorschauen diese Änderungen nicht überschreiben.

## Release 5.9.88 – Geführte MCP-Lagerplatzanlage

Der geschützte Admin-Endpunkt `POST /api/v1/admin/warehouse/locations` legt
einen Lagerplatz mit explizitem Code an. Er prüft Elternknoten und Duplikate;
Audit mit Herkunft `MCP/AI` und Idempotenzbeleg werden mit dem Datensatz
transaktional gespeichert. Der bisherige UI-Endpunkt bleibt bestehen.

## Release 5.9.87 – Eigenständige Kategorieanlage per MCP

Haupt-, Unter- und dritte Kategorien können per MCP unabhängig von einer
Produktanlage erstellt werden. Die API prüft Namen, Abkürzung, vorhandenen
Elternknoten und Duplikate innerhalb des Elternknotens. Kategorie, Audit mit
Herkunft `MCP/AI` und dauerhafter Idempotenzbeleg werden in einer Transaktion
gespeichert. Wiederholungen mit demselben Schlüssel liefern dieselbe ID.

## Release 5.9.86 – MCP-Stammdaten und Produktbeziehungen

Hersteller und Marken lassen sich für MCP/KI jetzt unabhängig von einer
Produktanlage erstellen. Herstellername und Website werden validiert; eine
Marke braucht eine vorhandene Hersteller-ID. Namensduplikate werden abgewiesen.
Anlage, Audit mit Herkunft `MCP/AI` und dauerhafter Idempotenzbeleg sind
jeweils transaktional. Bei fehlerhaften Typen im Produkt-Update nennt die API
jetzt das betroffene Feld.

`POST /api/v1/admin/products/{id}/dependencies` prüft für MCP/KI-Aufrufe
beide aktiven Produkte und die exakte Quellproduktversion. Die typisierte
Beziehung wird mit Produktversion, Audit-Herkunft `MCP/AI` und dauerhaftem
Idempotenzbeleg atomar angelegt oder geändert. Unveränderte Beziehungen und
veraltete Vorschauen werden abgewiesen. Der PostgreSQL-Integrationstest prüft
Anlage, Änderung, Replay, Versionskonflikt und Audit.

## Release 5.9.85 – Geschützter Produktlebenszyklus

Die Produktarchivierung prüft unter Datensatzsperre offene Jobanforderungen und
gepackte oder ausgegebene Geräte und blockiert den Wechsel bei aktiver Nutzung.
MCP/KI-Aufrufe für Archivierung und Wiederherstellung benötigen jetzt die exakte
Produktversion und einen Idempotenzschlüssel. Produktstatus, betroffene Geräte,
Inventarkennungen, Audit-Herkunft `MCP/AI` und Wiederholungsbeleg werden in einer
Transaktion gespeichert. Der Integrationstest nutzt
`WAREHOUSE_TEST_DATABASE_URL=postgres://.../warehouse_test go test ./internal/handlers`.

## Release 5.9.84 – Idempotente MCP-Produktanlage

`POST /api/v1/admin/products` speichert MCP/KI-Anlagen nun mit demselben
dauerhaften Idempotenzbeleg wie Produktänderungen. Wiederholte Anfragen mit
demselben Schlüssel liefern dieselben Produkt- und Geräte-IDs; abweichende
Eingaben mit einem bereits verwendeten Schlüssel werden abgewiesen.
Aufgelöste oder neu angelegte Stammdaten, Produkt, Bestand, Geräte, Audit mit
Herkunft `MCP/AI` und Wiederholungsbeleg werden in einer Transaktion gespeichert.
Der PostgreSQL-Integrationstest prüft Anlage, Replay, Konflikte und Update mit
`WAREHOUSE_TEST_DATABASE_URL=postgres://.../warehouse_test go test ./internal/handlers`.

## Release 5.9.83 – Startseiten-Hervorhebung

Freigegebene Mietparkprodukte können im Produktdialog zusätzlich auf der
Tsunami-Startseite angepinnt werden. `website_featured` wird im öffentlichen
Produktfeed nur für ohnehin sichtbare und aktive Produkte ausgegeben.
Archivieren oder Ausblenden entfernt die Hervorhebung. Die idempotente
Migration `046_website_featured.sql` ergänzt die Spalte; der Start-Upgrader
holt sie auch bei bestehenden Installationen nach.

## Release 5.9.82 – Geführte Produktänderung

`PUT /api/v1/admin/products/{id}` unterstützt für MCP/KI-Aufrufe nun die
exakte `expectedUpdatedAt`-Version und einen dauerhaften Idempotenzschlüssel.
Die Änderung sperrt das Produkt während der Transaktion; Produktdaten,
Audit-Herkunft und Wiederholungsbeleg werden zusammen gespeichert oder
zurückgerollt. Bestehende UI-Aufrufe bleiben ohne die zusätzlichen Felder
möglich. Ein leerer generischer Barcode entfernt nun den bisherigen Wert.
`migrations/045_product_mcp_idempotency.sql` dokumentiert die neue Tabelle;
der Start-Upgrader legt sie auch ohne historische SQL-Migration an. Der
PostgreSQL-Integrationstest läuft mit
`WAREHOUSE_TEST_DATABASE_URL=postgres://.../warehouse_product_test go test ./internal/handlers`.

## Release 5.9.81 – A4-Etikettenbögen und individuelle Stückzahlen

Das Druckcenter kann ausgewählte Labels nun mit einer eigenen Kopienzahl je
Eintrag ausgeben. PDF-Export und Browserdruck unterstützen wahlweise weiterhin
eine maßhaltige Seite je Label oder einen automatisch gefüllten A4-Bogen in
Hoch-/Querformat. Seitenrand, horizontale und vertikale Abstände sowie optionale
Schnittführungen sind einstellbar; dieselben individuellen Stückzahlen werden
auch beim Zebra-Direktdruck berücksichtigt. Pro Anfrage gelten weiterhin
höchstens 250 Ziele und 500 Labels.

## Release 5.9.80 – Flexibler Datentransfer

WarehouseCore stellt einen schema-beschriebenen Datentransfer für Produkte,
Geräte, Kontakte, Hersteller, Marken, Kategorien, Lagerbereiche, Kabel und Jobs
bereit. Exporte unterstützen frei gewählte Felder, lesbare oder technische
Spaltennamen, CSV mit drei Trennzeichen sowie XLSX. Admin-Importe akzeptieren CSV
und XLSX bis 5.000 Zeilen, ordnen Spalten anhand ihrer Überschrift zu, validieren
Typen und Referenzen in einer Vorschau und schreiben atomar. Konflikte werden
über stabile IDs, Codes, Barcodes, E-Mail oder Namen erkannt und lassen sich
überspringen oder je Feld mit Importwert, Bestandswert beziehungsweise
„nur leere Felder“ zusammenführen. Die bisherigen festen CSV-Endpunkte bleiben
kompatibel; jeder bestätigte Sammelimport wird mit Datensatz und Ergebnis im
Audit-Log protokolliert.

## Release 5.9.79 – Vollständige Dashboard-Lokalisierung

Die Suite-Sprache wirkt jetzt bidirektional auf deutsche und englische
Quelltexte. Das Warehouse-Dashboard lokalisiert Kennzahlen, Arbeitsvorrat,
Materialfluss, Bewegungen und dynamische Mengen beziehungsweise Zeitangaben.

## Release 5.9.78 – Atomare MCP-Produktanlage

`POST /api/v1/admin/products` kann fehlende Hersteller, Marken sowie die
dreistufige Kategoriehierarchie jetzt über explizite `*_name_input`-Felder
auflösen oder zusammen mit dem Produkt anlegen. Stammdaten, Produkt,
Anfangsbestand und Devices bleiben dabei in einer Transaktion; neue
Stammdatensätze und das Produkt werden dem handelnden Suite-Benutzer im
Audit-Log zugeordnet.

## Release 5.9.77 – Suiteweite Sprachwahl

Der bestehende deutsch/englische i18next-Katalog verwendet jetzt die gemeinsame
`cores_language`-Auswahl der gesamten Suite. Der Umschalter bleibt in Desktop-,
Kompakt- und Mobilnavigation zugänglich und synchronisiert sich beim Core-Wechsel.

## Release 5.9.76 – Robuster Start auf frischen Installationen

- Die Produktstamm-Migration legt `product_dependencies` nun selbst idempotent an, bevor sie die typisierten Beziehungen ergänzt. Frische Umbrella-Installationen und Systeme mit übersprungenen historischen Migrationen starten dadurch ohne manuellen Datenbankeingriff.

## Release 5.9.75 – Zuverlässige Produktarchivierung

- Produktarchivierung und Wiederherstellung schalten Produkt- und Device-Kennungen nun mit getrennt typisierten SQL-Parametern zuverlässig gemeinsam um.

## Release 5.9.74 – Geräte-Lebenszyklus

- Devices lassen sich archivieren, wiederherstellen und nach vorheriger Archivierung endgültig löschen; archivierte Geräte verschwinden aus Scans, Cases, Picklisten, Labels, Lagerbeständen und operativen Kennzahlen.
- Beim Archivieren eines Produkts werden alle aktiven Devices atomar mitarchiviert. Eine Wiederherstellung reaktiviert ausschließlich die Devices, die durch genau dieses Produktarchiv deaktiviert wurden.
- Archivierte Produkte können samt ihren archivierten Devices endgültig gelöscht werden, sofern keine geschützten historischen Produktverwendungen mehr bestehen.

## Release 5.9.73 – Packlisten und universeller Scanner

- Jobdetails erzeugen und speichern eine A4-Packliste als PDF mit Job-Barcode, Titel, Produktmengen, eingerücktem Zubehör und zentral gepflegtem Firmenlogo. Der gespeicherte Stand kann jederzeit heruntergeladen oder explizit aus den aktuellen Jobdaten neu erzeugt werden.
- Der Scanner erkennt Lagerplätze, bestätigte Jobs, Geräte, Mengenprodukte und Cases automatisch. „Lagerplatz → Artikel“ lagert ein, „Job → Artikel“ gibt auch noch nicht zugeordnete Geräte aus, reine Produktscans öffnen die Stammdaten und Case-Scans bewegen den gesamten Inhalt.
- Der eigene Case-Packmodus führt von einem dynamischen oder hybriden Case zu Geräten, Mengenartikeln oder Untercases und fragt Mengen ausdrücklich ab.
- Die Suite-Navigation steht wie in allen Cores am Ende der Fachnavigation als Core-Auswahl plus eigenständiger Dashboard-Link zur Verfügung.

## Einheitliches Cores Designsystem

WarehouseCore folgt dem verbindlichen Designvertrag aus [`nbt4/cores`](https://github.com/nbt4/cores/blob/main/docs/DESIGN_SYSTEM.md). Inter-Typografie, Graphitpalette, roter Akzent, 256/80-px-Sidebar, Tabellen, Formulare, Selects, Dropdowns, Scrollbars und Dashboard-Aufbau sind suite-weit identisch; Lagerstatusfarben bleiben ausschließlich semantisch.

`web/src/cores-theme.css` und `web/src/lib/cores-design.ts` sind generierte Umbrella-Artefakte und dürfen nicht direkt geändert werden. Vor Releases sind die Design-Sync-/Check-Skripte im Umbrella sowie Frontend-Build und Go-Tests auszuführen.

**Lagerverwaltung und Inventarmanagement im Cores-Ökosystem — Geräte-Tracking, Zonenverwaltung, LED-Bin-Highlighting, Etikettendruck und Barcode-Scanning.**

---

## Features

- **Geräteverwaltung** — Vollständiges Inventory-Tracking mit Hierarchiebaum, Statusverfolgung, Bewegungsprotokoll und Defekterfassung
- **Produktstammdaten 2.0** — Produktklasse, Zubehörrolle und Trackingart sind getrennt. Produkt, initiale Devices, Lagerzuordnung und automatisch erzeugte Kennungen entstehen transaktional; Modellnummer, Herstellerartikelnummer und EAN bleiben eigene Felder
- **Flexibler Datentransfer** — Schema-getriebene CSV-/XLSX-Exporte und atomare, vorschaubasierte Importe mit headerbasierter Zuordnung und spaltenweisen Konfliktregeln
- **ProcurementCore-Verknüpfung** — Bestehende Produktstämme lassen sich automatisch vorgeschlagen oder manuell eindeutig verbinden. Procurement-Artikel öffnen den vollständigen Warehouse-Produktdialog mit bearbeitbaren Vorbelegungen; Warehouse meldet daraus direkt einen Procurement-Bedarfsentwurf
- **Unveränderliche Scan-IDs** — Produkte (`PRD-…`), Devices (`DEV-…`) und Cases (`CAS-…`) erhalten automatisch globale Barcodes. Bestehende Gerätekennungen bleiben als Scan-Aliase gültig, während fehlende Barcodes und QR-Codes beim Upgrade sicher ergänzt werden
- **Typisierte Produktbeziehungen** — Benötigtes, empfohlenes, kompatibles, verbrauchtes, alternatives oder enthaltenes Zubehör wird mit Standardmenge gepflegt; feste Device-Komponenten können zusätzlich einem konkreten Exemplar zugewiesen werden
- **Konsistente Mengenbestände** — Lagerzonen sind die führende Bestandsquelle; Produktsummen werden automatisch aus `product_locations` synchronisiert und verteilte Bestände ausschließlich über Lager- und Scanabläufe korrigiert
- **Professionelle Lagersteuerung** — Hierarchische Standorte, Bereiche, Gänge, Regale, Ebenen und Fächer mit PostgreSQL-kompatibler automatischer Code- und Barcode-Erzeugung, optionaler Übernahme bestehender Fremdetiketten, getrennter Bauart/Prozessfunktion, Sperrstatus, Pick-Reihenfolge, Kapazität, Arbeitsvorrat und sicherem Archivieren nur leerer Orte
- **Live-Lagercockpit** — Handlungsorientiertes Dashboard mit priorisierten Aufgaben, Einsatzbereitschaft, Materialfluss, Tagesbewegungen, aktiven Jobs, Case-Prozessen, technischen Risiken und direktem Einstieg in Scanner und Lageraktionen
- **Scannerbasierte Zählinventur** — Offene oder verdeckte Sollmengen, automatische Platzsperre, Zählung von Geräten, Mengenartikeln und Cases, Abweichungsprüfung und kontrollierte Bestandsfreigabe
- **LED-Bin-Highlighting** — Echtzeit-Steuerung von LED-Streifen via MQTT zur visuellen Hervorhebung von Pick-Positionen. Unterstützt Selbsthosting (Mosquitto) und Cloud-Broker
- **Label Studio & Direktdruck** — Visueller Designer für Geräte-, Kabel-, Case- und Zonenlabels mit direktem Verschieben/Skalieren über Ziehpunkte und einpassbarer 25–200-%-Vorschau. Dauerhaft gespeicherte PDF-Master, schneller PDF-Download/Browserdruck aus dem Cache und protokollierter Zebra-ZPL-Direktdruck über TCP
- **Geführtes Barcode-Scanning** — Automatische Abläufe „Lagerplatz → Artikel“ und „Job → Artikel“, eigenständiger Case-Packmodus, Mengenabfrage für mengenbasierte Produkte, Produkt-Infodialog sowie nachvollziehbare Bewegungs- und Scanprotokolle
- **Eindeutige Gerätestatus** — `on_job` bezeichnet nur aktive Ausgaben. Nach Jobabschluss bleibt ein nicht eingebuchtes Gerät als „Rückgabe offen“ sichtbar; alte Datensätze ohne Job oder Lagerplatz werden als „Standort ungeklärt“ ausgewiesen.
- **Hybrides Kabelinventar** — Kabel als normale Produkte mit strukturierten Anschlüssen, Länge und Querschnitt verwalten. Wahlweise gemeinsamer Artikelbarcode mit Mengenbestand je Lagerzone oder individueller Barcode je physischem Kabel
- **Job-Picklisten und PDF-Packlisten** — Picklist-Scanbestätigung sowie dauerhaft gespeicherte, per Knopfdruck aktualisierbare A4-Packlisten mit Barcode, Firmenlogo und rekursiv eingerücktem Zubehör
- **Transparenter Rücklauf** — Dashboard und Scanner zeigen alle auf Rückgabe wartenden Geräte mit Produkt, Job und Betriebszustand; Zustand und Ziel-Lagerplatz lassen sich zusätzlich zum Scanprozess kontrolliert manuell bestätigen
- **Kontextuelle Bestandssuche** — Produkt-, Geräte-, Paket-, Kabel-, Label- und Wartungssuchen berücksichtigen neben Titeln auch Marke, Hersteller, Modellkontext, Kategorien, Barcodes, Seriennummern und technische Parameter; mehrere Suchbegriffe dürfen aus unterschiedlichen Feldern stammen
- **Dynamische Handling Units** — Leere Euroboxen und Flightcases je Job frei befüllen, feste oder hybride Soll-Inhalte pflegen, Geräte/Mengenartikel/Untercases scannen, versiegeln, komplett ausgeben, im Rücklauf prüfen und gesammelt zurücklagern
- **Wartungsmanagement** — Priorisierter Arbeitsvorrat für Defekte, vorbeugende Wartung, Prüfungen und Kalibrierungen; wiederkehrende Gerätepläne erzeugen fällige Aufträge automatisch, während Verantwortliche, Statusereignisse, Abschlussnachweis, Kosten und Folgetermin revisionsfähig zusammenbleiben
- **Öffentliche Produktseite** — Ungeschützte API für getrennte Produkt- und Paketlisten, jeweils mit Website-Freigabe und Bildern
- **Startseiten-Hervorhebung** — Im Produktdialog unter „Website“ lässt sich ein freigegebenes Produkt zusätzlich auf der Tsunami-Startseite pinnen. Der öffentliche Produktfeed liefert `website_featured`; ausgeblendete und archivierte Produkte erscheinen dort nicht. Die Änderung invalidiert den Website-Cache per Revalidate-Webhook.
- **Produktpakete** — Pakete unabhängig von normalen Produkten verwalten, Produkte mit Mengen zuweisen und eigene Paketbilder pflegen
- **Role-Based Access** — Feingranulares Rollensystem mit Admin-Bereich für Benutzer-, Kategorie- und LED-Konfiguration
- **Installierbare Mobile-App (PWA)** — Standalone-Modus mit WarehouseCore-App-Icon, Safe-Area-Unterstützung, großen Touch-Zielen, App-Tabbar und Drawer-Navigation; dasselbe Image läuft auf der eigenen Domain unter `/` oder im globalen Suite-Pfadmodus unter `/warehousecore/`
- **Zentrales Branding** — Live geladene Varianten für Bildmarke, Sidebar, Login, Browser-Tab und dynamisches PWA-Manifest über `/api/v1/branding`; auch die Branding-Defaults sind im Suite-Pfadmodus mount-sicher
- **Einheitliche Navigation** — Ein-/ausklappbare Sidebar mit normierter Logo-/Symbolfläche, suite-weitem Core-Auswahlfeld und eigenständigem Dashboard-Link an derselben Position in allen Cores

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

Die WarehouseCore-Oberfläche leitet bei fehlender Sitzung zum zentralen
Cores-Login weiter und kehrt nach lokaler oder Microsoft-Anmeldung zur
ursprünglichen Warehouse-Ansicht zurück. Die Auth-Endpunkte bleiben für
kompatible API-Clients bestehen.

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
| `GET`   | `/api/v1/scans/resolve`                 | Scan-Code ohne Bestandsänderung auflösen (🔒) |
| `GET`   | `/api/v1/scans/history`                 | Scan-Historie (🔒)                        |

`POST /api/v1/scans` akzeptiert `scan_code`, `action`, optional `job_id`, `zone_id` und `quantity`. `job_id` bezeichnet ausschließlich den echten Zieljob; Mengen werden nicht mehr über dieses Feld transportiert. Eine Ausgabe benötigt einen offenen Job, eine Einlagerung einen bestätigten Lagerplatz.

### Job-Packlisten

| Methode | Pfad                                      | Beschreibung |
|---------|-------------------------------------------|--------------|
| `GET`   | `/api/v1/jobs/:id/packing-list.pdf`       | Gespeicherte Packliste laden oder erstmalig erzeugen (🔒) |
| `POST`  | `/api/v1/jobs/:id/packing-list`           | Packliste aus aktuellen Jobdaten neu erzeugen und speichern (🔒) |

PDFs liegen dauerhaft unter `PACKING_LIST_DIR` (Standard `/var/lib/warehousecore/packing-lists`). Für Docker-Deployments ist dafür das Volume `warehousecore-packing-lists` vorgesehen.

### Wartung und Instandhaltung

| Methode | Pfad                                      | Beschreibung |
|---------|-------------------------------------------|--------------|
| `GET`   | `/api/v1/maintenance/overview`            | Fälligkeiten, offene Defekte, laufende Arbeiten und Monatsleistung |
| `GET`   | `/api/v1/maintenance/options`             | Geräte- und Mitarbeiterauswahl für Wartungsformulare |
| `GET`   | `/api/v1/maintenance/orders`              | Aktiven Arbeitsvorrat oder Historie filtern |
| `POST`  | `/api/v1/maintenance/orders`              | Defekt-, Wartungs-, Prüf- oder Kalibrierauftrag anlegen |
| `PUT`   | `/api/v1/maintenance/orders/:id`          | Stammdaten eines aktiven Auftrags aktualisieren |
| `POST`  | `/api/v1/maintenance/orders/:id/transition` | Kontrollierten Statuswechsel oder Abschluss buchen |
| `GET`   | `/api/v1/maintenance/orders/:id/events`   | Revisionsfähigen Auftragsverlauf laden |
| `GET`   | `/api/v1/maintenance/plans`               | Wiederkehrende gerätebezogene Pläne laden |
| `POST`  | `/api/v1/maintenance/plans`               | Wartungsplan anlegen |
| `PUT`   | `/api/v1/maintenance/plans/:id`           | Wartungsplan bearbeiten, pausieren oder aktivieren |

Aktive Pläne erzeugen einmalig innerhalb ihres Vorlaufs einen geplanten Arbeitsauftrag. Beim Start wird das Gerät auf `maintenance` gesetzt; Defektmeldungen setzen es auf `defective`. Ein erfolgreicher Abschluss stellt `available` nur wieder her, wenn kein weiterer blockierender Auftrag existiert. Abschluss und Plan aktualisieren außerdem `lastmaintenance` und `nextmaintenance` am Gerät. Sämtliche Wartungsendpunkte erfordern eine authentifizierte Sitzung.

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

Die Lagersteuerung unter `/zones` verwendet die erweiterten Endpunkte. Bauart (`location_kind`) und Prozessfunktion (`process_role`) sind bewusst getrennt: Ein physischer Bereich kann beispielsweise Wareneingang, Rücklauf, Prüfung, Quarantäne, Reparatur, Kommissionierung oder Job-Bereitstellung sein. Nur aktive, verfügbare und direkt belegbare Orte akzeptieren Einlagerungen. Kapazitätsgrenzen und gesperrte Elternbereiche werden serverseitig geprüft; belegte Orte können nicht archiviert werden.

Die Kennzahl „Nicht zugeordnet“ zählt alle Geräte ohne Lagerplatz, die weder ausgegeben noch in einem Case enthalten sind, sowie alle nicht ausgegebenen, nicht verschachtelten Cases ohne Lagerplatz. Case-Inhalte werden dadurch nicht doppelt gezählt; Mengenbestände ohne Ort erscheinen separat.

Das Dashboard aktualisiert Betriebskennzahlen automatisch alle 30 Sekunden und ordnet sie nach Handlungsbedarf. `GET /api/v1/dashboard/stats` liefert neben Lager- und Gerätezuständen auch heutige Ein-, Aus- und Umlagerungen, aktive Jobs, Case-Workflows, offene Defekte und – sofern das optionale Inspektionsmodul installiert ist – überfällige Prüfungen. Der manuelle Refresh aktualisiert Dashboard, Bewegungen und Lagerübersicht gemeinsam.

Geräte verwenden zwei voneinander unabhängige Statusdimensionen. Der Lagerstatus wird ausschließlich von den physischen Workflows gesetzt: `location_unknown` bei Neuanlage oder Verlust der Zuordnung, `in_storage` durch Einlagerung, Lagerplatzzuweisung, Inventurbestätigung oder Packen in ein Case, `on_job` durch Einzel- oder Case-Ausgabe und `return_pending` beim Jobabschluss bzw. prüfpflichtigen Rücklauf. Der Betriebszustand (`available`, `blocked`, `defective`, `maintenance`, `retired`) wird administrativ oder über Defekt-/Reparaturvorgänge gepflegt und bleibt bei Ortsbewegungen erhalten. Geräte sind nur mit `in_storage` plus `available` ausgabefähig. Änderungen an Status, Zustand und Ort werden in `device_status_history` protokolliert.

Job- und Lagerstatus bleiben ebenfalls getrennt. WarehouseCore zeigt für Vorbereitung, Einzelgeräte-, Mengenartikel- und Case-Ausgaben ausschließlich Jobs mit dem RentalCore-Status `Bestätigt`. `Planung` ist noch nicht zur Ausgabe freigegeben; `Abgeschlossen` und `Storniert` sind terminal. Beim Jobabschluss wechseln ausgegebene Geräte auf `return_pending`, bis eine physische Rücknahme sie wieder als `in_storage` bestätigt. Der Rechnungsstatus hat auf diesen Ablauf keinen Einfluss.

### Lagersteuerung & Inventur

| Methode | Pfad                                      | Beschreibung |
|---------|-------------------------------------------|--------------|
| `GET`   | `/api/v1/warehouse/overview`              | Kennzahlen, ungeklärte Bestände und fällige Inventuren |
| `GET`   | `/api/v1/warehouse/locations`             | Vollständige professionelle Lagerstruktur |
| `POST`  | `/api/v1/warehouse/locations`             | Lagerort anlegen |
| `PUT`   | `/api/v1/warehouse/locations/:id`         | Lagerort, Rolle, Kapazität und Zählintervall ändern |
| `POST`  | `/api/v1/warehouse/locations/:id/archive` | Leeren Lagerort sicher archivieren |
| `GET`   | `/api/v1/warehouse/tasks`                 | Putaway-, Move-, Pick-, Replenish-, Pack- und Prüfaufgaben |
| `POST`  | `/api/v1/warehouse/tasks`                 | Lageraufgabe anlegen |
| `PATCH` | `/api/v1/warehouse/tasks/:id/status`      | Arbeitsstatus ändern |
| `GET`   | `/api/v1/warehouse/counts`                | Zählinventuren und Historie |
| `POST`  | `/api/v1/warehouse/counts`                | Platz sperren und Zählinventur starten |
| `POST`  | `/api/v1/warehouse/counts/:id/scan`       | Gerät, Mengenartikel oder Case zählen |
| `POST`  | `/api/v1/warehouse/counts/:id/complete`   | Zählung abschließen und Differenzen anzeigen |
| `POST`  | `/api/v1/warehouse/counts/:id/approve`    | Geprüfte Werte freigeben und Bestand abgleichen |
| `POST`  | `/api/v1/warehouse/counts/:id/cancel`     | Inventur verwerfen und Platz freigeben |

### Produkte

| Methode  | Pfad                                  | Beschreibung                                      |
|----------|---------------------------------------|---------------------------------------------------|
| `GET`    | `/api/v1/admin/products`              | Aktive Produkte auflisten; Statusfilter möglich (🔒) |
| `POST`   | `/api/v1/admin/products`              | Typisiertes Produkt erstellen (🔒 Admin)          |
| `PUT`    | `/api/v1/admin/products/:id`          | Produktstammdaten aktualisieren (🔒 Admin)         |
| `DELETE` | `/api/v1/admin/products/:id`          | Produkt und seine aktiven Devices archivieren (🔒 Admin) |
| `PUT`    | `/api/v1/admin/products/:id/restore`  | Produkt und dadurch archivierte Devices wiederherstellen (🔒 Admin) |
| `DELETE` | `/api/v1/admin/products/:id/permanent` | Archiviertes Produkt samt Devices endgültig löschen (🔒 Admin) |

`POST /api/v1/admin/products` akzeptiert zusätzlich `product_kind`, `model_number`, `manufacturer_part_number`, `ean`, `initial_device_quantity` und `initial_zone_id`. Für geführte Integrationen lösen `manufacturer_name_input`, `brand_name_input`, `category_name_input`, `subcategory_name_input` und `third_category_name_input` exakte Stammdaten auf oder legen sie atomar an; neue Kategorien benötigen `category_abbreviation_input`, die tieferen Ebenen akzeptieren ihre jeweilige optionale Abkürzung. Bei Einzelverfolgung werden Produkt und Anfangsexemplare in derselben Transaktion angelegt. Die Zubehörendpunkte `/api/v1/admin/products/:id/dependencies` verwenden `relation_type` (`required`, `recommended`, `compatible`, `consumes`, `alternative`, `included`) und `assignment_scope`.

Die Admin-Registerkarte **Beschaffung** gleicht Warehouse-Produkte anhand stabiler Artikelmerkmale mit ProcurementCore ab. `GET /api/v1/admin/product-links` liefert Verknüpfungen und Vorschläge, `/products/:id/procurement-link` bestätigt oder löst eine Zuordnung und `/products/:id/procurement-requisitions` erzeugt einen Procurement-Bedarfsentwurf. Eine Neuanlage mit `procurement_product_id` speichert Warehouse-Produkt, automatisch erzeugte Produkt-/Device-IDs und die eindeutige Core-Verknüpfung gemeinsam in einer Transaktion. `PROCUREMENTCORE_PUBLIC_URL` steuert die serviceübergreifende Navigation.

`GET /api/v1/admin/products` akzeptiert `lifecycle_status=active|archived|all`. Mengenbestände werden aus `product_locations` berechnet; bei auf Lagerzonen verteiltem Bestand erfolgen Korrekturen über Zonen- oder Scanabläufe.

Devices besitzen denselben Lebenszyklus. `GET /api/v1/admin/devices-list` akzeptiert `lifecycle_status=active|archived|all` (Standard: `active`). `DELETE /api/v1/admin/devices/:id` archiviert ein Device, `PUT /api/v1/admin/devices/:id/restore` stellt es wieder her und `DELETE /api/v1/admin/devices/:id/permanent` löscht ein bereits archiviertes Device endgültig. Archivierte Devices werden nicht in Scans, Lagerbeständen, Cases, Picklisten, Labels oder operativen Kennzahlen berücksichtigt. Beim Archivieren eines Produkts werden dessen aktive Devices atomar mitarchiviert; beim Wiederherstellen werden nur die durch dieses Produktarchiv deaktivierten Devices reaktiviert.

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

### Cases & Handling Units

Die bisherigen `/cases`-Endpunkte bleiben kompatibel. Die UI verwendet zusätzlich `/handling-units`, um Euroboxen, Flightcases und Kits als echte Handling Units abzubilden.

| Methode | Pfad                                                        | Beschreibung |
|---------|-------------------------------------------------------------|--------------|
| `GET`   | `/api/v1/handling-units`                                    | Cases mit Typ, Workflow, Ort und Inhaltskennzahlen |
| `GET`   | `/api/v1/handling-unit-models`                              | Case-Modelle und Anzahl physischer Exemplare |
| `POST`  | `/api/v1/handling-units`                                    | Dynamisches, festes oder hybrides Case erstellen |
| `GET`   | `/api/v1/handling-units/scan?scan_code=…`                   | Case über Barcode, RFID oder Case-ID finden |
| `GET`   | `/api/v1/handling-units/:id/inventory`                      | Ist-Inhalt, Soll-Inhalt und Untercases |
| `POST`  | `/api/v1/handling-units/:id/inventory/scan`                 | Gerät, Mengenartikel oder Untercase einpacken |
| `POST`  | `/api/v1/handling-units/:id/template`                       | Soll-Inhalt eines festen/hybriden Cases setzen |
| `POST`  | `/api/v1/handling-units/:id/seal`                           | Vollständigkeit prüfen und Case versiegeln |
| `POST`  | `/api/v1/handling-units/:id/dispatch`                       | Case samt verschachteltem Inhalt an Job ausgeben |
| `POST`  | `/api/v1/handling-units/:id/return`                         | Case in Rücklauf/Prüfung übernehmen |
| `POST`  | `/api/v1/handling-units/:id/unpack`                         | Gesamten Inhalt kontrolliert auf Lagerplatz zurückbuchen |
| `GET`   | `/api/v1/handling-units/:id/events`                         | Audit-Verlauf der Case-Bewegungen |

### LED-Steuerung

| Methode | Pfad                                | Beschreibung                              |
|---------|-------------------------------------|-------------------------------------------|
| `GET`   | `/api/v1/led/status`                | LED-Status abrufen (🔒)                   |
| `POST`  | `/api/v1/led/highlight`             | Job-Bins hervorheben (🔒)                 |
| `POST`  | `/api/v1/led/clear`                 | Alle LEDs löschen (🔒)                    |
| `POST`  | `/api/v1/led/identify`              | LEDs identifizieren (🔒)                  |
| `POST`  | `/api/v1/led/test`                  | Bin testen (🔒)                           |
| `POST`  | `/api/v1/led/locate`                | Bin orten (🔒)                            |

### Datentransfer

| Methode | Pfad                                          | Beschreibung |
|---------|-----------------------------------------------|--------------|
| `GET`   | `/api/v1/admin/data-transfer/catalog`         | Datensätze, Felder, Typen und Importfähigkeit auflisten (🔒 Admin/Manager) |
| `POST`  | `/api/v1/admin/data-transfer/export`          | Feldauswahl als CSV oder XLSX exportieren (🔒 Admin/Manager) |
| `POST`  | `/api/v1/admin/data-transfer/import/preview`  | CSV/XLSX prüfen, Überschriften zuordnen und Konflikte ermitteln (🔒 Admin) |
| `POST`  | `/api/v1/admin/data-transfer/import`          | Geprüfte Zeilen atomar anlegen oder nach Feldregeln zusammenführen (🔒 Admin) |

Importdateien sind auf 10 MiB, 100 Spalten und 5.000 Datenzeilen begrenzt.
Beziehungen wie Kategorie, Produkt, Hersteller oder Lagerbereich werden über
exakte ID beziehungsweise exakten Namen aufgelöst; unbekannte Referenzen
blockieren den Import bereits in der Vorschau. Schreibvorgänge laufen in einer
Transaktion, sodass ein Fehler keine teilweise importierte Datei hinterlässt.

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
| `POST`   | `/api/v1/labels/pdf`                    | Auswahl als maßhaltige Einzelseiten oder A4-Etikettenbogen herunterladen (🔒) |
| `GET`    | `/api/v1/labels/printers`               | Druckerprofile auflisten (🔒)             |
| `POST`   | `/api/v1/labels/printers`               | Zebra-Netzwerkdrucker anlegen (🔒)        |
| `PUT`    | `/api/v1/labels/printers/:id`           | Druckerprofil aktualisieren (🔒)          |
| `DELETE` | `/api/v1/labels/printers/:id`           | Druckerprofil löschen (🔒)                |
| `POST`   | `/api/v1/labels/print`                  | Labels direkt als ZPL drucken (🔒)        |
| `GET`    | `/api/v1/labels/print-jobs`             | Druckaufträge und Fehler abrufen (🔒)     |

Das Label Studio verwendet für Geräte, Kabel, Cases und Lagerzonen jeweils getrennte Templates und ein eigenes Standardtemplate. Im Kabelbereich werden ausschließlich Datensätze aus `cable_products` angeboten; normale Produkte erscheinen dort nicht. Bei „Neu erzeugen“ wird pro Ziel ein maßhaltiges, einseitiges PDF als Master gespeichert und der vorherige Stand überschrieben; PNG-Dateien werden nicht gespeichert. Export, Browserdruck und ZPL-Direktdruck verwenden diesen Cache und rendern nur fehlende oder veraltete Labels neu. Ein Label gilt als veraltet, sobald sich sein Quelldatensatz, das gewählte Template oder dessen Revision ändert. Der PDF-Export führt die gespeicherten Master entsprechend der individuellen Stückzahlen entweder zu einer Mehrseiten-PDF oder maßhaltig auf A4-Bögen zusammen. Der A4-Modus berechnet Zeilen und Spalten aus Templategröße, Seitenrand und Abständen; zu große Kombinationen werden abgelehnt, statt Labels unbemerkt zu skalieren. Für Direktdruck wird im Studio ein aktiver Zebra-kompatibler Netzwerkdrucker mit IP/Hostname, TCP-Port (üblicherweise `9100`) und Auflösung (`203`, `300` oder `600` DPI) hinterlegt.

`POST /api/v1/labels/pdf` und `POST /api/v1/labels/print` akzeptieren die
rückwärtskompatiblen Felder `target_ids`/`copies` oder `items` als Liste aus
`target_id` und `copies`. Für PDF setzt `layout=a4_sheet` den Bogenmodus;
`orientation`, `margin_mm`, `horizontal_gap_mm`, `vertical_gap_mm` und
`show_guides` steuern die Ausgabe. `layout=single` erzeugt weiterhin eine
maßhaltige PDF-Seite je Label.

Die Migration `034_label_studio_and_direct_print.sql` ergänzt Templates um Zieltyp und Revision und legt `label_assets`, `label_printers` sowie `label_print_jobs` an. `035_cable_labels_and_pdf_export.sql` benennt das Standardtemplate konsistent für Kabel. `036_pdf_label_cache.sql` verwirft alte PNG-Cachepfade; die Dateien werden beim Start gezielt aus dem Label-Cache entfernt und bei Bedarf als PDF neu erzeugt. Die Migrationen werden beim WarehouseCore-Start idempotent angewendet.

Die Migration `038_device_status_lifecycle.sql` trennt Jobabschluss und physische Rückgabe. Ausgegebene Geräte abgeschlossener oder stornierter Jobs wechseln zu `return_pending`, bis ein Einlagerungsscan Lagerplatz und Rückgabe bestätigt. Nicht belegbare alte `on_job`-Werte werden je nach bekanntem Lagerkontext zu `in_storage` oder `location_unknown` normalisiert.

Die Migration `039_warehouse_operations_and_handling_units.sql` erweitert Lagerorte um Prozessrollen, Betriebszustände, Kapazitäts- und Inventursteuerung. Sie ergänzt dynamische/feste/hybride Handling Units, Mengen- und Untercase-Inhalte, Ereignisprotokolle, Lageraufgaben und scannerbasierte Zählinventuren. Die Migration verändert keine bestehenden Geräte-Lagerplatzzuordnungen automatisch.

Die Migration `041_product_master_v2.sql` ergänzt das unveränderliche Kennungssystem, Scan-Aliase, Produktklassen, technische Artikelnummern, typisierte Beziehungen, konkrete Device-Komponenten und Case-Modelle. Bestehende Produkte, Devices und Cases werden idempotent nachgezogen. Vorhandene Device-IDs bleiben erhalten; bisherige Cases mit Geräteinhalt werden als feste Cases klassifiziert und ihr Ist-Inhalt als Soll-Inhalt übernommen. Kabelprodukte werden automatisch unter „Kabel & Adapter“ einsortiert.

Die Migration `044_device_lifecycle.sql` ergänzt den aktiven bzw. archivierten Device-Zustand, den Archivzeitpunkt und die Herkunft eines Produktarchivs. Dadurch bleiben historische Verknüpfungen erhalten, während archivierte Geräte zuverlässig aus allen operativen Abläufen verschwinden.

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
| `PROCUREMENTCORE_PUBLIC_URL` | Öffentliche ProcurementCore-URL                  | –                      |
| `DASHBOARD_URL`         | Öffentliche URL des zentralen Cores-Dashboards         | automatisch erkannt    |

---

[Quellcode](https://github.com/nbt4/warehousecore) | [Monorepo](https://github.com/nbt4/cores) | `nobentie/warehousecore:latest`

# Release 5.9.72

Der Release führt die vollständige Main-Historie wieder mit der zentralen
Login-Weiterleitung aus `5.9.71` zusammen. Dadurch sind insbesondere die
ProcurementCore-Verknüpfung, die kontextuelle Suche über technische Parameter,
die Wartungsabläufe, automatische Lagerplatz-Barcodes und die bestätigten
Ausgabe-Workflows wieder gemeinsam enthalten. Das Image wird ausschließlich
aus diesem zusammengeführten Main-Stand gebaut.
