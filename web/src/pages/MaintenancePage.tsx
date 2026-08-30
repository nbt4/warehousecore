import {
  useCallback,
  useDeferredValue,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import {
  AlertTriangle,
  CalendarDays,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  ClipboardList,
  Clock,
  Euro,
  History,
  PackageSearch,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  UserRound,
  Wrench,
  type LucideIcon,
} from "lucide-react";
import {
  maintenanceApi,
  type MaintenanceDeviceOption,
  type MaintenanceEvent,
  type MaintenanceOrder,
  type MaintenanceOrderInput,
  type MaintenanceOrderStatus,
  type MaintenanceOrderType,
  type MaintenanceOptions,
  type MaintenanceOutcome,
  type MaintenancePlan,
  type MaintenancePlanInput,
  type MaintenancePriority,
  type MaintenanceOverview,
} from "../lib/api";
import { toast } from "../lib/toast";

type View = "worklist" | "plans" | "history";

const typeLabels: Record<MaintenanceOrderType, string> = {
  defect: "Defekt",
  preventive: "Wartung",
  inspection: "Prüfung",
  calibration: "Kalibrierung",
};
const priorityLabels: Record<MaintenancePriority, string> = {
  low: "Niedrig",
  normal: "Normal",
  high: "Hoch",
  critical: "Kritisch",
};
const statusLabels: Record<MaintenanceOrderStatus, string> = {
  open: "Offen",
  planned: "Geplant",
  in_progress: "In Arbeit",
  waiting_parts: "Wartet auf Teile",
  completed: "Abgeschlossen",
  cancelled: "Abgebrochen",
};
const outcomeLabels: Record<MaintenanceOutcome, string> = {
  passed: "Bestanden",
  passed_with_notes: "Bestanden mit Hinweis",
  failed: "Nicht bestanden",
  repaired: "Repariert",
};

const emptyOptions: MaintenanceOptions = { devices: [], users: [] };
const emptyOverview: MaintenanceOverview = {
  overdue_orders: 0,
  due_soon_orders: 0,
  open_defects: 0,
  in_progress_orders: 0,
  completed_this_month: 0,
  active_plans: 0,
  unavailable_devices: 0,
  cost_this_month: 0,
};

function today() {
  return new Date().toISOString().slice(0, 10);
}
function inDays(days: number) {
  const value = new Date();
  value.setDate(value.getDate() + days);
  return value.toISOString().slice(0, 10);
}
function formatDate(value?: string) {
  return value
    ? new Intl.DateTimeFormat("de-DE", { dateStyle: "medium" }).format(
        new Date(value),
      )
    : "Nicht terminiert";
}
function formatDateTime(value?: string) {
  return value
    ? new Intl.DateTimeFormat("de-DE", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date(value))
    : "–";
}
function formatCurrency(value?: number) {
  return new Intl.NumberFormat("de-DE", {
    style: "currency",
    currency: "EUR",
  }).format(value || 0);
}
function errorMessage(error: unknown, fallback: string) {
  if (typeof error === "object" && error && "response" in error) {
    const response = (error as { response?: { data?: { error?: string } } })
      .response;
    if (response?.data?.error) return response.data.error;
  }
  return fallback;
}
function emptyOrderForm(): MaintenanceOrderInput {
  return {
    device_id: "",
    order_type: "defect",
    priority: "normal",
    title: "",
    description: "",
    due_at: today(),
  };
}
function emptyPlanForm(): MaintenancePlanInput {
  return {
    device_id: "",
    name: "",
    maintenance_type: "preventive",
    interval_days: 365,
    lead_time_days: 14,
    instructions: "",
    next_due_at: inDays(30),
    is_active: true,
  };
}

export function MaintenancePage() {
  const [view, setView] = useState<View>("worklist");
  const [overview, setOverview] = useState(emptyOverview);
  const [options, setOptions] = useState(emptyOptions);
  const [orders, setOrders] = useState<MaintenanceOrder[]>([]);
  const [plans, setPlans] = useState<MaintenancePlan[]>([]);
  const [loading, setLoading] = useState(true);
  const [sectionLoading, setSectionLoading] = useState(false);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const deferredSearch = useDeferredValue(search);
  const [typeFilter, setTypeFilter] = useState<MaintenanceOrderType | "all">(
    "all",
  );
  const [orderForm, setOrderForm] =
    useState<MaintenanceOrderInput>(emptyOrderForm());
  const [editingOrderId, setEditingOrderId] = useState<number | null>(null);
  const [orderFormOpen, setOrderFormOpen] = useState(false);
  const [planForm, setPlanForm] =
    useState<MaintenancePlanInput>(emptyPlanForm());
  const [editingPlanId, setEditingPlanId] = useState<number | null>(null);
  const [planFormOpen, setPlanFormOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [completionOrder, setCompletionOrder] =
    useState<MaintenanceOrder | null>(null);
  const [completion, setCompletion] = useState<{
    outcome: MaintenanceOutcome;
    resolution: string;
    cost: string;
    next_due_at: string;
  }>({ outcome: "repaired", resolution: "", cost: "", next_due_at: "" });
  const [expandedOrder, setExpandedOrder] = useState<number | null>(null);
  const [events, setEvents] = useState<MaintenanceEvent[]>([]);
  const [eventsLoading, setEventsLoading] = useState(false);

  const loadBase = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [overviewResult, optionsResult, planResult] = await Promise.all([
        maintenanceApi.overview(),
        maintenanceApi.options(),
        maintenanceApi.plans(),
      ]);
      setOverview(overviewResult.data);
      setOptions(optionsResult.data);
      setPlans(planResult.data);
    } catch (loadError) {
      setError(
        errorMessage(loadError, "Wartungsdaten konnten nicht geladen werden."),
      );
    } finally {
      setLoading(false);
    }
  }, []);

  const loadOrders = useCallback(async () => {
    setSectionLoading(true);
    try {
      const result = await maintenanceApi.orders({
        scope: view === "history" ? "history" : "active",
        type: typeFilter === "all" ? undefined : typeFilter,
        search: deferredSearch.trim() || undefined,
      });
      setOrders(result.data);
    } catch (loadError) {
      setError(
        errorMessage(
          loadError,
          "Wartungsaufträge konnten nicht geladen werden.",
        ),
      );
    } finally {
      setSectionLoading(false);
    }
  }, [view, typeFilter, deferredSearch]);

  useEffect(() => {
    void loadBase();
  }, [loadBase]);
  useEffect(() => {
    if (view !== "plans") void loadOrders();
  }, [view, loadOrders]);

  const refresh = async () => {
    await Promise.all([
      loadBase(),
      view !== "plans" ? loadOrders() : Promise.resolve(),
    ]);
  };
  const deviceById = useMemo(
    () => new Map(options.devices.map((device) => [device.device_id, device])),
    [options.devices],
  );

  const openCreateOrder = () => {
    setEditingOrderId(null);
    setOrderForm(emptyOrderForm());
    setOrderFormOpen(true);
  };
  const openEditOrder = (order: MaintenanceOrder) => {
    setEditingOrderId(order.order_id);
    setOrderForm({
      device_id: order.device_id,
      order_type: order.order_type,
      priority: order.priority,
      title: order.title,
      description: order.description || "",
      due_at: order.due_at || "",
      assigned_to: order.assigned_to,
      cost: order.cost,
    });
    setOrderFormOpen(true);
  };
  const submitOrder = async (event: FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      if (editingOrderId)
        await maintenanceApi.updateOrder(editingOrderId, orderForm);
      else await maintenanceApi.createOrder(orderForm);
      toast.success(
        editingOrderId
          ? "Wartungsauftrag aktualisiert"
          : "Wartungsauftrag erstellt",
      );
      setOrderFormOpen(false);
      setEditingOrderId(null);
      await refresh();
    } catch (saveError) {
      toast.error(
        errorMessage(
          saveError,
          "Wartungsauftrag konnte nicht gespeichert werden.",
        ),
      );
    } finally {
      setSaving(false);
    }
  };

  const transition = async (
    order: MaintenanceOrder,
    status: MaintenanceOrderStatus,
    notes?: string,
  ) => {
    if (
      status === "cancelled" &&
      !confirm(`Auftrag „${order.title}“ wirklich abbrechen?`)
    )
      return;
    setSaving(true);
    try {
      await maintenanceApi.transitionOrder(order.order_id, { status, notes });
      toast.success(`Status auf „${statusLabels[status]}“ gesetzt`);
      await refresh();
    } catch (transitionError) {
      toast.error(
        errorMessage(transitionError, "Status konnte nicht geändert werden."),
      );
    } finally {
      setSaving(false);
    }
  };

  const openCompletion = (order: MaintenanceOrder) => {
    setCompletionOrder(order);
    setCompletion({
      outcome: order.order_type === "defect" ? "repaired" : "passed",
      resolution: "",
      cost: order.cost === undefined ? "" : String(order.cost),
      next_due_at: "",
    });
  };
  const complete = async (event: FormEvent) => {
    event.preventDefault();
    if (!completionOrder) return;
    setSaving(true);
    try {
      await maintenanceApi.transitionOrder(completionOrder.order_id, {
        status: "completed",
        outcome: completion.outcome,
        resolution: completion.resolution,
        cost: completion.cost.trim() ? Number(completion.cost) : undefined,
        next_due_at: completion.next_due_at || undefined,
      });
      toast.success("Wartungsauftrag abgeschlossen");
      setCompletionOrder(null);
      await refresh();
    } catch (completionError) {
      toast.error(
        errorMessage(
          completionError,
          "Wartungsauftrag konnte nicht abgeschlossen werden.",
        ),
      );
    } finally {
      setSaving(false);
    }
  };

  const toggleEvents = async (orderID: number) => {
    if (expandedOrder === orderID) {
      setExpandedOrder(null);
      return;
    }
    setExpandedOrder(orderID);
    setEventsLoading(true);
    try {
      const result = await maintenanceApi.events(orderID);
      setEvents(result.data);
    } catch (eventError) {
      toast.error(
        errorMessage(eventError, "Historie konnte nicht geladen werden."),
      );
    } finally {
      setEventsLoading(false);
    }
  };

  const openCreatePlan = () => {
    setEditingPlanId(null);
    setPlanForm(emptyPlanForm());
    setPlanFormOpen(true);
  };
  const openEditPlan = (plan: MaintenancePlan) => {
    setEditingPlanId(plan.plan_id);
    setPlanForm({
      device_id: plan.device_id,
      name: plan.name,
      maintenance_type: plan.maintenance_type,
      interval_days: plan.interval_days,
      lead_time_days: plan.lead_time_days,
      instructions: plan.instructions || "",
      next_due_at: plan.next_due_at,
      is_active: plan.is_active,
    });
    setPlanFormOpen(true);
  };
  const submitPlan = async (event: FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      if (editingPlanId)
        await maintenanceApi.updatePlan(editingPlanId, planForm);
      else await maintenanceApi.createPlan(planForm);
      toast.success(
        editingPlanId ? "Wartungsplan aktualisiert" : "Wartungsplan erstellt",
      );
      setPlanFormOpen(false);
      setEditingPlanId(null);
      await refresh();
    } catch (saveError) {
      toast.error(
        errorMessage(
          saveError,
          "Wartungsplan konnte nicht gespeichert werden.",
        ),
      );
    } finally {
      setSaving(false);
    }
  };
  const togglePlan = async (plan: MaintenancePlan) => {
    setSaving(true);
    try {
      await maintenanceApi.updatePlan(plan.plan_id, {
        device_id: plan.device_id,
        name: plan.name,
        maintenance_type: plan.maintenance_type,
        interval_days: plan.interval_days,
        lead_time_days: plan.lead_time_days,
        instructions: plan.instructions || "",
        next_due_at: plan.next_due_at,
        is_active: !plan.is_active,
      });
      toast.success(
        plan.is_active ? "Wartungsplan pausiert" : "Wartungsplan aktiviert",
      );
      await refresh();
    } catch (saveError) {
      toast.error(
        errorMessage(saveError, "Wartungsplan konnte nicht geändert werden."),
      );
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <MaintenanceSkeleton />;

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div
            className="text-xs font-semibold uppercase tracking-wider"
            style={{ color: "var(--accent-red)" }}
          >
            Technischer Betrieb
          </div>
          <h1
            className="mt-1 text-2xl font-bold sm:text-3xl"
            style={{ color: "var(--text-primary)" }}
          >
            Wartung & Instandhaltung
          </h1>
          <p
            className="mt-1 text-sm"
            style={{ color: "var(--text-secondary)" }}
          >
            Fälligkeiten planen, Defekte bearbeiten und die Einsatzbereitschaft
            nachvollziehbar wiederherstellen.
          </p>
        </div>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => void refresh()}
            className="flex h-11 w-11 items-center justify-center rounded-lg border"
            style={{
              borderColor: "var(--border-default)",
              background: "var(--bg-card)",
              color: "var(--text-secondary)",
            }}
            aria-label="Aktualisieren"
          >
            <RefreshCw className="h-4 w-4" />
          </button>
          <button
            type="button"
            onClick={view === "plans" ? openCreatePlan : openCreateOrder}
            className="flex min-h-11 items-center gap-2 rounded-lg px-4 py-2.5 font-semibold"
            style={{
              background: "var(--accent-red)",
              color: "var(--text-primary)",
            }}
          >
            <Plus className="h-4 w-4" />
            {view === "plans" ? "Neuer Plan" : "Neuer Auftrag"}
          </button>
        </div>
      </header>

      {error && (
        <div
          role="alert"
          className="flex flex-col gap-3 rounded-xl border p-4 sm:flex-row sm:items-center sm:justify-between"
          style={{
            borderColor: "var(--color-error)",
            background: "var(--bg-card)",
            color: "var(--text-primary)",
          }}
        >
          <span>{error}</span>
          <button
            type="button"
            onClick={() => void refresh()}
            className="rounded-lg border px-3 py-2 text-sm font-semibold"
            style={{ borderColor: "var(--border-default)" }}
          >
            Erneut versuchen
          </button>
        </div>
      )}

      <div className="suite-kpi-grid">
        <Metric
          icon={Clock}
          label="Überfällig"
          value={overview.overdue_orders}
          tone={overview.overdue_orders ? "error" : "neutral"}
          detail={`${overview.due_soon_orders} in den nächsten 30 Tagen`}
        />
        <Metric
          icon={AlertTriangle}
          label="Offene Defekte"
          value={overview.open_defects}
          tone={overview.open_defects ? "warning" : "neutral"}
          detail={`${overview.unavailable_devices} Geräte nicht einsatzbereit`}
        />
        <Metric
          icon={Wrench}
          label="In Arbeit"
          value={overview.in_progress_orders}
          tone={overview.in_progress_orders ? "info" : "neutral"}
          detail="inklusive Warten auf Teile"
        />
        <Metric
          icon={CheckCircle2}
          label="Diesen Monat erledigt"
          value={overview.completed_this_month}
          tone="success"
          detail={`${formatCurrency(overview.cost_this_month)} Aufwand`}
        />
      </div>

      <nav
        className="flex w-fit max-w-full gap-1 overflow-x-auto rounded-lg border p-1"
        style={{
          borderColor: "var(--border-default)",
          background: "var(--bg-card)",
        }}
        aria-label="Wartungsbereiche"
      >
        <ViewTab
          active={view === "worklist"}
          icon={ClipboardList}
          onClick={() => setView("worklist")}
        >
          Arbeitsvorrat
        </ViewTab>
        <ViewTab
          active={view === "plans"}
          icon={CalendarDays}
          onClick={() => setView("plans")}
        >
          Wartungspläne ({overview.active_plans})
        </ViewTab>
        <ViewTab
          active={view === "history"}
          icon={History}
          onClick={() => setView("history")}
        >
          Historie
        </ViewTab>
      </nav>

      {orderFormOpen && (
        <OrderEditor
          form={orderForm}
          setForm={setOrderForm}
          options={options}
          editing={Boolean(editingOrderId)}
          saving={saving}
          onSubmit={submitOrder}
          onClose={() => setOrderFormOpen(false)}
        />
      )}
      {planFormOpen && (
        <PlanEditor
          form={planForm}
          setForm={setPlanForm}
          options={options}
          editing={Boolean(editingPlanId)}
          saving={saving}
          onSubmit={submitPlan}
          onClose={() => setPlanFormOpen(false)}
        />
      )}
      {completionOrder && (
        <CompletionEditor
          order={completionOrder}
          value={completion}
          setValue={setCompletion}
          saving={saving}
          onSubmit={complete}
          onClose={() => setCompletionOrder(null)}
        />
      )}

      {view === "plans" ? (
        <PlansView
          plans={plans}
          saving={saving}
          onEdit={openEditPlan}
          onToggle={togglePlan}
        />
      ) : (
        <OrdersView
          view={view}
          orders={orders}
          sectionLoading={sectionLoading}
          search={search}
          setSearch={setSearch}
          typeFilter={typeFilter}
          setTypeFilter={setTypeFilter}
          expandedOrder={expandedOrder}
          events={events}
          eventsLoading={eventsLoading}
          onToggleEvents={toggleEvents}
          onEdit={openEditOrder}
          onTransition={transition}
          onComplete={openCompletion}
          deviceById={deviceById}
          saving={saving}
          onCreate={openCreateOrder}
        />
      )}
    </div>
  );
}

function Metric({
  icon: Icon,
  label,
  value,
  detail,
  tone,
}: {
  icon: LucideIcon;
  label: string;
  value: number;
  detail: string;
  tone: "neutral" | "error" | "warning" | "info" | "success";
}) {
  const color =
    tone === "error"
      ? "var(--color-error)"
      : tone === "warning"
        ? "var(--color-warning)"
        : tone === "info"
          ? "var(--color-info)"
          : tone === "success"
            ? "var(--color-success)"
            : "var(--text-secondary)";
  return (
    <div className="suite-kpi-card">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="suite-kpi-label">{label}</div>
          <div className="suite-kpi-value" style={{ color }}>
            {value}
          </div>
        </div>
        <Icon className="h-5 w-5" style={{ color }} />
      </div>
      <div className="mt-2 text-xs" style={{ color: "var(--text-secondary)" }}>
        {detail}
      </div>
    </div>
  );
}

function ViewTab({
  active,
  icon: Icon,
  onClick,
  children,
}: {
  active: boolean;
  icon: LucideIcon;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex min-h-10 items-center gap-2 whitespace-nowrap rounded-md px-3 text-sm font-semibold"
      style={{
        background: active ? "var(--accent-red)" : "transparent",
        color: active ? "var(--text-primary)" : "var(--text-secondary)",
      }}
    >
      <Icon className="h-4 w-4" />
      {children}
    </button>
  );
}

type OrdersViewProps = {
  view: View;
  orders: MaintenanceOrder[];
  sectionLoading: boolean;
  search: string;
  setSearch: (value: string) => void;
  typeFilter: MaintenanceOrderType | "all";
  setTypeFilter: (value: MaintenanceOrderType | "all") => void;
  expandedOrder: number | null;
  events: MaintenanceEvent[];
  eventsLoading: boolean;
  onToggleEvents: (id: number) => void;
  onEdit: (order: MaintenanceOrder) => void;
  onTransition: (
    order: MaintenanceOrder,
    status: MaintenanceOrderStatus,
    notes?: string,
  ) => void;
  onComplete: (order: MaintenanceOrder) => void;
  deviceById: Map<string, MaintenanceDeviceOption>;
  saving: boolean;
  onCreate: () => void;
};

function OrdersView({
  view,
  orders,
  sectionLoading,
  search,
  setSearch,
  typeFilter,
  setTypeFilter,
  expandedOrder,
  events,
  eventsLoading,
  onToggleEvents,
  onEdit,
  onTransition,
  onComplete,
  deviceById,
  saving,
  onCreate,
}: OrdersViewProps) {
  const selectOptionStyle = {
    background: "var(--surface-2)",
    color: "var(--text-primary)",
  };
  return (
    <section
      className="rounded-xl border"
      style={{
        borderColor: "var(--border-default)",
        background: "var(--bg-card)",
      }}
    >
      <div
        className="flex flex-col gap-3 border-b p-4 sm:flex-row sm:items-center sm:justify-between"
        style={{ borderColor: "var(--border-subtle)" }}
      >
        <div>
          <h2
            className="text-lg font-bold"
            style={{ color: "var(--text-primary)" }}
          >
            {view === "history"
              ? "Abgeschlossene Arbeiten"
              : "Jetzt bearbeiten"}
          </h2>
          <p className="text-xs" style={{ color: "var(--text-secondary)" }}>
            {view === "history"
              ? "Nachweis aller abgeschlossenen und abgebrochenen Aufträge."
              : "Nach Fälligkeit und Priorität sortierter Arbeitsvorrat."}
          </p>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="relative">
            <Search
              className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
              style={{ color: "var(--text-muted)" }}
            />
            <input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Gerät, Produkt oder Auftrag"
              className="min-h-11 w-full rounded-lg border py-2 pl-9 pr-3 text-sm sm:w-64"
              style={{
                borderColor: "var(--border-input)",
                background: "var(--surface-2)",
                color: "var(--text-primary)",
              }}
            />
          </div>
          <select
            value={typeFilter}
            onChange={(event) =>
              setTypeFilter(event.target.value as MaintenanceOrderType | "all")
            }
            className="min-h-11 rounded-lg border px-3 text-sm"
            style={{
              borderColor: "var(--border-input)",
              background: "var(--surface-2)",
              color: "var(--text-primary)",
            }}
          >
            <option value="all" style={selectOptionStyle}>
              Alle Arten
            </option>
            {Object.entries(typeLabels).map(([value, label]) => (
              <option key={value} value={value} style={selectOptionStyle}>
                {label}
              </option>
            ))}
          </select>
        </div>
      </div>
      <div className="divide-y" style={{ borderColor: "var(--border-subtle)" }}>
        {sectionLoading && (
          <div className="flex min-h-40 items-center justify-center">
            <RefreshCw
              className="h-6 w-6 animate-spin"
              style={{ color: "var(--accent-red)" }}
            />
          </div>
        )}
        {!sectionLoading &&
          orders.map((order) => (
            <OrderRow
              key={order.order_id}
              order={order}
              device={deviceById.get(order.device_id)}
              expanded={expandedOrder === order.order_id}
              events={events}
              eventsLoading={eventsLoading}
              onToggleEvents={onToggleEvents}
              onEdit={onEdit}
              onTransition={onTransition}
              onComplete={onComplete}
              saving={saving}
            />
          ))}
        {!sectionLoading && orders.length === 0 && (
          <div className="px-4 py-14 text-center">
            <ShieldCheck
              className="mx-auto h-9 w-9"
              style={{ color: "var(--text-muted)" }}
            />
            <div
              className="mt-3 font-semibold"
              style={{ color: "var(--text-primary)" }}
            >
              {view === "history"
                ? "Noch keine Wartungshistorie"
                : "Kein offener Arbeitsvorrat"}
            </div>
            <p
              className="mt-1 text-sm"
              style={{ color: "var(--text-secondary)" }}
            >
              {view === "history"
                ? "Abgeschlossene Aufträge erscheinen automatisch hier."
                : "Alle bekannten Wartungen sind erledigt oder noch nicht fällig."}
            </p>
            {view !== "history" && (
              <button
                type="button"
                onClick={onCreate}
                className="mt-4 rounded-lg border px-4 py-2 text-sm font-semibold"
                style={{
                  borderColor: "var(--border-default)",
                  color: "var(--text-primary)",
                }}
              >
                Auftrag anlegen
              </button>
            )}
          </div>
        )}
      </div>
    </section>
  );
}

type OrderRowProps = {
  order: MaintenanceOrder;
  device?: MaintenanceDeviceOption;
  expanded: boolean;
  events: MaintenanceEvent[];
  eventsLoading: boolean;
  onToggleEvents: (id: number) => void;
  onEdit: (order: MaintenanceOrder) => void;
  onTransition: (
    order: MaintenanceOrder,
    status: MaintenanceOrderStatus,
    notes?: string,
  ) => void;
  onComplete: (order: MaintenanceOrder) => void;
  saving: boolean;
};
function OrderRow({
  order,
  device,
  expanded,
  events,
  eventsLoading,
  onToggleEvents,
  onEdit,
  onTransition,
  onComplete,
  saving,
}: OrderRowProps) {
  const overdue = Boolean(
    order.due_at &&
      new Date(`${order.due_at}T23:59:59`) < new Date() &&
      !["completed", "cancelled"].includes(order.status),
  );
  return (
    <article className="p-4 sm:p-5">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span
              className="font-mono text-xs"
              style={{ color: "var(--text-muted)" }}
            >
              WA-{String(order.order_id).padStart(6, "0")}
            </span>
            <StatusBadge status={order.status} />
            <PriorityBadge priority={order.priority} />
            <span
              className="rounded-full border px-2 py-0.5 text-xs"
              style={{
                borderColor: "var(--border-subtle)",
                color: "var(--text-secondary)",
              }}
            >
              {typeLabels[order.order_type]}
            </span>
            {overdue && (
              <span
                className="rounded-full px-2 py-0.5 text-xs font-semibold"
                style={{
                  background:
                    "color-mix(in srgb, var(--color-error) 16%, transparent)",
                  color: "var(--color-error)",
                }}
              >
                Überfällig
              </span>
            )}
          </div>
          <h3
            className="mt-2 text-base font-bold"
            style={{ color: "var(--text-primary)" }}
          >
            {order.title}
          </h3>
          <p
            className="mt-1 line-clamp-2 text-sm"
            style={{ color: "var(--text-secondary)" }}
          >
            {order.description || "Ohne zusätzliche Beschreibung"}
          </p>
          <div
            className="mt-3 grid gap-2 text-xs sm:grid-cols-2 lg:grid-cols-4"
            style={{ color: "var(--text-secondary)" }}
          >
            <Meta
              icon={PackageSearch}
              label={`${order.product_name || "Unbekanntes Produkt"} · ${order.device_id}`}
            />
            <Meta
              icon={CalendarDays}
              label={formatDate(order.due_at)}
              tone={overdue ? "error" : undefined}
            />
            <Meta
              icon={UserRound}
              label={order.assigned_to_name || "Nicht zugewiesen"}
            />
            <Meta
              icon={Euro}
              label={
                order.cost === undefined
                  ? "Kosten offen"
                  : formatCurrency(order.cost)
              }
            />
          </div>
          {device?.serial_number && (
            <div
              className="mt-2 font-mono text-[11px]"
              style={{ color: "var(--text-muted)" }}
            >
              Seriennummer {device.serial_number}
              {order.zone_name ? ` · ${order.zone_name}` : ""}
            </div>
          )}
        </div>
        <div className="flex flex-wrap gap-2 xl:max-w-sm xl:justify-end">
          {(order.status === "open" || order.status === "planned") && (
            <>
              <button
                disabled={saving}
                type="button"
                onClick={() => onTransition(order, "in_progress")}
                className="flex min-h-10 items-center gap-2 rounded-lg px-3 text-sm font-semibold disabled:opacity-50"
                style={{
                  background: "var(--accent-red)",
                  color: "var(--text-primary)",
                }}
              >
                <Play className="h-4 w-4" />
                Starten
              </button>
              <button
                disabled={saving}
                type="button"
                onClick={() => onEdit(order)}
                className="min-h-10 rounded-lg border px-3 text-sm font-semibold disabled:opacity-50"
                style={{
                  borderColor: "var(--border-default)",
                  color: "var(--text-secondary)",
                }}
                aria-label="Auftrag bearbeiten"
              >
                <Pencil className="h-4 w-4" />
              </button>
            </>
          )}
          {order.status === "in_progress" && (
            <>
              <button
                disabled={saving}
                type="button"
                onClick={() => onComplete(order)}
                className="min-h-10 rounded-lg px-3 text-sm font-semibold disabled:opacity-50"
                style={{
                  background: "var(--color-success)",
                  color: "var(--bg-primary)",
                }}
              >
                Abschließen
              </button>
              <button
                disabled={saving}
                type="button"
                onClick={() =>
                  onTransition(
                    order,
                    "waiting_parts",
                    "Ersatzteile oder externe Leistung ausstehend",
                  )
                }
                className="min-h-10 rounded-lg border px-3 text-sm font-semibold disabled:opacity-50"
                style={{
                  borderColor: "var(--border-default)",
                  color: "var(--text-secondary)",
                }}
              >
                Wartet auf Teile
              </button>
            </>
          )}
          {order.status === "waiting_parts" && (
            <>
              <button
                disabled={saving}
                type="button"
                onClick={() => onTransition(order, "in_progress")}
                className="min-h-10 rounded-lg px-3 text-sm font-semibold disabled:opacity-50"
                style={{
                  background: "var(--accent-red)",
                  color: "var(--text-primary)",
                }}
              >
                Fortsetzen
              </button>
              <button
                disabled={saving}
                type="button"
                onClick={() => onComplete(order)}
                className="min-h-10 rounded-lg border px-3 text-sm font-semibold disabled:opacity-50"
                style={{
                  borderColor: "var(--color-success)",
                  color: "var(--color-success)",
                }}
              >
                Abschließen
              </button>
            </>
          )}
          {!["completed", "cancelled"].includes(order.status) && (
            <button
              disabled={saving}
              type="button"
              onClick={() => onTransition(order, "cancelled")}
              className="min-h-10 rounded-lg border px-3 text-sm disabled:opacity-50"
              style={{
                borderColor: "var(--border-default)",
                color: "var(--text-muted)",
              }}
            >
              Abbrechen
            </button>
          )}
          <button
            type="button"
            onClick={() => void onToggleEvents(order.order_id)}
            className="flex min-h-10 items-center gap-2 rounded-lg border px-3 text-sm"
            style={{
              borderColor: "var(--border-default)",
              color: "var(--text-secondary)",
            }}
          >
            Verlauf{" "}
            {expanded ? (
              <ChevronUp className="h-4 w-4" />
            ) : (
              <ChevronDown className="h-4 w-4" />
            )}
          </button>
        </div>
      </div>
      {expanded && <EventTimeline events={events} loading={eventsLoading} />}
    </article>
  );
}

function StatusBadge({ status }: { status: MaintenanceOrderStatus }) {
  const color =
    status === "completed"
      ? "var(--color-success)"
      : status === "cancelled"
        ? "var(--text-muted)"
        : status === "in_progress"
          ? "var(--color-info)"
          : status === "waiting_parts"
            ? "var(--color-warning)"
            : "var(--text-secondary)";
  return (
    <span
      className="rounded-full px-2 py-0.5 text-xs font-semibold"
      style={{
        background: `color-mix(in srgb, ${color} 14%, transparent)`,
        color,
      }}
    >
      {statusLabels[status]}
    </span>
  );
}
function PriorityBadge({ priority }: { priority: MaintenancePriority }) {
  const color =
    priority === "critical"
      ? "var(--color-error)"
      : priority === "high"
        ? "var(--color-warning)"
        : "var(--text-secondary)";
  return (
    <span
      className="rounded-full px-2 py-0.5 text-xs font-semibold"
      style={{
        background: `color-mix(in srgb, ${color} 12%, transparent)`,
        color,
      }}
    >
      {priorityLabels[priority]}
    </span>
  );
}
function Meta({
  icon: Icon,
  label,
  tone,
}: {
  icon: LucideIcon;
  label: string;
  tone?: "error";
}) {
  return (
    <div className="flex min-w-0 items-center gap-2">
      <Icon
        className="h-3.5 w-3.5 flex-none"
        style={{
          color: tone === "error" ? "var(--color-error)" : "var(--text-muted)",
        }}
      />
      <span
        className="truncate"
        style={{ color: tone === "error" ? "var(--color-error)" : undefined }}
      >
        {label}
      </span>
    </div>
  );
}

function EventTimeline({
  events,
  loading,
}: {
  events: MaintenanceEvent[];
  loading: boolean;
}) {
  return (
    <div
      className="mt-4 border-t pt-4"
      style={{ borderColor: "var(--border-subtle)" }}
    >
      <h4
        className="text-xs font-semibold uppercase tracking-wider"
        style={{ color: "var(--text-muted)" }}
      >
        Auftragsverlauf
      </h4>
      {loading ? (
        <RefreshCw
          className="mt-3 h-4 w-4 animate-spin"
          style={{ color: "var(--accent-red)" }}
        />
      ) : (
        <div className="mt-3 space-y-3">
          {events.map((event) => (
            <div
              key={event.event_id}
              className="grid gap-1 text-sm sm:grid-cols-[160px_minmax(0,1fr)]"
            >
              <time className="text-xs" style={{ color: "var(--text-muted)" }}>
                {formatDateTime(event.created_at)}
              </time>
              <div style={{ color: "var(--text-secondary)" }}>
                <span
                  className="font-semibold"
                  style={{ color: "var(--text-primary)" }}
                >
                  {event.actor_name || "System"}
                </span>
                {event.from_status && event.to_status
                  ? ` · ${statusLabels[event.from_status as MaintenanceOrderStatus] || event.from_status} → ${statusLabels[event.to_status as MaintenanceOrderStatus] || event.to_status}`
                  : event.event_type === "updated"
                    ? " · Stammdaten bearbeitet"
                    : event.event_type === "auto_created"
                      ? " · Automatisch aus Wartungsplan angelegt"
                      : " · Auftrag angelegt"}
                {event.notes && (
                  <div className="mt-0.5 text-xs">{event.notes}</div>
                )}
              </div>
            </div>
          ))}
          {events.length === 0 && (
            <div className="text-sm" style={{ color: "var(--text-muted)" }}>
              Noch keine Ereignisse protokolliert.
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function PlansView({
  plans,
  saving,
  onEdit,
  onToggle,
}: {
  plans: MaintenancePlan[];
  saving: boolean;
  onEdit: (plan: MaintenancePlan) => void;
  onToggle: (plan: MaintenancePlan) => void;
}) {
  return (
    <section
      className="rounded-xl border"
      style={{
        borderColor: "var(--border-default)",
        background: "var(--bg-card)",
      }}
    >
      <div
        className="border-b p-4"
        style={{ borderColor: "var(--border-subtle)" }}
      >
        <h2
          className="text-lg font-bold"
          style={{ color: "var(--text-primary)" }}
        >
          Wiederkehrende Wartungspläne
        </h2>
        <p className="text-xs" style={{ color: "var(--text-secondary)" }}>
          WarehouseCore erzeugt im eingestellten Vorlauf automatisch genau einen
          aktiven Arbeitsauftrag.
        </p>
      </div>
      <div className="suite-table-wrap border-0">
        <table className="w-full">
          <thead>
            <tr>
              <th>Plan / Gerät</th>
              <th>Art</th>
              <th>Intervall</th>
              <th>Nächste Fälligkeit</th>
              <th>Status</th>
              <th className="text-right">Aktionen</th>
            </tr>
          </thead>
          <tbody>
            {plans.map((plan) => {
              const overdue =
                plan.is_active &&
                new Date(`${plan.next_due_at}T23:59:59`) < new Date();
              return (
                <tr key={plan.plan_id}>
                  <td>
                    <div
                      className="font-semibold"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {plan.name}
                    </div>
                    <div
                      className="mt-0.5 text-xs"
                      style={{ color: "var(--text-secondary)" }}
                    >
                      {plan.product_name || "Unbekanntes Produkt"} ·{" "}
                      <span className="font-mono">{plan.device_id}</span>
                    </div>
                  </td>
                  <td>{typeLabels[plan.maintenance_type]}</td>
                  <td>
                    {plan.interval_days} Tage
                    <div
                      className="text-xs"
                      style={{ color: "var(--text-muted)" }}
                    >
                      {plan.lead_time_days} Tage Vorlauf
                    </div>
                  </td>
                  <td>
                    <span
                      style={{
                        color: overdue
                          ? "var(--color-error)"
                          : "var(--text-primary)",
                      }}
                    >
                      {formatDate(plan.next_due_at)}
                    </span>
                    {plan.last_completed_at && (
                      <div
                        className="text-xs"
                        style={{ color: "var(--text-muted)" }}
                      >
                        zuletzt {formatDate(plan.last_completed_at)}
                      </div>
                    )}
                  </td>
                  <td>
                    <span
                      className="rounded-full px-2 py-0.5 text-xs font-semibold"
                      style={{
                        color: plan.is_active
                          ? "var(--color-success)"
                          : "var(--text-muted)",
                        background: plan.is_active
                          ? "color-mix(in srgb, var(--color-success) 12%, transparent)"
                          : "var(--bg-subtle)",
                      }}
                    >
                      {plan.is_active ? "Aktiv" : "Pausiert"}
                    </span>
                    {plan.has_active_order && (
                      <div
                        className="mt-1 text-xs"
                        style={{ color: "var(--color-info)" }}
                      >
                        Auftrag erzeugt
                      </div>
                    )}
                  </td>
                  <td>
                    <div className="flex justify-end gap-2">
                      <button
                        disabled={saving}
                        type="button"
                        onClick={() => onEdit(plan)}
                        className="rounded-lg border p-2 disabled:opacity-50"
                        style={{
                          borderColor: "var(--border-default)",
                          color: "var(--text-secondary)",
                        }}
                        aria-label="Plan bearbeiten"
                      >
                        <Pencil className="h-4 w-4" />
                      </button>
                      <button
                        disabled={saving}
                        type="button"
                        onClick={() => void onToggle(plan)}
                        className="rounded-lg border px-3 py-2 text-xs font-semibold disabled:opacity-50"
                        style={{
                          borderColor: "var(--border-default)",
                          color: plan.is_active
                            ? "var(--color-warning)"
                            : "var(--color-success)",
                        }}
                      >
                        {plan.is_active ? "Pausieren" : "Aktivieren"}
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
            {plans.length === 0 && (
              <tr>
                <td
                  colSpan={6}
                  className="py-12 text-center"
                  style={{ color: "var(--text-muted)" }}
                >
                  Noch keine wiederkehrenden Wartungspläne angelegt.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

const inputStyle = {
  borderColor: "var(--border-input)",
  background: "var(--surface-2)",
  color: "var(--text-primary)",
};
const optionStyle = {
  background: "var(--surface-2)",
  color: "var(--text-primary)",
};
function deviceLabel(device: MaintenanceDeviceOption) {
  return `${device.product_name || "Unbekanntes Produkt"} · ${device.device_id}${device.serial_number ? ` · SN ${device.serial_number}` : ""}`;
}

function OrderEditor({
  form,
  setForm,
  options,
  editing,
  saving,
  onSubmit,
  onClose,
}: {
  form: MaintenanceOrderInput;
  setForm: React.Dispatch<React.SetStateAction<MaintenanceOrderInput>>;
  options: MaintenanceOptions;
  editing: boolean;
  saving: boolean;
  onSubmit: (event: FormEvent) => void;
  onClose: () => void;
}) {
  return (
    <form
      onSubmit={onSubmit}
      className="rounded-xl border p-5"
      style={{ borderColor: "var(--accent-red)", background: "var(--bg-card)" }}
    >
      <EditorHeader
        title={
          editing ? "Wartungsauftrag bearbeiten" : "Wartungsauftrag anlegen"
        }
        subtitle="Gerät, Handlungsbedarf, Termin und Verantwortung verbindlich festlegen."
        onClose={onClose}
      />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Field label="Gerät">
          <select
            required
            disabled={editing}
            value={form.device_id}
            onChange={(event) =>
              setForm((value) => ({ ...value, device_id: event.target.value }))
            }
            style={inputStyle}
          >
            <option value="" style={optionStyle}>
              Gerät auswählen
            </option>
            {options.devices.map((device) => (
              <option
                key={device.device_id}
                value={device.device_id}
                style={optionStyle}
              >
                {deviceLabel(device)}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Art">
          <select
            disabled={editing}
            value={form.order_type}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                order_type: event.target.value as MaintenanceOrderType,
              }))
            }
            style={inputStyle}
          >
            {Object.entries(typeLabels).map(([value, label]) => (
              <option key={value} value={value} style={optionStyle}>
                {label}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Priorität">
          <select
            value={form.priority}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                priority: event.target.value as MaintenancePriority,
              }))
            }
            style={inputStyle}
          >
            {Object.entries(priorityLabels).map(([value, label]) => (
              <option key={value} value={value} style={optionStyle}>
                {label}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Fällig am">
          <input
            required
            type="date"
            value={form.due_at}
            onChange={(event) =>
              setForm((value) => ({ ...value, due_at: event.target.value }))
            }
            style={inputStyle}
          />
        </Field>
        <Field label="Verantwortlich">
          <select
            value={form.assigned_to || ""}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                assigned_to: event.target.value
                  ? Number(event.target.value)
                  : undefined,
              }))
            }
            style={inputStyle}
          >
            <option value="" style={optionStyle}>
              Noch nicht zuweisen
            </option>
            {options.users.map((user) => (
              <option
                key={user.user_id}
                value={user.user_id}
                style={optionStyle}
              >
                {user.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Titel" wide>
          <input
            required
            maxLength={200}
            value={form.title}
            onChange={(event) =>
              setForm((value) => ({ ...value, title: event.target.value }))
            }
            placeholder="z. B. Sichtprüfung und Funktionstest"
            style={inputStyle}
          />
        </Field>
        <Field label="Vorkalkulation (€)">
          <input
            type="number"
            min="0"
            step="0.01"
            value={form.cost ?? ""}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                cost: event.target.value
                  ? Number(event.target.value)
                  : undefined,
              }))
            }
            style={inputStyle}
          />
        </Field>
        <Field label="Beschreibung / Arbeitsumfang" full>
          <textarea
            required
            rows={3}
            value={form.description}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                description: event.target.value,
              }))
            }
            style={inputStyle}
          />
        </Field>
      </div>
      <EditorActions
        saving={saving}
        submitLabel={editing ? "Änderungen speichern" : "Auftrag anlegen"}
        onClose={onClose}
      />
    </form>
  );
}

function PlanEditor({
  form,
  setForm,
  options,
  editing,
  saving,
  onSubmit,
  onClose,
}: {
  form: MaintenancePlanInput;
  setForm: React.Dispatch<React.SetStateAction<MaintenancePlanInput>>;
  options: MaintenanceOptions;
  editing: boolean;
  saving: boolean;
  onSubmit: (event: FormEvent) => void;
  onClose: () => void;
}) {
  return (
    <form
      onSubmit={onSubmit}
      className="rounded-xl border p-5"
      style={{ borderColor: "var(--accent-red)", background: "var(--bg-card)" }}
    >
      <EditorHeader
        title={editing ? "Wartungsplan bearbeiten" : "Wartungsplan anlegen"}
        subtitle="Ein aktiver Plan erzeugt vor seiner Fälligkeit automatisch einen Arbeitsauftrag."
        onClose={onClose}
      />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Field label="Gerät" wide>
          <select
            required
            disabled={editing}
            value={form.device_id}
            onChange={(event) =>
              setForm((value) => ({ ...value, device_id: event.target.value }))
            }
            style={inputStyle}
          >
            <option value="" style={optionStyle}>
              Gerät auswählen
            </option>
            {options.devices.map((device) => (
              <option
                key={device.device_id}
                value={device.device_id}
                style={optionStyle}
              >
                {deviceLabel(device)}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Planname" wide>
          <input
            required
            value={form.name}
            onChange={(event) =>
              setForm((value) => ({ ...value, name: event.target.value }))
            }
            placeholder="z. B. Jährliche DGUV-Prüfung"
            style={inputStyle}
          />
        </Field>
        <Field label="Art">
          <select
            value={form.maintenance_type}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                maintenance_type: event.target
                  .value as MaintenancePlanInput["maintenance_type"],
              }))
            }
            style={inputStyle}
          >
            {(["preventive", "inspection", "calibration"] as const).map(
              (value) => (
                <option key={value} value={value} style={optionStyle}>
                  {typeLabels[value]}
                </option>
              ),
            )}
          </select>
        </Field>
        <Field label="Intervall (Tage)">
          <input
            required
            type="number"
            min="1"
            max="3650"
            value={form.interval_days}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                interval_days: Number(event.target.value),
              }))
            }
            style={inputStyle}
          />
        </Field>
        <Field
          label="Vorlauf (Tage)"
          hint="So viele Tage vorher wird der Arbeitsauftrag erzeugt."
        >
          <input
            required
            type="number"
            min="0"
            max="365"
            value={form.lead_time_days}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                lead_time_days: Number(event.target.value),
              }))
            }
            style={inputStyle}
          />
        </Field>
        <Field label="Nächste Fälligkeit">
          <input
            required
            type="date"
            value={form.next_due_at}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                next_due_at: event.target.value,
              }))
            }
            style={inputStyle}
          />
        </Field>
        <Field label="Arbeitsanweisung" full>
          <textarea
            rows={3}
            value={form.instructions}
            onChange={(event) =>
              setForm((value) => ({
                ...value,
                instructions: event.target.value,
              }))
            }
            placeholder="Prüfschritte, Messmittel, Grenzwerte oder Hinweise"
            style={inputStyle}
          />
        </Field>
      </div>
      <label
        className="mt-4 flex min-h-11 items-center gap-2 text-sm"
        style={{ color: "var(--text-secondary)" }}
      >
        <input
          type="checkbox"
          checked={form.is_active}
          onChange={(event) =>
            setForm((value) => ({ ...value, is_active: event.target.checked }))
          }
        />
        Plan aktivieren und Fälligkeiten automatisch erzeugen
      </label>
      <EditorActions
        saving={saving}
        submitLabel={editing ? "Plan speichern" : "Plan anlegen"}
        onClose={onClose}
      />
    </form>
  );
}

function CompletionEditor({
  order,
  value,
  setValue,
  saving,
  onSubmit,
  onClose,
}: {
  order: MaintenanceOrder;
  value: {
    outcome: MaintenanceOutcome;
    resolution: string;
    cost: string;
    next_due_at: string;
  };
  setValue: React.Dispatch<
    React.SetStateAction<{
      outcome: MaintenanceOutcome;
      resolution: string;
      cost: string;
      next_due_at: string;
    }>
  >;
  saving: boolean;
  onSubmit: (event: FormEvent) => void;
  onClose: () => void;
}) {
  return (
    <form
      onSubmit={onSubmit}
      className="rounded-xl border p-5"
      style={{
        borderColor: "var(--color-success)",
        background: "var(--bg-card)",
      }}
    >
      <EditorHeader
        title={`WA-${String(order.order_id).padStart(6, "0")} abschließen`}
        subtitle="Ergebnis und durchgeführte Arbeit bilden den dauerhaften Wartungsnachweis."
        onClose={onClose}
      />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Field label="Ergebnis">
          <select
            value={value.outcome}
            onChange={(event) =>
              setValue((current) => ({
                ...current,
                outcome: event.target.value as MaintenanceOutcome,
              }))
            }
            style={inputStyle}
          >
            {Object.entries(outcomeLabels).map(([key, label]) => (
              <option key={key} value={key} style={optionStyle}>
                {label}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Ist-Kosten (€)">
          <input
            type="number"
            min="0"
            step="0.01"
            value={value.cost}
            onChange={(event) =>
              setValue((current) => ({ ...current, cost: event.target.value }))
            }
            style={inputStyle}
          />
        </Field>
        <Field
          label="Nächster Termin (optional)"
          hint={
            order.plan_id
              ? "Leer lassen: wird automatisch aus dem Plan berechnet."
              : "Bei einmaligen Aufträgen optional."
          }
        >
          <input
            type="date"
            value={value.next_due_at}
            onChange={(event) =>
              setValue((current) => ({
                ...current,
                next_due_at: event.target.value,
              }))
            }
            style={inputStyle}
          />
        </Field>
        <Field label="Abschlussnotiz" full>
          <textarea
            required
            rows={3}
            value={value.resolution}
            onChange={(event) =>
              setValue((current) => ({
                ...current,
                resolution: event.target.value,
              }))
            }
            placeholder="Durchgeführte Arbeiten, Messwerte, verbaute Teile und Resthinweise"
            style={inputStyle}
          />
        </Field>
      </div>
      <EditorActions
        saving={saving}
        submitLabel="Verbindlich abschließen"
        onClose={onClose}
        success
      />
    </form>
  );
}

function EditorHeader({
  title,
  subtitle,
  onClose,
}: {
  title: string;
  subtitle: string;
  onClose: () => void;
}) {
  return (
    <div className="mb-5 flex items-start justify-between gap-4">
      <div>
        <h2
          className="text-lg font-bold"
          style={{ color: "var(--text-primary)" }}
        >
          {title}
        </h2>
        <p className="mt-1 text-xs" style={{ color: "var(--text-secondary)" }}>
          {subtitle}
        </p>
      </div>
      <button
        type="button"
        onClick={onClose}
        className="min-h-10 rounded-lg px-3 text-sm"
        style={{ color: "var(--text-secondary)" }}
      >
        Schließen
      </button>
    </div>
  );
}
function EditorActions({
  saving,
  submitLabel,
  onClose,
  success = false,
}: {
  saving: boolean;
  submitLabel: string;
  onClose: () => void;
  success?: boolean;
}) {
  return (
    <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
      <button
        type="button"
        onClick={onClose}
        className="min-h-11 rounded-lg border px-4 text-sm font-semibold"
        style={{
          borderColor: "var(--border-default)",
          color: "var(--text-secondary)",
        }}
      >
        Abbrechen
      </button>
      <button
        disabled={saving}
        className="min-h-11 rounded-lg px-4 text-sm font-semibold disabled:opacity-50"
        style={{
          background: success ? "var(--color-success)" : "var(--accent-red)",
          color: success ? "var(--bg-primary)" : "var(--text-primary)",
        }}
      >
        {saving ? "Speichert…" : submitLabel}
      </button>
    </div>
  );
}
function Field({
  label,
  hint,
  wide = false,
  full = false,
  children,
}: {
  label: string;
  hint?: string;
  wide?: boolean;
  full?: boolean;
  children: React.ReactElement;
}) {
  return (
    <label
      className={`space-y-1 text-xs font-semibold ${full ? "md:col-span-2 xl:col-span-4" : wide ? "xl:col-span-2" : ""}`}
      style={{ color: "var(--text-secondary)" }}
    >
      <span>{label}</span>
      <div className="[&_input]:min-h-11 [&_input]:w-full [&_input]:rounded-lg [&_input]:border [&_input]:px-3 [&_input]:text-sm [&_select]:min-h-11 [&_select]:w-full [&_select]:rounded-lg [&_select]:border [&_select]:px-3 [&_select]:text-sm [&_textarea]:w-full [&_textarea]:rounded-lg [&_textarea]:border [&_textarea]:px-3 [&_textarea]:py-2.5 [&_textarea]:text-sm">
        {children}
      </div>
      {hint && (
        <span
          className="block font-normal leading-4"
          style={{ color: "var(--text-muted)" }}
        >
          {hint}
        </span>
      )}
    </label>
  );
}
function MaintenanceSkeleton() {
  return (
    <div className="space-y-6" aria-label="Wartung wird geladen">
      <div
        className="h-20 animate-pulse rounded-xl"
        style={{ background: "var(--bg-card)" }}
      />
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {[0, 1, 2, 3].map((item) => (
          <div
            key={item}
            className="h-28 animate-pulse rounded-xl"
            style={{ background: "var(--bg-card)" }}
          />
        ))}
      </div>
      <div
        className="h-80 animate-pulse rounded-xl"
        style={{ background: "var(--bg-card)" }}
      />
    </div>
  );
}
