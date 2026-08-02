import { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronDown, Eye, Image as ImageIcon, Pencil, Plus, RefreshCcw, Search, Star, Trash2, X } from 'lucide-react';
import { api } from '../../lib/api';
import { toast } from '../../lib/toast';
import { ModalPortal } from '../ModalPortal';

interface ProductPackage {
  package_id: number;
  package_code: string;
  name: string;
  description?: string | null;
  price?: number | string | null;
  category?: string | null;
  total_items: number;
  aliases?: string[];
  website_visible: boolean;
  website_thumbnail?: string | null;
  website_images?: string[];
  created_at: string;
  updated_at: string;
}

interface PackageItem {
  package_item_id?: number;
  product_id: number;
  product_name?: string;
  quantity: number;
  category_name?: string | null;
  brand_name?: string | null;
}

interface ProductPackageDetails extends ProductPackage {
  items: PackageItem[];
}

interface Product {
  product_id: number;
  name: string;
  category_name?: string | null;
  brand_name?: string | null;
}

interface Picture {
  file_name: string;
  size: number;
  content_type: string;
  modified_at: string;
  download_url: string;
  thumbnail_url: string;
  preview_url: string;
}

interface PackageForm {
  name: string;
  description: string;
  price: string;
  category: string;
  items: PackageItem[];
  aliases: string[];
  website_visible: boolean;
  website_thumbnail: string | null;
  website_images: string[];
}

const emptyForm: PackageForm = {
  name: '',
  description: '',
  price: '',
  category: '',
  items: [],
  aliases: [],
  website_visible: false,
  website_thumbnail: null,
  website_images: [],
};

const arrayOrEmpty = <T,>(value?: T[] | null): T[] => (Array.isArray(value) ? value : []);
const primaryButtonClass = 'rounded-lg bg-accent-red px-4 py-2 font-semibold text-white transition-colors hover:bg-accent-red-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red focus-visible:ring-offset-2 focus-visible:ring-offset-gray-900 disabled:cursor-not-allowed disabled:opacity-50';
const secondaryButtonClass = 'rounded-lg border border-white/10 bg-white/5 px-4 py-2 font-semibold text-white transition-colors hover:border-accent-red/60 hover:bg-accent-red/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red focus-visible:ring-offset-2 focus-visible:ring-offset-gray-900 disabled:cursor-not-allowed disabled:opacity-50';
const iconButtonClass = 'rounded-lg p-2 text-gray-400 transition-colors hover:bg-accent-red/10 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red';
const checkboxClass = 'h-4 w-4 cursor-pointer rounded border-gray-600 bg-gray-900 accent-accent-red focus:ring-2 focus:ring-accent-red focus:ring-offset-2 focus:ring-offset-gray-900';

function errorMessage(error: unknown, fallback: string) {
  if (typeof error === 'object' && error !== null && 'response' in error) {
    const response = (error as { response?: { data?: { error?: string } } }).response;
    if (response?.data?.error) return response.data.error;
  }
  return fallback;
}

function formatPrice(value?: number | string | null) {
  if (value === undefined || value === null || value === '') return '–';
  const number = typeof value === 'number' ? value : Number.parseFloat(value);
  return Number.isFinite(number) ? `${number.toFixed(2)} €` : '–';
}

export function ProductPackagesTab() {
  const [packages, setPackages] = useState<ProductPackage[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [modalOpen, setModalOpen] = useState(false);
  const [editingID, setEditingID] = useState<number | null>(null);
  const [viewPackage, setViewPackage] = useState<ProductPackageDetails | null>(null);
  const [form, setForm] = useState<PackageForm>(emptyForm);
  const [pictures, setPictures] = useState<Picture[]>([]);
  const [newImages, setNewImages] = useState<File[]>([]);
  const [newThumbnailIndex, setNewThumbnailIndex] = useState<number | null>(null);
  const [selectedProduct, setSelectedProduct] = useState<number | ''>('');
  const [selectedQuantity, setSelectedQuantity] = useState(1);
  const [productDropdownOpen, setProductDropdownOpen] = useState(false);
  const [aliasInput, setAliasInput] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const productDropdownRef = useRef<HTMLDivElement>(null);

  const fetchPackages = useCallback(async () => {
    setLoading(true);
    try {
      const response = await api.get<ProductPackage[]>('/admin/product-packages', {
        params: search.trim() ? { search: search.trim() } : undefined,
      });
      setPackages(arrayOrEmpty(response.data));
    } catch (error) {
      toast.error(errorMessage(error, 'Produktpakete konnten nicht geladen werden.'));
    } finally {
      setLoading(false);
    }
  }, [search]);

  const fetchProducts = async () => {
    try {
      const response = await api.get<Product[]>('/admin/products');
      setProducts(arrayOrEmpty(response.data));
    } catch (error) {
      toast.error(errorMessage(error, 'Produkte konnten nicht geladen werden.'));
    }
  };

  useEffect(() => {
    const timer = window.setTimeout(fetchPackages, 250);
    return () => window.clearTimeout(timer);
  }, [fetchPackages]);

  useEffect(() => {
    void fetchProducts();
  }, []);

  useEffect(() => {
    if (!modalOpen && !viewPackage) return;
    const scrollY = window.scrollY;
    document.documentElement.classList.add('modal-open');
    document.body.classList.add('modal-open');
    document.body.style.position = 'fixed';
    document.body.style.top = `-${scrollY}px`;
    document.body.style.width = '100%';
    return () => {
      document.documentElement.classList.remove('modal-open');
      document.body.classList.remove('modal-open');
      document.body.style.position = '';
      document.body.style.top = '';
      document.body.style.width = '';
      window.scrollTo(0, scrollY);
    };
  }, [modalOpen, viewPackage]);

  useEffect(() => {
    if (!productDropdownOpen) return;
    const closeOnOutsideClick = (event: PointerEvent) => {
      if (!productDropdownRef.current?.contains(event.target as Node)) setProductDropdownOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setProductDropdownOpen(false);
    };
    document.addEventListener('pointerdown', closeOnOutsideClick);
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('pointerdown', closeOnOutsideClick);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [productDropdownOpen]);

  const loadPictures = async (packageID: number) => {
    try {
      const response = await api.get<{ pictures: Picture[] }>(`/admin/product-packages/${packageID}/pictures`);
      setPictures(arrayOrEmpty(response.data?.pictures));
    } catch (error) {
      setPictures([]);
      toast.error(errorMessage(error, 'Paketbilder konnten nicht geladen werden.'));
    }
  };

  const openEditor = async (pkg?: ProductPackage) => {
    setFormError(null);
    setNewImages([]);
    setNewThumbnailIndex(null);
    setAliasInput('');
    setSelectedProduct('');
    setSelectedQuantity(1);
    setProductDropdownOpen(false);
    setPictures([]);
    if (!pkg) {
      setEditingID(null);
      setForm(emptyForm);
      setModalOpen(true);
      return;
    }
    try {
      const response = await api.get<ProductPackageDetails>(`/admin/product-packages/${pkg.package_id}`);
      const data = response.data;
      setEditingID(pkg.package_id);
      setForm({
        name: data.name,
        description: data.description || '',
        price: data.price === null || data.price === undefined ? '' : String(data.price),
        category: data.category || '',
        items: arrayOrEmpty(data.items),
        aliases: arrayOrEmpty(data.aliases),
        website_visible: Boolean(data.website_visible),
        website_thumbnail: data.website_thumbnail || null,
        website_images: arrayOrEmpty(data.website_images),
      });
      setModalOpen(true);
      void loadPictures(pkg.package_id);
    } catch (error) {
      toast.error(errorMessage(error, 'Paketdetails konnten nicht geladen werden.'));
    }
  };

  const closeEditor = () => {
    setModalOpen(false);
    setEditingID(null);
    setForm(emptyForm);
    setPictures([]);
    setNewImages([]);
    setNewThumbnailIndex(null);
    setProductDropdownOpen(false);
    setFormError(null);
  };

  const addItem = () => {
    if (!selectedProduct || selectedQuantity < 1) return;
    const product = products.find((entry) => entry.product_id === selectedProduct);
    if (!product) return;
    const existing = form.items.findIndex((item) => item.product_id === selectedProduct);
    if (existing >= 0) {
      const items = [...form.items];
      items[existing] = { ...items[existing], quantity: items[existing].quantity + selectedQuantity };
      setForm({ ...form, items });
    } else {
      setForm({
        ...form,
        items: [...form.items, { product_id: product.product_id, product_name: product.name, quantity: selectedQuantity }],
      });
    }
    setSelectedProduct('');
    setSelectedQuantity(1);
  };

  const updateItemQuantity = (productID: number, quantity: number) => {
    if (quantity < 1) return;
    setForm({
      ...form,
      items: form.items.map((item) => (item.product_id === productID ? { ...item, quantity } : item)),
    });
  };

  const addAlias = () => {
    const alias = aliasInput.trim();
    if (!alias || form.aliases.some((entry) => entry.toLowerCase() === alias.toLowerCase())) return;
    setForm({ ...form, aliases: [...form.aliases, alias] });
    setAliasInput('');
  };

  const toggleWebsiteImage = (filename: string) => {
    const selected = form.website_images.includes(filename);
    const websiteImages = selected
      ? form.website_images.filter((entry) => entry !== filename)
      : [...form.website_images, filename];
    let thumbnail = form.website_thumbnail;
    if (selected && thumbnail === filename) thumbnail = websiteImages[0] || null;
    if (!selected && !thumbnail) thumbnail = filename;
    setForm({ ...form, website_images: websiteImages, website_thumbnail: thumbnail });
  };

  const deletePicture = async (picture: Picture) => {
    if (!editingID || !window.confirm(`Bild „${picture.file_name}“ wirklich löschen?`)) return;
    try {
      await api.delete(`/admin/product-packages/${editingID}/pictures/${encodeURIComponent(picture.file_name)}`);
      setPictures((current) => current.filter((entry) => entry.file_name !== picture.file_name));
      setForm((current) => ({
        ...current,
        website_images: current.website_images.filter((entry) => entry !== picture.file_name),
        website_thumbnail: current.website_thumbnail === picture.file_name ? null : current.website_thumbnail,
      }));
    } catch (error) {
      toast.error(errorMessage(error, 'Bild konnte nicht gelöscht werden.'));
    }
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setFormError(null);
    if (!form.name.trim()) {
      setFormError('Bitte gib einen Namen für das Paket an.');
      return;
    }
    if (form.items.length === 0) {
      setFormError('Ein Paket muss mindestens ein Produkt enthalten.');
      return;
    }
    setSubmitting(true);
    try {
      const payload = {
        name: form.name.trim(),
        description: form.description.trim() || null,
        price: form.price ? Number.parseFloat(form.price) : null,
        category: form.category.trim() || null,
        items: form.items.map(({ product_id, quantity }) => ({ product_id, quantity })),
        aliases: form.aliases,
        website_visible: form.website_visible,
      };
      let packageID = editingID;
      if (packageID) {
        await api.put(`/admin/product-packages/${packageID}`, payload);
      } else {
        const response = await api.post<{ package_id: number }>('/admin/product-packages', payload);
        packageID = response.data.package_id;
      }

      let websiteImages = [...form.website_images];
      let websiteThumbnail = form.website_thumbnail;
      if (newImages.length > 0) {
        const upload = new FormData();
        newImages.forEach((file) => upload.append('files', file));
        if (newThumbnailIndex !== null) upload.append('thumbnail_index', String(newThumbnailIndex));
        const response = await api.post<{ uploaded_files: string[]; thumbnail?: string }>(
          `/admin/product-packages/${packageID}/pictures`,
          upload,
          { headers: { 'Content-Type': 'multipart/form-data' } },
        );
        const uploaded = arrayOrEmpty(response.data.uploaded_files);
        websiteImages = Array.from(new Set([...websiteImages, ...uploaded]));
        if (newThumbnailIndex !== null && uploaded[newThumbnailIndex]) {
          websiteThumbnail = uploaded[newThumbnailIndex];
        } else if (!websiteThumbnail) {
          websiteThumbnail = response.data.thumbnail || uploaded[0] || null;
        }
      }

      await api.put(`/admin/product-packages/${packageID}/website`, {
        website_visible: form.website_visible,
        website_thumbnail: websiteThumbnail,
        website_images: websiteImages,
      });
      toast.success(editingID ? 'Produktpaket gespeichert.' : 'Produktpaket erstellt.');
      closeEditor();
      await fetchPackages();
    } catch (error) {
      const message = errorMessage(error, 'Produktpaket konnte nicht gespeichert werden.');
      setFormError(message);
      toast.error(message);
    } finally {
      setSubmitting(false);
    }
  };

  const removePackage = async (pkg: ProductPackage) => {
    if (!window.confirm(`Produktpaket „${pkg.name}“ wirklich löschen?`)) return;
    try {
      await api.delete(`/admin/product-packages/${pkg.package_id}`);
      toast.success('Produktpaket gelöscht.');
      await fetchPackages();
    } catch (error) {
      toast.error(errorMessage(error, 'Produktpaket konnte nicht gelöscht werden.'));
    }
  };

  const openDetails = async (pkg: ProductPackage) => {
    try {
      const response = await api.get<ProductPackageDetails>(`/admin/product-packages/${pkg.package_id}`);
      setViewPackage({ ...response.data, items: arrayOrEmpty(response.data.items), aliases: arrayOrEmpty(response.data.aliases) });
    } catch (error) {
      toast.error(errorMessage(error, 'Paketdetails konnten nicht geladen werden.'));
    }
  };

  const productName = (item: PackageItem) =>
    item.product_name || products.find((product) => product.product_id === item.product_id)?.name || `Produkt ${item.product_id}`;

  const availableProducts = products.filter((product) => !form.items.some((item) => item.product_id === product.product_id));
  const selectedProductEntry = products.find((product) => product.product_id === selectedProduct);
  const productLabel = (product: Product) => `${product.name}${product.category_name ? ` (${product.category_name})` : ''}`;

  return (
    <div className="product-packages-tab space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-2xl font-bold text-white">Produktpakete</h2>
          <p className="text-sm text-gray-400">Pakete, Inhalte, Bilder und Website-Freigabe verwalten</p>
        </div>
        <button className={`${primaryButtonClass} flex items-center justify-center gap-2`} onClick={() => void openEditor()}>
          <Plus className="h-4 w-4" /> Neues Produktpaket
        </button>
      </div>

      <div className="flex gap-3">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-gray-400" />
          <input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Pakete durchsuchen …"
            className="w-full rounded-lg border border-gray-700 bg-gray-800 py-2 pl-10 pr-4 text-white outline-none focus:ring-2 focus:ring-accent-red"
          />
        </div>
        <button className={`${secondaryButtonClass} px-3`} onClick={() => void fetchPackages()} aria-label="Aktualisieren">
          <RefreshCcw className="h-5 w-5" />
        </button>
      </div>

      {loading ? (
        <div className="py-12 text-center text-gray-400">Produktpakete werden geladen …</div>
      ) : packages.length === 0 ? (
        <div className="rounded-xl border border-dashed border-gray-700 py-12 text-center text-gray-400">Keine Produktpakete gefunden.</div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-gray-800">
          <table className="w-full">
            <thead className="bg-gray-800/70 text-left text-sm text-gray-300">
              <tr><th className="px-4 py-3">Code</th><th className="px-4 py-3">Name</th><th className="px-4 py-3">Inhalt</th><th className="px-4 py-3">Preis</th><th className="px-4 py-3">Website</th><th className="px-4 py-3 text-right">Aktionen</th></tr>
            </thead>
            <tbody>
              {packages.map((pkg) => (
                <tr key={pkg.package_id} className="border-t border-gray-800 hover:bg-white/[0.03]">
                  <td className="px-4 py-3 font-mono text-sm text-gray-400">{pkg.package_code}</td>
                  <td className="px-4 py-3 font-medium text-white">{pkg.name}</td>
                  <td className="px-4 py-3 text-gray-300">{pkg.total_items} Artikel</td>
                  <td className="px-4 py-3 text-gray-300">{formatPrice(pkg.price)}</td>
                  <td className="px-4 py-3"><span className={`rounded-full border px-2 py-1 text-xs ${pkg.website_visible ? 'border-accent-red/30 bg-accent-red/15 text-white' : 'border-white/10 bg-white/5 text-gray-400'}`}>{pkg.website_visible ? 'Sichtbar' : 'Ausgeblendet'}</span></td>
                  <td className="px-4 py-3"><div className="flex justify-end gap-1">
                    <button className={iconButtonClass} onClick={() => void openDetails(pkg)} title="Details"><Eye className="h-4 w-4" /></button>
                    <button className={iconButtonClass} onClick={() => void openEditor(pkg)} title="Bearbeiten"><Pencil className="h-4 w-4" /></button>
                    <button className="rounded-lg p-2 text-red-400 transition-colors hover:bg-accent-red/10 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red" onClick={() => void removePackage(pkg)} title="Löschen"><Trash2 className="h-4 w-4" /></button>
                  </div></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {modalOpen && (
        <ModalPortal>
          <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/80 p-4">
            <div className="glass-dark max-h-[92vh] w-full max-w-4xl overflow-y-auto rounded-2xl border border-white/10 p-6 shadow-2xl">
              <div className="mb-6 flex items-center justify-between">
                <h3 className="text-2xl font-bold text-white">{editingID ? 'Produktpaket bearbeiten' : 'Produktpaket erstellen'}</h3>
                <button onClick={closeEditor} className={iconButtonClass}><X className="h-6 w-6" /></button>
              </div>
              <form onSubmit={submit} className="space-y-6">
                {formError && <div className="rounded-lg border border-red-500/40 bg-red-500/10 p-3 text-red-300">{formError}</div>}
                <div className="grid gap-4 md:grid-cols-2">
                  <label className="space-y-2 text-sm font-semibold text-white">Name *<input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} className="w-full rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 font-normal outline-none focus:ring-2 focus:ring-accent-red" /></label>
                  <label className="space-y-2 text-sm font-semibold text-white">Preis pro Tag<input type="number" min="0" step="0.01" value={form.price} onChange={(event) => setForm({ ...form, price: event.target.value })} className="w-full rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 font-normal outline-none focus:ring-2 focus:ring-accent-red" /></label>
                  <label className="space-y-2 text-sm font-semibold text-white">Kategorie<input value={form.category} onChange={(event) => setForm({ ...form, category: event.target.value })} className="w-full rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 font-normal outline-none focus:ring-2 focus:ring-accent-red" /></label>
                  <label className="flex items-center gap-3 self-end rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 text-white transition-colors hover:border-accent-red/60"><input type="checkbox" checked={form.website_visible} onChange={(event) => setForm({ ...form, website_visible: event.target.checked })} className={checkboxClass} /><span>Auf der Website anzeigen</span></label>
                </div>
                <label className="block space-y-2 text-sm font-semibold text-white">Beschreibung<textarea rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} className="w-full rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 font-normal outline-none focus:ring-2 focus:ring-accent-red" /></label>

                <section className="space-y-3 border-t border-gray-700 pt-5">
                  <div><h4 className="text-lg font-semibold text-white">Produkte und Anzahl</h4><p className="text-sm text-gray-400">Jedes Paket enthält mindestens ein Produkt.</p></div>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <div ref={productDropdownRef} className="relative flex-1">
                      <button
                        type="button"
                        onClick={() => setProductDropdownOpen((open) => !open)}
                        aria-haspopup="listbox"
                        aria-expanded={productDropdownOpen}
                        className="flex w-full items-center justify-between gap-3 rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 text-left text-white transition-colors hover:border-accent-red/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red"
                      >
                        <span className={selectedProductEntry ? 'truncate text-white' : 'truncate text-gray-400'}>{selectedProductEntry ? productLabel(selectedProductEntry) : 'Produkt auswählen …'}</span>
                        <ChevronDown className={`h-4 w-4 shrink-0 text-gray-400 transition-transform ${productDropdownOpen ? 'rotate-180 text-white' : ''}`} />
                      </button>
                      {productDropdownOpen && (
                        <div role="listbox" className="absolute z-50 mt-2 max-h-72 w-full overflow-y-auto rounded-lg border border-white/10 bg-gray-900 p-1 shadow-2xl shadow-black/60">
                          {availableProducts.length === 0 ? (
                            <div className="px-3 py-2 text-sm text-gray-400">Keine weiteren Produkte verfügbar.</div>
                          ) : availableProducts.map((product) => (
                            <button
                              key={product.product_id}
                              type="button"
                              role="option"
                              aria-selected={selectedProduct === product.product_id}
                              onClick={() => {
                                setSelectedProduct(product.product_id);
                                setProductDropdownOpen(false);
                              }}
                              className={`block w-full rounded-md px-3 py-2 text-left text-sm text-white transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-red ${selectedProduct === product.product_id ? 'bg-accent-red text-white' : 'bg-gray-900 hover:bg-accent-red/20'}`}
                            >
                              {productLabel(product)}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                    <input type="number" min="1" value={selectedQuantity} onChange={(event) => setSelectedQuantity(Math.max(1, Number(event.target.value)))} className="w-28 rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 text-white focus:border-accent-red focus:ring-2 focus:ring-accent-red/20" aria-label="Anzahl" />
                    <button type="button" onClick={addItem} className={`${secondaryButtonClass} flex items-center justify-center gap-2`}><Plus className="h-4 w-4" /> Hinzufügen</button>
                  </div>
                  {form.items.length === 0 ? <p className="rounded-lg bg-gray-800 p-4 text-center text-gray-400">Noch keine Produkte zugewiesen.</p> : <div className="space-y-2">{form.items.map((item) => <div key={item.product_id} className="flex items-center gap-3 rounded-lg bg-gray-800 p-3"><span className="min-w-0 flex-1 truncate text-white">{productName(item)}</span><label className="flex items-center gap-2 text-sm text-gray-400">Anzahl<input type="number" min="1" value={item.quantity} onChange={(event) => updateItemQuantity(item.product_id, Math.max(1, Number(event.target.value)))} className="w-20 rounded border border-gray-600 bg-gray-900 px-2 py-1 text-white focus:border-accent-red focus:ring-2 focus:ring-accent-red/20" /></label><button type="button" onClick={() => setForm({ ...form, items: form.items.filter((entry) => entry.product_id !== item.product_id) })} className="rounded p-2 text-red-400 transition-colors hover:bg-accent-red/10 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red"><Trash2 className="h-4 w-4" /></button></div>)}</div>}
                </section>

                <section className="space-y-3 border-t border-gray-700 pt-5">
                  <div><h4 className="flex items-center gap-2 text-lg font-semibold text-white"><ImageIcon className="h-5 w-5" /> Bilder</h4><p className="text-sm text-gray-400">Bilder hochladen, für die Website auswählen und ein Vorschaubild festlegen.</p></div>
                  {pictures.length > 0 && <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{pictures.map((picture) => { const selected = form.website_images.includes(picture.file_name); const thumbnail = form.website_thumbnail === picture.file_name; return <div key={picture.file_name} className={`overflow-hidden rounded-lg border ${selected ? 'border-accent-red shadow-[0_0_0_1px_rgba(208,2,27,0.25)]' : 'border-gray-700'} bg-gray-800`}><img src={picture.thumbnail_url} alt={picture.file_name} className="h-32 w-full object-cover" /><div className="space-y-2 p-3"><p className="truncate text-xs text-gray-300" title={picture.file_name}>{picture.file_name}</p><label className="flex items-center gap-2 text-xs text-gray-300"><input type="checkbox" checked={selected} onChange={() => toggleWebsiteImage(picture.file_name)} className={checkboxClass} /> Website</label><div className="flex gap-1"><button type="button" disabled={!selected} onClick={() => setForm({ ...form, website_thumbnail: picture.file_name })} className={`flex flex-1 items-center justify-center gap-1 rounded border px-2 py-1 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red ${thumbnail ? 'border-accent-red/40 bg-accent-red/20 text-white' : 'border-white/10 bg-white/5 text-gray-300 hover:border-accent-red/50 hover:bg-accent-red/10 disabled:opacity-40'}`}><Star className="h-3 w-3" />{thumbnail ? 'Thumbnail' : 'Festlegen'}</button><button type="button" onClick={() => void deletePicture(picture)} className="rounded border border-accent-red/20 bg-accent-red/10 px-2 text-red-400 transition-colors hover:bg-accent-red/20 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red"><Trash2 className="h-3 w-3" /></button></div></div></div>; })}</div>}
                  <input type="file" accept="image/*" multiple onChange={(event) => setNewImages((current) => [...current, ...Array.from(event.target.files || [])])} className="block w-full rounded-lg border border-dashed border-gray-600 bg-gray-800 p-3 text-sm text-gray-300 file:mr-3 file:rounded file:border-0 file:bg-accent-red file:px-3 file:py-1 file:text-white" />
                  {newImages.length > 0 && <div className="space-y-2">{newImages.map((file, index) => <div key={`${file.name}-${index}`} className="flex items-center gap-3 rounded-lg bg-gray-800 p-3"><span className="min-w-0 flex-1 truncate text-sm text-white">{file.name}</span><button type="button" onClick={() => setNewThumbnailIndex(index)} className={`rounded border px-2 py-1 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red ${newThumbnailIndex === index ? 'border-accent-red/40 bg-accent-red/20 text-white' : 'border-white/10 bg-white/5 text-gray-300 hover:border-accent-red/50 hover:bg-accent-red/10'}`}><Star className="mr-1 inline h-3 w-3" />{newThumbnailIndex === index ? 'Thumbnail' : 'Als Thumbnail'}</button><button type="button" onClick={() => { setNewImages((current) => current.filter((_, itemIndex) => itemIndex !== index)); if (newThumbnailIndex === index) setNewThumbnailIndex(null); }} className="rounded p-1 text-red-400 transition-colors hover:bg-accent-red/10 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-red"><X className="h-4 w-4" /></button></div>)}</div>}
                </section>

                <section className="space-y-3 border-t border-gray-700 pt-5">
                  <h4 className="text-lg font-semibold text-white">OCR-Zuordnungen</h4>
                  <div className="flex gap-2"><input value={aliasInput} onChange={(event) => setAliasInput(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); addAlias(); } }} placeholder="z. B. Basic Audio Set" className="flex-1 rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 text-white focus:border-accent-red focus:ring-2 focus:ring-accent-red/20" /><button type="button" onClick={addAlias} className={secondaryButtonClass}>Hinzufügen</button></div>
                  <div className="flex flex-wrap gap-2">{form.aliases.map((alias) => <span key={alias} className="flex items-center gap-2 rounded-full bg-white/10 px-3 py-1 text-sm text-white">{alias}<button type="button" onClick={() => setForm({ ...form, aliases: form.aliases.filter((entry) => entry !== alias) })}><X className="h-3 w-3" /></button></span>)}</div>
                </section>

                <div className="flex gap-3 pt-2"><button type="button" onClick={closeEditor} disabled={submitting} className={`${secondaryButtonClass} flex-1`}>Abbrechen</button><button type="submit" disabled={submitting} className={`${primaryButtonClass} flex-1`}>{submitting ? 'Speichert …' : 'Speichern'}</button></div>
              </form>
            </div>
          </div>
        </ModalPortal>
      )}

      {viewPackage && (
        <ModalPortal>
          <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/80 p-4">
            <div className="glass-dark max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-2xl border border-white/10 p-6 shadow-2xl">
              <div className="mb-5 flex items-start justify-between"><div><h3 className="text-2xl font-bold text-white">{viewPackage.name}</h3><p className="font-mono text-sm text-gray-400">{viewPackage.package_code}</p></div><button onClick={() => setViewPackage(null)} className={iconButtonClass}><X className="h-6 w-6" /></button></div>
              <div className="space-y-5">
                {viewPackage.website_images && viewPackage.website_images.length > 0 && <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">{viewPackage.website_images.map((image) => <img key={image} src={`/api/v1/admin/product-packages/${viewPackage.package_id}/pictures/${encodeURIComponent(image)}?variant=preview`} alt={image} className="h-40 w-full rounded-lg object-cover" />)}</div>}
                <div className="grid gap-3 sm:grid-cols-3"><div className="rounded-lg bg-gray-800 p-3"><p className="text-xs text-gray-400">Preis</p><p className="font-semibold text-white">{formatPrice(viewPackage.price)}</p></div><div className="rounded-lg bg-gray-800 p-3"><p className="text-xs text-gray-400">Artikel</p><p className="font-semibold text-white">{viewPackage.total_items}</p></div><div className="rounded-lg bg-gray-800 p-3"><p className="text-xs text-gray-400">Website</p><p className="font-semibold text-white">{viewPackage.website_visible ? 'Sichtbar' : 'Ausgeblendet'}</p></div></div>
                {viewPackage.description && <div><h4 className="mb-1 text-sm font-semibold text-gray-400">Beschreibung</h4><p className="whitespace-pre-wrap text-white">{viewPackage.description}</p></div>}
                <div><h4 className="mb-2 text-lg font-semibold text-white">Enthaltene Produkte</h4><div className="space-y-2">{viewPackage.items.map((item) => <div key={item.package_item_id || item.product_id} className="flex justify-between rounded-lg bg-gray-800 p-3"><span className="text-white">{productName(item)}</span><span className="font-semibold text-white">× {item.quantity}</span></div>)}</div></div>
                {arrayOrEmpty(viewPackage.aliases).length > 0 && <div><h4 className="mb-2 text-sm font-semibold text-gray-400">OCR-Zuordnungen</h4><div className="flex flex-wrap gap-2">{viewPackage.aliases?.map((alias) => <span key={alias} className="rounded-full bg-white/10 px-3 py-1 text-sm text-white">{alias}</span>)}</div></div>}
              </div>
            </div>
          </div>
        </ModalPortal>
      )}
    </div>
  );
}
