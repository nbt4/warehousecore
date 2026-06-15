import { useEffect, useState } from 'react';
import { KeyRound, Plus, Trash2, ToggleLeft, ToggleRight, RefreshCcw, Copy } from 'lucide-react';
import { apiKeysAdminApi, type APIKeyItem } from '../../lib/api';
import { toast } from '../../lib/toast';

export function APIKeysTab() {
  const [keys, setKeys] = useState<APIKeyItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const [plainKey, setPlainKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadKeys();
  }, []);

  const loadKeys = async () => {
    setLoading(true);
    setError(null);
    try {
      const { data } = await apiKeysAdminApi.list();
      setKeys(data.keys || []);
    } catch (err) {
      toast.error('Failed to load API keys' + " " + String(err));
      setError('Konnte API-Keys nicht laden');
    } finally {
      setLoading(false);
    }
  };

  const createKey = async () => {
    if (!newName.trim()) {
      setError('Name ist erforderlich');
      return;
    }
    setCreating(true);
    setError(null);
    setPlainKey(null);
    try {
      const { data } = await apiKeysAdminApi.create({ name: newName.trim() });
      setPlainKey(data.api_key);
      setNewName('');
      await loadKeys();
    } catch (err) {
      toast.error('Failed to create API key' + " " + String(err));
      setError('Erstellen fehlgeschlagen');
    } finally {
      setCreating(false);
    }
  };

  const toggleKey = async (id: number, active: boolean) => {
    try {
      await apiKeysAdminApi.updateStatus(id, !active);
      await loadKeys();
    } catch (err) {
      toast.error('Failed to update API key status' + " " + String(err));
      setError('Aktualisieren fehlgeschlagen');
    }
  };

  const deleteKey = async (id: number) => {
    if (!confirm('Diesen API-Key wirklich löschen?')) return;
    try {
      await apiKeysAdminApi.delete(id);
      await loadKeys();
    } catch (err) {
      toast.error('Failed to delete API key' + " " + String(err));
      setError('Löschen fehlgeschlagen');
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <KeyRound className="w-6 h-6 text-accent-red" />
        <div>
          <h2 className="text-2xl font-bold text-white">API-Keys</h2>
          <p className="text-gray-400 text-sm">Zugriff auf die öffentlichen Website-Feeds absichern</p>
        </div>
      </div>

      {error && <div className="bg-red-500/10 border border-red-500/30 text-red-400 px-4 py-3 rounded-lg">{error}</div>}

      <div className="glass-dark rounded-xl p-4 flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="flex-1">
          <label className="block text-sm text-gray-300 mb-1">Name / Beschreibung</label>
          <input
            className="w-full px-4 py-3 bg-dark-light border border-white/10 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red"
            placeholder="z.B. tsweb-frontend"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
          />
        </div>
        <button
          onClick={createKey}
          disabled={creating}
          className="flex items-center gap-2 px-4 py-3 bg-accent-red hover:bg-accent-red/80 rounded-lg text-white font-semibold disabled:opacity-60"
        >
          <Plus className="w-4 h-4" /> Neuer Key
        </button>
        <button
          onClick={loadKeys}
          className="flex items-center gap-2 px-4 py-3 bg-white/10 hover:bg-white/20 rounded-lg text-white"
        >
          <RefreshCcw className="w-4 h-4" /> Aktualisieren
        </button>
      </div>

      {plainKey && (
        <div className="bg-green-500/10 border border-green-500/20 text-green-200 px-4 py-3 rounded-lg flex items-center justify-between gap-3">
          <div>
            <div className="text-sm font-semibold">Neuer API-Key (nur einmal sichtbar):</div>
            <div className="font-mono break-all">{plainKey}</div>
          </div>
          <button
            onClick={() => navigator.clipboard.writeText(plainKey)}
            className="flex items-center gap-2 px-3 py-2 bg-green-500/20 hover:bg-green-500/30 rounded-md text-green-100"
          >
            <Copy className="w-4 h-4" /> Kopieren
          </button>
        </div>
      )}

      <div className="glass-dark rounded-xl overflow-hidden">
        <div className="grid grid-cols-12 px-4 py-3 text-sm font-semibold text-gray-300 border-b border-white/5">
          <div className="col-span-4">Name</div>
          <div className="col-span-3">Status</div>
          <div className="col-span-3">Zuletzt genutzt</div>
          <div className="col-span-2 text-right">Aktionen</div>
        </div>
        {loading ? (
          <div className="p-6 text-center text-gray-400">Lade...</div>
        ) : keys.length === 0 ? (
          <div className="p-6 text-center text-gray-400">Keine API-Keys vorhanden</div>
        ) : (
          keys.map((k) => (
            <div
              key={k.id}
              className="grid grid-cols-12 px-4 py-3 items-center border-b border-white/5 text-sm text-gray-100"
            >
              <div className="col-span-4">
                <div className="font-semibold text-white">{k.name}</div>
                <div className="text-xs text-gray-400">ID: {k.id}</div>
              </div>
              <div className="col-span-3 flex items-center gap-2">
                {k.is_active ? (
                  <>
                    <ToggleRight className="w-5 h-5 text-green-400" />
                    <span className="text-green-300">Aktiv</span>
                  </>
                ) : (
                  <>
                    <ToggleLeft className="w-5 h-5 text-gray-500" />
                    <span className="text-gray-400">Inaktiv</span>
                  </>
                )}
                <button
                  onClick={() => toggleKey(k.id, k.is_active)}
                  className="ml-3 px-3 py-1 rounded-md bg-white/10 hover:bg-white/20 text-white text-xs"
                >
                  {k.is_active ? 'Deaktivieren' : 'Aktivieren'}
                </button>
              </div>
              <div className="col-span-3 text-gray-300 text-sm">
                {k.last_used_at ? new Date(k.last_used_at).toLocaleString() : '—'}
              </div>
              <div className="col-span-2 flex justify-end gap-2">
                <button
                  onClick={() => deleteKey(k.id)}
                  className="p-2 rounded-md bg-red-500/10 hover:bg-red-500/20 text-red-300"
                  title="Löschen"
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
