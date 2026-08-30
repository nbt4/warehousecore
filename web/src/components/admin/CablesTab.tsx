import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Barcode,
  Cable,
  Eye,
  MapPin,
  PackagePlus,
  Pencil,
  Plus,
  RefreshCcw,
  Search,
  Trash2,
  X,
} from 'lucide-react';
import { cablesAdminApi, zonesApi } from '../../lib/api';
import type {
  Cable as CableInventoryItem,
  CableConnector,
  CableCreateInput,
  CableTrackingMode,
  CableType,
  CableUpdateInput,
  Zone,
} from '../../lib/api';
import { useBlockBodyScroll } from '../../hooks/useBlockBodyScroll';
import { toast } from '../../lib/toast';

interface CableFormData {
  name: string;
  connector1?: number;
  connector2?: number;
  typ?: number;
  length: number;
  mm2: string;
  trackingMode: CableTrackingMode;
  genericBarcode: string;
  quantity: number;
  zoneId: number | '';
}

interface InventoryFormData {
  quantity: number;
  zoneId: number | '';
}

const initialFormData: CableFormData = {
  name: '',
  length: 1,
  mm2: '',
  trackingMode: 'quantity',
  genericBarcode: '',
  quantity: 0,
  zoneId: '',
};

const secondaryButton =
  'inline-flex items-center justify-center gap-2 rounded-lg bg-light/5 px-3 py-2 text-sm font-semibold text-light transition-colors hover:bg-light/10 disabled:cursor-not-allowed disabled:opacity-50';
const primaryButton =
  'inline-flex items-center justify-center gap-2 rounded-lg bg-accent-red px-4 py-2 text-sm font-semibold text-light transition-colors hover:bg-accent-red-hover disabled:cursor-not-allowed disabled:opacity-50';
const iconButton =
  'inline-flex h-9 w-9 items-center justify-center rounded-lg bg-light/5 text-text-muted transition-colors hover:bg-light/10 hover:text-light';
const destructiveButton =
  'inline-flex items-center justify-center gap-2 rounded-lg bg-accent-red/15 px-3 py-2 text-sm font-semibold text-error transition-colors hover:bg-accent-red/25';

function useDebouncedValue<T>(value: T, delay: number) {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);

  return debounced;
}

function trackingLabel(mode: CableTrackingMode) {
  return mode === 'individual' ? 'Einzeln' : 'Menge';
}

function statusBadge(status: string) {
  if (status === 'free' || status === 'in_storage') return 'badge badge-success';
  if (status === 'defective' || status === 'defect') return 'badge badge-danger';
  if (status === 'on_job' || status === 'rented') return 'badge badge-warning';
  return 'badge badge-neutral';
}

function formatNumber(value: number) {
  return new Intl.NumberFormat('de-DE', { maximumFractionDigits: 2 }).format(value);
}

export function CablesTab() {
  const [cables, setCables] = useState<CableInventoryItem[]>([]);
  const [connectors, setConnectors] = useState<CableConnector[]>([]);
  const [cableTypes, setCableTypes] = useState<CableType[]>([]);
  const [zones, setZones] = useState<Zone[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState<number | ''>('');
  const [trackingFilter, setTrackingFilter] = useState<CableTrackingMode | ''>('');
  const [formOpen, setFormOpen] = useState(false);
  const [editingCable, setEditingCable] = useState<CableInventoryItem | null>(null);
  const [detailCable, setDetailCable] = useState<CableInventoryItem | null>(null);
  const [formData, setFormData] = useState<CableFormData>(initialFormData);
  const [inventoryForm, setInventoryForm] = useState<InventoryFormData>({ quantity: 1, zoneId: '' });
  const debouncedSearch = useDebouncedValue(search, 250);

  useBlockBodyScroll(formOpen || detailCable !== null);

  const connectorById = useMemo(
    () => new Map(connectors.map((connector) => [connector.connector_id, connector])),
    [connectors],
  );

  const totalStock = useMemo(
    () => cables.reduce((sum, cable) => sum + cable.stock_quantity, 0),
    [cables],
  );
  const individualProducts = useMemo(
    () => cables.filter((cable) => cable.tracking_mode === 'individual').length,
    [cables],
  );

  const formatConnector = useCallback(
    (id: number) => {
      const connector = connectorById.get(id);
      if (!connector) return 'Unbekannt';
      const abbreviation = connector.abbreviation ? ` (${connector.abbreviation})` : '';
      const gender = connector.gender ? ` · ${connector.gender}` : '';
      return `${connector.name}${abbreviation}${gender}`;
    },
    [connectorById],
  );

  const fetchCables = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await cablesAdminApi.getAll({
        search: debouncedSearch || undefined,
        type: typeFilter || undefined,
        tracking_mode: trackingFilter || undefined,
      });
      setCables(data ?? []);
    } catch (error) {
      toast.error(`Kabelbestand konnte nicht geladen werden: ${String(error)}`);
      setCables([]);
    } finally {
      setLoading(false);
    }
  }, [debouncedSearch, trackingFilter, typeFilter]);

  const loadMetadata = useCallback(async () => {
    try {
      const [connectorResponse, typeResponse, zoneResponse] = await Promise.all([
        cablesAdminApi.getConnectors(),
        cablesAdminApi.getTypes(),
        zonesApi.getAll(),
      ]);
      setConnectors(connectorResponse.data ?? []);
      setCableTypes(typeResponse.data ?? []);
      setZones((zoneResponse.data ?? []).filter((zone) => zone.is_active));
    } catch (error) {
      toast.error(`Kabelstammdaten konnten nicht geladen werden: ${String(error)}`);
    }
  }, []);

  useEffect(() => {
    void fetchCables();
  }, [fetchCables]);

  useEffect(() => {
    void loadMetadata();
  }, [loadMetadata]);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([fetchCables(), loadMetadata()]);
    setRefreshing(false);
  }, [fetchCables, loadMetadata]);

  const loadDetail = useCallback(async (cableID: number) => {
    try {
      const { data } = await cablesAdminApi.getById(cableID);
      setDetailCable(data);
    } catch (error) {
      toast.error(`Kabeldetails konnten nicht geladen werden: ${String(error)}`);
    }
  }, []);

  const openCreate = () => {
    setEditingCable(null);
    setFormData(initialFormData);
    setFormOpen(true);
  };

  const openEdit = (cable: CableInventoryItem) => {
    setEditingCable(cable);
    setFormData({
      name: cable.name,
      connector1: cable.connector1,
      connector2: cable.connector2,
      typ: cable.typ,
      length: cable.length,
      mm2: cable.mm2?.toString() ?? '',
      trackingMode: cable.tracking_mode,
      genericBarcode: cable.generic_barcode ?? '',
      quantity: 0,
      zoneId: '',
    });
    setDetailCable(null);
    setFormOpen(true);
  };

  const submitCable = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!formData.connector1 || !formData.connector2 || !formData.typ || formData.length <= 0) {
      toast.error('Bitte Kabeltyp, beide Anschlüsse und eine gültige Länge angeben.');
      return;
    }

    setSubmitting(true);
    try {
      const baseData = {
        name: formData.name.trim() || undefined,
        connector1: formData.connector1,
        connector2: formData.connector2,
        typ: formData.typ,
        length: formData.length,
        tracking_mode: formData.trackingMode,
        generic_barcode: formData.genericBarcode.trim() || undefined,
      };

      if (editingCable) {
        const updateData: CableUpdateInput = {
          ...baseData,
          mm2: formData.mm2 ? Number(formData.mm2) : null,
        };
        await cablesAdminApi.update(editingCable.cable_id, updateData);
        toast.success('Kabelprodukt aktualisiert.');
      } else {
        const createData: CableCreateInput = {
          ...baseData,
          mm2: formData.mm2 ? Number(formData.mm2) : undefined,
          quantity: formData.quantity,
          zone_id: formData.zoneId || null,
        };
        await cablesAdminApi.create(createData);
        toast.success('Kabelprodukt angelegt.');
      }
      setFormOpen(false);
      setEditingCable(null);
      setFormData(initialFormData);
      await refresh();
    } catch (error) {
      toast.error(`Kabelprodukt konnte nicht gespeichert werden: ${String(error)}`);
    } finally {
      setSubmitting(false);
    }
  };

  const deleteCable = async (cable: CableInventoryItem) => {
    if (!window.confirm(`„${cable.name}“ wirklich löschen? Der Bestand muss vorher null sein.`)) return;
    try {
      await cablesAdminApi.delete(cable.cable_id);
      setDetailCable(null);
      toast.success('Kabelprodukt gelöscht.');
      await refresh();
    } catch (error) {
      toast.error(`Kabelprodukt konnte nicht gelöscht werden: ${String(error)}`);
    }
  };

  const saveInventory = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!detailCable || inventoryForm.quantity < 0) return;
    setSubmitting(true);
    try {
      const payload = {
        quantity: inventoryForm.quantity,
        zone_id: inventoryForm.zoneId || null,
      };
      if (detailCable.tracking_mode === 'individual') {
        if (inventoryForm.quantity < 1) {
          toast.error('Mindestens ein Exemplar anlegen.');
          return;
        }
        await cablesAdminApi.createUnits(detailCable.cable_id, payload);
        toast.success(`${inventoryForm.quantity} Exemplar(e) angelegt.`);
      } else {
        await cablesAdminApi.setStock(detailCable.cable_id, payload);
        toast.success('Lagerbestand aktualisiert.');
      }
      setInventoryForm({ quantity: 1, zoneId: '' });
      await Promise.all([loadDetail(detailCable.cable_id), fetchCables()]);
    } catch (error) {
      toast.error(`Bestand konnte nicht aktualisiert werden: ${String(error)}`);
    } finally {
      setSubmitting(false);
    }
  };

  const deleteUnit = async (cableID: number, deviceID: string) => {
    if (!window.confirm(`Exemplar ${deviceID} wirklich löschen?`)) return;
    try {
      await cablesAdminApi.deleteUnit(cableID, deviceID);
      toast.success('Kabelexemplar gelöscht.');
      await Promise.all([loadDetail(cableID), fetchCables()]);
    } catch (error) {
      toast.error(`Kabelexemplar konnte nicht gelöscht werden: ${String(error)}`);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <Cable className="h-6 w-6 text-accent-red" />
            <h2 className="text-2xl font-bold text-light">Kabelbestand</h2>
          </div>
          <p className="mt-1 text-sm text-text-muted">
            Kabel als Produktbestand verwalten – mengenbasiert oder mit einzelnem Code.
          </p>
        </div>
        <button type="button" onClick={openCreate} className={primaryButton}>
          <Plus className="h-4 w-4" />
          Kabelprodukt anlegen
        </button>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="card p-4">
          <p className="text-xs font-semibold uppercase tracking-wide text-text-muted">Kabelprodukte</p>
          <p className="mt-2 text-2xl font-bold text-light">{cables.length}</p>
        </div>
        <div className="card p-4">
          <p className="text-xs font-semibold uppercase tracking-wide text-text-muted">Gesamtbestand</p>
          <p className="mt-2 text-2xl font-bold text-light">{formatNumber(totalStock)}</p>
        </div>
        <div className="card p-4">
          <p className="text-xs font-semibold uppercase tracking-wide text-text-muted">Mit Einzelcodes</p>
          <p className="mt-2 text-2xl font-bold text-light">{individualProducts}</p>
        </div>
      </div>

      <div className="card grid gap-3 p-4 md:grid-cols-[minmax(0,1fr)_220px_180px_auto]">
        <label className="suite-search-field">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted" />
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Name, Barcode oder Anschluss suchen"
            className="w-full py-2 pl-10 pr-3"
          />
        </label>
        <select
          value={typeFilter}
          onChange={(event) => setTypeFilter(event.target.value ? Number(event.target.value) : '')}
          className="w-full px-3 py-2"
          aria-label="Kabeltyp filtern"
        >
          <option value="">Alle Kabeltypen</option>
          {cableTypes.map((type) => (
            <option key={type.cable_type_id} value={type.cable_type_id}>{type.name}</option>
          ))}
        </select>
        <select
          value={trackingFilter}
          onChange={(event) => setTrackingFilter(event.target.value as CableTrackingMode | '')}
          className="w-full px-3 py-2"
          aria-label="Tracking filtern"
        >
          <option value="">Beide Trackingarten</option>
          <option value="quantity">Mengenbestand</option>
          <option value="individual">Einzelcodes</option>
        </select>
        <button type="button" onClick={() => void refresh()} disabled={refreshing} className={secondaryButton}>
          <RefreshCcw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
          Aktualisieren
        </button>
      </div>

      {loading ? (
        <div className="card p-12 text-center text-text-muted">Kabelbestand wird geladen …</div>
      ) : cables.length === 0 ? (
        <div className="card p-12 text-center">
          <Cable className="mx-auto h-9 w-9 text-text-tertiary" />
          <p className="mt-3 font-semibold text-light">Keine Kabelprodukte gefunden</p>
          <p className="mt-1 text-sm text-text-muted">Lege das erste Kabelprodukt an oder passe die Filter an.</p>
        </div>
      ) : (
        <div className="card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[860px]">
              <thead className="bg-light/5 text-left text-xs uppercase tracking-wide text-text-muted">
                <tr>
                  <th className="px-4 py-3">Kabelprodukt</th>
                  <th className="px-4 py-3">Spezifikation</th>
                  <th className="px-4 py-3">Tracking</th>
                  <th className="px-4 py-3">Barcode</th>
                  <th className="px-4 py-3 text-right">Bestand</th>
                  <th className="px-4 py-3 text-right">Aktionen</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-light/5">
                {cables.map((cable) => (
                  <tr key={cable.cable_id} className="transition-colors hover:bg-light/[0.03]">
                    <td className="px-4 py-4">
                      <p className="font-semibold text-light">{cable.name}</p>
                      <p className="mt-1 text-xs text-text-muted">Produkt #{cable.product_id}</p>
                    </td>
                    <td className="px-4 py-4 text-sm text-text-muted">
                      <p className="text-light">{cable.cable_type_name} · {formatNumber(cable.length)} m</p>
                      <p className="mt-1">{formatConnector(cable.connector1)} → {formatConnector(cable.connector2)}</p>
                      {cable.mm2 && <p className="mt-1">{formatNumber(cable.mm2)} mm²</p>}
                    </td>
                    <td className="px-4 py-4">
                      <span className={cable.tracking_mode === 'individual' ? 'badge badge-info' : 'badge badge-neutral'}>
                        {trackingLabel(cable.tracking_mode)}
                      </span>
                    </td>
                    <td className="px-4 py-4 font-mono text-sm text-text-muted">
                      {cable.generic_barcode ?? '—'}
                    </td>
                    <td className="px-4 py-4 text-right">
                      <p className="font-semibold text-light">{formatNumber(cable.stock_quantity)}</p>
                      {cable.available_quantity !== cable.stock_quantity && (
                        <p className="mt-1 text-xs text-text-muted">{formatNumber(cable.available_quantity)} verfügbar</p>
                      )}
                    </td>
                    <td className="px-4 py-4">
                      <div className="flex justify-end gap-2">
                        <button type="button" onClick={() => void loadDetail(cable.cable_id)} className={iconButton} title="Details">
                          <Eye className="h-4 w-4" />
                        </button>
                        <button type="button" onClick={() => openEdit(cable)} className={iconButton} title="Bearbeiten">
                          <Pencil className="h-4 w-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {formOpen && (
        <div className="fixed inset-0 z-[120] flex items-center justify-center bg-dark/90 p-4">
          <div className="max-h-[92vh] w-full max-w-3xl overflow-y-auto rounded-2xl border border-light/10 bg-dark-100 p-6 shadow-2xl">
            <div className="mb-6 flex items-start justify-between gap-4">
              <div>
                <h3 className="text-2xl font-bold text-light">
                  {editingCable ? 'Kabelprodukt bearbeiten' : 'Kabelprodukt anlegen'}
                </h3>
                <p className="mt-1 text-sm text-text-muted">Technische Daten und Bestandsführung festlegen.</p>
              </div>
              <button type="button" onClick={() => setFormOpen(false)} className={iconButton}>
                <X className="h-5 w-5" />
              </button>
            </div>

            <form onSubmit={submitCable} className="space-y-6">
              <div className="grid gap-4 sm:grid-cols-2">
                <label className="sm:col-span-2">
                  <span className="mb-2 block text-sm font-medium text-light">Name</span>
                  <input
                    type="text"
                    value={formData.name}
                    onChange={(event) => setFormData({ ...formData, name: event.target.value })}
                    placeholder="Wird aus Typ, Anschlüssen und Länge erzeugt"
                    className="w-full px-3 py-2"
                  />
                </label>
                <label>
                  <span className="mb-2 block text-sm font-medium text-light">Anschluss A *</span>
                  <select
                    required
                    value={formData.connector1 ?? ''}
                    onChange={(event) => setFormData({ ...formData, connector1: Number(event.target.value) || undefined })}
                    className="w-full px-3 py-2"
                  >
                    <option value="">Anschluss auswählen</option>
                    {connectors.map((connector) => (
                      <option key={connector.connector_id} value={connector.connector_id}>{formatConnector(connector.connector_id)}</option>
                    ))}
                  </select>
                </label>
                <label>
                  <span className="mb-2 block text-sm font-medium text-light">Anschluss B *</span>
                  <select
                    required
                    value={formData.connector2 ?? ''}
                    onChange={(event) => setFormData({ ...formData, connector2: Number(event.target.value) || undefined })}
                    className="w-full px-3 py-2"
                  >
                    <option value="">Anschluss auswählen</option>
                    {connectors.map((connector) => (
                      <option key={connector.connector_id} value={connector.connector_id}>{formatConnector(connector.connector_id)}</option>
                    ))}
                  </select>
                </label>
                <label>
                  <span className="mb-2 block text-sm font-medium text-light">Kabeltyp *</span>
                  <select
                    required
                    value={formData.typ ?? ''}
                    onChange={(event) => setFormData({ ...formData, typ: Number(event.target.value) || undefined })}
                    className="w-full px-3 py-2"
                  >
                    <option value="">Kabeltyp auswählen</option>
                    {cableTypes.map((type) => (
                      <option key={type.cable_type_id} value={type.cable_type_id}>{type.name}</option>
                    ))}
                  </select>
                </label>
                <label>
                  <span className="mb-2 block text-sm font-medium text-light">Artikelbarcode</span>
                  <input
                    type="text"
                    value={formData.genericBarcode}
                    onChange={(event) => setFormData({ ...formData, genericBarcode: event.target.value })}
                    placeholder="Automatisch, wenn leer"
                    className="w-full px-3 py-2 font-mono"
                  />
                </label>
                <label>
                  <span className="mb-2 block text-sm font-medium text-light">Länge in Metern *</span>
                  <input
                    type="number"
                    min="0.01"
                    step="0.01"
                    required
                    value={formData.length}
                    onChange={(event) => setFormData({ ...formData, length: Number(event.target.value) })}
                    className="w-full px-3 py-2"
                  />
                </label>
                <label>
                  <span className="mb-2 block text-sm font-medium text-light">Querschnitt in mm²</span>
                  <input
                    type="number"
                    min="0.01"
                    step="0.01"
                    value={formData.mm2}
                    onChange={(event) => setFormData({ ...formData, mm2: event.target.value })}
                    className="w-full px-3 py-2"
                  />
                </label>
              </div>

              <fieldset>
                <legend className="mb-3 text-sm font-medium text-light">Bestandsführung</legend>
                <div className="grid gap-3 sm:grid-cols-2">
                  {([
                    { value: 'quantity' as const, title: 'Mengenbestand', text: 'Ein Artikelbarcode, Bestand pro Lagerzone.' },
                    { value: 'individual' as const, title: 'Einzelcodes', text: 'Jedes physische Kabel erhält einen eigenen Code.' },
                  ]).map((option) => (
                    <label
                      key={option.value}
                      className={`cursor-pointer rounded-xl border p-4 transition-colors ${
                        formData.trackingMode === option.value
                          ? 'border-accent-red bg-accent-red/10'
                          : 'border-light/10 bg-light/[0.03] hover:bg-light/5'
                      } ${editingCable && editingCable.stock_quantity > 0 ? 'cursor-not-allowed opacity-60' : ''}`}
                    >
                      <input
                        type="radio"
                        name="trackingMode"
                        value={option.value}
                        checked={formData.trackingMode === option.value}
                        disabled={Boolean(editingCable && editingCable.stock_quantity > 0)}
                        onChange={() => setFormData({ ...formData, trackingMode: option.value })}
                        className="sr-only"
                      />
                      <span className="font-semibold text-light">{option.title}</span>
                      <span className="mt-1 block text-sm text-text-muted">{option.text}</span>
                    </label>
                  ))}
                </div>
              </fieldset>

              {!editingCable && (
                <div className="grid gap-4 rounded-xl border border-light/10 bg-light/[0.03] p-4 sm:grid-cols-2">
                  <label>
                    <span className="mb-2 block text-sm font-medium text-light">
                      {formData.trackingMode === 'individual' ? 'Anzahl Exemplare' : 'Anfangsbestand'}
                    </span>
                    <input
                      type="number"
                      min="0"
                      step="1"
                      value={formData.quantity}
                      onChange={(event) => setFormData({ ...formData, quantity: Math.max(0, Number(event.target.value)) })}
                      className="w-full px-3 py-2"
                    />
                  </label>
                  <label>
                    <span className="mb-2 block text-sm font-medium text-light">Lagerzone</span>
                    <select
                      value={formData.zoneId}
                      onChange={(event) => setFormData({ ...formData, zoneId: event.target.value ? Number(event.target.value) : '' })}
                      className="w-full px-3 py-2"
                    >
                      <option value="">Noch nicht zugeordnet</option>
                      {zones.map((zone) => (
                        <option key={zone.zone_id} value={zone.zone_id}>{zone.name} ({zone.code})</option>
                      ))}
                    </select>
                  </label>
                </div>
              )}

              <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
                <button type="button" onClick={() => setFormOpen(false)} className={secondaryButton}>Abbrechen</button>
                <button type="submit" disabled={submitting} className={primaryButton}>
                  {submitting ? 'Speichert …' : editingCable ? 'Änderungen speichern' : 'Kabelprodukt anlegen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {detailCable && (
        <div className="fixed inset-0 z-[120] flex items-center justify-center bg-dark/90 p-4">
          <div className="max-h-[92vh] w-full max-w-4xl overflow-y-auto rounded-2xl border border-light/10 bg-dark-100 p-6 shadow-2xl">
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="text-2xl font-bold text-light">{detailCable.name}</h3>
                  <span className={detailCable.tracking_mode === 'individual' ? 'badge badge-info' : 'badge badge-neutral'}>
                    {trackingLabel(detailCable.tracking_mode)}
                  </span>
                </div>
                <p className="mt-1 text-sm text-text-muted">Produkt #{detailCable.product_id} · Kabel #{detailCable.cable_id}</p>
              </div>
              <button type="button" onClick={() => setDetailCable(null)} className={iconButton}>
                <X className="h-5 w-5" />
              </button>
            </div>

            <div className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div className="rounded-xl bg-light/[0.04] p-4">
                <p className="text-xs uppercase tracking-wide text-text-muted">Kabeltyp</p>
                <p className="mt-2 font-semibold text-light">{detailCable.cable_type_name}</p>
              </div>
              <div className="rounded-xl bg-light/[0.04] p-4">
                <p className="text-xs uppercase tracking-wide text-text-muted">Länge / Querschnitt</p>
                <p className="mt-2 font-semibold text-light">
                  {formatNumber(detailCable.length)} m · {detailCable.mm2 ? `${formatNumber(detailCable.mm2)} mm²` : '—'}
                </p>
              </div>
              <div className="rounded-xl bg-light/[0.04] p-4">
                <p className="text-xs uppercase tracking-wide text-text-muted">Bestand</p>
                <p className="mt-2 font-semibold text-light">{formatNumber(detailCable.stock_quantity)}</p>
              </div>
              <div className="rounded-xl bg-light/[0.04] p-4">
                <p className="text-xs uppercase tracking-wide text-text-muted">Artikelbarcode</p>
                <p className="mt-2 break-all font-mono text-sm font-semibold text-light">{detailCable.generic_barcode ?? '—'}</p>
              </div>
            </div>

            <div className="mt-4 rounded-xl border border-light/10 p-4">
              <p className="text-sm font-semibold text-light">Anschlüsse</p>
              <p className="mt-2 text-sm text-text-muted">{formatConnector(detailCable.connector1)} → {formatConnector(detailCable.connector2)}</p>
            </div>

            <form onSubmit={saveInventory} className="mt-6 rounded-xl border border-light/10 bg-light/[0.03] p-4">
              <div className="flex items-center gap-2">
                <PackagePlus className="h-5 w-5 text-accent-red" />
                <h4 className="font-semibold text-light">
                  {detailCable.tracking_mode === 'individual' ? 'Exemplare hinzufügen' : 'Zonenbestand setzen'}
                </h4>
              </div>
              <div className="mt-4 grid gap-3 sm:grid-cols-[1fr_1fr_auto]">
                <label>
                  <span className="mb-2 block text-xs font-medium uppercase tracking-wide text-text-muted">Lagerzone</span>
                  <select
                    value={inventoryForm.zoneId}
                    onChange={(event) => setInventoryForm({ ...inventoryForm, zoneId: event.target.value ? Number(event.target.value) : '' })}
                    className="w-full px-3 py-2"
                  >
                    <option value="">Ohne Lagerzone</option>
                    {zones.map((zone) => (
                      <option key={zone.zone_id} value={zone.zone_id}>{zone.name} ({zone.code})</option>
                    ))}
                  </select>
                </label>
                <label>
                  <span className="mb-2 block text-xs font-medium uppercase tracking-wide text-text-muted">
                    {detailCable.tracking_mode === 'individual' ? 'Neue Exemplare' : 'Neuer Bestand'}
                  </span>
                  <input
                    type="number"
                    min={detailCable.tracking_mode === 'individual' ? 1 : 0}
                    step="1"
                    value={inventoryForm.quantity}
                    onChange={(event) => setInventoryForm({ ...inventoryForm, quantity: Math.max(0, Number(event.target.value)) })}
                    className="w-full px-3 py-2"
                  />
                </label>
                <button type="submit" disabled={submitting} className={`${primaryButton} self-end`}>
                  {detailCable.tracking_mode === 'individual' ? 'Anlegen' : 'Bestand setzen'}
                </button>
              </div>
            </form>

            {detailCable.tracking_mode === 'quantity' ? (
              <section className="mt-6">
                <h4 className="flex items-center gap-2 font-semibold text-light">
                  <MapPin className="h-4 w-4 text-accent-red" />
                  Bestand nach Lagerzone
                </h4>
                <div className="mt-3 divide-y divide-light/5 rounded-xl border border-light/10">
                  {(detailCable.zone_stocks ?? []).length === 0 ? (
                    <p className="p-4 text-sm text-text-muted">Noch kein Bestand erfasst.</p>
                  ) : detailCable.zone_stocks?.map((stock) => (
                    <div key={stock.zone_id ?? 'unassigned'} className="flex items-center justify-between gap-4 p-4">
                      <div>
                        <p className="font-medium text-light">{stock.zone_name}</p>
                        {stock.zone_code && <p className="text-xs text-text-muted">{stock.zone_code}</p>}
                      </div>
                      <p className="font-semibold text-light">{formatNumber(stock.quantity)} Stk</p>
                    </div>
                  ))}
                </div>
              </section>
            ) : (
              <section className="mt-6">
                <h4 className="flex items-center gap-2 font-semibold text-light">
                  <Barcode className="h-4 w-4 text-accent-red" />
                  Einzelne Kabelexemplare
                </h4>
                <div className="mt-3 overflow-hidden rounded-xl border border-light/10">
                  {(detailCable.units ?? []).length === 0 ? (
                    <p className="p-4 text-sm text-text-muted">Noch keine Exemplare angelegt.</p>
                  ) : (
                    <div className="overflow-x-auto">
                      <table className="w-full min-w-[620px]">
                        <thead className="bg-light/5 text-left text-xs uppercase tracking-wide text-text-muted">
                          <tr>
                            <th className="px-4 py-3">Code</th>
                            <th className="px-4 py-3">Status</th>
                            <th className="px-4 py-3">Lagerzone</th>
                            <th className="px-4 py-3 text-right">Aktion</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-light/5">
                          {detailCable.units?.map((unit) => (
                            <tr key={unit.device_id}>
                              <td className="px-4 py-3">
                                <p className="font-mono text-sm text-light">{unit.barcode ?? unit.device_id}</p>
                                <p className="mt-1 text-xs text-text-muted">{unit.device_id}</p>
                              </td>
                              <td className="px-4 py-3"><span className={statusBadge(unit.status)}>{unit.status}</span></td>
                              <td className="px-4 py-3 text-sm text-text-muted">{unit.zone_name}</td>
                              <td className="px-4 py-3 text-right">
                                <button
                                  type="button"
                                  onClick={() => void deleteUnit(detailCable.cable_id, unit.device_id)}
                                  disabled={unit.current_job_id !== null}
                                  className={`${iconButton} disabled:cursor-not-allowed disabled:opacity-40`}
                                  title={unit.current_job_id ? 'Im Job verwendet' : 'Exemplar löschen'}
                                >
                                  <Trash2 className="h-4 w-4" />
                                </button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              </section>
            )}

            <div className="mt-6 flex flex-col-reverse gap-3 border-t border-light/10 pt-5 sm:flex-row sm:justify-between">
              <button type="button" onClick={() => void deleteCable(detailCable)} className={destructiveButton}>
                <Trash2 className="h-4 w-4" />
                Kabelprodukt löschen
              </button>
              <div className="flex gap-3">
                <button type="button" onClick={() => setDetailCable(null)} className={secondaryButton}>Schließen</button>
                <button type="button" onClick={() => openEdit(detailCable)} className={primaryButton}>
                  <Pencil className="h-4 w-4" />
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
