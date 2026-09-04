import { useEffect, useState } from 'react';
import { X, ChevronRight, Package } from 'lucide-react';
import { ModalPortal } from './ModalPortal';
import { useBlockBodyScroll } from '../hooks/useBlockBodyScroll';
import { toast } from '../lib/toast';
import { formatStatus } from '../lib/utils';
import { appPath } from '../lib/app-paths';

interface Device {
  device_id: string;
  product_name: string;
  status: string;
  barcode?: string;
  serial_number?: string;
  zone_id?: number;
  zone_code?: string;
}

interface Subbiercategory {
  id: number;
  name: string;
  devices: Device[];
  device_count: number;
}

interface Subcategory {
  id: number;
  name: string;
  subbiercategories: Subbiercategory[];
  direct_devices: Device[];
  device_count: number;
}

interface Category {
  id: number;
  name: string;
  subcategories: Subcategory[];
  direct_devices: Device[];
  device_count: number;
}

interface DeviceTreeModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (selectedDevices: string[]) => void;
  zoneId: number;
}

export function DeviceTreeModal({ isOpen, onClose, onConfirm, zoneId }: DeviceTreeModalProps) {
  const [treeData, setTreeData] = useState<Category[]>([]);
  const [selectedDevices, setSelectedDevices] = useState<Set<string>>(new Set());
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);

  // Block body scroll when modal is open
  useBlockBodyScroll(isOpen);

  useEffect(() => {
    if (isOpen) {
      loadDeviceTree();
    }
  }, [isOpen]);

  const loadDeviceTree = async () => {
    setLoading(true);
    try {
      const response = await fetch(appPath('/api/v1/devices/tree'));
      const data = await response.json();
      setTreeData(data.treeData || []);
    } catch (error) {
      toast.error('Failed to load device tree:' + " " + String(error));
    } finally {
      setLoading(false);
    }
  };

  const toggleNode = (nodeId: string) => {
    const newExpanded = new Set(expandedNodes);
    if (newExpanded.has(nodeId)) {
      newExpanded.delete(nodeId);
    } else {
      newExpanded.add(nodeId);
    }
    setExpandedNodes(newExpanded);
  };

  const toggleDeviceSelection = (deviceId: string) => {
    const newSelected = new Set(selectedDevices);
    if (newSelected.has(deviceId)) {
      newSelected.delete(deviceId);
    } else {
      newSelected.add(deviceId);
    }
    setSelectedDevices(newSelected);
  };

  const handleConfirm = () => {
    onConfirm(Array.from(selectedDevices));
    setSelectedDevices(new Set());
    onClose();
  };

  const handleClose = () => {
    setSelectedDevices(new Set());
    onClose();
  };

  if (!isOpen) return null;

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
        <div className="glass-dark rounded-2xl w-full max-w-4xl flex flex-col shadow-2xl max-h-[85vh]">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <h2 className="text-2xl font-bold text-white flex items-center gap-2">
            <Package className="w-6 h-6 text-accent-red" />
            Geräte hinzufügen
          </h2>
          <button
            onClick={handleClose}
            className="p-2 hover:bg-white/10 rounded-lg transition-colors"
          >
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        {/* Selected Devices Count */}
        <div className="px-6 py-3 bg-white/5 border-b border-white/10">
          <p className="text-sm text-gray-400">
            {selectedDevices.size} Gerät{selectedDevices.size !== 1 ? 'e' : ''} ausgewählt
          </p>
        </div>

        {/* Device Tree */}
        <div className="flex-1 overflow-y-auto p-6">
          {loading ? (
            <div className="flex items-center justify-center h-64">
              <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-accent-red"></div>
            </div>
          ) : treeData.length === 0 ? (
            <div className="text-center py-12 text-gray-400">
              Keine Geräte gefunden
            </div>
          ) : (
            <div className="space-y-2">
              {treeData.map((category) => (
                <CategoryNode
                  key={category.id}
                  category={category}
                  expandedNodes={expandedNodes}
                  selectedDevices={selectedDevices}
                  onToggleNode={toggleNode}
                  onToggleDevice={toggleDeviceSelection}
                  currentZoneId={zoneId}
                />
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 p-6 border-t border-white/10">
          <button
            onClick={handleClose}
            className="px-6 py-2.5 glass hover:bg-white/10 text-white font-semibold rounded-xl transition-all"
          >
            Abbrechen
          </button>
          <button
            onClick={handleConfirm}
            disabled={selectedDevices.size === 0}
            className="px-6 py-2.5 bg-gradient-to-r from-accent-red to-red-700 text-white font-semibold rounded-xl hover:shadow-lg hover:shadow-accent-red/50 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {selectedDevices.size} Gerät{selectedDevices.size !== 1 ? 'e' : ''} hinzufügen
          </button>
        </div>
        </div>
      </div>
    </ModalPortal>
  );
}

// Category Node Component
function CategoryNode({
  category,
  expandedNodes,
  selectedDevices,
  onToggleNode,
  onToggleDevice,
  currentZoneId,
}: {
  category: Category;
  expandedNodes: Set<string>;
  selectedDevices: Set<string>;
  onToggleNode: (id: string) => void;
  onToggleDevice: (id: string) => void;
  currentZoneId: number;
}) {
  const nodeId = `cat-${category.id}`;
  const isExpanded = expandedNodes.has(nodeId);

  return (
    <div className="border border-white/10 rounded-lg bg-surface-1">
      <div
        className="flex items-center gap-2 p-3 cursor-pointer hover:bg-white/5 transition-colors rounded-lg"
        onClick={() => onToggleNode(nodeId)}
      >
        <ChevronRight
          className={`w-4 h-4 text-gray-400 transition-transform ${
            isExpanded ? 'rotate-90' : ''
          }`}
        />
        <span className="text-lg">📁</span>
        <span className="font-semibold text-white">{category.name}</span>
        <span className="text-sm text-gray-400">({category.device_count} Geräte)</span>
      </div>

      {isExpanded && (
        <div className="pl-6 pb-2 pr-2">
          {category.subcategories.map((subcategory) => (
            <SubcategoryNode
              key={subcategory.id}
              subcategory={subcategory}
              expandedNodes={expandedNodes}
              selectedDevices={selectedDevices}
              onToggleNode={onToggleNode}
              onToggleDevice={onToggleDevice}
              currentZoneId={currentZoneId}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// Subcategory Node Component
function SubcategoryNode({
  subcategory,
  expandedNodes,
  selectedDevices,
  onToggleNode,
  onToggleDevice,
  currentZoneId,
}: {
  subcategory: Subcategory;
  expandedNodes: Set<string>;
  selectedDevices: Set<string>;
  onToggleNode: (id: string) => void;
  onToggleDevice: (id: string) => void;
  currentZoneId: number;
}) {
  const nodeId = `subcat-${subcategory.id}`;
  const isExpanded = expandedNodes.has(nodeId);

  return (
    <div className="mt-2">
      <div
        className="flex items-center gap-2 p-2 cursor-pointer hover:bg-white/5 transition-colors rounded-lg"
        onClick={() => onToggleNode(nodeId)}
      >
        <ChevronRight
          className={`w-3.5 h-3.5 text-gray-400 transition-transform ${
            isExpanded ? 'rotate-90' : ''
          }`}
        />
        <span className="text-base">📂</span>
        <span className="font-medium text-white text-sm">{subcategory.name}</span>
        <span className="text-xs text-gray-400">({subcategory.device_count})</span>
      </div>

      {isExpanded && (
        <div className="pl-6 pt-1">
          {subcategory.subbiercategories.map((subbiercategory) => (
            <SubbiercategoryNode
              key={subbiercategory.id}
              subbiercategory={subbiercategory}
              expandedNodes={expandedNodes}
              selectedDevices={selectedDevices}
              onToggleNode={onToggleNode}
              onToggleDevice={onToggleDevice}
              currentZoneId={currentZoneId}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// Subbiercategory Node Component
function SubbiercategoryNode({
  subbiercategory,
  expandedNodes,
  selectedDevices,
  onToggleNode,
  onToggleDevice,
  currentZoneId,
}: {
  subbiercategory: Subbiercategory;
  expandedNodes: Set<string>;
  selectedDevices: Set<string>;
  onToggleNode: (id: string) => void;
  onToggleDevice: (id: string) => void;
  currentZoneId: number;
}) {
  const nodeId = `subbier-${subbiercategory.id}`;
  const isExpanded = expandedNodes.has(nodeId);

  return (
    <div className="mt-2">
      <div
        className="flex items-center gap-2 p-2 cursor-pointer hover:bg-white/5 transition-colors rounded-lg"
        onClick={() => onToggleNode(nodeId)}
      >
        <ChevronRight
          className={`w-3 h-3 text-gray-400 transition-transform ${
            isExpanded ? 'rotate-90' : ''
          }`}
        />
        <span className="text-sm">📄</span>
        <span className="font-medium text-white text-sm">{subbiercategory.name}</span>
        <span className="text-xs text-gray-400">({subbiercategory.device_count})</span>
      </div>

      {isExpanded && (
        <div className="pl-6 pt-1 space-y-1">
          {subbiercategory.devices.map((device) => (
            <DeviceNode
              key={device.device_id}
              device={device}
              isSelected={selectedDevices.has(device.device_id)}
              onToggle={onToggleDevice}
              isInCurrentZone={device.zone_id === currentZoneId}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// Device Node Component
function DeviceNode({
  device,
  isSelected,
  onToggle,
  isInCurrentZone,
}: {
  device: Device;
  isSelected: boolean;
  onToggle: (id: string) => void;
  isInCurrentZone: boolean;
}) {
  const statusColors: Record<string, string> = {
    in_storage: 'text-green-400',
    on_job: 'text-yellow-400',
    rented: 'text-yellow-400',
    return_pending: 'text-yellow-400',
    location_unknown: 'text-orange-400',
    defective: 'text-red-400',
    in_transit: 'text-blue-400',
  };

  return (
    <div
      className={`flex items-center justify-between p-2 rounded-lg cursor-pointer transition-all ${
        isSelected
          ? 'bg-accent-red/20 border border-accent-red'
          : 'hover:bg-white/5 border border-transparent'
      } ${isInCurrentZone ? 'opacity-50 cursor-not-allowed' : ''}`}
      onClick={() => !isInCurrentZone && onToggle(device.device_id)}
    >
      <div className="flex items-center gap-2 flex-1 min-w-0">
        <Package className={`w-4 h-4 flex-shrink-0 ${isSelected ? 'text-accent-red' : 'text-gray-400'}`} />
        <div className="flex-1 min-w-0">
          <div className="font-medium text-white text-sm truncate">{device.product_name}</div>
          <div className="text-xs text-gray-400 font-mono">
            {device.device_id}
            {device.serial_number && <span> • {device.serial_number}</span>}
          </div>
        </div>
      </div>
      <div className="flex items-center gap-2 flex-shrink-0">
        {isInCurrentZone && (
          <span className="text-xs text-gray-500 italic">Bereits in dieser Zone</span>
        )}
        {device.zone_code && !isInCurrentZone && (
          <span className="text-xs text-gray-500">📍 {device.zone_code}</span>
        )}
        <span className={`text-xs font-medium ${statusColors[device.status] || 'text-gray-400'}`}>
          {formatStatus(device.status)}
        </span>
      </div>
    </div>
  );
}
