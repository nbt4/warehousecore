import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { Package, Search, MapPin, Box, RefreshCw, Plus, Edit2, Trash2, X, Printer } from 'lucide-react';
import {
  casesApi,
  devicesApi,
  ledApi,
  zonesApi,
  labelsApi,
  type CaseSummary,
  type CaseDetail,
  type CaseDevice,
  type Device,
  type Zone,
} from '../lib/api';
import { formatStatus, getStatusColor } from '../lib/utils';
import { CaseDetailModal } from '../components/CaseDetailModal';
import { DeviceDetailModal } from '../components/DeviceDetailModal';
import { DeviceTreeModal } from '../components/DeviceTreeModal';
import { useBlockBodyScroll } from '../hooks/useBlockBodyScroll';
import { toast } from '../lib/toast';

type StatusFilter = 'all' | 'free' | 'rented' | 'maintance';

interface ActionMessage {
  type: 'success' | 'error';
  text: string;
}

interface CaseFormData {
  name: string;
  description?: string;
  width?: number;
  height?: number;
  depth?: number;
  weight?: number;
  status: 'free' | 'rented' | 'maintance' | '';
  zone_id?: number;
  barcode?: string;
  rfid_tag?: string;
}

export function CasesPage() {
  const navigate = useNavigate();
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [selectedCase, setSelectedCase] = useState<CaseDetail | null>(null);
  const [caseDevices, setCaseDevices] = useState<CaseDevice[]>([]);
  const [caseModalOpen, setCaseModalOpen] = useState(false);
  const [caseModalLoading, setCaseModalLoading] = useState(false);
  const [selectedDevice, setSelectedDevice] = useState<Device | null>(null);
  const [deviceModalOpen, setDeviceModalOpen] = useState(false);
  const [actionMessage, setActionMessage] = useState<ActionMessage | null>(null);
  const [deviceTreeModalOpen, setDeviceTreeModalOpen] = useState(false);
  const [deviceTreeModalContext, setDeviceTreeModalContext] = useState<'detail' | 'edit'>('detail');

  // Form modal state
  const [formModalOpen, setFormModalOpen] = useState(false);
  const [editingCaseId, setEditingCaseId] = useState<number | null>(null);
  const [editingCaseDevices, setEditingCaseDevices] = useState<CaseDevice[]>([]);
  const [zones, setZones] = useState<Zone[]>([]);
  const [formData, setFormData] = useState<CaseFormData>({
    name: '',
    description: '',
    status: 'free',
  });
  const [submitting, setSubmitting] = useState(false);

  // Block body scroll when form modal is open
  useBlockBodyScroll(formModalOpen);

  useEffect(() => {
    void loadCases();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusFilter]);

  const totalDevices = useMemo(
    () => cases.reduce((sum, current) => sum + current.device_count, 0),
    [cases]
  );

  const statusCounts = useMemo(() => {
    return cases.reduce<Record<string, number>>((acc, current) => {
      acc[current.status] = (acc[current.status] ?? 0) + 1;
      return acc;
    }, {});
  }, [cases]);

  const loadCases = async (overrideSearch?: string) => {
    setLoading(true);
    try {
      const params: { search?: string; status?: string } = {};
      const effectiveSearch = overrideSearch ?? search.trim();
      if (effectiveSearch) {
        params.search = effectiveSearch;
      }
      if (statusFilter !== 'all') {
        params.status = statusFilter;
      }
      const { data } = await casesApi.list(params);
      setCases(data.cases ?? []);
    } catch (error) {
      toast.error('Failed to load cases:' + " " + String(error));
      setCases([]);
    } finally {
      setLoading(false);
    }
  };

  const handleSearchSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    await loadCases();
  };

  const handleResetFilters = async () => {
    setSearch('');
    setStatusFilter('all');
    await loadCases('');
  };

  const handleOpenCase = async (caseSummary: CaseSummary) => {
    setCaseModalLoading(true);
    try {
      const [detailRes, devicesRes] = await Promise.all([
        casesApi.getById(caseSummary.case_id),
        casesApi.getDevices(caseSummary.case_id),
      ]);
      setSelectedCase(detailRes.data);
      setCaseDevices(devicesRes.data);
      setCaseModalOpen(true);
    } catch (error) {
      toast.error('Failed to load case:' + " " + String(error));
    } finally {
      setCaseModalLoading(false);
    }
  };

  const handleLocateDevice = async (device: CaseDevice) => {
    if (!device.zone_code) {
      setActionMessage({ type: 'error', text: 'Kein Fachcode vorhanden – Gerät nicht im Lager.' });
      clearActionMessage();
      return;
    }

    try {
      await ledApi.locateBin(device.zone_code);
      setActionMessage({
        type: 'success',
        text: `Fach ${device.zone_code} wird hervorgehoben.`,
      });
    } catch (error: any) {
      setActionMessage({
        type: 'error',
        text: 'LED-Befehl fehlgeschlagen: ' + (error.response?.data?.error || error.message || error.toString()),
      });
    } finally {
      clearActionMessage();
    }
  };

  const handleOpenZone = (info: { zone_id?: number; zone_code?: string }) => {
    if (info.zone_id) {
      navigate(`/zones/${info.zone_id}`);
      return;
    }
    if (info.zone_code) {
      navigate(`/zones?parent=${info.zone_code}`);
    }
  };

  const handleOpenDevice = async (deviceId: string) => {
    try {
      const { data } = await devicesApi.getById(deviceId);
      setSelectedDevice(data);
      setDeviceModalOpen(true);
    } catch (error) {
      toast.error('Failed to load device:' + " " + String(error));
      setActionMessage({
        type: 'error',
        text: 'Gerätedetails konnten nicht geladen werden.',
      });
      clearActionMessage();
    }
  };

  const clearActionMessage = () => {
    setTimeout(() => setActionMessage(null), 4000);
  };

  const handleAddDevicesToCase = () => {
    setDeviceTreeModalContext('detail');
    setDeviceTreeModalOpen(true);
  };

  const handleAddDevicesToEditCase = () => {
    setDeviceTreeModalContext('edit');
    setDeviceTreeModalOpen(true);
  };

  const handleConfirmAddDevices = async (deviceIds: string[]) => {
    const caseId = deviceTreeModalContext === 'detail' ? selectedCase?.case_id : editingCaseId;
    if (!caseId) return;

    try {
      const { data } = await casesApi.addDevices(caseId, deviceIds);

      if (data.success_count > 0) {
        setActionMessage({
          type: 'success',
          text: `${data.success_count} Gerät(e) erfolgreich hinzugefügt`,
        });

        // Reload case devices
        const devicesRes = await casesApi.getDevices(caseId);

        if (deviceTreeModalContext === 'detail') {
          setCaseDevices(devicesRes.data);
        } else {
          setEditingCaseDevices(devicesRes.data);
        }

        void loadCases(); // Refresh case list
      }

      if (data.errors && data.errors.length > 0) {
        console.warn('Add devices errors:', data.errors);
      }
    } catch (error: any) {
      setActionMessage({
        type: 'error',
        text: 'Fehler beim Hinzufügen der Geräte: ' + (error.response?.data?.error || error.message),
      });
    } finally {
      clearActionMessage();
    }
  };

  const handleRemoveDeviceFromCase = async (deviceId: string) => {
    if (!selectedCase) return;

    try {
      await casesApi.removeDevice(selectedCase.case_id, deviceId);

      setActionMessage({
        type: 'success',
        text: `Gerät ${deviceId} erfolgreich entfernt`,
      });

      // Reload case devices
      const devicesRes = await casesApi.getDevices(selectedCase.case_id);
      setCaseDevices(devicesRes.data);
      void loadCases(); // Refresh case list
    } catch (error: any) {
      setActionMessage({
        type: 'error',
        text: 'Fehler beim Entfernen: ' + (error.response?.data?.error || error.message),
      });
    } finally {
      clearActionMessage();
    }
  };

  const handleRemoveDeviceFromEditCase = async (deviceId: string) => {
    if (!editingCaseId) return;

    try {
      await casesApi.removeDevice(editingCaseId, deviceId);

      setActionMessage({
        type: 'success',
        text: `Gerät ${deviceId} erfolgreich entfernt`,
      });

      // Reload case devices
      const devicesRes = await casesApi.getDevices(editingCaseId);
      setEditingCaseDevices(devicesRes.data);
      void loadCases(); // Refresh case list
    } catch (error: any) {
      setActionMessage({
        type: 'error',
        text: 'Fehler beim Entfernen: ' + (error.response?.data?.error || error.message),
      });
    } finally {
      clearActionMessage();
    }
  };

  const handleRefreshCaseDevices = async () => {
    if (!selectedCase) return;

    try {
      const devicesRes = await casesApi.getDevices(selectedCase.case_id);
      setCaseDevices(devicesRes.data);
      void loadCases();
    } catch (error) {
      toast.error('Failed to refresh case devices:' + " " + String(error));
    }
  };

  const loadZones = async () => {
    try {
      const { data } = await zonesApi.getAll();
      setZones(data);
    } catch (error) {
      toast.error('Failed to load zones:' + " " + String(error));
    }
  };

  const handleOpenCreateModal = () => {
    setEditingCaseId(null);
    setEditingCaseDevices([]);
    setFormData({
      name: '',
      description: '',
      status: 'free',
    });
    setFormModalOpen(true);
    void loadZones();
  };

  const handleOpenEditModal = async (caseItem: CaseSummary) => {
    setEditingCaseId(caseItem.case_id);
    setFormData({
      name: caseItem.name,
      description: caseItem.description || '',
      width: caseItem.width || undefined,
      height: caseItem.height || undefined,
      depth: caseItem.depth || undefined,
      weight: caseItem.weight || undefined,
      status: caseItem.status as 'free' | 'rented' | 'maintance' | '',
      zone_id: caseItem.zone_id || undefined,
      barcode: undefined,
      rfid_tag: undefined,
    });

    // Load devices for this case
    try {
      const devicesRes = await casesApi.getDevices(caseItem.case_id);
      setEditingCaseDevices(devicesRes.data);
    } catch (error) {
      toast.error('Failed to load case devices:' + " " + String(error));
      setEditingCaseDevices([]);
    }

    setFormModalOpen(true);
    void loadZones();
  };

  const handleFormSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!formData.name.trim()) {
      setActionMessage({ type: 'error', text: 'Name ist erforderlich' });
      clearActionMessage();
      return;
    }

    setSubmitting(true);
    try {
      if (editingCaseId) {
        await casesApi.update(editingCaseId, formData);
        setActionMessage({ type: 'success', text: 'Case erfolgreich aktualisiert' });
      } else {
        await casesApi.create(formData);
        setActionMessage({ type: 'success', text: 'Case erfolgreich erstellt' });
      }
      setFormModalOpen(false);
      await loadCases();
    } catch (error: any) {
      setActionMessage({
        type: 'error',
        text: error.response?.data?.error || 'Fehler beim Speichern',
      });
    } finally {
      setSubmitting(false);
      clearActionMessage();
    }
  };

  const handleDelete = async (caseId: number, caseName: string) => {
    if (!confirm(`Möchten Sie das Case "${caseName}" wirklich löschen?`)) {
      return;
    }

    try {
      await casesApi.delete(caseId);
      setActionMessage({ type: 'success', text: 'Case erfolgreich gelöscht' });
      await loadCases();
    } catch (error: any) {
      setActionMessage({
        type: 'error',
        text: error.response?.data?.message || error.response?.data?.error || 'Fehler beim Löschen',
      });
    } finally {
      clearActionMessage();
    }
  };

  const handlePrintLabel = async (caseId: number, caseName: string) => {
    try {
      // Get default template
      const { data: templates } = await labelsApi.getTemplates();
      const defaultTemplate = templates.find(t => t.is_default) || templates[0];

      if (!defaultTemplate) {
        alert('Kein Label-Template gefunden. Bitte erstellen Sie erst ein Template unter "Labels".');
        return;
      }

      // Generate label data
      const { data: labelData } = await labelsApi.generateCaseLabel(caseId, defaultTemplate.id!);

      // Open label preview in new window
      const labelWindow = window.open('', '_blank', 'width=800,height=600');
      if (!labelWindow) {
        alert('Popup wurde blockiert. Bitte erlauben Sie Popups für diese Seite.');
        return;
      }

      // Generate simple HTML for label preview
      const html = `
        <!DOCTYPE html>
        <html>
        <head>
          <title>Label - ${caseName}</title>
          <style>
            body {
              margin: 0;
              padding: 20px;
              font-family: Arial, sans-serif;
              background: #1a1a1a;
              color: white;
            }
            .container {
              max-width: 800px;
              margin: 0 auto;
            }
            .header {
              display: flex;
              justify-content: space-between;
              align-items: center;
              margin-bottom: 20px;
              padding-bottom: 20px;
              border-bottom: 2px solid #444;
            }
            .label-preview {
              background: white;
              border: 1px solid #ccc;
              padding: 20px;
              margin: 20px 0;
              position: relative;
            }
            .element {
              position: absolute;
            }
            .text {
              color: black;
              white-space: nowrap;
            }
            button {
              background: #dc2626;
              color: white;
              border: none;
              padding: 10px 20px;
              border-radius: 8px;
              cursor: pointer;
              font-size: 16px;
            }
            button:hover {
              background: #b91c1c;
            }
            @media print {
              .header, button { display: none; }
              body { background: white; padding: 0; }
            }
          </style>
        </head>
        <body>
          <div class="container">
            <div class="header">
              <h1>Label: ${caseName}</h1>
              <button onclick="window.print()">Drucken</button>
            </div>
            <div class="label-preview" style="width: ${labelData.template.width}mm; height: ${labelData.template.height}mm;">
              ${labelData.elements.map((elem: any) => {
                if (elem.type === 'text') {
                  return `<div class="element text" style="left: ${elem.x}mm; top: ${elem.y}mm; font-size: ${elem.style?.fontSize || 12}px; font-weight: ${elem.style?.fontWeight || 'normal'};">${elem.content || ''}</div>`;
                } else if (elem.type === 'qrcode' || elem.type === 'barcode' || elem.type === 'image') {
                  return `<img class="element" src="${elem.image_data}" style="left: ${elem.x}mm; top: ${elem.y}mm; width: ${elem.width}mm; height: ${elem.height}mm;" />`;
                }
                return '';
              }).join('')}
            </div>
          </div>
        </body>
        </html>
      `;

      labelWindow.document.write(html);
      labelWindow.document.close();
    } catch (error: any) {
      toast.error('Failed to generate label:' + " " + String(error));
      alert('Fehler beim Erstellen des Labels: ' + (error.response?.data?.error || error.message));
    }
  };

  return (
    <div className="space-y-4 sm:space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-2xl sm:text-3xl font-bold text-white mb-1 sm:mb-2">Cases</h2>
          <p className="text-sm sm:text-base text-gray-400">
            {cases.length} Cases • {totalDevices} Geräte gesamt
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleOpenCreateModal}
            className="px-4 py-2 rounded-lg text-sm font-semibold bg-accent-red/80 hover:bg-accent-red transition-colors text-white flex items-center gap-2"
          >
            <Plus className="w-4 h-4" />
            Neues Case
          </button>
          <button
            onClick={() => loadCases()}
            className="px-4 py-2 rounded-lg text-sm font-semibold bg-white/10 hover:bg-white/20 transition-colors text-white flex items-center gap-2"
          >
            <RefreshCw className="w-4 h-4" />
            Aktualisieren
          </button>
        </div>
      </div>

      <form onSubmit={handleSearchSubmit} className="glass rounded-xl sm:rounded-2xl p-3 sm:p-4 space-y-3 sm:space-y-0 sm:flex sm:items-center sm:gap-4">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Cases suchen…"
            className="w-full pl-10 pr-3 py-2.5 bg-white/10 backdrop-blur-md border border-white/20 rounded-lg text-sm text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
          />
        </div>
        <div className="flex items-center gap-2">
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
            className="px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-sm text-white focus:outline-none focus:border-accent-red"
          >
            <option value="all">Alle Status</option>
            <option value="free">Frei</option>
            <option value="rented">Vermietet</option>
            <option value="maintance">Wartung</option>
          </select>
          <button
            type="submit"
            className="px-4 py-2 bg-accent-red/80 hover:bg-accent-red rounded-lg text-sm font-semibold text-white transition-colors"
          >
            Suchen
          </button>
          <button
            type="button"
            onClick={handleResetFilters}
            className="px-4 py-2 bg-white/10 hover:bg-white/20 rounded-lg text-sm font-semibold text-white transition-colors"
          >
            Zurücksetzen
          </button>
        </div>
      </form>

      {actionMessage && (
        <div
          className={`px-3 py-2 rounded-lg text-sm font-semibold ${
            actionMessage.type === 'success'
              ? 'bg-green-500/20 text-green-400'
              : 'bg-red-500/20 text-red-400'
          }`}
        >
          {actionMessage.text}
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
        <SummaryCard label="Frei" value={statusCounts['free'] ?? 0} tone="green" />
        <SummaryCard label="Vermietet" value={statusCounts['rented'] ?? 0} tone="red" />
        <SummaryCard label="Wartung" value={statusCounts['maintance'] ?? 0} tone="yellow" />
        <SummaryCard label="Geräte in Cases" value={totalDevices} tone="blue" />
      </div>

      <div className="glass-dark rounded-xl sm:rounded-2xl p-4 sm:p-6 border border-white/10 min-h-[320px]">
        {loading ? (
          <div className="flex flex-col items-center justify-center h-60 gap-3 text-gray-400">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-accent-red" />
            <p className="text-sm">Cases werden geladen…</p>
          </div>
        ) : cases.length === 0 ? (
          <div className="text-center py-12 text-gray-400 text-sm">
            Keine Cases gefunden.
          </div>
        ) : (
          <div className="space-y-3">
            {cases.map((caseItem) => (
              <div
                key={caseItem.case_id}
                className="rounded-xl border border-white/10 bg-white/[0.02] p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 transition-colors hover:bg-white/[0.04]"
              >
                <div className="flex items-start gap-3 sm:gap-4">
                  <div className="p-3 rounded-lg bg-gradient-to-br from-accent-red/20 to-red-700/20 flex-shrink-0">
                    <Box className="w-5 h-5 text-accent-red" />
                  </div>
                  <div>
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="text-base sm:text-lg font-bold text-white">{caseItem.name}</h3>
                      <span
                        className={`text-[10px] sm:text-xs font-semibold px-2 py-1 rounded-full bg-white/10 uppercase tracking-wide ${getStatusColor(caseItem.status)}`}
                      >
                        {formatStatus(caseItem.status)}
                      </span>
                    </div>
                    {caseItem.description && (
                      <p className="text-sm text-gray-400 mt-1">{caseItem.description}</p>
                    )}
                    <div className="flex flex-wrap items-center gap-3 text-xs text-gray-400 mt-2">
                      <span className="flex items-center gap-1">
                        <Package className="w-3 h-3" />
                        {caseItem.device_count} Gerät{caseItem.device_count === 1 ? '' : 'e'}
                      </span>
                      {caseItem.zone_name && (
                        <span className="flex items-center gap-1">
                          <MapPin className="w-3 h-3" />
                          {caseItem.zone_name}
                          {caseItem.zone_code && (
                            <span className="font-mono text-gray-500 ml-1">{caseItem.zone_code}</span>
                          )}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <button
                    onClick={() => handleOpenCase(caseItem)}
                    className="px-3 py-2 rounded-lg text-sm font-semibold bg-white/10 hover:bg-white/20 transition-colors text-white"
                  >
                    Details
                  </button>
                  <button
                    onClick={() => handleOpenZone({ zone_id: caseItem.zone_id, zone_code: caseItem.zone_code })}
                    disabled={!caseItem.zone_id && !caseItem.zone_code}
                    className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-semibold bg-white/10 hover:bg-white/20 transition-colors text-white disabled:opacity-50"
                  >
                    <MapPin className="w-4 h-4" /> Zone
                  </button>
                  <button
                    onClick={() => handlePrintLabel(caseItem.case_id, caseItem.name)}
                    className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-semibold bg-green-600/80 hover:bg-green-600 transition-colors text-white"
                  >
                    <Printer className="w-4 h-4" /> Label
                  </button>
                  <button
                    onClick={() => handleOpenEditModal(caseItem)}
                    className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-semibold bg-blue-600/80 hover:bg-blue-600 transition-colors text-white"
                  >
                    <Edit2 className="w-4 h-4" /> Bearbeiten
                  </button>
                  <button
                    onClick={() => handleDelete(caseItem.case_id, caseItem.name)}
                    className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-semibold bg-red-600/80 hover:bg-red-600 transition-colors text-white"
                  >
                    <Trash2 className="w-4 h-4" /> Löschen
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <CaseDetailModal
        caseInfo={selectedCase}
        devices={caseDevices}
        isOpen={caseModalOpen}
        loading={caseModalLoading}
        onClose={() => setCaseModalOpen(false)}
        onLocateDevice={handleLocateDevice}
        onOpenDevice={handleOpenDevice}
        onOpenZone={handleOpenZone}
        onAddDevices={handleAddDevicesToCase}
        onRemoveDevice={handleRemoveDeviceFromCase}
        onRefresh={handleRefreshCaseDevices}
      />

      <DeviceTreeModal
        isOpen={deviceTreeModalOpen}
        onClose={() => setDeviceTreeModalOpen(false)}
        onConfirm={handleConfirmAddDevices}
        zoneId={-1}
      />

      <DeviceDetailModal
        device={selectedDevice}
        isOpen={deviceModalOpen}
        onClose={() => setDeviceModalOpen(false)}
      />

      {/* Case Form Modal */}
      {formModalOpen && (
        <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
          <div className="glass-dark rounded-2xl w-full max-w-2xl shadow-2xl my-8">
            <div className="flex items-center justify-between p-6 border-b border-white/10">
              <h2 className="text-2xl font-bold text-white">
                {editingCaseId ? 'Case Bearbeiten' : 'Neues Case'}
              </h2>
              <button
                onClick={() => setFormModalOpen(false)}
                className="p-2 rounded-lg hover:bg-white/10 transition-colors text-gray-400 hover:text-white"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleFormSubmit} className="p-6 space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="sm:col-span-2">
                  <label className="block text-sm font-semibold text-white mb-2">
                    Name <span className="text-accent-red">*</span>
                  </label>
                  <input
                    type="text"
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
                    required
                  />
                </div>

                <div className="sm:col-span-2">
                  <label className="block text-sm font-semibold text-white mb-2">Beschreibung</label>
                  <textarea
                    value={formData.description || ''}
                    onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                    rows={3}
                    className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
                  />
                </div>

                <div>
                  <label className="block text-sm font-semibold text-white mb-2">Status</label>
                  <select
                    value={formData.status}
                    onChange={(e) => setFormData({ ...formData, status: e.target.value as any })}
                    className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white focus:outline-none focus:border-accent-red transition-colors"
                  >
                    <option value="free">Frei</option>
                    <option value="rented">Vermietet</option>
                    <option value="maintance">Wartung</option>
                  </select>
                </div>

                <div>
                  <label className="block text-sm font-semibold text-white mb-2">Zone</label>
                  <select
                    value={formData.zone_id || ''}
                    onChange={(e) => setFormData({ ...formData, zone_id: e.target.value ? parseInt(e.target.value) : undefined })}
                    className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white focus:outline-none focus:border-accent-red transition-colors"
                  >
                    <option value="">Keine Zone</option>
                    {zones.map((zone) => (
                      <option key={zone.zone_id} value={zone.zone_id}>
                        {zone.name} ({zone.code})
                      </option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-sm font-semibold text-white mb-2">Breite (cm)</label>
                  <input
                    type="number"
                    step="0.01"
                    value={formData.width || ''}
                    onChange={(e) => setFormData({ ...formData, width: e.target.value ? parseFloat(e.target.value) : undefined })}
                    className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
                  />
                </div>

                <div>
                  <label className="block text-sm font-semibold text-white mb-2">Höhe (cm)</label>
                  <input
                    type="number"
                    step="0.01"
                    value={formData.height || ''}
                    onChange={(e) => setFormData({ ...formData, height: e.target.value ? parseFloat(e.target.value) : undefined })}
                    className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
                  />
                </div>

                <div>
                  <label className="block text-sm font-semibold text-white mb-2">Tiefe (cm)</label>
                  <input
                    type="number"
                    step="0.01"
                    value={formData.depth || ''}
                    onChange={(e) => setFormData({ ...formData, depth: e.target.value ? parseFloat(e.target.value) : undefined })}
                    className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
                  />
                </div>

                <div>
                  <label className="block text-sm font-semibold text-white mb-2">Gewicht (kg)</label>
                  <input
                    type="number"
                    step="0.01"
                    value={formData.weight || ''}
                    onChange={(e) => setFormData({ ...formData, weight: e.target.value ? parseFloat(e.target.value) : undefined })}
                    className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
                  />
                </div>

                {!editingCaseId && (
                  <>
                    <div>
                      <label className="block text-sm font-semibold text-white mb-2">Barcode</label>
                      <input
                        type="text"
                        value={formData.barcode || ''}
                        onChange={(e) => setFormData({ ...formData, barcode: e.target.value })}
                        className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
                      />
                    </div>

                    <div>
                      <label className="block text-sm font-semibold text-white mb-2">RFID Tag</label>
                      <input
                        type="text"
                        value={formData.rfid_tag || ''}
                        onChange={(e) => setFormData({ ...formData, rfid_tag: e.target.value })}
                        className="w-full px-3 py-2 bg-white/10 border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
                      />
                    </div>
                  </>
                )}
              </div>

              {/* Device Management (only in edit mode) */}
              {editingCaseId && (
                <div className="border-t border-white/10 pt-4">
                  <div className="flex items-center justify-between mb-3">
                    <h3 className="text-lg font-semibold text-white">
                      Geräte ({editingCaseDevices.length})
                    </h3>
                    <button
                      type="button"
                      onClick={handleAddDevicesToEditCase}
                      className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-semibold bg-accent-red/80 hover:bg-accent-red transition-colors text-white"
                    >
                      <Plus className="w-4 h-4" />
                      Geräte hinzufügen
                    </button>
                  </div>

                  {editingCaseDevices.length === 0 ? (
                    <div className="text-center text-gray-400 text-sm py-8 bg-white/5 rounded-lg">
                      Keine Geräte in diesem Case.
                    </div>
                  ) : (
                    <div className="space-y-2 max-h-64 overflow-y-auto">
                      {editingCaseDevices.map((device) => (
                        <div
                          key={device.device_id}
                          className="flex items-center justify-between p-3 rounded-lg bg-white/5 border border-white/10"
                        >
                          <div className="flex-1 min-w-0">
                            <div className="font-medium text-white text-sm font-mono">
                              {device.device_id}
                            </div>
                            {device.product_name && (
                              <div className="text-xs text-gray-400 mt-1">{device.product_name}</div>
                            )}
                          </div>
                          <button
                            type="button"
                            onClick={() => {
                              if (confirm(`Gerät ${device.device_id} aus diesem Case entfernen?`)) {
                                void handleRemoveDeviceFromEditCase(device.device_id);
                              }
                            }}
                            className="ml-3 p-2 rounded-lg text-red-400 hover:bg-red-600/20 transition-colors"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              <div className="flex gap-3 pt-4">
                <button
                  type="submit"
                  disabled={submitting}
                  className="flex-1 px-4 py-3 bg-accent-red/80 hover:bg-accent-red rounded-lg font-semibold text-white transition-colors disabled:opacity-50"
                >
                  {submitting ? 'Speichern...' : editingCaseId ? 'Aktualisieren' : 'Erstellen'}
                </button>
                <button
                  type="button"
                  onClick={() => setFormModalOpen(false)}
                  className="px-4 py-3 bg-white/10 hover:bg-white/20 rounded-lg font-semibold text-white transition-colors"
                >
                  Abbrechen
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}

interface SummaryCardProps {
  label: string;
  value: number;
  tone: 'green' | 'red' | 'yellow' | 'blue';
}

function SummaryCard({ label, value, tone }: SummaryCardProps) {
  const toneClass = {
    green: 'from-green-500/20 to-emerald-500/20 text-green-300',
    red: 'from-red-500/20 to-rose-500/20 text-red-300',
    yellow: 'from-yellow-500/20 to-amber-500/20 text-yellow-300',
    blue: 'from-sky-500/20 to-blue-600/20 text-sky-300',
  }[tone];

  return (
    <div className={`rounded-xl border border-white/10 bg-gradient-to-br ${toneClass} p-3 sm:p-4`}>
      <p className="text-xs uppercase tracking-wide text-gray-300 truncate">{label}</p>
      <p className="text-xl sm:text-2xl font-bold text-white mt-1 sm:mt-2">{value}</p>
    </div>
  );
}
