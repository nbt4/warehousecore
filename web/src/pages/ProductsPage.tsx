import { useEffect, useState } from "react";
import { api } from "../lib/api";
import { catalogError, type CatalogProduct } from "../lib/catalog";
import {
  WorkspaceNotice,
  WorkspaceLoading,
} from "../components/catalog/Workspace";
import {
  Link,
  Navigate,
  Route,
  Routes,
  useNavigate,
  useSearchParams,
} from "react-router-dom";
import { useTranslation } from "react-i18next";
import { CatalogList } from "../components/catalog/CatalogList";
import { ProductWorkspace } from "../components/catalog/ProductWorkspace";
import { ProductForm } from "../components/catalog/ProductForm";
import { DevicesPanel } from "../components/catalog/DevicesPanel";
import { MasterDataPage } from "../components/catalog/MasterDataPage";
import { PackageWorkspace } from "../components/catalog/PackageWorkspace";
import { ProductPackagesTab } from "../components/admin/ProductPackagesTab";
import { ProductProcurementTab } from "../components/admin/ProductProcurementTab";
import { RentedProductsTab } from "../components/admin/RentedProductsTab";
import { WorkspaceHeader } from "../components/catalog/Workspace";
import "../components/catalog/catalog-workspace.css";

function NewProductPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const procurementId = params.get("procurement_product_id");
  const [prefill, setPrefill] = useState<Partial<CatalogProduct> | undefined>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(Boolean(procurementId));
  useEffect(() => {
    if (!procurementId) return;
    const abort = new AbortController();
    void api
      .get<Partial<CatalogProduct>>(
        `/admin/procurement/products/${procurementId}/import-preview`,
        { signal: abort.signal },
      )
      .then(({ data }) => setPrefill(data))
      .catch((err) => {
        if (!abort.signal.aborted)
          setError(catalogError(err, t("catalog.loadFailed")));
      })
      .finally(() => {
        if (!abort.signal.aborted) setLoading(false);
      });
    return () => abort.abort();
  }, [procurementId, t]);
  return (
    <div className="space-y-5">
      <Link className="suite-button" to="/products">
        ← {t("catalog.backToCatalog")}
      </Link>
      <WorkspaceHeader title={t("catalog.newProduct")} />
      {error ? (
        <WorkspaceNotice error>{error}</WorkspaceNotice>
      ) : loading ? (
        <WorkspaceLoading />
      ) : (
        <ProductForm
          key={procurementId ?? "new"}
          prefill={prefill}
          onSaved={(id) => navigate(`/products/${id}/stock`)}
          onCancel={() => navigate("/products")}
        />
      )}
    </div>
  );
}
function CatalogSection({
  section,
  children,
}: {
  section: string;
  children: React.ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-5">
      <WorkspaceHeader title={t(`catalog.${section}`)} />
      {children}
    </div>
  );
}
export function ProductsPage() {
  return (
    <div
      className="suite-page catalog-workspace space-y-6"
      style={{ color: "var(--text-primary)" }}
    >
      <Routes>
        <Route index element={<CatalogList />} />
        <Route path="catalog" element={<Navigate to="/products" replace />} />
        <Route path="new" element={<NewProductPage />} />
        <Route
          path="devices"
          element={
            <CatalogSection section="devices">
              <DevicesPanel />
            </CatalogSection>
          }
        />
        <Route path="website" element={<CatalogList websiteOnly />} />
        <Route
          path="procurement"
          element={
            <CatalogSection section="procurement">
              <ProductProcurementTab />
            </CatalogSection>
          }
        />
        <Route
          path="packages"
          element={
            <CatalogSection section="packages">
              <ProductPackagesTab />
            </CatalogSection>
          }
        />
        <Route
          path="packages/:packageId/:section?"
          element={<PackageWorkspace />}
        />
        <Route path="master-data/:group?" element={<MasterDataPage />} />
        <Route
          path="external"
          element={
            <CatalogSection section="external">
              <RentedProductsTab />
            </CatalogSection>
          }
        />
        <Route path=":id/:section?" element={<ProductWorkspace />} />
      </Routes>
    </div>
  );
}
