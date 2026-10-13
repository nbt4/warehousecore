package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"strings"
	"warehousecore/internal/repository"
)

type catalogMasterDefinition struct{ table, id, parent, parentTable, parentID, abbreviation, website, usage string }

var catalogMasterDefinitions = map[string]catalogMasterDefinition{
	"categories":       {"categories", "categoryid", "", "", "", "abbreviation", "", `SELECT count(*) FROM products WHERE categoryid=m.categoryid AND lifecycle_status='active'`},
	"subcategories":    {"subcategories", "subcategoryid", "categoryid", "categories", "categoryid", "abbreviation", "", `SELECT count(*) FROM products WHERE subcategoryid=m.subcategoryid AND lifecycle_status='active'`},
	"third-categories": {"subbiercategories", "subbiercategoryid", "subcategoryid", "subcategories", "subcategoryid", "abbreviation", "", `SELECT count(*) FROM products WHERE subbiercategoryid=m.subbiercategoryid AND lifecycle_status='active'`},
	"manufacturers":    {"manufacturer", "manufacturerid", "", "", "", "", "website", `SELECT count(*) FROM products WHERE manufacturerid=m.manufacturerid AND lifecycle_status='active'`},
	"brands":           {"brands", "brandid", "manufacturerid", "manufacturer", "manufacturerid", "", "", `SELECT count(*) FROM products WHERE brandid=m.brandid AND lifecycle_status='active'`},
	"units":            {"count_types", "count_type_id", "", "", "", "abbreviation", "", `SELECT count(*) FROM products WHERE count_type_id=m.count_type_id AND lifecycle_status='active'`},
	"cable-types":      {"cable_types", "cable_typesid", "", "", "", "", "", `SELECT count(*) FROM cable_products cp JOIN products p ON p.productid=cp.product_id WHERE cp.cable_type_id=m.cable_typesid AND p.lifecycle_status='active'`},
	"connectors":       {"cable_connectors", "cable_connectorsid", "", "", "", "abbreviation", "", `SELECT count(*) FROM cable_products cp JOIN products p ON p.productid=cp.product_id WHERE (cp.connector_a_id=m.cable_connectorsid OR cp.connector_b_id=m.cable_connectorsid) AND p.lifecycle_status='active'`},
}

func GetCatalogMaster(w http.ResponseWriter, r *http.Request) {
	definition, ok := catalogMasterDefinitions[mux.Vars(r)["entity"]]
	if !ok {
		respondJSON(w, 404, map[string]string{"error": "Unknown master data"})
		return
	}
	parent := "NULL"
	parentName := "NULL"
	abbreviation := "NULL"
	website := "NULL"
	gender := "NULL"
	if definition.parent != "" {
		parent = "m." + definition.parent
		parentName = fmt.Sprintf("(SELECT name FROM %s p WHERE p.%s=m.%s)", definition.parentTable, definition.parentID, definition.parent)
	}
	if definition.abbreviation != "" {
		abbreviation = "m." + definition.abbreviation
	}
	if definition.website != "" {
		website = "m." + definition.website
	}
	if mux.Vars(r)["entity"] == "connectors" {
		gender = "m.gender"
	}
	query := fmt.Sprintf(`SELECT jsonb_build_object('id',m.%s,'name',m.name,'parent_id',%s,'parent_name',%s,'abbreviation',%s,'website',%s,'gender',%s,'is_decimal',COALESCE((to_jsonb(m)->>'is_decimal')::boolean,false),'lifecycle_status',m.lifecycle_status,'product_count',(%s),'version',to_char(m.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM %s m WHERE ($1='all' OR m.lifecycle_status=$1) ORDER BY m.name`, definition.id, parent, parentName, abbreviation, website, gender, definition.usage, definition.table)
	lifecycle := r.URL.Query().Get("lifecycle_status")
	if lifecycle != "all" && lifecycle != "archived" {
		lifecycle = "active"
	}
	rows, err := repository.GetSQLDB().QueryContext(r.Context(), query, lifecycle)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Stammdaten konnten nicht geladen werden"})
		return
	}
	defer rows.Close()
	result := []json.RawMessage{}
	for rows.Next() {
		var data []byte
		if rows.Scan(&data) != nil {
			respondJSON(w, 500, map[string]string{"error": "Stammdaten konnten nicht gelesen werden"})
			return
		}
		result = append(result, json.RawMessage(data))
	}
	if rows.Err() != nil {
		respondJSON(w, 500, map[string]string{"error": "Stammdaten konnten nicht geladen werden"})
		return
	}
	respondJSON(w, 200, result)
}
func CatalogMasterLifecycle(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	definition, ok := catalogMasterDefinitions[vars["entity"]]
	if !ok {
		respondJSON(w, 404, map[string]string{"error": "Unknown master data"})
		return
	}
	var input struct {
		Version string `json:"expected_updated_at"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.Version == "" {
		respondJSON(w, 400, map[string]string{"error": "Aktuelle Stammdatenversion erforderlich"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Stammdaten konnten nicht gespeichert werden"})
		return
	}
	defer tx.Rollback()
	var version, current string
	query := fmt.Sprintf(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),lifecycle_status FROM %s WHERE %s::text=$1 FOR UPDATE`, definition.table, definition.id)
	if err = tx.QueryRow(query, vars["id"]).Scan(&version, &current); err != nil {
		respondJSON(w, 404, map[string]string{"error": "Stammdatensatz nicht gefunden"})
		return
	}
	if version != input.Version {
		respondJSON(w, 409, map[string]string{"error": "Stammdaten wurden inzwischen verändert. Bitte neu laden."})
		return
	}
	target := "archived"
	if vars["operation"] == "restore" {
		target = "active"
	}
	if target == current {
		respondJSON(w, 409, map[string]string{"error": "Stammdatensatz ist bereits in diesem Zustand"})
		return
	}
	// Every table is selected from a fixed allowlist; identifiers never come from input.
	fields := "lifecycle_status=$1,updated_at=clock_timestamp() AT TIME ZONE 'UTC'"
	query = fmt.Sprintf("UPDATE %s SET %s WHERE %s::text=$2", definition.table, fields, definition.id)
	if _, err = tx.Exec(query, target, vars["id"]); err != nil {
		message := "Stammdaten werden noch verwendet und können nicht archiviert werden"
		if strings.Contains(err.Error(), "Restore") {
			message = "Stammdaten zuerst wiederherstellen"
		}
		respondJSON(w, 409, map[string]string{"error": message})
		return
	}
	if err = recordCatalogAudit(tx, r, vars["entity"], vars["id"], "catalog.master."+vars["operation"], map[string]any{"lifecycle_status": target}); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Änderung konnte nicht protokolliert werden"})
		return
	}
	if tx.Commit() != nil {
		respondJSON(w, 500, map[string]string{"error": "Stammdaten konnten nicht gespeichert werden"})
		return
	}
	respondJSON(w, 200, map[string]string{"lifecycle_status": target})
}

func SaveCatalogMaster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	definition, ok := catalogMasterDefinitions[vars["entity"]]
	if !ok {
		respondJSON(w, 404, map[string]string{"error": "Unknown master data"})
		return
	}
	var input struct {
		Name         string  `json:"name"`
		Abbreviation string  `json:"abbreviation"`
		ParentID     *string `json:"parent_id"`
		Website      *string `json:"website"`
		Gender       string  `json:"gender"`
		IsDecimal    bool    `json:"is_decimal"`
		Version      string  `json:"expected_updated_at"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input) != nil {
		respondJSON(w, 400, map[string]string{"error": "Ungültige Stammdaten"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Abbreviation = strings.TrimSpace(input.Abbreviation)
	maxName := 255
	if vars["entity"] == "units" {
		maxName = 50
	}
	if vars["entity"] == "connectors" || vars["entity"] == "cable-types" {
		maxName = 100
	}
	if len([]rune(input.Name)) == 0 || len([]rune(input.Name)) > maxName {
		respondJSON(w, 400, map[string]string{"error": "Name fehlt oder ist zu lang"})
		return
	}
	if definition.abbreviation != "" {
		max := 10
		if vars["entity"] == "categories" || vars["entity"] == "subcategories" || vars["entity"] == "third-categories" {
			max = 3
			input.Abbreviation = strings.ToUpper(input.Abbreviation)
		}
		if len(input.Abbreviation) > max || (vars["entity"] == "categories" || vars["entity"] == "units") && input.Abbreviation == "" {
			respondJSON(w, 400, map[string]string{"error": "Abkürzung fehlt oder ist zu lang"})
			return
		}
	}
	if vars["entity"] == "manufacturers" {
		var valid bool
		input.Website, valid = normalizedManufacturerWebsite(input.Website)
		if !valid {
			respondJSON(w, 400, map[string]string{"error": "Website muss eine gültige HTTP(S)-Adresse sein"})
			return
		}
	}
	if len(input.Gender) > 20 {
		respondJSON(w, 400, map[string]string{"error": "Anschlussausführung ist zu lang"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Stammdaten konnten nicht gespeichert werden"})
		return
	}
	defer tx.Rollback()
	if definition.parent != "" && input.ParentID != nil && *input.ParentID != "" {
		var active bool
		query := fmt.Sprintf("SELECT lifecycle_status='active' FROM %s WHERE %s::text=$1 FOR SHARE", definition.parentTable, definition.parentID)
		if err = tx.QueryRow(query, *input.ParentID).Scan(&active); err != nil || !active {
			respondJSON(w, 409, map[string]string{"error": "Übergeordneten aktiven Stammdatensatz auswählen"})
			return
		}
	} else if vars["entity"] == "subcategories" || vars["entity"] == "third-categories" {
		respondJSON(w, 400, map[string]string{"error": "Übergeordnete Kategorie ist erforderlich"})
		return
	}
	creating := vars["id"] == ""
	id := vars["id"]
	action := "catalog.master.create"
	if !creating {
		action = "catalog.master.update"
		var version, lifecycle string
		var oldParent *string
		parent := "NULL::text"
		if definition.parent != "" {
			parent = definition.parent + "::text"
		}
		query := fmt.Sprintf(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),lifecycle_status,%s FROM %s WHERE %s::text=$1 FOR UPDATE`, parent, definition.table, definition.id)
		if err = tx.QueryRow(query, id).Scan(&version, &lifecycle, &oldParent); err != nil {
			respondJSON(w, 404, map[string]string{"error": "Stammdatensatz nicht gefunden"})
			return
		}
		if input.Version == "" || version != input.Version {
			respondJSON(w, 409, map[string]string{"error": "Stammdaten wurden inzwischen verändert. Bitte neu laden."})
			return
		}
		if lifecycle != "active" {
			respondJSON(w, 409, map[string]string{"error": "Archivierte Stammdaten zuerst wiederherstellen"})
			return
		}
		oldValue, newValue := "", ""
		if oldParent != nil {
			oldValue = *oldParent
		}
		if input.ParentID != nil {
			newValue = *input.ParentID
		}
		if definition.parent != "" && oldValue != newValue {
			usage := strings.ReplaceAll(definition.usage, " AND lifecycle_status='active'", "")
			usage = strings.ReplaceAll(usage, " AND p.lifecycle_status='active'", "")
			var used int
			query = fmt.Sprintf("SELECT (%s) FROM %s m WHERE m.%s::text=$1", usage, definition.table, definition.id)
			if err = tx.QueryRow(query, id).Scan(&used); err != nil {
				respondJSON(w, 500, map[string]string{"error": "Verwendung konnte nicht geprüft werden"})
				return
			}
			if used > 0 {
				respondJSON(w, 409, map[string]string{"error": "Verwendete Stammdaten können nicht direkt verschoben werden. Produkte zuerst umkategorisieren."})
				return
			}
		}
	}
	columns := []string{"name"}
	values := []any{input.Name}
	if definition.abbreviation != "" {
		columns = append(columns, definition.abbreviation)
		values = append(values, input.Abbreviation)
	}
	if definition.website != "" {
		columns = append(columns, definition.website)
		values = append(values, input.Website)
	}
	if definition.parent != "" {
		columns = append(columns, definition.parent)
		values = append(values, input.ParentID)
	}
	if vars["entity"] == "connectors" {
		columns = append(columns, "gender")
		values = append(values, input.Gender)
	}
	if vars["entity"] == "units" {
		columns = append(columns, "is_decimal")
		values = append(values, input.IsDecimal)
	}
	if creating {
		placeholders := make([]string, len(values))
		for i := range values {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		if vars["entity"] == "subcategories" || vars["entity"] == "third-categories" {
			columns = append(columns, definition.id)
			placeholders = append(placeholders, "gen_random_uuid()::varchar")
		}
		query := fmt.Sprintf("INSERT INTO %s(%s) VALUES(%s) RETURNING %s::text", definition.table, strings.Join(columns, ","), strings.Join(placeholders, ","), definition.id)
		err = tx.QueryRow(query, values...).Scan(&id)
	} else {
		fields := make([]string, 0, len(columns))
		for i, column := range columns {
			fields = append(fields, fmt.Sprintf("%s=$%d", column, i+1))
		}
		values = append(values, id)
		query := fmt.Sprintf("UPDATE %s SET %s WHERE %s::text=$%d", definition.table, strings.Join(fields, ","), definition.id, len(values))
		_, err = tx.Exec(query, values...)
	}
	if err == nil {
		err = recordCatalogAudit(tx, r, vars["entity"], id, action, input)
	}
	if err != nil {
		respondJSON(w, 409, map[string]string{"error": "Stammdaten konnten nicht gespeichert werden. Namen, Zuordnung und vorhandene Einträge prüfen."})
		return
	}
	if tx.Commit() != nil {
		respondJSON(w, 500, map[string]string{"error": "Stammdaten konnten nicht gespeichert werden"})
		return
	}
	status := http.StatusOK
	if creating {
		status = http.StatusCreated
	}
	respondJSON(w, status, map[string]string{"id": id})
}
