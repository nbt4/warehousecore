import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Building2,
  Eye,
  LayoutGrid,
  List,
  Pencil,
  Plus,
  RefreshCcw,
  Search,
  Trash2,
  X,
} from 'lucide-react';
import { api } from '../../lib/api';
import { ModalPortal } from '../ModalPortal';

interface FieldDefinition {
  id: number;
  name: string;
  field_type: 'text' | 'number' | 'dropdown';
  unit: string | null;
  dropdown_options: string | null;
  is_active: boolean;
}

interface FieldValueInput {
  field_definition_id: number;
  value: string;
  sort_order: number;
}

interface FieldValue {
  id: number;
  field_definition_id: number;
  value: string;
  sort_order: number;
  definition?: FieldDefinition;
}

interface RentalEquipment {
  equipment_id: number;
  product_name: string;
  supplier_name: string;
  rental_price: number;
  customer_price: number;
  category?: string | null;
  description?: string | null;
  notes?: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  field_values?: FieldValue[];
}

interface RentalEquipmentFormData {
  product_name: string;
  supplier_name: string;
  rental_price: number;
  customer_price: number;
  category: string;
  description: string;
  notes: string;
  is_active: boolean;
  field_values: FieldValueInput[];
}

const initialFormData: RentalEquipmentFormData = {
  product_name: '',
  supplier_name: '',
  rental_price: 0,
  customer_price: 0,
  category: '',
  description: '',
  notes: '',
  is_active: true,
  field_values: [],
};

const parseNumber = (value: string): number => {
  const trimmed = value.trim();
  if (trimmed === '') {
    return 0;
  }
  const parsed = Number(trimmed);
  return Number.isNaN(parsed) ? 0 : parsed;
};

function useDebouncedValue<T>(value: T, delay: number) {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const handle = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(handle);
  }, [value, delay]);

  return debounced;
}

export function RentedProductsTab() {
  const [equipment, setEquipment] = useState<RentalEquipment[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [viewEquipment, setViewEquipment] = useState<RentalEquipment | null>(null);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [formData, setFormData] = useState<RentalEquipmentFormData>(initialFormData);
  const [suppliers, setSuppliers] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [supplierContactResults, setSupplierContactResults] = useState<{ id: number; display_name: string }[]>([]);
  const [viewMode, setViewMode] = useState<'table' | 'cards'>(() => {
    return typeof window !== 'undefined' && window.innerWidth < 768 ? 'cards' : 'table';
  });
  const [searchTerm, setSearchTerm] = useState('');
  const [supplierFilter, setSupplierFilter] = useState<string>('');
  const [refreshing, setRefreshing] = useState(false);
  const [fieldDefs, setFieldDefs] = useState<FieldDefinition[]>([]);
  const scrollPosition = useRef(0);

  const debouncedSearch = useDebouncedValue(searchTerm, 300);

  useEffect(() => {
    api.get<FieldDefinition[]>('/admin/rental-field-definitions?active_only=true')
      .then(r => setFieldDefs(r.data || []))
      .catch(() => {});
  }, []);

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
    const anyModalOpen = modalOpen || !!viewEquipment;
    if (anyModalOpen) {
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
  }, [modalOpen, viewEquipment]);

  const fetchEquipment = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = {};
      if (debouncedSearch.trim()) {
        params.search = debouncedSearch.trim();
      }
      if (supplierFilter) {
        params.supplier = supplierFilter;
      }
      const { data } = await api.get<RentalEquipment[]>('/admin/rental-equipment', { params });
      setEquipment(data || []);
    } catch (error) {
      console.error('Failed to load rental equipment:', error);
      setEquipment([]);
    } finally {
      setLoading(false);
    }
  }, [debouncedSearch, supplierFilter]);

  const fetchSuppliers = useCallback(async () => {
    try {
      const { data } = await api.get<string[]>('/admin/rental-equipment/suppliers');
      setSuppliers(data || []);
    } catch (error) {
      console.error('Failed to load suppliers:', error);
    }
  }, []);

  useEffect(() => {
    fetchEquipment();
  }, [fetchEquipment]);

  useEffect(() => {
    fetchSuppliers();
  }, [fetchSuppliers]);

  const handleRefresh = useCallback(async () => {
    setRefreshing(true);
    await fetchEquipment();
    await fetchSuppliers();
    setRefreshing(false);
  }, [fetchEquipment, fetchSuppliers]);

  const clearFilters = () => {
    setSearchTerm('');
    setSupplierFilter('');
  };

  const closeModal = useCallback(() => {
    setModalOpen(false);
    setEditingId(null);
    setFormData(initialFormData);
    setSupplierContactResults([]);
  }, []);

  const closeDetailModal = () => {
    setViewEquipment(null);
  };

  const handleOpenCreateModal = () => {
    setFormData(initialFormData);
    setEditingId(null);
    setModalOpen(true);
  };

  const handleEditEquipment = (item: RentalEquipment) => {
    setFormData({
      product_name: item.product_name,
      supplier_name: item.supplier_name,
      rental_price: item.rental_price,
      customer_price: item.customer_price,
      category: item.category || '',
      description: item.description || '',
      notes: item.notes || '',
      is_active: item.is_active,
      field_values: (item.field_values ?? []).map(v => ({
        field_definition_id: v.field_definition_id,
        value: v.value,
        sort_order: v.sort_order,
      })),
    });
    setEditingId(item.equipment_id);
    setModalOpen(true);
  };

  const handleDelete = async (id: number, name: string) => {
    if (!window.confirm(`Mietprodukt "${name}" wirklich löschen?`)) {
      return;
    }

    try {
      await api.delete(`/admin/rental-equipment/${id}`);
      await fetchEquipment();
      await fetchSuppliers();
    } catch (error) {
      console.error('Failed to delete rental equipment:', error);
      window.alert('Fehler beim Löschen des Mietprodukts.');
    }
  };

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    if (!formData.product_name.trim()) {
      window.alert('Der Produktname ist erforderlich.');
      return;
    }
    if (!formData.supplier_name.trim()) {
      window.alert('Der Lieferantenname ist erforderlich.');
      return;
    }

    setSubmitting(true);

    const payload = {
      product_name: formData.product_name.trim(),
      supplier_name: formData.supplier_name.trim(),
      rental_price: formData.rental_price,
      customer_price: formData.customer_price,
      category: formData.category.trim() || null,
      description: formData.description.trim() || null,
      notes: formData.notes.trim() || null,
      is_active: formData.is_active,
      field_values: formData.field_values,
    };

    try {
      if (editingId) {
        await api.put(`/admin/rental-equipment/${editingId}`, payload);
      } else {
        await api.post('/admin/rental-equipment', payload);
      }

      await fetchEquipment();
      await fetchSuppliers();
      closeModal();
    } catch (error) {
      console.error('Failed to save rental equipment:', error);
      window.alert('Fehler beim Speichern des Mietprodukts.');
    } finally {
      setSubmitting(false);
    }
  };

  const sortedEquipment = useMemo(() =>
    [...equipment].sort((a, b) => {
      const supplierCompare = a.supplier_name.localeCompare(b.supplier_name);
      if (supplierCompare !== 0) return supplierCompare;
      return a.product_name.localeCompare(b.product_name);
    }),
    [equipment]
  );

  const hasActiveFilters = debouncedSearch.trim() !== '' || supplierFilter !== '';

  const formatCurrency = (value: number) => `${value.toFixed(2)} €`;

  const profitMargin = (rental: number, customer: number) => {
    if (rental === 0) return 0;
    return ((customer - rental) / rental * 100);
  };

  const totalRentalCost = sortedEquipment.reduce((sum, e) => sum + e.rental_price, 0);
  const totalCustomerRevenue = sortedEquipment.reduce((sum, e) => sum + e.customer_price, 0);

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-4">
        <div>
          <h2 className="text-xl font-bold text-white">Mietprodukte verwalten</h2>
          <p className="text-sm text-gray-400">
            {sortedEquipment.length} Mietprodukte geladen
            {hasActiveFilters ? ' - Filter aktiv' : ''}
          </p>
        </div>

        <div className="flex flex-col gap-3">
          <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <input
                value={searchTerm}
                onChange={event => setSearchTerm(event.target.value)}
                placeholder="Suchen (Name, Lieferant, Beschreibung ...)"
                className="w-full rounded-lg bg-white/10 py-2 pl-9 pr-3 text-sm text-white placeholder-gray-500 outline-none transition focus:bg-white/15 focus:ring-1 focus:ring-accent-red"
              />
            </div>

            <select
              value={supplierFilter}
              onChange={event => setSupplierFilter(event.target.value)}
              className="rounded-lg bg-white/10 px-3 py-2 text-sm text-white outline-none transition focus:bg-white/15 focus:ring-1 focus:ring-accent-red"
            >
              <option value="">Alle Lieferanten</option>
              {suppliers.map(supplier => (
                <option key={supplier} value={supplier}>
                  {supplier}
                </option>
              ))}
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
              className={`flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition ${
                viewMode === 'table'
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
              className={`flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition ${
                viewMode === 'cards'
                  ? 'bg-white/15 text-white'
                  : 'bg-white/5 text-gray-400 hover:bg-white/10 hover:text-white'
              }`}
            >
              <LayoutGrid className="h-4 w-4" />
              <span className="hidden sm:inline">Karten</span>
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
              <span className="hidden sm:inline">Neues Mietprodukt</span>
              <span className="sm:hidden">Neu</span>
            </button>
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-white/10 bg-white/5 p-4 text-sm text-gray-300 backdrop-blur">
        <div className="flex flex-wrap items-center gap-6">
          <span>
            <strong className="text-white">{sortedEquipment.length}</strong> Mietprodukte
          </span>
          <span>
            Gesamt Mietkosten: <strong className="text-white">{formatCurrency(totalRentalCost)}</strong>
          </span>
          <span>
            Gesamt Kundenpreis: <strong className="text-green-400">{formatCurrency(totalCustomerRevenue)}</strong>
          </span>
          {supplierFilter && (
            <span>
              Gefiltert nach: <strong className="text-white">{supplierFilter}</strong>
            </span>
          )}
        </div>
      </div>

      {loading ? (
        <div className="glass rounded-xl p-8 text-center text-gray-400">
          Lade Mietprodukte ...
        </div>
      ) : sortedEquipment.length === 0 ? (
        <div className="glass rounded-xl p-8 text-center text-gray-400">
          <Building2 className="mx-auto mb-4 h-12 w-12 text-gray-600" />
          Keine Mietprodukte gefunden
          {hasActiveFilters ? ' - bitte Filter anpassen.' : '.'}
        </div>
      ) : viewMode === 'table' ? (
        <div className="overflow-hidden rounded-xl border border-white/10 bg-white/5">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-white/10 text-sm text-gray-200">
              <thead className="bg-white/5 text-xs uppercase tracking-wide text-gray-400">
                <tr>
                  <th className="px-4 py-3 text-left font-semibold">Produkt</th>
                  <th className="px-4 py-3 text-left font-semibold">Lieferant</th>
                  <th className="px-4 py-3 text-right font-semibold">Mietpreis</th>
                  <th className="px-4 py-3 text-right font-semibold">Kundenpreis</th>
                  <th className="px-4 py-3 text-right font-semibold">Marge</th>
                  <th className="px-4 py-3 text-center font-semibold">Status</th>
                  <th className="px-4 py-3 text-right font-semibold">Aktionen</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/10">
                {sortedEquipment.map(item => {
                  const margin = profitMargin(item.rental_price, item.customer_price);
                  return (
                    <tr key={item.equipment_id} className="hover:bg-white/5">
                      <td className="px-4 py-3 align-top">
                        <div className="flex flex-col">
                          <span className="text-white font-medium">{item.product_name}</span>
                          {item.category && (
                            <span className="text-xs text-gray-500">{item.category}</span>
                          )}
                          {item.description && (
                            <span className="mt-1 text-xs text-gray-400 line-clamp-2">
                              {item.description}
                            </span>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3 align-top">
                        <div className="flex items-center gap-2">
                          <Building2 className="h-4 w-4 text-blue-400" />
                          <span className="text-gray-300">{item.supplier_name}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3 align-top text-right text-gray-300">
                        {formatCurrency(item.rental_price)}
                      </td>
                      <td className="px-4 py-3 align-top text-right font-medium text-green-400">
                        {formatCurrency(item.customer_price)}
                      </td>
                      <td className="px-4 py-3 align-top text-right">
                        <span className={margin > 0 ? 'text-green-400' : margin < 0 ? 'text-red-400' : 'text-gray-400'}>
                          {margin > 0 ? '+' : ''}{margin.toFixed(1)}%
                        </span>
                      </td>
                      <td className="px-4 py-3 align-top text-center">
                        <span className={`inline-flex items-center rounded-full px-2 py-1 text-xs font-medium ${
                          item.is_active
                            ? 'bg-green-500/20 text-green-400'
                            : 'bg-gray-500/20 text-gray-400'
                        }`}>
                          {item.is_active ? 'Aktiv' : 'Inaktiv'}
                        </span>
                      </td>
                      <td className="px-4 py-3 align-top">
                        <div className="flex justify-end gap-2">
                          <button
                            onClick={() => setViewEquipment(item)}
                            className="rounded-lg bg-white/10 p-2 text-gray-200 transition hover:bg-white/20 hover:text-white"
                            title="Details anzeigen"
                          >
                            <Eye className="h-4 w-4" />
                          </button>
                          <button
                            onClick={() => handleEditEquipment(item)}
                            className="rounded-lg bg-white/10 p-2 text-gray-200 transition hover:bg-white/20 hover:text-white"
                            title="Bearbeiten"
                          >
                            <Pencil className="h-4 w-4" />
                          </button>
                          <button
                            onClick={() => handleDelete(item.equipment_id, item.product_name)}
                            className="rounded-lg bg-red-600/80 p-2 text-white transition hover:bg-red-600"
                            title="Löschen"
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {sortedEquipment.map(item => {
            const margin = profitMargin(item.rental_price, item.customer_price);
            return (
              <div key={item.equipment_id} className="glass rounded-xl p-4">
                <div className="flex items-start justify-between gap-4">
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <h3 className="text-lg font-semibold text-white">{item.product_name}</h3>
                      <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
                        item.is_active
                          ? 'bg-green-500/20 text-green-400'
                          : 'bg-gray-500/20 text-gray-400'
                      }`}>
                        {item.is_active ? 'Aktiv' : 'Inaktiv'}
                      </span>
                    </div>
                    <div className="mt-1 flex items-center gap-2 text-sm text-gray-400">
                      <Building2 className="h-4 w-4 text-blue-400" />
                      {item.supplier_name}
                    </div>
                    {item.category && (
                      <p className="mt-1 text-xs text-gray-500">{item.category}</p>
                    )}
                    <div className="mt-3 grid grid-cols-3 gap-2 text-center">
                      <div>
                        <p className="text-xs text-gray-500">Mietpreis</p>
                        <p className="text-sm font-medium text-gray-300">{formatCurrency(item.rental_price)}</p>
                      </div>
                      <div>
                        <p className="text-xs text-gray-500">Kundenpreis</p>
                        <p className="text-sm font-medium text-green-400">{formatCurrency(item.customer_price)}</p>
                      </div>
                      <div>
                        <p className="text-xs text-gray-500">Marge</p>
                        <p className={`text-sm font-medium ${margin > 0 ? 'text-green-400' : margin < 0 ? 'text-red-400' : 'text-gray-400'}`}>
                          {margin > 0 ? '+' : ''}{margin.toFixed(1)}%
                        </p>
                      </div>
                    </div>
                  </div>
                  <div className="flex flex-col gap-2">
                    <button
                      onClick={() => setViewEquipment(item)}
                      className="rounded-lg bg-white/10 p-2 text-gray-200 transition hover:bg-white/20 hover:text-white"
                      title="Details anzeigen"
                    >
                      <Eye className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => handleEditEquipment(item)}
                      className="rounded-lg bg-white/10 p-2 text-gray-200 transition hover:bg-white/20 hover:text-white"
                      title="Bearbeiten"
                    >
                      <Pencil className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => handleDelete(item.equipment_id, item.product_name)}
                      className="rounded-lg bg-red-600/80 p-2 text-white transition hover:bg-red-600"
                      title="Löschen"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Create/Edit Modal */}
      {modalOpen && (
        <ModalPortal>
          <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
            <div className="glass-dark rounded-2xl border border-white/10 shadow-2xl p-6 max-w-2xl w-full max-h-[90vh] overflow-y-auto">
              <div className="flex justify-between items-center mb-6">
                <h3 className="text-2xl font-bold text-white">
                  {editingId ? 'Mietprodukt bearbeiten' : 'Neues Mietprodukt'}
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
                    Produktname <span className="text-accent-red">*</span>
                  </label>
                  <input
                    type="text"
                    value={formData.product_name}
                    onChange={event => setFormData({ ...formData, product_name: event.target.value })}
                    placeholder="z.B. Bühnenpodest 2x1m"
                    className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                    required
                  />
                </div>

                <div className="relative">
                  <label className="mb-2 block text-sm font-semibold text-white">
                    Lieferant <span className="text-accent-red">*</span>
                  </label>
                  <input
                    type="text"
                    value={formData.supplier_name}
                    onChange={event => {
                      const val = event.target.value;
                      setFormData({ ...formData, supplier_name: val });
                      if (val.trim().length >= 2) {
                        api.get<{ id: number; display_name: string }[]>(
                          `/admin/rental-equipment/supplier-contacts?q=${encodeURIComponent(val.trim())}`
                        ).then(r => setSupplierContactResults(r.data || [])).catch(() => setSupplierContactResults([]));
                      } else {
                        setSupplierContactResults([]);
                      }
                    }}
                    placeholder="z.B. Stagetec GmbH"
                    className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                    required
                  />
                  {supplierContactResults.length > 0 && (
                    <div className="absolute left-0 right-0 top-full z-50 mt-1 rounded-lg border border-white/10 bg-[#1a1a1a] shadow-xl overflow-hidden">
                      {supplierContactResults.map(s => (
                        <button
                          key={s.id}
                          type="button"
                          onClick={() => {
                            setFormData(prev => ({ ...prev, supplier_name: s.display_name }));
                            setSupplierContactResults([]);
                          }}
                          className="w-full px-3 py-2 text-left text-sm text-gray-200 hover:bg-white/10 transition-colors"
                        >
                          {s.display_name}
                        </button>
                      ))}
                    </div>
                  )}
                </div>

                <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">
                      Mietpreis (intern) <span className="text-accent-red">*</span>
                    </label>
                    <div className="relative">
                      <input
                        type="number"
                        step="0.01"
                        min="0"
                        value={formData.rental_price || ''}
                        onChange={event =>
                          setFormData({
                            ...formData,
                            rental_price: parseNumber(event.target.value),
                          })
                        }
                        placeholder="15.00"
                        className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 pr-8 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                      />
                      <span className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">€</span>
                    </div>
                    <p className="mt-1 text-xs text-gray-500">Was Sie dem Lieferanten zahlen</p>
                  </div>

                  <div>
                    <label className="mb-2 block text-sm font-semibold text-white">
                      Kundenpreis <span className="text-accent-red">*</span>
                    </label>
                    <div className="relative">
                      <input
                        type="number"
                        step="0.01"
                        min="0"
                        value={formData.customer_price || ''}
                        onChange={event =>
                          setFormData({
                            ...formData,
                            customer_price: parseNumber(event.target.value),
                          })
                        }
                        placeholder="25.00"
                        className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 pr-8 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                      />
                      <span className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">€</span>
                    </div>
                    <p className="mt-1 text-xs text-gray-500">Was der Kunde zahlt</p>
                  </div>
                </div>

                {formData.rental_price > 0 && formData.customer_price > 0 && (
                  <div className="rounded-lg bg-white/5 p-3 text-center">
                    <span className="text-sm text-gray-400">Gewinnmarge: </span>
                    <span className={`text-lg font-bold ${
                      profitMargin(formData.rental_price, formData.customer_price) > 0
                        ? 'text-green-400'
                        : 'text-red-400'
                    }`}>
                      {profitMargin(formData.rental_price, formData.customer_price) > 0 ? '+' : ''}
                      {profitMargin(formData.rental_price, formData.customer_price).toFixed(1)}%
                    </span>
                    <span className="ml-2 text-sm text-gray-400">
                      ({formatCurrency(formData.customer_price - formData.rental_price)} Gewinn)
                    </span>
                  </div>
                )}

                <div>
                  <label className="mb-2 block text-sm font-semibold text-white">Kategorie</label>
                  <input
                    type="text"
                    value={formData.category}
                    onChange={event => setFormData({ ...formData, category: event.target.value })}
                    placeholder="z.B. Bühne, Rigging, Audio"
                    className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                  />
                </div>

                <div>
                  <label className="mb-2 block text-sm font-semibold text-white">Beschreibung</label>
                  <textarea
                    value={formData.description}
                    onChange={event => setFormData({ ...formData, description: event.target.value })}
                    rows={3}
                    placeholder="Optionale Beschreibung des Mietprodukts..."
                    className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                  />
                </div>

                <div>
                  <label className="mb-2 block text-sm font-semibold text-white">Notizen</label>
                  <textarea
                    value={formData.notes}
                    onChange={event => setFormData({ ...formData, notes: event.target.value })}
                    rows={2}
                    placeholder="Interne Notizen (z.B. Kontaktperson, Lieferzeiten)..."
                    className="w-full rounded-lg border border-white/20 bg-white/10 px-3 py-2 text-white placeholder-gray-500 outline-none transition focus:border-accent-red"
                  />
                </div>

                <div className="flex items-center gap-3">
                  <input
                    type="checkbox"
                    id="is_active"
                    checked={formData.is_active}
                    onChange={event => setFormData({ ...formData, is_active: event.target.checked })}
                    className="h-4 w-4 rounded border-white/20 bg-white/10 text-accent-red focus:ring-accent-red"
                  />
                  <label htmlFor="is_active" className="text-sm font-medium text-white">
                    Produkt ist aktiv und kann für Jobs verwendet werden
                  </label>
                </div>

                {/* Custom Fields */}
                <div className="mt-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs font-medium text-gray-400 uppercase tracking-wider">Technische Felder</span>
                    <select
                      className="text-xs rounded px-2 py-1 bg-black/40 border border-white/10 text-gray-300 cursor-pointer"
                      value=""
                      onChange={e => {
                        const defId = Number(e.target.value);
                        if (!defId) return;
                        const alreadyAdded = formData.field_values.some(v => v.field_definition_id === defId);
                        if (alreadyAdded) return;
                        setFormData(prev => ({
                          ...prev,
                          field_values: [...prev.field_values, {
                            field_definition_id: defId,
                            value: '',
                            sort_order: prev.field_values.length,
                          }],
                        }));
                      }}
                    >
                      <option value="">+ Feld hinzufügen</option>
                      {fieldDefs
                        .filter(d => !formData.field_values.some(v => v.field_definition_id === d.id))
                        .map(d => (
                          <option key={d.id} value={d.id}>{d.name}</option>
                        ))}
                    </select>
                  </div>
                  {formData.field_values.length > 0 && (
                    <div className="space-y-2">
                      {formData.field_values.map((fv, idx) => {
                        const def = fieldDefs.find(d => d.id === fv.field_definition_id);
                        if (!def) return null;
                        return (
                          <div key={fv.field_definition_id} className="flex items-center gap-2">
                            <span className="text-sm text-gray-300 w-28 flex-shrink-0">{def.name}</span>
                            {def.field_type === 'dropdown' && def.dropdown_options ? (
                              <select
                                className="flex-1 rounded px-2 py-1 text-sm bg-black/40 border border-white/10 text-white"
                                value={fv.value}
                                onChange={e => {
                                  const updated = formData.field_values.map((v, i) =>
                                    i === idx ? { ...v, value: e.target.value } : v
                                  );
                                  setFormData(prev => ({ ...prev, field_values: updated }));
                                }}
                              >
                                <option value="">— wählen —</option>
                                {JSON.parse(def.dropdown_options).map((opt: string) => (
                                  <option key={opt} value={opt}>{opt}</option>
                                ))}
                              </select>
                            ) : (
                              <input
                                type={def.field_type === 'number' ? 'number' : 'text'}
                                className="flex-1 rounded px-2 py-1 text-sm bg-black/40 border border-white/10 text-white"
                                value={fv.value}
                                onChange={e => {
                                  const updated = formData.field_values.map((v, i) =>
                                    i === idx ? { ...v, value: e.target.value } : v
                                  );
                                  setFormData(prev => ({ ...prev, field_values: updated }));
                                }}
                              />
                            )}
                            {def.unit && <span className="text-xs text-gray-500 flex-shrink-0">{def.unit}</span>}
                            <button
                              type="button"
                              onClick={() => {
                                const updated = formData.field_values
                                  .filter((_, i) => i !== idx)
                                  .map((v, i) => ({ ...v, sort_order: i }));
                                setFormData(prev => ({ ...prev, field_values: updated }));
                              }}
                              className="text-gray-500 hover:text-red-400 transition-colors flex-shrink-0 text-lg leading-none"
                            >
                              ×
                            </button>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>

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
                    {submitting ? 'Speichert...' : editingId ? 'Speichern' : 'Erstellen'}
                  </button>
                </div>
              </form>
            </div>
          </div>
        </ModalPortal>
      )}

      {/* Detail Modal */}
      {viewEquipment && (
        <ModalPortal>
          <div className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4">
            <div className="glass-dark rounded-2xl border border-white/10 shadow-2xl p-6 max-w-2xl w-full max-h-[90vh] overflow-y-auto">
              <div className="flex justify-between items-center mb-6">
                <h3 className="text-2xl font-bold text-white">{viewEquipment.product_name}</h3>
                <button
                  onClick={closeDetailModal}
                  className="text-gray-400 hover:text-white p-2 rounded-lg hover:bg-white/10 transition-colors"
                >
                  <X className="w-6 h-6" />
                </button>
              </div>

              <div className="space-y-4">
                <div className="flex items-center gap-3 rounded-lg bg-white/5 p-4">
                  <Building2 className="h-6 w-6 text-blue-400" />
                  <div>
                    <p className="text-xs text-gray-500">Lieferant</p>
                    <p className="text-lg font-medium text-white">{viewEquipment.supplier_name}</p>
                  </div>
                </div>

                <div className="grid grid-cols-3 gap-4">
                  <div className="rounded-lg bg-white/5 p-4 text-center">
                    <p className="text-xs text-gray-500">Mietpreis</p>
                    <p className="text-xl font-bold text-gray-300">{formatCurrency(viewEquipment.rental_price)}</p>
                  </div>
                  <div className="rounded-lg bg-white/5 p-4 text-center">
                    <p className="text-xs text-gray-500">Kundenpreis</p>
                    <p className="text-xl font-bold text-green-400">{formatCurrency(viewEquipment.customer_price)}</p>
                  </div>
                  <div className="rounded-lg bg-white/5 p-4 text-center">
                    <p className="text-xs text-gray-500">Marge</p>
                    <p className={`text-xl font-bold ${
                      profitMargin(viewEquipment.rental_price, viewEquipment.customer_price) > 0
                        ? 'text-green-400'
                        : 'text-red-400'
                    }`}>
                      {profitMargin(viewEquipment.rental_price, viewEquipment.customer_price) > 0 ? '+' : ''}
                      {profitMargin(viewEquipment.rental_price, viewEquipment.customer_price).toFixed(1)}%
                    </p>
                  </div>
                </div>

                {viewEquipment.category && (
                  <div>
                    <p className="text-xs text-gray-500">Kategorie</p>
                    <p className="text-white">{viewEquipment.category}</p>
                  </div>
                )}

                {viewEquipment.description && (
                  <div>
                    <p className="text-xs text-gray-500">Beschreibung</p>
                    <p className="text-white">{viewEquipment.description}</p>
                  </div>
                )}

                {viewEquipment.notes && (
                  <div>
                    <p className="text-xs text-gray-500">Notizen</p>
                    <p className="text-gray-300">{viewEquipment.notes}</p>
                  </div>
                )}

                {viewEquipment.field_values && viewEquipment.field_values.length > 0 && (
                  <div className="mt-3">
                    <p className="text-xs text-gray-500 uppercase tracking-wider mb-2">Technische Felder</p>
                    <div className="space-y-1">
                      {viewEquipment.field_values.map(fv => (
                        <div key={fv.field_definition_id} className="flex items-center gap-2 text-sm">
                          <span className="text-gray-400">{fv.definition?.name ?? `Feld ${fv.field_definition_id}`}:</span>
                          <span className="text-white">{fv.value}{fv.definition?.unit ? ` ${fv.definition.unit}` : ''}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                <div className="flex items-center justify-between rounded-lg bg-white/5 p-4">
                  <span className="text-sm text-gray-400">Status</span>
                  <span className={`inline-flex items-center rounded-full px-3 py-1 text-sm font-medium ${
                    viewEquipment.is_active
                      ? 'bg-green-500/20 text-green-400'
                      : 'bg-gray-500/20 text-gray-400'
                  }`}>
                    {viewEquipment.is_active ? 'Aktiv' : 'Inaktiv'}
                  </span>
                </div>

                <div className="text-xs text-gray-500">
                  <p>Erstellt: {new Date(viewEquipment.created_at).toLocaleString('de-DE')}</p>
                  <p>Aktualisiert: {new Date(viewEquipment.updated_at).toLocaleString('de-DE')}</p>
                </div>
              </div>

              <button
                onClick={closeDetailModal}
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
