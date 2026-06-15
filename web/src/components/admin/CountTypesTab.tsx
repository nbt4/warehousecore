import { useEffect, useState } from 'react';
import { Plus, Edit2, Trash2, Save, X } from 'lucide-react';
import { api } from '../../lib/api';
import { toast } from '../../lib/toast';

interface CountType {
  count_type_id: number;
  name: string;
  abbreviation: string;
  is_active: boolean;
}

interface CountTypeFormData {
  name: string;
  abbreviation: string;
  is_active: boolean;
}

export function CountTypesTab() {
  const [countTypes, setCountTypes] = useState<CountType[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [saving, setSaving] = useState<boolean>(false);
  const [editing, setEditing] = useState<number | 'new' | null>(null);
  const [formData, setFormData] = useState<CountTypeFormData>({
    name: '',
    abbreviation: '',
    is_active: true,
  });
  const [message, setMessage] = useState<string>('');

  const loadCountTypes = async () => {
    setLoading(true);
    try {
      const { data } = await api.get<CountType[]>('/admin/count-types');
      setCountTypes(data || []);
    } catch (error: any) {
      toast.error('Failed to load measurement units:' + " " + String(error));
      setMessage('Mess-Einheiten konnten nicht geladen werden.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadCountTypes();
  }, []);

  const resetForm = () => {
    setEditing(null);
    setFormData({ name: '', abbreviation: '', is_active: true });
  };

  const handleSave = async () => {
    if (!formData.name.trim() || !formData.abbreviation.trim()) {
      setMessage('Name und Abkürzung sind erforderlich.');
      return;
    }

    setSaving(true);
    setMessage('');
    try {
      if (editing === 'new') {
        await api.post('/admin/count-types', formData);
      } else {
        await api.put(`/admin/count-types/${editing}`, formData);
      }
      await loadCountTypes();
      resetForm();
      setMessage('Mess-Einheit gespeichert.');
    } catch (error: any) {
      toast.error('Failed to save measurement unit:' + " " + String(error));
      setMessage(error?.response?.data?.error || 'Speichern fehlgeschlagen.');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm('Mess-Einheit wirklich löschen?')) return;
    setMessage('');
    try {
      await api.delete(`/admin/count-types/${id}`);
      await loadCountTypes();
      resetForm();
    } catch (error: any) {
      toast.error('Failed to delete measurement unit:' + " " + String(error));
      setMessage(error?.response?.data?.error || 'Löschen fehlgeschlagen.');
    }
  };

  const startEdit = (ct: CountType) => {
    setEditing(ct.count_type_id);
    setFormData({
      name: ct.name,
      abbreviation: ct.abbreviation,
      is_active: ct.is_active,
    });
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-xl font-bold text-white">Mess-Einheiten verwalten</h2>
          <p className="text-gray-400 text-sm">Einheiten für Zubehör und Verbrauchsmaterialien</p>
        </div>
        <button
          onClick={() => {
            setEditing('new');
            setFormData({ name: '', abbreviation: '', is_active: true });
          }}
          className="px-4 py-2 bg-accent-red text-white rounded-lg font-semibold hover:shadow-lg flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          Neue Einheit
        </button>
      </div>

      {message && (
        <div
          className={`px-4 py-3 rounded-lg text-sm font-semibold ${
            message.toLowerCase().includes('fehler') || message.toLowerCase().includes('fehlgeschlagen')
              ? 'bg-red-500/20 text-red-300'
              : 'bg-green-500/20 text-green-300'
          }`}
        >
          {message}
        </div>
      )}

      {editing && (
        <div className="glass rounded-xl p-4 space-y-3 border-2 border-accent-red">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <input
              type="text"
              placeholder="Name (z.B. Kilogramm)"
              value={formData.name}
              onChange={e => setFormData({ ...formData, name: e.target.value })}
              className="w-full px-3 py-2 rounded-lg glass text-white"
            />
            <input
              type="text"
              placeholder="Abkürzung (z.B. kg)"
              maxLength={10}
              value={formData.abbreviation}
              onChange={e => setFormData({ ...formData, abbreviation: e.target.value })}
              className="w-full px-3 py-2 rounded-lg glass text-white"
            />
            <label className="flex items-center gap-2 text-white font-semibold">
              <input
                type="checkbox"
                checked={formData.is_active}
                onChange={e => setFormData({ ...formData, is_active: e.target.checked })}
                className="w-5 h-5 rounded border-white/20 bg-white/10 text-accent-red focus:ring-accent-red"
              />
              Aktiv
            </label>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleSave}
              disabled={saving}
              className="flex-1 px-4 py-2 bg-green-600 text-white rounded-lg flex items-center justify-center gap-2 disabled:opacity-50"
            >
              <Save className="w-4 h-4" />
              {saving ? 'Speichert...' : 'Speichern'}
            </button>
            <button
              onClick={resetForm}
              className="flex-1 px-4 py-2 bg-gray-600 text-white rounded-lg flex items-center justify-center gap-2"
            >
              <X className="w-4 h-4" />
              Abbrechen
            </button>
          </div>
        </div>
      )}

      <div className="space-y-2">
        {loading ? (
          <div className="text-white">Lädt...</div>
        ) : countTypes.length === 0 ? (
          <div className="text-gray-400">Keine Mess-Einheiten gefunden.</div>
        ) : (
          countTypes.map(ct => (
            <div key={ct.count_type_id} className="glass rounded-xl p-4 flex items-center justify-between">
              <div>
                <h3 className="text-white font-semibold">{ct.name}</h3>
                <p className="text-gray-400 text-sm">{ct.abbreviation}</p>
                <p className={`text-xs mt-1 ${ct.is_active ? 'text-green-400' : 'text-gray-500'}`}>
                  {ct.is_active ? 'Aktiv' : 'Inaktiv'}
                </p>
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => startEdit(ct)}
                  className="p-2 hover:bg-white/10 rounded-lg text-blue-400"
                >
                  <Edit2 className="w-4 h-4" />
                </button>
                <button
                  onClick={() => handleDelete(ct.count_type_id)}
                  className="p-2 hover:bg-white/10 rounded-lg text-red-400"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
