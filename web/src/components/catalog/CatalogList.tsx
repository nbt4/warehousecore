import { useCatalogWriteAccess, useDebouncedValue } from "./useCatalog";
import { useCallback, useEffect, useState } from "react";
import { Link, useSearchParams, Navigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Plus, Search, Save } from "lucide-react";
import { api } from "../../lib/api";
import {
  catalogError,
  productCategory,
  type CatalogProduct,
} from "../../lib/catalog";
import {
  WorkspaceHeader,
  WorkspaceLoading,
  WorkspaceNotice,
  WorkspaceDialog,
  WorkspaceField,
} from "./Workspace";

const pageSize = 50;
interface SavedView {
  name: string;
  query: string;
}
export function CatalogList({
  websiteOnly = false,
}: {
  websiteOnly?: boolean;
}) {
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [params, setParams] = useSearchParams();
  const [products, setProducts] = useState<CatalogProduct[]>([]);
  const [total, setTotal] = useState(0);
  const [categories, setCategories] = useState<
    { category_id: number; name: string }[]
  >([]);
  const [search, setSearch] = useState(params.get("search") ?? "");
  const debounced = useDebouncedValue(search);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [views, setViews] = useState<SavedView[]>(() => {
    try {
      const value: unknown = JSON.parse(
        localStorage.getItem("warehouse_catalog_views") ?? "[]",
      );
      return Array.isArray(value)
        ? value.filter(
            (view): view is SavedView =>
              typeof view?.name === "string" && typeof view?.query === "string",
          )
        : [];
    } catch {
      return [];
    }
  });
  const [selected, setSelected] = useState<number[]>([]);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkCategory, setBulkCategory] = useState("");
  const [bulkSaving, setBulkSaving] = useState(false);
  const [savingView, setSavingView] = useState(false);
  const [viewName, setViewName] = useState("");
  const page = Math.max(1, Number(params.get("page")) || 1);
  const filtersWithoutSearch = new URLSearchParams(params);
  filtersWithoutSearch.delete("search");
  const query = filtersWithoutSearch.toString();
  const load = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true);
      setError("");
      try {
        const filters = new URLSearchParams(query);
        filters.delete("page");
        filters.set("search", debounced);
        filters.set("limit", String(pageSize));
        filters.set("offset", String((page - 1) * pageSize));
        const result = await api.get<CatalogProduct[]>(
          `/admin/products?${filters}`,
          { signal },
        );
        if (signal?.aborted) return;
        setProducts(result.data ?? []);
        setSelected([]);
        setTotal(
          Number(result.headers["x-total-count"]) || result.data?.length || 0,
        );
      } catch (err) {
        if (!signal?.aborted)
          setError(catalogError(err, t("catalog.loadFailed")));
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [query, debounced, page, t],
  );
  useEffect(() => {
    const abort = new AbortController();
    void load(abort.signal);
    return () => abort.abort();
  }, [load]);
  useEffect(() => {
    let current = true;
    void api
      .get<{ category_id: number; name: string }[]>("/admin/categories")
      .then(({ data }) => {
        if (current) setCategories(data ?? []);
      })
      .catch(() => {});
    return () => {
      current = false;
    };
  }, []);
  useEffect(() => {
    setSearch(params.get("search") ?? "");
  }, [params]);
  const filter = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    next.delete("page");
    setParams(next, { replace: true });
  };
  const applyView = (value: string) => {
    const next = new URLSearchParams();
    if (value === "missing") next.set("missing_images", "true");
    if (value === "unpublished") next.set("website_visible", "false");
    if (value === "low") next.set("below_min_stock", "true");
    setParams(next);
  };
  const assignCategory = async () => {
    setBulkSaving(true);
    setError("");
    try {
      await api.post("/admin/products/bulk-categories", {
        products: products
          .filter((product) => selected.includes(product.product_id))
          .map((product) => ({
            product_id: product.product_id,
            version: product.version,
          })),
        category_id: Number(bulkCategory) || null,
      });
      setBulkOpen(false);
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setBulkSaving(false);
    }
  };
  if (params.get("product_id"))
    return (
      <Navigate to={`/products/${params.get("product_id")}/overview`} replace />
    );
  if (params.get("procurement_product_id"))
    return (
      <Navigate
        to={`/products/new?procurement_product_id=${params.get("procurement_product_id")}`}
        replace
      />
    );
  return (
    <div className="space-y-5">
      <WorkspaceHeader
        title={t(websiteOnly ? "catalog.website" : "catalog.catalog")}
        description={t(
          websiteOnly
            ? "catalog.websiteIndexHelp"
            : "catalog.catalogDescription",
        )}
      >
        {writable && !websiteOnly && (
          <Link
            className="suite-button suite-button--primary"
            to="/products/new"
          >
            <Plus size={16} />
            {t("catalog.newProduct")}
          </Link>
        )}
      </WorkspaceHeader>
      {websiteOnly && (
        <Link className="suite-button" to="/products/packages">
          {t("catalog.packageWebsite")}
        </Link>
      )}
      <section className="suite-card p-5 space-y-4">
        <div className="flex flex-wrap gap-3">
          <div className="suite-search-field min-w-0 flex-1">
            <Search size={18} />
            <input
              type="search"
              value={search}
              placeholder={t("catalog.search")}
              onChange={(event) => {
                setSearch(event.target.value);
                filter("search", event.target.value);
              }}
            />
          </div>
          <select
            aria-label={t("catalog.category")}
            value={params.get("category_id") ?? ""}
            onChange={(event) => filter("category_id", event.target.value)}
          >
            <option value="">{t("catalog.allCategories")}</option>
            {categories.map((category) => (
              <option key={category.category_id} value={category.category_id}>
                {category.name}
              </option>
            ))}
          </select>
          <select
            aria-label={t("catalog.kind")}
            value={params.get("product_kind") ?? ""}
            onChange={(event) => filter("product_kind", event.target.value)}
          >
            <option value="">{t("catalog.allKinds")}</option>
            {["standard", "cable", "consumable", "container", "service"].map(
              (value) => (
                <option key={value} value={value}>
                  {t(`catalog.${value}`)}
                </option>
              ),
            )}
          </select>
          <select
            aria-label={t("catalog.lifecycle")}
            value={params.get("lifecycle_status") ?? "active"}
            onChange={(event) => filter("lifecycle_status", event.target.value)}
          >
            {["active", "archived", "all"].map((value) => (
              <option key={value} value={value}>
                {t(`catalog.${value}`)}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-wrap gap-2">
          {[
            ["all", "viewAll"],
            ["missing", "viewMissingImages"],
            ["unpublished", "viewUnpublished"],
            ["low", "viewLowStock"],
          ].map(([value, label]) => (
            <button
              key={value}
              className="suite-button"
              onClick={() => applyView(value)}
            >
              {t(`catalog.${label}`)}
            </button>
          ))}
          {views.map((view) => (
            <button
              key={view.name}
              className="suite-button"
              onClick={() => setParams(new URLSearchParams(view.query))}
            >
              {view.name}
            </button>
          ))}
          <button className="suite-button" onClick={() => setSavingView(true)}>
            <Save size={16} />
            {t("catalog.saveView")}
          </button>
        </div>
      </section>
      {selected.length > 0 && (
        <div className="flex flex-wrap items-center gap-3">
          <span>{t("catalog.selectedCount", { count: selected.length })}</span>
          <button className="suite-button" onClick={() => setBulkOpen(true)}>
            {t("catalog.assignCategory")}
          </button>
        </div>
      )}
      {error && (
        <WorkspaceNotice error retry={() => void load()}>
          {error}
        </WorkspaceNotice>
      )}
      {loading ? (
        <WorkspaceLoading />
      ) : (
        <>
          <div className="suite-table-wrap">
            <table>
              <thead>
                <tr>
                  {writable && (
                    <th>
                      <input
                        type="checkbox"
                        aria-label={t("catalog.selectPage")}
                        checked={
                          products.length > 0 &&
                          selected.length === products.length
                        }
                        onChange={(event) =>
                          setSelected(
                            event.target.checked
                              ? products.map((product) => product.product_id)
                              : [],
                          )
                        }
                      />
                    </th>
                  )}
                  <th>{t("catalog.product")}</th>
                  <th>{t("catalog.category")}</th>
                  <th>{t("catalog.stock")}</th>
                  <th>{t("catalog.website")}</th>
                  <th>{t("catalog.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {products.map((product) => (
                  <tr key={product.product_id}>
                    {writable && (
                      <td>
                        <input
                          type="checkbox"
                          aria-label={t("catalog.selectProduct", {
                            name: product.name,
                          })}
                          checked={selected.includes(product.product_id)}
                          onChange={(event) =>
                            setSelected((old) =>
                              event.target.checked
                                ? [...old, product.product_id]
                                : old.filter((id) => id !== product.product_id),
                            )
                          }
                        />
                      </td>
                    )}
                    <td>
                      <Link
                        className="font-semibold underline"
                        to={`/products/${product.product_id}/${websiteOnly ? "website" : "overview"}`}
                      >
                        {product.name}
                      </Link>
                      <div
                        className="font-mono text-xs"
                        style={{ color: "var(--text-secondary)" }}
                      >
                        {product.product_code}
                      </div>
                      <div
                        className="text-xs"
                        style={{ color: "var(--text-secondary)" }}
                      >
                        {[
                          product.brand_name || product.manufacturer_name,
                          product.model_number,
                        ]
                          .filter(Boolean)
                          .join(" · ")}
                      </div>
                    </td>
                    <td>
                      {productCategory(product) || "—"}
                      <div
                        className="text-xs"
                        style={{ color: "var(--text-secondary)" }}
                      >
                        {t(`catalog.${product.product_kind}`)}
                      </div>
                    </td>
                    <td>
                      <Link to={`/products/${product.product_id}/stock`}>
                        {product.tracking_mode === "individual"
                          ? t("catalog.unitCount", {
                              count: product.device_count ?? 0,
                            })
                          : product.tracking_mode === "quantity"
                            ? `${product.stock_quantity ?? 0} ${product.count_type_abbr || ""}`
                            : t("catalog.noTracking")}
                      </Link>
                    </td>
                    <td>
                      {t(
                        product.website_visible
                          ? "catalog.websiteState.published"
                          : "catalog.websiteState.draft",
                      )}
                      {!product.website_thumbnail && (
                        <div
                          className="text-xs"
                          style={{ color: "var(--color-warning)" }}
                        >
                          {t("catalog.coverMissing")}
                        </div>
                      )}
                    </td>
                    <td>
                      <Link
                        className="suite-button"
                        to={`/products/${product.product_id}/${websiteOnly ? "website" : "overview"}`}
                      >
                        {t("catalog.details")}
                      </Link>
                    </td>
                  </tr>
                ))}
                {!products.length && (
                  <tr>
                    <td colSpan={writable ? 6 : 5}>{t("catalog.noneFound")}</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <div
            className="flex flex-wrap items-center justify-between gap-3"
            style={{ color: "var(--text-secondary)" }}
          >
            <span>{t("catalog.results", { count: total })}</span>
            <div className="flex items-center gap-3">
              <button
                className="suite-button"
                disabled={page <= 1}
                onClick={() => {
                  const next = new URLSearchParams(params);
                  next.set("page", String(page - 1));
                  setParams(next);
                }}
              >
                {t("catalog.previous")}
              </button>
              <span className="text-sm">
                {t("catalog.page", {
                  page,
                  total: Math.max(1, Math.ceil(total / pageSize)),
                })}
              </span>
              <button
                className="suite-button"
                disabled={page * pageSize >= total}
                onClick={() => {
                  const next = new URLSearchParams(params);
                  next.set("page", String(page + 1));
                  setParams(next);
                }}
              >
                {t("catalog.next")}
              </button>
            </div>
          </div>
        </>
      )}
      {bulkOpen && (
        <WorkspaceDialog
          title={t("catalog.assignCategory")}
          onClose={() => {
            if (!bulkSaving) setBulkOpen(false);
          }}
        >
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault();
              void assignCategory();
            }}
          >
            <WorkspaceNotice>{t("catalog.bulkCategoryHelp")}</WorkspaceNotice>
            <WorkspaceField label={t("catalog.category")}>
              <select
                value={bulkCategory}
                onChange={(event) => setBulkCategory(event.target.value)}
              >
                <option value="">{t("catalog.noCategory")}</option>
                {categories.map((category) => (
                  <option
                    key={category.category_id}
                    value={category.category_id}
                  >
                    {category.name}
                  </option>
                ))}
              </select>
            </WorkspaceField>
            {error && <WorkspaceNotice error>{error}</WorkspaceNotice>}
            <button
              className="suite-button suite-button--primary"
              disabled={bulkSaving}
            >
              {t(bulkSaving ? "catalog.saving" : "catalog.save")}
            </button>
          </form>
        </WorkspaceDialog>
      )}
      {savingView && (
        <WorkspaceDialog
          title={t("catalog.saveView")}
          onClose={() => setSavingView(false)}
        >
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault();
              const next = [
                ...views.filter((view) => view.name !== viewName.trim()),
                { name: viewName.trim(), query: params.toString() },
              ].slice(-10);
              setViews(next);
              localStorage.setItem(
                "warehouse_catalog_views",
                JSON.stringify(next),
              );
              setSavingView(false);
              setViewName("");
            }}
          >
            <WorkspaceField label={t("catalog.viewName")}>
              <input
                required
                value={viewName}
                onChange={(event) => setViewName(event.target.value)}
                maxLength={80}
              />
            </WorkspaceField>
            <button
              className="suite-button suite-button--primary"
              disabled={!viewName.trim()}
            >
              {t("catalog.save")}
            </button>
          </form>
        </WorkspaceDialog>
      )}
    </div>
  );
}
