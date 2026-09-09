package services

import (
	"bytes"
	"database/sql"
	"fmt"
	"html/template"
	"math"
	"strings"
)

// PackingList contains the printable snapshot of a job.
type PackingList struct {
	JobID       int
	JobCode     string
	Title       string
	Customer    string
	StartDate   string
	EndDate     string
	CompanyName string
	LogoDataURI string
	BarcodeURI  string
	Items       []PackingListItem
}

// PackingListItem is one product or recursively expanded accessory row.
type PackingListItem struct {
	ProductID int
	Name      string
	Quantity  float64
	Unit      string
	Depth     int
	Accessory bool
}

// LoadPackingList loads job metadata and expands product dependencies up to
// five levels. A dependency inherits the multiplied quantity of its parent.
func LoadPackingList(db *sql.DB, jobID int) (*PackingList, error) {
	result := &PackingList{JobID: jobID}
	err := db.QueryRow(`
		SELECT COALESCE(NULLIF(j.job_code, ''), 'JOB' || LPAD(j.jobid::text, 4, '0')),
		       COALESCE(j.description, ''),
		       TRIM(COALESCE(c.firstname, '') || ' ' || COALESCE(c.lastname, '')),
		       COALESCE(TO_CHAR(j.startdate, 'DD.MM.YYYY'), ''),
		       COALESCE(TO_CHAR(j.enddate, 'DD.MM.YYYY'), '')
		FROM jobs j
		JOIN customers c ON c.customerid = j.customerid
		WHERE j.jobid = $1 AND j.deleted_at IS NULL
	`, jobID).Scan(&result.JobCode, &result.Title, &result.Customer, &result.StartDate, &result.EndDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("job %d not found", jobID)
		}
		return nil, fmt.Errorf("load job %d: %w", jobID, err)
	}

	rows, err := db.Query(`
		WITH RECURSIVE roots AS (
			SELECT jp.product_id, SUM(jp.quantity)::numeric AS quantity,
			       MIN(jp.unit) AS unit, MIN(jp.sort_order) AS sort_order
			FROM job_positions jp
			WHERE jp.job_id = $1 AND jp.position_type = 'product' AND jp.product_id IS NOT NULL
			GROUP BY jp.product_id
			UNION ALL
			SELECT jpr.product_id, jpr.quantity::numeric, 'Stück', 100000 + jpr.id::int
			FROM job_product_requirements jpr
			WHERE jpr.job_id = $1
			  AND NOT EXISTS (
				SELECT 1 FROM job_positions jp
				WHERE jp.job_id = jpr.job_id AND jp.product_id = jpr.product_id
			  )
		), tree AS (
			SELECT r.sort_order AS root_order, p.productid, p.name, r.quantity,
			       COALESCE(NULLIF(r.unit, ''), 'Stück') AS unit, 0 AS depth,
			       ARRAY[p.productid] AS path, COALESCE(p.is_accessory, false) AS accessory
			FROM roots r
			JOIN products p ON p.productid = r.product_id
			UNION ALL
			SELECT t.root_order, child.productid, child.name,
			       t.quantity * COALESCE(pd.default_quantity, 1), 'Stück', t.depth + 1,
			       t.path || child.productid, true
			FROM tree t
			JOIN product_dependencies pd ON pd.product_id = t.productid
			JOIN products child ON child.productid = pd.dependency_product_id
			WHERE t.depth < 5 AND NOT child.productid = ANY(t.path)
		)
		SELECT productid, name, quantity::float8, unit, depth, accessory
		FROM tree
		ORDER BY root_order, path
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("load packing-list products for job %d: %w", jobID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var item PackingListItem
		if err := rows.Scan(&item.ProductID, &item.Name, &item.Quantity, &item.Unit, &item.Depth, &item.Accessory); err != nil {
			return nil, fmt.Errorf("read packing-list product: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read packing-list products: %w", err)
	}
	return result, nil
}

var packingListTemplate = template.Must(template.New("packing-list").Funcs(template.FuncMap{
	"dataURI": func(value string) template.URL {
		if !strings.HasPrefix(value, "data:image/") {
			return ""
		}
		return template.URL(value)
	},
	"indent": func(depth int) template.CSS {
		if depth < 0 {
			depth = 0
		}
		if depth > 5 {
			depth = 5
		}
		return template.CSS(fmt.Sprintf("padding-left: %.1fmm", float64(depth)*6))
	},
	"quantity": func(value float64) string {
		if math.Abs(value-math.Round(value)) < 0.00001 {
			return fmt.Sprintf("%.0f", value)
		}
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
	},
}).Parse(`<!doctype html>
<html lang="de"><head><meta charset="utf-8"><style>
@page { size: A4; margin: 14mm; }
* { box-sizing: border-box; }
body { margin: 0; color: #161616; font: 10pt Arial, sans-serif; }
header { display: flex; justify-content: space-between; align-items: flex-start; border-bottom: 2px solid #d5202f; padding-bottom: 6mm; }
.logo { max-width: 58mm; max-height: 18mm; object-fit: contain; }
h1 { margin: 0 0 1.5mm; font-size: 22pt; } .muted { color: #606060; }
.meta { display: grid; grid-template-columns: 1fr 1fr; gap: 3mm 8mm; margin: 7mm 0; }
.label { font-size: 8pt; color: #6c6c6c; text-transform: uppercase; }
.barcode { width: 75mm; height: 15mm; object-fit: fill; margin-top: 2mm; }
table { border-collapse: collapse; width: 100%; }
th { border-bottom: 1px solid #333; text-align: left; padding: 2.5mm 2mm; font-size: 8pt; text-transform: uppercase; }
td { border-bottom: 1px solid #ddd; padding: 2.7mm 2mm; vertical-align: top; }
.qty { width: 25mm; text-align: right; white-space: nowrap; }
.accessory { color: #505050; } .accessory::before { content: "↳ "; color: #d5202f; }
.empty { padding: 12mm 0; text-align: center; color: #666; }
footer { margin-top: 8mm; display: flex; justify-content: space-between; font-size: 8pt; color: #777; }
</style></head><body>
<header><div>{{if .LogoDataURI}}<img class="logo" src="{{dataURI .LogoDataURI}}" alt="{{.CompanyName}}">{{else}}<strong>{{.CompanyName}}</strong>{{end}}</div>
<div><div class="label">Packliste</div><h1>{{.JobCode}}</h1>{{if .BarcodeURI}}<img class="barcode" src="{{dataURI .BarcodeURI}}" alt="Barcode {{.JobCode}}">{{end}}</div></header>
<section class="meta"><div><div class="label">Jobtitel</div><strong>{{if .Title}}{{.Title}}{{else}}—{{end}}</strong></div>
<div><div class="label">Kunde</div><strong>{{if .Customer}}{{.Customer}}{{else}}—{{end}}</strong></div>
<div><div class="label">Zeitraum</div><span>{{if .StartDate}}{{.StartDate}}{{else}}—{{end}}{{if .EndDate}} – {{.EndDate}}{{end}}</span></div>
<div><div class="label">Job-ID</div><span>{{.JobID}}</span></div></section>
<table><thead><tr><th>Produkt / Zubehör</th><th class="qty">Menge</th></tr></thead><tbody>
{{range .Items}}<tr><td style="{{indent .Depth}}" class="{{if or .Accessory (gt .Depth 0)}}accessory{{end}}">{{.Name}}</td><td class="qty">{{quantity .Quantity}} {{.Unit}}</td></tr>{{else}}<tr><td colspan="2" class="empty">Keine Produkte hinterlegt.</td></tr>{{end}}
</tbody></table><footer><span>{{.CompanyName}}</span><span>Automatisch aus WarehouseCore erzeugt</span></footer>
<script>Promise.all(Array.from(document.images).map(function(img){return img.complete ? Promise.resolve() : new Promise(function(resolve){img.onload=img.onerror=resolve;});})).then(function(){window.pdfReady=true;});</script>
</body></html>`))

// RenderPackingListHTML creates a self-contained, safely escaped print document.
func RenderPackingListHTML(data *PackingList) (string, error) {
	var output bytes.Buffer
	if err := packingListTemplate.Execute(&output, data); err != nil {
		return "", fmt.Errorf("render packing-list HTML: %w", err)
	}
	return output.String(), nil
}
