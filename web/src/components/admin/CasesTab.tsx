import { useCallback, useEffect, useState } from 'react';
import {
  Eye,
  LayoutGrid,
  List,
  Package,
  Pencil,
  Plus,
  Printer,
  RefreshCcw,
  Search,
  Trash2,
  X,
} from 'lucide-react';
import {
  casesApi,
  zonesApi,
  labelsApi,
  type CaseSummary,
  type CaseDetail,
  type CaseDevice,
  type Zone,
} from '../../lib/api';
import { formatStatus } from '../../lib/utils';
import { useBlockBodyScroll } from '../../hooks/useBlockBodyScroll';
import { toast } from '../../lib/toast';

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

const initialFormData: CaseFormData = {
  name: '',
  description: '',
  status: 'free',
};

type StatusFilter = 'all' | 'free' | 'rented' | 'maintance';

function useDebouncedValue<T>(value: T, delay: number) {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const handle = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(handle);
  }, [value, delay]);

  return debounced;
}

export function CasesTab() {
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [loadingCases, setLoadingCases] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [viewCase, setViewCase] = useState<CaseDetail | null>(null);
  const [viewCaseDevices, setViewCaseDevices] = useState<CaseDevice[]>([]);
  const [editingCaseId, setEditingCaseId] = useState<number | null>(null);
  const [formData, setFormData] = useState<CaseFormData>(initialFormData);
  const [zones, setZones] = useState<Zone[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [viewMode, setViewMode] = useState<'table' | 'cards'>('table');
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [zoneFilter, setZoneFilter] = useState<number | ''>('');
  const [refreshing, setRefreshing] = useState(false);
  const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // Block body scroll when any modal is open
  useBlockBodyScroll(modalOpen || viewCase !== null);

  const debouncedSearch = useDebouncedValue(searchTerm, 300);

  const fetchCases = useCallback(async () => {
    setLoadingCases(true);
    try {
      const params: { search?: string; status?: string } = {};
      if (debouncedSearch.trim()) {
        params.search = debouncedSearch.trim();
      }
      if (statusFilter !== 'all') {
        params.status = statusFilter;
      }
      const { data } = await casesApi.list(params);
      setCases(data.cases || []);
    } catch (error) {
      toast.error('Failed to load cases:' + " " + String(error));
      setCases([]);
    } finally {
      setLoadingCases(false);
    }
  }, [debouncedSearch, statusFilter]);

  const loadMetadata = useCallback(async () => {
    try {
      const { data } = await zonesApi.getAll();
      setZones(data || []);
    } catch (error) {
      toast.error('Failed to load zones:' + " " + String(error));
    }
  }, []);

  useEffect(() => {
    fetchCases();
  }, [fetchCases]);

  useEffect(() => {
    loadMetadata();
  }, [loadMetadata]);

  const handleRefresh = useCallback(async () => {
    setRefreshing(true);
    await fetchCases();
    setRefreshing(false);
  }, [fetchCases]);

  const clearFilters = () => {
    setSearchTerm('');
    setStatusFilter('all');
    setZoneFilter('');
  };

  const clearActionMessage = () => {
    setTimeout(() => setActionMessage(null), 4000);
  };

  const filteredCases = cases.filter((caseItem) => {
    const matchesZone = zoneFilter === '' || caseItem.zone_id === zoneFilter;
    return matchesZone;
  });

  const openCreateModal = () => {
    setEditingCaseId(null);
    setFormData(initialFormData);
    setModalOpen(true);
  };

  const openEditModal = (caseItem: CaseSummary) => {
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
    setModalOpen(true);
  };

  const handleViewCase = async (caseItem: CaseSummary) => {
    try {
      const [detailRes, devicesRes] = await Promise.all([
        casesApi.getById(caseItem.case_id),
        casesApi.getDevices(caseItem.case_id),
      ]);
      setViewCase(detailRes.data);
      setViewCaseDevices(devicesRes.data);
    } catch (error) {
      toast.error('Failed to load case details:' + " " + String(error));
      setActionMessage({ type: 'error', text: 'Fehler beim Laden der Case-Details' });
      clearActionMessage();
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
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
      setModalOpen(false);
      setFormData(initialFormData);
      await fetchCases();
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
    if (!window.confirm(`Möchten Sie das Case "${caseName}" wirklich löschen?`)) {
      return;
    }

    try {
      await casesApi.delete(caseId);
      setActionMessage({ type: 'success', text: 'Case erfolgreich gelöscht' });
      await fetchCases();
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
      const { data: templates } = await labelsApi.getTemplates();
      const defaultTemplate = templates.find(t => t.is_default) || templates[0];

      if (!defaultTemplate) {
        alert('Kein Label-Template gefunden. Bitte erstellen Sie erst ein Template unter "Labels".');
        return;
      }

      const { data: labelData } = await labelsApi.generateCaseLabel(caseId, defaultTemplate.id!);

      const labelWindow = window.open('', '_blank', 'width=800,height=600');
      if (!labelWindow) {
        alert('Popup wurde blockiert. Bitte erlauben Sie Popups für diese Seite.');
        return;
      }

      const html = `
        <!DOCTYPE html>
        <html>
        <head>
          <title>Label - ${caseName}</title>
          <style>
            body { margin: 0; padding: 20px; font-family: Arial, sans-serif; background: #1a1a1a; color: white; }
            .container { max-width: 800px; margin: 0 auto; }
            .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; padding-bottom: 20px; border-bottom: 2px solid #444; }
            .label-preview { background: white; border: 1px solid #ccc; padding: 20px; margin: 20px 0; position: relative; }
            .element { position: absolute; }
            .text { color: black; white-space: nowrap; }
            button { background: #dc2626; color: white; border: none; padding: 10px 20px; border-radius: 8px; cursor: pointer; font-size: 16px; }
            button:hover { background: #b91c1c; }
            @media print { .header, button { display: none; } body { background: white; padding: 0; } }
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

  const totalDevices = filteredCases.reduce((sum, current) => sum + current.device_count, 0);
  const statusCounts = filteredCases.reduce<Record<string, number>>((acc, current) => {
    acc[current.status] = (acc[current.status] ?? 0) + 1;
    return acc;
  }, {});

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Package className="w-6 h-6 text-accent-red" />
          <h2 className="text-2xl font-bold text-white">Cases-Verwaltung</h2>
        </div>
        <button
          onClick={openCreateModal}
          className="btn-primary flex items-center gap-2"
        >
          <Plus className="w-5 h-5" />
          Neues Case
        </button>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
        <div className="rounded-xl border border-white/10 bg-gradient-to-br from-green-500/20 to-emerald-500/20 text-green-300 p-4">
          <p className="text-xs uppercase tracking-wide text-gray-300">Frei</p>
          <p className="text-2xl font-bold text-white mt-2">{statusCounts['free'] ?? 0}</p>
        </div>
        <div className="rounded-xl border border-white/10 bg-gradient-to-br from-red-500/20 to-rose-500/20 text-red-300 p-4">
          <p className="text-xs uppercase tracking-wide text-gray-300">Vermietet</p>
          <p className="text-2xl font-bold text-white mt-2">{statusCounts['rented'] ?? 0}</p>
        </div>
        <div className="rounded-xl border border-white/10 bg-gradient-to-br from-yellow-500/20 to-amber-500/20 text-yellow-300 p-4">
          <p className="text-xs uppercase tracking-wide text-gray-300">Wartung</p>
          <p className="text-2xl font-bold text-white mt-2">{statusCounts['maintance'] ?? 0}</p>
        </div>
        <div className="rounded-xl border border-white/10 bg-gradient-to-br from-sky-500/20 to-blue-600/20 text-sky-300 p-4">
          <p className="text-xs uppercase tracking-wide text-gray-300">Geräte in Cases</p>
          <p className="text-2xl font-bold text-white mt-2">{totalDevices}</p>
        </div>
      </div>

      {/* Filters */}
      <div className="glass-dark rounded-xl p-4 space-y-4">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {/* Search */}
          <div className="relative lg:col-span-2">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-5 h-5 text-gray-400" />
            <input
              type="text"
              placeholder="Cases suchen (Name, Beschreibung)..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="input-field pl-10 w-full"
            />
          </div>

          {/* Status Filter */}
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
            className="input-field"
          >
            <option value="all">Alle Status</option>
            <option value="free">Frei</option>
            <option value="rented">Vermietet</option>
            <option value="maintance">Wartung</option>
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

      {/* Action Message */}
      {actionMessage && (
        <div
          className={`px-4 py-3 rounded-lg text-sm font-semibold ${
            actionMessage.type === 'success'
              ? 'bg-green-500/20 text-green-400'
              : 'bg-red-500/20 text-red-400'
          }`}
        >
          {actionMessage.text}
        </div>
      )}

      {/* Cases List */}
      {loadingCases ? (
        <div className="text-center py-12 text-gray-400">Lädt Cases...</div>
      ) : filteredCases.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          {debouncedSearch || statusFilter !== 'all' || zoneFilter
            ? 'Keine Cases gefunden mit den aktuellen Filtern'
            : 'Noch keine Cases vorhanden'}
        </div>
      ) : viewMode === 'table' ? (
        <div className="glass-dark rounded-xl overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-white/5">
                <tr>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Name</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Beschreibung</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Abmessungen</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Status</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Zone</th>
                  <th className="px-4 py-3 text-left text-sm font-semibold text-gray-300">Geräte</th>
                  <th className="px-4 py-3 text-right text-sm font-semibold text-gray-300">Aktionen</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {filteredCases.map((caseItem) => (
                  <tr key={caseItem.case_id} className="hover:bg-white/5 transition-colors">
                    <td className="px-4 py-3 text-sm text-white font-medium">{caseItem.name}</td>
                    <td className="px-4 py-3 text-sm text-gray-300">{caseItem.description || '-'}</td>
                    <td className="px-4 py-3 text-sm text-gray-300">
                      {caseItem.width && caseItem.height && caseItem.depth
                        ? `${caseItem.width}×${caseItem.height}×${caseItem.depth} cm`
                        : '-'}
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`px-2 py-1 rounded-full text-xs font-semibold ${
                          caseItem.status === 'free'
                            ? 'bg-green-500/20 text-green-400'
                            : caseItem.status === 'rented'
                            ? 'bg-red-500/20 text-red-400'
                            : 'bg-yellow-500/20 text-yellow-400'
                        }`}
                      >
                        {formatStatus(caseItem.status)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-300">
                      {caseItem.zone_name ? `${caseItem.zone_code} - ${caseItem.zone_name}` : '-'}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-300">{caseItem.device_count}</td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => handleViewCase(caseItem)}
                          className="p-1.5 hover:bg-white/10 rounded-lg transition-colors text-gray-400 hover:text-white"
                          title="Details anzeigen"
                        >
                          <Eye className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => handlePrintLabel(caseItem.case_id, caseItem.name)}
                          className="p-1.5 hover:bg-white/10 rounded-lg transition-colors text-green-400 hover:text-green-300"
                          title="Label drucken"
                        >
                          <Printer className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => openEditModal(caseItem)}
                          className="p-1.5 hover:bg-white/10 rounded-lg transition-colors text-blue-400 hover:text-blue-300"
                          title="Bearbeiten"
                        >
                          <Pencil className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => handleDelete(caseItem.case_id, caseItem.name)}
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
          {filteredCases.map((caseItem) => (
            <div key={caseItem.case_id} className="glass-dark rounded-xl p-4 space-y-3">
              <div className="flex items-start justify-between">
                <div>
                  <h3 className="font-bold text-white">{caseItem.name}</h3>
                  {caseItem.description && (
                    <p className="text-sm text-gray-400 mt-1">{caseItem.description}</p>
                  )}
                </div>
                <span
                  className={`px-2 py-1 rounded-full text-xs font-semibold ${
                    caseItem.status === 'free'
                      ? 'bg-green-500/20 text-green-400'
                      : caseItem.status === 'rented'
                      ? 'bg-red-500/20 text-red-400'
                      : 'bg-yellow-500/20 text-yellow-400'
                  }`}
                >
                  {formatStatus(caseItem.status)}
                </span>
              </div>

              <div className="space-y-1 text-sm">
                {caseItem.width && caseItem.height && caseItem.depth && (
                  <p className="text-gray-300">
                    <span className="text-gray-500">Größe:</span> {caseItem.width}×{caseItem.height}×{caseItem.depth} cm
                  </p>
                )}
                {caseItem.weight && (
                  <p className="text-gray-300">
                    <span className="text-gray-500">Gewicht:</span> {caseItem.weight} kg
                  </p>
                )}
                {caseItem.zone_name && (
                  <p className="text-gray-300">
                    <span className="text-gray-500">Zone:</span> {caseItem.zone_code} - {caseItem.zone_name}
                  </p>
                )}
                <p className="text-gray-300">
                  <span className="text-gray-500">Geräte:</span> {caseItem.device_count}
                </p>
              </div>

              <div className="flex items-center gap-2 pt-2 border-t border-white/10">
                <button
                  onClick={() => handleViewCase(caseItem)}
                  className="flex-1 px-3 py-2 bg-white/5 hover:bg-white/10 rounded-lg text-sm text-gray-300 transition-colors flex items-center justify-center gap-2"
                >
                  <Eye className="w-4 h-4" />
                  Details
                </button>
                <button
                  onClick={() => openEditModal(caseItem)}
                  className="flex-1 px-3 py-2 bg-blue-500/20 hover:bg-blue-500/30 rounded-lg text-sm text-blue-400 transition-colors flex items-center justify-center gap-2"
                >
                  <Pencil className="w-4 h-4" />
                  Bearbeiten
                </button>
                <button
                  onClick={() => handleDelete(caseItem.case_id, caseItem.name)}
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
                {editingCaseId ? 'Case bearbeiten' : 'Neues Case erstellen'}
              </h3>
              <button
                onClick={() => setModalOpen(false)}
                className="p-2 hover:bg-white/10 rounded-lg transition-colors"
              >
                <X className="w-6 h-6 text-gray-400" />
              </button>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              {/* Name */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Name *
                </label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  className="input-field w-full"
                  required
                />
              </div>

              {/* Description */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Beschreibung
                </label>
                <textarea
                  value={formData.description || ''}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  className="input-field w-full"
                  rows={3}
                />
              </div>

              {/* Status and Zone */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Status *
                  </label>
                  <select
                    value={formData.status}
                    onChange={(e) => setFormData({ ...formData, status: e.target.value as any })}
                    className="input-field w-full"
                    required
                  >
                    <option value="free">Frei</option>
                    <option value="rented">Vermietet</option>
                    <option value="maintance">Wartung</option>
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

              {/* Dimensions */}
              <div className="grid grid-cols-3 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Breite (cm)
                  </label>
                  <input
                    type="number"
                    step="0.01"
                    value={formData.width || ''}
                    onChange={(e) =>
                      setFormData({ ...formData, width: e.target.value ? parseFloat(e.target.value) : undefined })
                    }
                    className="input-field w-full"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Höhe (cm)
                  </label>
                  <input
                    type="number"
                    step="0.01"
                    value={formData.height || ''}
                    onChange={(e) =>
                      setFormData({ ...formData, height: e.target.value ? parseFloat(e.target.value) : undefined })
                    }
                    className="input-field w-full"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Tiefe (cm)
                  </label>
                  <input
                    type="number"
                    step="0.01"
                    value={formData.depth || ''}
                    onChange={(e) =>
                      setFormData({ ...formData, depth: e.target.value ? parseFloat(e.target.value) : undefined })
                    }
                    className="input-field w-full"
                  />
                </div>
              </div>

              {/* Weight */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Gewicht (kg)
                </label>
                <input
                  type="number"
                  step="0.01"
                  value={formData.weight || ''}
                  onChange={(e) =>
                    setFormData({ ...formData, weight: e.target.value ? parseFloat(e.target.value) : undefined })
                  }
                  className="input-field w-full"
                />
              </div>

              {/* Barcode and RFID (only for create) */}
              {!editingCaseId && (
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-2">
                      Barcode
                    </label>
                    <input
                      type="text"
                      value={formData.barcode || ''}
                      onChange={(e) => setFormData({ ...formData, barcode: e.target.value })}
                      className="input-field w-full"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-2">
                      RFID Tag
                    </label>
                    <input
                      type="text"
                      value={formData.rfid_tag || ''}
                      onChange={(e) => setFormData({ ...formData, rfid_tag: e.target.value })}
                      className="input-field w-full"
                    />
                  </div>
                </div>
              )}

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
                    : editingCaseId
                    ? 'Aktualisieren'
                    : 'Erstellen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* View Case Modal */}
      {viewCase && (
        <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
          <div className="glass-dark rounded-2xl p-6 max-w-2xl w-full max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between mb-6">
              <h3 className="text-2xl font-bold text-white">Case-Details</h3>
              <button
                onClick={() => setViewCase(null)}
                className="p-2 hover:bg-white/10 rounded-lg transition-colors"
              >
                <X className="w-6 h-6 text-gray-400" />
              </button>
            </div>

            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-sm text-gray-400">Name</p>
                  <p className="text-white font-semibold">{viewCase.name}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-400">Status</p>
                  <p className="text-white font-semibold">{formatStatus(viewCase.status)}</p>
                </div>
                {viewCase.description && (
                  <div className="col-span-2">
                    <p className="text-sm text-gray-400">Beschreibung</p>
                    <p className="text-white font-semibold">{viewCase.description}</p>
                  </div>
                )}
                {viewCase.width && viewCase.height && viewCase.depth && (
                  <div>
                    <p className="text-sm text-gray-400">Abmessungen</p>
                    <p className="text-white font-semibold">
                      {viewCase.width}×{viewCase.height}×{viewCase.depth} cm
                    </p>
                  </div>
                )}
                {viewCase.weight && (
                  <div>
                    <p className="text-sm text-gray-400">Gewicht</p>
                    <p className="text-white font-semibold">{viewCase.weight} kg</p>
                  </div>
                )}
                {viewCase.zone_name && (
                  <div>
                    <p className="text-sm text-gray-400">Zone</p>
                    <p className="text-white font-semibold">
                      {viewCase.zone_code} - {viewCase.zone_name}
                    </p>
                  </div>
                )}
                <div>
                  <p className="text-sm text-gray-400">Geräte</p>
                  <p className="text-white font-semibold">{viewCase.device_count}</p>
                </div>
              </div>

              {/* Devices in Case */}
              {viewCaseDevices.length > 0 && (
                <div className="border-t border-white/10 pt-4">
                  <h4 className="text-lg font-semibold text-white mb-3">Geräte in diesem Case</h4>
                  <div className="space-y-2 max-h-64 overflow-y-auto">
                    {viewCaseDevices.map((device) => (
                      <div
                        key={device.device_id}
                        className="p-3 rounded-lg bg-white/5 border border-white/10"
                      >
                        <div className="font-medium text-white text-sm font-mono">
                          {device.device_id}
                        </div>
                        {device.product_name && (
                          <div className="text-xs text-gray-400 mt-1">{device.product_name}</div>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <div className="flex gap-3 pt-4">
                <button
                  onClick={() => {
                    setViewCase(null);
                    const caseItem = cases.find(c => c.case_id === viewCase.case_id);
                    if (caseItem) openEditModal(caseItem);
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
