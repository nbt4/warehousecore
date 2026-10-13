import {
  useCatalogWriteAccess,
  useDebouncedValue,
} from "../catalog/useCatalog";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Search } from "lucide-react";
import {
  cablesAdminApi,
  zonesApi,
  type Cable,
  type CableConnector,
  type CableType,
  type CableCreateInput,
  type Zone,
} from "../../lib/api";
import { catalogError } from "../../lib/catalog";
import {
  WorkspaceDialog,
  WorkspaceField,
  WorkspaceLoading,
  WorkspaceNotice,
} from "../catalog/Workspace";
interface CableForm {
  name: string;
  connector1: string;
  connector2: string;
  typ: string;
  length: string;
  mm2: string;
  tracking_mode: "quantity" | "individual";
  generic_barcode: string;
  quantity: string;
  zone_id: string;
}
const empty: CableForm = {
  name: "",
  connector1: "",
  connector2: "",
  typ: "",
  length: "1",
  mm2: "",
  tracking_mode: "quantity",
  generic_barcode: "",
  quantity: "0",
  zone_id: "",
};
export function CablesTab() {
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [params, setParams] = useSearchParams();
  const [cables, setCables] = useState<Cable[]>([]);
  const [connectors, setConnectors] = useState<CableConnector[]>([]);
  const [types, setTypes] = useState<CableType[]>([]);
  const [zones, setZones] = useState<Zone[]>([]);
  const [search, setSearch] = useState(params.get("search") ?? "");
  const query = useDebouncedValue(search);
  const [type, setType] = useState("");
  const [connectorA, setConnectorA] = useState("");
  const [connectorB, setConnectorB] = useState("");
  const [minLength, setMinLength] = useState("");
  const [maxLength, setMaxLength] = useState("");
  const [tracking, setTracking] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [form, setForm] = useState<CableForm | null>(
    params.get("create") === "true" ? { ...empty } : null,
  );
  const [editing, setEditing] = useState<Cable | null>(null);
  const [saving, setSaving] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const { data } = await cablesAdminApi.getAll({
        search: query || undefined,
        type: Number(type) || undefined,
        connector1: Number(connectorA) || undefined,
        connector2: Number(connectorB) || undefined,
        length_min: minLength === "" ? undefined : Number(minLength),
        length_max: maxLength === "" ? undefined : Number(maxLength),
        tracking_mode:
          tracking === "quantity" || tracking === "individual"
            ? tracking
            : undefined,
      });
      setCables(data);
    } catch (err) {
      setError(catalogError(err, t("catalog.loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [query, type, connectorA, connectorB, minLength, maxLength, tracking, t]);
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    let current = true;
    void Promise.all([
      cablesAdminApi.getConnectors(),
      cablesAdminApi.getTypes(),
      zonesApi.getAll(),
    ])
      .then(([a, b, c]) => {
        if (current) {
          setConnectors(a.data);
          setTypes(b.data);
          setZones(c.data.filter((zone) => zone.is_active));
        }
      })
      .catch((err) => {
        if (current) setError(catalogError(err, t("catalog.loadFailed")));
      });
    return () => {
      current = false;
    };
  }, [t]);
  const edit = (cable: Cable) => {
    setEditing(cable);
    setForm({
      ...empty,
      name: cable.name,
      connector1: String(cable.connector1),
      connector2: String(cable.connector2),
      typ: String(cable.typ),
      length: String(cable.length),
      mm2: cable.mm2 == null ? "" : String(cable.mm2),
      tracking_mode: cable.tracking_mode,
      generic_barcode: cable.generic_barcode ?? "",
    });
  };
  const close = () => {
    setForm(null);
    setEditing(null);
    const next = new URLSearchParams(params);
    next.delete("create");
    setParams(next, { replace: true });
  };
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!form || !writable) return;
    setSaving(true);
    setError("");
    const payload: CableCreateInput = {
      name: form.name.trim() || undefined,
      connector1: Number(form.connector1),
      connector2: Number(form.connector2),
      typ: Number(form.typ),
      length: Number(form.length),
      mm2: form.mm2 === "" ? undefined : Number(form.mm2),
      tracking_mode: form.tracking_mode,
      generic_barcode: form.generic_barcode.trim() || undefined,
      quantity: Number(form.quantity),
      zone_id: Number(form.zone_id) || null,
    };
    try {
      if (editing)
        await cablesAdminApi.update(editing.cable_id, {
          ...payload,
          mm2: form.mm2 === "" ? null : Number(form.mm2),
        });
      else await cablesAdminApi.create(payload);
      close();
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center gap-3">
        <div className="suite-search-field flex-1">
          <Search size={18} />
          <input
            type="search"
            placeholder={t("catalog.cableSearch")}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        <select
          value={type}
          aria-label={t("catalog.cableType")}
          onChange={(event) => setType(event.target.value)}
        >
          <option value="">{t("catalog.allCableTypes")}</option>
          {types.map((item) => (
            <option key={item.cable_type_id} value={item.cable_type_id}>
              {item.name}
            </option>
          ))}
        </select>
        {writable && (
          <button
            className="suite-button suite-button--primary"
            onClick={() => {
              setEditing(null);
              setForm({ ...empty });
            }}
          >
            {t("catalog.newCable")}
          </button>
        )}
      </div>
      <details className="suite-card p-4">
        <summary className="cursor-pointer font-semibold">
          {t("catalog.moreFilters")}
        </summary>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 mt-4">
          <WorkspaceField label={t("catalog.connectorA")}>
            <select
              value={connectorA}
              onChange={(event) => setConnectorA(event.target.value)}
            >
              <option value="">{t("catalog.all")}</option>
              {connectors.map((item) => (
                <option key={item.connector_id} value={item.connector_id}>
                  {item.name} {item.gender}
                </option>
              ))}
            </select>
          </WorkspaceField>
          <WorkspaceField label={t("catalog.connectorB")}>
            <select
              value={connectorB}
              onChange={(event) => setConnectorB(event.target.value)}
            >
              <option value="">{t("catalog.all")}</option>
              {connectors.map((item) => (
                <option key={item.connector_id} value={item.connector_id}>
                  {item.name} {item.gender}
                </option>
              ))}
            </select>
          </WorkspaceField>
          <WorkspaceField label={t("catalog.tracking")}>
            <select
              value={tracking}
              onChange={(event) => setTracking(event.target.value)}
            >
              <option value="">{t("catalog.all")}</option>
              <option value="individual">{t("catalog.individual")}</option>
              <option value="quantity">{t("catalog.quantity")}</option>
            </select>
          </WorkspaceField>
          <WorkspaceField label={t("catalog.minLength")}>
            <input
              type="number"
              min="0"
              step="0.1"
              value={minLength}
              onChange={(event) => setMinLength(event.target.value)}
            />
          </WorkspaceField>
          <WorkspaceField label={t("catalog.maxLength")}>
            <input
              type="number"
              min="0"
              step="0.1"
              value={maxLength}
              onChange={(event) => setMaxLength(event.target.value)}
            />
          </WorkspaceField>
        </div>
      </details>
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
                <th>{t("catalog.product")}</th>
                <th>{t("catalog.specification")}</th>
                <th>{t("catalog.stock")}</th>
                <th>{t("catalog.tracking")}</th>
                <th>{t("catalog.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {cables.map((cable) => (
                <tr key={cable.cable_id}>
                  <td>
                    <Link
                      className="font-semibold underline"
                      to={`/products/${cable.product_id}/overview`}
                    >
                      {cable.name}
                    </Link>
                    <div
                      className="font-mono text-xs"
                      style={{ color: "var(--text-secondary)" }}
                    >
                      {cable.generic_barcode}
                    </div>
                  </td>
                  <td>
                    {cable.cable_type_name} · {cable.length.toLocaleString()} m
                    <div
                      className="text-xs"
                      style={{ color: "var(--text-secondary)" }}
                    >
                      {cable.connector1_name} → {cable.connector2_name}
                      {cable.mm2 ? ` · ${cable.mm2} mm²` : ""}
                    </div>
                  </td>
                  <td>
                    <Link to={`/products/${cable.product_id}/stock`}>
                      {cable.stock_quantity.toLocaleString()}
                    </Link>
                    <div
                      className="text-xs"
                      style={{ color: "var(--text-secondary)" }}
                    >
                      {t("catalog.availableQuantity", {
                        count: cable.available_quantity,
                      })}
                    </div>
                  </td>
                  <td>
                    {t(
                      cable.tracking_mode === "quantity"
                        ? "catalog.quantity"
                        : "catalog.individual",
                    )}
                  </td>
                  <td>
                    <div className="flex flex-wrap gap-2">
                      <Link
                        className="suite-button"
                        to={`/products/${cable.product_id}/overview`}
                      >
                        {t("catalog.details")}
                      </Link>
                      {writable && (
                        <button
                          className="suite-button"
                          onClick={() => edit(cable)}
                        >
                          {t("catalog.editCableSpec")}
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
              {!cables.length && (
                <tr>
                  <td colSpan={5}>{t("catalog.noneFound")}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
      {form && writable && (
        <WorkspaceDialog
          title={t(editing ? "catalog.editCableSpec" : "catalog.newCable")}
          onClose={close}
        >
          <form className="space-y-4" onSubmit={(event) => void save(event)}>
            <WorkspaceField
              label={t("catalog.name")}
              hint={t("catalog.cableNameHelp")}
            >
              <input
                value={form.name}
                onChange={(event) =>
                  setForm({ ...form, name: event.target.value })
                }
              />
            </WorkspaceField>
            <div className="grid gap-4 sm:grid-cols-2">
              {(["connector1", "connector2"] as const).map((key) => (
                <WorkspaceField
                  key={key}
                  label={t(
                    key === "connector1"
                      ? "catalog.connectorA"
                      : "catalog.connectorB",
                  )}
                >
                  <select
                    required
                    value={form[key]}
                    onChange={(event) =>
                      setForm({ ...form, [key]: event.target.value })
                    }
                  >
                    <option value="">{t("catalog.choose")}</option>
                    {connectors.map((item) => (
                      <option key={item.connector_id} value={item.connector_id}>
                        {item.name} {item.gender}
                      </option>
                    ))}
                  </select>
                </WorkspaceField>
              ))}
              <WorkspaceField label={t("catalog.cableType")}>
                <select
                  required
                  value={form.typ}
                  onChange={(event) =>
                    setForm({ ...form, typ: event.target.value })
                  }
                >
                  <option value="">{t("catalog.choose")}</option>
                  {types.map((item) => (
                    <option key={item.cable_type_id} value={item.cable_type_id}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </WorkspaceField>
              <WorkspaceField label={t("catalog.length")}>
                <input
                  type="number"
                  required
                  min="0.01"
                  step="0.01"
                  value={form.length}
                  onChange={(event) =>
                    setForm({ ...form, length: event.target.value })
                  }
                />
              </WorkspaceField>
              <WorkspaceField label={t("catalog.crossSection")}>
                <input
                  type="number"
                  min="0.01"
                  step="0.01"
                  value={form.mm2}
                  onChange={(event) =>
                    setForm({ ...form, mm2: event.target.value })
                  }
                />
              </WorkspaceField>
              <WorkspaceField label="Barcode">
                <input
                  value={form.generic_barcode}
                  onChange={(event) =>
                    setForm({ ...form, generic_barcode: event.target.value })
                  }
                />
              </WorkspaceField>
              {!editing && (
                <>
                  <WorkspaceField label={t("catalog.tracking")}>
                    <select
                      value={form.tracking_mode}
                      onChange={(event) =>
                        setForm({
                          ...form,
                          tracking_mode: event.target
                            .value as CableForm["tracking_mode"],
                        })
                      }
                    >
                      <option value="quantity">{t("catalog.quantity")}</option>
                      <option value="individual">
                        {t("catalog.individual")}
                      </option>
                    </select>
                  </WorkspaceField>
                  <WorkspaceField label={t("catalog.initialQuantity")}>
                    <input
                      type="number"
                      min="0"
                      step="1"
                      value={form.quantity}
                      onChange={(event) =>
                        setForm({ ...form, quantity: event.target.value })
                      }
                    />
                  </WorkspaceField>
                  <WorkspaceField label={t("catalog.location")}>
                    <select
                      value={form.zone_id}
                      onChange={(event) =>
                        setForm({ ...form, zone_id: event.target.value })
                      }
                    >
                      <option value="">{t("catalog.noLocation")}</option>
                      {zones.map((zone) => (
                        <option key={zone.zone_id} value={zone.zone_id}>
                          {zone.name}
                        </option>
                      ))}
                    </select>
                  </WorkspaceField>
                </>
              )}
            </div>
            <p className="text-sm">
              <Link to="/products/master-data/connectors">
                {t("catalog.manageConnectors")}
              </Link>{" "}
              ·{" "}
              <Link to="/products/master-data/cable-types">
                {t("catalog.manageCableTypes")}
              </Link>
            </p>
            <button
              className="suite-button suite-button--primary"
              disabled={saving}
            >
              {t(saving ? "catalog.saving" : "catalog.save")}
            </button>
          </form>
        </WorkspaceDialog>
      )}
    </div>
  );
}
