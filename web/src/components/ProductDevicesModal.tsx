import { Package, MapPin, Lightbulb } from 'lucide-react';
import type { Device } from '../lib/api';
import { formatStatus, getStatusColor } from '../lib/utils';
import { ModalPortal } from './ModalPortal';
import { useBlockBodyScroll } from '../hooks/useBlockBodyScroll';

interface ProductDevicesModalProps {
  productName: string;
  devices: Device[];
  isOpen: boolean;
  onClose: () => void;
  onLocate: (device: Device) => void;
  onOpenZone: (device: Device) => void;
  onOpenDevice: (device: Device) => void;
  loading?: boolean;
  locatingId?: string | null;
}

export function ProductDevicesModal({
  productName,
  devices,
  isOpen,
  onClose,
  onLocate,
  onOpenZone,
  onOpenDevice,
  loading = false,
  locatingId = null,
}: ProductDevicesModalProps) {
  // Block body scroll when modal is open
  useBlockBodyScroll(isOpen);

  if (!isOpen) return null;

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
        <div className="glass-dark w-full max-w-4xl rounded-2xl shadow-2xl border border-white/10 flex flex-col max-h-[85vh]">
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div>
            <h2 className="text-2xl font-bold text-white flex items-center gap-2">
              <Package className="w-6 h-6 text-accent-red" />
              {productName}
            </h2>
            <p className="text-sm text-gray-400 mt-1">
              {devices.length} Gerät{devices.length === 1 ? '' : 'e'}
            </p>
          </div>
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-lg text-sm font-semibold bg-white/10 text-white hover:bg-white/20 transition-colors"
          >
            Schließen
          </button>
        </div>
        <div className="overflow-y-auto">
          {devices.length === 0 ? (
            <div className="p-6 text-center text-gray-400 text-sm">
              Keine Geräte in diesem Produkt.
            </div>
          ) : (
            <div className="p-6 space-y-3">
              {devices.map((device) => (
                <div
                  key={device.device_id}
                  onClick={() => onOpenDevice(device)}
                  className="glass rounded-lg p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 border border-white/10 cursor-pointer hover:bg-white/10 transition-colors"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-semibold text-white text-sm sm:text-base truncate">
                        {device.device_id}
                      </span>
                      {device.status && (
                        <span
                          className={`text-[10px] sm:text-xs font-semibold px-2 py-1 rounded-full bg-white/10 uppercase tracking-wide ${getStatusColor(device.status)}`}
                        >
                          {formatStatus(device.status)}
                        </span>
                      )}
                    </div>
                    <div className="flex flex-wrap items-center gap-2 text-xs sm:text-sm text-gray-400 mt-1">
                      {device.zone_name && (
                        <span className="flex items-center gap-1">
                          <MapPin className="w-3 h-3" />
                          {device.zone_name}
                        </span>
                      )}
                      {device.zone_code && (
                        <span className="text-gray-500 font-mono">({device.zone_code})</span>
                      )}
                      {device.case_name && (
                        <span className="text-gray-400">📦 {device.case_name}</span>
                      )}
                      {device.job_number && (
                        <span className="text-gray-400">🔧 Job #{device.job_number}</span>
                      )}
                      {device.serial_number && (
                        <span className="text-gray-500">SN: {device.serial_number}</span>
                      )}
                      {device.barcode && (
                        <span className="text-gray-500">Barcode: {device.barcode}</span>
                      )}
                      {device.condition_rating !== undefined && device.condition_rating > 0 && (
                        <span className="text-gray-400">Zustand: {device.condition_rating}/10</span>
                      )}
                      {device.usage_hours !== undefined && device.usage_hours > 0 && (
                        <span className="text-gray-400">{device.usage_hours}h Betrieb</span>
                      )}
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        onLocate(device);
                      }}
                      disabled={loading || locatingId === device.device_id}
                      className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-semibold bg-white/10 hover:bg-white/20 transition-colors disabled:opacity-50"
                    >
                      <Lightbulb className="w-4 h-4 text-yellow-300" /> Fach aufleuchten
                    </button>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        onOpenZone(device);
                      }}
                      disabled={loading}
                      className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-semibold bg-accent-red/80 hover:bg-accent-red transition-colors text-white disabled:opacity-50"
                    >
                      <MapPin className="w-4 h-4" /> Zone öffnen
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
        </div>
      </div>
    </ModalPortal>
  );
}
