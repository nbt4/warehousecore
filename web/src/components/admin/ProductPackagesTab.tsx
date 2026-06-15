import { useEffect, useRef, useState } from 'react';
import {
  Plus,
  Pencil,
  Trash2,
  X,
  Search,
  RefreshCcw,
  Eye,
} from 'lucide-react';
import { api } from '../../lib/api';
import { ModalPortal } from '../ModalPortal';
import { toast } from '../../lib/toast';

interface ProductPackage {
  package_id: number;
  product_id: number;
  package_code: string;
  name: string;
  description?: string | null;
  price?: number | string | null;
  total_items: number;
  created_at: string;
  updated_at: string;
  aliases?: string[];
  category_id?: number | null;
  category_name?: string | null;
  subcategory_id?: string | null;
  website_visible?: boolean;
}

interface PackageItemDetail {
  package_item_id: number;
  product_id: number;
  product_name: string;
  quantity: number;
  category_name?: string | null;
  brand_name?: string | null;
}

interface ProductPackageWithItems extends ProductPackage {
  items?: PackageItemDetail[];
}

interface Product {
  product_id: number;
  name: string;
  category_name?: string | null;
  brand_name?: string | null;
}

interface Subcategory {
  subcategory_id: string;
  name: string;
  category_id: number;
}

interface Subbiercategory {
  subbiercategory_id: string;
  name: string;
  subcategory_id: string;
}

interface PackageFormData {
  name: string;
  description: string;
  price: string;
  items: Array<{
    product_id: number;
    quantity: number;
  }>;
  aliases: string[];
  category_id: number | '';
  subcategory_id: string;
  subbiercategory_id: string;
  device_quantity?: number;
  website_visible: boolean;
  website_thumbnail?: string;
  website_images?: string[];
}

interface Device {
  device_id: string;
  product_id?: number;
  status: string;
  serial_number?: string;
  barcode?: string;
}

const initialFormData: PackageFormData = {
  name: '',
  description: '',
  price: '',
  items: [],
  aliases: [],
  category_id: '',
  subcategory_id: '',
  subbiercategory_id: '',
  device_quantity: undefined,
  website_visible: false,
  website_thumbnail: undefined,
  website_images: [],
};

const ensureArray = <T,>(value: T[] | undefined | null): T[] => (Array.isArray(value) ? value : []);

function formatPrice(value?: number | string | null, fallback = '-') {
  if (value === undefined || value === null || value === '') {
    return fallback;
  }

  const numericValue = typeof value === 'number' ? value : parseFloat(value);
  if (Number.isNaN(numericValue)) {
    return fallback;
  }

  return `${numericValue.toFixed(2)} €`;
}

export function ProductPackagesTab() {
  const [packages, setPackages] = useState<ProductPackage[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [viewPackage, setViewPackage] = useState<ProductPackageWithItems | null>(null);
  const [editingPackage, setEditingPackage] = useState<number | null>(null);
  const [formData, setFormData] = useState<PackageFormData>(initialFormData);
  const [searchTerm, setSearchTerm] = useState('');
  const [products, setProducts] = useState<Product[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [selectedProduct, setSelectedProduct] = useState<number | ''>('');
  const [selectedQuantity, setSelectedQuantity] = useState(1);
  const [aliasInput, setAliasInput] = useState('');
  const [formError, setFormError] = useState<string | null>(null);
  const scrollPosition = useRef(0);
  const viewPackagePriceDisplay = viewPackage ? formatPrice(viewPackage.price, '') : '';
  const [categories, setCategories] = useState<Array<{ category_id: number; name: string }>>([]);
  const [subcategories, setSubcategories] = useState<Subcategory[]>([]);
  const [subbiercategories, setSubbiercategories] = useState<Subbiercategory[]>([]);
  const [packageDevices, setPackageDevices] = useState<Device[]>([]);
  const [devicesToDelete, setDevicesToDelete] = useState<Set<string>>(new Set());
  const [loadingDevices, setLoadingDevices] = useState(false);
  const [imageFiles, setImageFiles] = useState<File[]>([]);
  const [thumbnailIndex, setThumbnailIndex] = useState<number | null>(null);

  const fetchPackages = async () => {
    try {
      setLoading(true);
      const params = searchTerm ? { search: searchTerm } : {};
      const response = await api.get('/admin/product-packages', { params });
      const data = Array.isArray(response.data) ? response.data : [];
      if (!Array.isArray(response.data)) {
        console.warn('Unexpected product packages payload:', response.data);
      }
      setPackages(data);
    } catch (error) {
      toast.error('Failed to fetch product packages:' + " " + String(error));
    } finally {
      setLoading(false);
    }
  };

  const fetchProducts = async () => {
    try {
      const response = await api.get('/admin/products');
      const data = Array.isArray(response.data) ? response.data : [];
      if (!Array.isArray(response.data)) {
        console.warn('Unexpected products payload for packages tab:', response.data);
      }
      setProducts(data);
    } catch (error) {
      toast.error('Failed to fetch products:' + " " + String(error));
    }
  };

  const fetchCategories = async () => {
    try {
      const response = await api.get('/admin/categories');
      const data = Array.isArray(response.data) ? response.data : [];
      if (!Array.isArray(response.data)) {
        console.warn('Unexpected categories payload for packages tab:', response.data);
      }
      setCategories(data);
    } catch (error) {
      toast.error('Failed to fetch categories:' + " " + String(error));
    }
  };

  const fetchSubcategories = async () => {
    try {
      const response = await api.get('/admin/subcategories');
      const data = Array.isArray(response.data) ? response.data : [];
      setSubcategories(data);
    } catch (error) {
      toast.error('Failed to fetch subcategories:' + " " + String(error));
    }
  };

  const fetchSubbiercategories = async () => {
    try {
      const response = await api.get('/admin/subbiercategories');
      const data = Array.isArray(response.data) ? response.data : [];
      setSubbiercategories(data);
    } catch (error) {
      toast.error('Failed to fetch subbiercategories:' + " " + String(error));
    }
  };

  // Filter subcategories by selected category
  const filteredSubcategories = subcategories.filter(
    (sub) => !formData.category_id || sub.category_id === formData.category_id
  );

  // Filter subbiercategories by selected subcategory
  const filteredSubbiercategories = subbiercategories.filter(
    (subbier) => !formData.subcategory_id || subbier.subcategory_id === formData.subcategory_id
  );

  useEffect(() => {
    fetchPackages();
  }, [searchTerm]);

  useEffect(() => {
    if ((modalOpen || viewPackage) && products.length === 0) {
      fetchProducts();
    }
    if ((modalOpen || viewPackage) && categories.length === 0) {
      fetchCategories();
    }
    if ((modalOpen || viewPackage) && subcategories.length === 0) {
      fetchSubcategories();
    }
    if ((modalOpen || viewPackage) && subbiercategories.length === 0) {
      fetchSubbiercategories();
    }
  }, [modalOpen, viewPackage, products.length, categories.length, subcategories.length, subbiercategories.length]);

  useEffect(() => {
    const html = document.documentElement;
    const body = document.body;
    if (modalOpen || viewPackage) {
      scrollPosition.current = window.scrollY || window.pageYOffset || 0;
      html.classList.add('modal-open');
      body.classList.add('modal-open');
      body.style.position = 'fixed';
      body.style.top = `-${scrollPosition.current}px`;
      body.style.left = '0';
      body.style.right = '0';
      body.style.width = '100%';
      return () => {
        html.classList.remove('modal-open');
        body.classList.remove('modal-open');
        body.style.position = '';
        body.style.top = '';
        body.style.left = '';
        body.style.right = '';
        body.style.width = '';
        window.scrollTo(0, scrollPosition.current);
      };
    }
    return undefined;
  }, [modalOpen, viewPackage]);

  const loadPackageDevices = async (productId: number) => {
    setLoadingDevices(true);
    try {
      const { data } = await api.get<Device[]>(`/admin/products/${productId}/devices`);
      const devices = Array.isArray(data) ? data : [];
      if (!Array.isArray(data)) {
        console.warn('Unexpected package devices payload:', data);
      }
      setPackageDevices(devices);
      setDevicesToDelete(new Set());
    } catch (error) {
      toast.error('Failed to load package devices:' + " " + String(error));
      setPackageDevices([]);
    } finally {
      setLoadingDevices(false);
    }
  };

  const handleAddDevices = async (productId: number) => {
    const quantity = formData.device_quantity;
    if (!quantity || quantity <= 0) {
      window.alert('Bitte eine gültige Anzahl eingeben.');
      return;
    }

    try {
      await api.post(`/admin/products/${productId}/devices`, {
        product_id: productId,
        quantity: quantity,
      });

      // Reload devices
      await loadPackageDevices(productId);

      // Reset device creation fields
      setFormData({ ...formData, device_quantity: undefined });
    } catch (error) {
      toast.error('Failed to add devices:' + " " + String(error));
      window.alert('Fehler beim Hinzufügen der Geräte.');
    }
  };

  const handleRemoveDevice = (deviceId: string) => {
    setDevicesToDelete(prev => {
      const newSet = new Set(prev);
      if (newSet.has(deviceId)) {
        newSet.delete(deviceId);
      } else {
        newSet.add(deviceId);
      }
      return newSet;
    });
  };

  const handleOpenModal = async (pkg?: ProductPackage) => {
    if (pkg) {
      setEditingPackage(pkg.package_id);
      // Fetch full package details with items
      try {
        const res = await api.get<ProductPackageWithItems>(`/admin/product-packages/${pkg.package_id}`);
        const data = res.data;
        const packageItems = Array.isArray(data.items) ? data.items : [];
        const packageAliases = ensureArray(data.aliases);
        setFormData({
          name: data.name,
          description: data.description || '',
          price: data.price?.toString() || '',
          items: packageItems.map(item => ({
            product_id: item.product_id,
            quantity: item.quantity,
          })) || [],
          aliases: packageAliases,
          category_id: data.category_id || '',
          subcategory_id: data.subcategory_id || '',
          subbiercategory_id: '',
          device_quantity: undefined,
          website_visible: Boolean(data.website_visible),
        });
        // Load devices for the package's product
        if (data.product_id) {
          await loadPackageDevices(data.product_id);
        }
      } catch (err) {
        toast.error('Failed to fetch package details:' + " " + String(err));
      }
    } else {
      setEditingPackage(null);
      setFormData(initialFormData);
      setPackageDevices([]);
      setDevicesToDelete(new Set());
    }
    setAliasInput('');
    setFormError(null);
    setModalOpen(true);
  };

  const handleImageFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files) {
      const newFiles = Array.from(e.target.files);
      setImageFiles((prev) => [...prev, ...newFiles]);
    }
  };

  const handleRemoveImage = (index: number) => {
    setImageFiles((prev) => prev.filter((_, i) => i !== index));
    if (thumbnailIndex === index) {
      setThumbnailIndex(null);
    } else if (thumbnailIndex !== null && thumbnailIndex > index) {
      setThumbnailIndex(thumbnailIndex - 1);
    }
  };

  const handleSetThumbnail = (index: number) => {
    setThumbnailIndex(index);
  };

  const handleCloseModal = () => {
    setModalOpen(false);
    setEditingPackage(null);
    setFormData(initialFormData);
    setSelectedProduct('');
    setSelectedQuantity(1);
    setAliasInput('');
    setFormError(null);
    setPackageDevices([]);
    setDevicesToDelete(new Set());
    setImageFiles([]);
    setThumbnailIndex(null);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setFormError(null);

    if (!formData.name.trim()) {
      setFormError('Bitte gib einen Namen für das Paket an.');
      setSubmitting(false);
      return;
    }

    if (formData.items.length === 0) {
      setFormError('Ein Paket muss mindestens ein Produkt enthalten.');
      setSubmitting(false);
      return;
    }

    try {
      const payload = {
        name: formData.name,
        description: formData.description || null,
        price: formData.price ? parseFloat(formData.price) : null,
        items: formData.items,
        aliases: formData.aliases,
        category_id: formData.category_id || null,
        subcategory_id: formData.subcategory_id || null,
        subbiercategory_id: formData.subbiercategory_id || null,
        website_visible: formData.website_visible,
      };

      let productId: number | undefined;

      if (editingPackage) {
        await api.put(`/admin/product-packages/${editingPackage}`, payload);

        // Get the product_id for this package
        const pkgResponse = await api.get<ProductPackageWithItems>(`/admin/product-packages/${editingPackage}`);
        productId = pkgResponse.data.product_id;

        // Delete devices that were marked for deletion
        if (devicesToDelete.size > 0 && productId) {
          const deletePromises = Array.from(devicesToDelete).map(deviceId =>
            api.delete(`/admin/devices/${deviceId}`)
          );
          try {
            await Promise.all(deletePromises);
          } catch (deviceError) {
            toast.error('Failed to delete some devices:' + " " + String(deviceError));
            window.alert('Paket gespeichert, aber einige Geräte konnten nicht gelöscht werden.');
          }
        }
      } else {
        const { data } = await api.post<ProductPackageWithItems>('/admin/product-packages', payload);
        productId = data.product_id;

        // Create devices if quantity specified
        if (formData.device_quantity && formData.device_quantity > 0 && productId) {
          try {
            await api.post(`/admin/products/${productId}/devices`, {
              product_id: productId,
              quantity: formData.device_quantity,
            });
          } catch (deviceError) {
            toast.error('Failed to create devices:' + " " + String(deviceError));
            window.alert('Paket erstellt, aber Geräte konnten nicht angelegt werden.');
          }
        }
      }

      // Upload images if any were selected
      if (imageFiles.length > 0 && productId) {
        try {
          const uploadFormData = new FormData();
          imageFiles.forEach((file) => {
            uploadFormData.append('files', file);
          });
          if (thumbnailIndex !== null) {
            uploadFormData.append('thumbnail_index', thumbnailIndex.toString());
          }

          await api.post(`/admin/products/${productId}/pictures`, uploadFormData, {
            headers: {
              'Content-Type': 'multipart/form-data',
            },
          });
        } catch (imageError) {
          toast.error('Failed to upload images:' + " " + String(imageError));
          window.alert('Paket gespeichert, aber Bilder konnten nicht hochgeladen werden.');
        }
      }

      handleCloseModal();
      fetchPackages();
    } catch (error) {
      toast.error('Failed to save product package:' + " " + String(error));
      const message =
        (error as any)?.response?.data?.error || 'Fehler beim Speichern des Produktpakets';
      setFormError(message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm('Möchten Sie dieses Produktpaket wirklich löschen?')) return;

    try {
      await api.delete(`/admin/product-packages/${id}`);
      fetchPackages();
    } catch (error) {
      toast.error('Failed to delete product package:' + " " + String(error));
      alert('Fehler beim Löschen des Produktpakets');
    }
  };

  const handleViewPackage = async (pkg: ProductPackage) => {
    try {
      const response = await api.get<ProductPackageWithItems>(`/admin/product-packages/${pkg.package_id}`);
      const packageData = response.data;
      setViewPackage({
        ...packageData,
        aliases: ensureArray(packageData?.aliases),
        items: Array.isArray(packageData?.items) ? packageData.items : [],
      });
    } catch (error) {
      toast.error('Failed to fetch package details:' + " " + String(error));
    }
  };

  const handleAddItem = () => {
    if (!selectedProduct || selectedQuantity <= 0) return;

    const product = products.find(p => p.product_id === selectedProduct);
    if (!product) return;

    // Check if product already exists in items
    const existingIndex = formData.items.findIndex(item => item.product_id === selectedProduct);

    if (existingIndex >= 0) {
      // Update quantity
      const newItems = [...formData.items];
      newItems[existingIndex].quantity += selectedQuantity;
      setFormData({ ...formData, items: newItems });
    } else {
      // Add new item
      setFormData({
        ...formData,
        items: [...formData.items, { product_id: Number(selectedProduct), quantity: selectedQuantity }],
      });
    }

    setSelectedProduct('');
    setSelectedQuantity(1);
  };

  const handleRemoveItem = (index: number) => {
    const newItems = formData.items.filter((_, i) => i !== index);
    setFormData({ ...formData, items: newItems });
  };

  const handleAddAlias = () => {
    const value = aliasInput.trim();
    if (!value) return;
    const exists = formData.aliases.some(alias => alias.toLowerCase() === value.toLowerCase());
    if (exists) {
      setAliasInput('');
      return;
    }
    setFormData({ ...formData, aliases: [...formData.aliases, value] });
    setAliasInput('');
  };

  const handleRemoveAlias = (aliasToRemove: string) => {
    setFormData({
      ...formData,
      aliases: formData.aliases.filter(alias => alias !== aliasToRemove),
    });
  };

  const getProductName = (productId: number) => {
    const product = products.find(p => p.product_id === productId);
    return product?.name || `Produkt #${productId}`;
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <h2 className="text-2xl font-bold text-white">Produktpakete</h2>
          <p className="text-gray-400">Verwalten Sie Produktpakete für Jobs</p>
        </div>
        <button
          onClick={() => handleOpenModal()}
          className="flex items-center gap-2 rounded-xl bg-accent-red/90 px-5 py-3 text-sm font-semibold text-white shadow-lg shadow-red-900/40 transition-all hover:bg-accent-red focus:outline-none focus:ring-2 focus:ring-red-400"
        >
          <Plus className="w-5 h-5" />
          Neues Paket
        </button>
      </div>

      {/* Search & Filters */}
      <div className="flex flex-col sm:flex-row gap-4">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-5 h-5 text-gray-400" />
          <input
            type="text"
            placeholder="Pakete durchsuchen..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full pl-10 pr-4 py-2 bg-gray-800/50 border border-gray-700 rounded-lg text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-accent-red"
          />
        </div>
        <button
          onClick={fetchPackages}
          className="btn-secondary flex items-center gap-2"
        >
          <RefreshCcw className="w-5 h-5" />
          Aktualisieren
        </button>
      </div>

      {/* Packages Table */}
      {loading ? (
        <div className="text-center py-12 text-gray-400">Lade Produktpakete...</div>
      ) : packages.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          Keine Produktpakete gefunden. Erstellen Sie ein neues Paket.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-700">
                <th className="text-left py-3 px-4 text-gray-400 font-semibold">Name</th>
                <th className="text-left py-3 px-4 text-gray-400 font-semibold">Beschreibung</th>
                <th className="text-right py-3 px-4 text-gray-400 font-semibold">Preis</th>
                <th className="text-right py-3 px-4 text-gray-400 font-semibold">Artikel</th>
                <th className="text-right py-3 px-4 text-gray-400 font-semibold">Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {packages.map((pkg) => (
                <tr key={pkg.package_id} className="border-b border-gray-800 hover:bg-gray-800/30">
                  <td className="py-3 px-4 text-white font-medium">{pkg.name}</td>
                  <td className="py-3 px-4 text-gray-400">{pkg.description || '-'}</td>
                  <td className="py-3 px-4 text-right text-white">
                    {formatPrice(pkg.price)}
                  </td>
                  <td className="py-3 px-4 text-right text-white">{pkg.total_items}</td>
                  <td className="py-3 px-4 text-right">
                    <div className="flex justify-end gap-2">
                      <button
                        onClick={() => handleViewPackage(pkg)}
                        className="p-2 text-blue-400 hover:bg-blue-400/10 rounded-lg transition-colors"
                        title="Anzeigen"
                      >
                        <Eye className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => handleOpenModal(pkg)}
                        className="p-2 text-yellow-400 hover:bg-yellow-400/10 rounded-lg transition-colors"
                        title="Bearbeiten"
                      >
                        <Pencil className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => handleDelete(pkg.package_id)}
                        className="p-2 text-red-400 hover:bg-red-400/10 rounded-lg transition-colors"
                        title="Löschen"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create/Edit Modal */}
      {modalOpen && (
        <ModalPortal>
          <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
            <div className="glass-dark rounded-2xl border border-white/10 shadow-2xl p-6 max-w-2xl w-full max-h-[90vh] overflow-y-auto">
              <div className="flex justify-between items-center mb-6">
                <h3 className="text-2xl font-bold text-white">
                  {editingPackage ? 'Paket bearbeiten' : 'Neues Paket'}
                </h3>
                <button
                  onClick={handleCloseModal}
                  className="text-gray-400 hover:text-white p-2 rounded-lg hover:bg-white/10 transition-colors"
                >
                  <X className="w-6 h-6" />
                </button>
              </div>

            {formError && (
              <div className="mb-4 rounded-xl border border-red-500/40 bg-red-500/10 px-4 py-2 text-sm text-red-300">
                {formError}
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Name *
                </label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  className="w-full px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-accent-red"
                  required
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Beschreibung
                </label>
                <textarea
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  className="w-full px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-accent-red"
                  rows={3}
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Preis (€)
                </label>
                <input
                  type="number"
                  step="0.01"
                  value={formData.price}
                  onChange={(e) => setFormData({ ...formData, price: e.target.value })}
                  className="w-full px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-accent-red"
                />
              </div>

              <div className="flex items-center gap-3">
                <input
                  id="pkg-website-visible"
                  type="checkbox"
                  checked={formData.website_visible}
                  onChange={(e) => setFormData({ ...formData, website_visible: e.target.checked })}
                  className="h-4 w-4 rounded border-gray-600 text-accent-red focus:ring-accent-red"
                />
                <label htmlFor="pkg-website-visible" className="text-sm text-gray-200 select-none">
                  Paket auf Website anzeigen
                </label>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Kategorie
                </label>
                <select
                  value={formData.category_id}
                  onChange={(e) => {
                    const value = e.target.value ? Number(e.target.value) : '';
                    setFormData({
                      ...formData,
                      category_id: value,
                      subcategory_id: '',
                      subbiercategory_id: '',
                    });
                  }}
                  className="w-full px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-accent-red"
                >
                  <option value="">Keine Kategorie</option>
                  {categories.map((cat) => (
                    <option key={cat.category_id} value={cat.category_id}>
                      {cat.name}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Subkategorie
                </label>
                <select
                  value={formData.subcategory_id}
                  onChange={(e) => {
                    const value = e.target.value;
                    setFormData({
                      ...formData,
                      subcategory_id: value,
                      subbiercategory_id: '',
                    });
                  }}
                  disabled={!formData.category_id}
                  className="w-full px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-accent-red disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <option value="">Keine Subkategorie</option>
                  {filteredSubcategories.map((sub) => (
                    <option key={sub.subcategory_id} value={sub.subcategory_id}>
                      {sub.name}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Sub-Subkategorie
                </label>
                <select
                  value={formData.subbiercategory_id}
                  onChange={(e) => {
                    const value = e.target.value;
                    setFormData({
                      ...formData,
                      subbiercategory_id: value,
                    });
                  }}
                  disabled={!formData.subcategory_id}
                  className="w-full px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-accent-red disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <option value="">Keine Sub-Subkategorie</option>
                  {filteredSubbiercategories.map((subbier) => (
                    <option key={subbier.subbiercategory_id} value={subbier.subbiercategory_id}>
                      {subbier.name}
                    </option>
                  ))}
                </select>
              </div>

              <div className="border-t border-gray-700 pt-4">
                <h4 className="text-lg font-semibold text-white mb-3">Website-Bilder</h4>
                <p className="text-sm text-gray-400 mb-3">
                  Lade Bilder für die Website hoch. Markiere ein Bild als Thumbnail, das in der Übersicht angezeigt wird.
                </p>

                <div className="mb-3">
                  <label className="btn-secondary cursor-pointer inline-flex items-center gap-2">
                    <Plus className="w-4 h-4" />
                    Bilder auswählen
                    <input
                      type="file"
                      accept="image/*"
                      multiple
                      onChange={handleImageFileChange}
                      className="hidden"
                    />
                  </label>
                </div>

                {imageFiles.length > 0 && (
                  <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
                    {imageFiles.map((file, index) => (
                      <div key={index} className="relative group bg-gray-800 rounded-lg overflow-hidden">
                        <img
                          src={URL.createObjectURL(file)}
                          alt={file.name}
                          className="w-full h-32 object-cover"
                        />
                        <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex flex-col items-center justify-center gap-2">
                          <button
                            type="button"
                            onClick={() => handleSetThumbnail(index)}
                            className={`px-3 py-1 text-xs rounded ${
                              thumbnailIndex === index
                                ? 'bg-green-600 text-white'
                                : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
                            }`}
                          >
                            {thumbnailIndex === index ? '✓ Thumbnail' : 'Als Thumbnail'}
                          </button>
                          <button
                            type="button"
                            onClick={() => handleRemoveImage(index)}
                            className="px-3 py-1 text-xs bg-red-600 text-white rounded hover:bg-red-700"
                          >
                            Entfernen
                          </button>
                        </div>
                        {thumbnailIndex === index && (
                          <div className="absolute top-2 right-2 bg-green-600 text-white text-xs px-2 py-1 rounded">
                            Thumbnail
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div className="border-t border-gray-700 pt-4">
                <h4 className="text-lg font-semibold text-white mb-3">Produkte</h4>

                {/* Add Item Section */}
                <div className="flex gap-2 mb-4">
                  <select
                    value={selectedProduct}
                    onChange={(e) => setSelectedProduct(Number(e.target.value))}
                    className="flex-1 px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-accent-red"
                  >
                    <option value="">Produkt auswählen...</option>
                    {products.map((product) => (
                      <option key={product.product_id} value={product.product_id}>
                        {product.name} {product.category_name ? `(${product.category_name})` : ''}
                      </option>
                    ))}
                  </select>
                  <input
                    type="number"
                    min="1"
                    value={selectedQuantity}
                    onChange={(e) => setSelectedQuantity(parseInt(e.target.value) || 1)}
                    className="w-24 px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-accent-red"
                    placeholder="Anzahl"
                  />
                  <button
                    type="button"
                    onClick={handleAddItem}
                    className="btn-primary flex items-center gap-2 min-w-[120px]"
                  >
                    <Plus className="w-4 h-4" />
                    Hinzufügen
                  </button>
                </div>

                {/* Items List */}
                {formData.items.length === 0 ? (
                  <p className="text-gray-400 text-center py-4">Keine Produkte hinzugefügt</p>
                ) : (
                  <div className="space-y-2">
                    {formData.items.map((item, index) => (
                      <div key={index} className="flex items-center justify-between bg-gray-800 rounded-lg p-3">
                        <div>
                          <span className="text-white font-medium">{getProductName(item.product_id)}</span>
                          <span className="text-gray-400 ml-2">x {item.quantity}</span>
                        </div>
                        <button
                          type="button"
                          onClick={() => handleRemoveItem(index)}
                          className="text-red-400 hover:bg-red-400/10 p-2 rounded-lg transition-colors"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div className="border-t border-gray-700 pt-4">
                <h4 className="text-lg font-semibold text-white mb-2">OCR-Zuordnungen</h4>
                <p className="text-sm text-gray-400 mb-3">
                  Definiere Schlagwörter oder Kürzel, die bei OCR-Erkennung automatisch diesem Paket zugeordnet werden.
                </p>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={aliasInput}
                    onChange={(e) => setAliasInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleAddAlias();
                      }
                    }}
                    className="flex-1 px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-accent-red"
                    placeholder="z.B. Basic Audio Set"
                  />
                  <button
                    type="button"
                    onClick={handleAddAlias}
                    className="btn-secondary px-4"
                  >
                    Hinzufügen
                  </button>
                </div>
                {formData.aliases.length === 0 ? (
                  <p className="text-gray-500 text-sm mt-2">Noch keine Schlüsselwörter definiert.</p>
                ) : (
                  <div className="flex flex-wrap gap-2 mt-3">
                    {formData.aliases.map((alias) => (
                      <span
                        key={alias}
                        className="px-3 py-1 rounded-full bg-white/10 text-white text-sm flex items-center gap-2"
                      >
                        {alias}
                        <button
                          type="button"
                          onClick={() => handleRemoveAlias(alias)}
                          className="text-gray-400 hover:text-white"
                          aria-label="Alias entfernen"
                        >
                          <X className="w-3 h-3" />
                        </button>
                      </span>
                    ))}
                  </div>
                )}
              </div>

              {/* Device Management Section */}
              <div className="border-t border-gray-700 pt-4">
                <h4 className="text-lg font-semibold text-white mb-3">
                  {editingPackage ? 'Geräte verwalten' : 'Geräte erstellen (optional)'}
                </h4>

                {editingPackage && (
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="text-sm text-gray-300">
                        {packageDevices.length} Gerät(e) zugeordnet
                      </span>
                      {loadingDevices && (
                        <span className="text-xs text-gray-400">Lade...</span>
                      )}
                    </div>

                    {packageDevices.length > 0 && (
                      <div className="max-h-48 overflow-y-auto space-y-2 rounded-lg bg-white/5 p-3">
                        {packageDevices.map(device => (
                          <div
                            key={device.device_id}
                            className={`flex items-center justify-between rounded-lg px-3 py-2 transition ${
                              devicesToDelete.has(device.device_id)
                                ? 'bg-red-500/20 border border-red-500/50'
                                : 'bg-white/5 hover:bg-white/10'
                            }`}
                          >
                            <div className="flex-1">
                              <span className="text-sm font-medium text-white">
                                {device.device_id}
                              </span>
                              <span className="ml-2 text-xs text-gray-400">
                                {device.status}
                              </span>
                            </div>
                            <button
                              type="button"
                              onClick={() => handleRemoveDevice(device.device_id)}
                              className={`p-2 rounded-lg transition ${
                                devicesToDelete.has(device.device_id)
                                  ? 'bg-red-500/20 text-red-300 hover:bg-red-500/30'
                                  : 'text-gray-400 hover:bg-white/10 hover:text-red-400'
                              }`}
                              title={devicesToDelete.has(device.device_id) ? 'Löschen rückgängig' : 'Zum Löschen markieren'}
                            >
                              <Trash2 className="w-4 h-4" />
                            </button>
                          </div>
                        ))}
                      </div>
                    )}

                    {devicesToDelete.size > 0 && (
                      <p className="text-xs text-red-300">
                        {devicesToDelete.size} Gerät(e) zum Löschen markiert. Änderungen werden beim Speichern angewendet.
                      </p>
                    )}
                  </div>
                )}

                <div className="grid grid-cols-1 gap-4 md:grid-cols-2 mt-4">
                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">
                      Anzahl Geräte {editingPackage && 'hinzufügen'}
                    </label>
                    <input
                      type="number"
                      min="0"
                      value={formData.device_quantity ?? ''}
                      onChange={event =>
                        setFormData({
                          ...formData,
                          device_quantity: event.target.value ? parseInt(event.target.value) : undefined,
                        })
                      }
                      placeholder="z. B. 10"
                      className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                    />
                  </div>
                </div>

                {editingPackage && (
                  <button
                    type="button"
                    onClick={async () => {
                      const pkgResponse = await api.get<ProductPackageWithItems>(`/admin/product-packages/${editingPackage}`);
                      if (pkgResponse.data.product_id) {
                        await handleAddDevices(pkgResponse.data.product_id);
                      }
                    }}
                    disabled={!formData.device_quantity || formData.device_quantity <= 0}
                    className="w-full mt-3 rounded-lg bg-green-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    Geräte jetzt hinzufügen
                  </button>
                )}

                <p className="text-xs text-gray-400 mt-3">
                  Präfix und Nummerierung werden automatisch aus der Subkategorie generiert (z. B. LGT3001).
                </p>
              </div>

              <div className="flex gap-3 pt-4">
                <button
                  type="button"
                  onClick={handleCloseModal}
                  className="flex-1 btn-secondary"
                  disabled={submitting}
                >
                  Abbrechen
                </button>
                <button type="submit" className="flex-1 btn-primary" disabled={submitting}>
                  {submitting ? 'Speichert...' : editingPackage ? 'Speichern' : 'Erstellen'}
                </button>
              </div>
            </form>
          </div>
          </div>
        </ModalPortal>
      )}

      {/* View Modal */}
      {viewPackage && (
        <ModalPortal>
          <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
            <div className="glass-dark rounded-2xl border border-white/10 shadow-2xl p-6 max-w-2xl w-full max-h-[90vh] overflow-y-auto">
              <div className="flex justify-between items-center mb-6">
                <h3 className="text-2xl font-bold text-white">{viewPackage.name}</h3>
                <button
                  onClick={() => setViewPackage(null)}
                  className="text-gray-400 hover:text-white p-2 rounded-lg hover:bg-white/10 transition-colors"
                >
                  <X className="w-6 h-6" />
                </button>
              </div>

            <div className="space-y-4">
              {viewPackage.description && (
                <div>
                  <h4 className="text-sm font-medium text-gray-400 mb-1">Beschreibung</h4>
                  <p className="text-white">{viewPackage.description}</p>
                </div>
              )}

              {viewPackagePriceDisplay && (
                <div>
                  <h4 className="text-sm font-medium text-gray-400 mb-1">Preis</h4>
                  <p className="text-white text-xl font-bold">{viewPackagePriceDisplay}</p>
                </div>
              )}

              <div>
                <h4 className="text-sm font-medium text-gray-400 mb-1">OCR-Schlüsselwörter</h4>
                {viewPackage.aliases && viewPackage.aliases.length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {viewPackage.aliases.map((alias) => (
                      <span key={alias} className="px-3 py-1 rounded-full bg-white/10 text-white text-sm">
                        {alias}
                      </span>
                    ))}
                  </div>
                ) : (
                  <p className="text-gray-500 text-sm">Keine Schlüsselwörter definiert.</p>
                )}
              </div>

              <div>
                <h4 className="text-sm font-medium text-gray-400 mb-3">Produkte ({viewPackage.total_items} Artikel)</h4>
                {viewPackage.items && viewPackage.items.length > 0 ? (
                  <div className="space-y-2">
                    {viewPackage.items.map((item) => (
                      <div key={item.package_item_id} className="bg-gray-800 rounded-lg p-3">
                        <div className="flex justify-between items-center">
                          <div>
                            <span className="text-white font-medium">{item.product_name}</span>
                            {item.category_name && (
                              <span className="text-gray-400 text-sm ml-2">({item.category_name})</span>
                            )}
                          </div>
                          <span className="text-white font-semibold">x {item.quantity}</span>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-gray-400">Keine Produkte</p>
                )}
              </div>
            </div>

            <button
              onClick={() => setViewPackage(null)}
              className="w-full mt-6 btn-secondary"
            >
              Schließen
            </button>
          </div>
          </div>
        </ModalPortal>
      )}
    </div>
  );
}
