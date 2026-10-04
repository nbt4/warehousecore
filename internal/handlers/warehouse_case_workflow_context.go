package handlers

import (
	"database/sql"
	"fmt"
	"math"
)

// Workflow changes move a complete handling unit; they never rewrite the
// reviewed physical membership or quantity contents.
type warehouseCaseWorkflowRequest struct {
	CaseID                   int64  `json:"case_id"`
	JobID                    int64  `json:"job_id,omitempty"`
	DestinationZoneID        int64  `json:"destination_zone_id,omitempty"`
	ReturnMode               string `json:"return_mode,omitempty"`
	AcceptIncompleteTemplate bool   `json:"accept_incomplete_template,omitempty"`
	InspectionPassed         bool   `json:"inspection_passed,omitempty"`
	ExpectedUpdatedAt        string `json:"expected_updated_at,omitempty"`
	ExpectedContext          string `json:"expected_context,omitempty"`
	ConfirmChange            bool   `json:"confirm_change,omitempty"`
	ConfirmationText         string `json:"confirmation_text,omitempty"`
	Preview                  bool   `json:"preview,omitempty"`
}

func validateCaseWorkflowInput(op string, in warehouseCaseWorkflowRequest) error {
	if in.CaseID <= 0 || in.CaseID > math.MaxInt32 || in.JobID < 0 || in.JobID > math.MaxInt32 || in.DestinationZoneID < 0 || in.DestinationZoneID > math.MaxInt32 {
		return fmt.Errorf("valid exact serial identities required")
	}
	if op != "inspect_return" && in.InspectionPassed {
		return fmt.Errorf("inspection_passed is only valid for returned-case inspection")
	}
	switch op {
	case "seal":
		if in.JobID != 0 || in.DestinationZoneID != 0 || in.ReturnMode != "" {
			return fmt.Errorf("seal accepts only case identity and explicit incomplete-template consent")
		}
	case "unseal":
		if in.JobID != 0 || in.DestinationZoneID != 0 || in.ReturnMode != "" || in.AcceptIncompleteTemplate {
			return fmt.Errorf("unseal accepts only case identity")
		}
	case "move":
		if in.JobID != 0 || in.DestinationZoneID <= 0 || in.ReturnMode != "" || in.AcceptIncompleteTemplate {
			return fmt.Errorf("move requires an explicit destination only")
		}
	case "dispatch":
		if in.JobID <= 0 || in.DestinationZoneID != 0 || in.ReturnMode != "" || in.AcceptIncompleteTemplate {
			return fmt.Errorf("dispatch requires an exact confirmed job; seal separately before dispatch")
		}
	case "return":
		if in.JobID != 0 || in.DestinationZoneID <= 0 || !map[string]bool{"inspect": true, "sealed": true}[in.ReturnMode] || in.AcceptIncompleteTemplate {
			return fmt.Errorf("return requires an explicit destination and inspect or sealed mode; current job identity is resolved from the issued case")
		}
	case "inspect_return":
		if in.JobID != 0 || in.DestinationZoneID != 0 || in.ReturnMode != "" || in.AcceptIncompleteTemplate {
			return fmt.Errorf("returned inspection accepts case identity and explicit whole-tree inspection attestation only")
		}
	default:
		return fmt.Errorf("unsupported case workflow")
	}
	return nil
}

// This context includes physical state and redacted scheduling references.
// It contains neither commercial job positions nor employee/customer details.
func caseWorkflowContext(tx *sql.Tx, in warehouseCaseWorkflowRequest, userID int64) (map[string]any, error) {
	refs, err := caseContentContext(tx, warehouseCaseContentRequest{CaseID: in.CaseID})
	if err != nil {
		return nil, err
	}
	metadata, err := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id) SELECT jsonb_build_object('case_id',c.caseid,'lifecycle_status',c.lifecycle_status,'status',c.status,'workflow_status',c.workflow_status,'case_type',c.case_type,'current_job_id',c.current_job_id,'zone_id',c.zone_id,'sealed_at',c.sealed_at,'weight',c.weight,'max_weight_kg',c.max_weight_kg,'updated_at',to_char(c.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM tree t JOIN cases c ON c.caseid=t.case_id ORDER BY c.caseid LIMIT 1001`, in.CaseID)
	if err != nil {
		return nil, err
	}
	refs["case_states"] = metadata
	templates, err := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id) SELECT jsonb_build_object('template_line_id',ct.template_line_id,'case_id',ct.case_id,'product_id',ct.product_id,'product_name',p.name,'product_lifecycle',p.lifecycle_status,'tracking_mode',p.tracking_mode,'expected_quantity',ct.expected_quantity,'actual_quantity',(SELECT count(*) FROM devicescases dc JOIN devices d ON d.deviceid=dc.deviceid WHERE dc.caseid=ct.case_id AND d.productid=ct.product_id)+COALESCE((SELECT quantity FROM case_product_contents pc WHERE pc.case_id=ct.case_id AND pc.product_id=ct.product_id),0),'updated_at',ct.updated_at,'product_updated_at',p.updated_at) FROM case_content_templates ct JOIN tree t ON t.case_id=ct.case_id JOIN products p ON p.productid=ct.product_id WHERE ct.lifecycle_status='active' ORDER BY ct.case_id,ct.template_line_id LIMIT 1001`, in.CaseID)
	if err != nil {
		return nil, err
	}
	refs["active_templates"] = templates
	tasks, err := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id) SELECT jsonb_build_object('task_id',task_id,'task_type',task_type,'status',status,'case_id',case_id,'device_id',device_id,'job_id',job_id,'from_zone_id',from_zone_id,'to_zone_id',to_zone_id,'assigned_to',assigned_to,'updated_at',updated_at) FROM warehouse_tasks WHERE status IN('open','in_progress') AND (case_id IN(SELECT case_id FROM tree) OR device_id IN(SELECT deviceid FROM devicescases WHERE caseid IN(SELECT case_id FROM tree))) ORDER BY task_id LIMIT 1001`, in.CaseID)
	if err != nil {
		return nil, err
	}
	refs["open_tasks"] = tasks
	jobID := in.JobID
	if jobID == 0 {
		jobID = caseContentID(refs["case"].(map[string]any)["current_job_id"])
	}
	if jobID > 0 {
		job, e := inventoryMCPJSON(tx, `SELECT jsonb_build_object('job_id',j.jobid,'job_code',j.job_code,'status_id',j.statusid,'status',s.status,'deleted_at',j.deleted_at,'startdate',j.startdate,'enddate',j.enddate,'valid_dates',j.startdate<=j.enddate,'updated_at',to_char(j.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM jobs j LEFT JOIN status s ON s.statusid=j.statusid WHERE j.jobid=$1`, jobID)
		if e != nil {
			return nil, e
		}
		refs["target_job"] = job
		editors, e := inventoryMCPRows(tx, `SELECT jsonb_build_object('user_id',user_id,'last_seen',last_seen) FROM job_edit_sessions WHERE job_id=$1 AND user_id<>$2 AND last_seen>CURRENT_TIMESTAMP-INTERVAL '2 minutes' ORDER BY user_id LIMIT 1001`, jobID, userID)
		if e != nil {
			return nil, e
		}
		refs["active_editors"] = editors
	}
	// Frozen membership state detects changed reservations even where an older
	// native writer does not yet propagate its version to the parent job.
	reservations, err := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id), selected_devices AS(SELECT deviceid FROM devicescases WHERE caseid IN(SELECT case_id FROM tree)), selected AS(
 SELECT 'job_device'::text AS kind,jd.jobid AS job_id,jd.deviceid AS device_id,jsonb_build_object('pack_status',jd.pack_status,'pack_ts',jd.pack_ts) AS state FROM job_devices jd JOIN selected_devices d ON d.deviceid=jd.deviceid
 UNION ALL SELECT 'position_device',jp.job_id,pd.device_id,jsonb_build_object('position_id',pd.position_id) FROM job_position_devices pd JOIN selected_devices d ON d.deviceid=pd.device_id JOIN job_positions jp ON jp.position_id=pd.position_id
 UNION ALL SELECT 'package_reservation',jp.job_id,pr.device_id,jsonb_build_object('reservation_id',pr.reservation_id,'job_package_id',pr.job_package_id,'quantity',pr.quantity,'reservation_status',pr.reservation_status,'reserved_at',pr.reserved_at,'assigned_at',pr.assigned_at,'released_at',pr.released_at) FROM job_package_reservations pr JOIN selected_devices d ON d.deviceid=pr.device_id JOIN job_packages jp ON jp.job_package_id=pr.job_package_id
 ) SELECT jsonb_build_object('kind',kind,'job_id',job_id,'device_id',device_id,'state',state) FROM selected ORDER BY kind,job_id,device_id,state::text LIMIT 1001`, in.CaseID)
	if err != nil {
		return nil, err
	}
	refs["device_assignments"] = reservations
	components, err := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id), selected AS(SELECT deviceid FROM devicescases WHERE caseid IN(SELECT case_id FROM tree)) SELECT jsonb_build_object('device_id',device_id,'component_device_id',component_device_id) FROM device_components WHERE device_id IN(SELECT deviceid FROM selected) OR component_device_id IN(SELECT deviceid FROM selected) ORDER BY device_id,component_device_id LIMIT 1001`, in.CaseID)
	if err != nil {
		return nil, err
	}
	refs["component_memberships"] = components
	return refs, nil
}
