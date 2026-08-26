import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  AlertTriangle, Archive, Boxes, ChevronDown, ChevronRight, ClipboardCheck,
  Edit3, ListChecks, MapPin, PackageOpen, Plus, RefreshCw, Search, Warehouse,
} from 'lucide-react';
import {
  warehouseApi,
  type InventoryCount,
  type InventoryCountDetail,
  type WarehouseLocation,
  type WarehouseOverview,
  type WarehouseTask,
} from '../lib/api';
import { toast } from '../lib/toast';

type LocationForm = {
  name: string; code: string; barcode: string; location_kind: string; process_role: string;
  operational_status: string; description: string; parent_zone_id: string; capacity: string;
  is_storable: boolean; pick_sequence: string; max_weight_kg: string; inventory_frequency_days: string;
};

const emptyForm = (parent = ''): LocationForm => ({
  name: '', code: '', barcode: '', location_kind: 'area', process_role: 'storage',
  operational_status: 'available', description: '', parent_zone_id: parent,
  capacity: '', is_storable: false, pick_sequence: '', max_weight_kg: '', inventory_frequency_days: '',
});

const kindLabels: Record<string, string> = {
  site: 'Standort', area: 'Bereich', aisle: 'Gang', rack: 'Regal', level: 'Ebene',
  bin: 'Fach / Lagerplatz', floor: 'Bodenplatz', vehicle: 'Fahrzeug', virtual: 'Virtueller Ort',
};
const roleLabels: Record<string, string> = {
  storage: 'Lagerung', receiving: 'Wareneingang', return: 'Rücklauf', inspection: 'Prüfung',
  quarantine: 'Quarantäne', repair: 'Reparatur', charging: 'Laden', picking: 'Kommissionierung',
  staging: 'Job-Bereitstellung', shipping: 'Versand', transport: 'Transport', unknown: 'Unbekannt',
};
const statusLabels: Record<string, string> = {
  available: 'Verfügbar', blocked: 'Gesperrt', counting: 'Inventur', maintenance: 'Wartung', archived: 'Archiviert',
};

function numberOrUndefined(value: string) {
  if (!value.trim()) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function apiErrorMessage(error: unknown, fallback: string) {
  if (typeof error === 'object' && error !== null && 'response' in error) {
    const response = (error as { response?: { data?: { error?: string } } }).response;
    if (response?.data?.error) return response.data.error;
  }
  return error instanceof Error ? error.message : fallback;
}

export function ZonesPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [locations, setLocations] = useState<WarehouseLocation[]>([]);
  const [overview, setOverview] = useState<WarehouseOverview | null>(null);
  const [tasks, setTasks] = useState<WarehouseTask[]>([]);
  const [counts, setCounts] = useState<InventoryCount[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [showArchived, setShowArchived] = useState(true);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [formOpen, setFormOpen] = useState(Boolean(searchParams.get('parent')));
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<LocationForm>(emptyForm(searchParams.get('parent') || ''));
  const [saving, setSaving] = useState(false);
  const [activeView, setActiveView] = useState<'locations' | 'tasks' | 'counts'>('locations');

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [locationResult, overviewResult, taskResult, countResult] = await Promise.all([
        warehouseApi.locations(true), warehouseApi.overview(), warehouseApi.tasks(), warehouseApi.counts(),
      ]);
      setLocations(locationResult.data);
      setOverview(overviewResult.data);
      setTasks(taskResult.data);
      setCounts(countResult.data);
      setExpanded((current) => current.size ? current : new Set(locationResult.data.filter((item) => !item.parent_zone_id || item.child_count > 0).map((item) => item.zone_id)));
    } catch (error) {
      toast.error('Lagerstruktur konnte nicht geladen werden: ' + String(error));
    } finally { setLoading(false); }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const childrenByParent = useMemo(() => {
    const map = new Map<number | null, WarehouseLocation[]>();
    locations.forEach((item) => {
      const key = item.parent_zone_id ?? null;
      map.set(key, [...(map.get(key) || []), item]);
    });
    map.forEach((items) => items.sort((a, b) => (a.pick_sequence ?? 999999) - (b.pick_sequence ?? 999999) || a.code.localeCompare(b.code)));
    return map;
  }, [locations]);

  const visibleIds = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return null;
    const ids = new Set<number>();
    const byId = new Map(locations.map((item) => [item.zone_id, item]));
    locations.forEach((item) => {
      if ([item.name, item.code, item.barcode, kindLabels[item.location_kind], roleLabels[item.process_role]].some((value) => value?.toLowerCase().includes(term))) {
        let current: WarehouseLocation | undefined = item;
        while (current) {
          ids.add(current.zone_id);
          current = current.parent_zone_id ? byId.get(current.parent_zone_id) : undefined;
        }
      }
    });
    return ids;
  }, [locations, search]);

  const openCreate = (parentId?: number) => {
    setEditingId(null);
    setForm(emptyForm(parentId ? String(parentId) : ''));
    setFormOpen(true);
  };

  const openEdit = (item: WarehouseLocation) => {
    setEditingId(item.zone_id);
    setForm({
      name: item.name, code: item.code, barcode: item.barcode || '', location_kind: item.location_kind,
      process_role: item.process_role, operational_status: item.operational_status, description: item.description || '',
      parent_zone_id: item.parent_zone_id ? String(item.parent_zone_id) : '', capacity: item.capacity ? String(item.capacity) : '',
      is_storable: item.is_storable, pick_sequence: item.pick_sequence === undefined ? '' : String(item.pick_sequence),
      max_weight_kg: item.max_weight_kg === undefined ? '' : String(item.max_weight_kg),
      inventory_frequency_days: item.inventory_frequency_days === undefined ? '' : String(item.inventory_frequency_days),
    });
    setFormOpen(true);
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      const payload = {
        name: form.name.trim(), code: form.code.trim() || undefined, barcode: form.barcode.trim() || undefined,
        location_kind: form.location_kind, process_role: form.process_role, operational_status: form.operational_status,
        description: form.description.trim() || undefined, parent_zone_id: numberOrUndefined(form.parent_zone_id),
        capacity: numberOrUndefined(form.capacity), capacity_mode: 'item_count', is_storable: form.is_storable,
        pick_sequence: numberOrUndefined(form.pick_sequence), max_weight_kg: numberOrUndefined(form.max_weight_kg),
        inventory_frequency_days: numberOrUndefined(form.inventory_frequency_days),
      };
      if (editingId) await warehouseApi.updateLocation(editingId, payload as Partial<WarehouseLocation>);
      else await warehouseApi.createLocation(payload as Partial<WarehouseLocation>);
      toast.success(editingId ? 'Lagerplatz aktualisiert' : 'Lagerplatz erstellt');
      setFormOpen(false);
      await load();
    } catch (error: unknown) {
      toast.error(apiErrorMessage(error, 'Lagerplatz konnte nicht gespeichert werden'));
    } finally { setSaving(false); }
  };

  const archive = async (item: WarehouseLocation) => {
    if (!confirm(`„${item.name}“ archivieren? Der Bereich muss vollständig leer sein.`)) return;
    try { await warehouseApi.archiveLocation(item.zone_id); toast.success('Lagerplatz archiviert'); await load(); }
    catch (error: unknown) { toast.error(apiErrorMessage(error, 'Lagerplatz konnte nicht archiviert werden')); }
  };

  const toggleExpanded = (id: number) => setExpanded((current) => {
    const next = new Set(current); if (next.has(id)) next.delete(id); else next.add(id); return next;
  });

  const renderTree = (parent: number | null, depth = 0): React.ReactNode => {
    const children = (childrenByParent.get(parent) || []).filter((item) => (showArchived || item.is_active) && (!visibleIds || visibleIds.has(item.zone_id)));
    return children.map((item) => {
      const hasChildren = (childrenByParent.get(item.zone_id) || []).some((child) => showArchived || child.is_active);
      const isExpanded = expanded.has(item.zone_id) || Boolean(visibleIds);
      const utilization = Math.max(0, Math.min(100, item.utilization_percent || 0));
      return (
        <div key={item.zone_id}>
          <div className="group grid items-center gap-3 rounded-lg border px-3 py-3 transition-colors sm:grid-cols-[minmax(280px,1fr)_150px_120px_150px_auto]" style={{ marginLeft: `${Math.min(depth, 5) * 1.25}rem`, borderColor: 'var(--border-subtle)', background: item.is_active ? 'var(--bg-card)' : 'var(--bg-subtle)', opacity: item.is_active ? 1 : 0.68 }}>
            <div className="flex min-w-0 items-center gap-2">
              <button type="button" onClick={() => hasChildren && toggleExpanded(item.zone_id)} className="flex h-7 w-7 items-center justify-center rounded-md" style={{ color: 'var(--text-secondary)' }}>{hasChildren ? (isExpanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />) : <span className="h-4 w-4" />}</button>
              <MapPin className="h-4 w-4 flex-shrink-0" style={{ color: item.operational_status === 'available' ? 'var(--color-success)' : 'var(--color-warning)' }} />
              <button type="button" onClick={() => navigate(`/zones/${item.zone_id}`)} className="min-w-0 text-left"><div className="truncate font-semibold" style={{ color: 'var(--text-primary)' }}>{item.name}</div><div className="truncate font-mono text-xs" style={{ color: 'var(--text-muted)' }}>{item.code} · {kindLabels[item.location_kind] || item.location_kind}</div></button>
            </div>
            <div className="text-sm" style={{ color: 'var(--text-secondary)' }}>{roleLabels[item.process_role] || item.process_role}</div>
            <div><span className="rounded-full px-2 py-1 text-xs font-semibold" style={{ background: 'var(--bg-subtle)', color: item.operational_status === 'available' ? 'var(--color-success)' : 'var(--color-warning)' }}>{statusLabels[item.operational_status] || item.operational_status}</span></div>
            <div><div className="flex justify-between text-xs" style={{ color: 'var(--text-secondary)' }}><span>{item.occupancy.toFixed(1)}{item.capacity ? ` / ${item.capacity}` : ''}</span><span>{item.capacity ? `${Math.round(utilization)} %` : 'ohne Limit'}</span></div><div className="mt-1 h-1.5 overflow-hidden rounded-full" style={{ background: 'var(--bg-subtle)' }}><div className="h-full rounded-full" style={{ width: `${item.capacity ? utilization : 0}%`, background: utilization >= 90 ? 'var(--color-error)' : 'var(--accent-red)' }} /></div><div className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>{item.device_count} Geräte · {item.case_count} Cases · {item.product_quantity.toFixed(1)} Menge</div></div>
            <div className="flex justify-end gap-1"><button type="button" onClick={() => openCreate(item.zone_id)} title="Unterbereich" className="rounded-md p-2" style={{ color: 'var(--text-secondary)' }}><Plus className="h-4 w-4" /></button><button type="button" onClick={() => openEdit(item)} title="Bearbeiten" className="rounded-md p-2" style={{ color: 'var(--text-secondary)' }}><Edit3 className="h-4 w-4" /></button>{item.is_active && <button type="button" onClick={() => archive(item)} title="Archivieren" className="rounded-md p-2" style={{ color: 'var(--color-warning)' }}><Archive className="h-4 w-4" /></button>}</div>
          </div>
          {hasChildren && isExpanded && renderTree(item.zone_id, depth + 1)}
        </div>
      );
    });
  };

  if (loading) return <div className="flex h-96 items-center justify-center"><RefreshCw className="h-8 w-8 animate-spin" style={{ color: 'var(--accent-red)' }} /></div>;

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><h1 className="text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>Lagersteuerung</h1><p className="mt-1 text-sm" style={{ color: 'var(--text-secondary)' }}>Physische Struktur, Prozessbereiche, Kapazitäten und Lageraufgaben</p></div><div className="flex gap-2"><button type="button" onClick={() => void load()} className="rounded-lg border p-2.5" style={{ borderColor: 'var(--border-subtle)', color: 'var(--text-secondary)', background: 'var(--bg-card)' }}><RefreshCw className="h-4 w-4" /></button><button type="button" onClick={() => openCreate()} className="flex items-center gap-2 rounded-lg px-4 py-2.5 font-semibold" style={{ background: 'var(--accent-red)', color: 'var(--text-primary)' }}><Plus className="h-4 w-4" />Lagerplatz</button></div></div>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><Metric icon={Warehouse} label="Aktive Orte" value={overview?.active_locations || 0} /><Metric icon={AlertTriangle} label="Nicht zugeordnet" value={(overview?.unplaced_devices || 0) + (overview?.unplaced_cases || 0)} detail={`${overview?.unplaced_product_quantity || 0} Mengeneinheiten ohne Ort`} warning /><Metric icon={ListChecks} label="Offene Aufgaben" value={overview?.open_tasks || 0} /><Metric icon={ClipboardCheck} label="Inventuren fällig" value={overview?.counts_due || 0} warning={Boolean(overview?.counts_due)} /></div>
      <div className="flex flex-wrap gap-1 rounded-lg border p-1" style={{ width: 'fit-content', borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}><Tab active={activeView === 'locations'} onClick={() => setActiveView('locations')} icon={Boxes}>Lagerstruktur</Tab><Tab active={activeView === 'tasks'} onClick={() => setActiveView('tasks')} icon={ListChecks}>Arbeitsvorrat ({tasks.filter((task) => task.status === 'open' || task.status === 'in_progress').length})</Tab><Tab active={activeView === 'counts'} onClick={() => setActiveView('counts')} icon={ClipboardCheck}>Inventur ({counts.filter((count) => count.status === 'counting' || count.status === 'review').length})</Tab></div>
      {formOpen && <LocationEditor form={form} setForm={setForm} locations={locations} editing={Boolean(editingId)} saving={saving} onSubmit={submit} onClose={() => setFormOpen(false)} />}
      {activeView === 'locations' && <section className="rounded-xl border p-4" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}><div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div className="relative flex-1 sm:max-w-md"><Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style={{ color: 'var(--text-muted)' }} /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Name, Code, Barcode oder Funktion" className="w-full rounded-lg border py-2.5 pl-10 pr-3" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-subtle)', color: 'var(--text-primary)' }} /></div><label className="flex items-center gap-2 text-sm" style={{ color: 'var(--text-secondary)' }}><input type="checkbox" checked={showArchived} onChange={(event) => setShowArchived(event.target.checked)} /> Archivierte anzeigen</label></div><div className="space-y-2">{renderTree(null)}{(childrenByParent.get(null) || []).length === 0 && <div className="py-12 text-center" style={{ color: 'var(--text-muted)' }}><PackageOpen className="mx-auto mb-3 h-10 w-10" />Noch kein Hauptstandort vorhanden.</div>}</div></section>}
      {activeView === 'tasks' && <TaskBoard tasks={tasks} reload={load} />}
      {activeView === 'counts' && <CountBoard counts={counts} locations={locations} reload={load} />}
    </div>
  );
}

function Metric({ icon: Icon, label, value, detail, warning = false }: { icon: typeof Warehouse; label: string; value: number; detail?: string; warning?: boolean }) {
  return <div className="rounded-xl border p-4" style={{ borderColor: warning ? 'var(--color-warning)' : 'var(--border-subtle)', background: 'var(--bg-card)' }}><div className="flex items-center justify-between"><div><div className="text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--text-muted)' }}>{label}</div><div className="mt-1 text-2xl font-bold" style={{ color: warning ? 'var(--color-warning)' : 'var(--text-primary)' }}>{value}</div></div><Icon className="h-5 w-5" style={{ color: warning ? 'var(--color-warning)' : 'var(--accent-red)' }} /></div>{detail && <div className="mt-2 text-xs" style={{ color: 'var(--text-secondary)' }}>{detail}</div>}</div>;
}

function Tab({ active, onClick, icon: Icon, children }: { active: boolean; onClick: () => void; icon: typeof Boxes; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} className="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-semibold" style={{ background: active ? 'var(--accent-red)' : 'transparent', color: active ? 'var(--text-primary)' : 'var(--text-secondary)' }}><Icon className="h-4 w-4" />{children}</button>;
}

function LocationEditor({ form, setForm, locations, editing, saving, onSubmit, onClose }: { form: LocationForm; setForm: React.Dispatch<React.SetStateAction<LocationForm>>; locations: WarehouseLocation[]; editing: boolean; saving: boolean; onSubmit: (event: FormEvent) => void; onClose: () => void }) {
  const optionStyle = { background: 'var(--bg-card)', color: 'var(--text-primary)' };
  const inputStyle = { borderColor: 'var(--border-subtle)', background: 'var(--bg-subtle)', color: 'var(--text-primary)' };
  return <form onSubmit={onSubmit} className="rounded-xl border p-5" style={{ borderColor: 'var(--accent-red)', background: 'var(--bg-card)' }}><div className="mb-4 flex items-center justify-between"><div><h2 className="font-bold" style={{ color: 'var(--text-primary)' }}>{editing ? 'Lagerplatz bearbeiten' : 'Lagerplatz erstellen'}</h2><p className="text-xs" style={{ color: 'var(--text-secondary)' }}>Bauart, Prozessfunktion und Lagerfähigkeit werden getrennt gepflegt.</p></div><button type="button" onClick={onClose} style={{ color: 'var(--text-secondary)' }}>Schließen</button></div><div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
    <Field label="Name"><input required value={form.name} onChange={(e) => setForm((x) => ({ ...x, name: e.target.value }))} style={inputStyle} /></Field>
    <Field label="Code"><input value={form.code} onChange={(e) => setForm((x) => ({ ...x, code: e.target.value }))} placeholder="wird automatisch erzeugt" style={inputStyle} /></Field>
    <Field label="Barcode"><input value={form.barcode} onChange={(e) => setForm((x) => ({ ...x, barcode: e.target.value }))} style={inputStyle} /></Field>
    <Field label="Übergeordneter Bereich"><select value={form.parent_zone_id} onChange={(e) => setForm((x) => ({ ...x, parent_zone_id: e.target.value }))} style={{...inputStyle,...optionStyle}}><option value="" style={optionStyle}>Hauptebene</option>{locations.map((item) => <option key={item.zone_id} value={item.zone_id} style={optionStyle}>{item.code} · {item.name}</option>)}</select></Field>
    <Field label="Bauart"><select value={form.location_kind} onChange={(e) => { const kind = e.target.value; setForm((x) => ({ ...x, location_kind: kind, is_storable: ['bin','floor','vehicle','virtual'].includes(kind) })); }} style={{...inputStyle,...optionStyle}}>{Object.entries(kindLabels).map(([value,label]) => <option key={value} value={value} style={optionStyle}>{label}</option>)}</select></Field>
    <Field label="Prozessfunktion"><select value={form.process_role} onChange={(e) => setForm((x) => ({ ...x, process_role: e.target.value }))} style={{...inputStyle,...optionStyle}}>{Object.entries(roleLabels).map(([value,label]) => <option key={value} value={value} style={optionStyle}>{label}</option>)}</select></Field>
    <Field label="Betriebsstatus"><select value={form.operational_status} onChange={(e) => setForm((x) => ({ ...x, operational_status: e.target.value }))} style={{...inputStyle,...optionStyle}}>{Object.entries(statusLabels).map(([value,label]) => <option key={value} value={value} style={optionStyle}>{label}</option>)}</select></Field>
    <Field label="Kapazität"><input type="number" min="0.01" step="0.01" value={form.capacity} onChange={(e) => setForm((x) => ({ ...x, capacity: e.target.value }))} style={inputStyle} /></Field>
    <Field label="Pick-Reihenfolge"><input type="number" value={form.pick_sequence} onChange={(e) => setForm((x) => ({ ...x, pick_sequence: e.target.value }))} style={inputStyle} /></Field>
    <Field label="Max. Gewicht (kg)"><input type="number" min="0" step="0.1" value={form.max_weight_kg} onChange={(e) => setForm((x) => ({ ...x, max_weight_kg: e.target.value }))} style={inputStyle} /></Field>
    <Field label="Inventurintervall (Tage)"><input type="number" min="0" value={form.inventory_frequency_days} onChange={(e) => setForm((x) => ({ ...x, inventory_frequency_days: e.target.value }))} style={inputStyle} /></Field>
    <Field label="Beschreibung"><input value={form.description} onChange={(e) => setForm((x) => ({ ...x, description: e.target.value }))} style={inputStyle} /></Field>
  </div><label className="mt-4 flex items-center gap-2 text-sm" style={{ color: 'var(--text-secondary)' }}><input type="checkbox" checked={form.is_storable} onChange={(e) => setForm((x) => ({ ...x, is_storable: e.target.checked }))} />Direkt belegbarer Lagerplatz</label><div className="mt-4 flex justify-end gap-2"><button type="button" onClick={onClose} className="rounded-lg border px-4 py-2" style={{ borderColor: 'var(--border-subtle)', color: 'var(--text-secondary)' }}>Abbrechen</button><button disabled={saving} className="rounded-lg px-4 py-2 font-semibold disabled:opacity-50" style={{ background: 'var(--accent-red)', color: 'var(--text-primary)' }}>{saving ? 'Speichert…' : 'Speichern'}</button></div></form>;
}

function Field({ label, children }: { label: string; children: React.ReactElement }) {
  return <label className="space-y-1 text-xs font-semibold" style={{ color: 'var(--text-secondary)' }}><span>{label}</span><div className="[&_input]:w-full [&_input]:rounded-lg [&_input]:border [&_input]:px-3 [&_input]:py-2.5 [&_select]:w-full [&_select]:rounded-lg [&_select]:border [&_select]:px-3 [&_select]:py-2.5">{children}</div></label>;
}

function TaskBoard({ tasks, reload }: { tasks: WarehouseTask[]; reload: () => Promise<void> }) {
  const update = async (task: WarehouseTask, status: string) => { try { await warehouseApi.updateTaskStatus(task.task_id, status); await reload(); } catch (error) { toast.error(String(error)); } };
  return <section className="rounded-xl border p-4" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}><div className="space-y-2">{tasks.length === 0 ? <div className="py-10 text-center" style={{ color: 'var(--text-muted)' }}>Keine Lageraufgaben vorhanden.</div> : tasks.map((task) => <div key={task.task_id} className="flex flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-subtle)' }}><div><div className="font-semibold" style={{ color: 'var(--text-primary)' }}>{task.task_type.toUpperCase()} · Priorität {task.priority}</div><div className="text-xs" style={{ color: 'var(--text-secondary)' }}>{task.notes || 'Ohne Beschreibung'}{task.due_at ? ` · fällig ${new Date(task.due_at).toLocaleString('de-DE')}` : ''}</div></div><div className="flex gap-2"><span className="rounded-full px-2 py-1 text-xs" style={{ background: 'var(--bg-card)', color: 'var(--text-secondary)' }}>{task.status}</span>{task.status === 'open' && <button onClick={() => void update(task,'in_progress')} className="rounded-md px-3 py-1 text-xs font-semibold" style={{ background: 'var(--accent-red)', color: 'var(--text-primary)' }}>Starten</button>}{task.status === 'in_progress' && <button onClick={() => void update(task,'done')} className="rounded-md px-3 py-1 text-xs font-semibold" style={{ background: 'var(--color-success)', color: 'var(--bg-primary)' }}>Abschließen</button>}</div></div>)}</div></section>;
}

function CountBoard({ counts, locations, reload }: { counts: InventoryCount[]; locations: WarehouseLocation[]; reload: () => Promise<void> }) {
  const [zoneId, setZoneId] = useState('');
  const [blind, setBlind] = useState(true);
  const [selectedId, setSelectedId] = useState<number | null>(counts.find((item) => item.status === 'counting' || item.status === 'review')?.count_id || null);
  const [detail, setDetail] = useState<InventoryCountDetail | null>(null);
  const [scanCode, setScanCode] = useState('');
  const [quantity, setQuantity] = useState('1');
  const [busy, setBusy] = useState(false);
  const optionStyle = { background: 'var(--bg-card)', color: 'var(--text-primary)' };
  const inputStyle = { borderColor: 'var(--border-subtle)', background: 'var(--bg-subtle)', color: 'var(--text-primary)' };

  const loadDetail = useCallback(async (id: number) => {
    try { const response = await warehouseApi.count(id); setDetail(response.data); setSelectedId(id); }
    catch (error: unknown) { toast.error(apiErrorMessage(error, 'Inventur konnte nicht geladen werden')); }
  }, []);

  useEffect(() => { if (selectedId) void loadDetail(selectedId); else setDetail(null); }, [selectedId, loadDetail]);

  const start = async () => {
    if (!zoneId) return;
    setBusy(true);
    try {
      const response = await warehouseApi.createCount(Number(zoneId), blind);
      toast.success('Inventur gestartet; der Lagerplatz ist bis zur Freigabe gesperrt.');
      await reload();
      await loadDetail(response.data.count_id);
    } catch (error: unknown) { toast.error(apiErrorMessage(error, 'Inventur konnte nicht gestartet werden')); }
    finally { setBusy(false); }
  };

  const scan = async (event: FormEvent) => {
    event.preventDefault();
    if (!detail || !scanCode.trim()) return;
    setBusy(true);
    try {
      await warehouseApi.scanCount(detail.count.count_id, scanCode.trim(), Number(quantity) || 1);
      setScanCode('');
      await loadDetail(detail.count.count_id);
    } catch (error: unknown) { toast.error(apiErrorMessage(error, 'Scan konnte nicht gezählt werden')); }
    finally { setBusy(false); }
  };

  const transition = async (action: 'complete' | 'approve' | 'cancel') => {
    if (!detail) return;
    if (action === 'cancel' && !confirm('Inventur wirklich abbrechen? Alle Zählergebnisse werden verworfen.')) return;
    if (action === 'approve' && !confirm('Zählergebnis freigeben und die Lagerbestände auf die gezählten Werte korrigieren?')) return;
    setBusy(true);
    try {
      if (action === 'complete') await warehouseApi.completeCount(detail.count.count_id);
      else if (action === 'approve') await warehouseApi.approveCount(detail.count.count_id);
      else await warehouseApi.cancelCount(detail.count.count_id);
      toast.success(action === 'complete' ? 'Zählung abgeschlossen – Abweichungen sind jetzt sichtbar.' : action === 'approve' ? 'Inventur freigegeben und Bestand abgeglichen.' : 'Inventur abgebrochen.');
      await reload();
      await loadDetail(detail.count.count_id);
    } catch (error: unknown) { toast.error(apiErrorMessage(error, 'Aktion fehlgeschlagen')); }
    finally { setBusy(false); }
  };

  const availableLocations = locations.filter((item) => item.is_active && item.is_storable && item.operational_status === 'available');
  return <div className="grid gap-4 xl:grid-cols-[340px_minmax(0,1fr)]">
    <aside className="space-y-4">
      <section className="rounded-xl border p-4" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}>
        <h2 className="font-bold" style={{ color: 'var(--text-primary)' }}>Neue Zählinventur</h2>
        <p className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>Der Lagerplatz wird während der Zählung automatisch gesperrt.</p>
        <select value={zoneId} onChange={(event) => setZoneId(event.target.value)} className="mt-3 w-full rounded-lg border px-3 py-2.5" style={{ ...inputStyle, ...optionStyle }}><option value="" style={optionStyle}>Lagerplatz wählen</option>{availableLocations.map((item) => <option key={item.zone_id} value={item.zone_id} style={optionStyle}>{item.code} · {item.name}</option>)}</select>
        <label className="mt-3 flex items-center gap-2 text-sm" style={{ color: 'var(--text-secondary)' }}><input type="checkbox" checked={blind} onChange={(event) => setBlind(event.target.checked)} />Blindzählung – Sollwerte erst bei Prüfung zeigen</label>
        <button type="button" disabled={!zoneId || busy} onClick={() => void start()} className="mt-3 w-full rounded-lg px-4 py-2.5 font-semibold disabled:opacity-50" style={{ background: 'var(--accent-red)', color: 'var(--text-primary)' }}>Inventur starten</button>
      </section>
      <section className="rounded-xl border p-3" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}><div className="mb-2 px-1 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--text-muted)' }}>Historie</div><div className="max-h-96 space-y-1 overflow-auto">{counts.map((count) => <button key={count.count_id} type="button" onClick={() => setSelectedId(count.count_id)} className="w-full rounded-lg border p-2.5 text-left" style={{ borderColor: selectedId === count.count_id ? 'var(--accent-red)' : 'transparent', background: 'var(--bg-subtle)', color: 'var(--text-primary)' }}><div className="flex items-center justify-between gap-2"><span className="truncate font-semibold">{count.zone_code} · {count.zone_name}</span><span className="rounded-full px-2 py-0.5 text-[11px]" style={{ background: 'var(--bg-card)', color: count.status === 'approved' ? 'var(--color-success)' : count.status === 'cancelled' ? 'var(--text-muted)' : 'var(--color-warning)' }}>{count.status}</span></div><div className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>{count.counted_lines}/{count.line_count} Positionen · {count.variance_lines} Abweichungen</div></button>)}{counts.length === 0 && <div className="py-6 text-center text-sm" style={{ color: 'var(--text-muted)' }}>Noch keine Inventuren</div>}</div></section>
    </aside>
    <section className="rounded-xl border p-4" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}>
      {!detail ? <div className="flex min-h-72 items-center justify-center text-center" style={{ color: 'var(--text-muted)' }}><div><ClipboardCheck className="mx-auto mb-3 h-10 w-10" /><div>Inventur starten oder aus der Historie wählen.</div></div></div> : <div className="space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><h2 className="font-bold" style={{ color: 'var(--text-primary)' }}>{detail.count.zone_code} · {detail.count.zone_name}</h2><p className="text-xs" style={{ color: 'var(--text-secondary)' }}>{detail.count.blind_count ? 'Blindzählung' : 'Offene Zählung'} · {detail.count.status}</p></div><div className="flex flex-wrap gap-2">{detail.count.status === 'counting' && <><button disabled={busy} onClick={() => void transition('complete')} className="rounded-lg px-3 py-2 text-sm font-semibold" style={{ background: 'var(--color-success)', color: 'var(--bg-primary)' }}>Zählung abschließen</button><button disabled={busy} onClick={() => void transition('cancel')} className="rounded-lg border px-3 py-2 text-sm" style={{ borderColor: 'var(--border-subtle)', color: 'var(--text-secondary)' }}>Abbrechen</button></>}{detail.count.status === 'review' && <><button disabled={busy} onClick={() => void transition('approve')} className="rounded-lg px-3 py-2 text-sm font-semibold" style={{ background: 'var(--color-success)', color: 'var(--bg-primary)' }}>Bestand freigeben</button><button disabled={busy} onClick={() => void transition('cancel')} className="rounded-lg border px-3 py-2 text-sm" style={{ borderColor: 'var(--border-subtle)', color: 'var(--text-secondary)' }}>Verwerfen</button></>}</div></div>
        {detail.count.status === 'counting' && <form onSubmit={scan} className="grid gap-2 rounded-lg border p-3 sm:grid-cols-[minmax(0,1fr)_120px_auto]" style={{ borderColor: 'var(--accent-red)', background: 'var(--bg-subtle)' }}><input autoFocus value={scanCode} onChange={(event) => setScanCode(event.target.value)} placeholder="Gerät, Mengenartikel oder Case scannen" className="rounded-lg border px-3 py-2.5" style={inputStyle} /><input type="number" min="0.001" step="0.001" value={quantity} onChange={(event) => setQuantity(event.target.value)} title="Menge" className="rounded-lg border px-3 py-2.5" style={inputStyle} /><button disabled={busy || !scanCode.trim()} className="rounded-lg px-4 py-2.5 font-semibold disabled:opacity-50" style={{ background: 'var(--accent-red)', color: 'var(--text-primary)' }}>Zählen</button></form>}
        <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr style={{ color: 'var(--text-muted)' }}><th className="px-2 py-2">Art</th><th className="px-2 py-2">Position</th><th className="px-2 py-2 text-right">Soll</th><th className="px-2 py-2 text-right">Gezählt</th><th className="px-2 py-2 text-right">Differenz</th></tr></thead><tbody>{detail.lines.map((line) => <tr key={line.line_id} className="border-t" style={{ borderColor: 'var(--border-subtle)', color: 'var(--text-primary)' }}><td className="px-2 py-2 uppercase" style={{ color: 'var(--text-secondary)' }}>{line.item_type}</td><td className="px-2 py-2"><div className="font-medium">{line.item_name}</div><div className="font-mono text-[11px]" style={{ color: 'var(--text-muted)' }}>{line.item_key}</div></td><td className="px-2 py-2 text-right">{line.expected_quantity ?? 'verdeckt'}</td><td className="px-2 py-2 text-right">{line.counted_quantity ?? 'offen'}</td><td className="px-2 py-2 text-right font-semibold" style={{ color: line.variance ? 'var(--color-warning)' : 'var(--color-success)' }}>{line.variance ?? '–'}</td></tr>)}{detail.lines.length === 0 && <tr><td colSpan={5} className="py-10 text-center" style={{ color: 'var(--text-muted)' }}>Der Lagerplatz war beim Start leer. Unerwartete Artikel können trotzdem gescannt werden.</td></tr>}</tbody></table></div>
      </div>}
    </section>
  </div>;
}
