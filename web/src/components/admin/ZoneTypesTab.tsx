import { useState, useEffect } from 'react';
import { Plus, Edit2, Trash2, Save, X } from 'lucide-react';
import { api } from '../../lib/api';
import { toast } from '../../lib/toast';

interface ZoneType {
  id: number;
  key: string;
  label: string;
  description: string;
  default_led_pattern: string;
  default_led_color: string;
  default_intensity: number;
}

type ZoneTypeForm = {
  key: string;
  label: string;
  description?: string;
};

export function ZoneTypesTab() {
  const [zoneTypes, setZoneTypes] = useState<ZoneType[]>([]);
  const [editing, setEditing] = useState<number | 'new' | null>(null);
  const [formData, setFormData] = useState<ZoneTypeForm>({ key: '', label: '', description: '' });

  useEffect(() => {
    loadZoneTypes();
  }, []);

  const loadZoneTypes = async () => {
    try {
      const response = await api.get('/admin/zone-types');
      setZoneTypes(response.data);
    } catch (error) {
      toast.error('Failed to load zone types:' + " " + String(error));
    }
  };

  const handleSave = async () => {
    try {
      const payload = {
        key: formData.key,
        label: formData.label,
        description: formData.description,
      };

      if (editing === 'new') {
        await api.post('/admin/zone-types', payload);
      } else if (typeof editing === 'number') {
        await api.put(`/admin/zone-types/${editing}`, payload);
      }
      loadZoneTypes();
      setEditing(null);
      setFormData({ key: '', label: '', description: '' });
    } catch (error: any) {
      alert('Fehler: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm('Lagertyp wirklich löschen?')) return;
    try {
      await api.delete(`/admin/zone-types/${id}`);
      loadZoneTypes();
    } catch (error: any) {
      alert('Fehler: ' + (error.response?.data?.error || error.message));
    }
  };

  const startEdit = (zoneType: ZoneType) => {
    setEditing(zoneType.id);
    setFormData({
      key: zoneType.key,
      label: zoneType.label,
      description: zoneType.description,
    });
  };

  const startNew = () => {
    setEditing('new');
    setFormData({ key: '', label: '', description: '' });
  };

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex justify-between items-center">
        <h2 className="text-xl font-bold text-white">Lagertypen verwalten</h2>
        <button
          onClick={startNew}
          className="px-4 py-2 bg-accent-red text-white rounded-xl font-semibold hover:shadow-lg flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          Neuer Typ
        </button>
      </div>

      {/* Edit Form */}
      {editing && (
        <div className="glass rounded-xl p-4 space-y-3 border-2 border-accent-red">
          <p className="text-sm text-gray-400">
            LED-Farbe, Muster und Intensität verwaltest du jetzt unter{' '}
            <span className="text-white font-semibold">Admin &gt; LED-Verhalten</span>.
          </p>
          <div className="grid grid-cols-2 gap-3">
            <input
              type="text"
              placeholder="Key (z.B. shelf)"
              value={formData.key ?? ''}
              onChange={(e) => setFormData({ ...formData, key: e.target.value })}
              className="px-3 py-2 rounded-lg glass text-white"
            />
            <input
              type="text"
              placeholder="Label (z.B. Regal)"
              value={formData.label ?? ''}
              onChange={(e) => setFormData({ ...formData, label: e.target.value })}
              className="px-3 py-2 rounded-lg glass text-white"
            />
          </div>
          <input
            type="text"
            placeholder="Beschreibung"
            value={formData.description ?? ''}
            onChange={(e) => setFormData({ ...formData, description: e.target.value })}
            className="w-full px-3 py-2 rounded-lg glass text-white"
          />
          <div className="flex gap-2">
            <button onClick={handleSave} className="flex-1 px-4 py-2 bg-green-600 text-white rounded-lg flex items-center justify-center gap-2">
              <Save className="w-4 h-4" />
              Speichern
            </button>
            <button onClick={() => {setEditing(null); setFormData({ key: '', label: '', description: '' });}} className="flex-1 px-4 py-2 bg-gray-600 text-white rounded-lg flex items-center justify-center gap-2">
              <X className="w-4 h-4" />
              Abbrechen
            </button>
          </div>
        </div>
      )}

      {/* List */}
      <div className="space-y-2">
        {zoneTypes.map(zt => (
          <div key={zt.id} className="glass rounded-xl p-4 flex items-center justify-between">
            <div className="flex-1">
              <div className="flex items-center gap-3">
                <div className="w-12 h-12 rounded-lg" style={{ backgroundColor: zt.default_led_color }}></div>
                <div>
                  <h3 className="text-white font-semibold">{zt.label}</h3>
                  <p className="text-gray-400 text-sm">{zt.key} • {zt.default_led_pattern} • {zt.default_intensity}</p>
                  {zt.description && <p className="text-gray-500 text-xs">{zt.description}</p>}
                </div>
              </div>
            </div>
            <div className="flex gap-2">
              <button onClick={() => startEdit(zt)} className="p-2 hover:bg-white/10 rounded-lg text-blue-400">
                <Edit2 className="w-4 h-4" />
              </button>
              <button onClick={() => handleDelete(zt.id)} className="p-2 hover:bg-white/10 rounded-lg text-red-400">
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
