import { useCatalogWriteAccess } from "./useCatalog";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api } from "../../lib/api";
import { catalogError, type CatalogProduct } from "../../lib/catalog";
import {
  WorkspaceDialog,
  WorkspaceField,
  WorkspaceLoading,
  WorkspaceNotice,
} from "./Workspace";
interface StockLocation {
  source: "zone" | "case";
  id: number | null;
  name: string;
  code: string;
  quantity: number;
}
export function ProductStockPanel({ product }: { product: CatalogProduct }) {
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [locations, setLocations] = useState<StockLocation[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<StockLocation | null>(null);
  const [quantity, setQuantity] = useState(0);
  const [reason, setReason] = useState("");
  const [saving, setSaving] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const { data } = await api.get<StockLocation[]>(
        `/admin/products/${product.product_id}/stock`,
      );
      setLocations(data);
    } catch (err) {
      setError(catalogError(err, t("catalog.loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [product.product_id, t]);
  useEffect(() => {
    void load();
  }, [load]);
  const correct = async (event: FormEvent) => {
    event.preventDefault();
    if (!selected) return;
    setSaving(true);
    setError("");
    try {
      await api.post(`/admin/products/${product.product_id}/stock`, {
        zone_id: selected.id,
        quantity,
        expected_quantity: selected.quantity,
        reason,
      });
      setSelected(null);
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  return (
    <div className="space-y-4">
      {error && (
        <WorkspaceNotice error retry={() => void load()}>
          {error}
        </WorkspaceNotice>
      )}
      {loading ? (
        <WorkspaceLoading />
      ) : (
        <>
          <div className="suite-card p-5">
            <h2 className="text-xl font-bold">
              {locations
                .reduce((sum, item) => sum + item.quantity, 0)
                .toLocaleString()}{" "}
              {product.count_type_abbr || t("catalog.units")}
            </h2>
            <p className="text-sm" style={{ color: "var(--text-secondary)" }}>
              {t("catalog.stockHelp")}
            </p>
          </div>
          <div className="suite-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("catalog.location")}</th>
                  <th>{t("catalog.quantityLabel")}</th>
                  <th>{t("catalog.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {locations.map((location) => (
                  <tr key={`${location.source}-${location.id}`}>
                    <td>
                      {location.id && location.source === "zone" ? (
                        <Link to={`/zones/${location.id}`}>
                          {location.name} · {location.code}
                        </Link>
                      ) : (
                        location.name
                      )}
                    </td>
                    <td>
                      {location.quantity.toLocaleString()}{" "}
                      {product.count_type_abbr}
                    </td>
                    <td>
                      {writable && location.source === "zone" && (
                        <button
                          className="suite-button"
                          onClick={() => {
                            setSelected(location);
                            setQuantity(location.quantity);
                            setReason("");
                          }}
                        >
                          {t("catalog.correctStock")}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {!locations.length && (
                  <tr>
                    <td colSpan={3}>{t("catalog.noStock")}</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          {writable && (
            <button
              className="suite-button"
              onClick={() => {
                setSelected({
                  source: "zone",
                  id: null,
                  name: t("catalog.noLocation"),
                  code: "",
                  quantity:
                    locations.find(
                      (item) => item.source === "zone" && item.id === null,
                    )?.quantity ?? 0,
                });
                setQuantity(
                  locations.find(
                    (item) => item.source === "zone" && item.id === null,
                  )?.quantity ?? 0,
                );
                setReason("");
              }}
            >
              {t("catalog.unassignedStock")}
            </button>
          )}
        </>
      )}
      {selected && (
        <WorkspaceDialog
          title={t("catalog.correctStock")}
          onClose={() => setSelected(null)}
        >
          <form className="space-y-4" onSubmit={(event) => void correct(event)}>
            <p>
              {selected.name}: {selected.quantity.toLocaleString()} →{" "}
              {quantity.toLocaleString()} {product.count_type_abbr}
            </p>
            <WorkspaceField label={t("catalog.newQuantity")}>
              <input
                required
                type="number"
                min="0"
                step="0.001"
                value={quantity}
                onChange={(event) => setQuantity(Number(event.target.value))}
              />
            </WorkspaceField>
            <WorkspaceField label={t("catalog.reason")}>
              <textarea
                required
                rows={3}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
              />
            </WorkspaceField>
            <button
              className="suite-button suite-button--primary"
              disabled={saving || !reason.trim()}
            >
              {t(saving ? "catalog.saving" : "catalog.save")}
            </button>
          </form>
        </WorkspaceDialog>
      )}
    </div>
  );
}
