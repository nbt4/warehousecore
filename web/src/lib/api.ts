import axios from 'axios';
import { appPath } from './app-paths';

// Use relative path so it works on any host/platform
const API_BASE_URL = import.meta.env.VITE_API_URL || appPath('/api/v1');

export const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Types
export interface Device {
  device_id: string;
  product_id?: number;
  product_name?: string;
  product_category?: string;
  product_brand?: string;
  product_manufacturer?: string;
  barcode?: string;
  qr_code?: string;
  serial_number?: string;
  status: string;
  condition_status?: string;
  current_location?: string;
  zone_id?: number;
  zone_name?: string;
  zone_code?: string;
  case_name?: string;
  case_id?: number;
  current_job_id?: number;
  current_job_code?: string;
  current_job_status?: string;
  current_pack_status?: string;
  job_number?: string;
  status_label?: string;
  status_detail?: string;
  needs_return?: boolean;
  condition_rating?: number;
  usage_hours?: number;
  label_path?: string;
  purchase_date?: string;
  last_maintenance?: string;
  next_maintenance?: string;
  notes?: string;
}

export interface DeviceStatusHistory {
  history_id: number;
  previous_status?: string;
  new_status: string;
  previous_condition?: string;
  new_condition: string;
  previous_zone_id?: number;
  new_zone_id?: number;
  previous_location?: string;
  new_location?: string;
  change_source: string;
  changed_at: string;
}

export interface DeviceTreeDevice {
  device_id: string;
  product_id?: number;
  product_name: string;
  status: string;
  condition_status?: string;
  barcode?: string;
  qr_code?: string;
  serial_number?: string;
  zone_id?: number;
  zone_name?: string;
  zone_code?: string;
  case_id?: number;
  case_name?: string;
  current_job_id?: number;
  job_number?: string;
  condition_rating?: number;
  usage_hours?: number;
  label_path?: string;
  purchase_date?: string;
  last_maintenance?: string;
  next_maintenance?: string;
  notes?: string;
}

export interface DeviceTreeSubbiercategory {
  id: string | number;
  name: string;
  devices: DeviceTreeDevice[];
  device_count: number;
}

export interface DeviceTreeSubcategory {
  id: string | number;
  name: string;
  subbiercategories: DeviceTreeSubbiercategory[];
  direct_devices: DeviceTreeDevice[];
  device_count: number;
}

export interface DeviceTreeCategory {
  id: number;
  name: string;
  subcategories: DeviceTreeSubcategory[];
  direct_devices: DeviceTreeDevice[];
  device_count: number;
}

export interface DeviceTreeResponse {
  treeData: DeviceTreeCategory[];
}

export interface CaseSummary {
  case_id: number;
  name: string;
  description?: string;
  status: string;
  width?: number;
  height?: number;
  depth?: number;
  weight?: number;
  zone_id?: number;
  zone_name?: string;
  zone_code?: string;
  device_count: number;
  label_path?: string;
}

export type CaseDetail = CaseSummary;

export type HandlingUnitType = 'dynamic' | 'fixed' | 'hybrid';
export type HandlingUnitStatus = 'empty' | 'packing' | 'complete' | 'sealed' | 'staged' | 'on_job' | 'return_check' | 'maintenance';

export interface HandlingUnit {
  case_id: number;
  name: string;
  description?: string;
  status: string;
  case_type: HandlingUnitType;
	case_model_id?: number;
	case_model_name?: string;
  workflow_status: HandlingUnitStatus;
  width?: number;
  height?: number;
  depth?: number;
  weight?: number;
  max_weight_kg?: number;
  zone_id?: number;
  zone_name?: string;
  zone_code?: string;
  home_zone_id?: number;
  current_job_id?: number;
  barcode?: string;
  rfid_tag?: string;
  sealed_at?: string;
  device_count: number;
  product_line_count: number;
  product_quantity: number;
  child_case_count: number;
  expected_lines: number;
  complete: boolean;
}

export interface HandlingUnitInventory {
  devices: Array<{
    device_id: string;
    product_id?: number;
    product_name: string;
    status: string;
    serial_number?: string;
    barcode?: string;
  }>;
  products: Array<{
    product_id: number;
    product_name: string;
    quantity: number;
    unit: string;
    source_zone_id?: number;
  }>;
  template: Array<{
    product_id: number;
    product_name: string;
    expected_quantity: number;
    actual_quantity: number;
    complete: boolean;
  }>;
  child_cases: Array<{
    case_id: number;
    name: string;
    barcode?: string;
    workflow_status: string;
  }>;
  complete: boolean;
}

export interface HandlingUnitInput {
  name: string;
  description?: string;
  case_type: HandlingUnitType;
	case_model_id?: number;
  width?: number;
  height?: number;
  depth?: number;
  weight?: number;
  max_weight_kg?: number;
  zone_id?: number;
  home_zone_id?: number;
  barcode?: string;
  rfid_tag?: string;
}

export interface CaseDevice {
  device_id: string;
  status: string;
  serial_number?: string;
  barcode?: string;
  product_name?: string;
  zone_id?: number;
  zone_name?: string;
  zone_code?: string;
}

export interface CasesResponse {
  cases: CaseSummary[];
  meta?: {
    count: number;
  };
}

export interface Zone {
  zone_id: number;
  code: string;
  name: string;
  type: string;
  description?: string | null;
  parent_zone_id?: number | null;
  capacity?: number | null;
  is_active: boolean;
}

export type LocationStatus = 'available' | 'blocked' | 'counting' | 'maintenance' | 'archived';

export interface WarehouseLocation extends Zone {
  barcode?: string;
  location_kind: string;
  process_role: string;
  operational_status: LocationStatus;
  capacity_mode: string;
  is_storable: boolean;
  pick_sequence?: number;
  max_weight_kg?: number;
  max_volume_m3?: number;
  inventory_frequency_days?: number;
  last_counted_at?: string;
  next_count_at?: string;
  device_count: number;
  case_count: number;
  product_quantity: number;
  child_count: number;
  occupancy: number;
  utilization_percent?: number;
}

export interface WarehouseOverview {
  active_locations: number;
  blocked_locations: number;
  unplaced_devices: number;
  unplaced_cases: number;
  unplaced_product_quantity: number;
  open_tasks: number;
  counts_due: number;
}

export interface WarehouseTask {
  task_id: number;
  task_type: string;
  status: string;
  priority: number;
  from_zone_id?: number;
  to_zone_id?: number;
  case_id?: number;
  device_id?: string;
  product_id?: number;
  quantity?: number;
  job_id?: number;
  due_at?: string;
  notes?: string;
  created_at: string;
}

export interface InventoryCount {
  count_id: number;
  zone_id: number;
  zone_code: string;
  zone_name: string;
  status: 'open' | 'counting' | 'review' | 'approved' | 'cancelled';
  blind_count: boolean;
  line_count: number;
  counted_lines: number;
  variance_lines: number;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface InventoryCountLine {
  line_id: number;
  item_type: 'device' | 'product' | 'case';
  item_key: string;
  item_name: string;
  expected_quantity?: number;
  counted_quantity?: number;
  variance?: number;
}

export interface InventoryCountDetail {
  count: InventoryCount;
  lines: InventoryCountLine[];
}

export interface ZoneTypeDefinition {
  id: number;
  key: string;
  label: string;
  description?: string | null;
  default_led_pattern?: string;
  default_led_color?: string;
  default_intensity?: number;
}

export interface ScanRequest {
  scan_code: string;
  action: 'intake' | 'outtake' | 'check' | 'transfer';
  job_id?: number;
  zone_id?: number;
  quantity?: number;
  notes?: string;
}

export interface ScanResponse {
  success: boolean;
  message: string;
  device?: Device;
  action: string;
  previous_status?: string;
  new_status?: string;
  duplicate: boolean;
  product?: {
    product_id: number;
    name: string;
    unit: string;
    stock: number;
    is_accessory: boolean;
    is_consumable: boolean;
  };
}

export interface DashboardStats {
  in_storage: number;
  on_job: number;
  return_pending: number;
  location_unknown: number;
  available: number;
  blocked: number;
  defective: number;
  maintenance: number;
  retired: number;
  total: number;
  ready_for_dispatch: number;
  unavailable: number;
  movements_today: number;
  intakes_today: number;
  outtakes_today: number;
  transfers_today: number;
  active_jobs: number;
  cases_total: number;
  cases_on_job: number;
  cases_return_check: number;
  cases_packing: number;
  open_defects: number;
  overdue_inspections: number;
}

export interface Movement {
  movement_id: number;
  device_id: string;
  action: 'intake' | 'outtake' | 'transfer' | 'return' | 'move' | string;
  timestamp: string;
  from_zone_id?: number;
  to_zone_id?: number;
  from_job_id?: number;
  to_job_id?: number;
  barcode?: string;
  serial_number?: string;
  product_name?: string;
  from_zone_name?: string;
  to_zone_name?: string;
  from_job_description?: string;
  to_job_description?: string;
  performed_by?: string;
}

export interface Job {
  job_id: number;
  job_code: string;
  description?: string;
  start_date?: string;
  end_date?: string;
  status_id: number;
  status: string;
  customer_first_name?: string;
  customer_last_name?: string;
  device_count: number;
}

export interface JobDevice {
  device_id: string;
  status: string;
  product_name: string;
  zone_name?: string;
  barcode?: string;
  qr_code?: string;
  pack_status: string;
  scanned: boolean;
}

export interface JobSummary {
  job_id: number;
  job_code: string;
  description?: string;
  start_date?: string;
  end_date?: string;
  status_id: number;
  status: string;
  customer_first_name?: string;
  customer_last_name?: string;
  devices: JobDevice[];
}

// API Functions
export const dashboardApi = {
  getStats: () => api.get<DashboardStats>('/dashboard/stats'),
  getRecentMovements: (limit: number = 10) => api.get<Movement[]>('/movements', { params: { limit } }),
};

export const devicesApi = {
  getAll: (params?: { status?: string; zone_id?: number; limit?: number }) => api.get<Device[]>('/devices', { params }),
  getById: (id: string) => api.get<Device>(`/devices/${id}`),
  getMovements: (id: string) => api.get(`/devices/${id}/movements`),
  getStatusHistory: (id: string) => api.get<DeviceStatusHistory[]>(`/devices/${id}/status-history`),
  updateStatus: (id: string, data: { status?: string; condition_status?: string }) =>
    api.put<{ message: string }>(`/devices/${id}/status`, data),
  getTree: () => api.get<DeviceTreeResponse>('/devices/tree'),
};

// Device Admin API (for device CRUD operations)
export interface DeviceCreateInput {
  product_id: number;
  status?: string;
  condition_status?: string;
  serial_number?: string;
  barcode?: string;
  qr_code?: string;
  current_location?: string;
  zone_id?: number;
  condition_rating?: number;
  usage_hours?: number;
  purchase_date?: string;
  last_maintenance?: string;
  next_maintenance?: string;
  notes?: string;
  quantity?: number;
  auto_generate_label?: boolean;
  label_template_id?: number;
  regenerate_codes?: boolean;
  device_prefix?: string;
  starting_serial?: number;
  increment_serial?: boolean;
}

export interface DeviceUpdateInput {
  product_id?: number;
  status?: string;
  condition_status?: string;
  serial_number?: string;
  barcode?: string;
  qr_code?: string;
  current_location?: string;
  zone_id?: number | null;
  condition_rating?: number;
  usage_hours?: number;
  purchase_date?: string;
  last_maintenance?: string;
  next_maintenance?: string;
  notes?: string;
  regenerate_label?: boolean;
  label_template_id?: number;
  regenerate_codes?: boolean;
}

export const devicesAdminApi = {
  create: (data: DeviceCreateInput) => api.post<Device | Device[]>('/admin/devices', data),
  update: (id: string, data: DeviceUpdateInput) => api.put<Device>(`/admin/devices/${id}`, data),
  delete: (id: string) => api.delete<{ message: string }>(`/admin/devices/${id}`),
  getById: (id: string) => api.get<Device>(`/admin/devices/${id}`),
  downloadQR: (id: string) => appPath(`/api/v1/admin/devices/${id}/qr`),
  downloadBarcode: (id: string) => appPath(`/api/v1/admin/devices/${id}/barcode`),
};

export const casesApi = {
  list: (params?: { search?: string; status?: string }) => api.get<CasesResponse>('/cases', { params }),
  getById: (id: number) => api.get<CaseDetail>(`/cases/${id}`),
  getDevices: (id: number) => api.get<CaseDevice[]>(`/cases/${id}/contents`),
  create: (data: Partial<CaseDetail>) => api.post<{ case_id: number; message: string }>('/cases', data),
  update: (id: number, data: Partial<CaseDetail>) => api.put<{ message: string }>(`/cases/${id}`, data),
  delete: (id: number) => api.delete<{ message: string }>(`/cases/${id}`),
  addDevices: (caseId: number, deviceIds: string[]) =>
    api.post<{
      success_count: number;
      skipped_count: number;
      total: number;
      errors?: string[];
      message?: string;
    }>(`/cases/${caseId}/devices`, { device_ids: deviceIds }),
  removeDevice: (caseId: number, deviceId: string) => api.delete<{ message: string }>(`/cases/${caseId}/devices/${deviceId}`),
};

export const handlingUnitsApi = {
  list: (params?: { search?: string; workflow_status?: string }) => api.get<{ cases: HandlingUnit[]; meta: { count: number } }>('/handling-units', { params }),
  models: () => api.get<Array<{ model_id: number; name: string; description?: string; case_count: number }>>('/handling-unit-models'),
  get: (id: number) => api.get<HandlingUnit>(`/handling-units/${id}`),
  findByScan: (scanCode: string) =>
    api.get<HandlingUnit>('/handling-units/scan', {
      params: { scan_code: scanCode },
    }),
  create: (data: HandlingUnitInput) => api.post<{ case_id: number; message: string }>('/handling-units', data),
  update: (id: number, data: HandlingUnitInput) => api.put<{ message: string }>(`/handling-units/${id}`, data),
  delete: (id: number) => api.delete<{ message: string }>(`/handling-units/${id}`),
  inventory: (id: number) => api.get<HandlingUnitInventory>(`/handling-units/${id}/inventory`),
  packScan: (id: number, data: { scan_code: string; quantity?: number; source_zone_id?: number }) => api.post<{ message: string; item_type: string; duplicate?: boolean }>(`/handling-units/${id}/inventory/scan`, data),
  removeDevice: (id: number, deviceId: string) => api.delete(`/handling-units/${id}/inventory/devices/${encodeURIComponent(deviceId)}`),
  removeProduct: (id: number, productId: number, data: { quantity: number; destination_zone_id: number }) => api.post(`/handling-units/${id}/inventory/products/${productId}`, data),
  removeChild: (id: number, childId: number, destinationZoneId: number) =>
    api.post(`/handling-units/${id}/inventory/cases/${childId}`, {
      destination_zone_id: destinationZoneId,
    }),
  setTemplate: (id: number, productId: number, expectedQuantity: number) =>
    api.post(`/handling-units/${id}/template`, {
      product_id: productId,
      expected_quantity: expectedQuantity,
    }),
  setTemplateByScan: (id: number, scanCode: string, expectedQuantity: number) =>
    api.post(`/handling-units/${id}/template`, {
      scan_code: scanCode,
      expected_quantity: expectedQuantity,
    }),
  removeTemplate: (id: number, productId: number) => api.delete(`/handling-units/${id}/template/${productId}`),
  seal: (id: number, force = false) => api.post(`/handling-units/${id}/seal`, { force }),
  unseal: (id: number) => api.post(`/handling-units/${id}/unseal`),
  dispatch: (id: number, jobId: number, force = false) => api.post(`/handling-units/${id}/dispatch`, { job_id: jobId, force }),
  returnCase: (id: number, destinationZoneId: number, mode: 'sealed' | 'inspect') =>
    api.post(`/handling-units/${id}/return`, {
      destination_zone_id: destinationZoneId,
      mode,
    }),
  unpack: (id: number, destinationZoneId: number) =>
    api.post(`/handling-units/${id}/unpack`, {
      destination_zone_id: destinationZoneId,
    }),
  events: (id: number) => api.get<Array<Record<string, unknown>>>(`/handling-units/${id}/events`),
};

export interface ProductInZone {
  product_id: number;
  product_name: string;
  quantity: number;
  unit: string;
  is_accessory: boolean;
  is_consumable: boolean;
}

export const zonesApi = {
  getAll: () => api.get<Zone[]>('/zones'),
  getById: (id: number) => api.get<Zone>(`/zones/${id}`),
  getByScan: (scanCode: string) => api.get<Zone>('/zones/scan', { params: { scan_code: scanCode } }),
  getProducts: (id: number) => api.get<ProductInZone[]>(`/zones/${id}/products`),
  create: (data: Partial<Zone>) => api.post<Zone>('/zones', data),
  update: (id: number, data: Partial<Zone>) => api.put(`/zones/${id}`, data),
  delete: (id: number) => api.delete(`/zones/${id}`),
};

export const warehouseApi = {
  locations: (includeArchived = true) =>
    api.get<WarehouseLocation[]>('/warehouse/locations', {
      params: { include_archived: includeArchived },
    }),
  location: (id: number) => api.get<WarehouseLocation>(`/warehouse/locations/${id}`),
  createLocation: (data: Partial<WarehouseLocation>) => api.post<{ zone_id: number; message: string }>('/warehouse/locations', data),
  updateLocation: (id: number, data: Partial<WarehouseLocation>) => api.put<{ message: string }>(`/warehouse/locations/${id}`, data),
  archiveLocation: (id: number) => api.post<{ message: string }>(`/warehouse/locations/${id}/archive`),
  overview: () => api.get<WarehouseOverview>('/warehouse/overview'),
  tasks: (status?: string) =>
    api.get<WarehouseTask[]>('/warehouse/tasks', {
      params: status ? { status } : undefined,
    }),
  createTask: (data: Partial<WarehouseTask>) => api.post<{ task_id: number; message: string }>('/warehouse/tasks', data),
  updateTaskStatus: (id: number, status: string) => api.patch(`/warehouse/tasks/${id}/status`, { status }),
  counts: () => api.get<InventoryCount[]>('/warehouse/counts'),
  createCount: (zoneId: number, blindCount: boolean) =>
    api.post<{ count_id: number; message: string }>('/warehouse/counts', {
      zone_id: zoneId,
      blind_count: blindCount,
    }),
  count: (id: number) => api.get<InventoryCountDetail>(`/warehouse/counts/${id}`),
  scanCount: (id: number, scanCode: string, quantity = 1) => api.post(`/warehouse/counts/${id}/scan`, { scan_code: scanCode, quantity }),
  completeCount: (id: number) => api.post(`/warehouse/counts/${id}/complete`),
  approveCount: (id: number) => api.post(`/warehouse/counts/${id}/approve`),
  cancelCount: (id: number) => api.post(`/warehouse/counts/${id}/cancel`),
};

export const zoneTypesApi = {
  getAll: () => api.get<ZoneTypeDefinition[]>('/zone-types'),
};

export const scansApi = {
  process: (data: ScanRequest) => api.post<ScanResponse>('/scans', data),
  getHistory: (limit: number = 50) => api.get(`/scans/history`, { params: { limit } }),
};

export interface JobRequirement {
  id: number;
  product_id: number;
  product_name: string;
  required_quantity: number;
  booked_quantity: number;
}

export const jobsApi = {
  getAll: (params?: { status?: string }) => api.get<Job[]>('/jobs', { params }),
  getByScan: (scanCode: string) => api.get<JobSummary>('/jobs/scan', { params: { scan_code: scanCode } }),
  getById: (id: number) => api.get<JobSummary>(`/jobs/${id}`),
  getRequirements: (id: number) => api.get<JobRequirement[]>(`/jobs/${id}/requirements`),
};

export interface PicklistPositionDevice {
  device_id: string;
  scanned_at: string;
  scanned_by: string;
}

export interface PicklistPosition {
  position_id: number;
  product_id: number | null;
  product_name: string;
  needed: number;
  scanned: number;
  fulfilled: boolean;
  devices: PicklistPositionDevice[];
}

export interface JobPicklist {
  job_id: number;
  positions: PicklistPosition[];
}

export const picklistApi = {
  getByJob: (jobId: number) => api.get<JobPicklist>(`/jobs/${jobId}/picklist`),
  scanDevice: (jobId: number, deviceId: string, scannedBy?: string) => api.post<{ position_id: number; device_id: string; message: string }>(`/jobs/${jobId}/picklist/scan`, { device_id: deviceId, scanned_by: scannedBy || '' }),
};

export type MaintenanceOrderType = 'defect' | 'preventive' | 'inspection' | 'calibration';
export type MaintenancePriority = 'low' | 'normal' | 'high' | 'critical';
export type MaintenanceOrderStatus = 'open' | 'planned' | 'in_progress' | 'waiting_parts' | 'completed' | 'cancelled';
export type MaintenanceOutcome = 'passed' | 'passed_with_notes' | 'failed' | 'repaired';

export interface MaintenanceOrder {
  order_id: number;
  device_id: string;
  plan_id?: number;
  order_type: MaintenanceOrderType;
  priority: MaintenancePriority;
  status: MaintenanceOrderStatus;
  title: string;
  description?: string;
  due_at?: string;
  scheduled_at?: string;
  reported_by?: number;
  reported_by_name?: string;
  assigned_to?: number;
  assigned_to_name?: string;
  started_at?: string;
  completed_at?: string;
  outcome?: MaintenanceOutcome;
  resolution?: string;
  cost?: number;
  created_at: string;
  updated_at: string;
  product_name?: string;
  serial_number?: string;
  device_condition: string;
  zone_name?: string;
  plan_name?: string;
}

export interface MaintenancePlan {
  plan_id: number;
  device_id: string;
  name: string;
  maintenance_type: Exclude<MaintenanceOrderType, 'defect'>;
  interval_days: number;
  lead_time_days: number;
  instructions?: string;
  next_due_at: string;
  last_completed_at?: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  product_name?: string;
  serial_number?: string;
  device_condition: string;
  has_active_order: boolean;
}

export interface MaintenanceOverview {
  overdue_orders: number;
  due_soon_orders: number;
  open_defects: number;
  in_progress_orders: number;
  completed_this_month: number;
  active_plans: number;
  unavailable_devices: number;
  cost_this_month: number;
}

export interface MaintenanceDeviceOption {
  device_id: string;
  product_name: string;
  serial_number?: string;
  barcode?: string;
  status: string;
  condition_status: string;
}

export interface MaintenanceUserOption {
  user_id: number;
  name: string;
}
export interface MaintenanceOptions {
  devices: MaintenanceDeviceOption[];
  users: MaintenanceUserOption[];
}

export interface MaintenanceEvent {
  event_id: number;
  event_type: string;
  from_status?: string;
  to_status?: string;
  notes?: string;
  actor_id?: number;
  actor_name?: string;
  created_at: string;
}

export interface MaintenanceOrderInput {
  device_id: string;
  order_type: MaintenanceOrderType;
  priority: MaintenancePriority;
  title: string;
  description: string;
  due_at: string;
  assigned_to?: number;
  cost?: number;
}

export interface MaintenancePlanInput {
  device_id: string;
  name: string;
  maintenance_type: Exclude<MaintenanceOrderType, 'defect'>;
  interval_days: number;
  lead_time_days: number;
  instructions: string;
  next_due_at: string;
  is_active: boolean;
}

export const maintenanceApi = {
  overview: () => api.get<MaintenanceOverview>('/maintenance/overview'),
  options: () => api.get<MaintenanceOptions>('/maintenance/options'),
  orders: (params?: { scope?: 'active' | 'history'; status?: MaintenanceOrderStatus; type?: MaintenanceOrderType; search?: string }) => api.get<MaintenanceOrder[]>('/maintenance/orders', { params }),
  createOrder: (data: MaintenanceOrderInput) => api.post<{ order_id: number; message: string }>('/maintenance/orders', data),
  updateOrder: (id: number, data: MaintenanceOrderInput) => api.put<{ message: string }>(`/maintenance/orders/${id}`, data),
  transitionOrder: (
    id: number,
    data: {
      status: MaintenanceOrderStatus;
      outcome?: MaintenanceOutcome;
      resolution?: string;
      notes?: string;
      cost?: number;
      next_due_at?: string;
    },
  ) => api.post<{ message: string }>(`/maintenance/orders/${id}/transition`, data),
  events: (id: number) => api.get<MaintenanceEvent[]>(`/maintenance/orders/${id}/events`),
  plans: () => api.get<MaintenancePlan[]>('/maintenance/plans'),
  createPlan: (data: MaintenancePlanInput) => api.post<{ plan_id: number; message: string }>('/maintenance/plans', data),
  updatePlan: (id: number, data: MaintenancePlanInput) => api.put<{ message: string }>(`/maintenance/plans/${id}`, data),
};

// LED Control Types
export interface LEDStatus {
  mqtt_connected: boolean;
  mqtt_dry_run: boolean;
  mapping_loaded: boolean;
  warehouse_id: string;
  total_shelves: number;
  total_bins: number;
}

export interface LEDMapping {
  warehouse_id: string;
  shelves: Array<{
    shelf_id: string;
    bins: Array<{
      bin_id: string;
      pixels: number[];
    }>;
  }>;
  led_strip: {
    length: number;
    data_pin: number;
    chipset: string;
  };
  defaults: {
    color: string;
    pattern: string;
    intensity: number;
    speed?: number;
  };
}

export interface LEDAppearance {
  color: string;
  pattern: string;
  intensity: number;
  speed: number;
}

export interface LEDJobHighlightSettings {
  mode: 'all_bins' | 'required_only';
  required: LEDAppearance;
  non_required: LEDAppearance;
}

export interface LEDController {
  id: number;
  controller_id: string;
  display_name: string;
  topic_suffix: string;
  is_active: boolean;
  last_seen?: string | null;
  metadata?: Record<string, unknown> | null;
  ip_address?: string | null;
  hostname?: string | null;
  firmware_version?: string | null;
  mac_address?: string | null;
  status_data?: Record<string, unknown> | null;
  zone_types?: ZoneTypeDefinition[];
}

export interface LEDControllerPayload {
  controller_id?: string;
  display_name?: string;
  topic_suffix?: string;
  is_active?: boolean;
  metadata?: Record<string, unknown> | null;
  zone_type_ids?: number[];
}

export const ledApi = {
  getStatus: () => api.get<LEDStatus>('/led/status'),
  highlightJob: (jobId: number) => api.post(`/led/highlight?job_id=${jobId}`),
  clear: () => api.post('/led/clear'),
  identify: () => api.post('/led/identify'),
  testBin: (shelfId: string, binId: string) => api.post(`/led/test?shelf_id=${shelfId}&bin_id=${binId}`),
  locateBin: (binCode: string) => api.post(`/led/locate?bin_code=${binCode}`),
  getJobSettings: () => api.get<LEDJobHighlightSettings>('/admin/led/job-highlights'),
  updateJobSettings: (settings: LEDJobHighlightSettings) => api.put('/admin/led/job-highlights', settings),
  getMapping: () => api.get<LEDMapping>('/admin/led/mapping'),
  updateMapping: (mapping: LEDMapping) => api.put('/admin/led/mapping', mapping),
  validateMapping: (mapping: LEDMapping) => api.post('/admin/led/mapping/validate', mapping),
  preview: (appearances: LEDAppearance[], clearBefore: boolean = false, targetBinId?: string) => {
    const payload: Record<string, unknown> = {
      appearances,
    };
    if (clearBefore) {
      payload.clear_before = true;
    }
    if (targetBinId && targetBinId.trim().length > 0) {
      payload.target_bin_id = targetBinId.trim();
    }
    return api.post('/admin/led/preview', payload);
  },
  getControllers: () => api.get<LEDController[]>('/admin/led/controllers'),
  createController: (payload: LEDControllerPayload) => api.post('/admin/led/controllers', payload),
  updateController: (id: number, payload: LEDControllerPayload) => api.put(`/admin/led/controllers/${id}`, payload),
  deleteController: (id: number) => api.delete(`/admin/led/controllers/${id}`),
  configureController: (id: number, config: { led_count?: number; data_pin?: number; chipset?: string }) => api.post(`/admin/led/controllers/${id}/configure`, config),
  restartController: (id: number) => api.post(`/admin/led/controllers/${id}/restart`),
};

// Label API
export interface LabelTemplate {
  id?: number;
  name: string;
  description: string;
  width: number;
  height: number;
  template_json: string;
  is_default: boolean;
  target_type: LabelTargetType;
  revision: number;
  created_at?: string;
  updated_at?: string;
}

export type LabelTargetType = 'device' | 'product' | 'case' | 'zone';

export interface LabelFieldDefinition {
  key: string;
  label: string;
}

export interface LabelTarget {
  target_type: LabelTargetType;
  id: string;
  code: string;
  name: string;
  subtitle: string;
  label_path?: string;
  is_stale: boolean;
  fields?: Record<string, string>;
}

export interface LabelPrinter {
  id?: number;
  name: string;
  driver: 'zpl_tcp';
  address: string;
  port: number;
  dpi: 203 | 300 | 600;
  is_default: boolean;
  is_active: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface LabelPrintJob {
  id: number;
  target_type: LabelTargetType;
  target_id: string;
  template_id?: number;
  printer_id?: number;
  printer_name?: string;
  copies: number;
  status: 'queued' | 'printing' | 'completed' | 'failed';
  label_path?: string;
  error_message?: string;
  created_at: string;
  completed_at?: string;
}

export interface LabelElement {
  type: 'barcode' | 'qrcode' | 'text' | 'image';
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
  content: string;
  image_data?: string;
  style: {
    font_size?: number;
    font_weight?: string;
    font_family?: string;
    color?: string;
    alignment?: string;
    format?: string;
  };
}

export const labelsApi = {
  generateQRCode: (content: string, size: number = 256) => api.post<{ image_data: string }>('/labels/qrcode', { content, size }),
  generateBarcode: (content: string, width: number = 300, height: number = 100) =>
    api.post<{ image_data: string }>('/labels/barcode', {
      content,
      width,
      height,
    }),
  getTemplates: () => api.get<LabelTemplate[]>('/labels/templates'),
  getTemplate: (id: number) => api.get<LabelTemplate>(`/labels/templates/${id}`),
  createTemplate: (template: LabelTemplate) => api.post<LabelTemplate>('/labels/templates', template),
  updateTemplate: (id: number, updates: Partial<LabelTemplate>) => api.put(`/labels/templates/${id}`, updates),
  deleteTemplate: (id: number) => api.delete(`/labels/templates/${id}`),
  generateDeviceLabel: (deviceId: string, templateId: number) => api.post(`/labels/device/${deviceId}`, { template_id: templateId }),
  generateCaseLabel: (caseId: number, templateId: number) => api.post(`/labels/case/${caseId}`, { template_id: templateId }),
  saveLabel: (deviceId: string, imageData: string) =>
    api.post<{ label_path: string; message: string }>('/labels/save', {
      device_id: deviceId,
      image_data: imageData,
    }),
  saveCaseLabel: (caseId: number, imageData: string) =>
    api.post<{ label_path: string; message: string }>('/labels/save-case', {
      case_id: caseId,
      image_data: imageData,
    }),
  getTargets: (type: LabelTargetType, search = '', limit = 250) =>
    api.get<LabelTarget[]>('/labels/targets', {
      params: { type, search, limit },
    }),
  getTarget: (type: LabelTargetType, id: string) => api.get<LabelTarget>(`/labels/targets/${type}/${encodeURIComponent(id)}`),
  getFields: (type: LabelTargetType) => api.get<LabelFieldDefinition[]>(`/labels/fields/${type}`),
  renderTarget: (payload: { target_type: LabelTargetType; target_id: string; template_id: number; save: boolean }) =>
    api.post<{
      target: LabelTarget;
      template: LabelTemplate;
      elements: LabelElement[];
      image_data: string;
      label_path?: string;
    }>('/labels/render', payload),
  renderTargets: (payload: { target_type: LabelTargetType; target_ids: string[]; template_id: number; save: boolean; include_images: boolean }) =>
    api.post<{
      results: Array<{
        target: LabelTarget;
        template: LabelTemplate;
        elements: LabelElement[];
        image_data: string;
        label_path?: string;
      }>;
    }>('/labels/render-batch', payload),
  exportPDF: (payload: { target_type: LabelTargetType; target_ids: string[]; template_id: number; copies: number }) => api.post<Blob>('/labels/pdf', payload, { responseType: 'blob' }),
  getPrinters: () => api.get<LabelPrinter[]>('/labels/printers'),
  createPrinter: (printer: LabelPrinter) => api.post<LabelPrinter>('/labels/printers', printer),
  updatePrinter: (id: number, printer: LabelPrinter) => api.put<LabelPrinter>(`/labels/printers/${id}`, printer),
  deletePrinter: (id: number) => api.delete(`/labels/printers/${id}`),
  printDirect: (payload: { target_type: LabelTargetType; target_ids: string[]; template_id: number; printer_id: number; copies: number }) => api.post<{ jobs: LabelPrintJob[] }>('/labels/print', payload),
  getPrintJobs: (limit = 100) => api.get<LabelPrintJob[]>('/labels/print-jobs', { params: { limit } }),
};

// Admin Settings API
export interface APILimits {
  device_limit: number;
  case_limit: number;
}

export const adminSettingsApi = {
  getAPILimits: () => api.get<APILimits>('/admin/api-limits'),
  updateAPILimits: (limits: Partial<APILimits>) =>
    api.put<APILimits & { message: string }>('/admin/api-limits', {
      device_limit: limits.device_limit,
      case_limit: limits.case_limit,
    }),
};

// API Keys
export interface APIKeyItem {
  id: number;
  name: string;
  is_active: boolean;
  created_at: string;
  last_used_at?: string | null;
}

export const apiKeysAdminApi = {
  list: () => api.get<{ keys: APIKeyItem[] }>('/admin/api-keys'),
  create: (payload: { name: string }) => api.post<{ keys: APIKeyItem[]; api_key: string } | { api_key: string }>('/admin/api-keys', payload),
  updateStatus: (id: number, is_active: boolean) => api.put(`/admin/api-keys/${id}/status`, { is_active }),
  delete: (id: number) => api.delete(`/admin/api-keys/${id}`),
};

// Cable interfaces
export interface Cable {
  cable_id: number;
  product_id: number;
  name: string;
  connector1: number;
  connector2: number;
  typ: number;
  length: number;
  mm2: number | null;
  tracking_mode: CableTrackingMode;
  generic_barcode: string | null;
  stock_quantity: number;
  available_quantity: number;
  unit_count: number;
  connector1_name: string;
  connector2_name: string;
  cable_type_name: string;
  connector1_gender?: string | null;
  connector2_gender?: string | null;
  migrated_from_legacy: boolean;
  zone_stocks?: CableZoneStock[];
  units?: CableUnit[];
}

export type CableTrackingMode = 'quantity' | 'individual';

export interface CableZoneStock {
  zone_id: number | null;
  zone_name: string;
  zone_code: string;
  quantity: number;
}

export interface CableUnit {
  device_id: string;
  barcode: string | null;
  qr_code: string | null;
  status: string;
  zone_id: number | null;
  zone_name: string;
  zone_code: string;
  condition_rating: number;
  current_job_id: number | null;
}

export interface CableConnector {
  connector_id: number;
  name: string;
  abbreviation: string | null;
  gender: string | null;
}

export interface CableType {
  cable_type_id: number;
  name: string;
  count?: number;
}

export interface CableCreateInput {
  name?: string;
  connector1: number;
  connector2: number;
  typ: number;
  length: number;
  mm2?: number | null;
  tracking_mode: CableTrackingMode;
  generic_barcode?: string;
  quantity: number;
  zone_id?: number | null;
}

export interface CableUpdateInput {
  name?: string;
  connector1?: number;
  connector2?: number;
  typ?: number;
  length?: number;
  mm2?: number | null;
  tracking_mode?: CableTrackingMode;
  generic_barcode?: string;
}

// Cable admin API
export const cablesAdminApi = {
  getAll: (params?: { search?: string; connector1?: number; connector2?: number; type?: number; length_min?: number; length_max?: number; tracking_mode?: CableTrackingMode }) => {
    const queryParams = new URLSearchParams();
    if (params?.search) queryParams.append('search', params.search);
    if (params?.connector1) queryParams.append('connector1', params.connector1.toString());
    if (params?.connector2) queryParams.append('connector2', params.connector2.toString());
    if (params?.type) queryParams.append('type', params.type.toString());
    if (params?.length_min) queryParams.append('length_min', params.length_min.toString());
    if (params?.length_max) queryParams.append('length_max', params.length_max.toString());
    if (params?.tracking_mode) queryParams.append('tracking_mode', params.tracking_mode);
    const query = queryParams.toString();
    return api.get<Cable[]>(`/admin/cables${query ? `?${query}` : ''}`);
  },
  getById: (id: number) => api.get<Cable>(`/admin/cables/${id}`),
  create: (data: CableCreateInput) => api.post<{ cable_id: number; message: string }>('/admin/cables', data),
  update: (id: number, data: CableUpdateInput) => api.put<{ message: string }>(`/admin/cables/${id}`, data),
  delete: (id: number) => api.delete<{ message: string }>(`/admin/cables/${id}`),
  setStock: (id: number, data: { zone_id?: number | null; quantity: number }) => api.put<{ message: string }>(`/admin/cables/${id}/stock`, data),
  createUnits: (id: number, data: { zone_id?: number | null; quantity: number }) => api.post<{ created_count: number; message: string }>(`/admin/cables/${id}/units`, data),
  deleteUnit: (id: number, deviceId: string) => api.delete<{ message: string }>(`/admin/cables/${id}/units/${encodeURIComponent(deviceId)}`),
  getConnectors: () => api.get<CableConnector[]>('/admin/cable-connectors'),
  getTypes: () => api.get<CableType[]>('/admin/cable-types'),
};

export interface ProductPicture {
  file_name: string;
  size: number;
  content_type: string;
  modified_at: string;
  download_url: string;
  thumbnail_url?: string;
  preview_url?: string;
  temporary?: boolean;
}

export const productPicturesApi = {
  list: (productId: number) => api.get<{ pictures: ProductPicture[] }>(`/admin/products/${productId}/pictures`),
  upload: (productId: number, files: FileList | File[]) => {
    const data = new FormData();
    Array.from(files as ArrayLike<File>).forEach((file) => data.append('files', file));
    return api.post(`/admin/products/${productId}/pictures`, data, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
  },
  delete: (productId: number, fileName: string) => api.delete(`/admin/products/${productId}/pictures/${encodeURIComponent(fileName)}`),
};

export const productWebsiteApi = {
  update: (
    productId: number,
    payload: {
      website_visible: boolean;
      website_images: string[];
      website_thumbnail?: string | null;
    },
  ) => api.put(`/admin/products/${productId}/website`, payload),
};
