package handlers

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
)

// Physical packing has a closed contract, separate from case metadata and
// expected template quantities. Movement drafts bind the entire physical tree,
// stock locations, product/device versions and current job reservations.
type warehouseCaseContentRequest struct {
	CaseID            int64    `json:"case_id"`
	DeviceID          string   `json:"device_id,omitempty"`
	ProductID         int64    `json:"product_id,omitempty"`
	ChildCaseID       int64    `json:"child_case_id,omitempty"`
	Quantity          *float64 `json:"quantity,omitempty"`
	SourceZoneID      int64    `json:"source_zone_id,omitempty"`
	DestinationZoneID int64    `json:"destination_zone_id,omitempty"`
	ExpectedUpdatedAt string   `json:"expected_updated_at,omitempty"`
	ExpectedContext   string   `json:"expected_context,omitempty"`
	ConfirmChange     bool     `json:"confirm_change,omitempty"`
	ConfirmationText  string   `json:"confirmation_text,omitempty"`
	Preview           bool     `json:"preview,omitempty"`
}

func validateCaseContentInput(op string, in warehouseCaseContentRequest) error {
	if in.CaseID <= 0 || in.CaseID > math.MaxInt32 || in.ProductID < 0 || in.ProductID > math.MaxInt32 || in.ChildCaseID < 0 || in.ChildCaseID > math.MaxInt32 || in.SourceZoneID < 0 || in.SourceZoneID > math.MaxInt32 || in.DestinationZoneID < 0 || in.DestinationZoneID > math.MaxInt32 {
		return fmt.Errorf("valid positive serial identities required")
	}
	if strings.TrimSpace(in.DeviceID) != in.DeviceID || len(in.DeviceID) > 128 {
		return fmt.Errorf("exact bounded device identity required")
	}
	if in.Quantity != nil {
		q := *in.Quantity
		if q <= 0 || q > 999999999.999 || math.IsNaN(q) || math.IsInf(q, 0) || math.Abs(q*1000-math.Round(q*1000)) > 0.00001 {
			return fmt.Errorf("positive quantity up to 999999999.999 with three decimals required")
		}
	}
	switch op {
	case "pack_device", "unpack_device":
		if in.DeviceID == "" || in.ProductID != 0 || in.ChildCaseID != 0 || in.Quantity != nil || in.SourceZoneID != 0 {
			return fmt.Errorf("device movement requires device_id only; source is resolved from the current device")
		}
	case "pack_product", "unpack_product":
		if in.ProductID == 0 || in.Quantity == nil || in.DeviceID != "" || in.ChildCaseID != 0 {
			return fmt.Errorf("quantity movement requires product_id and quantity")
		}
	case "pack_case", "unpack_case":
		if in.ChildCaseID == 0 || in.DeviceID != "" || in.ProductID != 0 || in.Quantity != nil || in.SourceZoneID != 0 {
			return fmt.Errorf("nested case movement requires child_case_id only")
		}
	case "unpack_all":
		if in.DeviceID != "" || in.ProductID != 0 || in.ChildCaseID != 0 || in.Quantity != nil || in.SourceZoneID != 0 {
			return fmt.Errorf("complete unpack preserves every physical item and requires destination_zone_id only")
		}
	default:
		return fmt.Errorf("unsupported physical case operation")
	}
	if strings.HasPrefix(op, "pack_") {
		if in.DestinationZoneID != 0 {
			return fmt.Errorf("packing destination is the reviewed case, not a supplied location")
		}
		if op == "pack_product" && in.SourceZoneID <= 0 {
			return fmt.Errorf("explicit source_zone_id required for quantity packing")
		}
	} else if in.DestinationZoneID <= 0 || in.SourceZoneID != 0 {
		return fmt.Errorf("explicit destination_zone_id required for unpacking")
	}
	return nil
}

// Every SQL identifier and physical item kind is fixed by this business path.
// No price, private employee data or unrestricted table content is selected.
func caseContentContext(tx *sql.Tx, in warehouseCaseContentRequest) (map[string]any, error) {
	root, err := inventoryMCPItem(tx, "case", fmt.Sprint(in.CaseID), 0)
	if err != nil {
		return nil, err
	}
	metadata, err := caseMCPSnapshot(tx, in.CaseID)
	if err != nil {
		return nil, err
	}
	root["metadata"] = metadata
	templates, err := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT cc.child_case_id FROM tree t JOIN case_child_contents cc ON cc.parent_case_id=t.case_id) SELECT jsonb_build_object('template_line_id',ct.template_line_id,'case_id',ct.case_id,'product_id',ct.product_id,'expected_quantity',ct.expected_quantity,'lifecycle_status',ct.lifecycle_status,'updated_at',ct.updated_at) FROM case_content_templates ct JOIN tree t ON t.case_id=ct.case_id ORDER BY ct.case_id,ct.template_line_id LIMIT 1001`, in.CaseID)
	if err != nil {
		return nil, err
	}
	root["templates"] = templates
	nesting, err := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT cc.child_case_id FROM tree t JOIN case_child_contents cc ON cc.parent_case_id=t.case_id) SELECT jsonb_build_object('parent_case_id',cc.parent_case_id,'child_case_id',cc.child_case_id,'created_at',cc.created_at) FROM case_child_contents cc WHERE cc.parent_case_id IN(SELECT case_id FROM tree) OR cc.child_case_id IN(SELECT case_id FROM tree) ORDER BY cc.parent_case_id,cc.child_case_id LIMIT 1001`, in.CaseID)
	if err != nil {
		return nil, err
	}
	root["nesting"] = nesting
	refs := map[string]any{"case": root}
	if in.DeviceID != "" {
		device, e := inventoryMCPDevice(tx, in.DeviceID)
		if e != nil {
			return nil, e
		}
		refs["device"] = device
	}
	if in.ProductID > 0 {
		product, e := inventoryMCPItem(tx, "product", fmt.Sprint(in.ProductID), in.SourceZoneID)
		if e != nil {
			return nil, e
		}
		refs["product"] = product
	}
	if in.ChildCaseID > 0 {
		child, e := inventoryMCPItem(tx, "case", fmt.Sprint(in.ChildCaseID), 0)
		if e != nil {
			return nil, e
		}
		meta, e := caseMCPSnapshot(tx, in.ChildCaseID)
		if e != nil {
			return nil, e
		}
		child["metadata"] = meta
		refs["child_case"] = child
	}
	// Job versions include native assignment/position/package changes. Counts
	// alone cannot detect a changed reservation or a rescheduled existing job.
	jobs, err := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT $3::int WHERE $3>0 UNION SELECT cc.child_case_id FROM tree t JOIN case_child_contents cc ON cc.parent_case_id=t.case_id), selected_devices AS(
 SELECT dc.deviceid FROM devicescases dc WHERE dc.caseid IN(SELECT case_id FROM tree)
 UNION SELECT $2::text WHERE $2<>''
 UNION SELECT dc.deviceid FROM devicescases dc WHERE dc.caseid=$3
 ), selected_jobs AS(
 SELECT jd.jobid FROM job_devices jd JOIN selected_devices d ON d.deviceid=jd.deviceid
 UNION SELECT jp.job_id FROM job_position_devices pd JOIN selected_devices d ON d.deviceid=pd.device_id JOIN job_positions jp ON jp.position_id=pd.position_id
 UNION SELECT jp.job_id FROM job_package_reservations pr JOIN selected_devices d ON d.deviceid=pr.device_id JOIN job_packages jp ON jp.job_package_id=pr.job_package_id
 UNION SELECT c.current_job_id FROM cases c WHERE c.caseid IN(SELECT case_id FROM tree) OR c.caseid=$3
 ) SELECT jsonb_build_object('job_id',j.jobid,'job_code',j.job_code,'status_id',j.statusid,'status',s.status,'deleted_at',j.deleted_at,'startdate',j.startdate,'enddate',j.enddate,'updated_at',j.updated_at) FROM selected_jobs x JOIN jobs j ON j.jobid=x.jobid LEFT JOIN status s ON s.statusid=j.statusid ORDER BY j.jobid LIMIT 1001`, in.CaseID, in.DeviceID, in.ChildCaseID)
	if err != nil {
		return nil, err
	}
	refs["jobs"] = jobs
	return refs, nil
}
