import { useState, useEffect } from 'react';
import {
  Wrench,
  AlertTriangle,
  CheckCircle2,
  Clock,
  XCircle,
  Plus,
  Calendar,
  TrendingUp,
} from 'lucide-react';
import { maintenanceApi } from '../lib/api';
import type { Defect, Inspection, MaintenanceStats } from '../lib/api';
import { toast } from '../lib/toast';

type TabView = 'overview' | 'defects' | 'inspections';
type DefectFilter = 'all' | 'open' | 'in_progress' | 'repaired' | 'closed';
type InspectionFilter = 'all' | 'overdue' | 'upcoming';

export function MaintenancePage() {
  const [activeTab, setActiveTab] = useState<TabView>('overview');
  const [stats, setStats] = useState<MaintenanceStats | null>(null);
  const [defects, setDefects] = useState<Defect[]>([]);
  const [inspections, setInspections] = useState<Inspection[]>([]);
  const [loading, setLoading] = useState(true);

  // Filters
  const [defectFilter, setDefectFilter] = useState<DefectFilter>('all');
  const [inspectionFilter, setInspectionFilter] = useState<InspectionFilter>('all');

  // Defect Form
  const [showDefectForm, setShowDefectForm] = useState(false);
  const [defectForm, setDefectForm] = useState({
    device_id: '',
    severity: 'medium',
    title: '',
    description: '',
  });

  useEffect(() => {
    loadData();
  }, []);

  useEffect(() => {
    if (activeTab === 'defects') {
      loadDefects();
    } else if (activeTab === 'inspections') {
      loadInspections();
    }
  }, [activeTab, defectFilter, inspectionFilter]);

  const loadData = async () => {
    try {
      setLoading(true);
      const { data } = await maintenanceApi.getStats();
      setStats(data);
    } catch (error) {
      toast.error('Failed to load maintenance stats:' + " " + String(error));
    } finally {
      setLoading(false);
    }
  };

  const loadDefects = async () => {
    try {
      const params = defectFilter !== 'all' ? { status: defectFilter } : undefined;
      const { data } = await maintenanceApi.getDefects(params);
      setDefects(data);
    } catch (error) {
      toast.error('Failed to load defects:' + " " + String(error));
    }
  };

  const loadInspections = async () => {
    try {
      const params = inspectionFilter !== 'all' ? { status: inspectionFilter } : undefined;
      const { data } = await maintenanceApi.getInspections(params);
      setInspections(data);
    } catch (error) {
      toast.error('Failed to load inspections:' + " " + String(error));
    }
  };

  const handleCreateDefect = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await maintenanceApi.createDefect(defectForm);
      setShowDefectForm(false);
      setDefectForm({ device_id: '', severity: 'medium', title: '', description: '' });
      loadData();
      loadDefects();
    } catch (error) {
      toast.error('Failed to create defect:' + " " + String(error));
      alert('Fehler beim Erstellen des Defektberichts');
    }
  };

  const handleUpdateDefectStatus = async (defectId: number, status: string) => {
    try {
      await maintenanceApi.updateDefect(defectId, { status });
      loadData();
      loadDefects();
    } catch (error) {
      toast.error('Failed to update defect:' + " " + String(error));
      alert('Fehler beim Aktualisieren des Status');
    }
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'critical':
        return 'text-red-500 bg-red-500/20';
      case 'high':
        return 'text-orange-500 bg-orange-500/20';
      case 'medium':
        return 'text-yellow-500 bg-yellow-500/20';
      case 'low':
        return 'text-blue-500 bg-blue-500/20';
      default:
        return 'text-gray-500 bg-gray-500/20';
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'open':
        return 'text-red-400 bg-red-500/20';
      case 'in_progress':
        return 'text-yellow-400 bg-yellow-500/20';
      case 'repaired':
        return 'text-green-400 bg-green-500/20';
      case 'closed':
        return 'text-gray-400 bg-gray-500/20';
      default:
        return 'text-gray-500 bg-gray-500/20';
    }
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('de-DE', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  };

  const isOverdue = (dateString?: string) => {
    if (!dateString) return false;
    return new Date(dateString) < new Date();
  };

  // Overview Tab
  if (activeTab === 'overview') {
    return (
      <div className="space-y-4 sm:space-y-6">
        <div className="mb-4 sm:mb-8">
          <h1 className="text-2xl sm:text-4xl font-bold text-white mb-1 sm:mb-2">Wartung & Instandhaltung</h1>
          <p className="text-sm sm:text-base text-gray-400">Verwaltung von Defekten, Reparaturen und Inspektionen</p>
        </div>

        {loading ? (
          <div className="text-center py-8 sm:py-12">
            <div className="inline-block animate-spin rounded-full h-10 w-10 sm:h-12 sm:w-12 border-b-2 border-accent-red"></div>
            <p className="text-sm sm:text-base text-gray-400 mt-3 sm:mt-4">Lade Daten...</p>
          </div>
        ) : (
          <>
            {/* Stats Cards */}
            <div className="grid grid-cols-2 lg:grid-cols-5 gap-2 sm:gap-4 mb-4 sm:mb-8">
              <div className="glass-dark rounded-lg sm:rounded-2xl p-3 sm:p-6 border-2 border-white/10">
                <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between mb-1 sm:mb-2">
                  <AlertTriangle className="w-6 h-6 sm:w-8 sm:h-8 text-red-500 mb-1 sm:mb-0" />
                  <span className="text-2xl sm:text-3xl font-bold text-white">
                    {stats?.open_defects || 0}
                  </span>
                </div>
                <p className="text-gray-400 text-[10px] sm:text-sm">Offene Defekte</p>
              </div>

              <div className="glass-dark rounded-lg sm:rounded-2xl p-3 sm:p-6 border-2 border-white/10">
                <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between mb-1 sm:mb-2">
                  <Wrench className="w-6 h-6 sm:w-8 sm:h-8 text-yellow-500 mb-1 sm:mb-0" />
                  <span className="text-2xl sm:text-3xl font-bold text-white">
                    {stats?.in_progress_defects || 0}
                  </span>
                </div>
                <p className="text-gray-400 text-[10px] sm:text-sm">In Bearbeitung</p>
              </div>

              <div className="glass-dark rounded-lg sm:rounded-2xl p-3 sm:p-6 border-2 border-white/10">
                <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between mb-1 sm:mb-2">
                  <CheckCircle2 className="w-6 h-6 sm:w-8 sm:h-8 text-green-500 mb-1 sm:mb-0" />
                  <span className="text-2xl sm:text-3xl font-bold text-white">
                    {stats?.repaired_defects || 0}
                  </span>
                </div>
                <p className="text-gray-400 text-[10px] sm:text-sm">Repariert</p>
              </div>

              <div className="glass-dark rounded-lg sm:rounded-2xl p-3 sm:p-6 border-2 border-white/10">
                <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between mb-1 sm:mb-2">
                  <XCircle className="w-6 h-6 sm:w-8 sm:h-8 text-red-500 mb-1 sm:mb-0" />
                  <span className="text-2xl sm:text-3xl font-bold text-white">
                    {stats?.overdue_inspections || 0}
                  </span>
                </div>
                <p className="text-gray-400 text-[10px] sm:text-sm">Überfällige Prüfungen</p>
              </div>

              <div className="glass-dark rounded-lg sm:rounded-2xl p-3 sm:p-6 border-2 border-white/10">
                <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between mb-1 sm:mb-2">
                  <Calendar className="w-6 h-6 sm:w-8 sm:h-8 text-blue-500 mb-1 sm:mb-0" />
                  <span className="text-2xl sm:text-3xl font-bold text-white">
                    {stats?.upcoming_inspections || 0}
                  </span>
                </div>
                <p className="text-gray-400 text-[10px] sm:text-sm">Anstehende Prüfungen</p>
              </div>
            </div>

            {/* Quick Actions */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 sm:gap-6">
              <button
                onClick={() => setActiveTab('defects')}
                className="glass-dark rounded-xl sm:rounded-2xl p-4 sm:p-8 border-2 border-white/10 hover:border-accent-red transition-all text-left group"
              >
                <div className="flex items-center justify-between mb-3 sm:mb-4">
                  <AlertTriangle className="w-8 h-8 sm:w-12 sm:h-12 text-accent-red" />
                  <TrendingUp className="w-5 h-5 sm:w-6 sm:h-6 text-gray-500 group-hover:text-accent-red transition-colors" />
                </div>
                <h3 className="text-lg sm:text-2xl font-bold text-white mb-1 sm:mb-2">Defektmanagement</h3>
                <p className="text-xs sm:text-base text-gray-400">
                  Defektberichte erstellen, Reparaturen verfolgen und Status aktualisieren
                </p>
              </button>

              <button
                onClick={() => setActiveTab('inspections')}
                className="glass-dark rounded-xl sm:rounded-2xl p-4 sm:p-8 border-2 border-white/10 hover:border-accent-red transition-all text-left group"
              >
                <div className="flex items-center justify-between mb-3 sm:mb-4">
                  <Calendar className="w-8 h-8 sm:w-12 sm:h-12 text-blue-500" />
                  <TrendingUp className="w-5 h-5 sm:w-6 sm:h-6 text-gray-500 group-hover:text-accent-red transition-colors" />
                </div>
                <h3 className="text-lg sm:text-2xl font-bold text-white mb-1 sm:mb-2">Inspektionen</h3>
                <p className="text-xs sm:text-base text-gray-400">
                  Prüfintervalle verwalten, überfällige Prüfungen ansehen
                </p>
              </button>
            </div>
          </>
        )}
      </div>
    );
  }

  // Defects Tab
  if (activeTab === 'defects') {
    return (
      <div className="space-y-4 sm:space-y-6">
        <div className="mb-4 sm:mb-6">
          <button
            onClick={() => setActiveTab('overview')}
            className="text-sm sm:text-base text-gray-400 hover:text-white mb-3 sm:mb-4 flex items-center gap-2"
          >
            ← Zurück zur Übersicht
          </button>

          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 sm:gap-0">
            <div>
              <h1 className="text-2xl sm:text-4xl font-bold text-white mb-1 sm:mb-2">Defektberichte</h1>
              <p className="text-sm sm:text-base text-gray-400">Verwaltung von Defekten und Reparaturen</p>
            </div>
            <button
              onClick={() => setShowDefectForm(!showDefectForm)}
              className="px-4 sm:px-6 py-2.5 sm:py-3 bg-gradient-to-r from-accent-red to-red-700 text-white font-semibold text-sm sm:text-base rounded-lg sm:rounded-xl hover:shadow-lg hover:shadow-accent-red/50 transition-all flex items-center gap-2 self-start sm:self-auto"
            >
              <Plus className="w-4 h-4 sm:w-5 sm:h-5" />
              Neuer Defekt
            </button>
          </div>
        </div>

        {/* Create Defect Form */}
        {showDefectForm && (
          <div className="glass-dark rounded-xl sm:rounded-2xl p-4 sm:p-6 border-2 border-white/10 mb-4 sm:mb-6">
            <h3 className="text-lg sm:text-xl font-bold text-white mb-3 sm:mb-4">Defektbericht erstellen</h3>
            <form onSubmit={handleCreateDefect} className="space-y-3 sm:space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 sm:gap-4">
                <div>
                  <label className="block text-xs sm:text-sm font-medium text-gray-400 mb-1.5 sm:mb-2">
                    Geräte-ID *
                  </label>
                  <input
                    type="text"
                    value={defectForm.device_id}
                    onChange={(e) =>
                      setDefectForm({ ...defectForm, device_id: e.target.value })
                    }
                    required
                    className="w-full px-3 sm:px-4 py-2 sm:py-3 bg-white/10 border-2 border-white/20 rounded-lg sm:rounded-xl text-sm sm:text-base text-white focus:outline-none focus:border-accent-red"
                  />
                </div>

                <div>
                  <label className="block text-xs sm:text-sm font-medium text-gray-400 mb-1.5 sm:mb-2">
                    Schweregrad *
                  </label>
                  <select
                    value={defectForm.severity}
                    onChange={(e) =>
                      setDefectForm({ ...defectForm, severity: e.target.value })
                    }
                    className="w-full px-3 sm:px-4 py-2 sm:py-3 bg-white/10 border-2 border-white/20 rounded-lg sm:rounded-xl text-sm sm:text-base text-white focus:outline-none focus:border-accent-red"
                  >
                    <option value="low">Niedrig</option>
                    <option value="medium">Mittel</option>
                    <option value="high">Hoch</option>
                    <option value="critical">Kritisch</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-xs sm:text-sm font-medium text-gray-400 mb-1.5 sm:mb-2">
                  Titel *
                </label>
                <input
                  type="text"
                  value={defectForm.title}
                  onChange={(e) => setDefectForm({ ...defectForm, title: e.target.value })}
                  required
                  className="w-full px-3 sm:px-4 py-2 sm:py-3 bg-white/10 border-2 border-white/20 rounded-lg sm:rounded-xl text-sm sm:text-base text-white focus:outline-none focus:border-accent-red"
                />
              </div>

              <div>
                <label className="block text-xs sm:text-sm font-medium text-gray-400 mb-1.5 sm:mb-2">
                  Beschreibung *
                </label>
                <textarea
                  value={defectForm.description}
                  onChange={(e) =>
                    setDefectForm({ ...defectForm, description: e.target.value })
                  }
                  required
                  rows={3}
                  className="w-full px-3 sm:px-4 py-2 sm:py-3 bg-white/10 border-2 border-white/20 rounded-lg sm:rounded-xl text-sm sm:text-base text-white focus:outline-none focus:border-accent-red resize-none"
                />
              </div>

              <div className="flex flex-col sm:flex-row gap-2 sm:gap-3">
                <button
                  type="submit"
                  className="px-4 sm:px-6 py-2.5 sm:py-3 bg-gradient-to-r from-accent-red to-red-700 text-white font-semibold text-sm sm:text-base rounded-lg sm:rounded-xl hover:shadow-lg transition-all"
                >
                  Defekt erstellen
                </button>
                <button
                  type="button"
                  onClick={() => setShowDefectForm(false)}
                  className="px-4 sm:px-6 py-2.5 sm:py-3 glass text-gray-400 hover:text-white font-semibold text-sm sm:text-base rounded-lg sm:rounded-xl transition-all"
                >
                  Abbrechen
                </button>
              </div>
            </form>
          </div>
        )}

        {/* Filter Tabs */}
        <div className="flex gap-2 mb-4 sm:mb-6 overflow-x-auto pb-2">
          {(['all', 'open', 'in_progress', 'repaired', 'closed'] as DefectFilter[]).map(
            (filter) => (
              <button
                key={filter}
                onClick={() => setDefectFilter(filter)}
                className={`px-3 sm:px-4 py-1.5 sm:py-2 rounded-lg sm:rounded-xl text-xs sm:text-sm font-semibold whitespace-nowrap transition-all ${
                  defectFilter === filter
                    ? 'bg-accent-red text-white'
                    : 'glass text-gray-400 hover:text-white'
                }`}
              >
                {filter === 'all' ? 'Alle' : filter === 'open' ? 'Offen' : filter === 'in_progress' ? 'In Bearbeitung' : filter === 'repaired' ? 'Repariert' : 'Geschlossen'}
              </button>
            )
          )}
        </div>

        {/* Defects List */}
        <div className="space-y-3 sm:space-y-4">
          {defects.length === 0 ? (
            <div className="glass-dark rounded-xl sm:rounded-2xl p-8 sm:p-12 text-center">
              <AlertTriangle className="w-12 h-12 sm:w-16 sm:h-16 text-gray-600 mx-auto mb-3 sm:mb-4" />
              <p className="text-sm sm:text-lg text-gray-400">Keine Defektberichte gefunden</p>
            </div>
          ) : (
            defects.map((defect) => (
              <div
                key={defect.defect_id}
                className="glass-dark rounded-xl sm:rounded-2xl p-4 sm:p-6 border-2 border-white/10 hover:border-white/20 transition-all"
              >
                <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-3 sm:gap-4">
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2 mb-2">
                      <h3 className="text-base sm:text-xl font-bold text-white truncate">{defect.title}</h3>
                      <span
                        className={`px-2 sm:px-3 py-0.5 sm:py-1 rounded-full text-[10px] sm:text-xs font-semibold ${getSeverityColor(
                          defect.severity
                        )}`}
                      >
                        {defect.severity.toUpperCase()}
                      </span>
                      <span
                        className={`px-2 sm:px-3 py-0.5 sm:py-1 rounded-full text-[10px] sm:text-xs font-semibold ${getStatusColor(
                          defect.status
                        )}`}
                      >
                        {defect.status}
                      </span>
                    </div>
                    <p className="text-gray-400 text-xs sm:text-sm mb-2 sm:mb-3 line-clamp-2">{defect.description}</p>
                    <div className="flex flex-col sm:flex-row sm:items-center gap-1.5 sm:gap-4 text-xs sm:text-sm text-gray-500">
                      <span className="truncate">Gerät: {defect.device_id}</span>
                      {defect.product_name && <span className="hidden sm:inline">•</span>}
                      {defect.product_name && <span className="truncate">{defect.product_name}</span>}
                      <span className="hidden sm:inline">•</span>
                      <span>Gemeldet: {formatDate(defect.reported_at)}</span>
                      {defect.repair_cost && (
                        <>
                          <span className="hidden sm:inline">•</span>
                          <span>Kosten: €{defect.repair_cost.toFixed(2)}</span>
                        </>
                      )}
                    </div>
                  </div>

                  {/* Status Update Buttons */}
                  {defect.status !== 'closed' && (
                    <div className="flex flex-row lg:flex-col gap-2 self-start">
                      {defect.status === 'open' && (
                        <button
                          onClick={() =>
                            handleUpdateDefectStatus(defect.defect_id, 'in_progress')
                          }
                          className="px-3 sm:px-4 py-1.5 sm:py-2 bg-yellow-500/20 text-yellow-400 rounded-lg sm:rounded-xl hover:bg-yellow-500/30 transition-all text-xs sm:text-sm font-semibold whitespace-nowrap"
                        >
                          In Bearbeitung
                        </button>
                      )}
                      {defect.status === 'in_progress' && (
                        <button
                          onClick={() =>
                            handleUpdateDefectStatus(defect.defect_id, 'repaired')
                          }
                          className="px-3 sm:px-4 py-1.5 sm:py-2 bg-green-500/20 text-green-400 rounded-lg sm:rounded-xl hover:bg-green-500/30 transition-all text-xs sm:text-sm font-semibold whitespace-nowrap"
                        >
                          Repariert
                        </button>
                      )}
                      {defect.status === 'repaired' && (
                        <button
                          onClick={() =>
                            handleUpdateDefectStatus(defect.defect_id, 'closed')
                          }
                          className="px-3 sm:px-4 py-1.5 sm:py-2 bg-gray-500/20 text-gray-400 rounded-lg sm:rounded-xl hover:bg-gray-500/30 transition-all text-xs sm:text-sm font-semibold whitespace-nowrap"
                        >
                          Schließen
                        </button>
                      )}
                    </div>
                  )}
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    );
  }

  // Inspections Tab
  return (
    <div className="space-y-4 sm:space-y-6">
      <div className="mb-4 sm:mb-6">
        <button
          onClick={() => setActiveTab('overview')}
          className="text-sm sm:text-base text-gray-400 hover:text-white mb-3 sm:mb-4 flex items-center gap-2"
        >
          ← Zurück zur Übersicht
        </button>

        <h1 className="text-2xl sm:text-4xl font-bold text-white mb-1 sm:mb-2">Inspektionen</h1>
        <p className="text-sm sm:text-base text-gray-400">Verwaltung von Prüfintervallen und anstehenden Inspektionen</p>
      </div>

      {/* Filter Tabs */}
      <div className="flex gap-2 mb-4 sm:mb-6 overflow-x-auto pb-2">
        {(['all', 'overdue', 'upcoming'] as InspectionFilter[]).map((filter) => (
          <button
            key={filter}
            onClick={() => setInspectionFilter(filter)}
            className={`px-3 sm:px-4 py-1.5 sm:py-2 rounded-lg sm:rounded-xl text-xs sm:text-sm font-semibold whitespace-nowrap transition-all ${
              inspectionFilter === filter
                ? 'bg-accent-red text-white'
                : 'glass text-gray-400 hover:text-white'
            }`}
          >
            {filter === 'all' ? 'Alle' : filter === 'overdue' ? 'Überfällig' : 'Anstehend (30 Tage)'}
          </button>
        ))}
      </div>

      {/* Inspections List */}
      <div className="space-y-3 sm:space-y-4">
        {inspections.length === 0 ? (
          <div className="glass-dark rounded-xl sm:rounded-2xl p-8 sm:p-12 text-center">
            <Calendar className="w-12 h-12 sm:w-16 sm:h-16 text-gray-600 mx-auto mb-3 sm:mb-4" />
            <p className="text-sm sm:text-lg text-gray-400">Keine Inspektionen gefunden</p>
          </div>
        ) : (
          inspections.map((inspection) => (
            <div
              key={inspection.schedule_id}
              className={`glass-dark rounded-xl sm:rounded-2xl p-4 sm:p-6 border-2 ${
                isOverdue(inspection.next_inspection)
                  ? 'border-red-500/50'
                  : 'border-white/10'
              } hover:border-white/20 transition-all`}
            >
              <div className="flex items-start justify-between">
                <div className="flex-1 min-w-0">
                  <div className="flex flex-wrap items-center gap-2 mb-2 sm:mb-3">
                    <h3 className="text-base sm:text-xl font-bold text-white truncate">
                      {inspection.inspection_type}
                    </h3>
                    {isOverdue(inspection.next_inspection) && (
                      <span className="px-2 sm:px-3 py-0.5 sm:py-1 rounded-full text-[10px] sm:text-xs font-semibold bg-red-500/20 text-red-400">
                        ÜBERFÄLLIG
                      </span>
                    )}
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 sm:gap-4 text-xs sm:text-sm mb-2 sm:mb-3">
                    <div>
                      <p className="text-gray-500 text-[10px] sm:text-xs">Gerät/Produkt</p>
                      <p className="text-white font-semibold truncate">
                        {inspection.device_name || inspection.product_name || '-'}
                      </p>
                    </div>
                    <div>
                      <p className="text-gray-500 text-[10px] sm:text-xs">Intervall</p>
                      <p className="text-white font-semibold">
                        {inspection.interval_days} Tage
                      </p>
                    </div>
                    <div>
                      <p className="text-gray-500 text-[10px] sm:text-xs">Letzte Prüfung</p>
                      <p className="text-white font-semibold">
                        {inspection.last_inspection
                          ? formatDate(inspection.last_inspection)
                          : 'Noch keine'}
                      </p>
                    </div>
                  </div>

                  {inspection.next_inspection && (
                    <div className="mt-2 sm:mt-3 flex items-center gap-2">
                      <Clock
                        className={`w-3.5 h-3.5 sm:w-4 sm:h-4 flex-shrink-0 ${
                          isOverdue(inspection.next_inspection)
                            ? 'text-red-500'
                            : 'text-blue-500'
                        }`}
                      />
                      <span
                        className={`text-xs sm:text-sm font-semibold ${
                          isOverdue(inspection.next_inspection)
                            ? 'text-red-400'
                            : 'text-blue-400'
                        }`}
                      >
                        Nächste Prüfung: {formatDate(inspection.next_inspection)}
                      </span>
                    </div>
                  )}
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
