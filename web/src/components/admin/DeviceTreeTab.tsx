import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ChevronDown,
  ChevronRight,
  Lightbulb,
  Loader2,
  MapPin,
  Package,
  Search,
} from 'lucide-react';
import {
  devicesApi,
  ledApi,
  type Device,
  type DeviceTreeCategory,
  type DeviceTreeDevice,
  type DeviceTreeSubcategory,
  type DeviceTreeSubbiercategory,
} from '../../lib/api';
import { formatStatus, getStatusColor } from '../../lib/utils';
import { ProductDevicesModal } from '../ProductDevicesModal';
import { DeviceDetailModal } from '../DeviceDetailModal';
import { toast } from '../../lib/toast';

interface ActionMessage {
  type: 'success' | 'error';
  text: string;
}

export function DeviceTreeTab() {
  const navigate = useNavigate();
  const [devices, setDevices] = useState<Device[]>([]);
  const [treeData, setTreeData] = useState<DeviceTreeCategory[]>([]);
  const [treeLoading, setTreeLoading] = useState(true);
  const [deviceLoading, setDeviceLoading] = useState(true);
  const [treeError, setTreeError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set());
  const [selectedDevice, setSelectedDevice] = useState<Device | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [productModalOpen, setProductModalOpen] = useState(false);
  const [selectedProductName, setSelectedProductName] = useState<string>('');
  const [selectedProductDevices, setSelectedProductDevices] = useState<Device[]>([]);
  const [actionMessage, setActionMessage] = useState<ActionMessage | null>(null);

  useEffect(() => {
    loadDevices();
    loadDeviceTree();

    // Auto-refresh when page becomes visible (e.g., switching back from categories tab)
    const handleVisibilityChange = () => {
      if (!document.hidden) {
        loadDeviceTree();
        loadDevices();
      }
    };

    // Auto-refresh every 30 seconds when page is visible
    const interval = setInterval(() => {
      if (!document.hidden) {
        loadDeviceTree();
        loadDevices();
      }
    }, 30000);

    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      clearInterval(interval);
    };
  }, []);

  const deviceMap = useMemo(() => {
    const map = new Map<string, Device>();
    devices.forEach((device) => map.set(device.device_id, device));
    return map;
  }, [devices]);

  const { filteredTree, autoExpandKeys, totalDevices, totalProducts } = useMemo(() => {
    return filterTreeData(treeData, search);
  }, [treeData, search]);

  const isSearching = search.trim().length > 0;

  const effectiveExpandedNodes = useMemo(() => {
    const merged = new Set(expandedNodes);
    if (isSearching) {
      autoExpandKeys.forEach((key) => merged.add(key));
    }
    return merged;
  }, [expandedNodes, autoExpandKeys, isSearching]);

  const loadDevices = async () => {
    setDeviceLoading(true);
    try {
      const { data } = await devicesApi.getAll({ limit: 500 });
      setDevices(data);
    } catch (error) {
      toast.error('Failed to load devices:' + " " + String(error));
    } finally {
      setDeviceLoading(false);
    }
  };

  const loadDeviceTree = async () => {
    setTreeLoading(true);
    setTreeError(null);
    try {
      const { data } = await devicesApi.getTree();
      const tree = data.treeData || [];
      const sortedTree = sortDeviceTree(tree);
      setTreeData(sortedTree);
    } catch (error: any) {
      toast.error('Failed to load device tree:' + " " + String(error));
      setTreeError(error?.response?.data?.error || 'Gerätebaum konnte nicht geladen werden.');
    } finally {
      setTreeLoading(false);
    }
  };

  const toggleNode = (nodeId: string) => {
    setExpandedNodes((prev) => {
      const next = new Set(prev);
      if (next.has(nodeId)) {
        next.delete(nodeId);
      } else {
        next.add(nodeId);
      }
      return next;
    });
  };

  const handleLocateDevice = async (device: Pick<DeviceTreeDevice, 'zone_code' | 'device_id'>) => {
    if (!device.zone_code) {
      setActionMessage({
        type: 'error',
        text: 'Kein Fachcode vorhanden – Gerät nicht im Lager.',
      });
      clearActionMessage();
      return;
    }

    try {
      await ledApi.locateBin(device.zone_code);
      setActionMessage({
        type: 'success',
        text: `Fach ${device.zone_code} wird hervorgehoben.`,
      });
    } catch (error: any) {
      setActionMessage({
        type: 'error',
        text: 'LED-Befehl fehlgeschlagen: ' + (error.response?.data?.error || error.message || error.toString()),
      });
    } finally {
      clearActionMessage();
    }
  };

  const handleOpenZone = (device: Pick<DeviceTreeDevice, 'zone_id' | 'zone_code'>) => {
    if (device.zone_id) {
      navigate(`/zones/${device.zone_id}`);
    } else if (device.zone_code) {
      navigate(`/zones?parent=${device.zone_code}`);
    }
  };

  const handleOpenDevice = async (deviceId: string) => {
    let device = deviceMap.get(deviceId);

    if (!device) {
      try {
        const { data } = await devicesApi.getById(deviceId);
        device = data;
      } catch (error) {
        toast.error('Failed to load device:' + " " + String(error));
        setActionMessage({
          type: 'error',
          text: 'Gerätedetails konnten nicht geladen werden.',
        });
        clearActionMessage();
        return;
      }
    }

    setSelectedDevice(device);
    setDetailOpen(true);
  };

  const handleOpenProduct = (productName: string, treeDevices: DeviceTreeDevice[]) => {
    const enrichedDevices = treeDevices.map((treeDevice) => {
      const fullDevice = deviceMap.get(treeDevice.device_id);
      if (fullDevice) {
        return fullDevice;
      }
      return {
        device_id: treeDevice.device_id,
        product_name: treeDevice.product_name,
        status: treeDevice.status,
        barcode: treeDevice.barcode,
        serial_number: treeDevice.serial_number,
        zone_id: treeDevice.zone_id,
        zone_code: treeDevice.zone_code,
      } as Device;
    });

    setSelectedProductName(productName);
    setSelectedProductDevices(enrichedDevices);
    setProductModalOpen(true);
  };

  const clearActionMessage = () => {
    setTimeout(() => setActionMessage(null), 4000);
  };

  return (
    <div className="space-y-4 sm:space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-2xl sm:text-3xl font-bold text-white mb-1 sm:mb-2">Geräteverwaltung</h2>
          <p className="text-sm sm:text-base text-gray-400">
            {filteredTree.length} Kategorien • {totalProducts} Produktgruppen • {totalDevices}{' '}
            Geräte
          </p>
        </div>
        <button
          onClick={() => {
            loadDeviceTree();
            loadDevices();
          }}
          className="self-start sm:self-auto px-4 py-2 rounded-lg text-sm font-semibold bg-white/10 hover:bg-white/20 transition-colors text-white"
        >
          Aktualisieren
        </button>
      </div>

      <div className="glass rounded-xl sm:rounded-2xl p-3 sm:p-4">
        <div className="relative">
          <Search className="absolute left-3 sm:left-4 top-1/2 -translate-y-1/2 w-4 h-4 sm:w-5 sm:h-5 text-gray-400" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Geräte, Produkte oder Lagerorte suchen..."
            className="w-full pl-10 sm:pl-12 pr-3 sm:pr-4 py-2.5 sm:py-3 bg-white/10 backdrop-blur-md border border-white/20 rounded-lg sm:rounded-xl text-sm sm:text-base text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
          />
        </div>
      </div>

      {actionMessage && (
        <div
          className={`px-3 py-2 rounded-lg text-sm font-semibold ${
            actionMessage.type === 'success'
              ? 'bg-green-500/20 text-green-400'
              : 'bg-red-500/20 text-red-400'
          }`}
        >
          {actionMessage.text}
        </div>
      )}

      {treeError && (
        <div className="px-4 py-3 rounded-lg bg-red-500/10 border border-red-500/30 text-red-300 text-sm">
          {treeError}
        </div>
      )}

      <div className="glass-dark rounded-xl sm:rounded-2xl p-4 sm:p-6 border border-white/10 min-h-[320px]">
        {treeLoading ? (
          <div className="flex flex-col items-center justify-center h-64 gap-3 text-gray-400">
            <Loader2 className="h-10 w-10 animate-spin text-accent-red" />
            <p className="text-sm">Gerätebaum wird geladen...</p>
          </div>
        ) : (
          <DeviceTreeList
            categories={filteredTree}
            expandedNodes={effectiveExpandedNodes}
            onToggleNode={toggleNode}
            onOpenProduct={handleOpenProduct}
            onOpenDevice={handleOpenDevice}
            onLocateDevice={handleLocateDevice}
            onOpenZone={handleOpenZone}
            isLoading={deviceLoading}
            isFiltered={isSearching}
          />
        )}
      </div>

      <DeviceDetailModal device={selectedDevice} isOpen={detailOpen} onClose={() => setDetailOpen(false)} />

      <ProductDevicesModal
        isOpen={productModalOpen}
        onClose={() => setProductModalOpen(false)}
        productName={selectedProductName}
        devices={selectedProductDevices}
        onLocate={(device) => handleLocateDevice({ zone_code: device.zone_code, device_id: device.device_id })}
        onOpenZone={(device) => handleOpenZone({ zone_id: device.zone_id, zone_code: device.zone_code })}
        onOpenDevice={(device) => handleOpenDevice(device.device_id)}
      />
    </div>
  );
}

interface DeviceTreeListProps {
  categories: DeviceTreeCategory[];
  expandedNodes: Set<string>;
  onToggleNode: (id: string) => void;
  onOpenProduct: (productName: string, devices: DeviceTreeDevice[]) => void;
  onOpenDevice: (deviceId: string) => void;
  onLocateDevice: (device: DeviceTreeDevice) => void;
  onOpenZone: (device: DeviceTreeDevice) => void;
  isLoading: boolean;
  isFiltered: boolean;
}

function DeviceTreeList({
  categories,
  expandedNodes,
  onToggleNode,
  onOpenProduct,
  onOpenDevice,
  onLocateDevice,
  onOpenZone,
  isLoading,
  isFiltered,
}: DeviceTreeListProps) {
  if (categories.length === 0) {
    return (
      <div className="flex h-48 items-center justify-center text-center text-sm text-gray-400">
        {isFiltered ? 'Keine Geräte für diese Suche gefunden.' : 'Es sind keine Geräte im Lager hinterlegt.'}
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-gray-400">
        <Loader2 className="h-4 w-4 animate-spin" />
        Geräte werden aktualisiert...
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {categories.map((category) => (
        <CategoryNode
          key={category.id}
          category={category}
          expandedNodes={expandedNodes}
          onToggleNode={onToggleNode}
          onOpenProduct={onOpenProduct}
          onOpenDevice={onOpenDevice}
          onLocateDevice={onLocateDevice}
          onOpenZone={onOpenZone}
        />
      ))}
    </div>
  );
}

interface CategoryNodeProps {
  category: DeviceTreeCategory;
  expandedNodes: Set<string>;
  onToggleNode: (id: string) => void;
  onOpenProduct: (productName: string, devices: DeviceTreeDevice[]) => void;
  onOpenDevice: (deviceId: string) => void;
  onLocateDevice: (device: DeviceTreeDevice) => void;
  onOpenZone: (device: DeviceTreeDevice) => void;
}

function CategoryNode({
  category,
  expandedNodes,
  onToggleNode,
  onOpenProduct,
  onOpenDevice,
  onLocateDevice,
  onOpenZone,
}: CategoryNodeProps) {
  const nodeId = categoryNodeKey(category.id);
  const isExpanded = expandedNodes.has(nodeId);

  return (
    <div className="rounded-xl border border-white/10 bg-white/[0.03] shadow-sm">
      <button
        type="button"
        onClick={() => onToggleNode(nodeId)}
        className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left transition-colors hover:bg-white/5"
      >
        <div className="flex items-center gap-3">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-accent-red" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400" />
          )}
          <span className="text-sm sm:text-base font-semibold text-white">{category.name}</span>
        </div>
        <span className="text-xs font-semibold text-gray-400">{category.device_count} Geräte</span>
      </button>
      {isExpanded && (
        <div className="space-y-3 border-t border-white/5 p-4">
          {category.subcategories.map((subcategory) => (
            <SubcategoryNode
              key={subcategory.id}
              subcategory={subcategory}
              expandedNodes={expandedNodes}
              onToggleNode={onToggleNode}
              onOpenProduct={onOpenProduct}
              onOpenDevice={onOpenDevice}
              onLocateDevice={onLocateDevice}
              onOpenZone={onOpenZone}
            />
          ))}

          {category.direct_devices.length > 0 && (
            <div className="space-y-2">
              <p className="text-xs uppercase tracking-wide text-gray-500">
                Geräte ohne Unterkategorie
              </p>
              {category.direct_devices.map((device) => (
                <DeviceTreeItem
                  key={device.device_id}
                  device={device}
                  onOpenDevice={onOpenDevice}
                  onLocateDevice={onLocateDevice}
                  onOpenZone={onOpenZone}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

interface SubcategoryNodeProps {
  subcategory: DeviceTreeSubcategory;
  expandedNodes: Set<string>;
  onToggleNode: (id: string) => void;
  onOpenProduct: (productName: string, devices: DeviceTreeDevice[]) => void;
  onOpenDevice: (deviceId: string) => void;
  onLocateDevice: (device: DeviceTreeDevice) => void;
  onOpenZone: (device: DeviceTreeDevice) => void;
}

function SubcategoryNode({
  subcategory,
  expandedNodes,
  onToggleNode,
  onOpenProduct,
  onOpenDevice,
  onLocateDevice,
  onOpenZone,
}: SubcategoryNodeProps) {
  const nodeId = subcategoryNodeKey(subcategory.id);
  const isExpanded = expandedNodes.has(nodeId);

  return (
    <div className="rounded-lg border border-white/5 bg-white/[0.02]">
      <button
        type="button"
        onClick={() => onToggleNode(nodeId)}
        className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left transition-colors hover:bg-white/5"
      >
        <div className="flex items-center gap-3">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-accent-red" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400" />
          )}
          <span className="text-sm font-semibold text-white">{subcategory.name}</span>
        </div>
        <span className="text-xs font-semibold text-gray-400">{subcategory.device_count} Geräte</span>
      </button>

      {isExpanded && (
        <div className="space-y-2 border-t border-white/5 p-3 pl-6">
          {subcategory.subbiercategories.map((group) => (
            <SubbiercategoryNode
              key={group.id}
              group={group}
              expandedNodes={expandedNodes}
              onToggleNode={onToggleNode}
              onOpenProduct={onOpenProduct}
              onOpenDevice={onOpenDevice}
              onLocateDevice={onLocateDevice}
              onOpenZone={onOpenZone}
            />
          ))}

          {subcategory.direct_devices.length > 0 && (
            <div className="space-y-2">
              <p className="text-xs uppercase tracking-wide text-gray-500">
                Geräte ohne Produktgruppe
              </p>
              {subcategory.direct_devices.map((device) => (
                <DeviceTreeItem
                  key={device.device_id}
                  device={device}
                  onOpenDevice={onOpenDevice}
                  onLocateDevice={onLocateDevice}
                  onOpenZone={onOpenZone}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

interface SubbiercategoryNodeProps {
  group: DeviceTreeSubbiercategory;
  expandedNodes: Set<string>;
  onToggleNode: (id: string) => void;
  onOpenProduct: (productName: string, devices: DeviceTreeDevice[]) => void;
  onOpenDevice: (deviceId: string) => void;
  onLocateDevice: (device: DeviceTreeDevice) => void;
  onOpenZone: (device: DeviceTreeDevice) => void;
}

function SubbiercategoryNode({
  group,
  expandedNodes,
  onToggleNode,
  onOpenProduct,
  onOpenDevice,
  onLocateDevice,
  onOpenZone,
}: SubbiercategoryNodeProps) {
  const nodeId = subbierNodeKey(group.id);
  const isExpanded = expandedNodes.has(nodeId);

  return (
    <div className="rounded-lg border border-white/5 bg-white/[0.01]">
      <div className="flex flex-col gap-2 border-b border-white/5 px-3 py-2 text-left sm:flex-row sm:items-center sm:justify-between">
        <button
          type="button"
          onClick={() => onToggleNode(nodeId)}
          className="flex items-center gap-3 text-left transition-colors hover:text-white"
        >
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-accent-red" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400" />
          )}
          <span className="text-sm font-semibold text-white">{group.name}</span>
          <span className="text-xs font-semibold text-gray-400">
            {group.device_count} Gerät{group.device_count === 1 ? '' : 'e'}
          </span>
        </button>

        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => onOpenProduct(group.name, group.devices)}
            className="flex items-center gap-2 rounded-lg bg-white/10 px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-white/20"
          >
            <Package className="h-4 w-4" /> Produkt öffnen
          </button>
        </div>
      </div>

      {isExpanded && (
        <div className="space-y-2 px-3 py-3">
          {group.devices.map((device) => (
            <DeviceTreeItem
              key={device.device_id}
              device={device}
              onOpenDevice={onOpenDevice}
              onLocateDevice={onLocateDevice}
              onOpenZone={onOpenZone}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface DeviceTreeItemProps {
  device: DeviceTreeDevice;
  onOpenDevice: (deviceId: string) => void;
  onLocateDevice: (device: DeviceTreeDevice) => void;
  onOpenZone: (device: DeviceTreeDevice) => void;
}

function DeviceTreeItem({ device, onOpenDevice, onLocateDevice, onOpenZone }: DeviceTreeItemProps) {
  const isConsumable = (device as any).is_consumable === true;
  const isAccessory = (device as any).is_accessory === true;
  const stockQuantity = (device as any).stock_quantity;
  const unit = (device as any).unit;

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-white/5 bg-white/[0.02] p-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex-1 min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          {isConsumable || isAccessory ? (
            <span className="text-sm font-semibold text-white">{device.product_name || device.device_id}</span>
          ) : (
            <span className="font-mono text-sm font-semibold text-white">{device.device_id}</span>
          )}
          {isConsumable && (
            <span className="text-[10px] font-semibold uppercase tracking-wide px-2 py-1 rounded-full bg-blue-500/20 text-blue-400">
              Verbrauchsmaterial
            </span>
          )}
          {isAccessory && (
            <span className="text-[10px] font-semibold uppercase tracking-wide px-2 py-1 rounded-full bg-purple-500/20 text-purple-400">
              Zubehör
            </span>
          )}
          {!isConsumable && !isAccessory && (
            <span
              className={`text-[10px] font-semibold uppercase tracking-wide px-2 py-1 rounded-full bg-white/10 ${getStatusColor(device.status)}`}
            >
              {formatStatus(device.status)}
            </span>
          )}
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-gray-400">
          {!isConsumable && !isAccessory && device.product_name && <span>{device.product_name}</span>}
          {(isConsumable || isAccessory) && stockQuantity !== undefined && (
            <span className="font-semibold text-white">
              Lagerbestand: {stockQuantity} {unit || ''}
            </span>
          )}
          {!isConsumable && !isAccessory && device.zone_name && (
            <span className="text-gray-400">📍 {device.zone_name}</span>
          )}
          {!isConsumable && !isAccessory && device.zone_code && (
            <span className="font-mono text-gray-500">({device.zone_code})</span>
          )}
          {!isConsumable && !isAccessory && device.case_name && (
            <span className="text-gray-400">📦 {device.case_name}</span>
          )}
          {!isConsumable && !isAccessory && device.job_number && (
            <span className="text-gray-400">🔧 Job #{device.job_number}</span>
          )}
          {!isConsumable && !isAccessory && device.serial_number && (
            <span className="text-gray-500">SN: {device.serial_number}</span>
          )}
          {!isConsumable && !isAccessory && device.barcode && (
            <span className="text-gray-500">Barcode: {device.barcode}</span>
          )}
          {!isConsumable && !isAccessory && device.condition_rating !== undefined && device.condition_rating > 0 && (
            <span className="text-gray-400">Zustand: {device.condition_rating}/10</span>
          )}
          {!isConsumable && !isAccessory && device.usage_hours !== undefined && device.usage_hours > 0 && (
            <span className="text-gray-400">Betriebsstunden: {device.usage_hours}h</span>
          )}
        </div>
      </div>
      <div className="flex items-center gap-2">
        {!isConsumable && !isAccessory && (
          <>
            <button
              type="button"
              onClick={() => onOpenDevice(device.device_id)}
              className="rounded-lg bg-white/10 px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-white/20"
            >
              Details
            </button>
            <button
              type="button"
              onClick={() => onLocateDevice(device)}
              disabled={!device.zone_code}
              className="flex items-center gap-1 rounded-lg bg-white/10 px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-white/20 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Lightbulb className="h-4 w-4 text-yellow-300" /> Licht
            </button>
            <button
              type="button"
              onClick={() => onOpenZone(device)}
              disabled={!device.zone_id && !device.zone_code}
              className="flex items-center gap-1 rounded-lg bg-accent-red/80 px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-accent-red disabled:cursor-not-allowed disabled:opacity-50"
            >
              <MapPin className="h-4 w-4" /> Zone
            </button>
          </>
        )}
        {(isConsumable || isAccessory) && (
          <span className="text-xs text-gray-500 italic">Im Lager verfügbar</span>
        )}
      </div>
    </div>
  );
}

function categoryNodeKey(id: number): string {
  return `cat-${id}`;
}

function subcategoryNodeKey(id: string | number): string {
  return `sub-${id}`;
}

function subbierNodeKey(id: string | number): string {
  return `subb-${id}`;
}

function countDevicesInCategory(category: DeviceTreeCategory): number {
  const directDevices = category.direct_devices?.length ?? 0;
  const subcategoryDevices = category.subcategories?.reduce(
    (sum, subcategory) => sum + countDevicesInSubcategory(subcategory),
    0
  ) ?? 0;
  return directDevices + subcategoryDevices;
}

function countDevicesInSubcategory(subcategory: DeviceTreeSubcategory): number {
  const directDevices = subcategory.direct_devices?.length ?? 0;
  const groupedDevices = subcategory.subbiercategories?.reduce(
    (sum, group) => sum + (group.devices?.length ?? 0),
    0
  ) ?? 0;
  return directDevices + groupedDevices;
}

function countProductGroups(categories: DeviceTreeCategory[]): number {
  return categories.reduce((categorySum, category) => {
    const subcategoryGroups = category.subcategories?.reduce((sum, subcategory) => {
      const grouped = subcategory.subbiercategories?.length ?? 0;
      const ungrouped = subcategory.direct_devices?.length > 0 ? 1 : 0;
      return sum + grouped + ungrouped;
    }, 0) ?? 0;

    const categoryUngrouped = category.direct_devices?.length > 0 ? 1 : 0;

    return categorySum + subcategoryGroups + categoryUngrouped;
  }, 0);
}

function sortDeviceTree(tree: DeviceTreeCategory[]): DeviceTreeCategory[] {
  return [...tree]
    .map((category) => ({
      ...category,
      subcategories: sortSubcategories(category.subcategories ?? []),
      direct_devices: sortDevices(category.direct_devices ?? []),
    }))
    .sort((a, b) => compareAlpha(a.name, b.name));
}

function sortSubcategories(subcategories: DeviceTreeSubcategory[]): DeviceTreeSubcategory[] {
  return [...subcategories]
    .map((subcategory) => ({
      ...subcategory,
      subbiercategories: sortGroups(subcategory.subbiercategories ?? []),
      direct_devices: sortDevices(subcategory.direct_devices ?? []),
    }))
    .sort((a, b) => compareAlpha(a.name, b.name));
}

function sortGroups(groups: DeviceTreeSubbiercategory[]): DeviceTreeSubbiercategory[] {
  return [...groups]
    .map((group) => ({
      ...group,
      devices: sortDevices(group.devices ?? []),
    }))
    .sort((a, b) => compareAlpha(a.name, b.name));
}

function sortDevices(devices: DeviceTreeDevice[]): DeviceTreeDevice[] {
  return [...devices].sort((a, b) => compareAlpha(a.device_id, b.device_id));
}

function compareAlpha(a: string, b: string): number {
  return a.localeCompare(b, 'de', { sensitivity: 'base' });
}

function filterTreeData(treeData: DeviceTreeCategory[], term: string) {
  const trimmed = term.trim().toLowerCase();
  const autoExpandKeys = new Set<string>();

  if (!trimmed) {
    const totalDevices = treeData.reduce((sum, category) => sum + countDevicesInCategory(category), 0);
    return {
      filteredTree: treeData,
      autoExpandKeys,
      totalDevices,
      totalProducts: countProductGroups(treeData),
    };
  }

  const filteredCategories: DeviceTreeCategory[] = [];

  treeData.forEach((category) => {
    const categoryMatches = category.name.toLowerCase().includes(trimmed);
    const filteredSubcategories: DeviceTreeSubcategory[] = [];
    let categoryDeviceCount = 0;

    category.subcategories.forEach((subcategory) => {
      const subcategoryMatches = subcategory.name.toLowerCase().includes(trimmed);
      const filteredGroups: DeviceTreeSubbiercategory[] = [];
      let subcategoryDeviceCount = 0;

      subcategory.subbiercategories.forEach((group) => {
        const groupMatches = group.name.toLowerCase().includes(trimmed);
        const filteredDevices = group.devices.filter((device) => matchesDeviceTerm(device, trimmed));
        const hasMatch = groupMatches || filteredDevices.length > 0;

        if (hasMatch) {
          const devicesToUse = groupMatches ? group.devices : filteredDevices;
          const newGroup: DeviceTreeSubbiercategory = {
            ...group,
            devices: devicesToUse,
            device_count: devicesToUse.length,
          };

          filteredGroups.push(newGroup);
          subcategoryDeviceCount += newGroup.device_count;

          autoExpandKeys.add(categoryNodeKey(category.id));
          autoExpandKeys.add(subcategoryNodeKey(subcategory.id));
          autoExpandKeys.add(subbierNodeKey(group.id));
        }
      });

      const filteredDirectDevices = subcategory.direct_devices.filter((device) =>
        matchesDeviceTerm(device, trimmed)
      );

      if (filteredDirectDevices.length > 0) {
        autoExpandKeys.add(categoryNodeKey(category.id));
        autoExpandKeys.add(subcategoryNodeKey(subcategory.id));
      }

      const hasSubcategoryMatch =
        subcategoryMatches || filteredGroups.length > 0 || filteredDirectDevices.length > 0;

      if (hasSubcategoryMatch) {
        const newSubcategory: DeviceTreeSubcategory = {
          ...subcategory,
          subbiercategories: subcategoryMatches ? subcategory.subbiercategories : filteredGroups,
          direct_devices: subcategoryMatches ? subcategory.direct_devices : filteredDirectDevices,
          device_count: subcategoryMatches
            ? subcategory.device_count
            : filteredGroups.reduce((sum, group) => sum + group.device_count, 0) + filteredDirectDevices.length,
        };
        filteredSubcategories.push(newSubcategory);
        categoryDeviceCount += newSubcategory.device_count;

        autoExpandKeys.add(categoryNodeKey(category.id));
      }
    });

    const filteredCategoryDevices = category.direct_devices.filter((device) =>
      matchesDeviceTerm(device, trimmed)
    );

    if (filteredCategoryDevices.length > 0) {
      autoExpandKeys.add(categoryNodeKey(category.id));
    }

    const hasCategoryMatch =
      categoryMatches || filteredSubcategories.length > 0 || filteredCategoryDevices.length > 0;

    if (hasCategoryMatch) {
      const newCategory: DeviceTreeCategory = {
        ...category,
        subcategories: categoryMatches ? category.subcategories : filteredSubcategories,
        direct_devices: categoryMatches ? category.direct_devices : filteredCategoryDevices,
        device_count: categoryMatches
          ? category.device_count
          : categoryDeviceCount + filteredCategoryDevices.length,
      };
      filteredCategories.push(newCategory);
    }
  });

  const totalDevices = filteredCategories.reduce((sum, category) => sum + category.device_count, 0);
  const totalProducts = countProductGroups(filteredCategories);

  return { filteredTree: filteredCategories, autoExpandKeys, totalDevices, totalProducts };
}

function matchesDeviceTerm(device: DeviceTreeDevice, term: string): boolean {
  const target = term.toLowerCase();
  return (
    device.device_id.toLowerCase().includes(target) ||
    device.product_name.toLowerCase().includes(target) ||
    (device.serial_number?.toLowerCase().includes(target) ?? false) ||
    (device.zone_code?.toLowerCase().includes(target) ?? false)
  );
}
