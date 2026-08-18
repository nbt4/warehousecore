import { useEffect, useState } from 'react';
import { Package, Warehouse, AlertTriangle, TrendingUp, Clock3, MapPinOff } from 'lucide-react';
import { dashboardApi } from '../lib/api';
import type { DashboardStats, Movement } from '../lib/api';
import { LowStockAlertsWidget } from '../components/LowStockAlertsWidget';
import { toast } from '../lib/toast';

export function Dashboard() {
  const [stats, setStats] = useState<DashboardStats>({
    in_storage: 0,
    on_job: 0,
    return_pending: 0,
    location_unknown: 0,
    defective: 0,
    total: 0,
  });
  const [recentActivity, setRecentActivity] = useState<Movement[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    void loadData();
    const interval = setInterval(() => {
      void loadData();
    }, 10000); // Refresh every 10s
    return () => clearInterval(interval);
  }, []);

  const loadData = async () => {
    try {
      const { data } = await dashboardApi.getStats();
      setStats(data);
    } catch (error) {
      toast.error('Failed to load stats:' + " " + String(error));
    }

    try {
      const { data } = await dashboardApi.getRecentMovements(10);
      setRecentActivity(data);
    } catch (error) {
      toast.error('Failed to load recent activity:' + " " + String(error));
    }

    if (loading) {
      setLoading(false);
    }
  };

  const formatRelativeTime = (isoTimestamp: string): string => {
    const date = new Date(isoTimestamp);
    if (Number.isNaN(date.getTime())) {
      return '';
    }

    const diffMs = Date.now() - date.getTime();
    if (diffMs <= 0) {
      return 'gerade eben';
    }

    const diffSeconds = Math.floor(diffMs / 1000);
    if (diffSeconds < 60) {
      return 'vor wenigen Sekunden';
    }

    const diffMinutes = Math.floor(diffSeconds / 60);
    if (diffMinutes < 60) {
      return `vor ${diffMinutes} ${diffMinutes === 1 ? 'Minute' : 'Minuten'}`;
    }

    const diffHours = Math.floor(diffMinutes / 60);
    if (diffHours < 24) {
      return `vor ${diffHours} ${diffHours === 1 ? 'Stunde' : 'Stunden'}`;
    }

    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 7) {
      return `vor ${diffDays} ${diffDays === 1 ? 'Tag' : 'Tagen'}`;
    }

    const diffWeeks = Math.floor(diffDays / 7);
    if (diffWeeks < 5) {
      return `vor ${diffWeeks} ${diffWeeks === 1 ? 'Woche' : 'Wochen'}`;
    }

    const diffMonths = Math.floor(diffDays / 30);
    if (diffMonths < 12) {
      return `vor ${diffMonths} ${diffMonths === 1 ? 'Monat' : 'Monaten'}`;
    }

    const diffYears = Math.floor(diffDays / 365);
    return `vor ${diffYears} ${diffYears === 1 ? 'Jahr' : 'Jahren'}`;
  };

  const describeMovement = (movement: Movement): string => {
    const deviceLabel =
      movement.product_name ??
      movement.serial_number ??
      movement.device_id;

    switch (movement.action) {
      case 'intake':
        return movement.to_zone_name
          ? `${deviceLabel} in ${movement.to_zone_name} eingecheckt`
          : `${deviceLabel} ins Lager eingecheckt`;
      case 'outtake':
        return movement.to_job_description
          ? `${deviceLabel} für ${movement.to_job_description} ausgebucht`
          : `${deviceLabel} aus dem Lager ausgebucht`;
      case 'transfer':
        if (movement.from_zone_name && movement.to_zone_name) {
          return `${deviceLabel} von ${movement.from_zone_name} nach ${movement.to_zone_name} verschoben`;
        }
        if (movement.to_zone_name) {
          return `${deviceLabel} nach ${movement.to_zone_name} verschoben`;
        }
        if (movement.from_zone_name) {
          return `${deviceLabel} aus ${movement.from_zone_name} entnommen`;
        }
        return `${deviceLabel} verschoben`;
      case 'return':
        return `${deviceLabel} zurückgebucht`;
      case 'move':
        return `${deviceLabel} bewegt`;
      default:
        return `${deviceLabel} (${movement.action})`;
    }
  };

  const activityItems = recentActivity.slice(0, 5);

  const statCards = [
    {
      title: 'Im Lager',
      value: stats.in_storage,
      icon: Warehouse,
      iconColor: 'var(--text-secondary)',
      iconBg: 'var(--bg-hover)',
      valueColor: 'var(--text-primary)',
    },
    {
      title: 'Ausgegeben',
      value: stats.on_job,
      icon: Package,
      iconColor: 'var(--accent-red)',
      iconBg: 'rgba(var(--accent-red-rgb), 0.1)',
      valueColor: 'var(--accent-red-light)',
    },
    {
      title: 'Rückgabe offen',
      value: stats.return_pending,
      icon: Clock3,
      iconColor: 'var(--color-warning)',
      iconBg: 'rgba(var(--color-warning-rgb), 0.1)',
      valueColor: 'var(--color-warning)',
    },
    {
      title: 'Standort ungeklärt',
      value: stats.location_unknown,
      icon: MapPinOff,
      iconColor: 'var(--color-warning)',
      iconBg: 'rgba(var(--color-warning-rgb), 0.1)',
      valueColor: 'var(--color-warning)',
    },
    {
      title: 'Defekt',
      value: stats.defective,
      icon: AlertTriangle,
      iconColor: 'var(--color-warning)',
      iconBg: 'rgba(var(--color-warning-rgb), 0.1)',
      valueColor: 'var(--color-warning)',
    },
    {
      title: 'Gesamt',
      value: stats.total,
      icon: TrendingUp,
      iconColor: 'var(--color-info)',
      iconBg: 'rgba(var(--color-info-rgb), 0.1)',
      valueColor: 'var(--color-info)',
    },
  ];

  if (loading) {
    return (
      <div className="flex items-center justify-center h-96">
        <div
          className="rounded-full h-10 w-10 border-2 border-white/10 border-t-[var(--accent-red)] animate-spin"
        />
      </div>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-6">
      <div>
        <h2 className="font-bold text-white mb-1 text-[1.75rem]">Dashboard</h2>
        <p className="text-sm text-[var(--text-secondary)]">Lagerübersicht und Statistiken</p>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-3 sm:gap-4">
        {statCards.map((card) => {
          const Icon = card.icon;
          return (
            <div
              key={card.title}
              className="card p-4 sm:p-5 transition-all duration-200 hover:border-white/14 cursor-default"
            >
              <div
                className="inline-flex items-center justify-center w-10 h-10 rounded-lg mb-3"
                style={{ background: card.iconBg }}
              >
                <Icon className="w-5 h-5" style={{ color: card.iconColor }} />
              </div>
              <p className="text-xs font-medium mb-1 text-[var(--text-secondary)]">{card.title}</p>
              <p className="font-bold leading-none text-[2rem]" style={{ color: card.valueColor }}>
                {card.value}
              </p>
            </div>
          );
        })}
      </div>

      {/* Low Stock Alerts */}
      <LowStockAlertsWidget />

      {/* Recent Activity */}
      <div className="card p-4 sm:p-6">
        <h3 className="font-semibold text-white mb-4 text-base">Letzte Aktivität</h3>
        {activityItems.length === 0 ? (
          <div className="text-sm py-6 text-center text-[var(--text-secondary)]">
            Noch keine Aktivitäten erfasst.
          </div>
        ) : (
          <div className="space-y-2">
            {activityItems.map((activity) => (
              <div
                key={activity.movement_id}
                className="flex items-center gap-3 p-3 rounded-lg transition-colors bg-white/[0.03]"
              >
                <div
                  className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                  style={{ background: 'var(--accent-red)' }}
                />
                <div className="flex-1 min-w-0">
                  <p className="text-sm text-white font-medium truncate">
                    {describeMovement(activity)}
                  </p>
                  <div className="flex items-center gap-2 text-xs mt-0.5 text-[var(--text-secondary)]">
                    <span>{formatRelativeTime(activity.timestamp) || 'gerade eben'}</span>
                    {activity.performed_by && (
                      <>
                        <span>·</span>
                        <span className="truncate">{activity.performed_by}</span>
                      </>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
