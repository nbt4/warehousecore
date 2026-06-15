import { useEffect, useState } from 'react';
import { AlertTriangle, Package } from 'lucide-react';
import { toast } from '../lib/toast';

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
  const [alerts, setAlerts] = useState<LowStockAlert[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadLowStockAlerts();
    // Refresh alerts every 5 minutes
    const interval = setInterval(loadLowStockAlerts, 5 * 60 * 1000);
    return () => clearInterval(interval);
  }, []);

  const loadLowStockAlerts = async () => {
    try {
      setLoading(true);
      const response = await fetch('/api/v1/inventory/low-stock');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data: LowStockResponse = await response.json();
      setAlerts(data.alerts || []);
      setError(null);
    } catch (err) {
      toast.error('Failed to load low stock alerts:' + " " + String(err));
      setError('Failed to load alerts');
    } finally {
      setLoading(false);
    }
  };

  if (loading && alerts.length === 0) {
    return (
      <div className="glass rounded-xl p-4">
        <div className="flex items-center gap-3 mb-3">
          <AlertTriangle className="w-5 h-5 text-yellow-500" />
          <h3 className="text-lg font-semibold text-white">Low Stock Alerts</h3>
        </div>
        <p className="text-sm text-gray-400">Loading...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="glass rounded-xl p-4 border border-red-500/30">
        <div className="flex items-center gap-3 mb-3">
          <AlertTriangle className="w-5 h-5 text-red-500" />
          <h3 className="text-lg font-semibold text-white">Low Stock Alerts</h3>
        </div>
        <p className="text-sm text-red-400">{error}</p>
      </div>
    );
  }

  if (alerts.length === 0) {
    return (
      <div className="glass rounded-xl p-4">
        <div className="flex items-center gap-3 mb-3">
          <Package className="w-5 h-5 text-green-500" />
          <h3 className="text-lg font-semibold text-white">Low Stock Alerts</h3>
        </div>
        <p className="text-sm text-gray-400">All items are adequately stocked ✓</p>
      </div>
    );
  }

  return (
    <div className="glass rounded-xl p-4 border border-yellow-500/30">
      <div className="flex items-center justify-between gap-3 mb-3">
        <div className="flex items-center gap-3">
          <AlertTriangle className="w-5 h-5 text-yellow-500" />
          <h3 className="text-lg font-semibold text-white">Low Stock Alerts</h3>
        </div>
        <span className="px-2 py-1 bg-yellow-500/20 text-yellow-300 text-xs font-semibold rounded-full">
          {alerts.length}
        </span>
      </div>

      <div className="space-y-2 max-h-64 overflow-y-auto">
        {alerts.map((alert) => (
          <div
            key={alert.product_id}
            className="bg-white/5 rounded-lg p-3 border border-yellow-500/20 hover:bg-white/10 transition-colors"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <h4 className="text-sm font-semibold text-white truncate">
                    {alert.name}
                  </h4>
                  <span className={`px-2 py-0.5 text-xs rounded ${
                    alert.is_accessory
                      ? 'bg-blue-500/20 text-blue-300'
                      : 'bg-purple-500/20 text-purple-300'
                  }`}>
                    {alert.is_accessory ? 'Accessory' : 'Consumable'}
                  </span>
                </div>
                <p className="text-xs text-gray-400 mb-1">
                  Barcode: {alert.generic_barcode}
                </p>
                <div className="flex items-center gap-2 text-xs">
                  <span className="text-yellow-400 font-semibold">
                    {alert.stock_quantity.toFixed(2)} {alert.count_type_abbr}
                  </span>
                  <span className="text-gray-500">•</span>
                  <span className="text-gray-400">
                    Min: {alert.min_stock_level.toFixed(2)} {alert.count_type_abbr}
                  </span>
                </div>
              </div>
              <div className="flex-shrink-0">
                <div className="text-right">
                  <div className="text-xs font-semibold text-yellow-500">
                    {Math.round((alert.stock_quantity / alert.min_stock_level) * 100)}%
                  </div>
                  <div className="text-xs text-gray-500">of min</div>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>

      <button
        onClick={loadLowStockAlerts}
        className="mt-3 w-full py-2 text-xs font-semibold text-white bg-white/10 hover:bg-white/20 rounded-lg transition-colors"
      >
        Refresh
      </button>
    </div>
  );
}
