import { useCatalogWriteAccess } from "./useCatalog";
import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api } from "../../lib/api";
import {
  catalogError,
  productCategory,
  type CatalogProduct,
} from "../../lib/catalog";
import { ProductProcurementTab } from "../admin/ProductProcurementTab";
import { ProductForm } from "./ProductForm";
import { DevicesPanel } from "./DevicesPanel";
import { WebsitePanel } from "./WebsitePanel";
import {
  WorkspaceHeader,
  WorkspaceLoading,
  WorkspaceNotice,
} from "./Workspace";
import { RelationsPanel } from "./RelationsPanel";
import { ProductStockPanel } from "./ProductStockPanel";
import { ProductHistoryPanel } from "./ProductHistoryPanel";

export function ProductWorkspace() {
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const { id, section = "overview" } = useParams();
  const navigate = useNavigate();
  const [product, setProduct] = useState<CatalogProduct | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const load = useCallback(
    async (signal?: AbortSignal) => {
      setError("");
      try {
        const result = await api.get<CatalogProduct>(`/admin/products/${id}`, {
          signal,
        });
        if (!signal?.aborted) setProduct(result.data);
      } catch (err) {
        if (!signal?.aborted)
          setError(catalogError(err));
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [id],
  );
  useEffect(() => {
    setProduct(null);
    setLoading(true);
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);
  const lifecycle = async () => {
    if (
      !product ||
      !window.confirm(
        t(
          product.lifecycle_status === "archived"
            ? "catalog.confirmRestore"
            : "catalog.confirmArchive",
        ),
      )
    )
      return;
    try {
      if (product.lifecycle_status === "archived")
        await api.put(`/admin/products/${id}/restore`);
      else await api.delete(`/admin/products/${id}`);
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    }
  };
  if (loading) return <WorkspaceLoading />;
  if (!product)
    return (
      <WorkspaceNotice error retry={() => void load()}>
        {error || t("catalog.loadFailed")}
      </WorkspaceNotice>
    );
  const tabs = [
    "overview",
    "stock",
    "relations",
    "website",
    "procurement",
    "history",
  ];
  return (
    <div className="space-y-5">
      <Link className="suite-button" to="/products">
        ← {t("catalog.backToCatalog")}
      </Link>
      <WorkspaceHeader
        title={product.name}
        description={`${product.product_code} · ${productCategory(product) || t("catalog.noCategory")}`}
      >
        {writable && (
          <button className="suite-button" onClick={() => void lifecycle()}>
            {t(
              product.lifecycle_status === "archived"
                ? "catalog.restore"
                : "catalog.archive",
            )}
          </button>
        )}
      </WorkspaceHeader>
      <div
        className="flex flex-wrap gap-4 text-sm"
        style={{ color: "var(--text-secondary)" }}
      >
        <span>{t(`catalog.${product.lifecycle_status}`)}</span>
        <span>{t(`catalog.${product.product_kind}`)}</span>
        <span>
          {t(
            product.tracking_mode === "individual"
              ? "catalog.individual"
              : product.tracking_mode === "quantity"
                ? "catalog.quantity"
                : "catalog.noTracking",
          )}
        </span>
        <span>
          {t(
            product.website_visible
              ? "catalog.websiteState.published"
              : "catalog.websiteState.draft",
          )}
        </span>
      </div>
      <nav
        className="flex flex-wrap gap-2"
        aria-label={t("catalog.product")}
        style={{
          borderBottom: "1px solid var(--border-default)",
          paddingBottom: "var(--space-3)",
        }}
      >
        {tabs.map((tab) => (
          <Link
            key={tab}
            className={`suite-button ${section === tab ? "suite-button--primary" : ""}`}
            aria-current={section === tab ? "page" : undefined}
            to={`/products/${id}/${tab}`}
          >
            {t(`catalog.${tab}`)}
          </Link>
        ))}
      </nav>
      {error && <WorkspaceNotice error>{error}</WorkspaceNotice>}
      {section === "overview" && (
        <ProductForm
          key={`${product.product_id}-${product.version}`}
          product={product}
          onSaved={() => {
            void load();
          }}
        />
      )}
      {section === "stock" &&
        (product.tracking_mode === "individual" ? (
          <DevicesPanel product={product} />
        ) : product.tracking_mode === "quantity" ? (
          <ProductStockPanel product={product} />
        ) : (
          <WorkspaceNotice>{t("catalog.noTracking")}</WorkspaceNotice>
        ))}
      {section === "relations" && <RelationsPanel product={product} />}
      {section === "website" && (
        <WebsitePanel
          key={product.product_id}
          id={product.product_id}
          readOnly={product.lifecycle_status === "archived"}
          onChanged={() => {
            void load();
          }}
        />
      )}
      {section === "procurement" && (
        <ProductProcurementTab productId={product.product_id} />
      )}
      {section === "history" && <ProductHistoryPanel id={product.product_id} />}
      {!tabs.includes(section) && (
        <WorkspaceNotice>
          {t("catalog.sectionMissing")}{" "}
          <button
            className="suite-button"
            onClick={() => navigate(`/products/${id}/overview`)}
          >
            {t("catalog.overview")}
          </button>
        </WorkspaceNotice>
      )}
    </div>
  );
}
