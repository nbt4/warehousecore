package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"warehousecore/internal/middleware"
)

func recordCatalogAudit(tx *sql.Tx, r *http.Request, entity, id, action string, after any) error {
	var userID any
	if user, ok := middleware.GetUserFromContext(r); ok {
		userID = user.UserID
	}
	encoded, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO audit_log(user_id,entity_type,entity_id,action,old_values,new_values) VALUES($1,$2,$3,$4,'{}'::jsonb,$5::jsonb)`, userID, entity, id, action, string(encoded))
	return err
}
