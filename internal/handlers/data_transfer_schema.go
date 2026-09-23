package handlers

import (
	"fmt"
	"strings"
)

const (
	maxTransferFileBytes = 10 << 20
	maxTransferRows      = 5000
	maxTransferColumns   = 100
)

type transferField struct {
	Key             string            `json:"key"`
	Label           string            `json:"label"`
	Type            string            `json:"type"`
	Writable        bool              `json:"writable"`
	Required        bool              `json:"required,omitempty"`
	DefaultSelected bool              `json:"default_selected,omitempty"`
	Aliases         []string          `json:"-"`
	Expression      string            `json:"-"`
	Column          string            `json:"-"`
	Relation        *transferRelation `json:"-"`
}

type transferRelation struct {
	Table      string
	IDColumn   string
	NameColumn string
}

type transferMatch struct {
	Field  string
	Column string
	Exact  bool
}

type transferDataset struct {
	Key             string          `json:"key"`
	Label           string          `json:"label"`
	Description     string          `json:"description"`
	ImportSupported bool            `json:"import_supported"`
	Table           string          `json:"-"`
	Alias           string          `json:"-"`
	PrimaryKey      string          `json:"-"`
	From            string          `json:"-"`
	Where           string          `json:"-"`
	OrderBy         string          `json:"-"`
	Fields          []transferField `json:"fields"`
	Matches         []transferMatch `json:"-"`
}

type transferCatalogResponse struct {
	Datasets   []transferDataset `json:"datasets"`
	Formats    []string          `json:"formats"`
	Delimiters []string          `json:"delimiters"`
	MaxRows    int               `json:"max_rows"`
}

var transferDatasets = []transferDataset{
	{
		Key: "products", Label: "Produkte", Description: "Artikelstamm einschließlich Klassifikation, Preisen, Abmessungen und Beständen.", ImportSupported: true,
		Table: "products", Alias: "p", PrimaryKey: "productid",
		From: `products p
			LEFT JOIN categories c ON c.categoryid=p.categoryid
			LEFT JOIN subcategories sc ON sc.subcategoryid=p.subcategoryid
			LEFT JOIN subbiercategories tc ON tc.subbiercategoryid=p.subbiercategoryid
			LEFT JOIN manufacturer m ON m.manufacturerid=p.manufacturerid
			LEFT JOIN brands b ON b.brandid=p.brandid`,
		OrderBy: "p.name",
		Matches: []transferMatch{{Field: "product_id", Column: "productid", Exact: true}, {Field: "product_code", Column: "product_code"}, {Field: "barcode", Column: "generic_barcode"}, {Field: "name", Column: "name"}},
		Fields: []transferField{
			{Key: "product_id", Label: "Produkt-ID", Type: "integer", DefaultSelected: true, Expression: "p.productid::text"},
			{Key: "product_code", Label: "Produktcode", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(p.product_code,'')", Column: "product_code"},
			{Key: "name", Label: "Name", Type: "string", Writable: true, Required: true, DefaultSelected: true, Expression: "p.name", Column: "name"},
			{Key: "description", Label: "Beschreibung", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(p.description,'')", Column: "description"},
			{Key: "category", Label: "Kategorie", Type: "relation", Writable: true, DefaultSelected: true, Expression: "COALESCE(c.name,'')", Column: "categoryid", Relation: &transferRelation{Table: "categories", IDColumn: "categoryid", NameColumn: "name"}},
			{Key: "subcategory", Label: "Unterkategorie", Type: "relation", Writable: true, Expression: "COALESCE(sc.name,'')", Column: "subcategoryid", Relation: &transferRelation{Table: "subcategories", IDColumn: "subcategoryid", NameColumn: "name"}},
			{Key: "third_category", Label: "Dritte Kategorieebene", Type: "relation", Writable: true, Expression: "COALESCE(tc.name,'')", Column: "subbiercategoryid", Relation: &transferRelation{Table: "subbiercategories", IDColumn: "subbiercategoryid", NameColumn: "name"}},
			{Key: "manufacturer", Label: "Hersteller", Type: "relation", Writable: true, DefaultSelected: true, Expression: "COALESCE(m.name,'')", Column: "manufacturerid", Relation: &transferRelation{Table: "manufacturer", IDColumn: "manufacturerid", NameColumn: "name"}},
			{Key: "brand", Label: "Marke", Type: "relation", Writable: true, DefaultSelected: true, Expression: "COALESCE(b.name,'')", Column: "brandid", Relation: &transferRelation{Table: "brands", IDColumn: "brandid", NameColumn: "name"}},
			{Key: "product_kind", Label: "Artikelart", Type: "string", Writable: true, Expression: "COALESCE(p.product_kind,'standard')", Column: "product_kind"},
			{Key: "product_type", Label: "Produkttyp", Type: "string", Writable: true, Expression: "COALESCE(p.product_type,'equipment')", Column: "product_type"},
			{Key: "tracking_mode", Label: "Bestandsführung", Type: "string", Writable: true, Expression: "COALESCE(p.tracking_mode,'individual')", Column: "tracking_mode"},
			{Key: "lifecycle_status", Label: "Lebenszyklusstatus", Type: "string", Writable: true, Expression: "COALESCE(p.lifecycle_status,'active')", Column: "lifecycle_status"},
			{Key: "model_number", Label: "Modellnummer", Type: "string", Writable: true, Expression: "COALESCE(p.model_number,'')", Column: "model_number"},
			{Key: "manufacturer_part_number", Label: "Hersteller-Artikelnummer", Type: "string", Writable: true, Expression: "COALESCE(p.manufacturer_part_number,'')", Column: "manufacturer_part_number"},
			{Key: "ean", Label: "EAN", Type: "string", Writable: true, Expression: "COALESCE(p.ean,'')", Column: "ean"},
			{Key: "barcode", Label: "Barcode", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(p.generic_barcode,'')", Column: "generic_barcode"},
			{Key: "daily_cost", Label: "Kosten pro Tag", Type: "decimal", Writable: true, DefaultSelected: true, Aliases: []string{"Kosten/Tag", "item_cost_per_day"}, Expression: "COALESCE(p.itemcostperday::text,'')", Column: "itemcostperday"},
			{Key: "price_per_unit", Label: "Stückpreis", Type: "decimal", Writable: true, Expression: "COALESCE(p.price_per_unit::text,'')", Column: "price_per_unit"},
			{Key: "stock_quantity", Label: "Bestand", Type: "decimal", Writable: true, DefaultSelected: true, Expression: "COALESCE(p.stock_quantity::text,'')", Column: "stock_quantity"},
			{Key: "min_stock_level", Label: "Mindestbestand", Type: "decimal", Writable: true, Expression: "COALESCE(p.min_stock_level::text,'')", Column: "min_stock_level"},
			{Key: "maintenance_interval", Label: "Wartungsintervall", Type: "integer", Writable: true, Expression: "COALESCE(p.maintenanceinterval::text,'')", Column: "maintenanceinterval"},
			{Key: "weight", Label: "Gewicht (kg)", Type: "decimal", Writable: true, Expression: "COALESCE(p.weight::text,'')", Column: "weight"},
			{Key: "height", Label: "Höhe (cm)", Type: "decimal", Writable: true, Expression: "COALESCE(p.height::text,'')", Column: "height"},
			{Key: "width", Label: "Breite (cm)", Type: "decimal", Writable: true, Expression: "COALESCE(p.width::text,'')", Column: "width"},
			{Key: "depth", Label: "Tiefe (cm)", Type: "decimal", Writable: true, Expression: "COALESCE(p.depth::text,'')", Column: "depth"},
			{Key: "power_consumption", Label: "Stromverbrauch (W)", Type: "decimal", Writable: true, Expression: "COALESCE(p.powerconsumption::text,'')", Column: "powerconsumption"},
			{Key: "is_accessory", Label: "Zubehör", Type: "boolean", Writable: true, Expression: "COALESCE(p.is_accessory,false)::text", Column: "is_accessory"},
			{Key: "is_consumable", Label: "Verbrauchsmaterial", Type: "boolean", Writable: true, Expression: "COALESCE(p.is_consumable,false)::text", Column: "is_consumable"},
			{Key: "website_visible", Label: "Auf Webseite sichtbar", Type: "boolean", Writable: true, Expression: "COALESCE(p.website_visible,false)::text", Column: "website_visible"},
			{Key: "device_count", Label: "Geräte gesamt", Type: "integer", Expression: "(SELECT COUNT(*) FROM devices dc WHERE dc.productid=p.productid AND dc.lifecycle_status='active')::text"},
			{Key: "available_count", Label: "Geräte verfügbar", Type: "integer", Expression: "(SELECT COUNT(*) FROM devices dc WHERE dc.productid=p.productid AND dc.lifecycle_status='active' AND dc.status='in_storage' AND dc.condition_status='available')::text"},
			{Key: "in_use_count", Label: "Geräte im Einsatz", Type: "integer", Expression: "(SELECT COUNT(*) FROM devices dc WHERE dc.productid=p.productid AND dc.lifecycle_status='active' AND dc.status='on_job')::text"},
			{Key: "defect_count", Label: "Geräte nicht einsatzbereit", Type: "integer", Expression: "(SELECT COUNT(*) FROM devices dc WHERE dc.productid=p.productid AND dc.lifecycle_status='active' AND dc.condition_status IN ('defective','blocked','maintenance'))::text"},
		},
	},
	{
		Key: "devices", Label: "Geräte", Description: "Einzelgeräte mit Produktzuordnung, Identifikatoren, Zustand und Lagerort.", ImportSupported: true,
		Table: "devices", Alias: "d", PrimaryKey: "deviceid",
		From:    `devices d LEFT JOIN products p ON p.productid=d.productid LEFT JOIN storage_zones z ON z.zone_id=d.zone_id LEFT JOIN cases ca ON ca.caseid=d.current_case_id`,
		OrderBy: "d.deviceid",
		Matches: []transferMatch{{Field: "device_id", Column: "deviceid", Exact: true}, {Field: "serial_number", Column: "serialnumber"}, {Field: "barcode", Column: "barcode"}},
		Fields: []transferField{
			{Key: "device_id", Label: "Geräte-ID", Type: "string", Writable: true, Required: true, DefaultSelected: true, Expression: "d.deviceid", Column: "deviceid"},
			{Key: "product", Label: "Produkt", Type: "relation", Writable: true, Required: true, DefaultSelected: true, Expression: "COALESCE(p.name,'')", Column: "productid", Relation: &transferRelation{Table: "products", IDColumn: "productid", NameColumn: "name"}},
			{Key: "serial_number", Label: "Seriennummer", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(d.serialnumber,'')", Column: "serialnumber"},
			{Key: "barcode", Label: "Gerätebarcode", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(d.barcode,'')", Column: "barcode"},
			{Key: "status", Label: "Status", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(d.status,'in_storage')", Column: "status"},
			{Key: "condition_status", Label: "Zustand", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(d.condition_status,'available')", Column: "condition_status"},
			{Key: "lifecycle_status", Label: "Lebenszyklusstatus", Type: "string", Writable: true, Expression: "COALESCE(d.lifecycle_status,'active')", Column: "lifecycle_status"},
			{Key: "purchase_date", Label: "Kaufdatum", Type: "date", Writable: true, Expression: "COALESCE(d.purchasedate::text,'')", Column: "purchasedate"},
			{Key: "last_maintenance", Label: "Letzte Wartung", Type: "date", Writable: true, Expression: "COALESCE(d.lastmaintenance::text,'')", Column: "lastmaintenance"},
			{Key: "next_maintenance", Label: "Nächste Wartung", Type: "date", Writable: true, Expression: "COALESCE(d.nextmaintenance::text,'')", Column: "nextmaintenance"},
			{Key: "zone", Label: "Lagerbereich", Type: "relation", Writable: true, Expression: "COALESCE(z.name,'')", Column: "zone_id", Relation: &transferRelation{Table: "storage_zones", IDColumn: "zone_id", NameColumn: "name"}},
			{Key: "case", Label: "Case", Type: "relation", Writable: true, Expression: "COALESCE(ca.name,'')", Column: "current_case_id", Relation: &transferRelation{Table: "cases", IDColumn: "caseid", NameColumn: "name"}},
			{Key: "current_location", Label: "Aktueller Ort", Type: "string", Writable: true, Expression: "COALESCE(d.current_location,'')", Column: "current_location"},
			{Key: "condition_rating", Label: "Zustandsbewertung", Type: "decimal", Writable: true, Expression: "COALESCE(d.condition_rating::text,'')", Column: "condition_rating"},
			{Key: "notes", Label: "Notizen", Type: "string", Writable: true, Expression: "COALESCE(d.notes,'')", Column: "notes"},
		},
	},
	{
		Key: "customers", Label: "Kontakte", Description: "Kunden und Lieferanten mit Adress- und Kontaktdaten.", ImportSupported: true,
		Table: "customers", Alias: "cu", PrimaryKey: "customerid", From: "customers cu", Where: "COALESCE(cu.is_archived,false)=false", OrderBy: "COALESCE(cu.companyname,cu.name,cu.lastname,cu.firstname)",
		Matches: []transferMatch{{Field: "customer_id", Column: "customerid", Exact: true}, {Field: "email", Column: "email"}},
		Fields: []transferField{
			{Key: "customer_id", Label: "Kontakt-ID", Type: "integer", DefaultSelected: true, Expression: "cu.customerid::text"},
			{Key: "name", Label: "Anzeigename", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(cu.name,'')", Column: "name"},
			{Key: "company_name", Label: "Firma", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(cu.companyname,'')", Column: "companyname"},
			{Key: "first_name", Label: "Vorname", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(cu.firstname,'')", Column: "firstname"},
			{Key: "last_name", Label: "Nachname", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(cu.lastname,'')", Column: "lastname"},
			{Key: "email", Label: "E-Mail", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(cu.email,'')", Column: "email"},
			{Key: "phone", Label: "Telefon", Type: "string", Writable: true, Expression: "COALESCE(cu.phonenumber,'')", Column: "phonenumber"},
			{Key: "street", Label: "Straße", Type: "string", Writable: true, Expression: "COALESCE(cu.street,'')", Column: "street"},
			{Key: "house_number", Label: "Hausnummer", Type: "string", Writable: true, Expression: "COALESCE(cu.housenumber,'')", Column: "housenumber"},
			{Key: "postal_code", Label: "Postleitzahl", Type: "string", Writable: true, Expression: "COALESCE(cu.zip,'')", Column: "zip"},
			{Key: "city", Label: "Stadt", Type: "string", Writable: true, Expression: "COALESCE(cu.city,'')", Column: "city"},
			{Key: "state", Label: "Bundesland", Type: "string", Writable: true, Expression: "COALESCE(cu.federalstate,'')", Column: "federalstate"},
			{Key: "country", Label: "Land", Type: "string", Writable: true, Expression: "COALESCE(cu.country,'')", Column: "country"},
			{Key: "contact_type", Label: "Kontakttyp", Type: "string", Writable: true, Expression: "COALESCE(cu.customertype,'')", Column: "customertype"},
			{Key: "is_customer", Label: "Ist Kunde", Type: "boolean", Writable: true, Expression: "COALESCE(cu.is_customer,true)::text", Column: "is_customer"},
			{Key: "is_supplier", Label: "Ist Lieferant", Type: "boolean", Writable: true, Expression: "COALESCE(cu.is_supplier,false)::text", Column: "is_supplier"},
			{Key: "notes", Label: "Notizen", Type: "string", Writable: true, Expression: "COALESCE(cu.notes,'')", Column: "notes"},
		},
	},
	{
		Key: "manufacturers", Label: "Hersteller", Description: "Herstellerstamm mit Website.", ImportSupported: true,
		Table: "manufacturer", Alias: "m", PrimaryKey: "manufacturerid", From: "manufacturer m", OrderBy: "m.name",
		Matches: []transferMatch{{Field: "manufacturer_id", Column: "manufacturerid", Exact: true}, {Field: "name", Column: "name"}},
		Fields: []transferField{
			{Key: "manufacturer_id", Label: "Hersteller-ID", Type: "integer", DefaultSelected: true, Aliases: []string{"ID"}, Expression: "m.manufacturerid::text"},
			{Key: "name", Label: "Name", Type: "string", Writable: true, Required: true, DefaultSelected: true, Expression: "m.name", Column: "name"},
			{Key: "website", Label: "Webseite", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(m.website,'')", Column: "website"},
		},
	},
	{
		Key: "brands", Label: "Marken", Description: "Markenstamm mit Hersteller-Zuordnung.", ImportSupported: true,
		Table: "brands", Alias: "b", PrimaryKey: "brandid", From: "brands b LEFT JOIN manufacturer m ON m.manufacturerid=b.manufacturerid", OrderBy: "b.name",
		Matches: []transferMatch{{Field: "brand_id", Column: "brandid", Exact: true}, {Field: "name", Column: "name"}},
		Fields: []transferField{
			{Key: "brand_id", Label: "Marken-ID", Type: "integer", DefaultSelected: true, Aliases: []string{"ID"}, Expression: "b.brandid::text"},
			{Key: "name", Label: "Name", Type: "string", Writable: true, Required: true, DefaultSelected: true, Aliases: []string{"Markenname"}, Expression: "b.name", Column: "name"},
			{Key: "manufacturer", Label: "Hersteller", Type: "relation", Writable: true, DefaultSelected: true, Expression: "COALESCE(m.name,'')", Column: "manufacturerid", Relation: &transferRelation{Table: "manufacturer", IDColumn: "manufacturerid", NameColumn: "name"}},
		},
	},
	{
		Key: "categories", Label: "Kategorien", Description: "Produktkategorien mit Abkürzung.", ImportSupported: true,
		Table: "categories", Alias: "c", PrimaryKey: "categoryid", From: "categories c", OrderBy: "c.name",
		Matches: []transferMatch{{Field: "category_id", Column: "categoryid", Exact: true}, {Field: "name", Column: "name"}},
		Fields: []transferField{
			{Key: "category_id", Label: "Kategorie-ID", Type: "integer", DefaultSelected: true, Expression: "c.categoryid::text"},
			{Key: "name", Label: "Name", Type: "string", Writable: true, Required: true, DefaultSelected: true, Expression: "c.name", Column: "name"},
			{Key: "abbreviation", Label: "Abkürzung", Type: "string", Writable: true, DefaultSelected: true, Expression: "COALESCE(c.abbreviation,'')", Column: "abbreviation"},
		},
	},
	{
		Key: "zones", Label: "Lagerbereiche", Description: "Lagerzonen, Barcodes und Kapazitäten.",
		Table: "storage_zones", Alias: "z", PrimaryKey: "zone_id", From: "storage_zones z", OrderBy: "z.name",
		Fields: []transferField{
			{Key: "zone_id", Label: "Zonen-ID", Type: "integer", DefaultSelected: true, Expression: "z.zone_id::text"},
			{Key: "name", Label: "Name", Type: "string", DefaultSelected: true, Expression: "z.name"},
			{Key: "type", Label: "Typ", Type: "string", DefaultSelected: true, Expression: "COALESCE(z.type::text,'')"},
			{Key: "barcode", Label: "Barcode", Type: "string", DefaultSelected: true, Expression: "COALESCE(z.barcode,'')"},
			{Key: "capacity", Label: "Kapazität", Type: "integer", DefaultSelected: true, Expression: "COALESCE(z.capacity::text,'')"},
			{Key: "location", Label: "Standort", Type: "string", Expression: "COALESCE(z.location,'')"},
			{Key: "description", Label: "Beschreibung", Type: "string", Expression: "COALESCE(z.description,'')"},
			{Key: "device_count", Label: "Anzahl Geräte", Type: "integer", Expression: "(SELECT COUNT(*) FROM devices dc WHERE dc.zone_id=z.zone_id AND dc.lifecycle_status='active')::text"},
		},
	},
	{
		Key: "cables", Label: "Kabel", Description: "Kabelartikel mit Steckern, Abmessungen und Bestand.",
		Table: "cable_products", Alias: "cp", PrimaryKey: "cable_product_id", From: `cable_products cp JOIN products p ON p.productid=cp.product_id LEFT JOIN cable_types ct ON ct.cable_typesid=cp.cable_type_id LEFT JOIN cable_connectors ca ON ca.cable_connectorsid=cp.connector_a_id LEFT JOIN cable_connectors cb ON cb.cable_connectorsid=cp.connector_b_id`, OrderBy: "p.name",
		Fields: []transferField{
			{Key: "cable_id", Label: "Kabel-ID", Type: "integer", DefaultSelected: true, Expression: "cp.cable_product_id::text"},
			{Key: "product_id", Label: "Produkt-ID", Type: "integer", Expression: "p.productid::text"},
			{Key: "name", Label: "Name", Type: "string", DefaultSelected: true, Expression: "p.name"},
			{Key: "cable_type", Label: "Kabeltyp", Type: "string", DefaultSelected: true, Expression: "COALESCE(ct.name,'')"},
			{Key: "connector_a", Label: "Stecker A", Type: "string", DefaultSelected: true, Expression: "COALESCE(ca.name,'')"},
			{Key: "connector_b", Label: "Stecker B", Type: "string", DefaultSelected: true, Expression: "COALESCE(cb.name,'')"},
			{Key: "length_m", Label: "Länge (m)", Type: "decimal", DefaultSelected: true, Expression: "COALESCE(cp.length_m::text,'')"},
			{Key: "cross_section_mm2", Label: "Querschnitt (mm²)", Type: "decimal", Expression: "COALESCE(cp.cross_section_mm2::text,'')"},
			{Key: "tracking_mode", Label: "Bestandsführung", Type: "string", Expression: "COALESCE(cp.tracking_mode,'')"},
			{Key: "stock_quantity", Label: "Bestand", Type: "decimal", DefaultSelected: true, Expression: "COALESCE(p.stock_quantity::text,'')"},
		},
	},
	{
		Key: "jobs", Label: "Jobs", Description: "Jobs mit Kunde, Zeitraum, Status und Umsatz.",
		Table: "jobs", Alias: "j", PrimaryKey: "jobid", From: `jobs j LEFT JOIN customers cu ON cu.customerid=j.customerid LEFT JOIN status s ON s.statusid=j.statusid`, Where: "j.deleted_at IS NULL", OrderBy: "j.startdate DESC",
		Fields: []transferField{
			{Key: "job_id", Label: "Job-ID", Type: "integer", DefaultSelected: true, Expression: "j.jobid::text"},
			{Key: "job_code", Label: "Job-Code", Type: "string", DefaultSelected: true, Expression: "COALESCE(j.job_code,'')"},
			{Key: "customer", Label: "Kunde", Type: "string", DefaultSelected: true, Expression: "COALESCE(cu.companyname,cu.name,TRIM(CONCAT_WS(' ',cu.firstname,cu.lastname)),'')"},
			{Key: "status", Label: "Status", Type: "string", DefaultSelected: true, Expression: "COALESCE(s.status,'')"},
			{Key: "start_date", Label: "Startdatum", Type: "date", DefaultSelected: true, Expression: "COALESCE(j.startdate::text,'')"},
			{Key: "end_date", Label: "Enddatum", Type: "date", DefaultSelected: true, Expression: "COALESCE(j.enddate::text,'')"},
			{Key: "description", Label: "Beschreibung", Type: "string", Expression: "COALESCE(j.description,'')"},
			{Key: "revenue", Label: "Umsatz", Type: "decimal", DefaultSelected: true, Expression: "COALESCE(j.revenue::text,'')"},
			{Key: "final_revenue", Label: "Finaler Umsatz", Type: "decimal", Expression: "COALESCE(j.final_revenue::text,'')"},
			{Key: "device_count", Label: "Anzahl Geräte", Type: "integer", Expression: "(SELECT COUNT(DISTINCT jd.deviceid) FROM job_devices jd WHERE jd.jobid=j.jobid)::text"},
		},
	},
}

func transferDatasetByKey(key string) (*transferDataset, bool) {
	for i := range transferDatasets {
		if transferDatasets[i].Key == key {
			return &transferDatasets[i], true
		}
	}
	return nil, false
}

func (dataset *transferDataset) fieldByKey(key string) (*transferField, bool) {
	for i := range dataset.Fields {
		if dataset.Fields[i].Key == key {
			return &dataset.Fields[i], true
		}
	}
	return nil, false
}

func normalizeTransferHeader(value string) string {
	value = strings.TrimPrefix(value, "\ufeff")
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", " ", "_", "-", "_", "/", "_", ".", "", "(", "", ")", "")
	return replacer.Replace(value)
}

func (dataset *transferDataset) resolveHeader(header string) (*transferField, bool) {
	normalized := normalizeTransferHeader(header)
	for i := range dataset.Fields {
		field := &dataset.Fields[i]
		if normalized == normalizeTransferHeader(field.Key) || normalized == normalizeTransferHeader(field.Label) {
			return field, true
		}
		for _, alias := range field.Aliases {
			if normalized == normalizeTransferHeader(alias) {
				return field, true
			}
		}
	}
	return nil, false
}

func quoteTransferIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func (dataset *transferDataset) selectQuery(fields []transferField, extraWhere string) string {
	selects := make([]string, 0, len(fields))
	for _, field := range fields {
		selects = append(selects, fmt.Sprintf("%s AS %s", field.Expression, quoteTransferIdentifier(field.Key)))
	}
	where := strings.TrimSpace(dataset.Where)
	if strings.TrimSpace(extraWhere) != "" {
		if where != "" {
			where = "(" + where + ") AND (" + extraWhere + ")"
		} else {
			where = extraWhere
		}
	}
	query := "SELECT " + strings.Join(selects, ", ") + " FROM " + dataset.From
	if where != "" {
		query += " WHERE " + where
	}
	if extraWhere == "" && dataset.OrderBy != "" {
		query += " ORDER BY " + dataset.OrderBy
	}
	return query
}
