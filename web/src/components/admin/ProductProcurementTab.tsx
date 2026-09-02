import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import {
  ExternalLink,
  Link2,
  RefreshCw,
  Search,
  ShoppingCart,
  Unlink,
  X,
} from "lucide-react";
import { api } from "../../lib/api";
import { toast } from "../../lib/toast";
import { ModalPortal } from "../ModalPortal";

type ProcurementProduct = {
  product_id: number;
  sku: string;
  name: string;
  manufacturer: string;
  model: string;
  unit: string;
  category: string;
  reorder_point: number;
  target_stock: number;
  best_price_cents: number;
  warehouse_product_id?: number;
};
type WarehouseProduct = {
  product_id: number;
  product_code: string;
  name: string;
  manufacturer: string;
  model: string;
  stock_quantity: number;
  min_stock_level: number;
  tracking_mode: string;
  procurement_product_id?: number;
  suggested_procurement_product_id?: number;
  suggestion_score: number;
  suggestion_reasons: string[];
};
type LinkData = {
  products: WarehouseProduct[];
  procurement_products: ProcurementProduct[];
};

const procurementBase = () =>
  window.__APP_CONFIG__?.procurementCoreURL || "/procurementcore";
const procurementHref = (path: string) =>
  `${String(procurementBase()).replace(/\/$/, "")}${path}`;
const optionStyle = {
  background: "var(--surface-2)",
  color: "var(--text-primary)",
};

export function ProductProcurementTab() {
  const [data, setData] = useState<LinkData>({
    products: [],
    procurement_products: [],
  });
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [selection, setSelection] = useState<Record<number, number>>({});
  const [need, setNeed] = useState<WarehouseProduct | null>(null);
  const load = useCallback(async () => {
    setLoading(true);
    try {
      const response = await api.get<LinkData>("/admin/product-links");
      setData(response.data);
      setSelection(
        Object.fromEntries(
          response.data.products
            .filter(
              (product) =>
                !product.procurement_product_id &&
                product.suggested_procurement_product_id,
            )
            .map((product) => [
              product.product_id,
              product.suggested_procurement_product_id!,
            ]),
        ),
      );
    } catch (error) {
      toast.error(
        "Produktabgleich konnte nicht geladen werden: " + String(error),
      );
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  const rows = useMemo(() => {
    const value = search.trim().toLowerCase();
    return value
      ? data.products.filter((product) =>
          `${product.product_code} ${product.name} ${product.manufacturer} ${product.model}`
            .toLowerCase()
            .includes(value),
        )
      : data.products;
  }, [data.products, search]);
  const procurementByID = useMemo(
    () =>
      new Map(
        data.procurement_products.map((product) => [
          product.product_id,
          product,
        ]),
      ),
    [data.procurement_products],
  );
  const link = async (product: WarehouseProduct) => {
    const procurement_product_id = selection[product.product_id];
    if (!procurement_product_id) return;
    try {
      await api.post(`/admin/products/${product.product_id}/procurement-link`, {
        procurement_product_id,
      });
      toast.success("Produkte wurden verknüpft");
      await load();
    } catch (error) {
      toast.error("Verknüpfung fehlgeschlagen: " + String(error));
    }
  };
  const unlink = async (product: WarehouseProduct) => {
    if (!confirm(`Verknüpfung für „${product.name}“ lösen?`)) return;
    try {
      await api.delete(
        `/admin/products/${product.product_id}/procurement-link`,
      );
      toast.success("Verknüpfung wurde gelöst");
      await load();
    } catch (error) {
      toast.error("Verknüpfung konnte nicht gelöst werden: " + String(error));
    }
  };
  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2
            className="text-2xl font-bold"
            style={{ color: "var(--text-primary)" }}
          >
            Beschaffung & Produktabgleich
          </h2>
          <p
            className="mt-1 text-sm"
            style={{ color: "var(--text-secondary)" }}
          >
            Warehouse-Produkte mit dem Einkaufskatalog verbinden und Bedarfe
            direkt melden.
          </p>
        </div>
        <button
          onClick={() => void load()}
          className="suite-button suite-button--secondary"
        >
          <RefreshCw className="h-4 w-4" />
          Aktualisieren
        </button>
      </div>
      <div
        className="flex flex-col gap-3 rounded-xl border p-4 sm:flex-row sm:items-center"
        style={{
          borderColor: "var(--border-default)",
          background: "var(--surface-1)",
        }}
      >
        <div className="suite-search-field flex-1">
          <Search className="h-4 w-4" />
          <input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Warehouse-Produkt suchen …"
          />
        </div>
        <span className="text-sm" style={{ color: "var(--text-secondary)" }}>
          {
            data.products.filter((product) => product.procurement_product_id)
              .length
          }{" "}
          von {data.products.length} verknüpft
        </span>
      </div>
      {loading ? (
        <div
          className="rounded-xl border p-8 text-center"
          style={{
            borderColor: "var(--border-default)",
            color: "var(--text-secondary)",
          }}
        >
          Produktstämme werden abgeglichen …
        </div>
      ) : (
        <div className="suite-table-wrap">
          <table>
            <thead>
              <tr>
                <th>WarehouseCore</th>
                <th>ProcurementCore</th>
                <th>Abgleich</th>
                <th>Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((product) => {
                const linked = product.procurement_product_id
                  ? procurementByID.get(product.procurement_product_id)
                  : undefined;
                const selectedID = selection[product.product_id];
                const selected = selectedID
                  ? procurementByID.get(selectedID)
                  : undefined;
                return (
                  <tr key={product.product_id}>
                    <td>
                      <div
                        className="font-semibold"
                        style={{ color: "var(--text-primary)" }}
                      >
                        {product.name}
                      </div>
                      <div
                        className="font-mono text-xs"
                        style={{ color: "var(--text-muted)" }}
                      >
                        {product.product_code}
                      </div>
                    </td>
                    <td>
                      {linked ? (
                        <>
                          <div style={{ color: "var(--text-primary)" }}>
                            {linked.name}
                          </div>
                          <div
                            className="text-xs"
                            style={{ color: "var(--text-muted)" }}
                          >
                            {linked.sku} ·{" "}
                            {[linked.manufacturer, linked.model]
                              .filter(Boolean)
                              .join(" ")}
                          </div>
                        </>
                      ) : (
                        <select
                          value={selectedID || ""}
                          onChange={(event) =>
                            setSelection((current) => ({
                              ...current,
                              [product.product_id]: Number(event.target.value),
                            }))
                          }
                          className="w-full min-w-64"
                        >
                          <option value="" style={optionStyle}>
                            Procurement-Artikel wählen …
                          </option>
                          {data.procurement_products
                            .filter((item) => !item.warehouse_product_id)
                            .map((item) => (
                              <option
                                key={item.product_id}
                                value={item.product_id}
                                style={optionStyle}
                              >
                                {item.sku} · {item.name}
                              </option>
                            ))}
                        </select>
                      )}
                    </td>
                    <td>
                      {linked ? (
                        <span
                          className="text-sm"
                          style={{ color: "var(--color-success)" }}
                        >
                          Verknüpft
                        </span>
                      ) : selected ? (
                        <>
                          <div
                            className="text-sm"
                            style={{
                              color:
                                product.suggestion_score >= 80
                                  ? "var(--color-success)"
                                  : "var(--color-warning)",
                            }}
                          >
                            {product.suggested_procurement_product_id ===
                            selectedID
                              ? `${product.suggestion_score} Punkte`
                              : "Manuell gewählt"}
                          </div>
                          <div
                            className="text-xs"
                            style={{ color: "var(--text-muted)" }}
                          >
                            {product.suggested_procurement_product_id ===
                            selectedID
                              ? product.suggestion_reasons.join(" · ")
                              : "wird erst nach Bestätigung verbunden"}
                          </div>
                        </>
                      ) : (
                        <span style={{ color: "var(--text-muted)" }}>
                          Nicht verknüpft
                        </span>
                      )}
                    </td>
                    <td>
                      <div className="flex flex-wrap justify-end gap-2">
                        {linked ? (
                          <>
                            <button
                              onClick={() => setNeed(product)}
                              className="suite-button suite-button--primary"
                            >
                              <ShoppingCart className="h-4 w-4" />
                              Bedarf
                            </button>
                            <a
                              className="suite-button suite-button--secondary"
                              href={procurementHref("/catalog")}
                            >
                              <ExternalLink className="h-4 w-4" />
                              Öffnen
                            </a>
                            <button
                              onClick={() => void unlink(product)}
                              className="suite-button suite-button--secondary"
                              title="Verknüpfung lösen"
                            >
                              <Unlink className="h-4 w-4" />
                            </button>
                          </>
                        ) : (
                          <button
                            disabled={!selectedID}
                            onClick={() => void link(product)}
                            className="suite-button suite-button--primary disabled:opacity-50"
                          >
                            <Link2 className="h-4 w-4" />
                            Verknüpfen
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {need && (
        <NeedModal
          product={need}
          procurement={
            need.procurement_product_id
              ? procurementByID.get(need.procurement_product_id)
              : undefined
          }
          onClose={() => setNeed(null)}
        />
      )}
    </div>
  );
}

function NeedModal({
  product,
  procurement,
  onClose,
}: {
  product: WarehouseProduct;
  procurement?: ProcurementProduct;
  onClose: () => void;
}) {
  const suggested = Math.max(
    1,
    (product.min_stock_level || 0) - (product.stock_quantity || 0),
  );
  const [quantity, setQuantity] = useState(suggested);
  const [neededBy, setNeededBy] = useState("");
  const [costCenter, setCostCenter] = useState("");
  const [justification, setJustification] = useState("");
  const [saving, setSaving] = useState(false);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      const response = await api.post<{ number: string }>(
        `/admin/products/${product.product_id}/procurement-requisitions`,
        {
          quantity,
          needed_by: neededBy || null,
          cost_center: costCenter,
          justification,
        },
      );
      toast.success(
        `Bedarf ${response.data.number} wurde als Entwurf angelegt`,
      );
      onClose();
    } catch (error) {
      toast.error("Bedarf konnte nicht angelegt werden: " + String(error));
    } finally {
      setSaving(false);
    }
  };
  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[140] flex items-center justify-center bg-black/75 p-4">
        <form
          onSubmit={submit}
          className="w-full max-w-xl rounded-2xl border p-5 shadow-xl"
          style={{
            borderColor: "var(--border-default)",
            background: "var(--surface-1)",
          }}
        >
          <div className="mb-5 flex items-start justify-between">
            <div>
              <h3
                className="text-xl font-bold"
                style={{ color: "var(--text-primary)" }}
              >
                Bedarf melden
              </h3>
              <p
                className="mt-1 text-sm"
                style={{ color: "var(--text-secondary)" }}
              >
                {product.name} → {procurement?.name}
              </p>
            </div>
            <button
              type="button"
              onClick={onClose}
              className="suite-button suite-button--secondary"
              aria-label="Schließen"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <label
              className="text-sm"
              style={{ color: "var(--text-secondary)" }}
            >
              Menge
              <input
                required
                type="number"
                min="0.01"
                step="0.01"
                value={quantity}
                onChange={(event) => setQuantity(Number(event.target.value))}
                className="mt-1 w-full"
              />
            </label>
            <label
              className="text-sm"
              style={{ color: "var(--text-secondary)" }}
            >
              Benötigt bis
              <input
                type="date"
                value={neededBy}
                onChange={(event) => setNeededBy(event.target.value)}
                className="mt-1 w-full"
              />
            </label>
            <label
              className="text-sm sm:col-span-2"
              style={{ color: "var(--text-secondary)" }}
            >
              Kostenstelle
              <input
                value={costCenter}
                onChange={(event) => setCostCenter(event.target.value)}
                className="mt-1 w-full"
              />
            </label>
            <label
              className="text-sm sm:col-span-2"
              style={{ color: "var(--text-secondary)" }}
            >
              Begründung
              <textarea
                value={justification}
                onChange={(event) => setJustification(event.target.value)}
                className="mt-1 w-full"
                placeholder="Warum wird das Material benötigt?"
              />
            </label>
          </div>
          <div className="mt-5 flex justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              className="suite-button suite-button--secondary"
            >
              Abbrechen
            </button>
            <button
              disabled={saving}
              className="suite-button suite-button--primary"
            >
              {saving ? "Wird angelegt …" : "Als Entwurf anlegen"}
            </button>
          </div>
        </form>
      </div>
    </ModalPortal>
  );
}
