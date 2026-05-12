import { useEffect, useState } from 'react';
import { X, Plus, Trash2, Package, AlertCircle } from 'lucide-react';
import { api } from '../lib/api';
import { ModalPortal } from './ModalPortal';
import { useBlockBodyScroll } from '../hooks/useBlockBodyScroll';

interface ProductDependency {
  id: number;
  product_id: number;
  dependency_product_id: number;
  dependency_name: string;
  is_accessory: boolean;
  is_consumable: boolean;
  generic_barcode?: string;
  count_type_abbr?: string;
  stock_quantity?: number;
  is_optional: boolean;
  default_quantity: number;
  notes?: string;
  created_at: string;
}

interface AvailableProduct {
  product_id: number;
  name: string;
  is_accessory: boolean;
  is_consumable: boolean;
  generic_barcode?: string;
  count_type_abbr?: string;
  stock_quantity?: number;
}

interface Props {
  productId: number;
  productName: string;
  onClose: () => void;
}

export function ProductDependenciesModal({ productId, productName, onClose }: Props) {
  const [dependencies, setDependencies] = useState<ProductDependency[]>([]);
  const [availableProducts, setAvailableProducts] = useState<AvailableProduct[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddForm, setShowAddForm] = useState(false);
  const [selectedProductId, setSelectedProductId] = useState<number | null>(null);
  const [defaultQuantity, setDefaultQuantity] = useState(1);
  const [isOptional, setIsOptional] = useState(true);
  const [notes, setNotes] = useState('');
  const [searchTerm, setSearchTerm] = useState('');

  // Block body scroll when modal is open
  useBlockBodyScroll(true);

  useEffect(() => {
    loadDependencies();
    loadAvailableProducts();
  }, [productId]);

  const loadDependencies = async () => {
    try {
      const { data } = await api.get(`/admin/products/${productId}/dependencies`);
      setDependencies(data);
    } catch (err) {
      console.error('Failed to load dependencies:', err);
    } finally {
      setLoading(false);
    }
  };

  const loadAvailableProducts = async () => {
    try {
      // Fetch all accessories and consumables
      const { data } = await api.get('/admin/products');
      const filtered = data.filter((p: AvailableProduct) =>
        (p.is_accessory || p.is_consumable) && p.product_id !== productId
      );
      setAvailableProducts(filtered);
    } catch (err) {
      console.error('Failed to load available products:', err);
    }
  };

  const handleAddDependency = async () => {
    if (!selectedProductId) return;

    try {
      const { data } = await api.post(`/admin/products/${productId}/dependencies`, {
        dependency_product_id: selectedProductId,
        is_optional: isOptional,
        default_quantity: defaultQuantity,
        notes: notes || null,
      });

      setDependencies([data, ...dependencies]);
      setShowAddForm(false);
      setSelectedProductId(null);
      setDefaultQuantity(1);
      setIsOptional(true);
      setNotes('');
      setSearchTerm('');
    } catch (err) {
      console.error('Failed to add dependency:', err);
      alert('Failed to add dependency');
    }
  };

  const handleDeleteDependency = async (depId: number) => {
    if (!confirm('Remove this dependency?')) return;

    try {
      await api.delete(`/admin/products/${productId}/dependencies/${depId}`);
      setDependencies(dependencies.filter(d => d.id !== depId));
    } catch (err) {
      console.error('Failed to delete dependency:', err);
      alert('Failed to delete dependency');
    }
  };

  const filteredProducts = availableProducts.filter(p =>
    p.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    p.generic_barcode?.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const existingDepIds = dependencies.map(d => d.dependency_product_id);
  const availableToAdd = filteredProducts.filter(p => !existingDepIds.includes(p.product_id));

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
        <div className="bg-[#111111]/98 backdrop-blur-xl border border-white/10 rounded-xl max-w-3xl w-full max-h-[90vh] overflow-hidden flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div>
            <h2 className="text-xl font-bold text-white">Product Dependencies</h2>
            <p className="text-sm text-gray-400 mt-1">{productName}</p>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-white/10 rounded-lg transition-colors"
          >
            <X className="w-5 h-5 text-gray-400" />
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6">
          {loading ? (
            <p className="text-center text-gray-400">Loading...</p>
          ) : (
            <>
              {/* Add Button */}
              {!showAddForm && (
                <button
                  onClick={() => setShowAddForm(true)}
                  className="w-full mb-4 py-3 bg-blue-500/20 hover:bg-blue-500/30 text-blue-300 rounded-lg transition-colors flex items-center justify-center gap-2"
                >
                  <Plus className="w-4 h-4" />
                  Add Dependency
                </button>
              )}

              {/* Add Form */}
              {showAddForm && (
                <div className="bg-white/5 rounded-lg p-4 mb-4 border border-blue-500/30">
                  <h3 className="text-sm font-semibold text-white mb-3">Add New Dependency</h3>

                  {/* Search */}
                  <input
                    type="text"
                    placeholder="Search products..."
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                    className="w-full mb-3 px-3 py-2 bg-white/5 border border-white/10 rounded-lg text-white placeholder-gray-500 text-sm"
                  />

                  {/* Product Select */}
                  <select
                    value={selectedProductId || ''}
                    onChange={(e) => setSelectedProductId(Number(e.target.value))}
                    className="w-full mb-3 px-3 py-2 bg-white/5 border border-white/10 rounded-lg text-white text-sm"
                  >
                    <option value="">Select product...</option>
                    {availableToAdd.map((p) => (
                      <option key={p.product_id} value={p.product_id}>
                        {p.name} ({p.generic_barcode || `ID: ${p.product_id}`}) - Stock: {p.stock_quantity?.toFixed(1) || 0} {p.count_type_abbr || ''}
                      </option>
                    ))}
                  </select>

                  {/* Quantity */}
                  <div className="mb-3">
                    <label className="block text-xs text-gray-400 mb-1">Default Quantity</label>
                    <input
                      type="number"
                      min="0.1"
                      step="0.1"
                      value={defaultQuantity}
                      onChange={(e) => setDefaultQuantity(Number(e.target.value))}
                      className="w-full px-3 py-2 bg-white/5 border border-white/10 rounded-lg text-white text-sm"
                    />
                  </div>

                  {/* Optional */}
                  <div className="mb-3 flex items-center gap-2">
                    <input
                      type="checkbox"
                      id="is-optional"
                      checked={isOptional}
                      onChange={(e) => setIsOptional(e.target.checked)}
                      className="rounded"
                    />
                    <label htmlFor="is-optional" className="text-sm text-gray-300">
                      Optional (show as suggestion)
                    </label>
                  </div>

                  {/* Notes */}
                  <div className="mb-3">
                    <label className="block text-xs text-gray-400 mb-1">Notes (optional)</label>
                    <textarea
                      value={notes}
                      onChange={(e) => setNotes(e.target.value)}
                      rows={2}
                      className="w-full px-3 py-2 bg-white/5 border border-white/10 rounded-lg text-white text-sm resize-none"
                      placeholder="Why is this dependency needed?"
                    />
                  </div>

                  {/* Buttons */}
                  <div className="flex gap-2">
                    <button
                      onClick={handleAddDependency}
                      disabled={!selectedProductId}
                      className="flex-1 py-2 bg-blue-500 hover:bg-blue-600 disabled:bg-gray-600 disabled:cursor-not-allowed text-white rounded-lg text-sm font-medium transition-colors"
                    >
                      Add
                    </button>
                    <button
                      onClick={() => {
                        setShowAddForm(false);
                        setSelectedProductId(null);
                        setSearchTerm('');
                        setNotes('');
                      }}
                      className="px-4 py-2 bg-white/10 hover:bg-white/20 text-white rounded-lg text-sm transition-colors"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              )}

              {/* Dependencies List */}
              {dependencies.length === 0 ? (
                <div className="text-center py-8 text-gray-400">
                  <Package className="w-12 h-12 mx-auto mb-3 opacity-50" />
                  <p>No dependencies configured</p>
                  <p className="text-xs mt-1">Add accessories or consumables that are commonly needed with this product</p>
                </div>
              ) : (
                <div className="space-y-2">
                  {dependencies.map((dep) => (
                    <div
                      key={dep.id}
                      className="bg-white/5 rounded-lg p-4 border border-white/10 hover:bg-white/10 transition-colors"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 mb-1">
                            <h4 className="text-sm font-semibold text-white truncate">
                              {dep.dependency_name}
                            </h4>
                            <span className={`px-2 py-0.5 text-xs rounded ${
                              dep.is_accessory
                                ? 'bg-blue-500/20 text-blue-300'
                                : 'bg-purple-500/20 text-purple-300'
                            }`}>
                              {dep.is_accessory ? 'Accessory' : 'Consumable'}
                            </span>
                            {dep.is_optional && (
                              <span className="px-2 py-0.5 text-xs rounded bg-yellow-500/20 text-yellow-300">
                                Optional
                              </span>
                            )}
                          </div>

                          {dep.generic_barcode && (
                            <p className="text-xs text-gray-400 mb-1">
                              Barcode: {dep.generic_barcode}
                            </p>
                          )}

                          <div className="flex items-center gap-3 text-xs text-gray-400">
                            <span>
                              Default: {dep.default_quantity} {dep.count_type_abbr || 'pcs'}
                            </span>
                            {dep.stock_quantity !== undefined && (
                              <>
                                <span className="text-gray-600">•</span>
                                <span>
                                  Stock: {dep.stock_quantity.toFixed(1)} {dep.count_type_abbr || ''}
                                </span>
                              </>
                            )}
                          </div>

                          {dep.notes && (
                            <div className="mt-2 flex items-start gap-2 text-xs text-gray-400 bg-white/5 rounded p-2">
                              <AlertCircle className="w-3 h-3 mt-0.5 flex-shrink-0" />
                              <span>{dep.notes}</span>
                            </div>
                          )}
                        </div>

                        <button
                          onClick={() => handleDeleteDependency(dep.id)}
                          className="p-2 hover:bg-red-500/20 text-red-400 rounded-lg transition-colors"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end p-6 border-t border-white/10">
          <button
            onClick={onClose}
            className="px-4 py-2 bg-white/10 hover:bg-white/20 text-white rounded-lg transition-colors"
          >
            Close
          </button>
        </div>
        </div>
      </div>
    </ModalPortal>
  );
}
