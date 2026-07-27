import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { X, Package, Ruler, Weight, Zap, Tag, Box, DollarSign, Wrench, Barcode, Info, Image as ImageIcon, UploadCloud, Loader2, Eye, Cpu, MapPin } from 'lucide-react';
import { ModalPortal } from './ModalPortal';
import { useBlockBodyScroll } from '../hooks/useBlockBodyScroll';
import { productPicturesApi, productWebsiteApi, api, ledApi, devicesApi } from '../lib/api';
import type { ChangeEvent } from 'react';
import type { ProductPicture, Device } from '../lib/api';
import { DeviceDetailModal } from './DeviceDetailModal';
import { formatStatus, getStatusColor } from '../lib/utils';
import { toast } from '../lib/toast';

export interface ProductDetail {
  product_id: number;
  name: string;
  description?: string;
  category_name?: string;
  subcategory_name?: string;
  subbiercategory_name?: string;
  brand_name?: string;
  manufacturer_name?: string;
  item_cost_per_day?: number;
  maintenance_interval?: number;
  weight?: number;
  height?: number;
  width?: number;
  depth?: number;
  power_consumption?: number;
  pos_in_category?: number;
  is_accessory?: boolean;
  is_consumable?: boolean;
  stock_quantity?: number;
  min_stock_level?: number;
  generic_barcode?: string;
  price_per_unit?: number;
  count_type_abbreviation?: string;
  device_count?: number;
  website_visible?: boolean;
  website_thumbnail?: string | null;
  website_images?: string[];
}

interface ProductDetailModalProps {
  product: ProductDetail | null;
  isOpen: boolean;
  onClose: () => void;
}

export function ProductDetailModal({ product, isOpen, onClose }: ProductDetailModalProps) {
  useBlockBodyScroll(isOpen);

  const [pictures, setPictures] = useState<ProductPicture[]>([]);
  const [loadingPictures, setLoadingPictures] = useState(false);
  const [uploadingPictures, setUploadingPictures] = useState(false);
  const [pictureError, setPictureError] = useState<string | null>(null);
  const [picturesUnavailable, setPicturesUnavailable] = useState(false);
  const [previewIndex, setPreviewIndex] = useState<number | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [lightboxLoading, setLightboxLoading] = useState(false);
  const [websiteVisible, setWebsiteVisible] = useState(false);
  const [selectedImages, setSelectedImages] = useState<Set<string>>(new Set());
  const [websiteThumbnail, setWebsiteThumbnail] = useState<string | null>(null);
  const [savingWebsite, setSavingWebsite] = useState(false);
  const [websiteMessage, setWebsiteMessage] = useState<string | null>(null);

  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<'details' | 'devices'>('details');
  const [devices, setDevices] = useState<Device[]>([]);
  const [loadingDevices, setLoadingDevices] = useState(false);
  const [devicesLoaded, setDevicesLoaded] = useState(false);
  const [selectedDevice, setSelectedDevice] = useState<Device | null>(null);
  const [deviceDetailOpen, setDeviceDetailOpen] = useState(false);

  const formatCurrency = (value?: number) => {
    if (value == null) return '—';
    return new Intl.NumberFormat('de-DE', { style: 'currency', currency: 'EUR' }).format(value);
  };

  const formatMeasurement = (value?: number, unit?: string) => {
    if (value == null) return '—';
    return `${value.toFixed(2)} ${unit || ''}`;
  };

  const categoryPath = () => {
    if (!product) return '—';
    const parts = [product.category_name, product.subcategory_name, product.subbiercategory_name].filter(Boolean);
    return parts.length > 0 ? parts.join(' › ') : '—';
  };

  const loadPictures = async () => {
    if (!product) return;
    setLoadingPictures(true);
    setPictureError(null);
    setPicturesUnavailable(false);
    try {
      const response = await productPicturesApi.list(product.product_id);
      setPictures(response.data.pictures || []);
    } catch (error) {
      toast.error('Failed to load product pictures' + " " + String(error));
      const status = (error as { response?: { status?: number } })?.response?.status;
      if (status === 503 || status === 500) {
        setPicturesUnavailable(true);
        setPictureError('Bilderablage ist nicht erreichbar oder nicht konfiguriert.');
      } else {
        setPictureError('Bilder konnten nicht geladen werden.');
      }
      setPictures([]);
    } finally {
      setLoadingPictures(false);
    }
  };

  useEffect(() => {
    if (isOpen && product) {
      loadPictures();
      setWebsiteVisible(Boolean(product.website_visible));
      setSelectedImages(new Set(product.website_images || []));
      setWebsiteThumbnail(product.website_thumbnail || null);
      setWebsiteMessage(null);
      setActiveTab('details');
      setDevices([]);
      setDevicesLoaded(false);
    } else {
      setPictures([]);
      setPictureError(null);
      setSelectedImages(new Set());
      setWebsiteThumbnail(null);
      setWebsiteMessage(null);
      setDevices([]);
      setDevicesLoaded(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, product?.product_id]);

  const loadDevices = async () => {
    if (!product || devicesLoaded) return;
    setLoadingDevices(true);
    try {
      const { data } = await api.get<Device[]>(`/admin/products/${product.product_id}/devices`);
      setDevices(data);
      setDevicesLoaded(true);
    } catch (error) {
      toast.error('Failed to load devices:' + " " + String(error));
    } finally {
      setLoadingDevices(false);
    }
  };

  const handleLocateDevice = async (device: Device) => {
    if (!device.zone_code) return;
    try {
      await ledApi.locateBin(device.zone_code);
    } catch (error) {
      toast.error('LED locate failed:' + " " + String(error));
    }
  };

  const handleOpenZone = (device: Device) => {
    if (device.zone_id) {
      navigate(`/zones/${device.zone_id}`);
    }
  };

  const handleOpenDevice = async (device: Device) => {
    try {
      const { data } = await devicesApi.getById(device.device_id);
      setSelectedDevice(data);
      setDeviceDetailOpen(true);
    } catch (error) {
      toast.error('Failed to load device:' + " " + String(error));
    }
  };

  const handleTabChange = (tab: 'details' | 'devices') => {
    setActiveTab(tab);
    if (tab === 'devices' && !devicesLoaded) {
      loadDevices();
    }
  };

  const handleUploadPictures = async (event: ChangeEvent<HTMLInputElement>) => {
    if (!product) return;
    const files = event.target.files;
    if (!files || files.length === 0) return;

    const now = new Date().toISOString();
    const tempPreviews: ProductPicture[] = Array.from(files as ArrayLike<File>).map(file => {
      const url = URL.createObjectURL(file);
      return {
        file_name: file.name,
        size: file.size,
        content_type: file.type || 'image/*',
        modified_at: now,
        download_url: url,
        thumbnail_url: url,
        preview_url: url,
        temporary: true,
      };
    });
    if (tempPreviews.length > 0) {
      setPictures(prev => [...tempPreviews, ...prev]);
    }

    setUploadingPictures(true);
    setPictureError(null);
    try {
      await productPicturesApi.upload(product.product_id, files);
      await loadPictures();
    } catch (error) {
      toast.error('Failed to upload product pictures' + " " + String(error));
      setPictureError('Upload fehlgeschlagen. Bitte erneut versuchen.');
      setPictures(prev => prev.filter(pic => !pic.temporary));
    } finally {
      tempPreviews.forEach(pic => {
        if (pic.download_url.startsWith('blob:')) {
          URL.revokeObjectURL(pic.download_url);
        }
      });
      setUploadingPictures(false);
      event.target.value = '';
    }
  };

  const handleDeletePicture = async (fileName: string) => {
    if (!product) return;
    setDeleting(fileName);
    setPictureError(null);
    try {
      await productPicturesApi.delete(product.product_id, fileName);
      await loadPictures();
      if (previewIndex !== null && pictures[previewIndex]?.file_name === fileName) {
        setPreviewIndex(null);
      }
    } catch (error) {
      toast.error('Failed to delete product picture' + " " + String(error));
      setPictureError('Löschen fehlgeschlagen. Bitte erneut versuchen.');
    } finally {
      setDeleting(null);
    }
  };

  const toggleWebsiteImage = (fileName: string) => {
    setSelectedImages(prev => {
      const next = new Set(prev);
      if (next.has(fileName)) {
        next.delete(fileName);
        if (websiteThumbnail === fileName) {
          setWebsiteThumbnail(null);
        }
      } else {
        next.add(fileName);
      }
      return next;
    });
    setWebsiteMessage(null);
  };

  const persistWebsiteSettings = async (overrideVisible?: boolean) => {
    if (!product) return;
    setSavingWebsite(true);
    setWebsiteMessage(null);
    setPictureError(null);
    const images = Array.from(selectedImages);
    try {
      await productWebsiteApi.update(product.product_id, {
        website_visible: overrideVisible ?? websiteVisible,
        website_images: images,
        website_thumbnail: websiteThumbnail ?? undefined,
      });
      setWebsiteMessage('Website-Einstellungen gespeichert');
    } catch (error) {
      toast.error('Failed to save website settings' + " " + String(error));
      setPictureError('Website-Einstellungen konnten nicht gespeichert werden.');
    } finally {
      setSavingWebsite(false);
    }
  };

  const handleSaveWebsite = async () => {
    await persistWebsiteSettings();
  };

  useEffect(() => {
    if (previewIndex === null || pictures.length === 0) return;
    const current = pictures[previewIndex];
    setLightboxLoading(false);
    // Preload next/prev lightly without blocking
    const preload = (url: string) => {
      if (!url) return;
      const img = new Image();
      img.src = url;
    };
    const next = pictures[(previewIndex + 1) % pictures.length];
    const prev = pictures[(previewIndex - 1 + pictures.length) % pictures.length];
    preload(current.download_url);
    preload(next.download_url);
    preload(prev.download_url);
  }, [previewIndex, pictures]);

  const formatDate = (iso: string) => {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '—';
    return date.toLocaleString('de-DE');
  };

  if (!isOpen || !product) return null;

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
        <div className="glass-dark w-full max-w-4xl rounded-2xl shadow-2xl border border-white/10 flex flex-col max-h-[90vh]">
          {/* Header */}
          <div className="flex items-center justify-between p-6 border-b border-white/10">
            <div className="flex items-center gap-3">
              <Package className="w-8 h-8 text-accent-red" />
              <div>
                <h2 className="text-2xl font-bold text-white">{product.name}</h2>
                <p className="text-sm text-gray-400">
                  Produkt ID: {product.product_id}
                  {product.device_count !== undefined && ` • ${product.device_count} Geräte`}
                </p>
              </div>
            </div>
            <button
              onClick={onClose}
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-white/10 text-white hover:bg-white/20 transition-colors"
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          {/* Tabs */}
          <div className="flex border-b border-white/10 px-6">
            <button
              onClick={() => handleTabChange('details')}
              className={`px-4 py-3 text-sm font-semibold border-b-2 transition-colors ${
                activeTab === 'details'
                  ? 'border-accent-red text-white'
                  : 'border-transparent text-gray-400 hover:text-white'
              }`}
            >
              Details
            </button>
            <button
              onClick={() => handleTabChange('devices')}
              className={`flex items-center gap-1.5 px-4 py-3 text-sm font-semibold border-b-2 transition-colors ${
                activeTab === 'devices'
                  ? 'border-accent-red text-white'
                  : 'border-transparent text-gray-400 hover:text-white'
              }`}
            >
              <Cpu className="w-4 h-4" />
              Geräte
              {product.device_count !== undefined && product.device_count > 0 && (
                <span className="ml-1 rounded-full bg-accent-red/20 px-1.5 py-0.5 text-xs text-accent-red">
                  {product.device_count}
                </span>
              )}
            </button>
          </div>

          {/* Content */}
          {activeTab === 'details' ? (
          <div className="overflow-y-auto p-6 space-y-6">
            <div className="glass rounded-xl p-4">
              <div className="flex items-center justify-between mb-3 gap-3">
                <div className="flex items-center gap-2">
                  <ImageIcon className="w-5 h-5 text-accent-red" />
                  <h3 className="text-lg font-semibold text-white">Produktbilder</h3>
                </div>
                <label className="inline-flex items-center gap-2 rounded-lg bg-white/10 px-3 py-2 text-sm font-semibold text-white cursor-pointer hover:bg-white/20 transition disabled:opacity-60 disabled:cursor-not-allowed">
                  <UploadCloud className="w-4 h-4" />
                  <span>{uploadingPictures ? 'Lädt...' : 'Bilder hochladen'}</span>
                  <input
                    type="file"
                    accept="image/*"
                    multiple
                    onChange={handleUploadPictures}
                    disabled={uploadingPictures || picturesUnavailable}
                    className="hidden"
                  />
                </label>
              </div>
              {pictureError && <p className="text-sm text-red-400 mb-2">{pictureError}</p>}
              {picturesUnavailable ? (
                <p className="text-gray-400">Bilderablage ist nicht eingerichtet.</p>
              ) : loadingPictures ? (
                <div className="flex items-center gap-2 text-gray-300">
                  <Loader2 className="w-4 h-4 animate-spin" />
                  <span>Bilder werden geladen...</span>
                </div>
              ) : pictures.length === 0 ? (
                <p className="text-gray-400">Noch keine Bilder hochgeladen.</p>
              ) : (
                <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
                  {pictures.map((picture, index) => (
                    <div
                      key={picture.file_name}
                      className="group relative overflow-hidden rounded-lg border border-white/10 bg-white/5 cursor-zoom-in"
                      onClick={() => setPreviewIndex(index)}
                    >
                      <img
                        src={picture.thumbnail_url || picture.preview_url || picture.download_url}
                        alt={`${product.name} Bild`}
                        className="h-36 w-full object-cover transition duration-300 group-hover:scale-105"
                        loading="lazy"
                      />
                      <button
                        type="button"
                        onClick={event => {
                          event.stopPropagation();
                          handleDeletePicture(picture.file_name);
                        }}
                        className="absolute right-2 top-2 rounded-full bg-black/70 px-2 py-1 text-xs text-white opacity-90 transition group-hover:opacity-100 disabled:opacity-50"
                        disabled={deleting === picture.file_name}
                        title="Bild löschen"
                      >
                        {deleting === picture.file_name ? '...' : 'Löschen'}
                      </button>
                      <div className="absolute inset-0 bg-black/50 opacity-0 transition-opacity duration-200 group-hover:opacity-100" />
                      <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/80 to-transparent p-2 text-xs text-white">
                        <p className="font-semibold break-all">{picture.file_name}</p>
                        <p className="text-gray-300">{formatDate(picture.modified_at)}</p>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Website Settings */}
            <div className="glass rounded-xl p-4 space-y-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Eye className="w-5 h-5 text-accent-red" />
                  <h3 className="text-lg font-semibold text-white">Website</h3>
                </div>
                <label className="flex items-center gap-2 text-sm text-gray-200 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    className="h-4 w-4 rounded border-gray-600 bg-gray-900 accent-accent-red focus:ring-accent-red"
                    checked={websiteVisible}
                    onChange={e => {
                      const next = e.target.checked;
                      setWebsiteVisible(next);
                      void persistWebsiteSettings(next);
                    }}
                  />
                  Auf Website anzeigen
                </label>
              </div>
              {pictures.length > 0 ? (
                <div className="space-y-2">
                  <p className="text-sm text-gray-400">Bilder auswählen und Thumbnail festlegen:</p>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    {pictures.map(pic => (
                      <div key={pic.file_name} className="flex items-center gap-3 rounded-lg border border-white/10 bg-white/5 p-2 min-w-0">
                        <img
                          src={pic.thumbnail_url || pic.preview_url || pic.download_url}
                          alt=""
                          className="h-16 w-16 rounded object-cover flex-shrink-0"
                          loading="lazy"
                        />
                        <div className="flex-1 min-w-0">
                          <p className="text-sm text-white font-semibold break-all">{pic.file_name}</p>
                          <div className="flex items-center gap-3 mt-1 flex-wrap">
                            <label className="flex items-center gap-1 text-xs text-gray-200">
                              <input
                                type="checkbox"
                                checked={selectedImages.has(pic.file_name)}
                                onChange={() => toggleWebsiteImage(pic.file_name)}
                                className="accent-accent-red focus:ring-accent-red"
                              />
                              Auf Website
                            </label>
                            <label className="flex items-center gap-1 text-xs text-gray-200">
                              <input
                                type="radio"
                                name="website-thumb"
                                disabled={!selectedImages.has(pic.file_name)}
                                checked={websiteThumbnail === pic.file_name}
                                className="accent-accent-red focus:ring-accent-red"
                                onChange={() => {
                                  if (!selectedImages.has(pic.file_name)) toggleWebsiteImage(pic.file_name);
                                  setWebsiteThumbnail(pic.file_name);
                                  setWebsiteMessage(null);
                                }}
                              />
                              Thumbnail
                            </label>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="flex items-center justify-end gap-3">
                    {websiteMessage && <span className="text-xs text-green-400">{websiteMessage}</span>}
                    <button
                      onClick={handleSaveWebsite}
                      disabled={savingWebsite}
                      className="px-4 py-2 rounded-lg bg-accent-red text-white text-sm font-semibold hover:bg-accent-red/90 transition disabled:opacity-60"
                    >
                      {savingWebsite ? 'Speichert...' : 'Website speichern'}
                    </button>
                  </div>
                </div>
              ) : (
                <p className="text-sm text-gray-400">Keine Bilder vorhanden. Bitte zuerst Bilder hochladen.</p>
              )}
            </div>

            {/* Type Badges */}
            <div className="flex gap-2">
              {product.is_consumable && (
                <span className="rounded-full border border-accent-red/30 bg-accent-red/15 px-3 py-1 text-sm font-semibold text-white">
                  Verbrauchsmaterial
                </span>
              )}
              {product.is_accessory && (
                <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1 text-sm font-semibold text-white">
                  Zubehör
                </span>
              )}
              {!product.is_consumable && !product.is_accessory && (
                <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1 text-sm font-semibold text-white">
                  Standard-Produkt
                </span>
              )}
            </div>

            {/* Description */}
            {product.description && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-2 mb-2">
                  <Info className="w-5 h-5 text-accent-red" />
                  <h3 className="text-lg font-semibold text-white">Beschreibung</h3>
                </div>
                <p className="text-gray-300">{product.description}</p>
              </div>
            )}

            {/* Category & Classification */}
            <div className="glass rounded-xl p-4">
              <div className="flex items-center gap-2 mb-3">
                <Tag className="w-5 h-5 text-accent-red" />
                <h3 className="text-lg font-semibold text-white">Kategorisierung</h3>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <p className="text-sm text-gray-400">Kategoriepfad</p>
                  <p className="text-white font-medium">{categoryPath()}</p>
                </div>
                {product.brand_name && (
                  <div>
                    <p className="text-sm text-gray-400">Marke</p>
                    <p className="text-white font-medium">{product.brand_name}</p>
                  </div>
                )}
                {product.manufacturer_name && (
                  <div>
                    <p className="text-sm text-gray-400">Hersteller</p>
                    <p className="text-white font-medium">{product.manufacturer_name}</p>
                  </div>
                )}
                {product.pos_in_category != null && (
                  <div>
                    <p className="text-sm text-gray-400">Position in Kategorie</p>
                    <p className="text-white font-medium">{product.pos_in_category}</p>
                  </div>
                )}
              </div>
            </div>

            {/* Pricing */}
            {(product.item_cost_per_day != null || product.price_per_unit != null) && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-2 mb-3">
                  <DollarSign className="w-5 h-5 text-accent-red" />
                  <h3 className="text-lg font-semibold text-white">Preise</h3>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  {product.item_cost_per_day != null && (
                    <div className="bg-white/5 rounded-lg p-3">
                      <p className="text-sm text-gray-400">Preis pro Tag</p>
                      <p className="text-2xl font-bold text-white">{formatCurrency(product.item_cost_per_day)}</p>
                    </div>
                  )}
                  {product.price_per_unit != null && (
                    <div className="bg-white/5 rounded-lg p-3">
                      <p className="text-sm text-gray-400">Preis pro Einheit</p>
                      <p className="text-2xl font-bold text-white">{formatCurrency(product.price_per_unit)}</p>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Stock Information (for consumables/accessories) */}
            {(product.is_consumable || product.is_accessory) && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-2 mb-3">
                  <Box className="w-5 h-5 text-accent-red" />
                  <h3 className="text-lg font-semibold text-white">Lagerbestand</h3>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                  <div className="bg-white/5 rounded-lg p-3">
                    <p className="text-sm text-gray-400">Aktueller Bestand</p>
                    <p className="text-xl font-bold text-white">
                      {product.stock_quantity != null ? product.stock_quantity : '—'} {product.count_type_abbreviation || ''}
                    </p>
                  </div>
                  <div className="bg-white/5 rounded-lg p-3">
                    <p className="text-sm text-gray-400">Mindestbestand</p>
                    <p className="text-xl font-bold text-white">
                      {product.min_stock_level != null ? product.min_stock_level : '—'} {product.count_type_abbreviation || ''}
                    </p>
                  </div>
                  {product.generic_barcode && (
                    <div className="bg-white/5 rounded-lg p-3">
                      <p className="text-sm text-gray-400">Barcode</p>
                      <p className="text-sm font-mono text-white">{product.generic_barcode}</p>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Physical Properties */}
            {(product.weight != null || product.height != null || product.width != null || product.depth != null) && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-2 mb-3">
                  <Ruler className="w-5 h-5 text-accent-red" />
                  <h3 className="text-lg font-semibold text-white">Physische Eigenschaften</h3>
                </div>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                  {product.weight != null && (
                    <div className="bg-white/5 rounded-lg p-3">
                      <div className="flex items-center gap-2 mb-1">
                        <Weight className="w-4 h-4 text-gray-400" />
                        <p className="text-sm text-gray-400">Gewicht</p>
                      </div>
                      <p className="text-lg font-semibold text-white">{formatMeasurement(product.weight, 'kg')}</p>
                    </div>
                  )}
                  {product.height != null && (
                    <div className="bg-white/5 rounded-lg p-3">
                      <p className="text-sm text-gray-400">Höhe</p>
                      <p className="text-lg font-semibold text-white">{formatMeasurement(product.height, 'cm')}</p>
                    </div>
                  )}
                  {product.width != null && (
                    <div className="bg-white/5 rounded-lg p-3">
                      <p className="text-sm text-gray-400">Breite</p>
                      <p className="text-lg font-semibold text-white">{formatMeasurement(product.width, 'cm')}</p>
                    </div>
                  )}
                  {product.depth != null && (
                    <div className="bg-white/5 rounded-lg p-3">
                      <p className="text-sm text-gray-400">Tiefe</p>
                      <p className="text-lg font-semibold text-white">{formatMeasurement(product.depth, 'cm')}</p>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Technical Details */}
            {(product.power_consumption != null || product.maintenance_interval != null) && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-2 mb-3">
                  <Zap className="w-5 h-5 text-accent-red" />
                  <h3 className="text-lg font-semibold text-white">Technische Details</h3>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  {product.power_consumption != null && (
                    <div className="bg-white/5 rounded-lg p-3">
                      <p className="text-sm text-gray-400">Leistungsaufnahme</p>
                      <p className="text-lg font-semibold text-white">{formatMeasurement(product.power_consumption, 'W')}</p>
                    </div>
                  )}
                  {product.maintenance_interval != null && (
                    <div className="bg-white/5 rounded-lg p-3">
                      <div className="flex items-center gap-2 mb-1">
                        <Wrench className="w-4 h-4 text-gray-400" />
                        <p className="text-sm text-gray-400">Wartungsintervall</p>
                      </div>
                      <p className="text-lg font-semibold text-white">{product.maintenance_interval} Tage</p>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Barcode */}
            {product.generic_barcode && !product.is_consumable && !product.is_accessory && (
              <div className="glass rounded-xl p-4">
                <div className="flex items-center gap-2 mb-3">
                  <Barcode className="w-5 h-5 text-accent-red" />
                  <h3 className="text-lg font-semibold text-white">Identifikation</h3>
                </div>
                <div className="bg-white/5 rounded-lg p-3">
                  <p className="text-sm text-gray-400">Generischer Barcode</p>
                  <p className="text-lg font-mono text-white">{product.generic_barcode}</p>
                </div>
              </div>
            )}
          </div>
          ) : (
          <div className="overflow-y-auto p-6">
            {loadingDevices ? (
              <div className="flex items-center justify-center gap-2 py-12 text-gray-400">
                <Loader2 className="w-5 h-5 animate-spin" />
                <span>Geräte werden geladen...</span>
              </div>
            ) : devices.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-3 py-12 text-gray-500">
                <Cpu className="w-10 h-10 text-gray-600" />
                <p className="text-sm">Keine Geräte diesem Produkt zugeordnet.</p>
              </div>
            ) : (
              <div className="space-y-2">
                <p className="text-sm text-gray-400 mb-4">{devices.length} Gerät{devices.length === 1 ? '' : 'e'}</p>
                {devices.map((device) => (
                  <div
                    key={device.device_id}
                    onClick={() => handleOpenDevice(device)}
                    className="glass rounded-lg p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 border border-white/10 cursor-pointer hover:bg-white/10 transition-colors"
                  >
                    <div className="flex-1 min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-semibold text-white text-sm truncate">{device.device_id}</span>
                        {device.status && (
                          <span className={`text-[10px] font-semibold px-2 py-1 rounded-full bg-white/10 uppercase tracking-wide ${getStatusColor(device.status)}`}>
                            {formatStatus(device.status)}
                          </span>
                        )}
                      </div>
                      <div className="flex flex-wrap items-center gap-2 text-xs text-gray-400 mt-1">
                        {device.zone_name && (
                          <span className="flex items-center gap-1">
                            <MapPin className="w-3 h-3" />
                            {device.zone_name}
                          </span>
                        )}
                        {device.zone_code && (
                          <span className="text-gray-500 font-mono">({device.zone_code})</span>
                        )}
                        {device.case_name && <span>📦 {device.case_name}</span>}
                        {device.job_number && <span>🔧 Job #{device.job_number}</span>}
                        {device.serial_number && <span>SN: {device.serial_number}</span>}
                        {device.barcode && <span>Barcode: {device.barcode}</span>}
                      </div>
                    </div>
                    <div className="flex gap-2" onClick={(e) => e.stopPropagation()}>
                      <button
                        onClick={() => handleLocateDevice(device)}
                        className="flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-semibold bg-white/10 hover:bg-white/20 transition-colors"
                        title="Fach aufleuchten"
                      >
                        <Cpu className="w-4 h-4 text-yellow-300" />
                      </button>
                      <button
                        onClick={() => handleOpenZone(device)}
                        className="flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-semibold bg-accent-red/80 hover:bg-accent-red transition-colors text-white"
                        title="Zone öffnen"
                      >
                        <MapPin className="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
          )}

          <DeviceDetailModal
            device={selectedDevice}
            isOpen={deviceDetailOpen}
            onClose={() => setDeviceDetailOpen(false)}
          />

          {previewIndex !== null && pictures[previewIndex] && (
            <div className="fixed inset-0 z-[130] flex items-center justify-center bg-black/90 p-4">
              <div className="relative w-full max-w-5xl">
                <button
                  onClick={() => setPreviewIndex(null)}
                  className="absolute right-4 top-4 rounded-full bg-white/10 px-3 py-2 text-sm font-semibold text-white hover:bg-white/20"
                >
                  Schließen
                </button>
                <div className="absolute left-2 top-1/2 -translate-y-1/2">
                  <button
                    onClick={() =>
                      setPreviewIndex(prev => {
                        if (prev === null) return prev;
                        return (prev - 1 + pictures.length) % pictures.length;
                      })
                    }
                    className="rounded-full bg-white/10 px-3 py-2 text-white hover:bg-white/20"
                  >
                    ‹
                  </button>
                </div>
                <div className="absolute right-2 top-1/2 -translate-y-1/2">
                  <button
                    onClick={() =>
                      setPreviewIndex(prev => {
                        if (prev === null) return prev;
                        return (prev + 1) % pictures.length;
                      })
                    }
                    className="rounded-full bg-white/10 px-3 py-2 text-white hover:bg-white/20"
                  >
                    ›
                  </button>
                </div>
                <div className="overflow-hidden rounded-xl border border-white/10 bg-white/5">
                  <img
                    src={pictures[previewIndex].download_url}
                    alt={pictures[previewIndex].file_name}
                    className="max-h-[80vh] w-full object-contain bg-black"
                    onLoad={() => setLightboxLoading(false)}
                    loading="eager"
                  />
                  {lightboxLoading && (
                    <div className="absolute inset-0 flex items-center justify-center bg-black/60 text-white">
                      Lädt...
                    </div>
                  )}
                </div>
                <div className="mt-3 flex items-center justify-between text-sm text-gray-200">
                  <div>
                    <p className="font-semibold text-white">{pictures[previewIndex].file_name}</p>
                    <p>{formatDate(pictures[previewIndex].modified_at)}</p>
                  </div>
                  <button
                    onClick={() => handleDeletePicture(pictures[previewIndex].file_name)}
                    className="rounded-lg bg-red-600 px-4 py-2 font-semibold text-white hover:bg-red-700 disabled:opacity-60"
                    disabled={deleting === pictures[previewIndex].file_name}
                  >
                    {deleting === pictures[previewIndex].file_name ? 'Löscht...' : 'Bild löschen'}
                  </button>
                </div>
              </div>
            </div>
          )}

          {/* Footer */}
          <div className="flex justify-end gap-3 p-6 border-t border-white/10">
            <button
              onClick={onClose}
              className="px-6 py-2 rounded-lg text-sm font-semibold bg-white/10 text-white hover:bg-white/20 transition-colors"
            >
              Schließen
            </button>
          </div>
        </div>
      </div>
    </ModalPortal>
  );
}
