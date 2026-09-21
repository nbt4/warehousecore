package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"warehousecore/internal/middleware"
)

// resolveProductMasterInputs resolves exact normalized master-data names or
// creates the missing records inside the product transaction. Any later
// product validation or insert failure therefore rolls the whole chain back.
func resolveProductMasterInputs(tx *sql.Tx, r *http.Request, product *Product) error {
	if product.ManufacturerID == nil {
		name := optionalInput(product.ManufacturerNameInput)
		if name != "" {
			var id int
			err := tx.QueryRow(`SELECT manufacturerid FROM manufacturer WHERE LOWER(TRIM(name))=LOWER(TRIM($1))`, name).Scan(&id)
			if err == sql.ErrNoRows {
				website := optionalInput(product.ManufacturerWebsiteInput)
				err = tx.QueryRow(`INSERT INTO manufacturer(name,website) VALUES($1,NULLIF($2,'')) RETURNING manufacturerid`, name, website).Scan(&id)
				if err == nil {
					err = recordMasterDataAudit(tx, r, "manufacturer", id, map[string]any{"name": name, "website": website})
				}
			}
			if err != nil {
				return fmt.Errorf("Hersteller konnte nicht aufgelöst oder angelegt werden: %w", err)
			}
			product.ManufacturerID = &id
		}
	}

	if product.CategoryID == nil {
		name := optionalInput(product.CategoryNameInput)
		if name != "" {
			var id int
			err := tx.QueryRow(`SELECT categoryid FROM categories WHERE LOWER(TRIM(name))=LOWER(TRIM($1))`, name).Scan(&id)
			if err == sql.ErrNoRows {
				abbreviation := strings.ToUpper(optionalInput(product.CategoryAbbrInput))
				if abbreviation == "" {
					return fmt.Errorf("für eine neue Kategorie ist category_abbreviation_input erforderlich")
				}
				err = tx.QueryRow(`INSERT INTO categories(name,abbreviation) VALUES($1,$2) RETURNING categoryid`, name, abbreviation).Scan(&id)
				if err == nil {
					err = recordMasterDataAudit(tx, r, "category", id, map[string]any{"name": name, "abbreviation": abbreviation})
				}
			}
			if err != nil {
				return fmt.Errorf("Kategorie konnte nicht aufgelöst oder angelegt werden: %w", err)
			}
			product.CategoryID = &id
		}
	}

	if product.SubcategoryID == nil {
		name := optionalInput(product.SubcategoryNameInput)
		if name != "" {
			if product.CategoryID == nil {
				return fmt.Errorf("eine neue Unterkategorie benötigt eine aufgelöste Kategorie")
			}
			var id string
			err := tx.QueryRow(`SELECT subcategoryid FROM subcategories WHERE categoryid=$1 AND LOWER(TRIM(name))=LOWER(TRIM($2))`, *product.CategoryID, name).Scan(&id)
			if err == sql.ErrNoRows {
				abbreviation := strings.ToUpper(optionalInput(product.SubcategoryAbbrInput))
				err = tx.QueryRow(`INSERT INTO subcategories(subcategoryid,name,abbreviation,categoryid) VALUES(gen_random_uuid()::varchar,$1,$2,$3) RETURNING subcategoryid`, name, abbreviation, *product.CategoryID).Scan(&id)
				if err == nil {
					err = recordMasterDataAudit(tx, r, "subcategory", id, map[string]any{"name": name, "abbreviation": abbreviation, "category_id": *product.CategoryID})
				}
			}
			if err != nil {
				return fmt.Errorf("Unterkategorie konnte nicht aufgelöst oder angelegt werden: %w", err)
			}
			product.SubcategoryID = &id
		}
	}

	if product.SubbiercategoryID == nil {
		name := optionalInput(product.ThirdCategoryNameInput)
		if name != "" {
			if product.SubcategoryID == nil {
				return fmt.Errorf("eine neue Kategorie der dritten Ebene benötigt eine aufgelöste Unterkategorie")
			}
			var id string
			err := tx.QueryRow(`SELECT subbiercategoryid FROM subbiercategories WHERE subcategoryid=$1 AND LOWER(TRIM(name))=LOWER(TRIM($2))`, *product.SubcategoryID, name).Scan(&id)
			if err == sql.ErrNoRows {
				abbreviation := strings.ToUpper(optionalInput(product.ThirdCategoryAbbrInput))
				err = tx.QueryRow(`INSERT INTO subbiercategories(subbiercategoryid,name,abbreviation,subcategoryid) VALUES(gen_random_uuid()::varchar,$1,$2,$3) RETURNING subbiercategoryid`, name, abbreviation, *product.SubcategoryID).Scan(&id)
				if err == nil {
					err = recordMasterDataAudit(tx, r, "third_category", id, map[string]any{"name": name, "abbreviation": abbreviation, "subcategory_id": *product.SubcategoryID})
				}
			}
			if err != nil {
				return fmt.Errorf("Kategorie der dritten Ebene konnte nicht aufgelöst oder angelegt werden: %w", err)
			}
			product.SubbiercategoryID = &id
		}
	}

	if product.BrandID == nil {
		name := optionalInput(product.BrandNameInput)
		if name != "" {
			var id int
			var manufacturerID sql.NullInt64
			err := tx.QueryRow(`SELECT brandid,manufacturerid FROM brands WHERE LOWER(TRIM(name))=LOWER(TRIM($1))`, name).Scan(&id, &manufacturerID)
			if err == sql.ErrNoRows {
				err = tx.QueryRow(`INSERT INTO brands(name,manufacturerid) VALUES($1,$2) RETURNING brandid`, name, product.ManufacturerID).Scan(&id)
				if err == nil {
					values := map[string]any{"name": name}
					if product.ManufacturerID != nil {
						values["manufacturer_id"] = *product.ManufacturerID
					}
					err = recordMasterDataAudit(tx, r, "brand", id, values)
				}
			} else if err == nil && manufacturerID.Valid {
				if product.ManufacturerID == nil {
					value := int(manufacturerID.Int64)
					product.ManufacturerID = &value
				} else if *product.ManufacturerID != int(manufacturerID.Int64) {
					return fmt.Errorf("die bestehende Marke gehört zu einem anderen Hersteller")
				}
			}
			if err != nil {
				return fmt.Errorf("Marke konnte nicht aufgelöst oder angelegt werden: %w", err)
			}
			product.BrandID = &id
		}
	}

	return nil
}

func optionalInput(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func recordMasterDataAudit(tx *sql.Tx, r *http.Request, entityType string, entityID any, values map[string]any) error {
	var userID any
	if user, ok := middleware.GetUserFromContext(r); ok {
		userID = user.UserID
	}
	encoded, _ := json.Marshal(values)
	ipAddress := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		ipAddress = host
	}
	if len(ipAddress) > 45 {
		ipAddress = ipAddress[:45]
	}
	_, err := tx.Exec(`
		INSERT INTO audit_log (user_id,action,entity_type,entity_id,old_values,new_values,ip_address,user_agent)
		VALUES ($1,'master_data.create',$2,$3,NULL,$4::jsonb,$5,$6)
	`, userID, entityType, fmt.Sprint(entityID), string(encoded), ipAddress, r.UserAgent())
	return err
}
