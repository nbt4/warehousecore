import { MapPin, Package, Ruler, Weight, X, Lightbulb, Layers, Tag, Download, Plus, Trash2 } from 'lucide-react';
import type { CaseDetail, CaseDevice } from '../lib/api';
import { formatStatus, getStatusColor } from '../lib/utils';
import { useMemo } from 'react';
import { ModalPortal } from './ModalPortal';
import { useBlockBodyScroll } from '../hooks/useBlockBodyScroll';

interface CaseDetailModalProps {
  caseInfo: CaseDetail | null;
  devices: CaseDevice[];
  isOpen: boolean;
  loading?: boolean;
  onClose: () => void;
  onOpenZone: (device: { zone_id?: number; zone_code?: string }) => void;
  onOpenDevice: (deviceId: string) => void;
  onLocateDevice: (device: CaseDevice) => void;
  onAddDevices: () => void;
  onRemoveDevice: (deviceId: string) => void;
  onRefresh: () => void;
}

export function CaseDetailModal({
  caseInfo,
  devices,
  isOpen,
  loading = false,
  onClose,
  onOpenZone,
  onOpenDevice,
  onLocateDevice,
  onAddDevices,
  onRemoveDevice,
}: CaseDetailModalProps) {
  // Block body scroll when modal is open
  useBlockBodyScroll(isOpen);

  // Cache-busting for label image - regenerates URL when label_path changes
  const labelUrl = useMemo(() => {
    if (!caseInfo?.label_path) return null;
    return `${caseInfo.label_path}?t=${Date.now()}`;
  }, [caseInfo?.label_path]);
  const labelIsPDF = caseInfo?.label_path?.toLowerCase().endsWith('.pdf') ?? false;

  if (!isOpen) return null;

  const formatDimension = (value?: number) => {
    if (value === undefined || Number.isNaN(value)) {
      return '–';
    }
    return `${value.toFixed(1)} cm`;
  };

  const formatWeight = (value?: number) => {
    if (value === undefined || Number.isNaN(value)) {
      return '–';
    }
    return `${value.toFixed(1)} kg`;
  };

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
        <div className="glass-dark w-full max-w-5xl rounded-2xl border border-white/10 shadow-2xl flex flex-col max-h-[85vh]">
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div>
            <h2 className="text-2xl font-bold text-white flex items-center gap-2">
              <Package className="w-6 h-6 text-accent-red" />
              {caseInfo?.name ?? 'Case'}
            </h2>
            <p className="text-sm text-gray-400 mt-1">
              Case #{caseInfo?.case_id ?? '—'} • {devices.length} Gerät{devices.length === 1 ? '' : 'e'}
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-2 rounded-lg hover:bg-white/10 transition-colors"
            aria-label="Close case details"
          >
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        {caseInfo && (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 p-6 border-b border-white/10 bg-white/[0.02]">
            <div className="glass rounded-xl p-4 border border-white/10">
              <p className="text-xs text-gray-400 uppercase tracking-wide mb-1">Status</p>
              <p className={`inline-flex items-center gap-2 text-sm font-semibold ${getStatusColor(caseInfo.status)}`}>
                <span className="px-2 py-1 rounded-full bg-white/10 uppercase tracking-wide text-xs">
                  {formatStatus(caseInfo.status)}
                </span>
              </p>
              {caseInfo.description && (
                <p className="mt-3 text-sm text-gray-300">{caseInfo.description}</p>
              )}
            </div>

            <div className="glass rounded-xl p-4 border border-white/10">
              <p className="text-xs text-gray-400 uppercase tracking-wide mb-2">Abmessungen</p>
              <div className="flex items-center gap-2 text-sm text-white">
                <Ruler className="w-4 h-4 text-gray-400" />
                <span>{formatDimension(caseInfo.width)}</span>
                <span>×</span>
                <span>{formatDimension(caseInfo.height)}</span>
                <span>×</span>
                <span>{formatDimension(caseInfo.depth)}</span>
              </div>
              <div className="flex items-center gap-2 text-sm text-white mt-3">
                <Weight className="w-4 h-4 text-gray-400" />
                <span>{formatWeight(caseInfo.weight)}</span>
              </div>
            </div>

            <div className="glass rounded-xl p-4 border border-white/10">
              <p className="text-xs text-gray-400 uppercase tracking-wide mb-2">Zuordnung</p>
              <div className="flex flex-col gap-2 text-sm text-white">
                <div className="flex items-center gap-2">
                  <Layers className="w-4 h-4 text-gray-400" />
                  <span>{caseInfo.device_count} Gerät{caseInfo.device_count === 1 ? '' : 'e'}</span>
                </div>
                <div className="flex items-center gap-2">
                  <MapPin className="w-4 h-4 text-gray-400" />
                  <span>{caseInfo.zone_name ?? 'Keine Zone'}</span>
                  {caseInfo.zone_code && (
                    <span className="font-mono text-xs text-gray-500">{caseInfo.zone_code}</span>
                  )}
                </div>
              </div>
            </div>

            <div className="glass rounded-xl p-4 border border-white/10">
              <p className="text-xs text-gray-400 uppercase tracking-wide mb-2 flex items-center gap-1">
                <Tag className="w-3 h-3" />
                Label
              </p>
              {labelUrl ? (
                <div className="flex flex-col gap-2">
                  {labelIsPDF ? (
                    <object
                      data={labelUrl}
                      type="application/pdf"
                      aria-label={`Label für Case ${caseInfo.case_id}`}
                      className="h-56 w-full rounded border border-white/10 bg-white shadow-sm"
                    />
                  ) : (
                    <img
                      src={labelUrl}
                      alt={`Label für Case ${caseInfo.case_id}`}
                      className="w-full h-auto rounded border border-white/10 shadow-sm"
                    />
                  )}
                  <a
                    href={labelUrl}
                    download={`CASE-${caseInfo.case_id}_label.${labelIsPDF ? 'pdf' : 'png'}`}
                    className="flex items-center justify-center gap-1 px-2 py-1 text-xs font-semibold rounded-lg bg-white/10 hover:bg-white/20 transition-colors text-white"
                  >
                    <Download className="w-3 h-3" />
                    Download
                  </a>
                </div>
              ) : (
                <p className="text-sm text-gray-500 italic">Kein Label vorhanden</p>
              )}
            </div>
          </div>
        )}

        <div className="flex-1 overflow-y-auto p-6">
          {/* Add Devices Button */}
          <div className="mb-4">
            <button
              onClick={onAddDevices}
              className="flex items-center gap-2 px-4 py-2 rounded-xl font-semibold text-white bg-gradient-to-r from-accent-red to-red-700 hover:shadow-lg hover:shadow-accent-red/50 hover:scale-105 active:scale-95 transition-all"
            >
              <Plus className="w-4 h-4" />
              Geräte hinzufügen
            </button>
          </div>

          {loading ? (
            <div className="flex flex-col items-center justify-center h-60 gap-3 text-gray-400">
              <div className="animate-spin rounded-full h-10 w-10 border-b-2 border-accent-red" />
              <p className="text-sm">Case-Daten werden geladen…</p>
            </div>
          ) : devices.length === 0 ? (
            <div className="text-center text-gray-400 text-sm py-12">
              Keine Geräte in diesem Case.
            </div>
          ) : (
            <div className="space-y-3">
              {devices.map((device) => (
                <div
                  key={device.device_id}
                  className="glass rounded-xl border border-white/10 p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-semibold text-white text-sm sm:text-base font-mono">
                        {device.device_id}
                      </span>
                      <span
                        className={`text-[10px] sm:text-xs font-semibold px-2 py-1 rounded-full bg-white/10 uppercase tracking-wide ${getStatusColor(device.status)}`}
                      >
                        {formatStatus(device.status)}
                      </span>
                      {device.product_name && (
                        <span className="text-xs text-gray-400">{device.product_name}</span>
                      )}
                    </div>
                    <div className="flex flex-wrap items-center gap-3 text-xs text-gray-400 mt-1">
                      {device.zone_name && (
                        <span className="flex items-center gap-1">
                          <MapPin className="w-3 h-3" />
                          {device.zone_name}
                        </span>
                      )}
                      {device.zone_code && (
                        <span className="font-mono text-gray-500">{device.zone_code}</span>
                      )}
                      {device.serial_number && (
                        <span className="text-gray-500">SN: {device.serial_number}</span>
                      )}
                      {device.barcode && (
                        <span className="text-gray-500 font-mono">Barcode: {device.barcode}</span>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-2 flex-wrap">
                    <button
                      onClick={() => onOpenDevice(device.device_id)}
                      className="px-3 py-1.5 rounded-lg text-xs sm:text-sm font-semibold bg-white/10 hover:bg-white/20 transition-colors text-white"
                    >
                      Details
                    </button>
                    <button
                      onClick={() => onLocateDevice(device)}
                      disabled={!device.zone_code}
                      className="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs sm:text-sm font-semibold bg-white/10 hover:bg-white/20 transition-colors disabled:opacity-50"
                    >
                      <Lightbulb className="w-4 h-4 text-yellow-300" /> Licht
                    </button>
                    <button
                      onClick={() => onOpenZone({ zone_id: device.zone_id, zone_code: device.zone_code })}
                      disabled={!device.zone_id && !device.zone_code}
                      className="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs sm:text-sm font-semibold bg-accent-red/80 hover:bg-accent-red transition-colors text-white disabled:opacity-50"
                    >
                      <MapPin className="w-4 h-4" /> Zone
                    </button>
                    <button
                      onClick={() => {
                        if (confirm(`Gerät ${device.device_id} aus diesem Case entfernen?`)) {
                          onRemoveDevice(device.device_id);
                        }
                      }}
                      className="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs sm:text-sm font-semibold bg-red-600/80 hover:bg-red-600 transition-colors text-white"
                      title="Aus Case entfernen"
                    >
                      <Trash2 className="w-4 h-4" />
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
