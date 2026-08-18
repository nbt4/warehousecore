import { useEffect, useMemo, useRef, useState } from 'react';
import {
  BriefcaseBusiness,
  CheckCircle,
  MapPin,
  PackageCheck,
  PackageMinus,
  RotateCcw,
  ScanLine,
  Search,
  XCircle,
} from 'lucide-react';
import { jobsApi, scansApi, zonesApi } from '../lib/api';
import type { JobSummary, ScanResponse } from '../lib/api';
import { formatStatus } from '../lib/utils';
import { toast } from '../lib/toast';

type ScanAction = 'check' | 'intake' | 'outtake';
type ScanStep = 'job' | 'device' | 'zone';

const actionOptions: Array<{
  value: ScanAction;
  label: string;
  description: string;
  icon: typeof Search;
}> = [
  { value: 'check', label: 'Prüfen', description: 'Status und Standort anzeigen', icon: Search },
  { value: 'intake', label: 'Einlagern', description: 'Artikel und danach Lagerplatz scannen', icon: PackageCheck },
  { value: 'outtake', label: 'Auslagern', description: 'Job und danach Artikel scannen', icon: PackageMinus },
];

const closedJobStatuses = new Set([
  'abgeschlossen',
  'abgerechnet',
  'storniert',
  'completed',
  'paid',
  'canceled',
  'cancelled',
]);

const firstStep = (action: ScanAction): ScanStep => (action === 'outtake' ? 'job' : 'device');

function requestError(error: unknown, fallback: string): string {
  const candidate = error as { response?: { data?: { error?: string } }; message?: string };
  return candidate.response?.data?.error || candidate.message || fallback;
}

export function ScanPage() {
  const [action, setAction] = useState<ScanAction>('check');
  const [step, setStep] = useState<ScanStep>('device');
  const [scanCode, setScanCode] = useState('');
  const [quantity, setQuantity] = useState(1);
  const [result, setResult] = useState<ScanResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [selectedJob, setSelectedJob] = useState<JobSummary | null>(null);
  const [pendingItemCode, setPendingItemCode] = useState('');
  const [pendingItem, setPendingItem] = useState<ScanResponse | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, [action, step, loading]);

  const prompt = useMemo(() => {
    if (step === 'job') return 'Job-Code scannen';
    if (step === 'zone') return 'Lagerplatz scannen';
    if (action === 'outtake' && selectedJob) return `Artikel für ${selectedJob.job_code} scannen`;
    return 'Barcode, QR-Code oder Geräte-ID scannen';
  }, [action, selectedJob, step]);

  const resetWorkflow = (nextAction = action) => {
    setStep(firstStep(nextAction));
    setScanCode('');
    setQuantity(1);
    setResult(null);
    setSelectedJob(null);
    setPendingItemCode('');
    setPendingItem(null);
  };

  const changeAction = (nextAction: ScanAction) => {
    setAction(nextAction);
    resetWorkflow(nextAction);
  };

  const selectJob = async (code: string) => {
    const { data: job } = await jobsApi.getByScan(code);
    if (closedJobStatuses.has(job.status.trim().toLowerCase())) {
      setResult({
        success: false,
        message: `${job.job_code} ist bereits ${job.status} und kann nicht mehr ausgelagert werden.`,
        action: 'outtake',
        duplicate: false,
      });
      return;
    }
    setSelectedJob(job);
    setStep('device');
    setScanCode('');
    setResult({
      success: true,
      message: `${job.job_code} ausgewählt – jetzt Geräte oder Mengenartikel scannen.`,
      action: 'outtake',
      duplicate: false,
    });
  };

  const checkItemForIntake = async (code: string) => {
    const { data } = await scansApi.process({ scan_code: code, action: 'check' });
    setResult(data);
    if (!data.success) return;
    setPendingItemCode(code);
    setPendingItem(data);
    setStep('zone');
    setScanCode('');
  };

  const finishIntake = async (zoneCode: string) => {
    const { data: zone } = await zonesApi.getByScan(zoneCode);
    const { data } = await scansApi.process({
      scan_code: pendingItemCode,
      action: 'intake',
      zone_id: zone.zone_id,
      quantity,
    });
    setResult(data);
    if (!data.success) return;
    setStep('device');
    setScanCode('');
    setPendingItemCode('');
    setPendingItem(null);
    setQuantity(1);
  };

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    const code = scanCode.trim();
    if (!code || loading) return;
    setLoading(true);

    try {
      if (step === 'job') {
        await selectJob(code);
      } else if (action === 'intake' && step === 'device') {
        await checkItemForIntake(code);
      } else if (action === 'intake' && step === 'zone') {
        await finishIntake(code);
      } else if (action === 'outtake') {
        if (!selectedJob) {
          setStep('job');
          setResult({ success: false, message: 'Bitte zuerst einen Job scannen.', action, duplicate: false });
        } else {
          const { data } = await scansApi.process({
            scan_code: code,
            action: 'outtake',
            job_id: selectedJob.job_id,
            quantity,
          });
          setResult(data);
          setScanCode('');
          if (data.success) setQuantity(1);
        }
      } else {
        const { data } = await scansApi.process({ scan_code: code, action: 'check' });
        setResult(data);
        setScanCode('');
      }
    } catch (error) {
      const message = requestError(error, 'Scan fehlgeschlagen');
      toast.error(message);
      setResult({ success: false, message, action, duplicate: false });
    } finally {
      setLoading(false);
    }
  };

  const currentStepNumber = action === 'check' ? 1 : step === firstStep(action) ? 1 : 2;

  return (
    <div className="mx-auto w-full max-w-5xl space-y-4 p-3 sm:space-y-6 sm:p-6">
      <div>
        <h1 className="text-2xl font-bold text-white sm:text-3xl">Warehouse Scanner</h1>
        <p className="mt-1 text-sm text-gray-400">Geführte Ein- und Auslagerung mit eindeutiger Statusprüfung</p>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        {actionOptions.map((option) => {
          const Icon = option.icon;
          const active = option.value === action;
          return (
            <button
              key={option.value}
              type="button"
              onClick={() => changeAction(option.value)}
              className={`rounded-xl border p-4 text-left transition-colors ${
                active
                  ? 'border-accent-red bg-accent-red/15'
                  : 'border-white/10 bg-white/[0.04] hover:border-white/20 hover:bg-white/[0.07]'
              }`}
            >
              <div className="flex items-center gap-3">
                <Icon className={`h-5 w-5 ${active ? 'text-accent-red' : 'text-gray-400'}`} />
                <div>
                  <div className="font-semibold text-white">{option.label}</div>
                  <div className="mt-0.5 text-xs text-gray-400">{option.description}</div>
                </div>
              </div>
            </button>
          );
        })}
      </div>

      <div className="glass-dark rounded-2xl border-2 border-white/10 p-4 sm:p-6">
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="rounded-xl bg-accent-red/15 p-3 text-accent-red">
              {step === 'job' ? <BriefcaseBusiness className="h-6 w-6" /> : step === 'zone' ? <MapPin className="h-6 w-6" /> : <ScanLine className="h-6 w-6" />}
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                Schritt {currentStepNumber}{action === 'check' ? '' : ' von 2'}
              </div>
              <h2 className="text-xl font-bold text-white">{prompt}</h2>
            </div>
          </div>
          {(selectedJob || pendingItemCode) && (
            <button
              type="button"
              onClick={() => resetWorkflow()}
              className="flex items-center gap-2 rounded-lg border border-white/10 px-3 py-2 text-sm text-gray-300 hover:bg-white/10"
            >
              <RotateCcw className="h-4 w-4" /> Ablauf zurücksetzen
            </button>
          )}
        </div>

        {selectedJob && (
          <div className="mb-4 flex items-center justify-between rounded-xl border border-blue-500/30 bg-blue-500/10 p-3">
            <div>
              <div className="text-xs text-blue-300">Gewählter Job</div>
              <div className="font-semibold text-white">{selectedJob.job_code} · {selectedJob.status}</div>
            </div>
            <BriefcaseBusiness className="h-5 w-5 text-blue-300" />
          </div>
        )}

        {pendingItemCode && (
          <div className="mb-4 rounded-xl border border-green-500/30 bg-green-500/10 p-3">
            <div className="text-xs text-green-300">Artikel erkannt</div>
            <div className="font-semibold text-white">
              {pendingItem?.device?.product_name || pendingItem?.product?.name || pendingItemCode}
            </div>
            <div className="mt-1 text-xs text-gray-400">Jetzt den tatsächlichen Lagerplatz scannen.</div>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <input
            ref={inputRef}
            type="text"
            value={scanCode}
            onChange={(event) => setScanCode(event.target.value)}
            placeholder={step === 'job' ? 'JOB001234' : step === 'zone' ? 'Lagerplatz-Code' : 'Barcode / QR-Code / Geräte-ID'}
            autoComplete="off"
            className="w-full rounded-xl border-2 border-white/20 bg-white/10 px-4 py-4 text-lg text-white placeholder-gray-500 outline-none transition-colors focus:border-accent-red sm:px-6 sm:text-xl"
          />

          {action !== 'check' && step === 'device' && (
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-[180px_1fr] sm:items-center">
              <label htmlFor="scan-quantity" className="text-sm font-medium text-gray-300">Menge</label>
              <div>
                <input
                  id="scan-quantity"
                  type="number"
                  min="0.01"
                  step="0.01"
                  value={quantity}
                  onChange={(event) => setQuantity(Number(event.target.value))}
                  className="input-field w-full"
                />
                <p className="mt-1 text-xs text-gray-500">Für Einzelgeräte wird die Menge ignoriert.</p>
              </div>
            </div>
          )}

          <button
            type="submit"
            disabled={loading || !scanCode.trim() || quantity <= 0}
            className="w-full rounded-xl bg-gradient-to-r from-accent-red to-red-700 py-4 text-base font-bold text-white transition-all hover:shadow-lg hover:shadow-accent-red/40 disabled:cursor-not-allowed disabled:opacity-50 sm:text-lg"
          >
            {loading ? 'Verarbeite Scan…' : prompt}
          </button>
        </form>
      </div>

      {result && (
        <div className={`rounded-2xl border-2 p-4 sm:p-5 ${result.success ? 'border-green-500/40 bg-green-500/10' : 'border-red-500/40 bg-red-500/10'}`}>
          <div className="flex items-start gap-3">
            {result.success ? <CheckCircle className="mt-0.5 h-6 w-6 flex-shrink-0 text-green-400" /> : <XCircle className="mt-0.5 h-6 w-6 flex-shrink-0 text-red-400" />}
            <div className="min-w-0 flex-1">
              <p className={`font-semibold ${result.success ? 'text-green-300' : 'text-red-300'}`}>{result.message}</p>
              {result.device && (
                <div className="mt-3 grid grid-cols-1 gap-2 text-sm text-gray-300 sm:grid-cols-2">
                  <div><span className="text-gray-500">Gerät:</span> {result.device.device_id}</div>
                  <div><span className="text-gray-500">Produkt:</span> {result.device.product_name || '–'}</div>
                  <div>
                    <span className="text-gray-500">Status:</span>{' '}
                    {result.device.status_label || formatStatus(result.new_status || result.device.status)}
                  </div>
                  {result.device.status_detail && (
                    <div className="sm:col-span-2"><span className="text-gray-500">Hinweis:</span> {result.device.status_detail}</div>
                  )}
                </div>
              )}
              {result.product && (
                <div className="mt-3 text-sm text-gray-300">
                  <span className="text-gray-500">Bestand:</span> {result.product.stock} {result.product.unit}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
