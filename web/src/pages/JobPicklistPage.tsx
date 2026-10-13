import {
  useState,
  useEffect,
  useRef,
  useCallback,
  type FormEvent,
} from "react";
import { Link, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ScanLine, CheckCircle2 } from "lucide-react";
import {
  picklistApi,
  jobsApi,
  type JobPicklist,
  type JobSummary,
  type PicklistPosition,
} from "../lib/api";
import { catalogError } from "../lib/catalog";
import {
  WorkspaceHeader,
  WorkspaceField,
  WorkspaceLoading,
  WorkspaceNotice,
  WorkspaceDialog,
} from "../components/catalog/Workspace";
import { useAuth } from "../contexts/useAuth";
import "../components/catalog/catalog-workspace.css";

export function JobPicklistPage() {
  const { id } = useParams();
  const jobId = Number(id);
  const { t } = useTranslation();
  const writable = Boolean(useAuth().user);
  const [picklist, setPicklist] = useState<JobPicklist | null>(null);
  const [job, setJob] = useState<JobSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [scan, setScan] = useState("");
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [quantityItem, setQuantityItem] = useState<PicklistPosition | null>(
    null,
  );
  const [quantity, setQuantity] = useState(1);
  const receipt = useRef("");
  const input = useRef<HTMLInputElement>(null);
  const load = useCallback(async () => {
    try {
      const result = await picklistApi.getByJob(jobId);
      setPicklist(result.data);
      setError("");
    } catch (err) {
      setError(catalogError(err, t("catalog.loadFailed")));
    }
  }, [jobId, t]);
  useEffect(() => {
    let current = true;
    setLoading(true);
    void Promise.all([jobsApi.getById(jobId), picklistApi.getByJob(jobId)])
      .then(([j, p]) => {
        if (current) {
          setJob(j.data);
          setPicklist(p.data);
        }
      })
      .catch((err) => {
        if (current) setError(catalogError(err, t("catalog.loadFailed")));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    const timer = window.setInterval(() => void load(), 5000);
    return () => {
      current = false;
      window.clearInterval(timer);
    };
  }, [jobId, load, t]);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!scan.trim() || !writable) return;
    setSaving(true);
    setError("");
    try {
      await picklistApi.scanDevice(jobId, scan.trim());
      setMessage(t("catalog.materialAssigned"));
      setScan("");
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
      input.current?.focus();
    }
  };
  const confirm = async (event: FormEvent) => {
    event.preventDefault();
    if (!quantityItem?.product_id) return;
    setSaving(true);
    setError("");
    try {
      await picklistApi.confirmQuantity(
        jobId,
        quantityItem.product_id,
        quantity,
        receipt.current,
      );
      setQuantityItem(null);
      setMessage(t("catalog.materialAssigned"));
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  const done = picklist?.positions.filter((item) => item.fulfilled).length ?? 0;
  const total = picklist?.positions.length ?? 0;
  return (
    <div
      className="suite-page catalog-workspace space-y-5"
      style={{ color: "var(--text-primary)" }}
    >
      <Link className="suite-button" to={`/jobs/${jobId}`}>
        ← {t("catalog.backToJob")}
      </Link>
      <WorkspaceHeader
        title={`${job?.job_code ?? ""} · ${t("catalog.packingList")}`}
        description={t("catalog.packingProgress", {
          done,
          total,
          version: picklist?.version ?? 1,
        })}
      />
      {error && (
        <WorkspaceNotice error retry={() => void load()}>
          {error}
        </WorkspaceNotice>
      )}
      {message && <WorkspaceNotice>{message}</WorkspaceNotice>}
      {picklist?.warnings?.map((warning, index) => (
        <WorkspaceNotice key={index} error>
          {warning}
        </WorkspaceNotice>
      ))}
      {writable && (
        <form
          className="flex flex-wrap gap-3"
          onSubmit={(event) => void submit(event)}
        >
          <div className="suite-search-field flex-1">
            <ScanLine size={18} />
            <input
              ref={input}
              autoFocus
              value={scan}
              disabled={saving}
              placeholder={t("catalog.scanDevice")}
              onChange={(event) => setScan(event.target.value)}
            />
          </div>
          <button
            className="suite-button suite-button--primary"
            disabled={saving || !scan.trim()}
          >
            {t("catalog.assign")}
          </button>
        </form>
      )}
      {loading ? (
        <WorkspaceLoading />
      ) : (
        <div className="suite-table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("catalog.product")}</th>
                <th>{t("catalog.source")}</th>
                <th>{t("catalog.quantityLabel")}</th>
                <th>{t("catalog.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {picklist?.positions.map((item) => (
                <tr key={item.product_id ?? item.position_id}>
                  <td>
                    <Link
                      className="underline font-semibold"
                      to={`/products/${item.product_id}/overview`}
                    >
                      {item.product_name}
                    </Link>
                    {item.devices.length > 0 && (
                      <details>
                        <summary>
                          {t("catalog.devices")} ({item.devices.length})
                        </summary>
                        {item.devices.map((device) => (
                          <div
                            key={device.device_id}
                            className="font-mono text-xs"
                          >
                            {device.device_id} ·{" "}
                            {device.scanned_at ? new Date(device.scanned_at).toLocaleString() : "—"}
                          </div>
                        ))}
                      </details>
                    )}
                  </td>
                  <td>
                    {item.origins?.map((origin, index) => (
                      <div key={index}>
                        {t(`catalog.relation.${origin.relation_type}`)} ·{" "}
                        {origin.name}
                      </div>
                    ))}
                  </td>
                  <td>
                    {item.scanned.toLocaleString()} /{" "}
                    {item.needed.toLocaleString()} {item.unit}
                    {item.fulfilled && (
                      <span
                        className="flex gap-2 items-center"
                        style={{ color: "var(--color-success)" }}
                      >
                        <CheckCircle2 size={16} />
                        {t("catalog.fulfilled")}
                      </span>
                    )}
                  </td>
                  <td>
                    {writable &&
                      item.tracking_mode === "quantity" &&
                      !item.fulfilled && (
                        <button
                          className="suite-button"
                          onClick={() => {
                            setQuantityItem(item);
                            setQuantity(
                              Math.round((item.needed - item.scanned) * 1000) /
                                1000,
                            );
                            receipt.current = crypto.randomUUID();
                          }}
                        >
                          {t("catalog.confirmQuantity")}
                        </button>
                      )}
                  </td>
                </tr>
              ))}
              {!total && (
                <tr>
                  <td colSpan={4}>{t("catalog.noMaterial")}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
      {quantityItem && (
        <WorkspaceDialog
          title={t("catalog.confirmQuantity")}
          onClose={() => {
            if (!saving) setQuantityItem(null);
          }}
        >
          <form className="space-y-4" onSubmit={(event) => void confirm(event)}>
            <p>{quantityItem.product_name}</p>
            <WorkspaceNotice>
              {t("catalog.quantityConfirmationHelp")}
            </WorkspaceNotice>
            <WorkspaceField
              label={`${t("catalog.quantityLabel")} (${quantityItem.unit})`}
            >
              <input
                required
                type="number"
                min="0.001"
                step="0.001"
                max={
                  Math.round(
                    (quantityItem.needed - quantityItem.scanned) * 1000,
                  ) / 1000
                }
                value={quantity}
                disabled={saving}
                onChange={(event) => {
                  setQuantity(Number(event.target.value));
                  receipt.current = crypto.randomUUID();
                }}
              />
            </WorkspaceField>
            {error && <WorkspaceNotice error>{error}</WorkspaceNotice>}
            <button
              className="suite-button suite-button--primary"
              disabled={saving}
            >
              {t(saving ? "catalog.saving" : "catalog.confirmQuantity")}
            </button>
          </form>
        </WorkspaceDialog>
      )}
    </div>
  );
}
