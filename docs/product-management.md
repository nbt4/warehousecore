# Produktmanagement · 5.9.115

Der Produktkatalog ist ein eigener Arbeitsbereich mit stabilen URLs. Produkte,
Geräte, Kabel, Pakete, Website, Beschaffung, Fremdmietartikel und Stammdaten sind
über die Sidebar erreichbar. Die Produktseite trennt Stammdaten, Bestand,
Zubehör/Packregeln, Website, Beschaffung und Verlauf. Die alten unmontierten
Editoren wurden entfernt. Manager lesen; Admin und Warehouse-Admin bearbeiten.

## Bilder und Website

- Der Browser optimiert auf Wunsch JPEG/PNG/WebP vor dem Upload auf maximal
  2560 Pixel Kantenlänge; Transparenz bleibt erhalten. Das ist eine ausdrückliche
  Checkbox: deaktivieren, um Originaldateien zu behalten. Nicht unterstützte
  Formate bleiben unverändert. Maximal 20 Dateien zu je 10 MiB pro Upload.
- Drei parallele Uploads; zufällige Dateinamen verhindern Kollisionen.
  Nextcloud-Verzeichnisse werden fünf Minuten zwischengespeichert und nach einem
  externen Löschen beim nächsten Upload erneut angelegt.
- Originale liegen nach dem Upload im lokalen Cache. Thumbnail und Vorschau
  entstehen aus einem gemeinsamen Decode; höchstens zwei Bilder werden
  gleichzeitig verarbeitet. Maximal 40 Megapixel werden dekodiert. WebP ist
  Standard; JPEG bleibt verfügbar. HEIC bleibt bei fehlendem Decoder das Original.
- Listen haben 30 Sekunden Cache; Upload/Deletion invalidieren ihn. Varianten
  werden atomar geschrieben. Löschen und laufende Downloads sind serialisiert.
- Vorhandene Bildordner behalten ihre bisherigen Pfade. Neue Produkte/Pakete
  bekommen dauerhaft getrennte Ordner unabhängig vom Anzeigenamen.
- Uploads ändern weder die veröffentlichte Galerie noch das Titelbild. Der
  Website-Editor verwaltet Auswahl, Reihenfolge, Titelbild und Hervorhebung.
  Öffentlicher Titel/Beschreibung sind von internen Produktnotizen getrennt.
- „Entwurf speichern“, „Veröffentlichen“ und „Ausblenden“ sind getrennte Aktionen.
  Revisionen verhindern das Überschreiben einer inzwischen geänderten Auswahl.
  Die Bildbibliothek ist intern; öffentliche Bildrouten liefern ausschließlich
  veröffentlichte, ausgewählte Bilder aktiver Artikel.
- Bildlöschungen entfernen Referenzen in einer Datenbanktransaktion und merken
  die Entfernung in Nextcloud dauerhaft vor. Fehlgeschlagene Remote-Löschungen
  werden alle 30 Sekunden wiederholt; vorgemerkte Bilder werden nicht angeboten.
- Website-Aktualisierung ist eine persistente Hintergrundwarteschlange mit
  Generationen, 10-Sekunden-Timeout, Wiederholung und sichtbarem Status. Eine
  während einer laufenden Anfrage gespeicherte Änderung bleibt ausstehend.
  Fehlende Konfiguration erscheint als „nicht konfiguriert“; Speicherung ist
  kein Beweis, dass ein Website-Cache bereits aktualisiert wurde.

`TSWEB_REVALIDATE_URL` und `TSWEB_REVALIDATE_SECRET` behalten ihren Vertrag:
POST mit JSON `{"path":"/products"}` und Bearer-Authentifizierung. Die automatische
Aktualisierung läuft nur bei vollständiger Konfiguration. Der Link „Website
öffnen“ führt zum öffentlichen Mietpark; es wird kein eigener Detail-Slug erfunden.

## Bestand, Geräte, Pakete und Stammdaten

Geräte sind global und innerhalb ihres Produkts suchbar und paginiert. Der
Editor zeigt technische Zustände, Seriennummer, Betriebsstunden, Wartungsdaten,
Lagerort, Etikett und Statushistorie. Eine Geräteversion schützt vor parallelem
Überschreiben. Operative Orts-/Jobänderungen bleiben im Lagerworkflow.

Mengenbestände werden nach Lagerort und Case angezeigt. Eine Korrektur erfordert
Vorbestand und Begründung; die Einheit bestimmt, ob Dezimalmengen zulässig sind.
Eine normale Produktbearbeitung schreibt keinen veralteten Gesamtbestand zurück.

Kabel besitzen eigene technische Eingaben für Typ, Anschlüsse, Länge und
Querschnitt sowie einen Link zur gemeinsamen Produkt-/Bestandsseite. Beide
Kabel-Bestandsmodi bleiben mit dem Produkt synchron. Neue Kabel sind Zubehör mit
Produktart Kabel und der passenden Bestandsführung.

Pakete trennen Stammdaten/Inhalte von Website-Veröffentlichung. Aliasnamen haben
je eine Zeile; optionale Paketartikel bleiben bei Bearbeitung erhalten. Archiv
und Wiederherstellung nutzen bestehende Versions- und Buchungsprüfungen.

Stammdaten zeigen die dreistufige Kategorienhierarchie sowie Hersteller, Marken,
Einheiten, Kabeltypen und Anschlüsse. Änderungen vergleichen die Datensatzversion;
verwendete Einträge können nicht unkontrolliert archiviert oder verschoben werden.
Kategorien können für ausgewählte Produkte atomar zugewiesen werden; dabei werden
Unterkategorien ausdrücklich geleert. Suchfilter sind als persönliche Ansichten
speicherbar. Fremdmietartikel trennen Lieferanten, Einkauf und Kundenpreis; ihr
Aktivstatus wird bearbeitet, ohne den Artikel aus der Historie zu löschen.
Stammdaten und Zusatzfelder werden bei Updates gemeinsam gespeichert; eine
veraltete Version oder ungültige Zusatzfelder lassen den bestehenden Artikel erhalten.

Ungespeicherte Produkt-, Paket- und Website-Eingaben sind auch bei Browser-Zurück
und Tabwechsel geschützt. Bestätigtes Verwerfen stellt die gespeicherten Eingaben
wieder her; verspätete Upload-/Speicherantworten überschreiben diesen Zustand nicht.

## Packregeln und Materialstand

Beziehungen haben Typ (`required`, `recommended`, `included`, `consumes`,
`compatible`, `alternative`), Bezug (`product`, `device`, `case`), Menge und Notiz.
Bearbeiten, Archivieren und Wiederherstellen erhalten ihre Identität. Die bereits
bestehende Datenbankprüfung gegen Änderungen an Regeln aktiver Jobs bleibt aktiv.

Packlisten-PDF und Scanansicht verwenden denselben gespeicherten Materialstand:
Produktpositionen plus ausschließlich deren `manual_quantity`, zusätzlich
alleinstehender manueller Bedarf. Regeln werden beim ersten Materialstand
festgehalten. Mengen-/Case-Änderungen erzeugen eine neue Version, erhalten aber die
Regeln und vorhandene Scans; neue Wurzelprodukte ergänzen den gespeicherten Graph.
PDF-Dateinamen enthalten die Materialversion, sodass eine Mengenänderung keine
alte PDF unter demselben Stand zurückliefert. Hinweise stehen auch im PDF.

Pflicht-/Verbrauchs-/Enthalten-Regeln werden bis Tiefe fünf erweitert; Empfehlungen,
Kompatibilität und Alternativen werden nicht automatisch Pflichtmaterial.
Case-Regeln zählen die tatsächlich zugeordneten Cases des jeweiligen Produkts
und berücksichtigen dieselbe Regel pro Case nur einmal bei mehreren Herkunftspfaden.
Fehlende Case-Zuordnung bleibt ein sichtbarer Hinweis. Kleine positive Mengen
werden auf die unterstützte Genauigkeit von 0,001 aufgerundet.

Einzelgeräte haben einen eigenen Scanbeleg und werden für den Job reserviert.
Doppelte Geräte zählen einmal. Geräte anderer aktiver Jobs, ausgegebene Geräte,
archivierte oder gesperrte Geräte werden abgewiesen. Physisch zugeordnete
`device_components` erfüllen Enthalten-Regeln erst, wenn ihr tatsächliches
Elterngerät gescannt wird, einschließlich verschachtelter Komponenten. Alle Teile
unterliegen denselben Verfügbarkeitsprüfungen; der Vorgang ist atomar.

Mengenartikel werden mit Menge und wiederholbarem Beleg bestätigt. Das dokumentiert
den Packstand; es ersetzt keine Lagerbewegung. Scans verwenden keine erfundenen
Positions-Fremdschlüssel. Übermengen oder nicht mehr benötigte Zuordnungen werden
als Hinweise gezeigt. Buchungsänderungen löschen keine bereits gespeicherten Scans.

RentalCore 5.3.125 übermittelt Typ und Bezug der Beziehungen. Sein älterer
Vorschlagsdialog erhält ausschließlich erforderliche/empfohlene/verbrauchte
Beziehungen mit Produkt-/Gerätebezug; Case-Regeln, physisch enthaltene Geräte und
reine Informationen werden dort nicht als zusätzliche Auswahl angeboten.
Archivierte Warehouse-Einheiten werden aus Rental-Auswahllisten ausgeschlossen.

## API und Datenbank

Die vorhandenen Produkt-/Paket-/Bild-URLs bleiben verwendbar. Hinzu kommen
`/admin/products/{id}/website` (GET), `/admin/website-status` und Retry,
Stock/History/Dependency-Preview, Beziehung-PUT/Restore, `/admin/catalog-master`,
`/admin/products/bulk-categories`. Listen unterstützen serverseitige Pagination
und `X-Total-Count`. Native Website-PUT akzeptiert `action` und `expected_revision`;
ältere Payloads ohne Aktion behalten ihre Veröffentlichungskompatibilität.

`migrations/064_product_workspace.sql` entspricht dem Startup-Schema einschließlich
Material-Snapshot/-Scan-Tabellen. Umbrella-Mirror: `051_product_workspace.sql`.
Neue Tabellen sind additiv; es werden keine Produktionsdaten als Testdaten genutzt.

## Prüfung

Go-Regressionen umfassen parallele Bilder, MIME-Manipulation, gelöschte Ordner,
Download/Delete-Rennen, haltbare Veröffentlichung, manuelle Mengen, Regeln,
verschachtelte Komponenten, Fremdjob-Konflikte, Belegwiederholung, Versionskonflikte
und atomare Kategorienzuweisung. Datenbankläufe benötigen ausdrücklich eine
isolierte Datenbank mit Namen auf `_test` (`WAREHOUSE_TEST_DATABASE_URL`).

Browserprüfung mit ausschließlich synthetischen Daten: alle Hauptansichten bei
390/768/1280/1536 px in Hell/Dunkel; Fokus, Escape, Leserechte, Leer-/Fehlerzustand.
Bildtest: 4096×3072 JPEG, 8.005.692 → 3.377.996 Bytes (2560×1920); Transparenz bleibt
erhalten, HEIC-Fallback bleibt unverändert und Multipart enthält tatsächlich Dateien.
Die rund 58 % Einsparung ist ein synthetischer Test, keine garantierte Zeitmessung
für Produktionsverbindungen.

Zusätzliche Browserregressionen: Zurück abbrechen/verwerfen, Verwerfen bei
weiter montierter Form, erneute Änderung nach Verwerfen sowie verspätete
Uploadantwort nach Verwerfen im selben Website-Panel: erfolgreich.
