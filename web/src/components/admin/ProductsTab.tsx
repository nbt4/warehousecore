import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
	Archive,
  Cpu,
  Eye,
  GitBranch,
  LayoutGrid,
  List,
  Package,
  Pencil,
  Plus,
  RefreshCcw,
  RotateCcw,
  Search,
  X,
} from 'lucide-react';
import { api, devicesApi, ledApi, type Device } from '../../lib/api';
import { ModalPortal } from '../ModalPortal';
import { DeviceTreeTab } from './DeviceTreeTab';
import { ProductDependenciesModal } from '../ProductDependenciesModal';
import { ProductDetailModal } from '../ProductDetailModal';
import { ProductDevicesModal } from '../ProductDevicesModal';
import { DeviceDetailModal } from '../DeviceDetailModal';
import { toast } from '../../lib/toast';
import { formatStatus } from '../../lib/utils';

interface Product {
  product_id: number;
  name: string;
  category_id?: number | null;
  subcategory_id?: string | number | null;
  subbiercategory_id?: string | number | null;
  manufacturer_id?: number | null;
  brand_id?: number | null;
  description?: string | null;
  maintenance_interval?: number | null;
  item_cost_per_day?: number | null;
  weight?: number | null;
  height?: number | null;
  width?: number | null;
  depth?: number | null;
  power_consumption?: number | null;
  pos_in_category?: number | null;
  category_name?: string | null;
  subcategory_name?: string | null;
  subbiercategory_name?: string | null;
  brand_name?: string | null;
  manufacturer_name?: string | null;
  // Accessories & Consumables fields
  is_accessory?: boolean;
  is_consumable?: boolean;
  count_type_id?: number | null;
  stock_quantity?: number | null;
  min_stock_level?: number | null;
  generic_barcode?: string | null;
  price_per_unit?: number | null;
  count_type_name?: string | null;
  count_type_abbr?: string | null;
  website_visible?: boolean;
  website_thumbnail?: string | null;
  website_images?: string[];
  device_count?: number;
	product_type: 'equipment' | 'accessory' | 'consumable';
	tracking_mode: 'individual' | 'quantity' | 'none';
  lifecycle_status: 'active' | 'archived';
	product_code: string;
	product_kind: 'standard' | 'cable' | 'consumable' | 'container' | 'service';
	model_number?: string | null;
	manufacturer_part_number?: string | null;
	ean?: string | null;
}

interface Category {
  category_id: number;
  name: string;
}

interface Subcategory {
  subcategory_id: string | number;
  name: string;
  category_id: number;
}

interface Subbiercategory {
  subbiercategory_id: string | number;
  name: string;
  subcategory_id: string | number;
}

interface Brand {
  brand_id: number;
  name: string;
  manufacturer_id?: number | null;
  manufacturer_name?: string | null;
}

interface Manufacturer {
  manufacturer_id: number;
  name: string;
  website?: string | null;
}

interface ProductFormData {
  name: string;
  description: string;
  category_id?: number;
  subcategory_id?: string | number;
  subbiercategory_id?: string | number;
  brand_id?: number;
  manufacturer_id?: number;
  item_cost_per_day?: number;
  weight?: number;
  height?: number;
  width?: number;
  depth?: number;
  maintenance_interval?: number;
  power_consumption?: number;
  pos_in_category?: number;
  device_quantity?: number;
  // Accessories & Consumables fields
  is_accessory?: boolean;
  is_consumable?: boolean;
  count_type_id?: number;
  stock_quantity?: number;
  min_stock_level?: number;
  generic_barcode?: string;
  price_per_unit?: number;
	product_type: 'equipment' | 'accessory' | 'consumable';
	tracking_mode: 'individual' | 'quantity' | 'none';
	product_kind: 'standard' | 'cable' | 'consumable' | 'container' | 'service';
	model_number?: string;
	manufacturer_part_number?: string;
	ean?: string;
}

interface CountType {
  count_type_id: number;
  name: string;
  abbreviation: string;
  is_active: boolean;
}

const initialFormData: ProductFormData = {
  name: '',
  description: '',
	product_type: 'equipment',
	tracking_mode: 'individual',
	product_kind: 'standard',
};

const parseNumber = (value: string): number | undefined => {
  const trimmed = value.trim();
  if (trimmed === '') {
    return undefined;
  }
  const parsed = Number(trimmed);
  return Number.isNaN(parsed) ? undefined : parsed;
};

const parseInteger = (value: string): number | undefined => {
  const trimmed = value.trim();
  if (trimmed === '') {
    return undefined;
  }
  const parsed = Number.parseInt(trimmed, 10);
  return Number.isNaN(parsed) ? undefined : parsed;
};

function useDebouncedValue<T>(value: T, delay: number) {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const handle = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(handle);
  }, [value, delay]);

  return debounced;
}

export function ProductsTab() {
  const [products, setProducts] = useState<Product[]>([]);
  const [loadingProducts, setLoadingProducts] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [viewProduct, setViewProduct] = useState<Product | null>(null);
  const [editingProduct, setEditingProduct] = useState<number | null>(null);
  const [dependenciesModal, setDependenciesModal] = useState<{ productId: number; productName: string } | null>(null);
  const [formData, setFormData] = useState<ProductFormData>(initialFormData);
  const [categories, setCategories] = useState<Category[]>([]);
  const [subcategories, setSubcategories] = useState<Subcategory[]>([]);
  const [subbiercategories, setSubbiercategories] = useState<Subbiercategory[]>([]);
  const [brands, setBrands] = useState<Brand[]>([]);
  const [manufacturers, setManufacturers] = useState<Manufacturer[]>([]);
  const [countTypes, setCountTypes] = useState<CountType[]>([]);
  const [metadataLoaded, setMetadataLoaded] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [viewMode, setViewMode] = useState<'table' | 'cards' | 'tree'>(() => {
    // Set default based on screen width: mobile (<768px) = cards, desktop = table
    return typeof window !== 'undefined' && window.innerWidth < 768 ? 'cards' : 'table';
  });
  const navigate = useNavigate();

  const [devicesModal, setDevicesModal] = useState<{ productId: number; productName: string } | null>(null);
  const [devicesModalDevices, setDevicesModalDevices] = useState<Device[]>([]);
  const [devicesModalLoading, setDevicesModalLoading] = useState(false);
  const [devicesModalSelectedDevice, setDevicesModalSelectedDevice] = useState<Device | null>(null);
  const [devicesModalDetailOpen, setDevicesModalDetailOpen] = useState(false);
  const [devicesModalLocatingId, setDevicesModalLocatingId] = useState<string | null>(null);

  const [searchTerm, setSearchTerm] = useState('');
  const [categoryFilter, setCategoryFilter] = useState<number | ''>('');
	const [lifecycleFilter, setLifecycleFilter] = useState<'active' | 'archived' | 'all'>('active');
  const [refreshing, setRefreshing] = useState(false);
  // const scrollPosition = useRef(0);
  const [productDevices, setProductDevices] = useState<Device[]>([]);
  const [loadingDevices, setLoadingDevices] = useState(false);

  const debouncedSearch = useDebouncedValue(searchTerm, 300);

  // Update view mode based on screen size
  useEffect(() => {
    const handleResize = () => {
      const isMobile = window.innerWidth < 768;
      setViewMode(isMobile ? 'cards' : 'table');
    };

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  useEffect(() => {
    const html = document.documentElement;
    const body = document.body;
    const anyModalOpen = modalOpen || !!viewProduct;
    if (anyModalOpen) {
      // scrollPosition.current = window.scrollY || window.pageYOffset || 0; // Not needed if we don't fix body
      html.classList.add('modal-open');
      body.classList.add('modal-open');
      // Fix: Don't set position:fixed on body as it breaks modal positioning relative to viewport
      // body.style.position = 'fixed';
      // body.style.top = `-${scrollPosition.current}px`;
      // body.style.left = '0';
      // body.style.right = '0';
      // body.style.width = '100%';
      return () => {
        html.classList.remove('modal-open');
        body.classList.remove('modal-open');
        // body.style.position = '';
        // body.style.top = '';
        // body.style.left = '';
        // body.style.right = '';
        // body.style.width = '';
        // window.scrollTo(0, scrollPosition.current);
      };
    }

    return undefined;
  }, [modalOpen, viewProduct]);

  const fetchProducts = useCallback(
    async (searchValue?: string, categoryId?: number | '', lifecycle: 'active' | 'archived' | 'all' = 'active') => {
      setLoadingProducts(true);
      try {
        const params: Record<string, string> = {};
        if (searchValue && searchValue.trim().length > 0) {
          params.search = searchValue.trim();
        }
        if (categoryId !== '' && typeof categoryId === 'number') {
          params.category_id = String(categoryId);
        }
		params.lifecycle_status = lifecycle;
        const { data } = await api.get<Product[]>('/admin/products', { params });
        setProducts(data || []);
      } catch (error) {
        toast.error('Failed to load products:' + " " + String(error));
        setProducts([]);
      } finally {
        setLoadingProducts(false);
      }
    },
    []
  );

  const loadMetadata = useCallback(async () => {
    try {
      const [catRes, subRes, subbierRes, brandRes, manufacturerRes, countTypeRes] = await Promise.all([
        api.get<Category[]>('/admin/categories'),
        api.get<Subcategory[]>('/admin/subcategories'),
        api.get<Subbiercategory[]>('/admin/subbiercategories'),
        api.get<Brand[]>('/admin/brands'),
        api.get<Manufacturer[]>('/admin/manufacturers'),
        api.get<CountType[]>('/admin/count-types'),
      ]);

      setCategories(catRes.data || []);
      setSubcategories(subRes.data || []);
      setSubbiercategories(subbierRes.data || []);
      setBrands(brandRes.data || []);
      setManufacturers(manufacturerRes.data || []);
      setCountTypes(countTypeRes.data || []);

      setMetadataLoaded(true);
    } catch (error) {
      toast.error('Failed to load metadata:' + " " + String(error));
    }
  }, []);

  const ensureMetadataLoaded = useCallback(async () => {
    if (!metadataLoaded) {
      await loadMetadata();
    }
  }, [metadataLoaded, loadMetadata]);

  useEffect(() => {
    fetchProducts(debouncedSearch, categoryFilter, lifecycleFilter);
  }, [fetchProducts, debouncedSearch, categoryFilter, lifecycleFilter]);

  useEffect(() => {
    loadMetadata();
  }, [loadMetadata]);

  const handleRefresh = useCallback(async () => {
    setRefreshing(true);
    await fetchProducts(searchTerm, categoryFilter, lifecycleFilter);
    setRefreshing(false);
  }, [fetchProducts, searchTerm, categoryFilter, lifecycleFilter]);

  const handleOpenDevicesModal = async (productId: number, productName: string) => {
    setDevicesModal({ productId, productName });
    setDevicesModalDevices([]);
    setDevicesModalLoading(true);
    try {
      const { data } = await api.get<Device[]>(`/admin/products/${productId}/devices`);
      setDevicesModalDevices(data);
    } catch (error) {
      toast.error('Failed to load product devices:' + " " + String(error));
    } finally {
      setDevicesModalLoading(false);
    }
  };

  const handleDevicesModalLocate = async (device: Device) => {
    if (!device.zone_code) return;
    setDevicesModalLocatingId(device.device_id);
    try {
      await ledApi.locateBin(device.zone_code);
    } catch (error) {
      toast.error('LED locate failed:' + " " + String(error));
    } finally {
      setDevicesModalLocatingId(null);
    }
  };

  const handleDevicesModalOpenZone = (device: Device) => {
    if (device.zone_id) {
      navigate(`/zones/${device.zone_id}`);
    }
  };

  const handleDevicesModalOpenDevice = async (device: Device) => {
    try {
      const { data } = await devicesApi.getById(device.device_id);
      setDevicesModalSelectedDevice(data);
      setDevicesModalDetailOpen(true);
    } catch (error) {
      toast.error('Failed to load device detail:' + " " + String(error));
    }
  };

  const clearFilters = () => {
    setSearchTerm('');
    setCategoryFilter('');
	setLifecycleFilter('active');
  };

  const handleCategoryFilterChange = (value: string) => {
    if (!value) {
      setCategoryFilter('');
      return;
    }
    setCategoryFilter(Number(value));
  };

  const mapProductToFormData = useCallback(
    (product: Product): ProductFormData => ({
      name: product.name ?? '',
      description: product.description ?? '',
      category_id: product.category_id ?? undefined,
      subcategory_id: product.subcategory_id ?? undefined,
      subbiercategory_id: product.subbiercategory_id ?? undefined,
      brand_id: product.brand_id ?? undefined,
      manufacturer_id: product.manufacturer_id ?? undefined,
      item_cost_per_day: product.item_cost_per_day ?? undefined,
      weight: product.weight ?? undefined,
      height: product.height ?? undefined,
      width: product.width ?? undefined,
      depth: product.depth ?? undefined,
      maintenance_interval: product.maintenance_interval ?? undefined,
      power_consumption: product.power_consumption ?? undefined,
      pos_in_category: product.pos_in_category ?? undefined,
      device_quantity: undefined,
      // Accessories & Consumables fields
      is_accessory: product.is_accessory ?? false,
      is_consumable: product.is_consumable ?? false,
      count_type_id: product.count_type_id ?? undefined,
      stock_quantity: product.stock_quantity ?? undefined,
      min_stock_level: product.min_stock_level ?? undefined,
      generic_barcode: product.generic_barcode ?? '',
      price_per_unit: product.price_per_unit ?? undefined,
	  product_type: product.product_type ?? (product.is_consumable ? 'consumable' : product.is_accessory ? 'accessory' : 'equipment'),
	  tracking_mode: product.tracking_mode ?? ((product.is_accessory || product.is_consumable) ? 'quantity' : 'individual'),
	  product_kind: product.product_kind ?? (product.is_consumable ? 'consumable' : 'standard'),
	  model_number: product.model_number ?? '',
	  manufacturer_part_number: product.manufacturer_part_number ?? '',
	  ean: product.ean ?? '',
    }),
    []
  );

  const filteredSubcategories = useMemo(() => {
    if (!formData.category_id) {
      return subcategories;
    }
    return subcategories.filter(sub => sub.category_id === formData.category_id);
  }, [subcategories, formData.category_id]);

  const filteredSubbiercategories = useMemo(() => {
    if (!formData.subcategory_id) {
      return subbiercategories;
    }
    return subbiercategories.filter(subbier => subbier.subcategory_id === formData.subcategory_id);
  }, [subbiercategories, formData.subcategory_id]);

  const loadProductDevices = useCallback(async (productId: number) => {
    setLoadingDevices(true);
    try {
      const { data } = await api.get<Device[]>(`/admin/products/${productId}/devices`);
      setProductDevices(data || []);
    } catch (error) {
      toast.error('Failed to load product devices:' + " " + String(error));
      setProductDevices([]);
    } finally {
      setLoadingDevices(false);
    }
  }, []);

  const closeModal = useCallback(() => {
    setModalOpen(false);
    setEditingProduct(null);
    setFormData(initialFormData);
    setProductDevices([]);
  }, []);

  const closeDetailModal = () => {
    setViewProduct(null);
  };

  const handleOpenCreateModal = async () => {
    await ensureMetadataLoaded();
    setFormData(initialFormData);
    setEditingProduct(null);
    setModalOpen(true);
  };

  const handleEditProduct = async (productId: number) => {
    try {
      await ensureMetadataLoaded();
      const { data } = await api.get<Product>(`/admin/products/${productId}`);
      setFormData(mapProductToFormData(data));
      setEditingProduct(productId);
      setModalOpen(true);
      await loadProductDevices(productId);
    } catch (error) {
      toast.error('Failed to load product details:' + " " + String(error));
      window.alert('Produkt konnte nicht geladen werden.');
    }
  };

  const handleAddDevices = async () => {
    if (!editingProduct) return;

    const quantity = formData.device_quantity;
    if (!quantity || quantity <= 0) {
      window.alert('Bitte eine gültige Anzahl eingeben.');
      return;
    }

    try {
      await api.post(`/admin/products/${editingProduct}/devices`, {
        product_id: editingProduct,
        quantity: quantity,
      });

      // Reload devices
      await loadProductDevices(editingProduct);

      // Reset device creation fields
      setFormData({ ...formData, device_quantity: undefined });
    } catch (error) {
      toast.error('Failed to add devices:' + " " + String(error));
      window.alert('Fehler beim Hinzufügen der Geräte.');
    }
  };

  const handleArchive = async (product: Product) => {
    if (!window.confirm(`Produkt "${product.name}" archivieren? Zugeordnete Geräte und historische Daten bleiben erhalten.`)) {
      return;
    }

    try {
      await api.delete(`/admin/products/${product.product_id}`);
	  toast.success('Produkt wurde archiviert.');
      await fetchProducts(searchTerm, categoryFilter, lifecycleFilter);
    } catch (error) {
      toast.error('Produkt konnte nicht archiviert werden: ' + String(error));
    }
  };

	const handleRestore = async (product: Product) => {
	  try {
		await api.put(`/admin/products/${product.product_id}/restore`);
		toast.success('Produkt wurde wiederhergestellt.');
		await fetchProducts(searchTerm, categoryFilter, lifecycleFilter);
	  } catch (error) {
		toast.error('Produkt konnte nicht wiederhergestellt werden: ' + String(error));
	  }
	};

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    if (!formData.name.trim()) {
      window.alert('Der Produktname ist erforderlich.');
      return;
    }

    setSubmitting(true);

    const payload = {
      name: formData.name.trim(),
      description: formData.description.trim() || null,
      category_id: formData.category_id ?? null,
      subcategory_id: formData.subcategory_id ?? null,
      subbiercategory_id: formData.subbiercategory_id ?? null,
      brand_id: formData.brand_id ?? null,
      manufacturer_id: formData.manufacturer_id ?? null,
      item_cost_per_day: formData.item_cost_per_day ?? null,
      weight: formData.weight ?? null,
      height: formData.height ?? null,
      width: formData.width ?? null,
      depth: formData.depth ?? null,
      maintenance_interval: formData.maintenance_interval ?? null,
      power_consumption: formData.power_consumption ?? null,
      pos_in_category: formData.pos_in_category ?? null,
      // Accessories & Consumables fields
      is_accessory: formData.product_type === 'accessory',
      is_consumable: formData.product_type === 'consumable',
      count_type_id: formData.count_type_id ?? null,
      stock_quantity: formData.stock_quantity ?? null,
      min_stock_level: formData.min_stock_level ?? null,
      generic_barcode: formData.generic_barcode?.trim() || null,
      price_per_unit: formData.price_per_unit ?? null,
	  product_type: formData.product_type,
	  tracking_mode: formData.tracking_mode,
	  product_kind: formData.product_kind,
	  model_number: formData.model_number?.trim() || null,
	  manufacturer_part_number: formData.manufacturer_part_number?.trim() || null,
	  ean: formData.ean?.trim() || null,
	  initial_device_quantity: editingProduct ? 0 : (formData.device_quantity ?? 0),
    };

    try {
      if (editingProduct) {
        await api.put(`/admin/products/${editingProduct}`, payload);
      } else {
		await api.post<Product>('/admin/products', payload);

		// Produkt, initiale Devices und Barcodes werden serverseitig in einer
		// gemeinsamen Transaktion erzeugt.
      }

      await fetchProducts(searchTerm, categoryFilter, lifecycleFilter);
      closeModal();
    } catch (error) {
      toast.error('Failed to save product:' + " " + String(error));
      window.alert('Fehler beim Speichern des Produkts.');
    } finally {
      setSubmitting(false);
    }
  };

  const handleViewProduct = async (productId: number) => {
    try {
      const { data } = await api.get<Product>(`/admin/products/${productId}`);
      setViewProduct(data);
    } catch (error) {
      toast.error('Failed to load product details:' + " " + String(error));
      window.alert('Produkt konnte nicht geladen werden.');
    }
  };

  const brandsByName = useMemo(() => brands, [brands]);
  const manufacturerOptions = useMemo(() => manufacturers, [manufacturers]);
  const sortedProducts = useMemo(() => [...products].sort((a, b) => a.name.localeCompare(b.name)), [
    products,
  ]);

  const hasActiveFilters = debouncedSearch.trim() !== '' || categoryFilter !== '' || lifecycleFilter !== 'active';

  const categoryPath = (product: Product) =>
    [product.category_name, product.subcategory_name, product.subbiercategory_name]
      .filter(Boolean)
      .join(' · ') || 'Keiner Kategorie zugeordnet';

  const formatCurrency = (value?: number | null) =>
    value != null ? `${value.toFixed(2)} €` : '—';

  const averageDailyPrice =
    sortedProducts.length === 0
      ? null
      : sortedProducts.reduce((sum, product) => sum + (product.item_cost_per_day ?? 0), 0) /
      sortedProducts.length;

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-4">
        <div>
          <h2 className="text-xl font-bold text-white">Produkte verwalten</h2>
          <p className="text-sm text-gray-400">
            {sortedProducts.length} Produkte geladen
            {hasActiveFilters ? ' • Filter aktiv' : ''}
          </p>
        </div>

        <div className="flex flex-col gap-3">
          <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <input
                value={searchTerm}
                onChange={event => setSearchTerm(event.target.value)}
                placeholder="Suchen (Name, Beschreibung …)"
                className="w-full rounded-lg bg-white/10 py-2 pl-9 pr-3 text-sm text-white placeholder-gray-500 outline-none transition focus:bg-white/15 focus:ring-1 focus:ring-accent-red"
              />
            </div>

            <select
              value={categoryFilter}
              onChange={event => handleCategoryFilterChange(event.target.value)}
              className="rounded-lg bg-white/10 px-3 py-2 text-sm text-white outline-none transition focus:bg-white/15 focus:ring-1 focus:ring-accent-red"
            >
              <option value="">Alle Kategorien</option>
              {categories.map(category => (
                <option className="bg-gray-900 text-white" key={category.category_id} value={category.category_id}>
                  {category.name}
                </option>
              ))}
            </select>

			<select
			  value={lifecycleFilter}
			  onChange={event => setLifecycleFilter(event.target.value as 'active' | 'archived' | 'all')}
			  className="rounded-lg bg-white/10 px-3 py-2 text-sm text-white outline-none transition focus:bg-white/15 focus:ring-1 focus:ring-accent-red"
			  aria-label="Produktstatus filtern"
			>
			  <option className="bg-gray-900 text-white" value="active">Aktive Produkte</option>
			  <option className="bg-gray-900 text-white" value="archived">Archivierte Produkte</option>
			  <option className="bg-gray-900 text-white" value="all">Alle Produkte</option>
			</select>

            {hasActiveFilters && (
              <button
                type="button"
                onClick={clearFilters}
                className="rounded-lg bg-white/10 px-3 py-2 text-sm text-white hover:bg-white/20 whitespace-nowrap"
              >
                Filter zurücksetzen
              </button>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => setViewMode('table')}
              className={`flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition ${viewMode === 'table'
                ? 'bg-white/15 text-white'
                : 'bg-white/5 text-gray-400 hover:bg-white/10 hover:text-white'
                }`}
            >
              <List className="h-4 w-4" />
              <span className="hidden sm:inline">Tabelle</span>
            </button>
            <button
              type="button"
              onClick={() => setViewMode('cards')}
              className={`flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition ${viewMode === 'cards'
                ? 'bg-white/15 text-white'
                : 'bg-white/5 text-gray-400 hover:bg-white/10 hover:text-white'
                }`}
            >
              <LayoutGrid className="h-4 w-4" />
              <span className="hidden sm:inline">Karten</span>
            </button>
            <button
              type="button"
              onClick={() => setViewMode('tree')}
              className={`flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition ${viewMode === 'tree'
                ? 'bg-white/15 text-white'
                : 'bg-white/5 text-gray-400 hover:bg-white/10 hover:text-white'
                }`}
            >
              <GitBranch className="h-4 w-4" />
              <span className="hidden sm:inline">Gerätebaum</span>
            </button>
            <button
              type="button"
              onClick={handleRefresh}
              disabled={refreshing}
              className="flex items-center gap-2 rounded-lg bg-white/10 px-3 py-2 text-sm font-semibold text-white transition hover:bg-white/20 disabled:opacity-50"
            >
              <RefreshCcw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
              <span className="hidden sm:inline">Aktualisieren</span>
            </button>
            <button
              onClick={handleOpenCreateModal}
              className="flex items-center gap-2 rounded-xl bg-accent-red px-4 py-2 font-semibold text-white hover:shadow-lg"
            >
              <Plus className="h-4 w-4" />
              <span className="hidden sm:inline">Neues Produkt</span>
              <span className="sm:hidden">Neu</span>
            </button>
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-white/10 bg-white/5 p-4 text-sm text-gray-300 backdrop-blur">
        <div className="flex flex-wrap items-center gap-6">
          <span>
            <strong className="text-white">{sortedProducts.length}</strong> Produkte
          </span>
          <span>
            Durchschnittlicher Tagespreis:{' '}
            <strong className="text-white">
              {averageDailyPrice != null ? `${averageDailyPrice.toFixed(2)} €` : '—'}
            </strong>
          </span>
          {categoryFilter !== '' && (
            <span>
              Gefiltert nach Kategorie:{' '}
              <strong className="text-white">
                {
                  categories.find(category => category.category_id === categoryFilter)
                    ?.name
                }
              </strong>
            </span>
          )}
          {debouncedSearch.trim() && (
            <span>
              Suchbegriff: <strong className="text-white">"{debouncedSearch}"</strong>
            </span>
          )}
        </div>
      </div>

      {loadingProducts ? (
        <div className="glass rounded-xl p-8 text-center text-gray-400">
          Lade Produkte …
        </div>
      ) : sortedProducts.length === 0 ? (
        <div className="glass rounded-xl p-8 text-center text-gray-400">
          <Package className="mx-auto mb-4 h-12 w-12 text-gray-600" />
          Keine Produkte gefunden
          {hasActiveFilters ? ' – bitte Filter anpassen.' : '.'}
        </div>
      ) : viewMode === 'table' ? (
        <div className="overflow-hidden rounded-xl border border-white/10 bg-white/5">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-white/10 text-sm text-gray-200">
              <thead className="bg-white/5 text-xs uppercase tracking-wide text-gray-400">
                <tr>
                  <th className="px-4 py-3 text-left font-semibold">Produkt</th>
                  <th className="px-4 py-3 text-left font-semibold">Kategorie</th>
                  <th className="px-4 py-3 text-left font-semibold">Brand / Hersteller</th>
                  <th className="px-4 py-3 text-left font-semibold">Preis pro Tag</th>
				  <th className="px-4 py-3 text-left font-semibold">Typ / Bestand</th>
                  <th className="px-4 py-3 text-left font-semibold">Geräte</th>
                  <th className="px-4 py-3 text-right font-semibold">Aktionen</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/10">
                {sortedProducts.map(product => (
                  <tr key={product.product_id} className="hover:bg-white/5">
                    <td className="px-4 py-3 align-top">
                      <div className="flex flex-col">
                        <span className="text-white font-medium">{product.name}</span>
						<div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>{product.product_code}</div>
                        <span className="text-xs text-gray-500">
                          ID: {product.product_id}
                          {product.pos_in_category != null
                            ? ` • Position ${product.pos_in_category}`
                            : ''}
                        </span>
                        {product.description && (
                          <span className="mt-1 text-xs text-gray-400 line-clamp-2">
                            {product.description}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3 align-top text-sm text-gray-300">
                      {categoryPath(product)}
                    </td>
                    <td className="px-4 py-3 align-top text-sm text-gray-300">
                      <div className="flex flex-col">
                        <span>{product.brand_name || '—'}</span>
                        <span className="text-xs text-gray-500">
                          {product.manufacturer_name || 'Kein Hersteller hinterlegt'}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3 align-top text-sm text-gray-200">
                      {formatCurrency(product.item_cost_per_day)}
                    </td>
					<td className="px-4 py-3 align-top text-sm text-gray-300">
					  <div className="flex flex-col gap-1">
						<span>{product.product_type === 'equipment' ? 'Gerät' : product.product_type === 'accessory' ? 'Zubehör' : 'Verbrauchsmaterial'}</span>
						{product.tracking_mode === 'quantity' && (
						  <span className="text-xs text-gray-500">{product.stock_quantity ?? 0} {product.count_type_abbr || ''} im Bestand</span>
						)}
					  </div>
					</td>
                    <td className="px-4 py-3 align-top">
                      {(product.device_count ?? 0) > 0 ? (
                        <button
                          onClick={() => handleOpenDevicesModal(product.product_id, product.name)}
                          className="flex items-center gap-1.5 rounded-lg bg-accent-red/20 px-2.5 py-1 text-xs font-semibold text-accent-red hover:bg-accent-red/30 transition-colors"
                        >
                          <Cpu className="h-3.5 w-3.5" />
                          {product.device_count} Gerät{product.device_count === 1 ? '' : 'e'}
                        </button>
                      ) : (
                        <span className="text-xs text-gray-500">— Keine</span>
                      )}
                    </td>
                    <td className="px-4 py-3 align-top">
                      <div className="flex justify-end gap-2">
                        <button
                          onClick={() => handleViewProduct(product.product_id)}
                          className="rounded-lg bg-white/10 p-2 text-gray-200 transition hover:bg-white/20 hover:text-white"
                          title="Details anzeigen"
                        >
                          <Eye className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => handleEditProduct(product.product_id)}
                          className="rounded-lg bg-white/10 p-2 text-gray-200 transition hover:bg-white/20 hover:text-white"
                          title="Bearbeiten"
                        >
                          <Pencil className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => setDependenciesModal({ productId: product.product_id, productName: product.name })}
                          className="rounded-lg bg-accent-red/80 p-2 text-white transition hover:bg-accent-red"
                          title="Dependencies verwalten"
                        >
                          <GitBranch className="h-4 w-4" />
                        </button>
						{product.lifecycle_status === 'archived' ? (
						  <button onClick={() => handleRestore(product)} className="rounded-lg bg-emerald-600/80 p-2 text-white transition hover:bg-emerald-600" title="Wiederherstellen">
							<RotateCcw className="h-4 w-4" />
						  </button>
						) : (
						  <button onClick={() => handleArchive(product)} className="rounded-lg bg-amber-600/80 p-2 text-white transition hover:bg-amber-600" title="Archivieren">
							<Archive className="h-4 w-4" />
						  </button>
						)}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : viewMode === 'cards' ? (
        <div className="grid w-full gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {sortedProducts.map(product => (
            <div key={product.product_id} className="glass rounded-xl p-4 min-w-0">
              <div className="flex items-start justify-between gap-4">
                <div className="flex-1 min-w-0 space-y-1">
                  <div className="flex items-center gap-2 min-w-0">
                    <div><h3 className="text-lg font-semibold text-white break-words leading-tight">{product.name}</h3><div className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>{product.product_code}</div></div>
                    <span className="rounded-md bg-white/10 px-2 py-0.5 text-xs text-gray-300 whitespace-nowrap">
                      #{product.product_id}
                    </span>
                  </div>
                  <p className="text-sm text-gray-400 break-words">{categoryPath(product)}</p>
				  <p className="text-xs text-gray-500">
					{product.product_type === 'equipment' ? 'Gerät' : product.product_type === 'accessory' ? 'Zubehör' : 'Verbrauchsmaterial'}
					{' · '}{product.tracking_mode === 'individual' ? 'Einzelverfolgung' : product.tracking_mode === 'quantity' ? `${product.stock_quantity ?? 0} ${product.count_type_abbr || ''}` : 'Keine Bestandsverfolgung'}
				  </p>
                  {(product.device_count ?? 0) > 0 ? (
                    <button
                      onClick={() => handleOpenDevicesModal(product.product_id, product.name)}
                      className="flex items-center gap-1.5 rounded-lg bg-accent-red/20 px-2.5 py-1 text-xs font-semibold text-accent-red hover:bg-accent-red/30 transition-colors w-fit"
                    >
                      <Cpu className="h-3.5 w-3.5" />
                      {product.device_count} Gerät{product.device_count === 1 ? '' : 'e'}
                    </button>
                  ) : (
                    <span className="text-xs text-gray-500">Keine Geräte</span>
                  )}
                  {(product.brand_name || product.manufacturer_name) && (
                    <p className="text-xs text-gray-500 break-words">
                      {product.brand_name || 'Unbekannte Marke'}
                      {product.manufacturer_name ? ` • ${product.manufacturer_name}` : ''}
                    </p>
                  )}
                  {product.description && (
                    <p className="text-xs text-gray-400 break-words line-clamp-3">{product.description}</p>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => handleViewProduct(product.product_id)}
                    className="rounded-lg bg-white/10 p-2 text-gray-200 transition hover:bg-white/20 hover:text-white"
                    title="Details anzeigen"
                  >
                    <Eye className="h-4 w-4" />
                  </button>
                  <button
                    onClick={() => handleEditProduct(product.product_id)}
                    className="rounded-lg bg-white/10 p-2 text-gray-200 transition hover:bg-white/20 hover:text-white"
                    title="Bearbeiten"
                  >
                    <Pencil className="h-4 w-4" />
                  </button>
                  <button
                    onClick={() => setDependenciesModal({ productId: product.product_id, productName: product.name })}
                    className="rounded-lg bg-accent-red/80 p-2 text-white transition hover:bg-accent-red"
                    title="Dependencies verwalten"
                  >
                    <GitBranch className="h-4 w-4" />
                  </button>
				  {product.lifecycle_status === 'archived' ? (
					<button onClick={() => handleRestore(product)} className="rounded-lg bg-emerald-600/80 p-2 text-white transition hover:bg-emerald-600" title="Wiederherstellen">
					  <RotateCcw className="h-4 w-4" />
					</button>
				  ) : (
					<button onClick={() => handleArchive(product)} className="rounded-lg bg-amber-600/80 p-2 text-white transition hover:bg-amber-600" title="Archivieren">
					  <Archive className="h-4 w-4" />
					</button>
				  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="mt-2">
          <DeviceTreeTab />
        </div>
      )}

      {devicesModal && (
        <ProductDevicesModal
          isOpen={!!devicesModal}
          onClose={() => { setDevicesModal(null); setDevicesModalDevices([]); }}
          productName={devicesModal.productName}
          devices={devicesModalDevices}
          loading={devicesModalLoading}
          locatingId={devicesModalLocatingId}
          onLocate={handleDevicesModalLocate}
          onOpenZone={handleDevicesModalOpenZone}
          onOpenDevice={handleDevicesModalOpenDevice}
        />
      )}

      <DeviceDetailModal
        device={devicesModalSelectedDevice}
        isOpen={devicesModalDetailOpen}
        onClose={() => setDevicesModalDetailOpen(false)}
      />

      {modalOpen && (
        <ModalPortal>
          <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
            <div className="glass-dark rounded-2xl border border-white/10 shadow-2xl p-6 max-w-3xl w-full max-h-[90vh] overflow-y-auto">
              <div className="flex justify-between items-center mb-6">
                <h3 className="text-2xl font-bold text-white">
                  {editingProduct ? 'Produkt bearbeiten' : 'Neues Produkt'}
                </h3>
                <button
                  onClick={closeModal}
                  className="text-gray-400 hover:text-white p-2 rounded-lg hover:bg-white/10 transition-colors"
                >
                  <X className="w-6 h-6" />
                </button>
              </div>

              <form onSubmit={handleSubmit} className="space-y-6">
                <div>
                  <label className="mb-2 block text-sm font-semibold text-white">
                    Name <span className="text-accent-red">*</span>
                  </label>
                  <input
                    type="text"
                    value={formData.name}
                    onChange={event => setFormData({ ...formData, name: event.target.value })}
                    className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                    required
                  />
                </div>

                <div>
                  <label className="mb-2 block text-sm font-semibold text-white">Beschreibung</label>
                  <textarea
                    value={formData.description}
                    onChange={event => setFormData({ ...formData, description: event.target.value })}
                    rows={3}
                    className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                  />
                </div>

				<div className="suite-card p-4">
				  <h3 className="text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>1 · Stammdaten</h3>
				  <p className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>Produktklasse, Marke und Hersteller beschreiben den Artikel; die Bestandsführung wird separat gewählt.</p>
				  <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
					<div>
					  <label className="mb-2 block text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>Produktklasse</label>
					  <select value={formData.product_kind} onChange={event => setFormData({...formData, product_kind: event.target.value as ProductFormData['product_kind']})} className="w-full">
						<option value="standard">Standardprodukt</option>
						<option value="cable">Kabel / Adapter</option>
						<option value="consumable">Verbrauchsmaterial</option>
						<option value="container">Behälter / Case</option>
						<option value="service">Dienstleistung</option>
					  </select>
					</div>
					<div>
					  <label className="mb-2 block text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>Modellbezeichnung</label>
					  <input value={formData.model_number || ''} onChange={event => setFormData({...formData, model_number: event.target.value})} placeholder="z. B. SM58-LCE" className="w-full" />
					</div>
					<div>
					  <label className="mb-2 block text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>Hersteller-Artikelnummer</label>
					  <input value={formData.manufacturer_part_number || ''} onChange={event => setFormData({...formData, manufacturer_part_number: event.target.value})} className="w-full" />
					</div>
					<div>
					  <label className="mb-2 block text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>EAN / GTIN</label>
					  <input value={formData.ean || ''} onChange={event => setFormData({...formData, ean: event.target.value})} inputMode="numeric" className="w-full font-mono" />
					</div>
				  </div>
				</div>

				<div className="space-y-4 rounded-xl border border-white/10 p-4">
				  <div>
					<h3 className="text-sm font-semibold text-white">Produkttyp und Bestandsführung</h3>
					<p className="mt-1 text-xs text-gray-400">Legt fest, ob konkrete Exemplare oder eine Menge verwaltet werden.</p>
				  </div>
				  <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
					<div>
					  <label className="mb-2 block text-sm font-semibold text-white">Produkttyp</label>
					  <select
						value={formData.product_type}
						onChange={event => {
						  const productType = event.target.value as ProductFormData['product_type'];
						  const trackingMode = productType === 'equipment' ? 'individual' : 'quantity';
						  setFormData({ ...formData, product_type: productType, tracking_mode: trackingMode, device_quantity: trackingMode === 'individual' ? formData.device_quantity : undefined });
						}}
						className="w-full rounded-lg border border-white/20 bg-gray-900 px-3 py-2 text-white outline-none transition focus:border-accent-red"
					  >
						<option className="bg-gray-900 text-white" value="equipment">Vermiet-/Betriebsmittel</option>
						<option className="bg-gray-900 text-white" value="accessory">Zubehörrolle</option>
						<option className="bg-gray-900 text-white" value="consumable">Verbrauchsmaterial</option>
					  </select>
					</div>
					<div>
					  <label className="mb-2 block text-sm font-semibold text-white">Verfolgung</label>
					  <select
						value={formData.tracking_mode}
						onChange={event => setFormData({ ...formData, tracking_mode: event.target.value as ProductFormData['tracking_mode'] })}
						className="w-full rounded-lg border border-white/20 bg-gray-900 px-3 py-2 text-white outline-none transition focus:border-accent-red"
					  >
						{formData.product_type !== 'consumable' && <option className="bg-gray-900 text-white" value="individual">Einzelne Exemplare</option>}
						{formData.product_type !== 'equipment' && <option className="bg-gray-900 text-white" value="quantity">Mengenbestand</option>}
						{formData.product_type === 'equipment' && <option className="bg-gray-900 text-white" value="none">Keine Bestandsverfolgung</option>}
					  </select>
					</div>
				  </div>
				</div>

                <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">Kategorie</label>
                    <select
                      value={formData.category_id ?? ''}
                      onChange={event => {
                        const value = event.target.value ? Number(event.target.value) : undefined;
                        setFormData({
                          ...formData,
                          category_id: value,
                          subcategory_id: undefined,
                          subbiercategory_id: undefined,
                        });
                      }}
                      className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white outline-none transition focus:border-accent-red"
                    >
                      <option value="">Keine</option>
                      {categories.map(category => (
                        <option key={category.category_id} value={category.category_id}>
                          {category.name}
                        </option>
                      ))}
                    </select>
                  </div>

                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">Unterkategorie</label>
                    <select
                      value={formData.subcategory_id ?? ''}
                      onChange={event => {
                        const value = event.target.value || undefined;
                        setFormData({
                          ...formData,
                          subcategory_id: value,
                          subbiercategory_id: undefined,
                        });
                      }}
                      disabled={!formData.category_id}
                      className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white outline-none transition focus:border-accent-red disabled:opacity-50"
                    >
                      <option value="">Keine</option>
                      {filteredSubcategories.map(sub => (
                        <option key={sub.subcategory_id} value={sub.subcategory_id}>
                          {sub.name}
                        </option>
                      ))}
                    </select>
                  </div>

                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">
                      Sub-Unterkategorie
                    </label>
                    <select
                      value={formData.subbiercategory_id ?? ''}
                      onChange={event => {
                        const value = event.target.value || undefined;
                        setFormData({
                          ...formData,
                          subbiercategory_id: value,
                        });
                      }}
                      disabled={!formData.subcategory_id}
                      className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white outline-none transition focus:border-accent-red disabled:opacity-50"
                    >
                      <option value="">Keine</option>
                      {filteredSubbiercategories.map(subbier => (
                        <option key={subbier.subbiercategory_id} value={subbier.subbiercategory_id}>
                          {subbier.name}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>

                <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">Marke</label>
                    <select
                      value={formData.brand_id ?? ''}
                      onChange={event => {
                        const value = event.target.value ? Number(event.target.value) : undefined;
                        let manufacturerId = formData.manufacturer_id;
                        if (value) {
                          const brand = brandsByName.find(b => b.brand_id === value);
                          if (brand?.manufacturer_id) {
                            manufacturerId = brand.manufacturer_id;
                          }
                        }
                        setFormData({
                          ...formData,
                          brand_id: value,
                          manufacturer_id: manufacturerId ?? formData.manufacturer_id,
                        });
                      }}
                      className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white outline-none transition focus:border-accent-red"
                    >
                      <option value="">Keine</option>
                      {brandsByName.map(brand => (
                        <option key={brand.brand_id} value={brand.brand_id}>
                          {brand.name}
                          {brand.manufacturer_name ? ` • ${brand.manufacturer_name}` : ''}
                        </option>
                      ))}
                    </select>
                  </div>

                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">Hersteller</label>
                    <select
                      value={formData.manufacturer_id ?? ''}
                      onChange={event => {
                        const value = event.target.value ? Number(event.target.value) : undefined;
                        setFormData({
                          ...formData,
                          manufacturer_id: value,
                        });
                      }}
                      className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white outline-none transition focus:border-accent-red"
                    >
                      <option value="">Keine</option>
                      {manufacturerOptions.map(manufacturer => (
                        <option key={manufacturer.manufacturer_id} value={manufacturer.manufacturer_id}>
                          {manufacturer.name}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>

                <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">
                      Preis pro Tag (€)
                    </label>
                    <input
                      type="number"
                      step="0.01"
                      min="0"
                      value={formData.item_cost_per_day ?? ''}
                      onChange={event =>
                        setFormData({
                          ...formData,
                          item_cost_per_day: parseNumber(event.target.value),
                        })
                      }
                      className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                    />
                  </div>
                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">
                      Position in Kategorie
                    </label>
                    <input
                      type="number"
                      value={formData.pos_in_category ?? ''}
                      onChange={event =>
                        setFormData({
                          ...formData,
                          pos_in_category: parseInteger(event.target.value),
                        })
                      }
                      className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                    />
                  </div>
                </div>

                <div className="space-y-4 rounded-xl border border-white/10 p-4">
                  <h3 className="text-sm font-semibold text-white">Physische Eigenschaften</h3>
                  <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
                    {[
                      { label: 'Gewicht (kg)', key: 'weight' as const },
                      { label: 'Höhe (cm)', key: 'height' as const },
                      { label: 'Breite (cm)', key: 'width' as const },
                      { label: 'Tiefe (cm)', key: 'depth' as const },
                    ].map(field => (
                      <div key={field.key}>
                        <label className="mb-2 block text-sm font-semibold text-white">
                          {field.label}
                        </label>
                        <input
                          type="number"
                          step="0.01"
                          min="0"
                          value={formData[field.key] ?? ''}
                          onChange={event =>
                            setFormData({
                              ...formData,
                              [field.key]: parseNumber(event.target.value),
                            })
                          }
                          className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                        />
                      </div>
                    ))}
                  </div>
                </div>

                <div className="space-y-4 rounded-xl border border-white/10 p-4">
                  <h3 className="text-sm font-semibold text-white">Technische Angaben</h3>
                  <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                    <div>
                      <label className="mb-2 block text-sm font-semibold text-white">
                        Wartungsintervall (Tage)
                      </label>
                      <input
                        type="number"
                        min="0"
                        value={formData.maintenance_interval ?? ''}
                        onChange={event =>
                          setFormData({
                            ...formData,
                            maintenance_interval: parseInteger(event.target.value),
                          })
                        }
                        className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                      />
                    </div>
                    <div>
                      <label className="mb-2 block text-sm font-semibold text-white">
                        Leistungsaufnahme (W)
                      </label>
                      <input
                        type="number"
                        step="0.01"
                        min="0"
                        value={formData.power_consumption ?? ''}
                        onChange={event =>
                          setFormData({
                            ...formData,
                            power_consumption: parseNumber(event.target.value),
                          })
                        }
                        className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                      />
                    </div>
                  </div>
                </div>

                {formData.tracking_mode === 'quantity' && (
				<div className="space-y-4 rounded-xl border border-white/10 p-4">
				  <h3 className="text-sm font-semibold text-white">Mengenbestand</h3>
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                      <div>
                        <label className="mb-2 block text-sm font-semibold text-white">
						  Mengeneinheit <span className="text-accent-red">*</span>
                        </label>
                        <select
                          value={formData.count_type_id || ''}
                          onChange={e => setFormData({
                            ...formData,
                            count_type_id: e.target.value ? Number(e.target.value) : undefined
                          })}
						  required
						  className="w-full rounded-lg border border-white/20 bg-gray-900 px-3 py-2 text-white outline-none transition focus:border-accent-red"
                        >
						  <option className="bg-gray-900 text-white" value="">Einheit auswählen …</option>
                          {countTypes.map(ct => (
							<option className="bg-gray-900 text-white" key={ct.count_type_id} value={ct.count_type_id}>
                              {ct.name} ({ct.abbreviation})
                            </option>
                          ))}
                        </select>
                      </div>

                      <div>
                        <label className="mb-2 block text-sm font-semibold text-white">
						  Interner Produktbarcode
                        </label>
                        <input
                          type="text"
                          value={formData.generic_barcode || ''}
						  readOnly
						  placeholder="wird automatisch erzeugt"
                          className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                        />
						<p className="text-xs text-gray-400 mt-1">Wird unveränderlich erzeugt; Hersteller-EAN steht in den Stammdaten.</p>
                      </div>

                      <div>
                        <label className="mb-2 block text-sm font-semibold text-white">
						  Aktueller Bestand
                        </label>
                        <input
                          type="number"
                          step="0.001"
                          min="0"
                          value={formData.stock_quantity ?? ''}
                          onChange={e => setFormData({
                            ...formData,
                            stock_quantity: parseNumber(e.target.value)
                          })}
                          placeholder="0"
                          className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                        />
						{editingProduct && <p className="mt-1 text-xs text-gray-400">Verteilter Bestand wird über Lagerzonen und Scanvorgänge korrigiert.</p>}
                      </div>

                      <div>
                        <label className="mb-2 block text-sm font-semibold text-white">
						  Mindestbestand
                        </label>
                        <input
                          type="number"
                          step="0.001"
                          min="0"
                          value={formData.min_stock_level ?? ''}
                          onChange={e => setFormData({
                            ...formData,
                            min_stock_level: parseNumber(e.target.value)
                          })}
						  placeholder="Schwellwert für Bestandswarnungen"
                          className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                        />
                      </div>

                      <div>
                        <label className="mb-2 block text-sm font-semibold text-white">
						  Preis pro Einheit (€)
                        </label>
                        <input
                          type="number"
                          step="0.01"
                          min="0"
                          value={formData.price_per_unit ?? ''}
                          onChange={e => setFormData({
                            ...formData,
                            price_per_unit: parseNumber(e.target.value)
                          })}
                          placeholder="0.00"
                          className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                        />
                      </div>
                    </div>
                </div>
				)}

				{formData.tracking_mode === 'individual' && (
				<div className="space-y-4 rounded-xl border border-white/10 p-4">
                  <h3 className="text-sm font-semibold text-white">
                    {editingProduct ? 'Geräte verwalten' : 'Geräte erstellen (optional)'}
                  </h3>

                  {editingProduct && (
                    <div className="space-y-3">
                      <div className="flex items-center justify-between">
                        <span className="text-sm text-gray-300">
                          {productDevices.length} Gerät(e) zugeordnet
                        </span>
                        {loadingDevices && (
                          <span className="text-xs text-gray-400">Lade...</span>
                        )}
                      </div>

                      {productDevices.length > 0 && (
                        <div className="max-h-48 overflow-y-auto space-y-2 rounded-lg bg-white/5 p-3">
                          {productDevices.map(device => (
                            <div
                              key={device.device_id}
							  className="flex items-center justify-between rounded-lg bg-white/5 px-3 py-2"
                            >
                              <div className="flex-1">
                                <span className="text-sm font-medium text-white">
                                  {device.device_id}
                                </span>
                                <span className="ml-2 text-xs text-gray-400">
                                  {formatStatus(device.status)}
                                </span>
                              </div>
                            </div>
                          ))}
                        </div>
                      )}

					  <p className="text-xs text-gray-400">Geräte werden separat über die Geräteverwaltung bearbeitet oder ausgemustert.</p>
                    </div>
                  )}

                  <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                    <div>
                      <label className="mb-2 block text-sm font-semibold text-white">
                        Anzahl Geräte {editingProduct && 'hinzufügen'}
                      </label>
                      <input
                        type="number"
                        min="0"
                        value={formData.device_quantity ?? ''}
                        onChange={event =>
                          setFormData({
                            ...formData,
                            device_quantity: parseInteger(event.target.value),
                          })
                        }
                        placeholder="z. B. 10"
                        className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                      />
                    </div>
                  </div>

                  {editingProduct && (
                    <button
                      type="button"
                      onClick={handleAddDevices}
                      disabled={!formData.device_quantity || formData.device_quantity <= 0}
                      className="w-full rounded-lg bg-accent-red px-4 py-2 text-sm font-semibold text-white transition hover:bg-accent-red-hover disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      Geräte jetzt hinzufügen
                    </button>
                  )}

                  <p className="text-xs text-gray-400">
                    Geräte-ID, Barcode und QR-Code werden automatisch und unabhängig von der Kategorie erzeugt.
                  </p>
                </div>
				)}

                <div className="flex gap-3 pt-4">
                  <button
                    type="button"
                    onClick={closeModal}
                    className="flex-1 btn-secondary"
                    disabled={submitting}
                  >
                    Abbrechen
                  </button>
                  <button type="submit" className="flex-1 btn-primary" disabled={submitting}>
                    {submitting ? 'Speichert...' : editingProduct ? 'Speichern' : 'Erstellen'}
                  </button>
                </div>
              </form>
            </div>
          </div>
        </ModalPortal>
      )}

      <ProductDetailModal
        product={viewProduct ? {
          product_id: viewProduct.product_id,
          name: viewProduct.name,
          description: viewProduct.description || undefined,
          website_visible: viewProduct.website_visible,
          website_thumbnail: viewProduct.website_thumbnail || undefined,
          website_images: viewProduct.website_images || [],
          category_name: viewProduct.category_name || undefined,
          subcategory_name: viewProduct.subcategory_name || undefined,
          subbiercategory_name: viewProduct.subbiercategory_name || undefined,
          brand_name: viewProduct.brand_name || undefined,
          manufacturer_name: viewProduct.manufacturer_name || undefined,
          item_cost_per_day: viewProduct.item_cost_per_day || undefined,
          maintenance_interval: viewProduct.maintenance_interval || undefined,
          weight: viewProduct.weight || undefined,
          height: viewProduct.height || undefined,
          width: viewProduct.width || undefined,
          depth: viewProduct.depth || undefined,
          power_consumption: viewProduct.power_consumption || undefined,
          pos_in_category: viewProduct.pos_in_category || undefined,
          is_accessory: viewProduct.is_accessory,
          is_consumable: viewProduct.is_consumable,
          stock_quantity: viewProduct.stock_quantity || undefined,
          min_stock_level: viewProduct.min_stock_level || undefined,
          generic_barcode: viewProduct.generic_barcode || undefined,
          price_per_unit: viewProduct.price_per_unit || undefined,
          count_type_abbreviation: viewProduct.count_type_abbr || undefined,
          device_count: viewProduct.device_count,
        } : null}
        isOpen={!!viewProduct}
        onClose={closeDetailModal}
      />

      {/* Product Dependencies Modal */}
      {dependenciesModal && (
        <ProductDependenciesModal
          productId={dependenciesModal.productId}
          productName={dependenciesModal.productName}
          onClose={() => setDependenciesModal(null)}
        />
      )}
    </div>
  );
}
