import { useCallback, useEffect, useState } from 'react';
import {
  Download,
  Eye,
  LayoutGrid,
  List,
  Package,
  Pencil,
  Plus,
  QrCode,
  RefreshCcw,
  Search,
  Trash2,
  X,
} from 'lucide-react';
import { api, devicesAdminApi, labelsApi } from '../../lib/api';
import type { Device, DeviceCreateInput, DeviceUpdateInput, LabelTemplate } from '../../lib/api';
import { useBlockBodyScroll } from '../../hooks/useBlockBodyScroll';
import { toast } from '../../lib/toast';

interface Product {
  product_id: number;
  name: string;
  category_name?: string | null;
  subcategory_name?: string | null;
}

interface Zone {
  zone_id: number;
  code: string;
  name: string;
}

interface DeviceFormData {
  product_id?: number;
  status: string;
  serial_number: string;
  barcode: string;
  qr_code: string;
  current_location: string;
  zone_id?: number;
  condition_rating?: number;
  usage_hours?: number;
  purchase_date: string;
  last_maintenance: string;
  next_maintenance: string;
  notes: string;
  quantity: number;
  device_prefix: string;
  increment_serial: boolean;
  auto_generate_label: boolean;
  label_template_id?: number;
  regenerate_codes: boolean;
  regenerate_label: boolean;
}

const initialFormData: DeviceFormData = {
  status: 'free',
  serial_number: '',
  barcode: '',
  qr_code: '',
  current_location: '',
  notes: '',
  quantity: 1,
  device_prefix: '',
  increment_serial: false,
  purchase_date: '',
  last_maintenance: '',
  next_maintenance: '',
  auto_generate_label: true,
  regenerate_codes: false,
  regenerate_label: false,
};

function useDebouncedValue<T>(value: T, delay: number) {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const handle = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(handle);
  }, [value, delay]);

  return debounced;
}

export function DevicesTab() {
  const [devices, setDevices] = useState<Device[]>([]);
  const [loadingDevices, setLoadingDevices] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [viewDevice, setViewDevice] = useState<Device | null>(null);
  const [editingDevice, setEditingDevice] = useState<string | null>(null);
  const [formData, setFormData] = useState<DeviceFormData>(initialFormData);
  const [products, setProducts] = useState<Product[]>([]);
  const [zones, setZones] = useState<Zone[]>([]);
  const [labelTemplates, setLabelTemplates] = useState<LabelTemplate[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [viewMode, setViewMode] = useState<'table' | 'cards'>('table');
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [productFilter, setProductFilter] = useState<number | ''>('');
  const [zoneFilter, setZoneFilter] = useState<number | ''>('');
  const [refreshing, setRefreshing] = useState(false);

  // Block body scroll when any modal is open
  useBlockBodyScroll(modalOpen || viewDevice !== null);

  const debouncedSearch = useDebouncedValue(searchTerm, 300);

  const fetchDevices = useCallback(async () => {
    setLoadingDevices(true);
    try {
      const { data } = await api.get<Device[]>('/admin/devices-list');
      setDevices(data || []);
    } catch (error) {
      toast.error('Failed to load devices:' + " " + String(error));
      setDevices([]);
    } finally {
      setLoadingDevices(false);
    }
  }, []);

  const loadMetadata = useCallback(async () => {
    try {
      const [productsRes, zonesRes, templatesRes] = await Promise.all([
        api.get<Product[]>('/admin/products'),
        api.get<Zone[]>('/zones'),
        labelsApi.getTemplates(),
      ]);

      setProducts(productsRes.data || []);
      setZones(zonesRes.data || []);
      setLabelTemplates(templatesRes.data || []);
    } catch (error) {
      toast.error('Failed to load metadata:' + " " + String(error));
    }
  }, []);

  useEffect(() => {
    fetchDevices();
    loadMetadata();
  }, [fetchDevices, loadMetadata]);

  const handleRefresh = useCallback(async () => {
    setRefreshing(true);
    await fetchDevices();
    setRefreshing(false);
  }, [fetchDevices]);

  const clearFilters = () => {
    setSearchTerm('');
    setStatusFilter('');
    setProductFilter('');
    setZoneFilter('');
  };

  const filteredDevices = devices.filter((device) => {
    const matchesSearch =
      !debouncedSearch ||
      device.device_id.toLowerCase().includes(debouncedSearch.toLowerCase()) ||
      device.product_name?.toLowerCase().includes(debouncedSearch.toLowerCase()) ||
      device.product_category?.toLowerCase().includes(debouncedSearch.toLowerCase()) ||
      device.serial_number?.toLowerCase().includes(debouncedSearch.toLowerCase()) ||
      device.barcode?.toLowerCase().includes(debouncedSearch.toLowerCase()) ||
      device.notes?.toLowerCase().includes(debouncedSearch.toLowerCase());

    const matchesStatus = !statusFilter || device.status === statusFilter;
    const matchesProduct =
      productFilter === '' || device.product_id === productFilter;
    const matchesZone = zoneFilter === '' || device.zone_id === zoneFilter;

    return matchesSearch && matchesStatus && matchesProduct && matchesZone;
  });

  const openCreateModal = () => {
    setEditingDevice(null);
    setFormData({ ...initialFormData, label_template_id: undefined });
    setModalOpen(true);
  };

  const openEditModal = (device: Device) => {
    setEditingDevice(device.device_id);
    setFormData({
      product_id: device.product_id,
      status: device.status || 'free',
      serial_number: device.serial_number || '',
      barcode: device.barcode || '',
      qr_code: device.qr_code || '',
      current_location: device.current_location || '',
      zone_id: device.zone_id,
      condition_rating: device.condition_rating,
      usage_hours: device.usage_hours,
      purchase_date: device.purchase_date || '',
      last_maintenance: device.last_maintenance || '',
      next_maintenance: device.next_maintenance || '',
      notes: device.notes || '',
      quantity: 1,
      device_prefix: '',
      increment_serial: false,
      auto_generate_label: true,
      regenerate_codes: false,
      regenerate_label: false,
      label_template_id: undefined,
    });
    setModalOpen(true);
  };

  const handleDelete = async (deviceId: string) => {
    if (!window.confirm('Möchten Sie dieses Gerät wirklich löschen?')) {
      return;
    }

    try {
      await devicesAdminApi.delete(deviceId);
      await fetchDevices();
    } catch (error: unknown) {
      toast.error('Failed to delete device:' + " " + String(error));
      alert('Fehler beim Löschen des Geräts');
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);

    try {
      if (editingDevice) {
        // Update existing device
        const updateData: DeviceUpdateInput = {
          product_id: formData.product_id,
          status: formData.status,
          serial_number: formData.serial_number || undefined,
          barcode: formData.barcode || undefined,
          qr_code: formData.qr_code || undefined,
          current_location: formData.current_location || undefined,
          zone_id: formData.zone_id,
          condition_rating: formData.condition_rating,
          usage_hours: formData.usage_hours,
          notes: formData.notes || undefined,
          purchase_date: formData.purchase_date || undefined,
          last_maintenance: formData.last_maintenance || undefined,
          next_maintenance: formData.next_maintenance || undefined,
        };

        if (formData.regenerate_codes) {
          updateData.regenerate_codes = true;
        }
        if (formData.label_template_id) {
          updateData.label_template_id = formData.label_template_id;
        }
        if (formData.regenerate_label || formData.label_template_id) {
          updateData.regenerate_label = true;
        }

        await devicesAdminApi.update(editingDevice, updateData);
      } else {
        // Create new device(s)
        const createData: DeviceCreateInput = {
          product_id: formData.product_id!,
          status: formData.status,
          serial_number: formData.serial_number || undefined,
          barcode: formData.barcode || undefined,
          qr_code: formData.qr_code || undefined,
          current_location: formData.current_location || undefined,
          zone_id: formData.zone_id,
          condition_rating: formData.condition_rating,
          usage_hours: formData.usage_hours,
          notes: formData.notes || undefined,
          purchase_date: formData.purchase_date || undefined,
          last_maintenance: formData.last_maintenance || undefined,
          next_maintenance: formData.next_maintenance || undefined,
          quantity: formData.quantity,
          device_prefix: formData.device_prefix || undefined,
          increment_serial: formData.increment_serial,
        };

        createData.auto_generate_label = formData.auto_generate_label;
        if (formData.label_template_id) {
          createData.label_template_id = formData.label_template_id;
        }
        if (formData.regenerate_codes) {
          createData.regenerate_codes = true;
        }

        await devicesAdminApi.create(createData);
      }

      setModalOpen(false);
      setFormData({ ...initialFormData, label_template_id: undefined });
      await fetchDevices();
    } catch (error: unknown) {
      toast.error('Failed to save device:' + " " + String(error));
      alert('Fehler beim Speichern des Geräts');
    } finally {
      setSubmitting(false);
    }
  };

  const handleViewDevice = async (device: Device) => {
    try {
      const { data } = await devicesAdminApi.getById(device.device_id);
      setViewDevice(data);
    } catch (error) {
      toast.error('Failed to load device details:' + " " + String(error));
      setViewDevice(device);
    }
  };

  const downloadQR = (deviceId: string) => {
    window.open(devicesAdminApi.downloadQR(deviceId), '_blank');
  };

  const downloadBarcode = (deviceId: string) => {
    window.open(devicesAdminApi.downloadBarcode(deviceId), '_blank');
  };

  const openLabel = (labelPath?: string) => {
    if (!labelPath) {
      return;
    }
    window.open(labelPath, '_blank');
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Package className="w-6 h-6 text-accent-red" />
          <h2 className="text-2xl font-bold text-white">Geräte-Verwaltung</h2>
        </div>
        <button
          onClick={openCreateModal}
          className="btn-primary flex items-center gap-2"
        >
          <Plus className="w-5 h-5" />
          Neues Gerät
        </button>
      </div>

      {/* Filters */}
      <div className="glass-dark rounded-xl p-4 space-y-4">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
          {/* Search */}
          <div className="relative lg:col-span-2">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-5 h-5 text-gray-400" />
            <input
              type="text"
              placeholder="Suchen (ID, Produkt, Serial, Barcode)..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="input-field pl-10 w-full"
            />
          </div>

          {/* Status Filter */}
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="input-field"
          >
            <option value="">Alle Status</option>
            <option value="free">Frei</option>
            <option value="on_job">Im Einsatz</option>
            <option value="defective">Defekt</option>
            <option value="maintenance">Wartung</option>
            <option value="retired">Ausgemustert</option>
          </select>

          {/* Product Filter */}
          <select
            value={productFilter}
            onChange={(e) => setProductFilter(e.target.value ? Number(e.target.value) : '')}
            className="input-field"
          >
            <option value="">Alle Produkte</option>
            {products.map((product) => (
              <option key={product.product_id} value={product.product_id}>
                {product.name}
              </option>
            ))}
          </select>

          {/* Zone Filter */}
          <select
            value={zoneFilter}
            onChange={(e) => setZoneFilter(e.target.value ? Number(e.target.value) : '')}
            className="input-field"
          >
            <option value="">Alle Zonen</option>
            {zones.map((zone) => (
              <option key={zone.zone_id} value={zone.zone_id}>
                {zone.code} - {zone.name}
              </option>
            ))}
          </select>
        </div>

        {/* Action Buttons */}
        <div className="flex items-center justify-between">
          <div className="flex gap-2">
            <button
              onClick={clearFilters}
              className="px-4 py-2 bg-white/5 hover:bg-white/10 rounded-lg text-sm text-gray-300 transition-colors"
            >
              <X className="w-4 h-4 inline mr-1" />
              Filter löschen
            </button>
            <button
              onClick={handleRefresh}
              disabled={refreshing}
              className="px-4 py-2 bg-white/5 hover:bg-white/10 rounded-lg text-sm text-gray-300 transition-colors disabled:opacity-50"
            >
              <RefreshCcw className={`w-4 h-4 inline mr-1 ${refreshing ? 'animate-spin' : ''}`} />
              Aktualisieren
            </button>
          </div>

          {/* View Mode Toggle */}
          <div className="flex gap-2">
            <button
              onClick={() => setViewMode('table')}
              className={`p-2 rounded-lg transition-colors ${
                viewMode === 'table' ? 'bg-accent-red text-white' : 'bg-white/5 text-gray-400 hover:bg-white/10'
              }`}
            >
              <List className="w-5 h-5" />
            </button>
            <button
              onClick={() => setViewMode('cards')}
              className={`p-2 rounded-lg transition-colors ${
                viewMode === 'cards' ? 'bg-accent-red text-white' : 'bg-white/5 text-gray-400 hover:bg-white/10'
              }`}
            >
              <LayoutGrid className="w-5 h-5" />
            </button>
          </div>
        </div>
      </div>

      {/* Device List */}
      {loadingDevices ? (
        <div className="text-center py-12 text-gray-400">Lädt Geräte...</div>
      ) : filteredDevices.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          {debouncedSearch || statusFilter || productFilter || zoneFilter
            ? 'Keine Geräte gefunden mit den aktuellen Filtern'
            : 'Noch keine Geräte vorhanden'}
        </div>
      ) : viewMode === 'table' ? (
        <div className="glass-dark rounded-xl overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-white/5">
                <tr>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">ID</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Produkt</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Serial</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Status</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Zone</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Zustand</th>
                  <th className="px-4 py-3 text-right text-sm font-semibold text-gray-300">Aktionen</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {filteredDevices.map((device) => (
                  <tr key={device.device_id} className="hover:bg-white/5 transition-colors">
                    <td className="px-4 py-3 text-sm text-gray-300">{device.device_id}</td>
                    <td className="px-4 py-3 text-sm">
                      <div className="flex flex-col">
                        <span className="text-white">{device.product_name || '-'}</span>
                        {device.product_category && (
                          <span className="text-xs text-gray-400">{device.product_category}</span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-300">{device.serial_number || '-'}</td>
                    <td className="px-4 py-3">
                      <span
                        className={`px-2 py-1 rounded-full text-xs font-semibold ${
                          device.status === 'free'
                            ? 'bg-green-500/20 text-green-400'
                            : device.status === 'on_job'
                            ? 'bg-blue-500/20 text-blue-400'
                            : device.status === 'defective'
                            ? 'bg-red-500/20 text-red-400'
                            : 'bg-yellow-500/20 text-yellow-400'
                        }`}
                      >
                        {device.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-300">
                      {device.zone_code ? `${device.zone_code} - ${device.zone_name}` : '-'}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-300">
                      {device.condition_rating ? `${device.condition_rating}/10` : '-'}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => handleViewDevice(device)}
                          className="p-1.5 hover:bg-white/10 rounded-lg transition-colors text-gray-400 hover:text-white"
                          title="Details anzeigen"
                        >
                          <Eye className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => downloadQR(device.device_id)}
                          className="p-1.5 hover:bg-white/10 rounded-lg transition-colors text-gray-400 hover:text-white"
                          title="QR-Code herunterladen"
                        >
                          <QrCode className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => downloadBarcode(device.device_id)}
                          className="p-1.5 hover:bg-white/10 rounded-lg transition-colors text-gray-400 hover:text-white"
                          title="Barcode herunterladen"
                        >
                          <Download className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => openEditModal(device)}
                          className="p-1.5 hover:bg-white/10 rounded-lg transition-colors text-blue-400 hover:text-blue-300"
                          title="Bearbeiten"
                        >
                          <Pencil className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => handleDelete(device.device_id)}
                          className="p-1.5 hover:bg-white/10 rounded-lg transition-colors text-red-400 hover:text-red-300"
                          title="Löschen"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredDevices.map((device) => (
            <div key={device.device_id} className="glass-dark rounded-xl p-4 space-y-3">
            <div className="flex items-start justify-between">
              <div>
                <h3 className="font-bold text-white">{device.device_id}</h3>
                <p className="text-sm text-gray-400">{device.product_name || 'Unbekanntes Produkt'}</p>
                {device.product_category && (
                  <p className="text-xs text-gray-500">{device.product_category}</p>
                )}
              </div>
                <span
                  className={`px-2 py-1 rounded-full text-xs font-semibold ${
                    device.status === 'free'
                      ? 'bg-green-500/20 text-green-400'
                      : device.status === 'on_job'
                      ? 'bg-blue-500/20 text-blue-400'
                      : device.status === 'defective'
                      ? 'bg-red-500/20 text-red-400'
                      : 'bg-yellow-500/20 text-yellow-400'
                  }`}
                >
                  {device.status}
                </span>
              </div>

              <div className="space-y-1 text-sm">
                {device.serial_number && (
                  <p className="text-gray-300">
                    <span className="text-gray-500">Serial:</span> {device.serial_number}
                  </p>
                )}
                {device.zone_code && (
                  <p className="text-gray-300">
                    <span className="text-gray-500">Zone:</span> {device.zone_code} - {device.zone_name}
                  </p>
                )}
                {device.condition_rating && (
                  <p className="text-gray-300">
                    <span className="text-gray-500">Zustand:</span> {device.condition_rating}/10
                  </p>
                )}
                {device.purchase_date && (
                  <p className="text-gray-300">
                    <span className="text-gray-500">Kauf:</span> {device.purchase_date}
                  </p>
                )}
                {device.usage_hours && (
                  <p className="text-gray-300">
                    <span className="text-gray-500">Stunden:</span> {device.usage_hours}h
                  </p>
                )}
              </div>

              <div className="flex items-center gap-2 pt-2 border-t border-white/10">
                <button
                  onClick={() => handleViewDevice(device)}
                  className="flex-1 px-3 py-2 bg-white/5 hover:bg-white/10 rounded-lg text-sm text-gray-300 transition-colors flex items-center justify-center gap-2"
                >
                  <Eye className="w-4 h-4" />
                  Details
                </button>
                <button
                  onClick={() => openEditModal(device)}
                  className="flex-1 px-3 py-2 bg-blue-500/20 hover:bg-blue-500/30 rounded-lg text-sm text-blue-400 transition-colors flex items-center justify-center gap-2"
                >
                  <Pencil className="w-4 h-4" />
                  Bearbeiten
                </button>
                <button
                  onClick={() => handleDelete(device.device_id)}
                  className="px-3 py-2 bg-red-500/20 hover:bg-red-500/30 rounded-lg text-sm text-red-400 transition-colors"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create/Edit Modal */}
      {modalOpen && (
        <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
          <div className="glass-dark rounded-2xl p-6 max-w-2xl w-full max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between mb-6">
              <h3 className="text-2xl font-bold text-white">
                {editingDevice ? 'Gerät bearbeiten' : 'Neues Gerät erstellen'}
              </h3>
              <button
                onClick={() => setModalOpen(false)}
                className="p-2 hover:bg-white/10 rounded-lg transition-colors"
              >
                <X className="w-6 h-6 text-gray-400" />
              </button>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              {/* Product Selection */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Produkt *
                </label>
                <select
                  value={formData.product_id || ''}
                  onChange={(e) =>
                    setFormData({ ...formData, product_id: e.target.value ? Number(e.target.value) : undefined })
                  }
                  className="input-field w-full"
                  required
                  disabled={!!editingDevice}
                >
                  <option value="">Produkt auswählen...</option>
                  {products.map((product) => (
                    <option key={product.product_id} value={product.product_id}>
                      {product.name}
                      {product.category_name && ` (${product.category_name})`}
                    </option>
                  ))}
                </select>
              </div>

              {/* Quantity (only for new devices) */}
              {!editingDevice && (
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-2">
                      Anzahl
                    </label>
                    <input
                      type="number"
                      min="1"
                      max="100"
                      value={formData.quantity}
                      onChange={(e) =>
                        setFormData({ ...formData, quantity: Number(e.target.value) || 1 })
                      }
                      className="input-field w-full"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-2">
                      Präfix (optional)
                    </label>
                    <input
                      type="text"
                      value={formData.device_prefix}
                      onChange={(e) =>
                        setFormData({ ...formData, device_prefix: e.target.value })
                      }
                      className="input-field w-full"
                      placeholder="z.B. LED-"
                    />
                  </div>
                </div>
              )}

              {!editingDevice && formData.quantity > 1 && (
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="increment_serial"
                    checked={formData.increment_serial}
                    onChange={(e) =>
                      setFormData({ ...formData, increment_serial: e.target.checked })
                    }
                    className="w-4 h-4"
                  />
                  <label htmlFor="increment_serial" className="text-sm text-gray-300">
                    Seriennummern automatisch durchnummerieren
                  </label>
                </div>
              )}

              {/* Status */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Status *
                  </label>
                  <select
                    value={formData.status}
                    onChange={(e) => setFormData({ ...formData, status: e.target.value })}
                    className="input-field w-full"
                    required
                  >
                    <option value="free">Frei</option>
                    <option value="on_job">Im Einsatz</option>
                    <option value="defective">Defekt</option>
                    <option value="maintenance">Wartung</option>
                    <option value="retired">Ausgemustert</option>
                  </select>
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Zone
                  </label>
                  <select
                    value={formData.zone_id || ''}
                    onChange={(e) =>
                      setFormData({ ...formData, zone_id: e.target.value ? Number(e.target.value) : undefined })
                    }
                    className="input-field w-full"
                  >
                    <option value="">Keine Zone</option>
                    {zones.map((zone) => (
                      <option key={zone.zone_id} value={zone.zone_id}>
                        {zone.code} - {zone.name}
                      </option>
                    ))}
                  </select>
                </div>
              </div>

              {/* Serial, Barcode, QR */}
              <div className="grid grid-cols-3 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Seriennummer
                  </label>
                  <input
                    type="text"
                    value={formData.serial_number}
                    onChange={(e) => setFormData({ ...formData, serial_number: e.target.value })}
                    className="input-field w-full"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Barcode
                  </label>
                  <input
                    type="text"
                    value={formData.barcode}
                    onChange={(e) => setFormData({ ...formData, barcode: e.target.value })}
                    className="input-field w-full"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    QR-Code
                  </label>
                  <input
                    type="text"
                    value={formData.qr_code}
                    onChange={(e) => setFormData({ ...formData, qr_code: e.target.value })}
                    className="input-field w-full"
                  />
                </div>
              </div>

              {/* Location and Condition */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Aktueller Standort
                  </label>
                  <input
                    type="text"
                    value={formData.current_location}
                    onChange={(e) => setFormData({ ...formData, current_location: e.target.value })}
                    className="input-field w-full"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Zustand (1-10)
                  </label>
                  <input
                    type="number"
                    min="0"
                    max="10"
                    step="0.1"
                    value={formData.condition_rating || ''}
                    onChange={(e) =>
                      setFormData({
                        ...formData,
                        condition_rating: e.target.value ? Number(e.target.value) : undefined,
                      })
                    }
                    className="input-field w-full"
                  />
              </div>
              </div>

              {/* Dates */}
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Kaufdatum
                  </label>
                  <input
                    type="date"
                    value={formData.purchase_date}
                    onChange={(e) => setFormData({ ...formData, purchase_date: e.target.value })}
                    className="input-field w-full"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Letzte Wartung
                  </label>
                  <input
                    type="date"
                    value={formData.last_maintenance}
                    onChange={(e) => setFormData({ ...formData, last_maintenance: e.target.value })}
                    className="input-field w-full"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Nächste Wartung
                  </label>
                  <input
                    type="date"
                    value={formData.next_maintenance}
                    onChange={(e) => setFormData({ ...formData, next_maintenance: e.target.value })}
                    className="input-field w-full"
                  />
                </div>
              </div>

              {/* Usage Hours */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Betriebsstunden
                </label>
                <input
                  type="number"
                  min="0"
                  step="0.1"
                  value={formData.usage_hours || ''}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      usage_hours: e.target.value ? Number(e.target.value) : undefined,
                    })
                  }
                  className="input-field w-full"
                />
              </div>

              {/* Notes */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Notizen
                </label>
                <textarea
                  value={formData.notes}
                  onChange={(e) => setFormData({ ...formData, notes: e.target.value })}
                  className="input-field w-full"
                  rows={3}
                />
              </div>

              {/* Label & Code Options */}
              <div className="border-t border-white/10 pt-4 space-y-3">
                {editingDevice ? (
                  <>
                    <label className="flex items-center gap-2 text-sm text-gray-300">
                      <input
                        type="checkbox"
                        checked={formData.regenerate_codes}
                        onChange={(e) => setFormData({ ...formData, regenerate_codes: e.target.checked })}
                        className="w-4 h-4"
                      />
                      Barcodes/QR-Codes neu generieren
                    </label>
                    <label className="flex items-center gap-2 text-sm text-gray-300">
                      <input
                        type="checkbox"
                        checked={formData.regenerate_label}
                        onChange={(e) => setFormData({ ...formData, regenerate_label: e.target.checked })}
                        className="w-4 h-4"
                      />
                      Label mit aktueller Vorlage neu rendern
                    </label>
                  </>
                ) : (
                  <label className="flex items-center gap-2 text-sm text-gray-300">
                    <input
                      type="checkbox"
                      checked={formData.auto_generate_label}
                      onChange={(e) => setFormData({ ...formData, auto_generate_label: e.target.checked })}
                      className="w-4 h-4"
                    />
                    Label nach Erstellung automatisch speichern
                  </label>
                )}

                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Label-Vorlage
                  </label>
                  <select
                    value={formData.label_template_id ?? ''}
                    onChange={(e) =>
                      setFormData({
                        ...formData,
                        label_template_id: e.target.value ? Number(e.target.value) : undefined,
                      })
                    }
                    className="input-field w-full"
                  >
                    <option value="">Standard (Default)</option>
                    {labelTemplates.map((template) => (
                      <option key={template.id} value={template.id}>
                        {template.name} {template.is_default ? '(Standard)' : ''}
                      </option>
                    ))}
                  </select>
                  <p className="text-xs text-gray-500 mt-1">
                    Wird eine Vorlage gewählt, wird das Label mit dieser Vorlage erzeugt bzw. neu gerendert.
                  </p>
                </div>
              </div>

              {/* Submit Buttons */}
              <div className="flex gap-3 pt-4">
                <button
                  type="button"
                  onClick={() => setModalOpen(false)}
                  className="flex-1 px-4 py-3 bg-white/5 hover:bg-white/10 rounded-lg font-semibold text-gray-300 transition-colors"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={submitting}
                  className="flex-1 btn-primary disabled:opacity-50"
                >
                  {submitting
                    ? 'Speichert...'
                    : editingDevice
                    ? 'Aktualisieren'
                    : 'Erstellen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* View Device Modal */}
      {viewDevice && (
        <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
          <div className="glass-dark rounded-2xl p-6 max-w-2xl w-full max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between mb-6">
              <h3 className="text-2xl font-bold text-white">Geräte-Details</h3>
              <button
                onClick={() => setViewDevice(null)}
                className="p-2 hover:bg-white/10 rounded-lg transition-colors"
              >
                <X className="w-6 h-6 text-gray-400" />
              </button>
            </div>

            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-sm text-gray-400">Geräte-ID</p>
                  <p className="text-white font-semibold">{viewDevice.device_id}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-400">Produkt</p>
                  <p className="text-white font-semibold">{viewDevice.product_name || '-'}</p>
                </div>
                {viewDevice.product_category && (
                  <div>
                    <p className="text-sm text-gray-400">Kategorie</p>
                    <p className="text-white font-semibold">{viewDevice.product_category}</p>
                  </div>
                )}
                <div>
                  <p className="text-sm text-gray-400">Status</p>
                  <p className="text-white font-semibold">{viewDevice.status}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-400">Seriennummer</p>
                  <p className="text-white font-semibold">{viewDevice.serial_number || '-'}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-400">Barcode</p>
                  <p className="text-white font-semibold">{viewDevice.barcode || '-'}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-400">QR-Code</p>
                  <p className="text-white font-semibold">{viewDevice.qr_code || '-'}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-400">Zone</p>
                  <p className="text-white font-semibold">
                    {viewDevice.zone_code ? `${viewDevice.zone_code} - ${viewDevice.zone_name}` : '-'}
                  </p>
                </div>
                <div>
                  <p className="text-sm text-gray-400">Standort</p>
                  <p className="text-white font-semibold">{viewDevice.current_location || '-'}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-400">Zustand</p>
                  <p className="text-white font-semibold">
                    {viewDevice.condition_rating ? `${viewDevice.condition_rating}/10` : '-'}
                  </p>
                </div>
                <div>
                  <p className="text-sm text-gray-400">Betriebsstunden</p>
                  <p className="text-white font-semibold">
                    {viewDevice.usage_hours ? `${viewDevice.usage_hours}h` : '-'}
                  </p>
                </div>
                {viewDevice.purchase_date && (
                  <div>
                    <p className="text-sm text-gray-400">Kaufdatum</p>
                    <p className="text-white font-semibold">
                      {viewDevice.purchase_date}
                    </p>
                  </div>
                )}
                {viewDevice.last_maintenance && (
                  <div>
                    <p className="text-sm text-gray-400">Letzte Wartung</p>
                    <p className="text-white font-semibold">
                      {viewDevice.last_maintenance}
                    </p>
                  </div>
                )}
                {viewDevice.next_maintenance && (
                  <div>
                    <p className="text-sm text-gray-400">Nächste Wartung</p>
                    <p className="text-white font-semibold">
                      {viewDevice.next_maintenance}
                    </p>
                  </div>
                )}
                {viewDevice.case_name && (
                  <div>
                    <p className="text-sm text-gray-400">Case</p>
                    <p className="text-white font-semibold">{viewDevice.case_name}</p>
                  </div>
                )}
                {viewDevice.job_number && (
                  <div>
                    <p className="text-sm text-gray-400">Job</p>
                    <p className="text-white font-semibold">{viewDevice.job_number}</p>
                  </div>
                )}
              </div>

              {viewDevice.notes && (
                <div className="border border-white/10 rounded-xl p-4 text-sm text-gray-300 bg-white/5">
                  <p className="font-semibold text-white mb-1">Notizen</p>
                  <p className="whitespace-pre-line">{viewDevice.notes}</p>
                </div>
              )}

              <div className="flex flex-wrap gap-3 pt-4">
                <button
                  onClick={() => downloadQR(viewDevice.device_id)}
                  className="flex-1 px-4 py-3 bg-white/5 hover:bg-white/10 rounded-lg font-semibold text-gray-300 transition-colors flex items-center justify-center gap-2"
                >
                  <QrCode className="w-5 h-5" />
                  QR-Code
                </button>
                <button
                  onClick={() => downloadBarcode(viewDevice.device_id)}
                  className="flex-1 px-4 py-3 bg-white/5 hover:bg-white/10 rounded-lg font-semibold text-gray-300 transition-colors flex items-center justify-center gap-2"
                >
                  <Download className="w-5 h-5" />
                  Barcode
                </button>
                {viewDevice.label_path && (
                  <button
                    onClick={() => openLabel(viewDevice.label_path)}
                    className="flex-1 px-4 py-3 bg-white/5 hover:bg-white/10 rounded-lg font-semibold text-gray-300 transition-colors flex items-center justify-center gap-2"
                  >
                    <Download className="w-5 h-5" />
                    Label
                  </button>
                )}
                <button
                  onClick={() => {
                    setViewDevice(null);
                    openEditModal(viewDevice);
                  }}
                  className="flex-1 btn-primary flex items-center justify-center gap-2"
                >
                  <Pencil className="w-5 h-5" />
                  Bearbeiten
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
