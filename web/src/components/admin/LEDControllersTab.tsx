import { useEffect, useMemo, useState } from 'react';
import { ledApi, api, type LEDController, type LEDControllerPayload, type ZoneTypeDefinition } from '../../lib/api';
import { Plus, Save, X, RefreshCcw, Trash2, Cpu, Settings, RotateCw } from 'lucide-react';
import { useBlockBodyScroll } from '../../hooks/useBlockBodyScroll';

type EditorTarget = number | 'new' | null;

interface ControllerForm {
  controller_id: string;
  display_name: string;
  topic_suffix: string;
  is_active: boolean;
  metadata: string;
  zoneTypeIds: number[];
}

const defaultForm: ControllerForm = {
  controller_id: '',
  display_name: '',
  topic_suffix: '',
  is_active: true,
  metadata: '{\n  \n}',
  zoneTypeIds: [],
};

const ONLINE_THRESHOLD_MS = 5 * 60 * 1000; // 5 minutes

const formatUptime = (seconds?: number): string | null => {
  if (seconds === undefined || seconds === null || Number.isNaN(seconds)) return null;
  const totalSeconds = Math.max(0, Math.floor(seconds));
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const parts: string[] = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (parts.length === 0) {
    const secs = totalSeconds % 60;
    parts.push(`${secs}s`);
  }
  return parts.join(' ');
};

export function LEDControllersTab() {
  const [controllers, setControllers] = useState<LEDController[]>([]);
  const [zoneTypes, setZoneTypes] = useState<ZoneTypeDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [editor, setEditor] = useState<EditorTarget>(null);
  const [form, setForm] = useState<ControllerForm>(defaultForm);
  const [message, setMessage] = useState<string>('');
  const [configureLedCount, setConfigureLedCount] = useState<number>(600);
  const [configureDataPin, setConfigureDataPin] = useState<number>(0);
  const [configureChipset, setConfigureChipset] = useState<string>('SK6812_GRBW');
  const [configuring, setConfiguring] = useState(false);
  const [restarting, setRestarting] = useState<number | null>(null);

  // Block body scroll when editor modal is open
  useBlockBodyScroll(editor !== null);

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    setLoading(true);
    try {
      const [controllerRes, zoneTypeRes] = await Promise.all([
        ledApi.getControllers(),
        api.get<ZoneTypeDefinition[]>('/admin/zone-types'),
      ]);
      setControllers(controllerRes.data);
      setZoneTypes(zoneTypeRes.data);
    } catch (error) {
      console.error('Failed to load controllers:', error);
      setMessage('Fehler beim Laden der Controller.');
    } finally {
      setLoading(false);
    }
  };

  const startNew = () => {
    setEditor('new');
    setForm({ ...defaultForm, metadata: '{\n\n}', zoneTypeIds: [] });
  };

  const startEdit = (controller: LEDController) => {
    setEditor(controller.id);
    setForm({
      controller_id: controller.controller_id,
      display_name: controller.display_name,
      topic_suffix: controller.topic_suffix,
      is_active: controller.is_active,
      metadata: controller.metadata ? JSON.stringify(controller.metadata, null, 2) : '{\n\n}',
      zoneTypeIds: controller.zone_types?.map((zt) => zt.id) ?? [],
    });

    // Load hardware config from controller status
    const statusData = controller.status_data as Record<string, unknown> | undefined;
    const currentLedCount = typeof statusData?.['led_count'] === 'number' ? statusData['led_count'] as number : 600;
    const currentDataPin = typeof statusData?.['data_pin'] === 'number' ? statusData['data_pin'] as number : 0;
    const currentChipset = typeof statusData?.['chipset'] === 'string' ? statusData['chipset'] as string : 'SK6812_GRBW';
    setConfigureLedCount(currentLedCount);
    setConfigureDataPin(currentDataPin);
    setConfigureChipset(currentChipset);
  };

  const resetEditor = () => {
    setEditor(null);
    setForm(defaultForm);
    setSaving(false);
  };

  const parseMetadata = (): Record<string, unknown> | null => {
    if (!form.metadata.trim()) return null;
    try {
      return JSON.parse(form.metadata);
    } catch (error) {
      alert('Ungültiges Metadata JSON: ' + (error as Error).message);
      throw error;
    }
  };

  const sortedZoneTypes = useMemo(() => zoneTypes.slice().sort((a, b) => a.label.localeCompare(b.label, 'de')), [zoneTypes]);

  const selectedZones = useMemo(
    () => sortedZoneTypes.filter((zone) => form.zoneTypeIds.includes(zone.id)),
    [sortedZoneTypes, form.zoneTypeIds],
  );

  const handleZoneSelectChange = (values: number[]) => {
    setForm((prev) => ({ ...prev, zoneTypeIds: values }));
  };

  const handleSave = async () => {
    try {
      setSaving(true);
      const payload: LEDControllerPayload = {
        controller_id: editor === 'new' ? form.controller_id.trim() : undefined,
        display_name: form.display_name.trim(),
        topic_suffix: form.topic_suffix.trim(),
        is_active: form.is_active,
        metadata: parseMetadata(),
        zone_type_ids: form.zoneTypeIds,
      };

      if (editor === 'new') {
        if (!payload.controller_id) {
          alert('Controller ID ist erforderlich.');
          return;
        }
        if (!payload.display_name) {
          alert('Anzeigename ist erforderlich.');
          return;
        }
        await ledApi.createController(payload);
        setMessage('✓ Controller angelegt');
      } else if (typeof editor === 'number') {
        await ledApi.updateController(editor, payload);
        setMessage('✓ Controller aktualisiert');
      }
      resetEditor();
      loadData();
    } catch (error: any) {
      const msg = error.response?.data?.error || error.message || 'Unbekannter Fehler';
      alert('Fehler: ' + msg);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm('Controller deaktivieren? Er wird aus der Liste entfernt und taucht nur wieder auf, wenn er einen neuen Heartbeat sendet.')) return;
    try {
      // Set is_active to false instead of deleting
      await ledApi.updateController(id, { is_active: false });
      setMessage('✓ Controller deaktiviert');
      if (editor === id) {
        resetEditor();
      }
      loadData();
    } catch (error: any) {
      const msg = error.response?.data?.error || error.message || 'Unbekannter Fehler';
      alert('Fehler: ' + msg);
    }
  };

  const handleHardwareConfig = async () => {
    if (editor === null || editor === 'new') return;
    if (configureLedCount < 1 || configureLedCount > 1200) {
      alert('LED-Anzahl muss zwischen 1 und 1200 liegen.');
      return;
    }
    if (configureDataPin < 0 || configureDataPin > 50) {
      alert('Data Pin muss zwischen 0 und 50 liegen.');
      return;
    }
    try {
      setConfiguring(true);
      await ledApi.configureController(editor, {
        led_count: configureLedCount,
        data_pin: configureDataPin,
        chipset: configureChipset,
      });
      setMessage(`✓ Hardware-Konfiguration gesendet.`);
      // Reload after 2 seconds to show updated values
      setTimeout(() => loadData(), 2000);
    } catch (error: any) {
      const msg = error.response?.data?.error || error.message || 'Unbekannter Fehler';
      alert('Fehler beim Senden der Hardware-Konfiguration: ' + msg);
    } finally {
      setConfiguring(false);
    }
  };

  const handleRestart = async (controllerId: number) => {
    if (!confirm('ESP32 wirklich neu starten? Der Controller wird für ca. 5-10 Sekunden offline sein.')) return;

    try {
      setRestarting(controllerId);
      await ledApi.restartController(controllerId);
      setMessage('✓ Neustart-Befehl gesendet. ESP32 startet in 2 Sekunden neu.');
      // Reload after 10 seconds to show controller coming back online
      setTimeout(() => {
        loadData();
        setRestarting(null);
      }, 10000);
    } catch (error: any) {
      const msg = error.response?.data?.error || error.message || 'Unbekannter Fehler';
      alert('Fehler: ' + msg);
      setRestarting(null);
    }
  };

  const handleDeleteOffline = async () => {
    const offlineControllers = controllers.filter(c => {
      if (!c.last_seen) return true;
      const diff = Date.now() - new Date(c.last_seen).getTime();
      return diff >= ONLINE_THRESHOLD_MS;
    });

    if (offlineControllers.length === 0) {
      alert('Keine offline Controller gefunden.');
      return;
    }

    if (!confirm(`${offlineControllers.length} offline Controller löschen?`)) return;

    try {
      await Promise.all(offlineControllers.map(c => ledApi.deleteController(c.id)));
      setMessage(`✓ ${offlineControllers.length} offline Controller gelöscht`);
      loadData();
    } catch (error: any) {
      const msg = error.response?.data?.error || error.message || 'Unbekannter Fehler';
      alert('Fehler: ' + msg);
    }
  };

  const controllerStatus = (controller: LEDController) => {
    if (!controller.last_seen) return { label: 'Offline', className: 'text-gray-400' };
    const last = new Date(controller.last_seen).getTime();
    const diff = Date.now() - last;
    if (diff < ONLINE_THRESHOLD_MS) {
      return { label: 'Online', className: 'text-green-400' };
    }
    return { label: 'Offline', className: 'text-gray-400' };
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Cpu className="w-5 h-5 text-accent-red" /> Mikrocontroller verwalten
          </h2>
          <p className="text-gray-400 text-sm">Verwalte verbaute Controller, deren Topics und zugehörige Lagerarten.</p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleDeleteOffline}
            className="px-3 py-2 bg-red-600/20 text-red-400 rounded-lg flex items-center gap-2 text-sm hover:bg-red-600/30"
            title="Alle offline Controller löschen"
          >
            <Trash2 className="w-4 h-4" /> Offline löschen
          </button>
          <button
            onClick={loadData}
            className="px-3 py-2 bg-white/10 text-white rounded-lg flex items-center gap-2 text-sm hover:bg-white/20"
          >
            <RefreshCcw className="w-4 h-4" /> Neu laden
          </button>
          <button
            onClick={startNew}
            className="px-4 py-2 bg-accent-red text-white rounded-lg flex items-center gap-2 text-sm hover:bg-accent-red/80"
          >
            <Plus className="w-4 h-4" /> Neuer Controller
          </button>
        </div>
      </div>

      {message && (
        <div className="glass rounded-xl px-4 py-2 text-sm text-green-400">{message}</div>
      )}

      {editor && (
        <div
          className="fixed inset-0 z-[120] flex min-h-screen items-center justify-center bg-black/80 p-4"
          style={{ backgroundColor: 'var(--bg-overlay)' }}
          onClick={resetEditor}
        >
          <div
            className="relative bg-gradient-to-b from-gray-900 to-black rounded-2xl border border-accent-red/50 shadow-2xl w-full max-w-3xl max-h-[85vh] overflow-hidden flex flex-col"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Header */}
            <div className="bg-gradient-to-r from-accent-red/10 to-transparent border-b border-white/10 px-6 py-4 flex items-center justify-between">
              <h3 className="text-xl font-bold text-white flex items-center gap-3">
                <div className="w-10 h-10 rounded-lg bg-accent-red/20 flex items-center justify-center">
                  <Settings className="w-5 h-5 text-accent-red" />
                </div>
                {editor === 'new' ? 'Neuer Controller' : 'Controller bearbeiten'}
              </h3>
              <button
                onClick={resetEditor}
                className="p-2 hover:bg-white/10 rounded-lg transition-all hover:rotate-90 duration-300"
                title="Schließen"
              >
                <X className="w-5 h-5 text-gray-400 hover:text-white" />
              </button>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-y-auto p-6 space-y-6">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {editor === 'new' && (
                  <div>
                    <label className="block text-sm font-semibold text-gray-300 mb-2">Controller ID</label>
                    <input
                      type="text"
                      className="w-full px-4 py-2.5 rounded-lg bg-white/5 border border-white/10 text-white focus:border-accent-red focus:ring-2 focus:ring-accent-red/20 transition-all placeholder-gray-500"
                      placeholder="z.B. esp-regal-1"
                      value={form.controller_id}
                      onChange={(e) => setForm((prev) => ({ ...prev, controller_id: e.target.value }))}
                    />
                  </div>
                )}
                <div>
                  <label className="block text-sm font-semibold text-gray-300 mb-2">Anzeigename</label>
                  <input
                    type="text"
                    className="w-full px-4 py-2.5 rounded-lg bg-white/5 border border-white/10 text-white focus:border-accent-red focus:ring-2 focus:ring-accent-red/20 transition-all placeholder-gray-500"
                    placeholder="Anzeigename"
                    value={form.display_name}
                    onChange={(e) => setForm((prev) => ({ ...prev, display_name: e.target.value }))}
                  />
                </div>
                <div>
                  <label className="block text-sm font-semibold text-gray-300 mb-2">Topic-Suffix (optional)</label>
                  <input
                    type="text"
                    className="w-full px-4 py-2.5 rounded-lg bg-white/5 border border-white/10 text-white focus:border-accent-red focus:ring-2 focus:ring-accent-red/20 transition-all placeholder-gray-500"
                    placeholder="optional"
                    value={form.topic_suffix}
                    onChange={(e) => setForm((prev) => ({ ...prev, topic_suffix: e.target.value }))}
                  />
                </div>
                <div>
                  <label className="block text-sm font-semibold text-gray-300 mb-2">Status</label>
                  <label className="flex items-center gap-3 px-4 py-2.5 bg-white/5 border border-white/10 rounded-lg cursor-pointer hover:bg-white/10 transition-all">
                    <input
                      type="checkbox"
                      className="w-5 h-5 rounded border-gray-600 text-accent-red focus:ring-accent-red focus:ring-2"
                      checked={form.is_active}
                      onChange={(e) => setForm((prev) => ({ ...prev, is_active: e.target.checked }))}
                    />
                    <span className="text-sm text-gray-300 font-medium">Aktiv</span>
                  </label>
                </div>
              </div>

              <div className="space-y-3">
                <label className="block text-sm font-semibold text-gray-300">Zuständige Lagerzonen</label>
                {selectedZones.length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {selectedZones.map((zone) => (
                      <span key={zone.id} className="bg-gradient-to-r from-accent-red/20 to-accent-red/10 text-accent-red px-3 py-1.5 rounded-lg text-sm font-medium border border-accent-red/30">
                        {zone.label}
                      </span>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-gray-500 italic">Noch keine Zonen zugewiesen.</p>
                )}
                <select
                  multiple
                  value={form.zoneTypeIds.map(String)}
                  onChange={(e) => {
                    const selected = Array.from(e.target.selectedOptions).map((opt) => Number(opt.value));
                    handleZoneSelectChange(selected);
                  }}
                  className="w-full rounded-lg bg-white/5 border border-white/10 text-white px-4 py-3 focus:border-accent-red focus:ring-2 focus:ring-accent-red/20 transition-all min-h-[120px]"
                >
                  {sortedZoneTypes.map((zone) => (
                    <option key={zone.id} value={zone.id} className="py-1">
                      {zone.label}
                    </option>
                  ))}
                </select>
                <p className="text-xs text-gray-500">
                  💡 Tipp: Halte <span className="font-semibold text-gray-400">Strg / Cmd</span>, um mehrere Zonen auszuwählen.
                </p>
              </div>

              <div>
                <label className="block text-sm font-semibold text-gray-300 mb-2">Metadata (JSON)</label>
                <textarea
                  className="w-full h-32 rounded-lg bg-white/5 border border-white/10 text-white font-mono text-sm px-4 py-3 focus:border-accent-red focus:ring-2 focus:ring-accent-red/20 transition-all"
                  value={form.metadata}
                  onChange={(e) => setForm((prev) => ({ ...prev, metadata: e.target.value }))}
                  placeholder='{\n  "key": "value"\n}'
                />
              </div>

              {editor !== 'new' && (
                <div className="bg-gradient-to-br from-blue-500/10 via-transparent to-purple-500/10 rounded-xl p-5 space-y-4 border border-blue-500/30">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-lg bg-blue-500/20 flex items-center justify-center">
                      <Cpu className="w-5 h-5 text-blue-400" />
                    </div>
                    <div>
                      <h4 className="text-white font-bold">Hardware-Konfiguration</h4>
                      <p className="text-xs text-gray-400">
                        ESP32 per MQTT konfigurieren und dauerhaft speichern
                      </p>
                    </div>
                  </div>
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                    <div>
                      <label className="block text-sm font-semibold text-gray-300 mb-2">LED-Anzahl</label>
                      <input
                        type="number"
                        min="1"
                        max="1200"
                        className="w-full px-4 py-2.5 rounded-lg bg-white/5 border border-white/10 text-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all"
                        value={configureLedCount}
                        onChange={(e) => setConfigureLedCount(parseInt(e.target.value) || 0)}
                      />
                      <p className="text-xs text-gray-500 mt-1">1-1200 LEDs</p>
                    </div>
                    <div>
                      <label className="block text-sm font-semibold text-gray-300 mb-2">Data Pin (GPIO)</label>
                      <input
                        type="number"
                        min="0"
                        max="50"
                        className="w-full px-4 py-2.5 rounded-lg bg-white/5 border border-white/10 text-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all"
                        value={configureDataPin}
                        onChange={(e) => setConfigureDataPin(parseInt(e.target.value) || 0)}
                      />
                      <p className="text-xs text-gray-500 mt-1">GPIO 0-50</p>
                    </div>
                    <div>
                      <label className="block text-sm font-semibold text-gray-300 mb-2">LED-Chipset</label>
                      <select
                        className="w-full px-4 py-2.5 rounded-lg bg-white/5 border border-white/10 text-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all"
                        value={configureChipset}
                        onChange={(e) => setConfigureChipset(e.target.value)}
                      >
                        <option value="SK6812_GRBW">SK6812 GRBW</option>
                        <option value="SK6812_GRB">SK6812 GRB</option>
                        <option value="WS2812B">WS2812B</option>
                        <option value="WS2811">WS2811</option>
                        <option value="APA102">APA102</option>
                      </select>
                      <p className="text-xs text-gray-500 mt-1">LED-Typ</p>
                    </div>
                  </div>
                  <div className="flex flex-col sm:flex-row gap-3 items-center">
                    <div className="bg-yellow-500/10 border border-yellow-500/30 rounded-lg p-3 flex-1">
                      <p className="text-yellow-300 text-xs">
                        <strong>⚠️ Hinweis:</strong> Pin/Chipset-Änderungen erfordern ESP32-Neustart
                      </p>
                    </div>
                    <button
                      onClick={handleHardwareConfig}
                      disabled={configuring}
                      className="px-5 py-2.5 rounded-lg text-sm font-bold bg-gradient-to-r from-blue-600 to-blue-500 text-white flex items-center justify-center gap-2 disabled:opacity-50 hover:from-blue-700 hover:to-blue-600 transition-all shadow-lg shadow-blue-500/20 whitespace-nowrap"
                      title="Hardware-Konfiguration senden"
                    >
                      <Save className="w-4 h-4" /> {configuring ? 'Sende...' : 'Hardware speichern'}
                    </button>
                  </div>
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="border-t border-white/10 bg-gradient-to-r from-gray-900 to-black px-6 py-4 flex gap-3 justify-end">
              <button
                onClick={resetEditor}
                className="px-6 py-2.5 rounded-lg text-sm font-bold bg-white/5 border border-white/10 text-gray-300 flex items-center gap-2 hover:bg-white/10 transition-all"
              >
                <X className="w-4 h-4" /> Abbrechen
              </button>
              <button
                onClick={handleSave}
                disabled={saving}
                className="px-6 py-2.5 rounded-lg text-sm font-bold bg-gradient-to-r from-green-600 to-green-500 text-white flex items-center gap-2 disabled:opacity-50 hover:from-green-700 hover:to-green-600 transition-all shadow-lg shadow-green-500/20"
              >
                <Save className="w-4 h-4" /> {saving ? 'Speichert...' : 'Speichern'}
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="space-y-2">
        {loading ? (
          <div className="glass rounded-xl p-5 text-center text-gray-400">Lade Controller...</div>
        ) : controllers.length === 0 ? (
          <div className="glass rounded-xl p-5 text-center text-gray-400">Noch keine Controller registriert.</div>
        ) : (
          controllers
            .filter((c) => {
              if (!c.is_active || !c.last_seen) return false;
              const diff = Date.now() - new Date(c.last_seen).getTime();
              return diff < ONLINE_THRESHOLD_MS;
            })
            .map((controller) => {
            const status = controllerStatus(controller);
            const statusData = controller.status_data as Record<string, unknown> | undefined;
            const wifiRSSI =
              typeof statusData?.['wifi_rssi'] === 'number' ? (statusData['wifi_rssi'] as number) : undefined;
            const uptimeSeconds =
              typeof statusData?.['uptime_seconds'] === 'number' ? (statusData['uptime_seconds'] as number) : undefined;
            const ledCount =
              typeof statusData?.['led_count'] === 'number' ? (statusData['led_count'] as number) : undefined;
            const heartbeatAtRaw =
              typeof statusData?.['heartbeat_received_at'] === 'string'
                ? (statusData['heartbeat_received_at'] as string)
                : undefined;
            const heartbeatAt = heartbeatAtRaw ? new Date(heartbeatAtRaw) : undefined;
            const uptimeLabel = formatUptime(uptimeSeconds);
            return (
              <div key={controller.id} className="glass rounded-xl p-4 flex flex-col gap-3 border border-white/10">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-3">
                    <div className={`w-2 h-2 rounded-full flex-shrink-0 ${status.className}`}></div>
                    <div className="min-w-0 flex-1">
                      <h3 className="text-white font-semibold text-sm sm:text-base flex items-center gap-2 flex-wrap">
                        <span className="truncate">{controller.display_name}</span>
                        {!controller.is_active && <span className="text-xs text-gray-500 flex-shrink-0">deaktiviert</span>}
                      </h3>
                      <p className="text-xs text-gray-400 truncate">
                        ID: <span className="font-mono">{controller.controller_id}</span> • Topic: <span className="font-mono">{controller.topic_suffix}</span>
                      </p>
                      <p className={`text-xs ${status.className}`}>{status.label}</p>
                      {controller.zone_types && controller.zone_types.length > 0 && (
                        <p className="text-xs text-gray-400 mt-1">
                          Lagerarten: {controller.zone_types.map((zt) => zt.label).join(', ')}
                        </p>
                      )}
                      {controller.last_seen && (
                        <p className="text-xs text-gray-500">Letzter Kontakt: {new Date(controller.last_seen).toLocaleString()}</p>
                      )}
                      <div className="mt-2 grid grid-cols-1 sm:grid-cols-2 gap-y-1 text-xs text-gray-400 break-all">
                        {controller.hostname && <span className="truncate">Hostname: <span className="font-mono text-gray-300">{controller.hostname}</span></span>}
                        {controller.ip_address && <span className="truncate">IP: <span className="font-mono text-gray-300">{controller.ip_address}</span></span>}
                        {controller.mac_address && <span className="truncate">MAC: <span className="font-mono text-gray-300">{controller.mac_address}</span></span>}
                        {controller.firmware_version && <span className="truncate">Firmware: <span className="font-mono text-gray-300">{controller.firmware_version}</span></span>}
                        {typeof ledCount === 'number' && <span>LEDs: <span className="font-mono text-gray-300">{ledCount}</span></span>}
                        {wifiRSSI !== undefined && <span>WiFi RSSI: <span className="font-mono text-gray-300">{wifiRSSI} dBm</span></span>}
                        {uptimeLabel && <span>Uptime: <span className="font-mono text-gray-300">{uptimeLabel}</span></span>}
                        {heartbeatAt && <span>Heartbeat: <span className="font-mono text-gray-300">{heartbeatAt.toLocaleTimeString()}</span></span>}
                      </div>
                    </div>
                  </div>
                </div>
                <div className="flex flex-wrap gap-2">
                  {status.label === 'Online' && (
                    <button
                      onClick={() => handleRestart(controller.id)}
                      disabled={restarting === controller.id}
                      className="px-3 py-2 rounded-lg text-sm font-semibold bg-orange-600/20 text-orange-400 hover:bg-orange-600/30 flex items-center gap-2 disabled:opacity-50"
                      title="ESP32 neu starten"
                    >
                      <RotateCw className="w-4 h-4" /> <span className="hidden sm:inline">{restarting === controller.id ? 'Startet...' : 'Neustart'}</span>
                    </button>
                  )}
                  <button
                    onClick={() => startEdit(controller)}
                    className="px-3 py-2 rounded-lg text-sm font-semibold bg-blue-600/20 text-blue-400 hover:bg-blue-600/30 flex items-center gap-2"
                    title="Controller bearbeiten & konfigurieren"
                  >
                    <Settings className="w-4 h-4" /> <span className="hidden sm:inline">Bearbeiten</span>
                  </button>
                  <button
                    onClick={() => handleDelete(controller.id)}
                    className="p-2 rounded-lg text-red-400 hover:bg-white/10"
                    title="Controller löschen"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
