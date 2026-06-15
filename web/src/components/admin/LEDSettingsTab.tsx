import { useState, useEffect, useMemo } from 'react';
import { Save, Lightbulb, RefreshCcw, SlidersHorizontal, FileText, Square } from 'lucide-react';
import { api, ledApi, zonesApi, type LEDAppearance, type LEDJobHighlightSettings, type LEDMapping, type Zone } from '../../lib/api';
import { toast } from '../../lib/toast';

const ZONE_KEYWORDS = ['bin', 'fach', 'slot', 'compartment', 'shelf', 'gitterbox'];

const zoneLabelForOption = (zone: Zone): string => {
  const zoneName = zone.name || zone.code || 'Unbenanntes Fach';
  const code = zone.code || '';
  return code ? `${zoneName} (${code})` : zoneName;
};

interface LEDDefault {
  color: string;
  pattern: string;
  intensity: number;
}

interface ZoneTypeLED {
  id: number;
  key: string;
  label: string;
  description?: string;
  default_led_pattern: string;
  default_led_color: string;
  default_intensity: number;
}

const defaultJobSettings: LEDJobHighlightSettings = {
  mode: 'all_bins',
  required: {
    color: 'var(--color-led-green)',
    pattern: 'solid',
    intensity: 220,
    speed: 1200,
  },
  non_required: {
    color: 'var(--color-led-red)',
    pattern: 'solid',
    intensity: 160,
    speed: 1200,
  },
};

export function LEDSettingsTab() {
  const [defaults, setDefaults] = useState<LEDDefault>({
    color: 'var(--color-led-orange)',
    pattern: 'breathe',
    intensity: 180,
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');
  const [jobSettings, setJobSettings] = useState<LEDJobHighlightSettings>(defaultJobSettings);
  const [jobSaving, setJobSaving] = useState(false);
  const [jobMessage, setJobMessage] = useState('');
  const [zoneTypes, setZoneTypes] = useState<ZoneTypeLED[]>([]);
  const [zoneTypeLoading, setZoneTypeLoading] = useState(true);
  const [zoneTypeSaving, setZoneTypeSaving] = useState<number | null>(null);
  const [zoneTypeMessages, setZoneTypeMessages] = useState<Record<number, string>>({});
  const [mapping, setMapping] = useState<LEDMapping | null>(null);
  const [mappingLoading, setMappingLoading] = useState(true);
  const [mappingSaving, setMappingSaving] = useState(false);
  const [mappingValidating, setMappingValidating] = useState(false);
  const [mappingMessage, setMappingMessage] = useState('');
  const [pixelsInput, setPixelsInput] = useState<Record<string, string>>({});
  const [zones, setZones] = useState<Zone[]>([]);

  const zoneOptions = useMemo(() => {
    const filtered = zones.filter((zone) => {
      const type = (zone.type || '').toLowerCase();
      if (!type) return false;
      return ZONE_KEYWORDS.some((keyword) => type.includes(keyword));
    });
    const list = filtered.length > 0 ? filtered : zones;
    return [...list].sort((a, b) =>
      zoneLabelForOption(a).localeCompare(zoneLabelForOption(b), 'de', { sensitivity: 'base' })
    );
  }, [zones]);
  const previewBinOptions = useMemo(() => {
    if (!mapping) return [];
    const seen = new Set<string>();
    const entries: string[] = [];
    mapping.shelves.forEach((shelf) => {
      shelf.bins.forEach((bin) => {
        if (bin.bin_id && !seen.has(bin.bin_id)) {
          seen.add(bin.bin_id);
          entries.push(bin.bin_id);
        }
      });
    });
    return entries.sort((a, b) => a.localeCompare(b, 'de', { sensitivity: 'base' }));
  }, [mapping]);
  const [previewBinId, setPreviewBinId] = useState('');
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewActive, setPreviewActive] = useState(false);
  const [stopLoading, setStopLoading] = useState(false);
  const [previewMessage, setPreviewMessage] = useState('');
  const [previewTarget, setPreviewTarget] = useState<string | null>(null);

  useEffect(() => {
    loadDefaults();
    loadJobSettings();
    loadZoneTypes();
    loadMapping();
  }, []);

  useEffect(() => {
    const fetchZones = async () => {
      try {
        const { data } = await zonesApi.getAll();
        setZones(data);
      } catch (error) {
        toast.error('Failed to load zones for LED mapping:' + " " + String(error));
      }
    };

    fetchZones();
  }, []);

  const loadDefaults = async () => {
    try {
      const response = await api.get('/admin/led/single-bin-default');
      setDefaults(response.data);
    } catch (error) {
      toast.error('Failed to load LED defaults:' + " " + String(error));
    } finally {
      setLoading(false);
    }
  };

  const loadJobSettings = async () => {
    try {
      const { data } = await ledApi.getJobSettings();
      setJobSettings(data);
    } catch (error) {
      toast.error('Failed to load job highlight settings:' + " " + String(error));
    }
  };

  const loadZoneTypes = async () => {
    try {
      const response = await api.get('/admin/zone-types');
      setZoneTypes(response.data);
    } catch (error) {
      toast.error('Failed to load zone type LED defaults:' + " " + String(error));
    } finally {
      setZoneTypeLoading(false);
    }
  };

  const rebuildPixelInputs = (mappingData: LEDMapping): Record<string, string> => {
    const next: Record<string, string> = {};
    mappingData.shelves.forEach((shelf, shelfIndex) => {
      shelf.bins.forEach((bin, binIndex) => {
        next[`${shelfIndex}:${binIndex}`] = bin.pixels.join(', ');
      });
    });
    return next;
  };

  const loadMapping = async () => {
    setMappingLoading(true);
    try {
      const { data } = await ledApi.getMapping();
      setMapping(data);
      setPixelsInput(rebuildPixelInputs(data));
      if (!previewBinId.trim()) {
        const firstShelf = data.shelves.find((shelf) => shelf.bins.length > 0);
        if (firstShelf) {
          const firstBin = firstShelf.bins[0]?.bin_id;
          if (firstBin) {
            setPreviewBinId(firstBin);
          }
        }
      }
      setMappingMessage('');
    } catch (error) {
      toast.error('Failed to load LED mapping:' + " " + String(error));
      setMapping(null);
      setPixelsInput({});
      setMappingMessage('Fehler beim Laden des LED-Mappings.');
    } finally {
      setMappingLoading(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    setMessage('');

    try {
      await api.put('/admin/led/single-bin-default', defaults);
      setMessage('✓ Einstellungen gespeichert');
      setTimeout(() => setMessage(''), 3000);
    } catch (error: any) {
      setMessage('Fehler: ' + (error.response?.data?.error || error.message));
    } finally {
      setSaving(false);
    }
  };

  const updateJobAppearance = (section: 'required' | 'non_required', patch: Partial<LEDAppearance>) => {
    setJobSettings((prev) => ({
      ...prev,
      [section]: { ...prev[section], ...patch },
    }));
  };

  const handleJobSettingsSave = async () => {
    setJobSaving(true);
    setJobMessage('');

    try {
      await ledApi.updateJobSettings(jobSettings);
      setJobMessage('✓ Job-Highlight gespeichert');
      setTimeout(() => setJobMessage(''), 3000);
    } catch (error: any) {
      setJobMessage('Fehler: ' + (error.response?.data?.error || error.message));
    } finally {
      setJobSaving(false);
    }
  };

  const handleMappingValidate = async () => {
    if (!mapping) {
      setMappingMessage('Kein Mapping geladen.');
      return;
    }
    setMappingValidating(true);
    setMappingMessage('');
    try {
      const { data } = await ledApi.validateMapping(mapping);
      if (data.valid) {
        setMappingMessage(`✓ Mapping gültig (${data.total_bins ?? 0} Bereiche, ${data.total_shelves ?? 0} Gruppen)`);
      } else {
        const errors = Array.isArray(data.errors) ? data.errors.join(', ') : 'Unbekannter Fehler';
        setMappingMessage('⚠️ ' + errors);
      }
    } catch (error: any) {
      setMappingMessage('Fehler: ' + (error.response?.data?.error || error.message || error.toString()));
    } finally {
      setMappingValidating(false);
    }
  };

  const handleMappingSave = async () => {
    if (!mapping) {
      setMappingMessage('Kein Mapping geladen.');
      return;
    }
    setMappingSaving(true);
    setMappingMessage('');
    try {
      await ledApi.updateMapping(mapping);
      setMappingMessage('✓ Mapping gespeichert');
      setTimeout(() => setMappingMessage(''), 4000);
    } catch (error: any) {
      setMappingMessage('Fehler: ' + (error.response?.data?.error || error.message || error.toString()));
    } finally {
      setMappingSaving(false);
    }
  };

  const updateMappingDefaults = (patch: Partial<LEDMapping['defaults']>) => {
    setMapping((prev) => {
      if (!prev) return prev;
      return { ...prev, defaults: { ...prev.defaults, ...patch } };
    });
  };

  const updateShelfId = (shelfIndex: number, value: string) => {
    setMapping((prev) => {
      if (!prev) return prev;
      const shelves = prev.shelves.map((shelf, idx) =>
        idx === shelfIndex ? { ...shelf, shelf_id: value } : shelf
      );
      return { ...prev, shelves };
    });
  };

  const addShelf = () => {
    setMapping((prev) => {
      if (!prev) {
        return prev;
      }
      const next: LEDMapping = {
        ...prev,
        shelves: [...prev.shelves, { shelf_id: `${prev.warehouse_id}-shelf-${prev.shelves.length + 1}`, bins: [] }],
      };
      setPixelsInput(rebuildPixelInputs(next));
      return next;
    });
  };

  const removeShelf = (shelfIndex: number) => {
    setMapping((prev) => {
      if (!prev) return prev;
      const next: LEDMapping = {
        ...prev,
        shelves: prev.shelves.filter((_, idx) => idx !== shelfIndex),
      };
      setPixelsInput(rebuildPixelInputs(next));
      return next;
    });
  };

  const addBin = (shelfIndex: number) => {
    setMapping((prev) => {
      if (!prev) return prev;
      const shelves = prev.shelves.map((shelf, idx) =>
        idx === shelfIndex ? { ...shelf, bins: [...shelf.bins, { bin_id: '', pixels: [] }] } : shelf
      );
      const next: LEDMapping = { ...prev, shelves };
      setPixelsInput(rebuildPixelInputs(next));
      return next;
    });
  };

  const removeBin = (shelfIndex: number, binIndex: number) => {
    setMapping((prev) => {
      if (!prev) return prev;
      const shelves = prev.shelves.map((shelf, idx) =>
        idx === shelfIndex
          ? { ...shelf, bins: shelf.bins.filter((_, bIdx) => bIdx !== binIndex) }
          : shelf
      );
      const next: LEDMapping = { ...prev, shelves };
      setPixelsInput(rebuildPixelInputs(next));
      return next;
    });
  };

  const updateBinId = (shelfIndex: number, binIndex: number, value: string) => {
    setMapping((prev) => {
      if (!prev) return prev;
      const shelves = prev.shelves.map((shelf, idx) => {
        if (idx !== shelfIndex) return shelf;
        const bins = shelf.bins.map((bin, bIdx) =>
          bIdx === binIndex ? { ...bin, bin_id: value } : bin
        );
        return { ...shelf, bins };
      });
      return { ...prev, shelves };
    });
  };

  const parsePixels = (value: string): number[] => {
    if (!value.trim()) {
      return [];
    }
    return value
      .split(/[,\s]+/)
      .map((part) => parseInt(part.trim(), 10))
      .filter((num) => !Number.isNaN(num));
  };

  const handlePixelInputChange = (shelfIndex: number, binIndex: number, value: string) => {
    const key = `${shelfIndex}:${binIndex}`;
    setPixelsInput((prev) => ({ ...prev, [key]: value }));
    const pixels = parsePixels(value);
    setMapping((prev) => {
      if (!prev) return prev;
      const shelves = prev.shelves.map((shelf, idx) => {
        if (idx !== shelfIndex) return shelf;
        const bins = shelf.bins.map((bin, bIdx) =>
          bIdx === binIndex ? { ...bin, pixels } : bin
        );
        return { ...shelf, bins };
      });
      return { ...prev, shelves };
    });
  };

  const toPreviewAppearance = (color: string, pattern: string, intensity: number, speed?: number): LEDAppearance => ({
    color,
    pattern,
    intensity: Math.max(0, Math.min(255, intensity)),
    speed: speed && speed > 0 ? speed : 1200,
  });

  const triggerPreview = async (appearances: LEDAppearance[], targetOverride?: string, clearBefore: boolean = false) => {
    if (appearances.length === 0) {
      return;
    }
    setPreviewLoading(true);
    setPreviewMessage('');
    const formTarget = previewBinId.trim();
    const target = (targetOverride && targetOverride.trim().length > 0 ? targetOverride.trim() : '') || formTarget || undefined;
    try {
      await ledApi.preview(appearances, clearBefore, target);
      setPreviewActive(true);
      setPreviewTarget(target ?? 'all');
      setPreviewMessage(
        target
          ? `✓ Vorschau für ${target} gestartet – „Vorschau stoppen“ beendet das Leuchten.`
          : '✓ Vorschau gestartet – „Vorschau stoppen“ beendet das Leuchten.'
      );
      setTimeout(() => setPreviewMessage(''), 4000);
    } catch (error: any) {
      setPreviewMessage('Fehler bei der Vorschau: ' + (error.response?.data?.error || error.message));
      setPreviewActive(false);
      setPreviewTarget(null);
    } finally {
      setPreviewLoading(false);
    }
  };

  const handlePreviewStop = async () => {
    setStopLoading(true);
    try {
      await ledApi.clear();
      setPreviewActive(false);
      setPreviewTarget(null);
      setPreviewMessage('✓ Vorschau gestoppt.');
      setTimeout(() => setPreviewMessage(''), 3000);
    } catch (error: any) {
      setPreviewMessage('Fehler beim Stoppen: ' + (error.response?.data?.error || error.message));
    } finally {
      setStopLoading(false);
    }
  };

  const handleZoneTypeFieldChange = (id: number, field: keyof ZoneTypeLED, value: string | number) => {
    setZoneTypes((prev) =>
      prev.map((zoneType) =>
        zoneType.id === id
          ? { ...zoneType, [field]: value }
          : zoneType
      )
    );
  };

  const setZoneTypeFeedback = (id: number, text: string) => {
    setZoneTypeMessages((prev) => ({ ...prev, [id]: text }));
    if (text) {
      setTimeout(() => {
        setZoneTypeMessages((prev) => {
          const next = { ...prev };
          delete next[id];
          return next;
        });
      }, 3000);
    }
  };

  const handleZoneTypeSave = async (zoneType: ZoneTypeLED) => {
    setZoneTypeSaving(zoneType.id);
    setZoneTypeFeedback(zoneType.id, '');

    try {
      await api.put(`/admin/zone-types/${zoneType.id}`, {
        default_led_pattern: zoneType.default_led_pattern,
        default_led_color: zoneType.default_led_color,
        default_intensity: zoneType.default_intensity,
      });
      setZoneTypeFeedback(zoneType.id, '✓ Lagertyp gespeichert');
      loadZoneTypes();
    } catch (error: any) {
      setZoneTypeFeedback(
        zoneType.id,
        'Fehler: ' + (error.response?.data?.error || error.message)
      );
    } finally {
      setZoneTypeSaving(null);
    }
  };

  const applyGlobalDefaultsToZoneType = (zoneTypeId: number) => {
    setZoneTypes((prev) =>
      prev.map((zoneType) =>
        zoneType.id === zoneTypeId
          ? {
              ...zoneType,
              default_led_color: defaults.color,
              default_led_pattern: defaults.pattern,
              default_intensity: defaults.intensity,
            }
          : zoneType
      )
    );
  };

  if (loading || zoneTypeLoading) return <div className="text-white">Lädt...</div>;

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-white mb-2">LED-Verhalten</h1>
        <p className="text-gray-400">Konfiguriere das Aussehen und Verhalten der LED-Beleuchtung</p>
      </div>

      <div className="glass rounded-xl p-6 space-y-6 border border-accent-red/30">
        <div className="flex items-center gap-3">
          <Lightbulb className="w-6 h-6 text-yellow-400" />
          <div>
            <h2 className="text-xl font-bold text-white">Einzelfach-Highlight</h2>
            <p className="text-gray-400 text-sm">Standard-Aussehen für die "Fach beleuchten" Funktion</p>
          </div>
        </div>
        {/* Pattern Selection */}
        <div>
          <label className="block text-sm font-semibold text-gray-400 mb-3">Muster</label>
          <div className="grid grid-cols-3 gap-3">
            {['solid', 'breathe', 'blink'].map(pattern => (
              <button
                key={pattern}
                onClick={() => setDefaults({ ...defaults, pattern })}
                className={`px-4 py-3 rounded-xl font-semibold transition-all ${
                  defaults.pattern === pattern
                    ? 'bg-accent-red text-white shadow-lg'
                    : 'glass text-gray-400 hover:bg-white/10'
                }`}
              >
                {pattern === 'solid' && 'Durchgehend'}
                {pattern === 'breathe' && 'Atmen'}
                {pattern === 'blink' && 'Blinken'}
              </button>
            ))}
          </div>
        </div>

        {/* Color Picker */}
        <div>
          <label className="block text-sm font-semibold text-gray-400 mb-3">Farbe</label>
          <div className="flex items-center gap-4">
            <input
              type="color"
              value={defaults.color}
              onChange={(e) => setDefaults({ ...defaults, color: e.target.value })}
              className="w-20 h-20 rounded-xl cursor-pointer"
            />
            <div className="flex-1">
              <input
                type="text"
                value={defaults.color}
                onChange={(e) => setDefaults({ ...defaults, color: e.target.value })}
                className="w-full px-4 py-3 rounded-xl glass text-white font-mono"
                placeholder="#FF4500"
              />
              <p className="text-gray-500 text-xs mt-1">Hex-Format (z.B. #FF4500 für Orange)</p>
            </div>
          </div>
        </div>

        {/* Intensity Slider */}
        <div>
          <label className="block text-sm font-semibold text-gray-400 mb-3">
            Intensität: {defaults.intensity} / 255
          </label>
          <input
            type="range"
            min="0"
            max="255"
            value={defaults.intensity}
            onChange={(e) => setDefaults({ ...defaults, intensity: parseInt(e.target.value) })}
            className="w-full h-3 rounded-lg bg-white/10 appearance-none cursor-pointer"
            style={{
              background: `linear-gradient(to right, ${defaults.color} 0%, ${defaults.color} ${(defaults.intensity / 255) * 100}%, rgba(255,255,255,0.1) ${(defaults.intensity / 255) * 100}%, rgba(255,255,255,0.1) 100%)`
            }}
          />
        </div>

        {/* Preview */}
        <div>
          <label className="block text-sm font-semibold text-gray-400 mb-3">Vorschau</label>
          <div className="glass rounded-xl p-8 flex items-center justify-center">
            <div
              className="w-32 h-32 rounded-2xl transition-all duration-1000"
              style={{
                backgroundColor: defaults.color,
                opacity: defaults.intensity / 255,
                animation: defaults.pattern === 'breathe' ? 'breathe 2s infinite' : defaults.pattern === 'blink' ? 'blink 1s infinite' : 'none'
              }}
            ></div>
          </div>
        </div>

        {/* Save Button */}
        <div className="pt-4 border-t border-white/10">
          <div className="mb-3">
            <label className="block text-sm font-semibold text-gray-400 mb-2">
              Fachcode für Vorschau (optional)
            </label>
            <input
              type="text"
              list="preview-bin-options"
              value={previewBinId}
              onChange={(e) => setPreviewBinId(e.target.value)}
              placeholder="z. B. WDL-06-RG-02-F-01"
              className="w-full px-3 py-2 rounded-lg glass text-white font-mono"
            />
            <datalist id="preview-bin-options">
              {previewBinOptions.map((bin) => (
                <option key={bin} value={bin} />
              ))}
            </datalist>
            <p className="text-xs text-gray-500 mt-2">
              Leer lassen, um automatisch das erste Fach aus dem Mapping zu verwenden.
            </p>
          </div>
          <div className="flex flex-col sm:flex-row gap-3">
            <button
              onClick={handleSave}
              disabled={saving}
              className={`flex-1 py-3 px-4 sm:px-6 rounded-xl font-semibold text-white transition-all flex items-center justify-center gap-2 ${
                saving
                  ? 'bg-gray-600 cursor-not-allowed'
                  : 'bg-gradient-to-r from-accent-red to-red-700 hover:shadow-lg hover:shadow-red-500/50 hover:scale-105 active:scale-95'
              }`}
            >
              <Save className="w-5 h-5 flex-shrink-0" />
              <span className="hidden sm:inline">{saving ? 'Speichert...' : 'Einstellungen speichern'}</span>
              <span className="sm:hidden">{saving ? 'Speichert...' : 'Speichern'}</span>
            </button>
            <button
              onClick={() =>
                triggerPreview(
                  [
                    toPreviewAppearance(
                      defaults.color,
                      defaults.pattern,
                      defaults.intensity,
                      defaults.pattern === 'solid' ? 1200 : 1200
                    ),
                  ]
                )
              }
              disabled={previewLoading}
              className={`flex-1 py-3 px-4 sm:px-6 rounded-xl font-semibold transition-all flex items-center justify-center gap-2 ${
                previewLoading
                  ? 'bg-gray-600 text-gray-300 cursor-not-allowed'
                  : 'bg-white/10 text-white hover:bg-white/20'
              }`}
            >
              <Lightbulb className="w-5 h-5 flex-shrink-0 text-yellow-300" />
              <span className="hidden sm:inline">{previewLoading ? 'Vorschau läuft…' : 'LED Vorschau'}</span>
              <span className="sm:hidden">{previewLoading ? 'Läuft…' : 'Vorschau'}</span>
            </button>
            <button
              onClick={handlePreviewStop}
              disabled={(!previewActive && !previewLoading) || stopLoading}
              className={`flex-1 py-3 px-4 sm:px-6 rounded-xl font-semibold transition-all flex items-center justify-center gap-2 ${
                (!previewActive && !previewLoading) || stopLoading
                  ? 'bg-gray-600 text-gray-300 cursor-not-allowed'
                  : 'bg-white/5 text-white hover:bg-white/10'
              }`}
            >
              <Square className="w-5 h-5 flex-shrink-0 text-red-300" />
              <span className="hidden sm:inline">{stopLoading ? 'Stoppt…' : 'Vorschau stoppen'}</span>
              <span className="sm:hidden">{stopLoading ? 'Stoppt…' : 'Stop'}</span>
            </button>
          </div>

          {message && (
            <div className={`mt-3 p-3 rounded-lg text-center text-sm font-semibold ${
              message.includes('✓')
                ? 'bg-green-500/20 text-green-400'
                : 'bg-red-500/20 text-red-400'
            }`}>
              {message}
            </div>
          )}

          {previewMessage && (
            <div className={`mt-3 p-3 rounded-lg text-center text-sm font-semibold ${
              previewMessage.startsWith('✓')
                ? 'bg-green-500/15 text-green-300'
                : 'bg-red-500/20 text-red-400'
            }`}>
              {previewMessage}
            </div>
          )}
        </div>
      </div>

      {/* Inline CSS for animations */}
      <style>{`
        @keyframes breathe {
          0%, 100% { opacity: ${defaults.intensity / 255}; }
          50% { opacity: ${(defaults.intensity / 255) * 0.3}; }
        }
        @keyframes blink {
          0%, 49% { opacity: ${defaults.intensity / 255}; }
          50%, 100% { opacity: 0; }
      }
      `}</style>

      {/* Job highlight behaviour */}
      <div className="glass rounded-xl p-6 space-y-6 border border-blue-500/30">
        <div className="flex items-center gap-3">
          <SlidersHorizontal className="w-6 h-6 text-blue-400" />
          <div>
            <h2 className="text-xl font-bold text-white">Job-Highlight</h2>
            <p className="text-gray-400 text-sm">
              Konfiguriere das Aussehen der LEDs bei der Job-Visualisierung
            </p>
          </div>
        </div>

        <div className="space-y-4">
          <div className="flex flex-wrap gap-3">
            <button
              onClick={() => setJobSettings((prev) => ({ ...prev, mode: 'all_bins' }))}
              className={`px-4 py-2 rounded-lg font-semibold transition-all ${
                jobSettings.mode === 'all_bins'
                  ? 'bg-accent-red text-white shadow-lg shadow-accent-red/30'
                  : 'glass text-gray-300 hover:text-white'
              }`}
            >
              Alle Fächer markieren
            </button>
            <button
              onClick={() => setJobSettings((prev) => ({ ...prev, mode: 'required_only' }))}
              className={`px-4 py-2 rounded-lg font-semibold transition-all ${
                jobSettings.mode === 'required_only'
                  ? 'bg-accent-red text-white shadow-lg shadow-accent-red/30'
                  : 'glass text-gray-300 hover:text-white'
              }`}
            >
              Nur benötigte Fächer
            </button>
          </div>
          <p className="text-xs text-gray-500">
            {jobSettings.mode === 'all_bins'
              ? 'Alle Fächer leuchten: benötigte Fächer heben sich durch andere Farben/Pattern hervor.'
              : 'Nur Fächer mit noch zu packenden Geräten leuchten – alle anderen werden ausgeschaltet.'}
          </p>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {[{
              key: 'required' as const,
              title: 'Fächer mit fehlenden Geräten',
              description: 'Wird verwendet, wenn in einem Fach noch Geräte gepackt werden müssen.',
            }, {
              key: 'non_required' as const,
              title: 'Fächer ohne Bedarf',
              description: 'Fächer, die für den aktuellen Job nicht benötigt werden.',
            }].map((cfg) => {
              const appearance = jobSettings[cfg.key];
              return (
                <div key={cfg.key} className="glass-dark rounded-xl p-5 space-y-4 border border-white/5">
                  <div>
                    <h4 className="text-white font-semibold">{cfg.title}</h4>
                    <p className="text-xs text-gray-400 mt-1">{cfg.description}</p>
                  </div>
                  <div>
                    <label className="block text-sm font-semibold text-gray-400 mb-2">Farbe</label>
                    <div className="flex items-center gap-3">
                      <input
                        type="color"
                        value={appearance.color}
                        onChange={(e) => updateJobAppearance(cfg.key, { color: e.target.value })}
                        className="w-14 h-14 rounded-lg cursor-pointer"
                      />
                      <input
                        type="text"
                        value={appearance.color}
                        onChange={(e) => updateJobAppearance(cfg.key, { color: e.target.value })}
                        className="flex-1 px-3 py-2 rounded-lg glass text-white font-mono"
                        placeholder="#00FF00"
                      />
                    </div>
                  </div>
                  <div>
                    <label className="block text-sm font-semibold text-gray-400 mb-2">Muster</label>
                    <select
                      value={appearance.pattern}
                      onChange={(e) => updateJobAppearance(cfg.key, { pattern: e.target.value as LEDAppearance['pattern'] })}
                      className="w-full px-3 py-2 rounded-lg glass text-white"
                    >
                      <option value="solid">Durchgehend</option>
                      <option value="breathe">Atmen</option>
                      <option value="blink">Blinken</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm font-semibold text-gray-400 mb-2">
                      Intensität: {appearance.intensity} / 255
                    </label>
                    <input
                      type="range"
                      min={0}
                      max={255}
                      value={appearance.intensity}
                      onChange={(e) => updateJobAppearance(cfg.key, { intensity: parseInt(e.target.value, 10) })}
                      className="w-full h-3 rounded-lg bg-white/10 appearance-none cursor-pointer"
                      style={{
                        background: `linear-gradient(to right, ${appearance.color} 0%, ${appearance.color} ${(appearance.intensity / 255) * 100}%, rgba(255,255,255,0.1) ${(appearance.intensity / 255) * 100}%, rgba(255,255,255,0.1) 100%)`,
                      }}
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-semibold text-gray-400 mb-2">
                      Geschwindigkeit{appearance.pattern === 'solid' ? '' : `: ${appearance.speed} ms`}
                    </label>
                    <input
                      type="range"
                      min={200}
                      max={3000}
                      step={100}
                      value={appearance.pattern === 'solid' ? 1200 : appearance.speed}
                      disabled={appearance.pattern === 'solid'}
                      onChange={(e) => updateJobAppearance(cfg.key, { speed: parseInt(e.target.value, 10) })}
                      className="w-full h-3 rounded-lg bg-white/10 appearance-none cursor-pointer disabled:opacity-40"
                    />
                    <p className="text-xs text-gray-500 mt-1">
                      {appearance.pattern === 'solid'
                        ? 'Keine Animation — keine Geschwindigkeit erforderlich.'
                        : 'Niedrige Werte = schnelleres Atmen/Blinken.'}
                    </p>
                  </div>
                </div>
              );
            })}
          </div>

          <div className="pt-4 border-t border-white/10 space-y-3">
            <div className="flex flex-col sm:flex-row gap-3">
              <button
                onClick={handleJobSettingsSave}
                disabled={jobSaving}
                className={`flex-1 px-4 py-2 rounded-lg font-semibold text-white flex items-center justify-center gap-2 ${
                  jobSaving ? 'bg-gray-600 cursor-not-allowed' : 'bg-accent-red hover:bg-red-600 transition-colors'
                }`}
              >
                <Save className="w-4 h-4" />
                <span>{jobSaving ? 'Speichert...' : 'Job-Highlight speichern'}</span>
              </button>
              <button
                onClick={() =>
                  triggerPreview(
                    [
                      toPreviewAppearance(
                        jobSettings.required.color,
                        jobSettings.required.pattern,
                        jobSettings.required.intensity,
                        jobSettings.required.speed
                      ),
                      toPreviewAppearance(
                        jobSettings.non_required.color,
                        jobSettings.non_required.pattern,
                        jobSettings.non_required.intensity,
                        jobSettings.non_required.speed
                      ),
                    ]
                  )
                }
                disabled={previewLoading}
                className={`flex-1 px-4 py-2 rounded-lg font-semibold flex items-center justify-center gap-2 ${
                  previewLoading
                    ? 'bg-gray-600 text-gray-300 cursor-not-allowed'
                    : 'bg-white/10 text-white hover:bg-white/20'
                }`}
              >
                <Lightbulb className="w-4 h-4 text-yellow-300" />
                <span>{previewLoading ? 'Vorschau läuft…' : 'Job-Highlight Vorschau'}</span>
              </button>
              <button
                onClick={handlePreviewStop}
                disabled={(!previewActive && !previewLoading) || stopLoading}
                className={`flex-1 px-4 py-2 rounded-lg font-semibold flex items-center justify-center gap-2 ${
                  (!previewActive && !previewLoading) || stopLoading
                    ? 'bg-gray-600 text-gray-300 cursor-not-allowed'
                    : 'bg-white/5 text-white hover:bg-white/10'
                }`}
              >
                <Square className="w-4 h-4 text-red-300" />
                <span>{stopLoading ? 'Stoppt…' : 'Vorschau stoppen'}</span>
              </button>
            </div>
            {jobMessage && (
              <div
                className={`px-3 py-2 rounded-lg text-sm font-semibold ${
                  jobMessage.startsWith('✓') ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'
                }`}
              >
                {jobMessage}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* LED mapping editor */}
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <FileText className="w-5 h-5 text-accent-red" />
          <h3 className="text-lg font-semibold text-white">LED-Mapping konfigurieren</h3>
        </div>
        <p className="text-sm text-gray-400">
          Weise einzelnen Bereichen (z. B. Fächer, Gitterboxen) ihre LED-Pixel zu. Die Konfiguration wird im Container persistiert und survive Deployments.
        </p>

        {mappingLoading ? (
          <div className="glass rounded-xl p-5 text-sm text-gray-400">Mapping wird geladen...</div>
        ) : !mapping ? (
          <div className="glass rounded-xl p-5 text-sm text-red-400">
            Mapping konnte nicht geladen werden. Prüfe die Server-Logs.
          </div>
        ) : (
          <div className="space-y-6">
            <div className="glass rounded-xl p-6 space-y-4 border border-white/10">
              <div className="flex items-center justify-between">
                <h4 className="text-white font-semibold">Standard-Erscheinung</h4>
                <button
                  onClick={() =>
                    triggerPreview(
                      [
                        toPreviewAppearance(
                          mapping.defaults.color,
                          mapping.defaults.pattern,
                          mapping.defaults.intensity,
                          mapping.defaults.speed
                        ),
                      ]
                    )
                  }
                  disabled={previewLoading && previewTarget !== 'all'}
                  className={`px-4 py-2 rounded-lg font-semibold flex items-center justify-center gap-2 ${
                    previewActive && previewTarget === 'all'
                      ? 'bg-red-600 text-white'
                      : previewLoading
                      ? 'bg-gray-600 cursor-not-allowed text-gray-300'
                      : 'bg-white/10 text-white hover:bg-white/20'
                  }`}
                >
                  <Lightbulb className="w-4 h-4 text-yellow-300" />
                  <span>{previewActive && previewTarget === 'all' ? 'Vorschau stoppen' : previewLoading ? 'Vorschau läuft…' : 'Vorschau'}</span>
                </button>
              </div>
              <p className="text-sm text-gray-400">
                Definiere das Standard-Aussehen für LED-Highlights beim Mapping
              </p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-semibold text-gray-300 mb-2">Farbe</label>
                  <div className="flex items-center gap-3">
                    <input
                      type="color"
                      value={mapping.defaults.color}
                      onChange={(e) => updateMappingDefaults({ color: e.target.value })}
                      className="w-16 h-16 rounded-lg cursor-pointer border-2 border-white/10"
                    />
                    <input
                      type="text"
                      value={mapping.defaults.color}
                      onChange={(e) => updateMappingDefaults({ color: e.target.value })}
                      className="flex-1 px-3 py-2 rounded-lg glass text-white font-mono"
                      placeholder="#FF7A00"
                    />
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-semibold text-gray-300 mb-2">Muster</label>
                  <select
                    value={mapping.defaults.pattern}
                    onChange={(e) => updateMappingDefaults({ pattern: e.target.value })}
                    className="w-full px-3 py-2 rounded-lg glass text-white"
                  >
                    <option value="solid">Durchgehend</option>
                    <option value="breathe">Atmen</option>
                    <option value="blink">Blinken</option>
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-semibold text-gray-300 mb-2">
                    Intensität: {mapping.defaults.intensity} / 255
                  </label>
                  <input
                    type="range"
                    min={0}
                    max={255}
                    value={mapping.defaults.intensity}
                    onChange={(e) => updateMappingDefaults({ intensity: parseInt(e.target.value, 10) })}
                    className="w-full h-3 rounded-lg bg-white/10 appearance-none cursor-pointer"
                    style={{
                      background: `linear-gradient(to right, ${mapping.defaults.color} 0%, ${mapping.defaults.color} ${(mapping.defaults.intensity / 255) * 100}%, rgba(255,255,255,0.1) ${(mapping.defaults.intensity / 255) * 100}%, rgba(255,255,255,0.1) 100%)`
                    }}
                  />
                </div>
                <div>
                  <label className="block text-sm font-semibold text-gray-300 mb-2">
                    Geschwindigkeit{mapping.defaults.pattern === 'solid' ? '' : `: ${mapping.defaults.speed ?? 1200} ms`}
                  </label>
                  <input
                    type="range"
                    min={200}
                    max={3000}
                    step={100}
                    value={mapping.defaults.pattern === 'solid' ? 1200 : mapping.defaults.speed ?? 1200}
                    disabled={mapping.defaults.pattern === 'solid'}
                    onChange={(e) => updateMappingDefaults({ speed: parseInt(e.target.value, 10) })}
                    className="w-full h-3 rounded-lg bg-white/10 appearance-none cursor-pointer disabled:opacity-40"
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    {mapping.defaults.pattern === 'solid' ? 'Keine Animation' : 'Niedrigere Werte = schneller'}
                  </p>
                </div>
              </div>
            </div>

            <div className="flex flex-wrap gap-3">
              <button
                onClick={loadMapping}
                disabled={mappingLoading}
                className="flex items-center gap-2 px-3 py-2 glass text-gray-300 hover:text-white rounded-lg transition-colors disabled:opacity-50"
              >
                <RefreshCcw className="w-4 h-4" /> Neu laden
              </button>
              <button
                onClick={addShelf}
                className="flex items-center gap-2 px-3 py-2 bg-accent-red/80 hover:bg-accent-red text-white rounded-lg transition-colors"
              >
                Gruppe hinzufügen
              </button>
            </div>

            {mapping.shelves.length === 0 ? (
              <div className="glass rounded-xl p-5 text-sm text-gray-400">
                Noch keine Gruppen angelegt. Füge über „Gruppe hinzufügen“ einen Eintrag hinzu.
              </div>
            ) : (
              <div className="space-y-4">
                {mapping.shelves.map((shelf, shelfIndex) => (
                  <div key={`${shelf.shelf_id}-${shelfIndex}`} className="glass-dark rounded-xl p-5 space-y-4 border border-white/10">
                    <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
                      <div className="flex-1">
                        <label className="block text-xs font-semibold text-gray-400 mb-1">Gruppenkennung (z. B. Regal A, Gitterbox 1)</label>
                        <input
                          type="text"
                          value={shelf.shelf_id}
                          onChange={(e) => updateShelfId(shelfIndex, e.target.value)}
                          className="w-full px-3 py-2 rounded-lg glass text-white"
                        />
                      </div>
                      <button
                        onClick={() => removeShelf(shelfIndex)}
                        className="px-3 py-2 rounded-lg text-sm font-semibold bg-white/10 hover:bg-white/20 text-red-300"
                      >
                        Gruppe entfernen
                      </button>
                    </div>

                    <div className="space-y-3">
                      {shelf.bins.map((bin, binIndex) => {
                        const key = `${shelfIndex}:${binIndex}`;
                        const pixelValue = pixelsInput[key] ?? '';
                        const selectedZone = zones.find((zone) => zone.code === bin.bin_id);
                        return (
                          <div key={`${bin.bin_id}-${binIndex}`} className="glass rounded-lg p-4 space-y-3 border border-white/10">
                            <div className="grid gap-3 md:grid-cols-3">
                              <div>
                                <label className="block text-xs font-semibold text-gray-400 mb-1">Bereich auswählen</label>
                                <select
                                  value={selectedZone ? selectedZone.code : ''}
                                  onChange={(e) => updateBinId(shelfIndex, binIndex, e.target.value || bin.bin_id)}
                                  className="w-full px-3 py-2 rounded-lg glass text-white"
                                >
                                  <option value="">-- Bereich wählen --</option>
                                  {zoneOptions.map((zone) => (
                                    <option key={zone.zone_id} value={zone.code ?? ''} className="bg-dark">
                                      {zoneLabelForOption(zone)}
                                    </option>
                                  ))}
                                </select>
                                <input
                                  type="text"
                                  value={bin.bin_id}
                                  onChange={(e) => updateBinId(shelfIndex, binIndex, e.target.value)}
                                  className="mt-2 w-full px-3 py-2 rounded-lg glass text-white"
                                  placeholder="Fachcode (z.B. WDL-06-RG-02-F-01)"
                                />
                              </div>
                              <div>
                                <label className="block text-xs font-semibold text-gray-400 mb-1">LED-Pixel (Komma getrennt)</label>
                                <input
                                  type="text"
                                  value={pixelValue}
                                  onChange={(e) => handlePixelInputChange(shelfIndex, binIndex, e.target.value)}
                                  className="w-full px-3 py-2 rounded-lg glass text-white font-mono"
                                  placeholder="0,1,2,3"
                                />
                              </div>
                              <div className="flex flex-col gap-2">
                                <button
                                  onClick={() =>
                                    previewActive && previewTarget === bin.bin_id
                                      ? handlePreviewStop()
                                      : triggerPreview(
                                          [
                                            toPreviewAppearance(
                                              mapping.defaults.color,
                                              mapping.defaults.pattern,
                                              mapping.defaults.intensity,
                                              mapping.defaults.speed
                                            ),
                                          ],
                                          bin.bin_id
                                        )
                                  }
                                  disabled={previewLoading && previewTarget !== bin.bin_id}
                                  className={`px-3 py-2 rounded-lg font-semibold flex items-center justify-center gap-2 ${
                                    previewActive && previewTarget === bin.bin_id
                                      ? 'bg-red-600 text-white'
                                      : previewLoading
                                      ? 'bg-gray-600 cursor-not-allowed text-gray-300'
                                      : 'bg-white/10 text-white hover:bg-white/20'
                                  }`}
                                >
                                  <Lightbulb className="w-4 h-4 text-yellow-300" />
                                  <span>{previewActive && previewTarget === bin.bin_id ? 'Stoppen' : previewLoading && previewTarget !== bin.bin_id ? 'Wird geladen…' : 'Vorschau'}</span>
                                </button>
                                <button
                                  onClick={() => removeBin(shelfIndex, binIndex)}
                                  className="px-3 py-2 rounded-lg text-sm font-semibold bg-white/10 hover:bg-white/20 text-red-300"
                                >
                                  Fach entfernen
                                </button>
                              </div>
                            </div>
                            {selectedZone && (
                              <p className="text-xs text-gray-500">
                                {selectedZone.name}{' '}
                                {selectedZone.code && (
                                  <span className="italic text-gray-400">
                                    ({selectedZone.code})
                                  </span>
                                )}
                              </p>
                            )}
                          </div>
                        );
                      })}
                    </div>

                    <button
                      onClick={() => addBin(shelfIndex)}
                      className="px-3 py-2 rounded-lg text-sm font-semibold bg-accent-red/80 hover:bg-accent-red text-white"
                    >
                      Fach hinzufügen
                    </button>
                  </div>
                ))}
              </div>
            )}

            <div className="flex flex-wrap gap-3">
              <button
                onClick={handleMappingValidate}
                disabled={mappingValidating}
                className={`px-4 py-2 rounded-lg font-semibold flex items-center gap-2 ${
                  mappingValidating ? 'bg-gray-600 cursor-not-allowed text-gray-200' : 'glass text-gray-200 hover:text-white'
                }`}
              >
                {mappingValidating ? 'Validiere...' : 'Mapping prüfen'}
              </button>
              <button
                onClick={handleMappingSave}
                disabled={mappingSaving}
                className={`px-4 py-2 rounded-lg font-semibold text-white flex items-center gap-2 ${
                  mappingSaving ? 'bg-gray-600 cursor-not-allowed' : 'bg-accent-red hover:bg-red-600 transition-colors'
                }`}
              >
                <Save className="w-4 h-4" />
                <span>{mappingSaving ? 'Speichert...' : 'Mapping speichern'}</span>
              </button>
            </div>

            {mappingMessage && (
              <div
                className={`px-3 py-2 rounded-lg text-sm font-semibold ${
                  mappingMessage.startsWith('✓')
                    ? 'bg-green-500/20 text-green-400'
                    : mappingMessage.startsWith('⚠️')
                      ? 'bg-yellow-500/20 text-yellow-400'
                      : 'bg-red-500/20 text-red-400'
                }`}
              >
                {mappingMessage}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Zone type specific defaults */}
      <div className="glass rounded-xl p-6 space-y-6 border border-purple-500/30">
        <div className="flex items-center gap-3">
          <SlidersHorizontal className="w-6 h-6 text-purple-400" />
          <div>
            <h2 className="text-xl font-bold text-white">Lagertyp-Spezifische Einstellungen</h2>
            <p className="text-gray-400 text-sm">
              Passe das LED-Verhalten für einzelne Lagertypen an
            </p>
          </div>
        </div>

        <div className="space-y-4">
          {zoneTypes.map((zoneType) => (
            <div key={zoneType.id} className="glass rounded-xl p-5 space-y-4">
              <div className="flex flex-wrap justify-between gap-3">
                <div>
                  <h4 className="text-white font-semibold">{zoneType.label}</h4>
                  <p className="text-xs text-gray-500 font-mono">{zoneType.key}</p>
                  {zoneType.description && (
                    <p className="text-sm text-gray-400 mt-1">{zoneType.description}</p>
                  )}
                </div>
                <button
                  onClick={() => applyGlobalDefaultsToZoneType(zoneType.id)}
                  className="flex items-center gap-2 px-3 py-2 glass text-gray-300 hover:text-white rounded-lg transition-colors"
                  title="Globale LED-Standards übernehmen"
                >
                  <RefreshCcw className="w-4 h-4" />
                  <span>Global übernehmen</span>
                </button>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div>
                  <label className="block text-sm font-semibold text-gray-400 mb-2">Muster</label>
                  <select
                    value={zoneType.default_led_pattern}
                    onChange={(e) => handleZoneTypeFieldChange(zoneType.id, 'default_led_pattern', e.target.value)}
                    className="w-full px-3 py-2 rounded-lg glass text-white"
                  >
                    <option value="solid">Durchgehend</option>
                    <option value="breathe">Atmen</option>
                    <option value="blink">Blinken</option>
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-semibold text-gray-400 mb-2">Farbe</label>
                  <div className="flex items-center gap-3">
                    <input
                      type="color"
                      value={zoneType.default_led_color || '#FF7A00'}
                      onChange={(e) => handleZoneTypeFieldChange(zoneType.id, 'default_led_color', e.target.value)}
                      className="w-14 h-14 rounded-lg cursor-pointer"
                    />
                    <input
                      type="text"
                      value={zoneType.default_led_color || ''}
                      onChange={(e) => handleZoneTypeFieldChange(zoneType.id, 'default_led_color', e.target.value)}
                      className="flex-1 px-3 py-2 rounded-lg glass text-white font-mono"
                      placeholder="#FF4500"
                    />
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-semibold text-gray-400 mb-2">
                    Intensität: {zoneType.default_intensity} / 255
                  </label>
                  <input
                    type="range"
                    min="0"
                    max="255"
                    value={zoneType.default_intensity}
                    onChange={(e) =>
                      handleZoneTypeFieldChange(zoneType.id, 'default_intensity', parseInt(e.target.value, 10))
                    }
                    className="w-full h-3 rounded-lg bg-white/10 appearance-none cursor-pointer"
                    style={{
                      background: `linear-gradient(to right, ${zoneType.default_led_color || defaults.color} 0%, ${
                        zoneType.default_led_color || defaults.color
                      } ${(zoneType.default_intensity / 255) * 100}%, rgba(255,255,255,0.1) ${
                        (zoneType.default_intensity / 255) * 100
                      }%, rgba(255,255,255,0.1) 100%)`,
                    }}
                  />
                </div>
              </div>

              <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
                <div className="flex flex-col sm:flex-row gap-3 flex-1">
                  <button
                    onClick={() => handleZoneTypeSave(zoneType)}
                    disabled={zoneTypeSaving === zoneType.id}
                    className={`flex-1 px-4 py-2 rounded-lg font-semibold text-white flex items-center justify-center gap-2 ${
                      zoneTypeSaving === zoneType.id
                        ? 'bg-gray-600 cursor-not-allowed'
                        : 'bg-accent-red hover:bg-red-600 transition-colors'
                    }`}
                  >
                    <Save className="w-4 h-4" />
                    <span>{zoneTypeSaving === zoneType.id ? 'Speichert...' : 'Lagertyp speichern'}</span>
                  </button>
                  <button
                    onClick={() =>
                      triggerPreview(
                        [
                          toPreviewAppearance(
                            zoneType.default_led_color || defaults.color,
                            zoneType.default_led_pattern || 'solid',
                            zoneType.default_intensity ?? 180,
                            zoneType.default_led_pattern === 'solid' ? 1200 : 1200
                          ),
                        ]
                      )
                    }
                    disabled={previewLoading}
                    className={`flex-1 px-4 py-2 rounded-lg font-semibold flex items-center justify-center gap-2 ${
                      previewLoading
                        ? 'bg-gray-600 text-gray-300 cursor-not-allowed'
                        : 'bg-white/10 text-white hover:bg-white/20'
                    }`}
                  >
                    <Lightbulb className="w-4 h-4 text-yellow-300" />
                    <span>{previewLoading ? 'Vorschau läuft…' : 'LED Vorschau'}</span>
                  </button>
                  <button
                    onClick={handlePreviewStop}
                    disabled={(!previewActive && !previewLoading) || stopLoading}
                    className={`flex-1 px-4 py-2 rounded-lg font-semibold flex items-center justify-center gap-2 ${
                      (!previewActive && !previewLoading) || stopLoading
                        ? 'bg-gray-600 text-gray-300 cursor-not-allowed'
                        : 'bg-white/5 text-white hover:bg-white/10'
                    }`}
                  >
                    <Square className="w-4 h-4 text-red-300" />
                    <span>{stopLoading ? 'Stoppt…' : 'Vorschau stoppen'}</span>
                  </button>
            </div>
            {zoneTypeMessages[zoneType.id] && (
              <div
                className={`px-3 py-2 rounded-lg text-sm font-semibold ${
                  zoneTypeMessages[zoneType.id].startsWith('✓')
                        ? 'bg-green-500/20 text-green-400'
                        : 'bg-red-500/20 text-red-400'
                    }`}
                  >
                    {zoneTypeMessages[zoneType.id]}
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
