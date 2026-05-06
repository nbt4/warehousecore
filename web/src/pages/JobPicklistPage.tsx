import { useState, useEffect, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, CheckCircle2, Clock, ScanLine, Package } from 'lucide-react';
import { picklistApi, jobsApi } from '../lib/api';
import type { JobPicklist, JobSummary } from '../lib/api';

export function JobPicklistPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const jobId = parseInt(id || '0', 10);

  const [picklist, setPicklist] = useState<JobPicklist | null>(null);
  const [job, setJob] = useState<JobSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [scanInput, setScanInput] = useState('');
  const [scanning, setScanning] = useState(false);
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; msg: string } | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const loadPicklist = useCallback(async () => {
    try {
      const res = await picklistApi.getByJob(jobId);
      setPicklist(res.data);
    } catch (e) {
      console.error(e);
    }
  }, [jobId]);

  useEffect(() => {
    Promise.all([
      jobsApi.getById(jobId),
      picklistApi.getByJob(jobId),
    ]).then(([jRes, pRes]) => {
      setJob(jRes.data);
      setPicklist(pRes.data);
    }).catch(console.error).finally(() => setLoading(false));

    const interval = setInterval(loadPicklist, 5000);
    return () => clearInterval(interval);
  }, [jobId, loadPicklist]);

  const handleScan = async (e: React.FormEvent) => {
    e.preventDefault();
    const deviceId = scanInput.trim();
    if (!deviceId) return;
    setScanning(true);
    setFeedback(null);
    try {
      await picklistApi.scanDevice(jobId, deviceId);
      setFeedback({ type: 'success', msg: `✓ ${deviceId} zugewiesen` });
      setScanInput('');
      loadPicklist();
    } catch (err: any) {
      const msg = err?.response?.data?.error || 'Kein passender Platz gefunden';
      setFeedback({ type: 'error', msg: `✗ ${msg}` });
    } finally {
      setScanning(false);
      inputRef.current?.focus();
      setTimeout(() => setFeedback(null), 3000);
    }
  };

  if (loading) return (
    <div className="flex justify-center py-20">
      <div className="w-8 h-8 border-4 border-accent-red/20 border-t-accent-red rounded-full animate-spin" />
    </div>
  );

  const total = picklist?.positions.length || 0;
  const done = picklist?.positions.filter(p => p.fulfilled).length || 0;

  return (
    <div className="p-4 max-w-2xl mx-auto space-y-4">
      {/* Header */}
      <div className="flex items-center gap-3">
        <button onClick={() => navigate(`/jobs/${jobId}`)} className="p-2 hover:bg-white/10 rounded-lg transition-colors">
          <ArrowLeft className="w-5 h-5" />
        </button>
        <div className="flex-1">
          <h1 className="text-xl font-bold text-white">{job?.job_code} — Picklist</h1>
          <p className="text-sm text-gray-400">{done}/{total} Positionen abgeschlossen</p>
        </div>
        {done === total && total > 0 && (
          <span className="flex items-center gap-1.5 text-green-400 text-sm font-medium">
            <CheckCircle2 className="w-4 h-4" /> Vollständig
          </span>
        )}
      </div>

      {/* Progress bar */}
      {total > 0 && (
        <div className="w-full bg-white/5 rounded-full h-2">
          <div
            className="bg-accent-red h-2 rounded-full transition-all duration-500"
            style={{ width: `${(done / total) * 100}%` }}
          />
        </div>
      )}

      {/* Scan input */}
      <form onSubmit={handleScan}>
        <div className="relative">
          <ScanLine className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-500" />
          <input
            ref={inputRef}
            type="text"
            value={scanInput}
            onChange={e => setScanInput(e.target.value)}
            placeholder="Device-ID scannen oder eingeben..."
            disabled={scanning}
            autoFocus
            className="w-full pl-10 pr-4 py-3 bg-white/5 border border-white/10 rounded-xl text-white placeholder-gray-500 focus:outline-none focus:border-accent-red/50 focus:bg-white/8 transition-all"
          />
        </div>
        {feedback && (
          <p className={`mt-2 text-sm font-medium ${feedback.type === 'success' ? 'text-green-400' : 'text-red-400'}`}>
            {feedback.msg}
          </p>
        )}
      </form>

      {/* Positions */}
      {!picklist?.positions.length && (
        <div className="glass-dark rounded-xl border border-white/10 p-6 text-center text-gray-500">
          Keine Produktpositionen für diesen Job.
        </div>
      )}

      <div className="space-y-3">
        {picklist?.positions.map(pos => (
          <div
            key={pos.position_id}
            className={`glass-dark rounded-xl border p-4 transition-all ${
              pos.fulfilled ? 'border-green-500/20' : 'border-white/10'
            }`}
          >
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2">
                <Package className={`w-4 h-4 ${pos.fulfilled ? 'text-green-400' : 'text-accent-red'}`} />
                <span className="font-medium text-white">{pos.product_name}</span>
              </div>
              <div className="flex items-center gap-2">
                {pos.fulfilled ? (
                  <span className="flex items-center gap-1 text-xs text-green-400 font-medium bg-green-500/10 border border-green-500/20 px-2 py-0.5 rounded-full">
                    <CheckCircle2 className="w-3 h-3" /> {pos.scanned}/{pos.needed}
                  </span>
                ) : (
                  <span className="flex items-center gap-1 text-xs text-yellow-400 font-medium bg-yellow-500/10 border border-yellow-500/20 px-2 py-0.5 rounded-full">
                    <Clock className="w-3 h-3" /> {pos.scanned}/{pos.needed}
                  </span>
                )}
              </div>
            </div>

            {/* Progress mini bar */}
            <div className="w-full bg-white/5 rounded-full h-1 mb-2">
              <div
                className={`h-1 rounded-full transition-all ${pos.fulfilled ? 'bg-green-500' : 'bg-accent-red'}`}
                style={{ width: `${Math.min(100, (pos.scanned / Math.max(1, pos.needed)) * 100)}%` }}
              />
            </div>

            {/* Scanned devices */}
            {pos.devices?.length > 0 && (
              <div className="mt-2 space-y-1">
                {pos.devices.map(d => (
                  <div key={d.device_id} className="flex items-center gap-2 text-xs text-gray-400">
                    <CheckCircle2 className="w-3 h-3 text-green-500" />
                    <span className="font-mono text-green-400">{d.device_id}</span>
                    <span className="text-gray-600">{new Date(d.scanned_at).toLocaleString('de-DE')}</span>
                    {d.scanned_by && <span className="text-gray-600">— {d.scanned_by}</span>}
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
