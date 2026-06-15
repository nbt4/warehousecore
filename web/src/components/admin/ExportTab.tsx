import { useState } from 'react';
import { Download, Package, Building2, Tag, Layers, Cable, Briefcase, FileText, TrendingUp } from 'lucide-react';
import { toast } from '../../lib/toast';

type ExportType = {
  id: string;
  label: string;
  description: string;
  icon: typeof Download;
};

export function ExportTab() {
  const [loading, setLoading] = useState<string | null>(null);

  const exportTypes: ExportType[] = [
    {
      id: 'products',
      label: 'Alle Produkte',
      description: 'Exportiert alle Produkte mit Basisdaten (ID, Name, Beschreibung, Kategorie, etc.)',
      icon: Package,
    },
    {
      id: 'products-with-count',
      label: 'Produkte mit Geräteanzahl',
      description: 'Alle Produkte mit Anzahl verfügbarer, im Einsatz und defekter Geräte',
      icon: TrendingUp,
    },
    {
      id: 'products-with-brand',
      label: 'Produkte mit Marke & Hersteller',
      description: 'Alle Produkte inklusive Hersteller- und Markeninformationen',
      icon: Tag,
    },
    {
      id: 'devices',
      label: 'Alle Geräte',
      description: 'Vollständige Geräteliste mit Status, Seriennummer, Kaufdatum und Standort',
      icon: FileText,
    },
    {
      id: 'manufacturers',
      label: 'Alle Hersteller',
      description: 'Liste aller Hersteller mit Land, Webseite und Notizen',
      icon: Building2,
    },
    {
      id: 'manufacturers-with-brands',
      label: 'Hersteller mit Marken',
      description: 'Hersteller mit zugeordneten Markennamen',
      icon: Building2,
    },
    {
      id: 'brands',
      label: 'Alle Marken',
      description: 'Liste aller Marken mit Hersteller-Zuordnung',
      icon: Tag,
    },
    {
      id: 'zones',
      label: 'Alle Lagerbereiche',
      description: 'Lagerzonen mit Typ, Barcode, Kapazität und Geräteanzahl',
      icon: Layers,
    },
    {
      id: 'cables',
      label: 'Alle Kabel',
      description: 'Kabel mit Typ, Steckern, Länge und Spezifikationen',
      icon: Cable,
    },
    {
      id: 'jobs',
      label: 'Alle Jobs',
      description: 'Jobs mit Datum, Kunde, Status und Geräteanzahl',
      icon: Briefcase,
    },
  ];

  const handleExport = async (exportType: string, label: string) => {
    setLoading(exportType);

    try {
      const response = await fetch(`/api/v1/admin/export/${exportType}`, {
        method: 'GET',
        credentials: 'include',
      });

      if (!response.ok) {
        throw new Error('Export failed');
      }

      // Get the blob from response
      const blob = await response.blob();

      // Create download link
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;

      // Get filename from Content-Disposition header or generate one
      const contentDisposition = response.headers.get('Content-Disposition');
      let filename = `export_${exportType}_${new Date().toISOString().split('T')[0]}.csv`;

      if (contentDisposition) {
        const filenameMatch = contentDisposition.match(/filename=(.+)/);
        if (filenameMatch) {
          filename = filenameMatch[1];
        }
      }

      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      window.URL.revokeObjectURL(url);

      // Show success message
      const event = new CustomEvent('toast', {
        detail: {
          type: 'success',
          message: `${label} erfolgreich exportiert`,
        },
      });
      window.dispatchEvent(event);
    } catch (error) {
      toast.error('Export error:' + " " + String(error));
      const event = new CustomEvent('toast', {
        detail: {
          type: 'error',
          message: 'Fehler beim Exportieren',
        },
      });
      window.dispatchEvent(event);
    } finally {
      setLoading(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-2xl font-bold text-white mb-2">CSV-Export</h2>
        <p className="text-gray-400">
          Exportieren Sie verschiedene Datensätze als CSV-Dateien für Excel oder andere Anwendungen.
          Alle Exporte verwenden UTF-8 Encoding und deutsches CSV-Format (Semikolon-getrennt).
        </p>
      </div>

      {/* Export Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {exportTypes.map((exportType) => {
          const Icon = exportType.icon;
          const isLoading = loading === exportType.id;

          return (
            <button
              key={exportType.id}
              onClick={() => handleExport(exportType.id, exportType.label)}
              disabled={isLoading}
              className="glass-dark p-6 rounded-xl text-left hover:bg-white/10 transition-all group disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <div className="flex items-start gap-4">
                <div className="p-3 bg-accent-red/20 rounded-lg group-hover:bg-accent-red/30 transition-colors">
                  <Icon className="w-6 h-6 text-accent-red" />
                </div>
                <div className="flex-1">
                  <h3 className="text-lg font-semibold text-white mb-2 group-hover:text-accent-red transition-colors">
                    {exportType.label}
                  </h3>
                  <p className="text-sm text-gray-400 leading-relaxed">
                    {exportType.description}
                  </p>
                </div>
              </div>

              {/* Download Button */}
              <div className="mt-4 flex items-center justify-end">
                {isLoading ? (
                  <div className="flex items-center gap-2 text-accent-red">
                    <div className="w-4 h-4 border-2 border-accent-red border-t-transparent rounded-full animate-spin" />
                    <span className="text-sm font-medium">Exportiere...</span>
                  </div>
                ) : (
                  <div className="flex items-center gap-2 text-gray-400 group-hover:text-accent-red transition-colors">
                    <Download className="w-4 h-4" />
                    <span className="text-sm font-medium">CSV herunterladen</span>
                  </div>
                )}
              </div>
            </button>
          );
        })}
      </div>

      {/* Info Box */}
      <div className="glass-dark p-6 rounded-xl border border-white/10">
        <h3 className="text-lg font-semibold text-white mb-3 flex items-center gap-2">
          <FileText className="w-5 h-5 text-accent-red" />
          Wichtige Hinweise zum CSV-Export
        </h3>
        <ul className="space-y-2 text-gray-400 text-sm">
          <li className="flex items-start gap-2">
            <span className="text-accent-red mt-1">•</span>
            <span>
              <strong className="text-white">Encoding:</strong> Alle CSV-Dateien verwenden UTF-8 mit BOM für optimale
              Excel-Kompatibilität
            </span>
          </li>
          <li className="flex items-start gap-2">
            <span className="text-accent-red mt-1">•</span>
            <span>
              <strong className="text-white">Trennzeichen:</strong> Semikolon (;) als Spalten-Trennzeichen (deutsches
              CSV-Format)
            </span>
          </li>
          <li className="flex items-start gap-2">
            <span className="text-accent-red mt-1">•</span>
            <span>
              <strong className="text-white">Zahlenformat:</strong> Komma (,) als Dezimaltrennzeichen
            </span>
          </li>
          <li className="flex items-start gap-2">
            <span className="text-accent-red mt-1">•</span>
            <span>
              <strong className="text-white">Datumsformat:</strong> DD.MM.YYYY HH:MM (deutsches Format)
            </span>
          </li>
          <li className="flex items-start gap-2">
            <span className="text-accent-red mt-1">•</span>
            <span>
              <strong className="text-white">Excel-Import:</strong> Dateien können direkt in Excel geöffnet werden. Bei
              Problemen nutzen Sie "Daten → Aus Text/CSV"
            </span>
          </li>
        </ul>
      </div>
    </div>
  );
}
