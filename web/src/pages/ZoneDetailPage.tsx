import { useEffect, useMemo, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { MapPin, Package, ChevronRight, ArrowLeft, Plus, Trash2, FlaskConical } from 'lucide-react';
import { zonesApi, type ProductInZone } from '../lib/api';
import { DeviceTreeModal } from '../components/DeviceTreeModal';
import { DeviceDetailModal } from '../components/DeviceDetailModal';
import type { Device } from '../lib/api';
import { useZoneTypes } from '../lib/useZoneTypes';
import { toast } from '../lib/toast';

interface ZoneDetails {
  zone_id: number;
  code: string;
  name: string;
  type: string;
  description?: string;
  capacity?: number;
  device_count: number;
  is_active: boolean;
  breadcrumb: Array<{ zone_id: string; code: string; name: string }>;
  subzones: Array<{
    zone_id: number;
    code: string;
    name: string;
    type: string;
    capacity?: number;
    device_count: number;
    subzone_count: number;
  }>;
}

interface DeviceInZone {
  device_id: string;
  product_id?: number;
  product_name: string;
  serial_number?: string;
  manufacturer?: string;
  model?: string;
  status: string;
  barcode?: string;
  qr_code?: string;
  zone_code?: string;
  condition_rating: number;
  usage_hours: number;
}

export function ZoneDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [zone, setZone] = useState<ZoneDetails | null>(null);
  const [devices, setDevices] = useState<DeviceInZone[]>([]);
  const [products, setProducts] = useState<ProductInZone[]>([]);
  const [loading, setLoading] = useState(true);
  const [isDeviceModalOpen, setIsDeviceModalOpen] = useState(false);
  const [selectedDevice, setSelectedDevice] = useState<Device | null>(null);
  const [isDetailModalOpen, setIsDetailModalOpen] = useState(false);
  const { zoneTypes: zoneTypeDefs } = useZoneTypes();

  const zoneTypeMap = useMemo(() => {
    const iconMap: Record<string, string> = {
      warehouse: '🏭',
      rack: '🗄️',
      gitterbox: '📦',
      shelf: '📚',
      vehicle: '🚚',
      stage: '🎤',
      case: '🧳',
      other: '📍',
    };

    const map: Record<string, { label: string; icon: string }> = {};

    zoneTypeDefs.forEach((type) => {
      map[type.key] = {
        label: type.label,
        icon: iconMap[type.key] || iconMap.other,
      };
    });

    return map;
  }, [zoneTypeDefs]);

  useEffect(() => {
    if (id) {
      loadZoneDetails();
      loadDevices();
      loadProducts();

      // Auto-refresh when page becomes visible
      const handleVisibilityChange = () => {
        if (!document.hidden) {
          loadZoneDetails();
          loadDevices();
          loadProducts();
        }
      };

      // Auto-refresh every 30 seconds
      const interval = setInterval(() => {
        if (!document.hidden) {
          loadZoneDetails();
          loadDevices();
          loadProducts();
        }
      }, 30000);

      document.addEventListener('visibilitychange', handleVisibilityChange);

      return () => {
        document.removeEventListener('visibilitychange', handleVisibilityChange);
        clearInterval(interval);
      };
    }
  }, [id]);

  const loadZoneDetails = async () => {
    try {
      const { data } = await zonesApi.getById(Number(id));
      setZone(data as ZoneDetails);
    } catch (error) {
      toast.error('Failed to load zone details:' + " " + String(error));
    } finally {
      setLoading(false);
    }
  };

  const loadDevices = async () => {
    try {
      const response = await fetch(`/api/v1/zones/${id}/devices`);
      const data = await response.json();
      setDevices(data);
    } catch (error) {
      toast.error('Failed to load devices:' + " " + String(error));
    }
  };

  const loadProducts = async () => {
    try {
      const { data } = await zonesApi.getProducts(Number(id));
      setProducts(data);
    } catch (error) {
      toast.error('Failed to load products:' + " " + String(error));
    }
  };

  const handleZoneClick = (zoneId: number) => {
    navigate(`/zones/${zoneId}`);
  };

  const handleCreateSubzone = () => {
    navigate(`/zones?parent=${id}`);
  };

  const handleCreateShelves = async (count: number) => {
    if (!zone) return;

    try {
      // Create shelves sequentially to avoid race conditions with auto-naming
      for (let i = 0; i < count; i++) {
        await zonesApi.create({
          type: 'shelf',
          parent_zone_id: zone.zone_id,
          capacity: 10, // Default capacity for shelves
          is_active: true,
        });
      }
      loadZoneDetails(); // Reload to show new shelves
    } catch (error) {
      toast.error('Failed to create shelves:' + " " + String(error));
      alert('Fehler beim Erstellen der Fächer');
    }
  };

  const handleDelete = async () => {
    if (!zone) return;

    if (zone.device_count > 0) {
      alert('Diese Zone enthält noch Geräte und kann nicht gelöscht werden.');
      return;
    }

    if (zone.subzones && zone.subzones.length > 0) {
      alert('Diese Zone enthält noch Unterzonen und kann nicht gelöscht werden.');
      return;
    }

    if (!confirm(`Zone "${zone.name}" (${zone.code}) wirklich löschen?`)) {
      return;
    }

    try {
      await zonesApi.delete(zone.zone_id);
      navigate('/zones');
    } catch (error) {
      toast.error('Failed to delete zone:' + " " + String(error));
      alert('Fehler beim Löschen der Zone');
    }
  };

  const handleDeleteSubzone = async (e: React.MouseEvent, subzone: any) => {
    e.stopPropagation(); // Prevent navigation when clicking delete

    if (!confirm(`Zone "${subzone.name}" (${subzone.code}) wirklich löschen?`)) {
      return;
    }

    try {
      await zonesApi.delete(subzone.zone_id);
      loadZoneDetails(); // Reload to show updated subzones list
    } catch (error) {
      toast.error('Failed to delete subzone:' + " " + String(error));
      alert('Fehler beim Löschen der Zone. Prüfe ob die Zone Unterzonen oder Geräte enthält.');
    }
  };

  const handleDeviceClick = (device: DeviceInZone) => {
    // Convert DeviceInZone to Device type
    const deviceForModal: Device = {
      device_id: device.device_id,
      product_id: device.product_id,
      product_name: device.product_name,
      serial_number: device.serial_number,
      barcode: device.barcode,
      qr_code: device.qr_code,
      status: device.status,
      zone_name: zone?.name,
      zone_code: device.zone_code,
      zone_id: zone?.zone_id,
      condition_rating: device.condition_rating,
      usage_hours: device.usage_hours,
    };
    setSelectedDevice(deviceForModal);
    setIsDetailModalOpen(true);
  };

  const handleAssignDevices = async (deviceIds: string[]) => {
    if (!id || deviceIds.length === 0) return;

    try {
      const response = await fetch(`/api/v1/zones/${id}/devices`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ device_ids: deviceIds }),
      });

      const result = await response.json();

      if (response.ok) {
        alert(`${result.success} von ${result.total} Geräten erfolgreich zugewiesen`);
        loadDevices(); // Reload devices
        loadZoneDetails(); // Reload zone details to update device count
      } else {
        alert(`Fehler beim Zuweisen der Geräte: ${result.error || 'Unbekannter Fehler'}`);
      }
    } catch (error) {
      toast.error('Failed to assign devices:' + " " + String(error));
      alert('Fehler beim Zuweisen der Geräte');
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-96">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-accent-red"></div>
      </div>
    );
  }

  if (!zone) {
    return (
      <div className="text-center py-12">
        <p className="text-gray-400">Zone nicht gefunden</p>
      </div>
    );
  }

  const defaultTypeInfo = { label: 'Sonstige', icon: '📍' };
  const typeInfo = zoneTypeMap[zone.type] || defaultTypeInfo;

  return (
    <div className="space-y-4 sm:space-y-6 max-w-full overflow-x-hidden">
      {/* Header with Breadcrumb */}
      <div>
        <button
          onClick={() => navigate('/zones')}
          className="flex items-center gap-2 text-sm sm:text-base text-gray-400 hover:text-white mb-3 sm:mb-4 transition-colors"
        >
          <ArrowLeft className="w-4 h-4" />
          Zurück zu Lager
        </button>

        {/* Breadcrumb */}
        {zone.breadcrumb && zone.breadcrumb.length > 0 && (
          <div className="flex items-center gap-2 text-xs sm:text-sm text-gray-400 mb-3 sm:mb-4 flex-wrap overflow-x-auto pb-2">
            {zone.breadcrumb.map((crumb, index) => (
              <div key={crumb.zone_id} className="flex items-center gap-2 flex-shrink-0">
                {index > 0 && <ChevronRight className="w-3 h-3 sm:w-4 sm:h-4" />}
                <button
                  onClick={() => handleZoneClick(Number(crumb.zone_id))}
                  className="hover:text-white transition-colors whitespace-nowrap"
                >
                  {crumb.name}
                </button>
              </div>
            ))}
          </div>
        )}

        <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-4">
          <div className="flex items-center gap-3 sm:gap-4 min-w-0">
            <div className="text-3xl sm:text-5xl flex-shrink-0">{typeInfo.icon}</div>
            <div className="min-w-0 flex-1">
              <h2 className="text-xl sm:text-3xl font-bold text-white mb-0.5 sm:mb-1 truncate">{zone.name}</h2>
              <p className="text-gray-400 font-mono text-xs sm:text-sm truncate">{zone.code}</p>
              <p className="text-xs sm:text-sm text-gray-500">{typeInfo.label}</p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2 sm:gap-3">
            <button
              onClick={handleDelete}
              className="flex items-center gap-1.5 sm:gap-2 px-3 sm:px-4 py-2 glass text-red-400 hover:text-red-300 hover:bg-red-900/20 font-semibold text-sm sm:text-base rounded-lg sm:rounded-xl transition-all whitespace-nowrap"
            >
              <Trash2 className="w-4 h-4" />
              <span className="hidden sm:inline">Löschen</span>
              <span className="sm:hidden">🗑️</span>
            </button>
            <button
              onClick={() => setIsDeviceModalOpen(true)}
              className="flex items-center gap-1.5 sm:gap-2 px-3 sm:px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white font-semibold text-sm sm:text-base rounded-lg sm:rounded-xl transition-all whitespace-nowrap"
            >
              <Package className="w-4 h-4" />
              <span className="hidden sm:inline">Geräte hinzufügen</span>
              <span className="sm:hidden">Geräte</span>
            </button>
            {zone.type === 'rack' && (
              <button
                onClick={() => {
                  const count = prompt('Wie viele Fächer sollen erstellt werden?', '5');
                  if (count) {
                    handleCreateShelves(parseInt(count));
                  }
                }}
                className="flex items-center gap-1.5 sm:gap-2 px-3 sm:px-4 py-2 bg-purple-600 hover:bg-purple-700 text-white font-semibold text-sm sm:text-base rounded-lg sm:rounded-xl transition-all whitespace-nowrap"
              >
                <Plus className="w-4 h-4" />
                <span className="hidden sm:inline">Fächer erstellen</span>
                <span className="sm:hidden">Fächer</span>
              </button>
            )}
            <button
              onClick={handleCreateSubzone}
              className="flex items-center gap-1.5 sm:gap-2 px-3 sm:px-4 py-2 bg-gradient-to-r from-accent-red to-red-700 text-white font-semibold text-sm sm:text-base rounded-lg sm:rounded-xl hover:shadow-lg hover:shadow-accent-red/50 transition-all whitespace-nowrap"
            >
              <Plus className="w-4 h-4" />
              <span className="hidden sm:inline">Unterzone erstellen</span>
              <span className="sm:hidden">Unterzone</span>
            </button>
          </div>
        </div>

        {zone.description && (
          <p className="text-sm sm:text-base text-gray-400 mt-3 sm:mt-4">{zone.description}</p>
        )}

        {/* Stats */}
        <div className="grid grid-cols-3 gap-2 sm:gap-4 mt-4 sm:mt-6">
          <div className="glass rounded-lg sm:rounded-xl p-3 sm:p-4">
            <div className="text-xl sm:text-2xl font-bold text-white">{zone.device_count}</div>
            <div className="text-xs sm:text-sm text-gray-400">Geräte</div>
          </div>
          <div className="glass rounded-lg sm:rounded-xl p-3 sm:p-4">
            <div className="text-xl sm:text-2xl font-bold text-white">{zone.subzones?.length || 0}</div>
            <div className="text-xs sm:text-sm text-gray-400">Unterzonen</div>
          </div>
          {zone.capacity && (
            <div className="glass rounded-lg sm:rounded-xl p-3 sm:p-4">
              <div className="text-xl sm:text-2xl font-bold text-white">{zone.capacity}</div>
              <div className="text-xs sm:text-sm text-gray-400">Kapazität</div>
            </div>
          )}
        </div>
      </div>

      {/* Subzones */}
      {zone.subzones && zone.subzones.length > 0 && (
        <div>
          <h3 className="text-lg sm:text-xl font-bold text-white mb-3 sm:mb-4 flex items-center gap-2">
            <MapPin className="w-4 h-4 sm:w-5 sm:h-5" />
            Unterzonen
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-4">
            {zone.subzones.map((subzone) => {
              const subTypeInfo = zoneTypeMap[subzone.type] || defaultTypeInfo;
              return (
                <div
                  key={subzone.zone_id}
                  className="glass rounded-lg sm:rounded-xl p-4 sm:p-5 hover:bg-white/20 transition-all cursor-pointer group relative"
                  onClick={() => handleZoneClick(subzone.zone_id)}
                >
                  <button
                    onClick={(e) => handleDeleteSubzone(e, subzone)}
                    className="absolute top-2 right-2 sm:top-3 sm:right-3 p-1.5 sm:p-2 glass-dark rounded-lg text-red-400 hover:text-red-300 hover:bg-red-900/20 opacity-0 group-hover:opacity-100 transition-all z-10"
                    title="Zone löschen"
                  >
                    <Trash2 className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
                  </button>
                  <div className="flex items-start gap-3 sm:gap-4">
                    <div className="text-2xl sm:text-3xl flex-shrink-0">{subTypeInfo.icon}</div>
                    <div className="flex-1 min-w-0">
                      <h4 className="text-sm sm:text-base font-bold text-white truncate group-hover:text-accent-red transition-colors">
                        {subzone.name}
                      </h4>
                      <p className="text-xs sm:text-sm text-gray-400 font-mono truncate">{subzone.code}</p>
                      <div className="flex items-center gap-2 sm:gap-3 mt-1.5 sm:mt-2 text-[10px] sm:text-xs text-gray-500">
                        <span>{subzone.device_count} Geräte</span>
                        {subzone.subzone_count > 0 && (
                          <span>{subzone.subzone_count} Unterzonen</span>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Devices */}
      {devices.length > 0 && (
        <div>
          <h3 className="text-lg sm:text-xl font-bold text-white mb-3 sm:mb-4 flex items-center gap-2">
            <Package className="w-4 h-4 sm:w-5 sm:h-5" />
            Gelagerte Geräte ({devices.length})
          </h3>
          <div className="glass-dark rounded-xl sm:rounded-2xl overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[640px]">
                <thead>
                  <tr className="border-b border-white/10">
                    <th className="text-left p-2 sm:p-4 text-xs sm:text-sm font-semibold text-gray-400 whitespace-nowrap">Geräte-ID</th>
                    <th className="text-left p-2 sm:p-4 text-xs sm:text-sm font-semibold text-gray-400 whitespace-nowrap">Produkt</th>
                    <th className="text-left p-2 sm:p-4 text-xs sm:text-sm font-semibold text-gray-400 whitespace-nowrap hidden md:table-cell">Hersteller</th>
                    <th className="text-left p-2 sm:p-4 text-xs sm:text-sm font-semibold text-gray-400 whitespace-nowrap hidden md:table-cell">Modell</th>
                    <th className="text-left p-2 sm:p-4 text-xs sm:text-sm font-semibold text-gray-400 whitespace-nowrap hidden lg:table-cell">Barcode</th>
                  </tr>
                </thead>
                <tbody>
                  {devices.map((device) => (
                    <tr
                      key={device.device_id}
                      onClick={() => handleDeviceClick(device)}
                      className="border-b border-white/5 hover:bg-white/5 hover:cursor-pointer transition-colors"
                    >
                      <td className="p-2 sm:p-4 font-mono text-xs sm:text-sm text-white">{device.device_id}</td>
                      <td className="p-2 sm:p-4 text-xs sm:text-base text-white">{device.product_name || '-'}</td>
                      <td className="p-2 sm:p-4 text-xs sm:text-base text-gray-400 hidden md:table-cell">{device.manufacturer || '-'}</td>
                      <td className="p-2 sm:p-4 text-xs sm:text-base text-gray-400 hidden md:table-cell">{device.model || '-'}</td>
                      <td className="p-2 sm:p-4 font-mono text-xs sm:text-sm text-gray-400 hidden lg:table-cell">{device.barcode || '-'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* Products (Consumables & Accessories) */}
      {products.length > 0 && (
        <div>
          <h3 className="text-lg sm:text-xl font-bold text-white mb-3 sm:mb-4 flex items-center gap-2">
            <FlaskConical className="w-4 h-4 sm:w-5 sm:h-5" />
            Verbrauchsmaterialien & Zubehör ({products.length})
          </h3>
          <div className="glass-dark rounded-xl sm:rounded-2xl overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[640px]">
                <thead>
                  <tr className="border-b border-white/10">
                    <th className="text-left p-2 sm:p-4 text-xs sm:text-sm font-semibold text-gray-400 whitespace-nowrap">Produkt</th>
                    <th className="text-left p-2 sm:p-4 text-xs sm:text-sm font-semibold text-gray-400 whitespace-nowrap">Typ</th>
                    <th className="text-left p-2 sm:p-4 text-xs sm:text-sm font-semibold text-gray-400 whitespace-nowrap">Menge</th>
                    <th className="text-left p-2 sm:p-4 text-xs sm:text-sm font-semibold text-gray-400 whitespace-nowrap">Einheit</th>
                  </tr>
                </thead>
                <tbody>
                  {products.map((product) => (
                    <tr
                      key={product.product_id}
                      className="border-b border-white/5 hover:bg-white/5 transition-colors"
                    >
                      <td className="p-2 sm:p-4 text-xs sm:text-base text-white">{product.product_name}</td>
                      <td className="p-2 sm:p-4 text-xs sm:text-sm">
                        {product.is_consumable && (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-blue-500/20 text-blue-400 text-xs font-medium">
                            Verbrauchsmaterial
                          </span>
                        )}
                        {product.is_accessory && (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-purple-500/20 text-purple-400 text-xs font-medium">
                            Zubehör
                          </span>
                        )}
                      </td>
                      <td className="p-2 sm:p-4 text-xs sm:text-base text-white font-semibold">{product.quantity}</td>
                      <td className="p-2 sm:p-4 text-xs sm:text-sm text-gray-400">{product.unit}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {devices.length === 0 && products.length === 0 && (!zone.subzones || zone.subzones.length === 0) && (
        <div className="text-center py-8 sm:py-12 glass rounded-xl sm:rounded-2xl">
          <Package className="w-12 h-12 sm:w-16 sm:h-16 text-gray-600 mx-auto mb-3 sm:mb-4" />
          <p className="text-sm sm:text-base text-gray-400 mb-3 sm:mb-4">Diese Zone ist leer</p>
          <button
            onClick={handleCreateSubzone}
            className="text-sm sm:text-base text-accent-red hover:text-red-500 font-semibold"
          >
            Unterzone erstellen
          </button>
        </div>
      )}

      {/* Device Tree Modal */}
      <DeviceTreeModal
        isOpen={isDeviceModalOpen}
        onClose={() => setIsDeviceModalOpen(false)}
        onConfirm={handleAssignDevices}
        zoneId={zone.zone_id}
      />

      {/* Device Detail Modal */}
      <DeviceDetailModal
        device={selectedDevice}
        isOpen={isDetailModalOpen}
        onClose={() => setIsDetailModalOpen(false)}
      />
    </div>
  );
}
