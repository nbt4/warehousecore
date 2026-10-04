package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"

	"github.com/gorilla/mux"
)

// Native removal requires a current authenticated warehouse user. AI
// delegation must use the separately confirmed administrator lifecycle API.
func archiveNativeCase(w http.ResponseWriter, r *http.Request) {
	if isWarehouseDelegatedRequest(r) {
		respondJSON(w, 403, map[string]string{"error": "Use the reviewed owning-Core MCP case lifecycle"})
		return
	}
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID == 0 {
		respondJSON(w, 403, map[string]string{"error": "Signed-in warehouse user required"})
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil || id <= 0 {
		respondJSON(w, 400, map[string]string{"error": "Exact case identity required"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='15s'`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var active bool
	if err = tx.QueryRow(`SELECT is_active FROM users WHERE userid=$1 FOR SHARE`, user.UserID).Scan(&active); err != nil || !active {
		respondJSON(w, 403, map[string]string{"error": "Current active user required"})
		return
	}
	if _, err = tx.Exec(`LOCK TABLE cases,case_events,inventory_identifiers IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE devices,devicescases,case_product_contents,case_child_contents,case_content_templates,warehouse_tasks,jobs,status IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	before, err := caseMCPSnapshot(tx, id)
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Case not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if before["lifecycle_status"] == "archived" {
		respondJSON(w, 200, map[string]any{"message": "Case bereits archiviert", "case": before})
		return
	}
	deps, err := caseMCPDependencies(tx, id)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if hasDeviceDependencies(deps) {
		respondJSON(w, 409, map[string]any{"error": "Case zuerst entpacken, zurücknehmen und offene Aufgaben abschließen", "dependencies": deps})
		return
	}
	if _, err = tx.Exec(`INSERT INTO case_events(case_id,event_type,metadata) VALUES($1,'archive',jsonb_build_object('origin','UI','actor_id',$2::bigint))`, id, user.UserID); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if _, err = tx.Exec(`UPDATE cases SET lifecycle_status='archived' WHERE caseid=$1`, id); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	after, err := caseMCPSnapshot(tx, id)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	oldRaw, _ := json.Marshal(before)
	newRaw, _ := json.Marshal(map[string]any{"origin": "UI", "after": after, "updated_at": after["updated_at"]})
	var audit int64
	if err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,'case.archive','case',$2,$3::jsonb,$4::jsonb,$5) RETURNING id`, user.UserID, fmt.Sprint(id), string(oldRaw), string(newRaw), r.UserAgent()).Scan(&audit); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, 200, map[string]any{"message": "Case archiviert; Identität und Historie bleiben erhalten", "case": after, "audit_id": audit})
}
