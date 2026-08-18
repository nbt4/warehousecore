import { X, Package, MapPin, Barcode, Hash, Activity, Wrench, Lightbulb, LightbulbOff, Tag, Download } from 'lucide-react';
import { ledApi } from '../lib/api';
import type { Device } from '../lib/api';
import { useState, useEffect, useMemo } from 'react';
import { ModalPortal } from './ModalPortal';
import { useBlockBodyScroll } from '../hooks/useBlockBodyScroll';
import { toast } from '../lib/toast';
import { formatStatus } from '../lib/utils';

interface DeviceDetailModalProps {
  device: Device | null;
  isOpen: boolean;
  onClose: () => void;
}

export function DeviceDetailModal({ device, isOpen, onClose }: DeviceDetailModalProps) {
  const [locating, setLocating] = useState(false);
  const [locateMessage, setLocateMessage] = useState<string | null>(null);
  const [ledActive, setLedActive] = useState(false);

  // Block body scroll when modal is open
  useBlockBodyScroll(isOpen);

  // Cleanup LEDs when modal closes
  useEffect(() => {
    if (!isOpen && ledActive) {
      ledApi.clear().catch(err => toast.error('Failed to clear LEDs:' + " " + String(err)));
      setLedActive(false);
    }
  }, [isOpen, ledActive]);

  // Handle ESC key to close modal
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [isOpen, onClose]);

  // Cache-busting for label image - regenerates URL when label_path changes
  const labelUrl = useMemo(() => {
    if (!device?.label_path) return null;
    return `${device.label_path}?t=${Date.now()}`;
  }, [device?.label_path]);
  const labelIsPDF = device?.label_path?.toLowerCase().endsWith('.pdf') ?? false;

  if (!isOpen || !device) return null;

  const handleLocate = async () => {
    if (!device.zone_code) {
      setLocateMessage('Gerät hat keinen Lagerort');
      setTimeout(() => setLocateMessage(null), 3000);
      return;
    }

    setLocating(true);
    setLocateMessage(null);

    try {
      await ledApi.locateBin(device.zone_code);
      setLedActive(true);
      setLocateMessage(`✓ Fach ${device.zone_code} leuchtet jetzt orange`);
      setTimeout(() => setLocateMessage(null), 5000);
    } catch (error: unknown) {
      toast.error('Failed to locate bin:' + " " + String(error));
      setLocateMessage('Fehler beim Beleuchten des Fachs');
      setTimeout(() => setLocateMessage(null), 3000);
    } finally {
      setLocating(false);
    }
  };

  const handleClearLED = async () => {
    setLocating(true);
    setLocateMessage(null);

    try {
      await ledApi.clear();
      setLedActive(false);
      setLocateMessage('✓ LEDs ausgeschaltet');
      setTimeout(() => setLocateMessage(null), 3000);
    } catch (error: unknown) {
      toast.error('Failed to clear LEDs:' + " " + String(error));
      setLocateMessage('Fehler beim Ausschalten');
      setTimeout(() => setLocateMessage(null), 3000);
    } finally {
      setLocating(false);
    }
  };

  const statusColors: Record<string, { bg: string; text: string }> = {
    in_storage: { bg: 'bg-green-500/20', text: 'text-green-400' },
    on_job: { bg: 'bg-accent-red/20', text: 'text-accent-red' },
    rented: { bg: 'bg-yellow-500/20', text: 'text-yellow-400' },
    return_pending: { bg: 'bg-yellow-500/20', text: 'text-yellow-400' },
    location_unknown: { bg: 'bg-orange-500/20', text: 'text-orange-400' },
    defective: { bg: 'bg-red-500/20', text: 'text-red-400' },
    in_transit: { bg: 'bg-white/10', text: 'text-gray-200' },
  };

  const statusColor = statusColors[device.status] || { bg: 'bg-gray-500/20', text: 'text-gray-400' };

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
        <div className="glass-dark rounded-2xl w-full max-w-2xl shadow-2xl max-h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <h2 className="text-2xl font-bold text-white flex items-center gap-2">
            <Package className="w-6 h-6 text-accent-red" />
            Geräte-Details
          </h2>
          <button
            onClick={onClose}
            className="p-2 hover:bg-white/10 rounded-lg transition-colors"
          >
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 space-y-6">
          {/* Product Name */}
          <div>
            <h3 className="text-3xl font-bold text-white mb-2">
              {device.product_name || 'Unbekanntes Gerät'}
            </h3>
            <div className="flex items-center gap-2">
              <span className={`px-3 py-1 rounded-full text-sm font-semibold ${statusColor.bg} ${statusColor.text}`}>
                {formatStatus(device.status)}
              </span>
            </div>
          </div>

          {/* Details Grid */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {/* Device ID */}
            <div className="glass rounded-xl p-4">
              <div className="flex items-center gap-3">
                <Hash className="w-5 h-5 text-gray-400" />
                <div>
                  <p className="text-xs text-gray-400">Geräte-ID</p>
                  <p className="text-white font-mono font-semibold">{device.device_id}</p>
                </div>
              </div>
            </div>

            {/* Serial Number */}
            {device.serial_number && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-3">
                  <Hash className="w-5 h-5 text-gray-400" />
                  <div>
                    <p className="text-xs text-gray-400">Seriennummer</p>
                    <p className="text-white font-mono font-semibold">{device.serial_number}</p>
                  </div>
                </div>
              </div>
            )}

            {/* Barcode */}
            {device.barcode && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-3">
                  <Barcode className="w-5 h-5 text-gray-400" />
                  <div>
                    <p className="text-xs text-gray-400">Barcode</p>
                    <p className="text-white font-mono font-semibold">{device.barcode}</p>
                  </div>
                </div>
              </div>
            )}

            {/* QR Code */}
            {device.qr_code && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-3">
                  <Barcode className="w-5 h-5 text-gray-400" />
                  <div>
                    <p className="text-xs text-gray-400">QR-Code</p>
                    <p className="text-white font-mono font-semibold">{device.qr_code}</p>
                  </div>
                </div>
              </div>
            )}

            {/* Location */}
            {device.zone_name && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-3">
                  <MapPin className="w-5 h-5 text-gray-400" />
                  <div>
                    <p className="text-xs text-gray-400">Lagerort</p>
                    <p className="text-white font-semibold">{device.zone_name}</p>
                    {device.zone_code && (
                      <p className="text-xs text-gray-500 font-mono">{device.zone_code}</p>
                    )}
                  </div>
                </div>
              </div>
            )}

            {/* Case */}
            {device.case_name && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-3">
                  <Package className="w-5 h-5 text-gray-400" />
                  <div>
                    <p className="text-xs text-gray-400">Case</p>
                    <p className="text-white font-semibold">{device.case_name}</p>
                  </div>
                </div>
              </div>
            )}

            {/* Job */}
            {device.job_number && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-3">
                  <Activity className="w-5 h-5 text-gray-400" />
                  <div>
                    <p className="text-xs text-gray-400">Job</p>
                    <p className="text-white font-semibold">#{device.job_number}</p>
                  </div>
                </div>
              </div>
            )}

            {/* Condition */}
            <div className="glass rounded-xl p-4">
              <div className="flex items-center gap-3">
                <Wrench className="w-5 h-5 text-gray-400" />
                <div>
                  <p className="text-xs text-gray-400">Zustand</p>
                  <p className="text-white font-semibold">{device.condition_rating}/10</p>
                </div>
              </div>
            </div>

            {/* Usage Hours */}
            <div className="glass rounded-xl p-4">
              <div className="flex items-center gap-3">
                <Activity className="w-5 h-5 text-gray-400" />
                <div>
                  <p className="text-xs text-gray-400">Betriebsstunden</p>
                  <p className="text-white font-semibold">{device.usage_hours}h</p>
                </div>
              </div>
            </div>
          </div>

          {/* LED Control Buttons */}
          {device.zone_code && (
            <div className="pt-4 border-t border-white/10">
              <div className="flex gap-3">
                {/* Locate Button */}
                <button
                  onClick={handleLocate}
                  disabled={locating || ledActive}
                  className={`flex-1 py-3 px-4 rounded-xl font-semibold text-white transition-all flex items-center justify-center gap-2 ${
                    locating || ledActive
                      ? 'bg-gray-600 cursor-not-allowed opacity-50'
                      : 'bg-gradient-to-r from-orange-600 to-orange-700 hover:shadow-lg hover:shadow-orange-500/50 hover:scale-105 active:scale-95'
                  }`}
                >
                  <Lightbulb className="w-5 h-5" />
                  <span>{locating ? 'Beleuchte...' : 'Fach beleuchten'}</span>
                </button>

                {/* Clear LEDs Button */}
                {ledActive && (
                  <button
                    onClick={handleClearLED}
                    disabled={locating}
                    className={`flex-1 py-3 px-4 rounded-xl font-semibold text-white transition-all flex items-center justify-center gap-2 ${
                      locating
                        ? 'bg-gray-600 cursor-not-allowed opacity-50'
                        : 'bg-gradient-to-r from-red-600 to-red-700 hover:shadow-lg hover:shadow-red-500/50 hover:scale-105 active:scale-95'
                    }`}
                  >
                    <LightbulbOff className="w-5 h-5" />
                    <span>Ausschalten</span>
                  </button>
                )}
              </div>

              {/* Locate Message */}
              {locateMessage && (
                <div className={`mt-3 p-3 rounded-lg text-center text-sm font-semibold ${
                  locateMessage.includes('✓')
                    ? 'bg-green-500/20 text-green-400'
                    : 'bg-red-500/20 text-red-400'
                }`}>
                  {locateMessage}
                </div>
              )}
            </div>
          )}

          {/* Label Preview Section */}
          {labelUrl && (
            <div className="pt-4 border-t border-white/10">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-lg font-semibold text-white flex items-center gap-2">
                  <Tag className="w-5 h-5 text-accent-red" />
                  Geräte-Label
                </h3>
                <a
                  href={labelUrl}
                  download={`${device.device_id}_label.${labelIsPDF ? 'pdf' : 'png'}`}
                  className="flex items-center gap-2 rounded-xl bg-accent-red px-4 py-2 font-semibold text-white transition-all hover:scale-105 hover:bg-accent-red-hover hover:shadow-lg hover:shadow-accent-red/30 active:scale-95"
                >
                  <Download className="w-4 h-4" />
                  Herunterladen
                </a>
              </div>
              <div className="flex justify-center p-4 bg-black/20 rounded-xl">
                {labelIsPDF ? (
                  <object
                    data={labelUrl}
                    type="application/pdf"
                    aria-label={`Label für ${device.device_id}`}
                    className="h-64 w-full max-w-sm rounded border border-white/10 bg-white shadow-lg"
                  />
                ) : (
                  <img
                    src={labelUrl}
                    alt={`Label für ${device.device_id}`}
                    className="max-w-sm h-auto border border-white/10 rounded shadow-lg"
                  />
                )}
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 p-6 border-t border-white/10">
          <button
            onClick={onClose}
            className="px-6 py-2.5 glass hover:bg-white/10 text-white font-semibold rounded-xl transition-all"
          >
            Schließen
          </button>
        </div>
        </div>
      </div>
    </ModalPortal>
  );
}
