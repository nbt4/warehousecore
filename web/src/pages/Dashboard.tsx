import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Activity, AlertTriangle, ArrowDownToLine, ArrowRight, ArrowUpFromLine, Boxes,
  BriefcaseBusiness, CheckCircle2, ClipboardCheck, Clock3, MapPinOff, MoveRight,
  PackageCheck, RefreshCw, ScanLine, ShieldAlert, Warehouse, Wrench,
} from 'lucide-react';
import { dashboardApi, warehouseApi } from '../lib/api';
import type { DashboardStats, Movement, WarehouseOverview } from '../lib/api';
import { LowStockAlertsWidget } from '../components/LowStockAlertsWidget';
import { toast } from '../lib/toast';
import { useAuth } from '../contexts/AuthContext';
import { suiteGreeting } from '../lib/cores-design';

const emptyStats: DashboardStats = {
  in_storage: 0, on_job: 0, return_pending: 0, location_unknown: 0,
  available: 0, blocked: 0, defective: 0, maintenance: 0, retired: 0, total: 0,
  ready_for_dispatch: 0, unavailable: 0, movements_today: 0, intakes_today: 0,
  outtakes_today: 0, transfers_today: 0, active_jobs: 0, cases_total: 0,
  cases_on_job: 0, cases_return_check: 0, cases_packing: 0, open_defects: 0,
  overdue_inspections: 0,
};

function relativeTime(value: string) {
  const timestamp = new Date(value).getTime();
  if (!Number.isFinite(timestamp)) return '';
  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000));
  if (seconds < 60) return 'gerade eben';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `vor ${minutes} ${minutes === 1 ? 'Minute' : 'Minuten'}`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `vor ${hours} ${hours === 1 ? 'Stunde' : 'Stunden'}`;
  const days = Math.floor(hours / 24);
  return `vor ${days} ${days === 1 ? 'Tag' : 'Tagen'}`;
}

function movementText(item: Movement) {
  const device = item.product_name || item.serial_number || item.device_id;
  if (item.action === 'intake') return `${device} in ${item.to_zone_name || 'das Lager'} eingelagert`;
  if (item.action === 'outtake') return `${device} für ${item.to_job_description || 'einen Job'} ausgegeben`;
  if (item.action === 'transfer' || item.action === 'move' || item.action === 'assignment') {
    if (item.from_zone_name && item.to_zone_name) return `${device}: ${item.from_zone_name} → ${item.to_zone_name}`;
    return `${device} nach ${item.to_zone_name || 'neuem Lagerplatz'} bewegt`;
  }
  if (item.action === 'return') return `${device} zurückgenommen`;
  return `${device}: ${item.action}`;
}

export function Dashboard() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const [stats, setStats] = useState<DashboardStats>(emptyStats);
  const [overview, setOverview] = useState<WarehouseOverview | null>(null);
  const [activity, setActivity] = useState<Movement[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);

  const loadData = useCallback(async (notify = false) => {
    setRefreshing(true);
    try {
      const [statsResult, movementResult, overviewResult] = await Promise.all([
        dashboardApi.getStats(), dashboardApi.getRecentMovements(20), warehouseApi.overview(),
      ]);
      setStats(statsResult.data);
      setActivity(movementResult.data || []);
      setOverview(overviewResult.data);
      setLastUpdated(new Date());
    } catch (error) {
      if (notify) toast.error(`Dashboard konnte nicht aktualisiert werden: ${String(error)}`);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void loadData(true);
    const interval = window.setInterval(() => void loadData(), 30_000);
    return () => window.clearInterval(interval);
  }, [loadData]);

  const unassigned = (overview?.unplaced_devices || 0) + (overview?.unplaced_cases || 0);
  const readiness = stats.total ? Math.round((stats.ready_for_dispatch / stats.total) * 100) : 0;

  const priorities = useMemo(() => {
    const items: Array<{ title: string; detail: string; value: number; tone: 'critical' | 'warning'; path: string }> = [];
    if (overview && overview.active_locations === 0) items.push({ title: 'Lagerstruktur fehlt', detail: 'Ohne belegbare Lagerplätze bleibt Material ungeklärt.', value: unassigned, tone: 'critical', path: '/zones' });
    else if (unassigned > 0) items.push({ title: 'Material ohne Lagerplatz', detail: `${overview?.unplaced_devices || 0} Geräte und ${overview?.unplaced_cases || 0} Cases zuordnen.`, value: unassigned, tone: 'warning', path: '/zones' });
    if (stats.return_pending > 0 || stats.cases_return_check > 0) items.push({ title: 'Rücklauf bearbeiten', detail: 'Geräte und Cases warten auf Prüfung oder Einlagerung.', value: stats.return_pending + stats.cases_return_check, tone: 'warning', path: '/scan?mode=returns' });
    if (stats.open_defects > 0 || stats.overdue_inspections > 0) items.push({ title: 'Technische Klärung', detail: `${stats.open_defects} offene Defekte · ${stats.overdue_inspections} Prüfungen überfällig`, value: stats.open_defects + stats.overdue_inspections, tone: 'warning', path: '/maintenance' });
    if ((overview?.counts_due || 0) > 0) items.push({ title: 'Inventuren fällig', detail: 'Fällige Lagerplätze jetzt zählen.', value: overview?.counts_due || 0, tone: 'warning', path: '/zones' });
    return items;
  }, [overview, stats, unassigned]);

  const flow = [
    { label: 'Im Lager', value: stats.in_storage, color: 'var(--color-success)', path: '/zones' },
    { label: 'Auf Job', value: stats.on_job, color: 'var(--color-info)', path: '/jobs' },
    { label: 'Rückgabe offen', value: stats.return_pending, color: 'var(--color-warning)', path: '/scan?mode=returns' },
    { label: 'Standort ungeklärt', value: stats.location_unknown, color: 'var(--color-error)', path: '/zones' },
  ];

  if (loading) return <DashboardSkeleton />;

  return (
    <div className="suite-dashboard">
      <header className="suite-dashboard-header">
        <div className="suite-dashboard-heading">
          <div className="suite-dashboard-eyebrow" style={{ color: 'var(--color-success)' }}><span className="suite-dashboard-eyebrow-dot animate-pulse" />Live-Lagerbetrieb</div>
          <h1 className="suite-dashboard-title">{suiteGreeting(user)}</h1>
          <p className="suite-dashboard-subtitle">Prioritäten, Materialfluss und Einsatzbereitschaft auf einen Blick.</p>
        </div>
        <div className="suite-dashboard-actions">
          <span className="suite-dashboard-timestamp">{lastUpdated ? `Aktualisiert ${lastUpdated.toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })}` : ''}</span>
          <button type="button" onClick={() => void loadData(true)} disabled={refreshing} className="suite-button"><RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />Aktualisieren</button>
        </div>
      </header>

      <section className="suite-kpi-grid">
        <KpiCard icon={PackageCheck} label="Einsatzbereit im Lager" value={stats.ready_for_dispatch} detail={`${readiness}% aller Geräte`} color="var(--color-success)" progress={readiness} onClick={() => navigate('/scan')} />
		<KpiCard icon={BriefcaseBusiness} label="Bestätigte Jobs" value={stats.active_jobs} detail={`${stats.on_job} Geräte · ${stats.cases_on_job} Cases draußen`} color="var(--color-info)" onClick={() => navigate('/jobs')} />
        <KpiCard icon={MapPinOff} label="Nicht zugeordnet" value={unassigned} detail={`${overview?.unplaced_product_quantity || 0} Mengeneinheiten zusätzlich`} color={unassigned ? 'var(--color-error)' : 'var(--color-success)'} onClick={() => navigate('/zones')} />
        <KpiCard icon={ClipboardCheck} label="Offene Lagerarbeit" value={(overview?.open_tasks || 0) + (overview?.counts_due || 0)} detail={`${overview?.open_tasks || 0} Aufgaben · ${overview?.counts_due || 0} Inventuren`} color={(overview?.open_tasks || overview?.counts_due) ? 'var(--color-warning)' : 'var(--text-secondary)'} onClick={() => navigate('/zones')} />
      </section>

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.45fr)_minmax(320px,0.75fr)]">
        <div className="card overflow-hidden">
          <div className="flex items-center justify-between border-b px-4 py-4 sm:px-5" style={{ borderColor: 'var(--border-subtle)' }}><div><h2 className="font-bold" style={{ color: 'var(--text-primary)' }}>Jetzt bearbeiten</h2><p className="mt-0.5 text-xs" style={{ color: 'var(--text-secondary)' }}>Priorisiert nach Auswirkung auf den Lagerbetrieb</p></div><ShieldAlert className="h-5 w-5" style={{ color: priorities.length ? 'var(--color-warning)' : 'var(--color-success)' }} /></div>
          {priorities.length === 0 ? <div className="flex min-h-44 items-center justify-center p-6 text-center"><div><CheckCircle2 className="mx-auto mb-2 h-9 w-9" style={{ color: 'var(--color-success)' }} /><div className="font-semibold" style={{ color: 'var(--text-primary)' }}>Keine kritischen Aufgaben</div><div className="mt-1 text-sm" style={{ color: 'var(--text-secondary)' }}>Der Lagerbetrieb ist aktuell im grünen Bereich.</div></div></div> : <div className="divide-y" style={{ borderColor: 'var(--border-subtle)' }}>{priorities.map((item) => <button key={item.title} type="button" onClick={() => navigate(item.path)} className="group flex w-full items-center gap-3 px-4 py-3.5 text-left transition-colors hover:bg-white/[0.04] sm:px-5"><div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg" style={{ background: item.tone === 'critical' ? 'rgba(var(--color-error-rgb),0.12)' : 'rgba(var(--color-warning-rgb),0.1)', color: item.tone === 'critical' ? 'var(--color-error)' : 'var(--color-warning)' }}><AlertTriangle className="h-5 w-5" /></div><div className="min-w-0 flex-1"><div className="font-semibold" style={{ color: 'var(--text-primary)' }}>{item.title}</div><div className="truncate text-xs" style={{ color: 'var(--text-secondary)' }}>{item.detail}</div></div><span className="text-xl font-bold" style={{ color: item.tone === 'critical' ? 'var(--color-error)' : 'var(--color-warning)' }}>{item.value}</span><ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-1" style={{ color: 'var(--text-muted)' }} /></button>)}</div>}
        </div>

        <div className="card p-4 sm:p-5">
          <div className="flex items-center justify-between"><div><h2 className="font-bold" style={{ color: 'var(--text-primary)' }}>Schnellstart</h2><p className="mt-0.5 text-xs" style={{ color: 'var(--text-secondary)' }}>Häufige Lageraktionen</p></div><ScanLine className="h-5 w-5" style={{ color: 'var(--accent-red)' }} /></div>
          <div className="mt-4 grid grid-cols-2 gap-2">
            <QuickAction icon={ScanLine} label="Scannen" primary onClick={() => navigate('/scan')} />
            <QuickAction icon={Boxes} label="Cases" onClick={() => navigate('/cases')} />
            <QuickAction icon={Warehouse} label="Lagerplätze" onClick={() => navigate('/zones')} />
            <QuickAction icon={BriefcaseBusiness} label="Jobs" onClick={() => navigate('/jobs')} />
          </div>
          <div className="mt-4 rounded-lg border p-3" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-subtle)' }}><div className="flex items-center justify-between text-xs" style={{ color: 'var(--text-secondary)' }}><span>Cases im Prozess</span><span className="font-semibold" style={{ color: 'var(--text-primary)' }}>{stats.cases_packing}</span></div><div className="mt-2 flex items-center justify-between text-xs" style={{ color: 'var(--text-secondary)' }}><span>Cases in Rücklaufprüfung</span><span className="font-semibold" style={{ color: stats.cases_return_check ? 'var(--color-warning)' : 'var(--text-primary)' }}>{stats.cases_return_check}</span></div></div>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.25fr)_minmax(320px,0.75fr)]">
        <div className="card p-4 sm:p-5">
          <div className="mb-5 flex items-center justify-between"><div><h2 className="font-bold" style={{ color: 'var(--text-primary)' }}>Materialfluss</h2><p className="mt-0.5 text-xs" style={{ color: 'var(--text-secondary)' }}>Physischer Status aller {stats.total} Geräte</p></div><Activity className="h-5 w-5" style={{ color: 'var(--color-info)' }} /></div>
          <div className="space-y-4">{flow.map((item) => <FlowRow key={item.label} {...item} total={stats.total} onClick={() => navigate(item.path)} />)}</div>
          <div className="mt-5 flex flex-wrap gap-2 border-t pt-4" style={{ borderColor: 'var(--border-subtle)' }}>
            <Condition label="Einsatzbereit" value={stats.available} color="var(--color-success)" />
            <Condition label="Gesperrt" value={stats.blocked} color="var(--color-warning)" />
            <Condition label="Defekt" value={stats.defective} color="var(--color-error)" />
            <Condition label="Wartung" value={stats.maintenance} color="var(--color-warning)" />
            <Condition label="Ausgemustert" value={stats.retired} color="var(--text-muted)" />
          </div>
        </div>

        <div className="card p-4 sm:p-5">
          <div className="mb-4 flex items-center justify-between"><div><h2 className="font-bold" style={{ color: 'var(--text-primary)' }}>Heute bewegt</h2><p className="mt-0.5 text-xs" style={{ color: 'var(--text-secondary)' }}>Seit 00:00 Uhr</p></div><span className="text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>{stats.movements_today}</span></div>
          <div className="grid grid-cols-3 gap-2">
            <DailyFlow icon={ArrowDownToLine} label="Einlagerung" value={stats.intakes_today} color="var(--color-success)" />
            <DailyFlow icon={ArrowUpFromLine} label="Ausgabe" value={stats.outtakes_today} color="var(--color-info)" />
            <DailyFlow icon={MoveRight} label="Umlagerung" value={stats.transfers_today} color="var(--color-info)" />
          </div>
          <button type="button" onClick={() => navigate('/scan')} className="mt-4 flex w-full items-center justify-center gap-2 rounded-lg border px-3 py-2.5 text-sm font-semibold" style={{ borderColor: 'var(--border-default)', color: 'var(--text-primary)' }}><ScanLine className="h-4 w-4" />Scanner öffnen</button>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(340px,0.85fr)]">
        <div className="card overflow-hidden">
          <div className="flex items-center justify-between border-b px-4 py-4 sm:px-5" style={{ borderColor: 'var(--border-subtle)' }}><div><h2 className="font-bold" style={{ color: 'var(--text-primary)' }}>Letzte Bewegungen</h2><p className="mt-0.5 text-xs" style={{ color: 'var(--text-secondary)' }}>Scanner- und Lageraktivität</p></div><Clock3 className="h-5 w-5" style={{ color: 'var(--text-muted)' }} /></div>
          {activity.length === 0 ? <div className="py-12 text-center text-sm" style={{ color: 'var(--text-secondary)' }}>Noch keine Bewegungen erfasst.</div> : <div className="divide-y" style={{ borderColor: 'var(--border-subtle)' }}>{activity.slice(0, 8).map((item) => <div key={item.movement_id} className="flex items-center gap-3 px-4 py-3 sm:px-5"><span className="h-2 w-2 shrink-0 rounded-full" style={{ background: item.action === 'intake' ? 'var(--color-success)' : item.action === 'outtake' ? 'var(--accent-red)' : 'var(--color-info)' }} /><div className="min-w-0 flex-1"><div className="truncate text-sm font-medium" style={{ color: 'var(--text-primary)' }}>{movementText(item)}</div><div className="mt-0.5 text-xs" style={{ color: 'var(--text-muted)' }}>{relativeTime(item.timestamp)}{item.performed_by ? ` · ${item.performed_by}` : ''}</div></div><span className="hidden font-mono text-[10px] sm:block" style={{ color: 'var(--text-muted)' }}>{item.device_id}</span></div>)}</div>}
        </div>
        <LowStockAlertsWidget />
      </section>

      {(stats.unavailable > 0 || stats.open_defects > 0) && <button type="button" onClick={() => navigate('/maintenance')} className="flex w-full items-center gap-3 rounded-xl border p-4 text-left" style={{ borderColor: 'rgba(var(--color-warning-rgb),0.35)', background: 'rgba(var(--color-warning-rgb),0.06)' }}><Wrench className="h-5 w-5" style={{ color: 'var(--color-warning)' }} /><div className="flex-1"><div className="font-semibold" style={{ color: 'var(--text-primary)' }}>Technische Verfügbarkeit prüfen</div><div className="text-xs" style={{ color: 'var(--text-secondary)' }}>{stats.unavailable} Geräte nicht einsatzbereit · {stats.open_defects} offene Defektmeldungen</div></div><ArrowRight className="h-4 w-4" style={{ color: 'var(--text-muted)' }} /></button>}
    </div>
  );
}

function KpiCard({ icon: Icon, label, value, detail, color, progress, onClick }: { icon: typeof Warehouse; label: string; value: number; detail: string; color: string; progress?: number; onClick: () => void }) {
  return <button type="button" onClick={onClick} className="card group p-4 text-left transition-all hover:-translate-y-0.5 hover:border-white/15 sm:p-5"><div className="flex items-start justify-between"><div className="flex h-10 w-10 items-center justify-center rounded-lg" style={{ background: `color-mix(in srgb, ${color} 12%, transparent)`, color }}><Icon className="h-5 w-5" /></div><ArrowRight className="h-4 w-4 opacity-0 transition-all group-hover:translate-x-1 group-hover:opacity-100" style={{ color: 'var(--text-muted)' }} /></div><div className="mt-4 text-3xl font-bold leading-none" style={{ color }}>{value}</div><div className="mt-2 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--text-secondary)' }}>{label}</div><div className="mt-1 text-xs" style={{ color: 'var(--text-muted)' }}>{detail}</div>{progress !== undefined && <div className="mt-3 h-1.5 overflow-hidden rounded-full" style={{ background: 'var(--bg-hover)' }}><div className="h-full rounded-full" style={{ width: `${Math.min(100, progress)}%`, background: color }} /></div>}</button>;
}

function QuickAction({ icon: Icon, label, onClick, primary = false }: { icon: typeof ScanLine; label: string; onClick: () => void; primary?: boolean }) {
  return <button type="button" onClick={onClick} className="flex min-h-20 flex-col items-center justify-center gap-2 rounded-lg border text-sm font-semibold transition-transform hover:-translate-y-0.5" style={{ borderColor: primary ? 'var(--accent-red)' : 'var(--border-subtle)', background: primary ? 'var(--accent-red)' : 'var(--bg-subtle)', color: 'var(--text-primary)' }}><Icon className="h-5 w-5" />{label}</button>;
}

function FlowRow({ label, value, total, color, onClick }: { label: string; value: number; total: number; color: string; path: string; onClick: () => void }) {
  const percent = total ? Math.round((value / total) * 100) : 0;
  return <button type="button" onClick={onClick} className="w-full text-left"><div className="mb-1.5 flex items-center justify-between text-sm"><span style={{ color: 'var(--text-secondary)' }}>{label}</span><span className="font-semibold" style={{ color: 'var(--text-primary)' }}>{value} <span className="font-normal" style={{ color: 'var(--text-muted)' }}>· {percent}%</span></span></div><div className="h-2 overflow-hidden rounded-full" style={{ background: 'var(--bg-hover)' }}><div className="h-full rounded-full transition-all" style={{ width: `${percent}%`, background: color }} /></div></button>;
}

function Condition({ label, value, color }: { label: string; value: number; color: string }) {
  return <span className="rounded-full border px-2.5 py-1 text-xs" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-subtle)', color }}><span className="font-bold">{value}</span> {label}</span>;
}

function DailyFlow({ icon: Icon, label, value, color }: { icon: typeof ArrowDownToLine; label: string; value: number; color: string }) {
  return <div className="rounded-lg border p-3 text-center" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-subtle)' }}><Icon className="mx-auto h-4 w-4" style={{ color }} /><div className="mt-2 text-xl font-bold" style={{ color: 'var(--text-primary)' }}>{value}</div><div className="mt-1 truncate text-[10px] uppercase tracking-wide" style={{ color: 'var(--text-muted)' }}>{label}</div></div>;
}

function DashboardSkeleton() {
  return <div className="space-y-5 animate-pulse"><div className="h-20 rounded-xl" style={{ background: 'var(--bg-subtle)' }} /><div className="grid grid-cols-2 gap-3 xl:grid-cols-4">{Array.from({ length: 4 }).map((_, index) => <div key={index} className="h-40 rounded-xl" style={{ background: 'var(--bg-card)' }} />)}</div><div className="grid gap-4 xl:grid-cols-2"><div className="h-72 rounded-xl" style={{ background: 'var(--bg-card)' }} /><div className="h-72 rounded-xl" style={{ background: 'var(--bg-card)' }} /></div></div>;
}
