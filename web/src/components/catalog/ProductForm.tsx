import { useCatalogWriteAccess } from "./useCatalog";
import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { api } from "../../lib/api";
import {
  catalogError,
  loadCatalogMetadata,
  type CatalogProduct,
  type CatalogMetadata,
} from "../../lib/catalog";
import { useUnsavedChanges } from "./useCatalog";
import { WorkspaceField, WorkspaceLoading, WorkspaceNotice } from "./Workspace";

export function ProductForm({
  product,
  prefill,
  onSaved,
  onCancel,
}: {
  product?: CatalogProduct;
  prefill?: Partial<CatalogProduct>;
  onSaved: (id: number) => void;
  onCancel?: () => void;
}) {
  const { t } = useTranslation();
  const permitted = useCatalogWriteAccess();
  const writable = permitted && product?.lifecycle_status !== "archived";
  const [metadata, setMetadata] = useState<CatalogMetadata | null>(null);
  const [data, setData] = useState<Partial<CatalogProduct>>(() =>
    product
      ? { ...product }
      : {
          name: "",
          product_type: "equipment",
          product_kind: "standard",
          tracking_mode: "individual",
          ...prefill,
        },
  );
  const [attributes, setAttributes] = useState(
    JSON.stringify(product?.attributes ?? prefill?.attributes ?? {}, null, 2),
  );
  const [initialQuantity, setInitialQuantity] = useState(0);
  const [stage, setStage] = useState(0);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [savedData, setSavedData] = useState(
    JSON.stringify({ data, attributes, initialQuantity }),
  );
  const releaseNavigationGuard = useUnsavedChanges(
    writable &&
      JSON.stringify({ data, attributes, initialQuantity }) !== savedData,
    t("catalog.confirmDiscard"),
    () => {
      const baseline = JSON.parse(savedData);
      setData(baseline.data);
      setAttributes(baseline.attributes);
      setInitialQuantity(baseline.initialQuantity);
    },
  );
  useEffect(() => {
    let current = true;
    void loadCatalogMetadata()
      .then((value) => {
        if (current) setMetadata(value);
      })
      .catch((err) => {
        if (current) setError(catalogError(err, t("catalog.loadFailed")));
      });
    return () => {
      current = false;
    };
  }, [t]);
  const set = <K extends keyof CatalogProduct>(
    key: K,
    value: CatalogProduct[K],
  ) => setData((old) => ({ ...old, [key]: value }));
  const numeric = (key: keyof CatalogProduct, label: string, step = "0.01") => (
    <WorkspaceField key={key} label={t(`catalog.${label}`)}>
      <input
        type="number"
        min="0"
        step={step}
        value={(data[key] as number) ?? ""}
        disabled={!writable || saving}
        onChange={(event) =>
          setData((old) => ({
            ...old,
            [key]:
              event.target.value === "" ? null : Number(event.target.value),
          }))
        }
      />
    </WorkspaceField>
  );
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!writable) return;
    let parsed: unknown;
    try {
      parsed = JSON.parse(attributes);
      if (
        typeof parsed !== "object" ||
        parsed === null ||
        Array.isArray(parsed)
      )
        throw new Error();
    } catch {
      setError(t("catalog.invalidAttributes"));
      return;
    }
    setSaving(true);
    setError("");
    try {
      const payload = {
        ...data,
        expectedUpdatedAt: product?.version,
        attributes: parsed,
        is_accessory: data.product_type === "accessory",
        is_consumable: data.product_type === "consumable",
        initial_device_quantity: product
          ? 0
          : data.tracking_mode === "individual"
            ? initialQuantity
            : 0,
        stock_quantity:
          !product && data.tracking_mode === "quantity"
            ? initialQuantity
            : undefined,
      };
      if (product) {
        await api.put(`/admin/products/${product.product_id}`, payload);
        setSavedData(JSON.stringify({ data, attributes, initialQuantity }));
        releaseNavigationGuard();
        onSaved(product.product_id);
      } else {
        const result = await api.post<CatalogProduct>(
          "/admin/products",
          payload,
        );
        setSavedData(JSON.stringify({ data, attributes, initialQuantity }));
        releaseNavigationGuard();
        onSaved(result.data.product_id);
      }
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  if (!metadata)
    return error ? (
      <WorkspaceNotice
        error
        retry={() => {
          setError("");
          void loadCatalogMetadata()
            .then(setMetadata)
            .catch((err) =>
              setError(catalogError(err, t("catalog.loadFailed"))),
            );
        }}
      >
        {error}
      </WorkspaceNotice>
    ) : (
      <WorkspaceLoading />
    );
  const stages = ["identity", "stock", "technical"];
  return (
    <form className="space-y-5" onSubmit={(event) => void save(event)}>
      {error && <WorkspaceNotice error>{error}</WorkspaceNotice>}
      {!writable && (
        <WorkspaceNotice>{t("catalog.noPermission")}</WorkspaceNotice>
      )}
      {!product && (
        <p>
          <Link className="underline" to="/cables?create=true">
            {t("catalog.newCable")}
          </Link>
        </p>
      )}
      {!product && (
        <nav
          className="flex flex-wrap gap-2"
          aria-label={t("catalog.newProduct")}
        >
          {stages.map((name, index) => (
            <button
              type="button"
              className={`suite-button ${stage === index ? "suite-button--primary" : ""}`}
              key={name}
              aria-current={stage === index ? "step" : undefined}
              onClick={() => setStage(index)}
            >
              {index + 1}. {t(`catalog.${name}`)}
            </button>
          ))}
        </nav>
      )}
      {(product || stage === 0) && (
        <section className="suite-card p-5 space-y-4">
          <h2 className="text-xl font-bold">{t("catalog.identity")}</h2>
          <div className="grid gap-4 sm:grid-cols-2">
            <WorkspaceField label={t("catalog.name")}>
              <input
                required
                maxLength={255}
                value={data.name ?? ""}
                disabled={!writable || saving}
                onChange={(event) => set("name", event.target.value)}
              />
            </WorkspaceField>
            <WorkspaceField label={t("catalog.kind")}>
              <select
                value={data.product_kind}
                disabled={!writable || saving}
                onChange={(event) => {
                  const kind = event.target
                    .value as CatalogProduct["product_kind"];
                  setData((old) => ({
                    ...old,
                    product_kind: kind,
                    ...(kind === "consumable"
                      ? {
                          product_type: "consumable",
                          tracking_mode: "quantity",
                        }
                      : kind === "service"
                        ? { product_type: "equipment", tracking_mode: "none" }
                        : {}),
                  }));
                }}
              >
                {((product?.product_kind ?? prefill?.product_kind) === "cable"
                  ? ["cable"]
                  : ["standard", "consumable", "container", "service"]
                ).map((kind) => (
                  <option key={kind} value={kind}>
                    {t(`catalog.${kind}`)}
                  </option>
                ))}
              </select>
            </WorkspaceField>
            <WorkspaceField label={t("catalog.manufacturer")}>
              <select
                value={data.manufacturer_id ?? ""}
                disabled={!writable || saving}
                onChange={(event) =>
                  setData((old) => ({
                    ...old,
                    manufacturer_id: Number(event.target.value) || null,
                    manufacturer_name_input: undefined,
                    brand_id: null,
                  }))
                }
              >
                <option value="">{t("catalog.choose")}</option>
                {metadata.manufacturers.map((item) => (
                  <option
                    key={item.manufacturer_id}
                    value={item.manufacturer_id}
                  >
                    {item.name}
                  </option>
                ))}
              </select>
            </WorkspaceField>
            <WorkspaceField label={t("catalog.brand")}>
              <select
                value={data.brand_id ?? ""}
                disabled={!writable || saving}
                onChange={(event) =>
                  set("brand_id", Number(event.target.value) || null)
                }
              >
                <option value="">{t("catalog.choose")}</option>
                {metadata.brands.map((item) => (
                  <option key={item.brand_id} value={item.brand_id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </WorkspaceField>
            <WorkspaceField label={t("catalog.category")}>
              <select
                value={data.category_id ?? ""}
                disabled={!writable || saving}
                onChange={(event) =>
                  setData((old) => ({
                    ...old,
                    category_id: Number(event.target.value) || null,
                    subcategory_id: null,
                    subbiercategory_id: null,
                  }))
                }
              >
                <option value="">{t("catalog.choose")}</option>
                {metadata.categories.map((item) => (
                  <option key={item.category_id} value={item.category_id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </WorkspaceField>
            <WorkspaceField label={t("catalog.subcategory")}>
              <select
                value={data.subcategory_id ?? ""}
                disabled={!writable || saving || !data.category_id}
                onChange={(event) =>
                  setData((old) => ({
                    ...old,
                    subcategory_id: event.target.value || null,
                    subbiercategory_id: null,
                  }))
                }
              >
                <option value="">{t("catalog.choose")}</option>
                {metadata.subcategories
                  .filter((item) => item.category_id === data.category_id)
                  .map((item) => (
                    <option
                      key={item.subcategory_id}
                      value={item.subcategory_id}
                    >
                      {item.name}
                    </option>
                  ))}
              </select>
            </WorkspaceField>
            <WorkspaceField label={t("catalog.thirdCategory")}>
              <select
                value={data.subbiercategory_id ?? ""}
                disabled={!writable || saving || !data.subcategory_id}
                onChange={(event) =>
                  set("subbiercategory_id", event.target.value || null)
                }
              >
                <option value="">{t("catalog.choose")}</option>
                {metadata.thirdCategories
                  .filter(
                    (item) =>
                      String(item.subcategory_id) ===
                      String(data.subcategory_id),
                  )
                  .map((item) => (
                    <option
                      key={item.subbiercategory_id}
                      value={item.subbiercategory_id}
                    >
                      {item.name}
                    </option>
                  ))}
              </select>
            </WorkspaceField>
            <WorkspaceField label={t("catalog.model")}>
              <input
                value={data.model_number ?? ""}
                disabled={!writable || saving}
                onChange={(event) => set("model_number", event.target.value)}
              />
            </WorkspaceField>
            <WorkspaceField label={t("catalog.manufacturerPart")}>
              <input
                value={data.manufacturer_part_number ?? ""}
                disabled={!writable || saving}
                onChange={(event) =>
                  set("manufacturer_part_number", event.target.value)
                }
              />
            </WorkspaceField>
            <WorkspaceField label="EAN / GTIN">
              <input
                value={data.ean ?? ""}
                disabled={!writable || saving}
                onChange={(event) => set("ean", event.target.value)}
              />
            </WorkspaceField>
          </div>
          <WorkspaceField label={t("catalog.description")}>
            <textarea
              rows={4}
              value={data.description ?? ""}
              disabled={!writable || saving}
              onChange={(event) => set("description", event.target.value)}
            />
          </WorkspaceField>
        </section>
      )}
      {(product || stage === 1) && (
        <section className="suite-card p-5 space-y-4">
          <h2 className="text-xl font-bold">{t("catalog.stock")}</h2>
          <div className="grid gap-4 sm:grid-cols-2">
            <WorkspaceField label={t("catalog.type")}>
              <select
                value={data.product_type}
                disabled={!writable || saving}
                onChange={(event) => {
                  const type = event.target
                    .value as CatalogProduct["product_type"];
                  setData((old) => ({
                    ...old,
                    product_type: type,
                    tracking_mode:
                      type === "consumable"
                        ? "quantity"
                        : type === "equipment" &&
                            old.tracking_mode === "quantity"
                          ? "individual"
                          : old.tracking_mode,
                  }));
                }}
              >
                {["equipment", "accessory", "consumable"].map((type) => (
                  <option key={type} value={type}>
                    {t(`catalog.${type}`)}
                  </option>
                ))}
              </select>
            </WorkspaceField>
            <WorkspaceField
              label={t("catalog.tracking")}
              hint={t("catalog.trackingHelp")}
            >
              <select
                value={data.tracking_mode}
                disabled={!writable || saving}
                onChange={(event) =>
                  set(
                    "tracking_mode",
                    event.target.value as CatalogProduct["tracking_mode"],
                  )
                }
              >
                {data.product_type !== "consumable" && (
                  <option value="individual">{t("catalog.individual")}</option>
                )}
                {data.product_type !== "equipment" && (
                  <option value="quantity">{t("catalog.quantity")}</option>
                )}
                {data.product_type !== "consumable" &&
                  data.product_kind !== "cable" && (
                    <option value="none">{t("catalog.noTracking")}</option>
                  )}
              </select>
            </WorkspaceField>
            {data.tracking_mode === "quantity" && (
              <WorkspaceField label={t("catalog.unit")}>
                <select
                  required
                  value={data.count_type_id ?? ""}
                  disabled={!writable || saving}
                  onChange={(event) =>
                    set("count_type_id", Number(event.target.value) || null)
                  }
                >
                  <option value="">{t("catalog.choose")}</option>
                  {metadata.units.map((item) => (
                    <option key={item.count_type_id} value={item.count_type_id}>
                      {item.name} ({item.abbreviation})
                    </option>
                  ))}
                </select>
              </WorkspaceField>
            )}
            {!product && data.tracking_mode !== "none" && (
              <WorkspaceField label={t("catalog.initialQuantity")}>
                <input
                  type="number"
                  min="0"
                  step={data.tracking_mode === "individual" ? "1" : "0.01"}
                  value={initialQuantity}
                  onChange={(event) =>
                    setInitialQuantity(Number(event.target.value))
                  }
                />
              </WorkspaceField>
            )}
            {numeric("min_stock_level", "minStock")}
            {numeric("item_cost_per_day", "dailyPrice")}
            {numeric("price_per_unit", "unitPrice")}
            <WorkspaceField label="Barcode">
              <input
                value={data.generic_barcode ?? ""}
                disabled={!writable || saving}
                onChange={(event) => set("generic_barcode", event.target.value)}
              />
            </WorkspaceField>
          </div>
        </section>
      )}
      {(product || stage === 2) && (
        <section className="suite-card p-5 space-y-4">
          <h2 className="text-xl font-bold">{t("catalog.technical")}</h2>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {numeric("weight", "weight")}
            {numeric("width", "width")}
            {numeric("height", "height")}
            {numeric("depth", "depth")}
            {numeric("power_consumption", "power")}
            {numeric("maintenance_interval", "maintenanceInterval", "1")}
          </div>
          <WorkspaceField label={t("catalog.attributes")}>
            <textarea
              className="font-mono"
              rows={5}
              value={attributes}
              disabled={!writable || saving}
              onChange={(event) => setAttributes(event.target.value)}
            />
          </WorkspaceField>
        </section>
      )}
      <div className="flex flex-wrap justify-end gap-2">
        {onCancel && (
          <button type="button" className="suite-button" onClick={onCancel}>
            {t("catalog.cancel")}
          </button>
        )}
        {!product && stage < 2 && (
          <button
            type="button"
            className="suite-button"
            onClick={() => setStage(stage + 1)}
          >
            {t("catalog.next")}
          </button>
        )}
        {writable && (product || stage === 2) && (
          <button
            className="suite-button suite-button--primary"
            disabled={saving || !data.name?.trim()}
          >
            {t(
              saving
                ? "catalog.saving"
                : product
                  ? "catalog.save"
                  : "catalog.newProduct",
            )}
          </button>
        )}
      </div>
    </form>
  );
}
