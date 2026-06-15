import { useState, useEffect } from 'react';
import { Save, Database, AlertCircle } from 'lucide-react';
import { adminSettingsApi, type APILimits } from '../../lib/api';
import { toast } from '../../lib/toast';

export function APISettingsTab() {
  const [limits, setLimits] = useState<APILimits>({ device_limit: 50000, case_limit: 50000 });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  useEffect(() => {
    loadLimits();
  }, []);

  const loadLimits = async () => {
    try {
      const { data } = await adminSettingsApi.getAPILimits();
      setLimits(data);
    } catch (error) {
      toast.error('Failed to load API limits:' + " " + String(error));
      setMessage({ type: 'error', text: 'Fehler beim Laden der Einstellungen' });
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    // Validation
    if (limits.device_limit < 1 || limits.case_limit < 1) {
      setMessage({ type: 'error', text: 'Limits müssen mindestens 1 sein' });
      return;
    }

    if (limits.device_limit > 1000000 || limits.case_limit > 1000000) {
      setMessage({ type: 'error', text: 'Limits dürfen nicht größer als 1.000.000 sein' });
      return;
    }

    setSaving(true);
    setMessage(null);

    try {
      const { data } = await adminSettingsApi.updateAPILimits(limits);
      setLimits({ device_limit: data.device_limit, case_limit: data.case_limit });
      setMessage({ type: 'success', text: 'Einstellungen erfolgreich gespeichert!' });
    } catch (error) {
      toast.error('Failed to update API limits:' + " " + String(error));
      setMessage({ type: 'error', text: 'Fehler beim Speichern der Einstellungen' });
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-accent-red"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3 mb-6">
        <Database className="w-6 h-6 text-accent-red" />
        <div>
          <h2 className="text-2xl font-bold text-white">API-Limits</h2>
          <p className="text-gray-400 text-sm">
            Konfiguriere die maximale Anzahl von Devices und Cases, die von der API geladen werden
          </p>
        </div>
      </div>

      {message && (
        <div
          className={`p-4 rounded-lg flex items-center gap-3 ${
            message.type === 'success'
              ? 'bg-green-500/10 border border-green-500/20'
              : 'bg-red-500/10 border border-red-500/20'
          }`}
        >
          <AlertCircle
            className={`w-5 h-5 ${message.type === 'success' ? 'text-green-500' : 'text-red-500'}`}
          />
          <span className={message.type === 'success' ? 'text-green-500' : 'text-red-500'}>
            {message.text}
          </span>
        </div>
      )}

      <div className="space-y-6">
        {/* Device Limit */}
        <div className="bg-white/5 rounded-xl p-6 border border-white/10">
          <label className="block text-sm font-medium text-gray-300 mb-3">Device API Limit</label>
          <p className="text-sm text-gray-400 mb-4">
            Maximale Anzahl von Devices, die beim Laden der Label-Seite und anderen Anfragen geladen werden.
          </p>
          <input
            type="number"
            min="1"
            max="1000000"
            value={limits.device_limit}
            onChange={(e) =>
              setLimits({ ...limits, device_limit: parseInt(e.target.value) || 1 })
            }
            className="w-full sm:w-64 px-4 py-3 bg-dark-light border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
            placeholder="z.B. 50000"
          />
          <div className="mt-2 text-xs text-gray-500">
            Empfohlen: 50000 für normale Nutzung, höher für sehr große Bestände
          </div>
        </div>

        {/* Case Limit */}
        <div className="bg-white/5 rounded-xl p-6 border border-white/10">
          <label className="block text-sm font-medium text-gray-300 mb-3">Case API Limit</label>
          <p className="text-sm text-gray-400 mb-4">
            Maximale Anzahl von Cases, die beim Laden der Label-Seite und anderen Anfragen geladen werden.
          </p>
          <input
            type="number"
            min="1"
            max="1000000"
            value={limits.case_limit}
            onChange={(e) =>
              setLimits({ ...limits, case_limit: parseInt(e.target.value) || 1 })
            }
            className="w-full sm:w-64 px-4 py-3 bg-dark-light border border-white/20 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-red transition-colors"
            placeholder="z.B. 50000"
          />
          <div className="mt-2 text-xs text-gray-500">
            Empfohlen: 50000 für normale Nutzung, höher für sehr viele Cases
          </div>
        </div>

        {/* Info Box */}
        <div className="bg-blue-500/10 border border-blue-500/20 rounded-xl p-6">
          <div className="flex items-start gap-3">
            <AlertCircle className="w-5 h-5 text-blue-400 flex-shrink-0 mt-0.5" />
            <div className="space-y-2">
              <h3 className="text-blue-400 font-semibold">Wichtige Hinweise</h3>
              <ul className="text-sm text-blue-300 space-y-1 list-disc list-inside">
                <li>Sehr hohe Limits können die Performance beeinträchtigen</li>
                <li>Die Limits gelten nur für die API-Anfragen, nicht für die Datenbank</li>
                <li>Änderungen werden sofort wirksam, kein Neustart erforderlich</li>
                <li>Bei sehr großen Datenbeständen kann das Laden länger dauern</li>
              </ul>
            </div>
          </div>
        </div>
      </div>

      {/* Save Button */}
      <div className="flex justify-end pt-4">
        <button
          onClick={handleSave}
          disabled={saving}
          className="flex items-center gap-2 px-6 py-3 bg-accent-red hover:bg-accent-red/80 disabled:bg-gray-600 disabled:cursor-not-allowed text-white font-semibold rounded-lg transition-colors"
        >
          <Save className="w-5 h-5" />
          {saving ? 'Speichert...' : 'Einstellungen speichern'}
        </button>
      </div>
    </div>
  );
}
