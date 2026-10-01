package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type warehouseMasterLifecycleRequest struct {
	ID                int64  `json:"id"`
	ExpectedUpdatedAt string `json:"expected_updated_at"`
	ConfirmLifecycle  bool   `json:"confirm_lifecycle"`
	ConfirmationText  string `json:"confirmation_text"`
	Preview           bool   `json:"preview"`
}

func WarehouseMasterLifecycleMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID == 0 || !user.IsAdmin || !isWarehouseMCPMutation(r) {
		respondJSON(w, 403, map[string]string{"error": "Signed-in warehouse administrator and guided MCP origin required"})
		return
	}
	entity, op := mux.Vars(r)["entity"], mux.Vars(r)["operation"]
	table, key := "manufacturer", "manufacturerid"
	if entity == "brand" {
		table, key = "brands", "brandid"
	} else if entity != "manufacturer" {
		respondJSON(w, 400, map[string]string{"error": "Unsupported master entity"})
		return
	}
	if op != "archive" && op != "restore" {
		respondJSON(w, 400, map[string]string{"error": "Unsupported lifecycle operation"})
		return
	}
	var in warehouseMasterLifecycleRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid bounded lifecycle body"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF || in.ID <= 0 || in.ID > math.MaxInt32 {
		respondJSON(w, 400, map[string]string{"error": "Exact positive master ID and one JSON object required"})
		return
	}
	preview := in.Preview || !in.ConfirmLifecycle
	phrase := fmt.Sprintf("%s WAREHOUSE %s %d", strings.ToUpper(op), strings.ToUpper(entity), in.ID)
	if !preview && (in.ExpectedUpdatedAt == "" || in.ConfirmationText != phrase) {
		respondJSON(w, 428, map[string]string{"error": "Exact version and record-bound lifecycle confirmation are required"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='15s'`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var receipt int64
	if !preview {
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, entity+"."+op, map[string]any{"id": in.ID, "expected_updated_at": in.ExpectedUpdatedAt, "confirmation_text": phrase})
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	// Freeze active references, including writers from the existing Core UI.
	if _, err = tx.Exec(`LOCK TABLE manufacturer,brands IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE products IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	fields := `'website',website`
	if entity == "brand" {
		fields = `'manufacturer_id',manufacturerid`
	}
	query := `SELECT jsonb_build_object('id',` + key + `,'name',name,` + fields + `,'lifecycle_status',lifecycle_status,'archived_at',archived_at,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM ` + table + ` WHERE ` + key + `=$1 FOR UPDATE`
	var raw []byte
	if err = tx.QueryRow(query, in.ID).Scan(&raw); err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Master record not found"})
		return
	} else if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	current := map[string]any{}
	if err = json.Unmarshal(raw, &current); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	version := current["updated_at"].(string)
	required := []string{}
	if in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != version {
		required = append(required, "expected_updated_at")
	}
	wanted, target := "active", "archived"
	if op == "restore" {
		wanted, target = target, wanted
	}
	if current["lifecycle_status"] != wanted {
		required = append(required, "lifecycle_status")
	}
	var activeProducts, allProducts, activeBrands, allBrands int64
	depsQuery := `SELECT count(*) FILTER(WHERE lifecycle_status='active'),count(*) FROM products WHERE manufacturerid=$1`
	if entity == "brand" {
		depsQuery = `SELECT count(*) FILTER(WHERE lifecycle_status='active'),count(*) FROM products WHERE brandid=$1`
	}
	if err = tx.QueryRow(depsQuery, in.ID).Scan(&activeProducts, &allProducts); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if entity == "manufacturer" {
		if err = tx.QueryRow(`SELECT count(*) FILTER(WHERE lifecycle_status='active'),count(*) FROM brands WHERE manufacturerid=$1`, in.ID).Scan(&activeBrands, &allBrands); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
	}
	if op == "archive" && (activeProducts > 0 || activeBrands > 0) {
		required = append(required, "active_dependencies")
	}
	if op == "restore" {
		name, _ := current["name"].(string)
		if strings.TrimSpace(name) == "" || len([]rune(name)) > 255 {
			required = append(required, "invalid_retained_name")
		}
		if entity == "manufacturer" {
			if website, ok := current["website"].(string); ok {
				if _, valid := normalizedManufacturerWebsite(&website); !valid {
					required = append(required, "invalid_retained_website")
				}
			}
		} else if parent, ok := current["manufacturer_id"].(float64); ok {
			var active bool
			if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM manufacturer WHERE manufacturerid=$1 AND lifecycle_status='active')`, int64(parent)).Scan(&active); err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
			if !active {
				required = append(required, "active_manufacturer_required")
			}
		}
		var duplicate bool
		duplicateQuery := `SELECT EXISTS(SELECT 1 FROM manufacturer WHERE manufacturerid<>$1 AND lower(trim(name))=lower(trim($2)))`
		args := []any{in.ID, name}
		if entity == "brand" {
			duplicateQuery = `SELECT EXISTS(SELECT 1 FROM brands WHERE brandid<>$1 AND lower(trim(name))=lower(trim($2)) AND manufacturerid IS NOT DISTINCT FROM $3::int)`
			args = append(args, current["manufacturer_id"])
		}
		if err = tx.QueryRow(duplicateQuery, args...).Scan(&duplicate); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if duplicate {
			required = append(required, "duplicate_retained_identity")
		}
	}
	proposed := map[string]any{}
	for k, v := range current {
		proposed[k] = v
	}
	proposed["lifecycle_status"] = target
	result := map[string]any{"operation_status": "confirmation_required", "current": current, "draft": proposed, "dependencies": map[string]any{"active_products": activeProducts, "all_products": allProducts, "active_brands": activeBrands, "all_brands": allBrands}, "diff": map[string]any{"lifecycle_status": map[string]any{"before": wanted, "after": target}}, "expected_updated_at": version, "required_confirmation_text": phrase, "ready_to_execute": len(required) == 0, "required_fields": required, "preview": true}
	if preview || len(required) > 0 {
		if len(required) > 0 {
			result["operation_status"] = "needs_input"
		}
		respondJSON(w, 200, result)
		return
	}
	var updated string
	err = tx.QueryRow(`UPDATE `+table+` SET lifecycle_status=$1::text,archived_at=CASE WHEN $1::text='archived' THEN clock_timestamp() AT TIME ZONE 'UTC' ELSE NULL END WHERE `+key+`=$2 RETURNING to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`, target, in.ID).Scan(&updated)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = tx.QueryRow(query, in.ID).Scan(&raw); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = json.Unmarshal(raw, &proposed); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	oldJSON, _ := json.Marshal(current)
	newJSON, _ := json.Marshal(map[string]any{"origin": "MCP/AI", "after": proposed, "updated_at": updated})
	var auditID int64
	if err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7) RETURNING id`, user.UserID, entity+"."+op, entity, fmt.Sprint(in.ID), string(oldJSON), string(newJSON), r.UserAgent()).Scan(&auditID); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	result = map[string]any{"operation_status": op + "d", entity: proposed, "audit_id": auditID, "updated_at": updated}
	if op == "restore" {
		result["operation_status"] = "restored"
	}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, 200, result)
}
