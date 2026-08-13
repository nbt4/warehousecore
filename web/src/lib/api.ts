import axios from 'axios';

// Use relative path so it works on any host/platform
const API_BASE_URL = import.meta.env.VITE_API_URL || '/api/v1';

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
  barcode?: string;
  qr_code?: string;
  serial_number?: string;
  status: string;
  current_location?: string;
  zone_id?: number;
  zone_name?: string;
  zone_code?: string;
  case_name?: string;
  case_id?: number;
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

export interface DeviceTreeDevice {
  device_id: string;
  product_id?: number;
  product_name: string;
  status: string;
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

export interface CaseDetail extends CaseSummary {}

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
    stock_quantity: number;
    min_stock_level: number;
    unit: string;
  };
}

export interface DashboardStats {
  in_storage: number;
  on_job: number;
  defective: number;
  total: number;
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
  status: string;
  customer_first_name?: string;
  customer_last_name?: string;
  devices: JobDevice[];
}

// API Functions
export const dashboardApi = {
  getStats: () => api.get<DashboardStats>('/dashboard/stats'),
  getRecentMovements: (limit: number = 10) =>
    api.get<Movement[]>('/movements', { params: { limit } }),
};

export const devicesApi = {
  getAll: (params?: { status?: string; zone_id?: number; limit?: number }) =>
    api.get<Device[]>('/devices', { params }),
  getById: (id: string) => api.get<Device>(`/devices/${id}`),
  getMovements: (id: string) => api.get(`/devices/${id}/movements`),
  getTree: () => api.get<DeviceTreeResponse>('/devices/tree'),
};

// Device Admin API (for device CRUD operations)
export interface DeviceCreateInput {
  product_id: number;
  status?: string;
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
  regenerate_label?: boolean;
  label_template_id?: number;
  regenerate_codes?: boolean;
}

export const devicesAdminApi = {
  create: (data: DeviceCreateInput) => api.post<Device | Device[]>('/admin/devices', data),
  update: (id: string, data: DeviceUpdateInput) => api.put<Device>(`/admin/devices/${id}`, data),
  delete: (id: string) => api.delete<{ message: string }>(`/admin/devices/${id}`),
  getById: (id: string) => api.get<Device>(`/admin/devices/${id}`),
  downloadQR: (id: string) => `/api/v1/admin/devices/${id}/qr`,
  downloadBarcode: (id: string) => `/api/v1/admin/devices/${id}/barcode`,
};

export const casesApi = {
  list: (params?: { search?: string; status?: string }) =>
    api.get<CasesResponse>('/cases', { params }),
  getById: (id: number) => api.get<CaseDetail>(`/cases/${id}`),
  getDevices: (id: number) => api.get<CaseDevice[]>(`/cases/${id}/contents`),
  create: (data: Partial<CaseDetail>) => api.post<{ case_id: number; message: string }>('/cases', data),
  update: (id: number, data: Partial<CaseDetail>) => api.put<{ message: string }>(`/cases/${id}`, data),
  delete: (id: number) => api.delete<{ message: string }>(`/cases/${id}`),
  addDevices: (caseId: number, deviceIds: string[]) =>
    api.post<{ success_count: number; skipped_count: number; total: number; errors?: string[]; message?: string }>(`/cases/${caseId}/devices`, { device_ids: deviceIds }),
  removeDevice: (caseId: number, deviceId: string) =>
    api.delete<{ message: string }>(`/cases/${caseId}/devices/${deviceId}`),
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
  getByJob: (jobId: number) =>
    api.get<JobPicklist>(`/jobs/${jobId}/picklist`),
  scanDevice: (jobId: number, deviceId: string, scannedBy?: string) =>
    api.post<{ position_id: number; device_id: string; message: string }>(
      `/jobs/${jobId}/picklist/scan`,
      { device_id: deviceId, scanned_by: scannedBy || '' }
    ),
};

export interface Defect {
  defect_id: number;
  device_id: string;
  severity: 'low' | 'medium' | 'high' | 'critical';
  status: 'open' | 'in_progress' | 'repaired' | 'closed';
  title: string;
  description: string;
  reported_at: string;
  repair_cost?: number;
  repaired_at?: string;
  closed_at?: string;
  product_name?: string;
}

export interface Inspection {
  schedule_id: number;
  device_id?: string;
  product_id?: number;
  inspection_type: string;
  interval_days: number;
  last_inspection?: string;
  next_inspection?: string;
  is_active: boolean;
  product_name?: string;
  device_name?: string;
}

export interface MaintenanceStats {
  open_defects: number;
  in_progress_defects: number;
  repaired_defects: number;
  overdue_inspections: number;
  upcoming_inspections: number;
}

export const maintenanceApi = {
  getStats: () => api.get<MaintenanceStats>('/maintenance/stats'),
  getDefects: (params?: { status?: string; severity?: string }) =>
    api.get<Defect[]>('/defects', { params }),
  createDefect: (data: {
    device_id: string;
    severity: string;
    title: string;
    description: string;
  }) => api.post<{ defect_id: number; message: string }>('/defects', data),
  updateDefect: (id: number, data: {
    status?: string;
    repair_cost?: number;
    repair_notes?: string;
  }) => api.put(`/defects/${id}`, data),
  getInspections: (params?: { status?: string }) =>
    api.get<Inspection[]>('/maintenance/inspections', { params }),
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
  testBin: (shelfId: string, binId: string) =>
    api.post(`/led/test?shelf_id=${shelfId}&bin_id=${binId}`),
  locateBin: (binCode: string) =>
    api.post(`/led/locate?bin_code=${binCode}`),
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
  configureController: (id: number, config: { led_count?: number; data_pin?: number; chipset?: string }) =>
    api.post(`/admin/led/controllers/${id}/configure`, config),
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
  created_at?: string;
  updated_at?: string;
}

export interface LabelElement {
  type: 'barcode' | 'qrcode' | 'text' | 'image';
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
  content: string;
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
  generateQRCode: (content: string, size: number = 256) =>
    api.post<{ image_data: string }>('/labels/qrcode', { content, size }),
  generateBarcode: (content: string, width: number = 300, height: number = 100) =>
    api.post<{ image_data: string }>('/labels/barcode', { content, width, height }),
  getTemplates: () => api.get<LabelTemplate[]>('/labels/templates'),
  getTemplate: (id: number) => api.get<LabelTemplate>(`/labels/templates/${id}`),
  createTemplate: (template: LabelTemplate) => api.post<LabelTemplate>('/labels/templates', template),
  updateTemplate: (id: number, updates: Partial<LabelTemplate>) => api.put(`/labels/templates/${id}`, updates),
  deleteTemplate: (id: number) => api.delete(`/labels/templates/${id}`),
  generateDeviceLabel: (deviceId: string, templateId: number) =>
    api.post(`/labels/device/${deviceId}`, { template_id: templateId }),
  generateCaseLabel: (caseId: number, templateId: number) =>
    api.post(`/labels/case/${caseId}`, { template_id: templateId }),
  saveLabel: (deviceId: string, imageData: string) =>
    api.post<{ label_path: string; message: string }>('/labels/save', { device_id: deviceId, image_data: imageData }),
  saveCaseLabel: (caseId: number, imageData: string) =>
    api.post<{ label_path: string; message: string }>('/labels/save-case', { case_id: caseId, image_data: imageData }),
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
  create: (data: CableCreateInput) => api.post<{cable_id: number; message: string}>('/admin/cables', data),
  update: (id: number, data: CableUpdateInput) => api.put<{message: string}>(`/admin/cables/${id}`, data),
  delete: (id: number) => api.delete<{message: string}>(`/admin/cables/${id}`),
  setStock: (id: number, data: { zone_id?: number | null; quantity: number }) =>
    api.put<{message: string}>(`/admin/cables/${id}/stock`, data),
  createUnits: (id: number, data: { zone_id?: number | null; quantity: number }) =>
    api.post<{created_count: number; message: string}>(`/admin/cables/${id}/units`, data),
  deleteUnit: (id: number, deviceId: string) =>
    api.delete<{message: string}>(`/admin/cables/${id}/units/${encodeURIComponent(deviceId)}`),
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
    Array.from(files as ArrayLike<File>).forEach(file => data.append('files', file));
    return api.post(`/admin/products/${productId}/pictures`, data, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
  },
  delete: (productId: number, fileName: string) =>
    api.delete(`/admin/products/${productId}/pictures/${encodeURIComponent(fileName)}`),
};

export const productWebsiteApi = {
  update: (productId: number, payload: { website_visible: boolean; website_images: string[]; website_thumbnail?: string | null }) =>
    api.put(`/admin/products/${productId}/website`, payload),
};
