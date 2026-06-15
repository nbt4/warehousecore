import { useState, useEffect } from 'react';
import { Plus, Edit2, Trash2, Save, X } from 'lucide-react';
import { api } from '../../lib/api';
import { toast } from '../../lib/toast';

interface Category {
  category_id: number;
  name: string;
  abbreviation: string;
}

interface Subcategory {
  subcategory_id: string;
  name: string;
  abbreviation: string;
  category_id: number;
}

interface Subbiercategory {
  subbiercategory_id: string;
  name: string;
  abbreviation: string;
  subcategory_id: string;
}

type Level = 'category' | 'subcategory' | 'subbiercategory';

export function CategoriesTab() {
  const [categories, setCategories] = useState<Category[]>([]);
  const [subcategories, setSubcategories] = useState<Subcategory[]>([]);
  const [subbiercategories, setSubbiercategories] = useState<Subbiercategory[]>([]);
  const [activeLevel, setActiveLevel] = useState<Level>('category');
  const [editing, setEditing] = useState<number | string | 'new' | null>(null);
  const [formData, setFormData] = useState<any>({});
  const [subcategoryFilter, setSubcategoryFilter] = useState<number | ''>('');
  const [subbiercategoryFilter, setSubbiercategoryFilter] = useState<string | ''>('');

  useEffect(() => {
    loadCategories();
    loadSubcategories();
    loadSubbiercategories();
  }, []);

  const loadCategories = async () => {
    try {
      const { data } = await api.get('/admin/categories');
      setCategories(data || []);
    } catch (error) {
      toast.error('Failed to load categories:' + " " + String(error));
    }
  };

  const loadSubcategories = async () => {
    try {
      const { data } = await api.get('/admin/subcategories');
      setSubcategories(data || []);
    } catch (error) {
      toast.error('Failed to load subcategories:' + " " + String(error));
    }
  };

  const loadSubbiercategories = async () => {
    try {
      const { data } = await api.get('/admin/subbiercategories');
      setSubbiercategories(data || []);
    } catch (error) {
      toast.error('Failed to load subbiercategories:' + " " + String(error));
    }
  };

  const handleSaveCategory = async () => {
    try {
      if (editing === 'new') {
        await api.post('/admin/categories', formData);
      } else {
        await api.put(`/admin/categories/${editing}`, formData);
      }
      loadCategories();
      setEditing(null);
      setFormData({});
    } catch (error: any) {
      alert('Fehler: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleSaveSubcategory = async () => {
    try {
      if (editing === 'new') {
        await api.post('/admin/subcategories', formData);
      } else {
        await api.put(`/admin/subcategories/${editing}`, formData);
      }
      loadSubcategories();
      setEditing(null);
      setFormData({});
    } catch (error: any) {
      alert('Fehler: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleSaveSubbiercategory = async () => {
    try {
      if (editing === 'new') {
        await api.post('/admin/subbiercategories', formData);
      } else {
        await api.put(`/admin/subbiercategories/${editing}`, formData);
      }
      loadSubbiercategories();
      setEditing(null);
      setFormData({});
    } catch (error: any) {
      alert('Fehler: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleDelete = async (level: Level, id: number | string) => {
    if (!confirm('Wirklich löschen?')) return;

    try {
      await api.delete(`/admin/${level === 'category' ? 'categories' : level === 'subcategory' ? 'subcategories' : 'subbiercategories'}/${id}`);
      if (level === 'category') loadCategories();
      else if (level === 'subcategory') loadSubcategories();
      else loadSubbiercategories();
    } catch (error: any) {
      alert('Fehler: ' + (error.response?.data?.error || error.message));
    }
  };

  const filteredSubcategories = subcategoryFilter === '' ? subcategories : subcategories.filter((subcat) => subcat.category_id === subcategoryFilter);
  const filteredSubbiercategories = subbiercategoryFilter === '' ? subbiercategories : subbiercategories.filter((subbiercat) => subbiercat.subcategory_id === subbiercategoryFilter);

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-bold text-white">Kategorien verwalten</h2>

      {/* Level Selector */}
      <div className="flex gap-2 overflow-x-auto scrollbar-thin">
        <button
          onClick={() => setActiveLevel('category')}
          className={`px-3 sm:px-4 py-2 rounded-lg font-semibold whitespace-nowrap flex-shrink-0 text-sm sm:text-base ${activeLevel === 'category' ? 'bg-accent-red text-white' : 'bg-white/10 text-gray-400'}`}
        >
          Kategorien
        </button>
        <button
          onClick={() => setActiveLevel('subcategory')}
          className={`px-3 sm:px-4 py-2 rounded-lg font-semibold whitespace-nowrap flex-shrink-0 text-sm sm:text-base ${activeLevel === 'subcategory' ? 'bg-accent-red text-white' : 'bg-white/10 text-gray-400'}`}
        >
          Unterkategorien
        </button>
        <button
          onClick={() => setActiveLevel('subbiercategory')}
          className={`px-3 sm:px-4 py-2 rounded-lg font-semibold whitespace-nowrap flex-shrink-0 text-sm sm:text-base ${activeLevel === 'subbiercategory' ? 'bg-accent-red text-white' : 'bg-white/10 text-gray-400'}`}
        >
          Sub-Unterkategorien
        </button>
      </div>

      {/* Categories */}
      {activeLevel === 'category' && (
        <div className="space-y-2">
          <button
            onClick={() => { setEditing('new'); setFormData({}); }}
            className="px-4 py-2 bg-accent-red text-white rounded-lg font-semibold hover:shadow-lg flex items-center gap-2"
          >
            <Plus className="w-4 h-4" />
            Neue Kategorie
          </button>

          {editing && (
            <div className="glass rounded-xl p-4 space-y-3 border-2 border-accent-red">
              <input
                type="text"
                placeholder="Name"
                value={formData.name || ''}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                className="w-full px-3 py-2 rounded-lg glass text-white"
              />
              <input
                type="text"
                placeholder="Abkürzung (max. 3 Zeichen)"
                maxLength={3}
                value={formData.abbreviation || ''}
                onChange={(e) => setFormData({ ...formData, abbreviation: e.target.value.toUpperCase() })}
                className="w-full px-3 py-2 rounded-lg glass text-white"
              />
              <div className="flex gap-2">
                <button onClick={handleSaveCategory} className="flex-1 px-4 py-2 bg-green-600 text-white rounded-lg flex items-center justify-center gap-2">
                  <Save className="w-4 h-4" />
                  Speichern
                </button>
                <button onClick={() => { setEditing(null); setFormData({}); }} className="flex-1 px-4 py-2 bg-gray-600 text-white rounded-lg flex items-center justify-center gap-2">
                  <X className="w-4 h-4" />
                  Abbrechen
                </button>
              </div>
            </div>
          )}

          {categories.map(cat => (
            <div key={cat.category_id} className="glass rounded-xl p-4 flex items-center justify-between">
              <div>
                <h3 className="text-white font-semibold">{cat.name}</h3>
                <p className="text-gray-400 text-sm">{cat.abbreviation}</p>
              </div>
              <div className="flex gap-2">
                <button onClick={() => { setEditing(cat.category_id); setFormData(cat); }} className="p-2 hover:bg-white/10 rounded-lg text-blue-400">
                  <Edit2 className="w-4 h-4" />
                </button>
                <button onClick={() => handleDelete('category', cat.category_id)} className="p-2 hover:bg-white/10 rounded-lg text-red-400">
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Subcategories */}
      {activeLevel === 'subcategory' && (
        <div className="space-y-2">
          <button
            onClick={() => { setEditing('new'); setFormData({}); }}
            className="px-4 py-2 bg-accent-red text-white rounded-lg font-semibold hover:shadow-lg flex items-center gap-2"
          >
            <Plus className="w-4 h-4" />
            Neue Unterkategorie
          </button>

          <div className="glass rounded-xl p-3 sm:p-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between border border-white/10">
            <div className="text-sm font-semibold text-white">Nach Kategorie filtern</div>
            <select
              value={subcategoryFilter}
              onChange={(e) => setSubcategoryFilter(e.target.value === '' ? '' : parseInt(e.target.value, 10))}
              className="w-full sm:w-64 px-3 py-2 rounded-lg glass text-white"
            >
              <option value="">Alle Kategorien</option>
              {categories.map(cat => (
                <option key={cat.category_id} value={cat.category_id}>{cat.name}</option>
              ))}
            </select>
          </div>

          {editing && (
            <div className="glass rounded-xl p-4 space-y-3 border-2 border-accent-red">
              <input
                type="text"
                placeholder="Name"
                value={formData.name || ''}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                className="w-full px-3 py-2 rounded-lg glass text-white"
              />
              <input
                type="text"
                placeholder="Abkürzung (max. 3 Zeichen)"
                maxLength={3}
                value={formData.abbreviation || ''}
                onChange={(e) => setFormData({ ...formData, abbreviation: e.target.value.toUpperCase() })}
                className="w-full px-3 py-2 rounded-lg glass text-white"
              />
              <select
                value={formData.category_id || ''}
                onChange={(e) => setFormData({ ...formData, category_id: parseInt(e.target.value) })}
                className="w-full px-3 py-2 rounded-lg glass text-white"
              >
                <option value="">-- Kategorie wählen --</option>
                {categories.map(cat => (
                  <option key={cat.category_id} value={cat.category_id}>{cat.name}</option>
                ))}
              </select>
              <div className="flex gap-2">
                <button onClick={handleSaveSubcategory} className="flex-1 px-4 py-2 bg-green-600 text-white rounded-lg flex items-center justify-center gap-2">
                  <Save className="w-4 h-4" />
                  Speichern
                </button>
                <button onClick={() => { setEditing(null); setFormData({}); }} className="flex-1 px-4 py-2 bg-gray-600 text-white rounded-lg flex items-center justify-center gap-2">
                  <X className="w-4 h-4" />
                  Abbrechen
                </button>
              </div>
            </div>
          )}

          {filteredSubcategories.map(subcat => {
            const parentCategory = categories.find(c => c.category_id === subcat.category_id);
            return (
              <div key={subcat.subcategory_id} className="glass rounded-xl p-4 flex items-center justify-between">
                <div>
                  <h3 className="text-white font-semibold">{subcat.name}</h3>
                  <p className="text-gray-400 text-sm">{subcat.abbreviation}</p>
                  {parentCategory && <p className="text-gray-500 text-xs mt-1">Kategorie: {parentCategory.name}</p>}
                </div>
                <div className="flex gap-2">
                  <button onClick={() => { setEditing(subcat.subcategory_id); setFormData(subcat); }} className="p-2 hover:bg-white/10 rounded-lg text-blue-400">
                    <Edit2 className="w-4 h-4" />
                  </button>
                  <button onClick={() => handleDelete('subcategory', subcat.subcategory_id)} className="p-2 hover:bg-white/10 rounded-lg text-red-400">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Subbiercategories */}
      {activeLevel === 'subbiercategory' && (
        <div className="space-y-2">
          <button
            onClick={() => { setEditing('new'); setFormData({}); }}
            className="px-4 py-2 bg-accent-red text-white rounded-lg font-semibold hover:shadow-lg flex items-center gap-2"
          >
            <Plus className="w-4 h-4" />
            Neue Sub-Unterkategorie
          </button>

          <div className="glass rounded-xl p-3 sm:p-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between border border-white/10">
            <div className="text-sm font-semibold text-white">Nach Unterkategorie filtern</div>
            <select
              value={subbiercategoryFilter}
              onChange={(e) => setSubbiercategoryFilter(e.target.value === '' ? '' : e.target.value)}
              className="w-full sm:w-64 px-3 py-2 rounded-lg glass text-white"
            >
              <option value="">Alle Unterkategorien</option>
              {subcategories.map(subcat => {
                const parentCategory = categories.find(c => c.category_id === subcat.category_id);
                return (
                  <option key={subcat.subcategory_id} value={subcat.subcategory_id}>
                    {subcat.name}{parentCategory ? ` (${parentCategory.name})` : ''}
                  </option>
                );
              })}
            </select>
          </div>

          {editing && (
            <div className="glass rounded-xl p-4 space-y-3 border-2 border-accent-red">
              <input
                type="text"
                placeholder="Name"
                value={formData.name || ''}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                className="w-full px-3 py-2 rounded-lg glass text-white"
              />
              <input
                type="text"
                placeholder="Abkürzung (max. 3 Zeichen)"
                maxLength={3}
                value={formData.abbreviation || ''}
                onChange={(e) => setFormData({ ...formData, abbreviation: e.target.value.toUpperCase() })}
                className="w-full px-3 py-2 rounded-lg glass text-white"
              />
              <select
                value={formData.subcategory_id || ''}
                onChange={(e) => setFormData({ ...formData, subcategory_id: e.target.value })}
                className="w-full px-3 py-2 rounded-lg glass text-white"
              >
                <option value="">-- Unterkategorie wählen --</option>
                {subcategories.map(subcat => (
                  <option key={subcat.subcategory_id} value={subcat.subcategory_id}>{subcat.name}</option>
                ))}
              </select>
              <div className="flex gap-2">
                <button onClick={handleSaveSubbiercategory} className="flex-1 px-4 py-2 bg-green-600 text-white rounded-lg flex items-center justify-center gap-2">
                  <Save className="w-4 h-4" />
                  Speichern
                </button>
                <button onClick={() => { setEditing(null); setFormData({}); }} className="flex-1 px-4 py-2 bg-gray-600 text-white rounded-lg flex items-center justify-center gap-2">
                  <X className="w-4 h-4" />
                  Abbrechen
                </button>
              </div>
            </div>
          )}

          {filteredSubbiercategories.map(subbiercat => {
            const parentSubcategory = subcategories.find(s => s.subcategory_id === subbiercat.subcategory_id);
            return (
              <div key={subbiercat.subbiercategory_id} className="glass rounded-xl p-4 flex items-center justify-between">
                <div>
                  <h3 className="text-white font-semibold">{subbiercat.name}</h3>
                  <p className="text-gray-400 text-sm">{subbiercat.abbreviation}</p>
                  {parentSubcategory && <p className="text-gray-500 text-xs mt-1">Unterkategorie: {parentSubcategory.name}</p>}
                </div>
                <div className="flex gap-2">
                  <button onClick={() => { setEditing(subbiercat.subbiercategory_id); setFormData(subbiercat); }} className="p-2 hover:bg-white/10 rounded-lg text-blue-400">
                    <Edit2 className="w-4 h-4" />
                  </button>
                  <button onClick={() => handleDelete('subbiercategory', subbiercat.subbiercategory_id)} className="p-2 hover:bg-white/10 rounded-lg text-red-400">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
