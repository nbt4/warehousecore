import { useCallback, useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, RefreshCw } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { toast } from '../lib/toast';
import { appPath } from '../lib/app-paths';

interface LowStockAlert {
  product_id: number;
  name: string;
  stock_quantity: number;
  min_stock_level: number;
  count_type_name: string;
  count_type_abbr: string;
  generic_barcode: string;
  is_accessory: boolean;
  is_consumable: boolean;
}

interface LowStockResponse {
  alerts: LowStockAlert[];
}

export function LowStockAlertsWidget() {
  const navigate = useNavigate();
  const [alerts, setAlerts] = useState<LowStockAlert[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadLowStockAlerts = useCallback(async (notify = false) => {
    try {
      setLoading(true);
      const response = await fetch(appPath('/api/v1/inventory/low-stock'));
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data: LowStockResponse = await response.json();
      setAlerts(data.alerts || []);
      setError(null);
    } catch (err) {
      if (notify) toast.error(`Mindestbestände konnten nicht geladen werden: ${String(err)}`);
      setError('Mindestbestände konnten nicht geladen werden.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadLowStockAlerts();
    const interval = window.setInterval(() => void loadLowStockAlerts(), 5 * 60 * 1000);
    return () => window.clearInterval(interval);
  }, [loadLowStockAlerts]);

  if (loading && alerts.length === 0) {
    return (
      <div className="card p-4 sm:p-5">
        <WidgetTitle icon={RefreshCw} title="Mindestbestände" spinning />
        <p className="mt-3 text-sm" style={{ color: 'var(--text-secondary)' }}>Bestände werden geprüft…</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="card p-4 sm:p-5" style={{ borderColor: 'rgba(var(--color-error-rgb),0.35)' }}>
        <WidgetTitle icon={AlertTriangle} title="Mindestbestände" color="var(--color-error)" />
        <p className="mt-3 text-sm" style={{ color: 'var(--color-error)' }}>{error}</p>
        <button type="button" onClick={() => void loadLowStockAlerts(true)} className="mt-4 flex items-center gap-2 rounded-lg border px-3 py-2 text-xs font-semibold" style={{ borderColor: 'var(--border-default)', color: 'var(--text-primary)' }}>
          <RefreshCw className="h-3.5 w-3.5" /> Erneut versuchen
        </button>
      </div>
    );
  }

  if (alerts.length === 0) {
    return (
      <div className="card p-4 sm:p-5">
        <WidgetTitle icon={CheckCircle2} title="Mindestbestände" color="var(--color-success)" />
        <p className="mt-3 text-sm" style={{ color: 'var(--text-secondary)' }}>Alle überwachten Artikel liegen über ihrem Mindestbestand.</p>
      </div>
    );
  }

  return (
    <div className="card p-4 sm:p-5" style={{ borderColor: 'rgba(var(--color-warning-rgb),0.35)' }}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <WidgetTitle icon={AlertTriangle} title="Mindestbestände" color="var(--color-warning)" />
          <p className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>Nachbestellung oder Bestandsprüfung erforderlich</p>
        </div>
        <span className="rounded-full px-2.5 py-1 text-xs font-bold" style={{ background: 'rgba(var(--color-warning-rgb),0.14)', color: 'var(--color-warning)' }}>{alerts.length}</span>
      </div>

      <div className="mt-4 max-h-64 space-y-2 overflow-y-auto pr-1">
        {alerts.map((alert) => {
          const percentage = alert.min_stock_level > 0
            ? Math.round((alert.stock_quantity / alert.min_stock_level) * 100)
            : 0;
          return (
            <button key={alert.product_id} type="button" onClick={() => navigate('/products')} className="w-full rounded-lg border p-3 text-left transition-colors hover:bg-white/[0.04]" style={{ borderColor: 'var(--border-subtle)', background: 'var(--bg-subtle)' }}>
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <h4 className="truncate text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>{alert.name}</h4>
                    <span className="rounded px-2 py-0.5 text-[10px]" style={{ background: alert.is_accessory ? 'rgba(var(--color-info-rgb),0.14)' : 'rgba(var(--color-warning-rgb),0.14)', color: alert.is_accessory ? 'var(--color-info)' : 'var(--color-warning)' }}>{alert.is_accessory ? 'Zubehör' : 'Verbrauch'}</span>
                  </div>
                  <p className="mt-1 truncate font-mono text-[10px]" style={{ color: 'var(--text-muted)' }}>{alert.generic_barcode || 'Ohne Barcode'}</p>
                  <p className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}><strong style={{ color: 'var(--color-warning)' }}>{alert.stock_quantity.toFixed(2)} {alert.count_type_abbr}</strong> · Minimum {alert.min_stock_level.toFixed(2)} {alert.count_type_abbr}</p>
                </div>
                <span className="text-xs font-bold" style={{ color: 'var(--color-warning)' }}>{percentage}%</span>
              </div>
            </button>
          );
        })}
      </div>

      <div className="mt-3 grid grid-cols-2 gap-2">
        <button type="button" onClick={() => void loadLowStockAlerts(true)} disabled={loading} className="flex items-center justify-center gap-2 rounded-lg border py-2 text-xs font-semibold disabled:opacity-50" style={{ borderColor: 'var(--border-subtle)', color: 'var(--text-primary)' }}><RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />Aktualisieren</button>
        <button type="button" onClick={() => navigate('/products')} className="rounded-lg py-2 text-xs font-semibold" style={{ background: 'var(--accent-red)', color: 'var(--text-primary)' }}>Artikel öffnen</button>
      </div>
    </div>
  );
}

function WidgetTitle({ icon: Icon, title, color = 'var(--text-secondary)', spinning = false }: { icon: typeof AlertTriangle; title: string; color?: string; spinning?: boolean }) {
  return <div className="flex items-center gap-3"><Icon className={`h-5 w-5 ${spinning ? 'animate-spin' : ''}`} style={{ color }} /><h3 className="font-bold" style={{ color: 'var(--text-primary)' }}>{title}</h3></div>;
}
