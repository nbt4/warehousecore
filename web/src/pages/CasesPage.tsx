import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box, Boxes, CheckCircle2, ClipboardCheck, Lock, Package, PackageCheck,
  Plus, Printer, RefreshCw, ScanLine, Search, Send, Trash2, Unlock, Undo2, X,
} from 'lucide-react';
import {
  handlingUnitsApi, jobsApi, warehouseApi,
  type HandlingUnit, type HandlingUnitInput, type HandlingUnitInventory,
  type Job, type WarehouseLocation,
} from '../lib/api';
import { toast } from '../lib/toast';

const workflowLabels: Record<string, string> = {
  empty: 'Leer', packing: 'Wird gepackt', complete: 'Vollständig', sealed: 'Versiegelt',
  staged: 'Bereitgestellt', on_job: 'Auf Job', return_check: 'Rücklaufprüfung', maintenance: 'Wartung',
};
const typeLabels = { dynamic: 'Dynamisch', fixed: 'Festes Kit', hybrid: 'Hybrid' } as const;

const blankCase: HandlingUnitInput = { name: '', description: '', case_type: 'dynamic' };

function apiErrorMessage(error: unknown, fallback: string) {
  if (typeof error === 'object' && error !== null && 'response' in error) {
    const response = (error as { response?: { data?: { error?: string } } }).response;
    if (response?.data?.error) return response.data.error;
  }
  return error instanceof Error ? error.message : fallback;
}

export function CasesPage() {
  const navigate = useNavigate();
  const [cases, setCases] = useState<HandlingUnit[]>([]);
  const [locations, setLocations] = useState<WarehouseLocation[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [selected, setSelected] = useState<HandlingUnit | null>(null);
  const [inventory, setInventory] = useState<HandlingUnitInventory | null>(null);
  const [events, setEvents] = useState<Array<Record<string, unknown>>>([]);
  const [loading, setLoading] = useState(true);
  const [workspaceLoading, setWorkspaceLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [formOpen, setFormOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<HandlingUnitInput>(blankCase);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [caseResult, locationResult, jobResult] = await Promise.all([
        handlingUnitsApi.list({ search: searchQuery || undefined, workflow_status: statusFilter || undefined }),
        warehouseApi.locations(false), jobsApi.getAll(),
      ]);
      setCases(caseResult.data.cases);
      setLocations(locationResult.data.filter((item) => item.is_storable && item.operational_status === 'available'));
      setJobs(jobResult.data.filter((job) => !['completed', 'cancelled', 'canceled'].includes(job.status.toLowerCase())));
    } catch (error) { toast.error('Cases konnten nicht geladen werden: ' + String(error)); }
    finally { setLoading(false); }
  }, [searchQuery, statusFilter]);

  useEffect(() => { void load(); }, [load]);

  const openWorkspace = async (item: HandlingUnit) => {
    setSelected(item);
    setWorkspaceLoading(true);
    try {
      const [detailResult, inventoryResult, eventResult] = await Promise.all([
        handlingUnitsApi.get(item.case_id), handlingUnitsApi.inventory(item.case_id), handlingUnitsApi.events(item.case_id),
      ]);
      setSelected(detailResult.data); setInventory(inventoryResult.data); setEvents(eventResult.data);
    } catch (error) { toast.error('Case-Inhalt konnte nicht geladen werden: ' + String(error)); }
    finally { setWorkspaceLoading(false); }
  };

  const refreshWorkspace = async () => { if (selected) await openWorkspace(selected); await load(); };

  const openCreate = () => { setEditingId(null); setForm(blankCase); setFormOpen(true); };
  const openEdit = (item: HandlingUnit) => {
    setEditingId(item.case_id);
    setForm({ name: item.name, description: item.description, case_type: item.case_type, width: item.width, height: item.height, depth: item.depth, weight: item.weight, max_weight_kg: item.max_weight_kg, zone_id: item.zone_id, home_zone_id: item.home_zone_id, barcode: item.barcode, rfid_tag: item.rfid_tag });
    setFormOpen(true);
  };

  const saveCase = async (event: FormEvent) => {
    event.preventDefault();
    try {
      if (editingId) await handlingUnitsApi.update(editingId, form);
      else await handlingUnitsApi.create(form);
      toast.success(editingId ? 'Case aktualisiert' : 'Case erstellt'); setFormOpen(false); await load();
    } catch (error: unknown) { toast.error(apiErrorMessage(error, 'Case konnte nicht gespeichert werden')); }
  };

  const deleteCase = async (item: HandlingUnit) => {
    if (!confirm(`Case „${item.name}“ wirklich löschen?`)) return;
    try { await handlingUnitsApi.delete(item.case_id); toast.success('Case gelöscht'); if (selected?.case_id === item.case_id) { setSelected(null); setInventory(null); } await load(); }
    catch (error: unknown) { toast.error(apiErrorMessage(error, 'Case konnte nicht gelöscht werden')); }
  };

  const summary = useMemo(() => ({
    total: cases.length,
    dynamic: cases.filter((item) => item.case_type === 'dynamic').length,
    packing: cases.filter((item) => ['packing','complete','sealed','staged'].includes(item.workflow_status)).length,
    onJob: cases.filter((item) => item.workflow_status === 'on_job').length,
  }), [cases]);

  return <div className="space-y-5">
    <header className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><h1 className="text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>Cases & Handling Units</h1><p className="mt-1 text-sm" style={{ color: 'var(--text-secondary)' }}>Dynamische Euroboxen, feste Kits und hybride Cases scanbasiert packen</p></div><div className="flex gap-2"><button onClick={() => void load()} className="rounded-lg border p-2.5" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)', color: 'var(--text-secondary)' }}><RefreshCw className="h-4 w-4" /></button><button onClick={openCreate} className="flex items-center gap-2 rounded-lg px-4 py-2.5 font-semibold" style={{ background: 'var(--accent-red)', color: 'var(--text-primary)' }}><Plus className="h-4 w-4" />Neues Case</button></div></header>
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><Metric label="Cases gesamt" value={summary.total} icon={Boxes} /><Metric label="Dynamisch" value={summary.dynamic} icon={Box} /><Metric label="In Vorbereitung" value={summary.packing} icon={PackageCheck} /><Metric label="Auf Job" value={summary.onJob} icon={Send} /></div>
    <form onSubmit={(event) => { event.preventDefault(); setSearchQuery(search.trim()); }} className="flex flex-col gap-3 rounded-xl border p-3 sm:flex-row" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}><div className="suite-search-field flex-1"><Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style={{ color: 'var(--text-muted)' }} /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Name, Barcode oder Case-ID" className="w-full rounded-lg border py-2.5 pl-10 pr-3" style={fieldStyle} /></div><select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)} className="rounded-lg border px-3 py-2.5" style={selectStyle}><option value="" style={optionStyle}>Alle Status</option>{Object.entries(workflowLabels).map(([value,label]) => <option key={value} value={value} style={optionStyle}>{label}</option>)}</select><button className="rounded-lg px-4 py-2 font-semibold" style={{ background: 'var(--accent-red)', color: 'var(--text-primary)' }}>Suchen</button></form>
    {formOpen && <CaseEditor form={form} setForm={setForm} locations={locations} editing={Boolean(editingId)} onSubmit={saveCase} onClose={() => setFormOpen(false)} />}
    <div className="grid items-start gap-5 xl:grid-cols-[minmax(360px,0.85fr)_minmax(620px,1.7fr)]">
      <section className="rounded-xl border p-3" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}>{loading ? <Loading /> : cases.length === 0 ? <Empty text="Keine Cases gefunden" /> : <div className="space-y-2">{cases.map((item) => <CaseRow key={item.case_id} item={item} active={selected?.case_id === item.case_id} onOpen={() => void openWorkspace(item)} onEdit={() => openEdit(item)} onLabel={() => navigate('/labels')} onDelete={() => void deleteCase(item)} />)}</div>}</section>
      <section className="min-h-[440px] rounded-xl border" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}>{selected ? <PackingWorkspace unit={selected} inventory={inventory} events={events} locations={locations} jobs={jobs} loading={workspaceLoading} refresh={refreshWorkspace} close={() => { setSelected(null); setInventory(null); }} /> : <Empty text="Case auswählen, um Packen, Soll-Inhalt, Ausgabe und Rücklauf zu bearbeiten." />}</section>
    </div>
  </div>;
}

const fieldStyle = { borderColor: 'var(--border-subtle)', background: 'var(--bg-subtle)', color: 'var(--text-primary)' };
const optionStyle = { background: 'var(--bg-card)', color: 'var(--text-primary)' };
const selectStyle = { ...fieldStyle, ...optionStyle };

function Metric({ label, value, icon: Icon }: { label: string; value: number; icon: typeof Boxes }) { return <div className="rounded-xl border p-4" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-card)' }}><div className="flex items-center justify-between"><div><div className="text-xs font-semibold uppercase" style={{ color: 'var(--text-muted)' }}>{label}</div><div className="mt-1 text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>{value}</div></div><Icon className="h-5 w-5" style={{ color: 'var(--accent-red)' }} /></div></div>; }
function Loading() { return <div className="flex h-56 items-center justify-center"><RefreshCw className="h-7 w-7 animate-spin" style={{ color: 'var(--accent-red)' }} /></div>; }
function Empty({ text }: { text: string }) { return <div className="flex min-h-[300px] flex-col items-center justify-center p-8 text-center text-sm" style={{ color: 'var(--text-muted)' }}><Package className="mb-3 h-10 w-10" />{text}</div>; }

function CaseRow({ item, active, onOpen, onEdit, onLabel, onDelete }: { item: HandlingUnit; active: boolean; onOpen: () => void; onEdit: () => void; onLabel: () => void; onDelete: () => void }) {
  return <div className="rounded-lg border p-3" style={{ borderColor: active ? 'var(--accent-red)' : 'var(--border-subtle)', background: active ? 'var(--bg-subtle)' : 'transparent' }}><button onClick={onOpen} className="w-full text-left"><div className="flex items-start justify-between gap-2"><div><div className="font-semibold" style={{ color: 'var(--text-primary)' }}>{item.name}</div><div className="mt-0.5 font-mono text-xs" style={{ color: 'var(--text-muted)' }}>{item.barcode || `CASE-${item.case_id}`}</div></div><span className="rounded-full px-2 py-1 text-[11px] font-semibold" style={{ background: 'var(--bg-card)', color: item.workflow_status === 'on_job' ? 'var(--color-warning)' : 'var(--color-success)' }}>{workflowLabels[item.workflow_status] || item.workflow_status}</span></div><div className="mt-3 flex flex-wrap gap-2 text-xs" style={{ color: 'var(--text-secondary)' }}><span>{typeLabels[item.case_type]}</span><span>•</span><span>{item.device_count} Geräte</span><span>{item.product_quantity.toFixed(1)} Menge</span>{item.child_case_count > 0 && <span>{item.child_case_count} Untercases</span>}</div>{item.zone_name && <div className="mt-2 text-xs" style={{ color: 'var(--text-muted)' }}>{item.zone_code} · {item.zone_name}</div>}</button><div className="mt-3 flex justify-end gap-1"><button onClick={onLabel} title="Case-Label im Label Studio" className="rounded-md p-1.5" style={{ color: 'var(--text-secondary)' }}><Printer className="h-4 w-4" /></button><button onClick={onEdit} className="rounded-md px-2 py-1 text-xs" style={{ color: 'var(--text-secondary)' }}>Bearbeiten</button><button onClick={onDelete} className="rounded-md p-1.5" style={{ color: 'var(--color-error)' }}><Trash2 className="h-4 w-4" /></button></div></div>;
}

function CaseEditor({ form, setForm, locations, editing, onSubmit, onClose }: { form: HandlingUnitInput; setForm: React.Dispatch<React.SetStateAction<HandlingUnitInput>>; locations: WarehouseLocation[]; editing: boolean; onSubmit: (event: FormEvent) => void; onClose: () => void }) {
  return <form onSubmit={onSubmit} className="rounded-xl border p-5" style={{ borderColor: 'var(--accent-red)', background: 'var(--bg-card)' }}><div className="mb-4 flex justify-between"><div><h2 className="font-bold" style={{ color: 'var(--text-primary)' }}>{editing ? 'Case bearbeiten' : 'Case erstellen'}</h2><p className="text-xs" style={{ color: 'var(--text-secondary)' }}>Dynamisch = wechselnder Inhalt, Fest/Hybrid = Soll-Inhalt möglich.</p></div><button type="button" onClick={onClose}><X className="h-5 w-5" style={{ color: 'var(--text-secondary)' }} /></button></div><div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4"><EditorField label="Name"><input required value={form.name} onChange={(e) => setForm((x) => ({...x,name:e.target.value}))} style={fieldStyle} /></EditorField><EditorField label="Case-Typ"><select value={form.case_type} onChange={(e) => setForm((x) => ({...x,case_type:e.target.value as HandlingUnitInput['case_type']}))} style={selectStyle}>{Object.entries(typeLabels).map(([value,label]) => <option key={value} value={value} style={optionStyle}>{label}</option>)}</select></EditorField><EditorField label="Aktueller Lagerplatz"><select value={form.zone_id || ''} onChange={(e) => setForm((x) => ({...x,zone_id:e.target.value ? Number(e.target.value) : undefined}))} style={selectStyle}><option value="" style={optionStyle}>Nicht zugeordnet</option>{locations.map((location) => <option key={location.zone_id} value={location.zone_id} style={optionStyle}>{location.code} · {location.name}</option>)}</select></EditorField><EditorField label="Heimatlagerplatz"><select value={form.home_zone_id || ''} onChange={(e) => setForm((x) => ({...x,home_zone_id:e.target.value ? Number(e.target.value) : undefined}))} style={selectStyle}><option value="" style={optionStyle}>Kein Heimatplatz</option>{locations.map((location) => <option key={location.zone_id} value={location.zone_id} style={optionStyle}>{location.code} · {location.name}</option>)}</select></EditorField><EditorField label="Barcode"><input value={form.barcode || ''} onChange={(e) => setForm((x) => ({...x,barcode:e.target.value || undefined}))} placeholder="wird automatisch erzeugt" style={fieldStyle} /></EditorField><EditorField label="RFID"><input value={form.rfid_tag || ''} onChange={(e) => setForm((x) => ({...x,rfid_tag:e.target.value || undefined}))} style={fieldStyle} /></EditorField><EditorField label="Eigengewicht (kg)"><input type="number" min="0" step="0.1" value={form.weight || ''} onChange={(e) => setForm((x) => ({...x,weight:e.target.value ? Number(e.target.value) : undefined}))} style={fieldStyle} /></EditorField><EditorField label="Max. Gesamtgewicht (kg)"><input type="number" min="0" step="0.1" value={form.max_weight_kg || ''} onChange={(e) => setForm((x) => ({...x,max_weight_kg:e.target.value ? Number(e.target.value) : undefined}))} style={fieldStyle} /></EditorField></div><EditorField label="Beschreibung"><textarea value={form.description || ''} onChange={(e) => setForm((x) => ({...x,description:e.target.value || undefined}))} className="mt-3 w-full rounded-lg border px-3 py-2" style={fieldStyle} /></EditorField><div className="mt-4 flex justify-end gap-2"><button type="button" onClick={onClose} className="rounded-lg border px-4 py-2" style={{ borderColor: 'var(--border-subtle)', color: 'var(--text-secondary)' }}>Abbrechen</button><button className="rounded-lg px-4 py-2 font-semibold" style={{ background: 'var(--accent-red)', color: 'var(--text-primary)' }}>Speichern</button></div></form>;
}
function EditorField({ label, children }: { label: string; children: React.ReactElement }) { return <label className="block space-y-1 text-xs font-semibold" style={{ color: 'var(--text-secondary)' }}><span>{label}</span><div className="[&_input]:w-full [&_input]:rounded-lg [&_input]:border [&_input]:px-3 [&_input]:py-2 [&_select]:w-full [&_select]:rounded-lg [&_select]:border [&_select]:px-3 [&_select]:py-2">{children}</div></label>; }

function PackingWorkspace({ unit, inventory, events, locations, jobs, loading, refresh, close }: { unit: HandlingUnit; inventory: HandlingUnitInventory | null; events: Array<Record<string, unknown>>; locations: WarehouseLocation[]; jobs: Job[]; loading: boolean; refresh: () => Promise<void>; close: () => void }) {
  const scanRef = useRef<HTMLInputElement>(null);
  const [scanCode,setScanCode]=useState(''); const [quantity,setQuantity]=useState(1); const [sourceZone,setSourceZone]=useState('');
  const [templateCode,setTemplateCode]=useState(''); const [templateQuantity,setTemplateQuantity]=useState(1);
  const [destination,setDestination]=useState(''); const [jobId,setJobId]=useState(''); const [busy,setBusy]=useState(false);
  const editable=!['sealed','on_job'].includes(unit.workflow_status);
  const run=async(action:()=>Promise<unknown>,success:string)=>{setBusy(true);try{await action();toast.success(success);await refresh()}catch(error:unknown){toast.error(apiErrorMessage(error,'Aktion fehlgeschlagen'))}finally{setBusy(false)}};
  const pack=async(event:FormEvent)=>{event.preventDefault();if(!scanCode.trim())return;await run(()=>handlingUnitsApi.packScan(unit.case_id,{scan_code:scanCode.trim(),quantity,source_zone_id:sourceZone?Number(sourceZone):undefined}),'Inhalt hinzugefügt');setScanCode('');setQuantity(1);scanRef.current?.focus()};
  const addTemplate=async(event:FormEvent)=>{event.preventDefault();if(!templateCode.trim())return;await run(()=>handlingUnitsApi.setTemplateByScan(unit.case_id,templateCode.trim(),templateQuantity),'Soll-Inhalt gespeichert');setTemplateCode('');setTemplateQuantity(1)};
  const requireDestination=()=>{if(!destination){toast.error('Bitte zuerst einen Lagerplatz wählen');return false}return true};
  if(loading||!inventory)return <Loading/>;
  return <div>
    <div className="border-b p-4" style={{borderColor:'var(--border-subtle)'}}><div className="flex items-start justify-between gap-3"><div><div className="flex flex-wrap items-center gap-2"><h2 className="text-xl font-bold" style={{color:'var(--text-primary)'}}>{unit.name}</h2><span className="rounded-full px-2 py-1 text-xs font-semibold" style={{background:'var(--bg-subtle)',color:'var(--color-success)'}}>{typeLabels[unit.case_type]}</span><span className="rounded-full px-2 py-1 text-xs font-semibold" style={{background:'var(--bg-subtle)',color:unit.workflow_status==='on_job'?'var(--color-warning)':'var(--text-secondary)'}}>{workflowLabels[unit.workflow_status]}</span></div><div className="mt-1 font-mono text-xs" style={{color:'var(--text-muted)'}}>{unit.barcode||`CASE-${unit.case_id}`}{unit.zone_code?` · ${unit.zone_code}`:''}</div></div><button onClick={close}><X className="h-5 w-5" style={{color:'var(--text-secondary)'}}/></button></div><div className="mt-4 grid grid-cols-4 gap-2"><Mini label="Geräte" value={inventory.devices.length}/><Mini label="Mengenartikel" value={inventory.products.reduce((sum,item)=>sum+item.quantity,0)}/><Mini label="Untercases" value={inventory.child_cases.length}/><Mini label="Soll vollständig" value={inventory.complete?'Ja':'Nein'} ok={inventory.complete}/></div></div>
    <div className="space-y-5 p-4">
      <form onSubmit={pack} className="rounded-xl border p-4" style={{borderColor:editable?'var(--accent-red)':'var(--border-subtle)',background:'var(--bg-subtle)'}}><div className="mb-3 flex items-center gap-2 font-semibold" style={{color:'var(--text-primary)'}}><ScanLine className="h-5 w-5" style={{color:'var(--accent-red)'}}/>Inhalt scannen</div><div className="grid gap-2 sm:grid-cols-[1fr_100px_180px_auto]"><input ref={scanRef} autoFocus value={scanCode} onChange={(e)=>setScanCode(e.target.value)} placeholder="Gerät, Artikel, Untercase" disabled={!editable||busy} className="rounded-lg border px-3 py-3" style={fieldStyle}/><input type="number" min="0.01" step="0.01" value={quantity} onChange={(e)=>setQuantity(Number(e.target.value))} disabled={!editable||busy} className="rounded-lg border px-3 py-3" style={fieldStyle}/><select value={sourceZone} onChange={(e)=>setSourceZone(e.target.value)} disabled={!editable||busy} className="rounded-lg border px-3" style={selectStyle}><option value="" style={optionStyle}>Quelle automatisch</option>{locations.map((x)=><option key={x.zone_id} value={x.zone_id} style={optionStyle}>{x.code}</option>)}</select><button disabled={!editable||busy||!scanCode.trim()} className="rounded-lg px-4 font-semibold disabled:opacity-50" style={{background:'var(--accent-red)',color:'var(--text-primary)'}}>Hinzufügen</button></div><p className="mt-2 text-xs" style={{color:'var(--text-muted)'}}>Bei Einzelgeräten wird die Menge ignoriert. Ein Case-Barcode verschachtelt das Untercase.</p></form>
      {(unit.case_type==='fixed'||unit.case_type==='hybrid')&&<section className="rounded-xl border p-4" style={{borderColor:'var(--border-subtle)'}}><div className="mb-3 flex items-center gap-2 font-semibold" style={{color:'var(--text-primary)'}}><ClipboardCheck className="h-5 w-5"/>Soll-Inhalt</div><form onSubmit={addTemplate} className="grid gap-2 sm:grid-cols-[1fr_110px_auto]"><input value={templateCode} onChange={(e)=>setTemplateCode(e.target.value)} placeholder="Artikelbarcode oder Produkt-ID" className="rounded-lg border px-3 py-2" style={fieldStyle}/><input type="number" min="0.01" step="0.01" value={templateQuantity} onChange={(e)=>setTemplateQuantity(Number(e.target.value))} className="rounded-lg border px-3 py-2" style={fieldStyle}/><button disabled={busy} className="rounded-lg px-3 font-semibold" style={{background:'var(--bg-subtle)',color:'var(--text-primary)'}}>Soll setzen</button></form><div className="mt-3 space-y-2">{inventory.template.map((line)=><div key={line.product_id} className="flex items-center justify-between rounded-lg p-2" style={{background:'var(--bg-subtle)'}}><div><div className="text-sm font-semibold" style={{color:'var(--text-primary)'}}>{line.product_name}</div><div className="text-xs" style={{color:'var(--text-secondary)'}}>{line.actual_quantity} / {line.expected_quantity}</div></div><div className="flex items-center gap-2">{line.complete?<CheckCircle2 className="h-4 w-4" style={{color:'var(--color-success)'}}/>:<span className="text-xs" style={{color:'var(--color-warning)'}}>fehlt</span>}<button onClick={()=>void run(()=>handlingUnitsApi.removeTemplate(unit.case_id,line.product_id),'Soll-Inhalt entfernt')}><Trash2 className="h-4 w-4" style={{color:'var(--color-error)'}}/></button></div></div>)}</div></section>}
      <section><h3 className="mb-2 font-semibold" style={{color:'var(--text-primary)'}}>Tatsächlicher Inhalt</h3><div className="space-y-2">{inventory.devices.map((item)=><ContentRow key={item.device_id} title={item.product_name||item.device_id} detail={`${item.device_id}${item.serial_number?` · SN ${item.serial_number}`:''}`} type="Gerät" onRemove={editable?()=>void run(()=>handlingUnitsApi.removeDevice(unit.case_id,item.device_id),'Gerät entfernt'):undefined}/>) }{inventory.products.map((item)=><ContentRow key={`p-${item.product_id}`} title={item.product_name} detail={`${item.quantity} ${item.unit}`} type="Menge" onRemove={editable?()=>{if(requireDestination()){const raw=prompt(`Wie viel ${item.unit} zurücklagern?`,String(item.quantity));if(raw)void run(()=>handlingUnitsApi.removeProduct(unit.case_id,item.product_id,{quantity:Number(raw),destination_zone_id:Number(destination)}),'Artikel zurückgelagert')}}:undefined}/>) }{inventory.child_cases.map((item)=><ContentRow key={`c-${item.case_id}`} title={item.name} detail={item.barcode||`CASE-${item.case_id}`} type="Untercase" onRemove={editable?()=>{if(requireDestination())void run(()=>handlingUnitsApi.removeChild(unit.case_id,item.case_id,Number(destination)),'Untercase zurückgelagert')}:undefined}/>) }{inventory.devices.length+inventory.products.length+inventory.child_cases.length===0&&<div className="rounded-lg border p-6 text-center text-sm" style={{borderColor:'var(--border-subtle)',color:'var(--text-muted)'}}>Case ist leer</div>}</div></section>
      <section className="rounded-xl border p-4" style={{borderColor:'var(--border-subtle)',background:'var(--bg-subtle)'}}><div className="mb-3 font-semibold" style={{color:'var(--text-primary)'}}>Case-Aktionen</div><div className="grid gap-2 sm:grid-cols-2"><select value={destination} onChange={(e)=>setDestination(e.target.value)} className="rounded-lg border px-3 py-2" style={selectStyle}><option value="" style={optionStyle}>Ziel-/Rücklagerplatz wählen</option>{locations.map((x)=><option key={x.zone_id} value={x.zone_id} style={optionStyle}>{x.code} · {x.name}</option>)}</select><select value={jobId} onChange={(e)=>setJobId(e.target.value)} className="rounded-lg border px-3 py-2" style={selectStyle}><option value="" style={optionStyle}>Job für Ausgabe wählen</option>{jobs.map((job)=><option key={job.job_id} value={job.job_id} style={optionStyle}>{job.job_code} · {job.description||job.status}</option>)}</select></div><div className="mt-3 flex flex-wrap gap-2">{editable?<Action icon={Lock} label="Versiegeln" onClick={()=>void run(()=>handlingUnitsApi.seal(unit.case_id,false),'Case versiegelt')}/>:unit.workflow_status!=='on_job'&&<Action icon={Unlock} label="Öffnen" onClick={()=>void run(()=>handlingUnitsApi.unseal(unit.case_id),'Case geöffnet')}/>}<Action icon={Send} label="Für Job ausgeben" disabled={!jobId||unit.workflow_status!=='sealed'} onClick={()=>void run(()=>handlingUnitsApi.dispatch(unit.case_id,Number(jobId)),'Case ausgegeben')}/>{unit.workflow_status==='on_job'&&<><Action icon={Undo2} label="Rücklauf + Prüfung" disabled={!destination} onClick={()=>void run(()=>handlingUnitsApi.returnCase(unit.case_id,Number(destination),'inspect'),'Rücklauf erfasst')}/><Action icon={PackageCheck} label="Versiegelt zurück" disabled={!destination} onClick={()=>void run(()=>handlingUnitsApi.returnCase(unit.case_id,Number(destination),'sealed'),'Versiegelter Rücklauf erfasst')}/></>}<Action icon={PackageCheck} label="Vollständig entpacken" disabled={!destination||['on_job','sealed'].includes(unit.workflow_status)} onClick={()=>confirm('Case vollständig entpacken und alle Inhalte auf den gewählten Lagerplatz buchen?')&&void run(()=>handlingUnitsApi.unpack(unit.case_id,Number(destination)),'Case entpackt')}/></div></section>
      {events.length>0&&<details><summary className="cursor-pointer text-sm font-semibold" style={{color:'var(--text-secondary)'}}>Verlauf ({events.length})</summary><div className="mt-2 max-h-48 space-y-1 overflow-auto">{events.slice(0,30).map((event,index)=><div key={String(event.event_id||index)} className="flex justify-between rounded-md px-2 py-1 text-xs" style={{background:'var(--bg-subtle)',color:'var(--text-secondary)'}}><span>{String(event.event_type)}</span><span>{event.created_at?new Date(String(event.created_at)).toLocaleString('de-DE'):''}</span></div>)}</div></details>}
    </div>
  </div>;
}

function Mini({label,value,ok=false}:{label:string;value:string|number;ok?:boolean}){return <div className="rounded-lg p-2" style={{background:'var(--bg-subtle)'}}><div className="text-[10px] uppercase" style={{color:'var(--text-muted)'}}>{label}</div><div className="font-bold" style={{color:ok?'var(--color-success)':'var(--text-primary)'}}>{value}</div></div>}
function ContentRow({title,detail,type,onRemove}:{title:string;detail:string;type:string;onRemove?:()=>void}){return <div className="flex items-center justify-between rounded-lg border p-3" style={{borderColor:'var(--border-subtle)',background:'var(--bg-subtle)'}}><div><div className="text-sm font-semibold" style={{color:'var(--text-primary)'}}>{title}</div><div className="text-xs" style={{color:'var(--text-secondary)'}}>{type} · {detail}</div></div>{onRemove&&<button onClick={onRemove}><Trash2 className="h-4 w-4" style={{color:'var(--color-error)'}}/></button>}</div>}
function Action({icon:Icon,label,onClick,disabled=false}:{icon:typeof Lock;label:string;onClick:()=>void;disabled?:boolean}){return <button onClick={onClick} disabled={disabled} className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-semibold disabled:opacity-40" style={{borderColor:'var(--border-subtle)',background:'var(--bg-card)',color:'var(--text-primary)'}}><Icon className="h-4 w-4"/>{label}</button>}
