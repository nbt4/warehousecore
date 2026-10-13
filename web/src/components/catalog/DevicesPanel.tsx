import { useCatalogWriteAccess, useDebouncedValue } from "./useCatalog";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Plus, Search } from "lucide-react";
import {
  api,
  devicesAdminApi,
  devicesApi,
  type Device,
  type DeviceStatusHistory,
} from "../../lib/api";
import { catalogError, type CatalogProduct } from "../../lib/catalog";
import { appAssetPath } from "../../lib/app-paths";
import { formatStatus } from "../../lib/utils";
import {
  WorkspaceDialog,
  WorkspaceField,
  WorkspaceLoading,
  WorkspaceNotice,
} from "./Workspace";

export function DevicesPanel({ product }: { product?: CatalogProduct }) {
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [params, setParams] = useSearchParams();
  const [devices, setDevices] = useState<Device[]>([]);
  const [search, setSearch] = useState(params.get("search") ?? "");
  const [lifecycle, setLifecycle] = useState(
    params.get("lifecycle_status") ?? "active",
  );
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [page,setPage]=useState(1);const [total,setTotal]=useState(0);
  const [creating, setCreating] = useState(false);
  const [quantity, setQuantity] = useState(1);
  const [saving, setSaving] = useState(false);
  const debounced = useDebouncedValue(search);
  const selected = params.get("device");
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await api.get<Device[]>("/admin/devices-list", {
        params: {
          lifecycle_status: lifecycle,
          product_id: product?.product_id,
          search: debounced,
          limit: 100, offset:(page-1)*100,
        },
      });
      setDevices(result.data ?? []);setTotal(Number(result.headers['x-total-count'])||result.data?.length||0);
    } catch (err) {
      setError(catalogError(err));
    } finally {
      setLoading(false);
    }
  }, [product?.product_id, lifecycle, debounced, page]);
  useEffect(() => {
    void load();
  }, [load]);
  const choose = (id: string | null) => {
    const next = new URLSearchParams(params);
    if (id) next.set("device", id);
    else next.delete("device");
    setParams(next);
  };
  const create = async (event: FormEvent) => {
    event.preventDefault();
    if (!product) return;
    setSaving(true);
    setError("");
    try {
      await devicesAdminApi.create({
        product_id: product.product_id,
        quantity,
        auto_generate_label: true,
      });
      setCreating(false);
      setQuantity(1);
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="suite-search-field flex-1">
          <Search size={18} />
          <input
            value={search}
            placeholder={t("catalog.deviceSearch")}
            onChange={(event) => {setSearch(event.target.value);setPage(1);}}
          />
        </div>
        <select
          aria-label={t("catalog.lifecycle")}
          value={lifecycle}
          onChange={(event) => {setLifecycle(event.target.value);setPage(1);}}
        >
          {["active", "archived", "all"].map((value) => (
            <option key={value} value={value}>
              {t(`catalog.${value}`)}
            </option>
          ))}
        </select>
        {writable && product?.lifecycle_status === "active" && (
          <button
            className="suite-button suite-button--primary"
            onClick={() => setCreating(true)}
          >
            <Plus size={16} />
            {t("catalog.addDevices")}
          </button>
        )}
      </div>
      {error && (
        <WorkspaceNotice error retry={() => void load()}>
          {error}
        </WorkspaceNotice>
      )}
      {loading ? (
        <WorkspaceLoading />
      ) : (
        <div className="suite-table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("catalog.device")}</th>
                <th>{t("catalog.product")}</th>
                <th>{t("catalog.status")}</th>
                <th>{t("catalog.location")}</th>
                <th>{t("catalog.serialNumber")}</th>
                <th>{t("catalog.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {devices.map((device) => (
                <tr key={device.device_id}>
                  <td>
                    <button
                      className="font-mono underline text-left"
                      onClick={() => choose(device.device_id)}
                    >
                      {device.device_id}
                    </button>
                  </td>
                  <td>
                    {device.product_id ? (
                      <Link to={`/products/${device.product_id}/stock`}>
                        {device.product_name}
                      </Link>
                    ) : (
                      device.product_name
                    )}
                  </td>
                  <td>
                    {formatStatus(device.status)}
                    <div
                      className="text-xs"
                      style={{ color: "var(--text-secondary)" }}
                    >
                      {t(
                        `catalog.condition.${device.condition_status ?? "available"}`,
                      )}
                    </div>
                  </td>
                  <td>
                    {device.zone_id ? (
                      <Link to={`/zones/${device.zone_id}`}>
                        {device.zone_name || device.zone_code}
                      </Link>
                    ) : (
                      device.case_name || "—"
                    )}
                    {device.job_number && (
                      <div className="text-xs">{device.job_number}</div>
                    )}
                  </td>
                  <td>{device.serial_number || "—"}</td>
                  <td>
                    <button
                      className="suite-button"
                      onClick={() => choose(device.device_id)}
                    >
                      {t(writable ? "catalog.edit" : "catalog.details")}
                    </button>
                  </td>
                </tr>
              ))}
              {!devices.length && (
                <tr>
                  <td colSpan={6}>{t("catalog.noneFound")}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
      <div className="flex flex-wrap justify-between items-center gap-3"><span>{t('catalog.results',{count:total})}</span><div className="flex gap-2"><button className="suite-button" disabled={page<=1} onClick={()=>setPage(page-1)}>{t('catalog.previous')}</button><span>{t('catalog.page',{page,total:Math.max(1,Math.ceil(total/100))})}</span><button className="suite-button" disabled={page*100>=total} onClick={()=>setPage(page+1)}>{t('catalog.next')}</button></div></div>
      {!product && (
        <WorkspaceNotice>{t("catalog.addDevicesHelp")}</WorkspaceNotice>
      )}
      {selected && (
        <DeviceEditor
          key={selected}
          id={selected}
          onClose={() => choose(null)}
          onSaved={() => {
            void load();
          }}
        />
      )}
      {creating && product && (
        <WorkspaceDialog
          title={t("catalog.addDevices")}
          onClose={() => setCreating(false)}
        >
          <form className="space-y-4" onSubmit={(event) => void create(event)}>
            <p>{product.name}</p>
            <WorkspaceField
              label={t("catalog.numberOfUnits")}
              hint={t("catalog.generatedDeviceIds")}
            >
              <input
                type="number"
                min="1"
                max="1000"
                step="1"
                required
                value={quantity}
                onChange={(event) => setQuantity(Number(event.target.value))}
              />
            </WorkspaceField>
            <button
              className="suite-button suite-button--primary"
              disabled={saving}
            >
              {t(saving ? "catalog.saving" : "catalog.addDevices")}
            </button>
          </form>
        </WorkspaceDialog>
      )}
    </div>
  );
}

function DeviceEditor({
  id,
  onClose,
  onSaved,
}: {
  id: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [device, setDevice] = useState<Device | null>(null);
  const [original, setOriginal] = useState<Device | null>(null);
  const [history, setHistory] = useState<DeviceStatusHistory[]>([]);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const load = useCallback(async () => {
    try {
      const [detail, events] = await Promise.all([
        devicesAdminApi.getById(id),
        devicesApi.getStatusHistory(id),
      ]);
      setDevice(detail.data);
      setOriginal(detail.data);
      setHistory(events.data ?? []);
    } catch (err) {
      setError(catalogError(err));
    }
  }, [id]);
  useEffect(() => {
    void load();
  }, [load]);
  const close = () => {
    if (
      JSON.stringify(device) !== JSON.stringify(original) &&
      !window.confirm(t("catalog.confirmDiscard"))
    )
      return;
    onClose();
  };
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!device || !writable) return;
    setSaving(true);
    setError("");
    try {
      const result = await devicesAdminApi.update(id, {
        expected_updated_at: original?.version,
        serial_number: device.serial_number ?? "",
        notes: device.notes ?? "",
        condition_status: device.condition_status,
        condition_rating: device.condition_rating,
        usage_hours: device.usage_hours,
        purchase_date: device.purchase_date?.slice(0, 10) ?? "",
        last_maintenance: device.last_maintenance?.slice(0, 10) ?? "",
        next_maintenance: device.next_maintenance?.slice(0, 10) ?? "",
      });
      setDevice(result.data);
      setOriginal(result.data);
      onSaved();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  const lifecycle = async () => {
    if (
      !device ||
      !window.confirm(
        t(
          device.lifecycle_status === "archived"
            ? "catalog.confirmRestore"
            : "catalog.confirmDeviceArchive",
        ),
      )
    )
      return;
    setSaving(true);
    setError("");
    try {
      if (device.lifecycle_status === "archived")
        await devicesAdminApi.restore(id);
      else await devicesAdminApi.archive(id);
      onSaved();
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  return (
    <WorkspaceDialog title={`${t("catalog.device")} ${id}`} onClose={close}>
      {error && (
        <WorkspaceNotice error retry={() => void load()}>
          {error}
        </WorkspaceNotice>
      )}
      {!device ? (
        <WorkspaceLoading />
      ) : (
        <form className="space-y-5" onSubmit={(event) => void save(event)}>
          <p className="text-lg font-semibold">{device.product_name}</p>
          <p style={{ color: "var(--text-secondary)" }}>
            {formatStatus(device.status)} ·{" "}
            {device.zone_name || device.case_name || t("catalog.noLocation")}
          </p>
          <fieldset
            disabled={
              !writable || saving || device.lifecycle_status === "archived"
            }
            className="grid gap-4 sm:grid-cols-2"
          >
            <WorkspaceField label={t("catalog.serialNumber")}>
              <input
                value={device.serial_number ?? ""}
                onChange={(event) =>
                  setDevice({ ...device, serial_number: event.target.value })
                }
              />
            </WorkspaceField>
            <WorkspaceField label={t("catalog.operationalCondition")}>
              <select
                value={device.condition_status ?? "available"}
                onChange={(event) =>
                  setDevice({ ...device, condition_status: event.target.value })
                }
              >
                {[
                  "available",
                  "blocked",
                  "defective",
                  "maintenance",
                  "retired",
                ].map((value) => (
                  <option value={value} key={value}>
                    {t(`catalog.condition.${value}`)}
                  </option>
                ))}
              </select>
            </WorkspaceField>
            <WorkspaceField label={t("catalog.conditionRating")}>
              <input
                type="number"
                min="0"
                max="5"
                step="0.1"
                value={device.condition_rating ?? ""}
                onChange={(event) =>
                  setDevice({
                    ...device,
                    condition_rating: Number(event.target.value),
                  })
                }
              />
            </WorkspaceField>
            <WorkspaceField label={t("catalog.usageHours")}>
              <input
                type="number"
                min="0"
                step="0.01"
                value={device.usage_hours ?? ""}
                onChange={(event) =>
                  setDevice({
                    ...device,
                    usage_hours: Number(event.target.value),
                  })
                }
              />
            </WorkspaceField>
            {(
              ["purchase_date", "last_maintenance", "next_maintenance"] as const
            ).map((key) => (
              <WorkspaceField key={key} label={t(`catalog.${key}`)}>
                <input
                  type="date"
                  value={device[key]?.slice(0, 10) ?? ""}
                  onChange={(event) =>
                    setDevice({ ...device, [key]: event.target.value })
                  }
                />
              </WorkspaceField>
            ))}
            <WorkspaceField label={t("catalog.notes")}>
              <textarea
                rows={3}
                value={device.notes ?? ""}
                onChange={(event) =>
                  setDevice({ ...device, notes: event.target.value })
                }
              />
            </WorkspaceField>
          </fieldset>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex gap-2">
              {device.zone_id && (
                <Link className="suite-button" to={`/zones/${device.zone_id}`}>
                  {t("catalog.openLocation")}
                </Link>
              )}
              {device.label_path && (
                <a
                  className="suite-button"
                  href={appAssetPath(device.label_path)}
                  target="_blank"
                  rel="noreferrer"
                >
                  {t("catalog.label")}
                </a>
              )}
            </div>
            {writable && (
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  className="suite-button"
                  disabled={saving}
                  onClick={() => void lifecycle()}
                >
                  {t(
                    device.lifecycle_status === "archived"
                      ? "catalog.restore"
                      : "catalog.archive",
                  )}
                </button>
                {device.lifecycle_status !== "archived" && (
                  <button
                    className="suite-button suite-button--primary"
                    disabled={saving}
                  >
                    {t(saving ? "catalog.saving" : "catalog.save")}
                  </button>
                )}
              </div>
            )}
          </div>
          <details>
            <summary className="cursor-pointer font-semibold">
              {t("catalog.history")} ({history.length})
            </summary>
            <div className="suite-table-wrap mt-3">
              <table>
                <thead>
                  <tr>
                    <th>{t("catalog.date")}</th>
                    <th>{t("catalog.status")}</th>
                    <th>{t("catalog.source")}</th>
                  </tr>
                </thead>
                <tbody>
                  {history.map((item) => (
                    <tr key={item.history_id}>
                      <td>{new Date(item.changed_at).toLocaleString()}</td>
                      <td>
                        {formatStatus(item.new_status)} · {item.new_condition}
                      </td>
                      <td>{item.change_source}</td>
                    </tr>
                  ))}
                  {!history.length && (
                    <tr>
                      <td colSpan={3}>{t("catalog.noHistory")}</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </details>
        </form>
      )}
    </WorkspaceDialog>
  );
}
