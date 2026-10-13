import {
  useCatalogWriteAccess,
  useDebouncedValue,
} from "../catalog/useCatalog";
import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Search } from "lucide-react";
import { api } from "../../lib/api";
import { catalogError } from "../../lib/catalog";
import type { CatalogPackage } from "../catalog/PackageWorkspace";
import { WorkspaceLoading, WorkspaceNotice } from "../catalog/Workspace";
export function ProductPackagesTab() {
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [packages, setPackages] = useState<CatalogPackage[]>([]);
  const [search, setSearch] = useState("");
  const query = useDebouncedValue(search);
  const [lifecycle, setLifecycle] = useState("active");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const { data } = await api.get<CatalogPackage[]>(
        "/admin/product-packages",
        { params: { search: query, lifecycle_status: lifecycle } },
      );
      setPackages(data);
    } catch (err) {
      setError(catalogError(err, t("catalog.loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [query, lifecycle, t]);
  useEffect(() => {
    void load();
  }, [load]);
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <div className="suite-search-field flex-1">
          <Search size={18} />
          <input
            type="search"
            placeholder={t("catalog.searchPackages")}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        <select
          value={lifecycle}
          aria-label={t("catalog.lifecycle")}
          onChange={(event) => setLifecycle(event.target.value)}
        >
          {["active", "archived", "all"].map((value) => (
            <option key={value} value={value}>
              {t(`catalog.${value}`)}
            </option>
          ))}
        </select>
        {writable && (
          <Link
            className="suite-button suite-button--primary"
            to="/products/packages/new"
          >
            {t("catalog.newPackage")}
          </Link>
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
                <th>{t("catalog.name")}</th>
                <th>{t("catalog.contents")}</th>
                <th>{t("catalog.dailyPrice")}</th>
                <th>{t("catalog.website")}</th>
                <th>{t("catalog.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {packages.map((pkg) => (
                <tr key={pkg.package_id}>
                  <td>
                    <Link
                      className="font-semibold underline"
                      to={`/products/packages/${pkg.package_id}/overview`}
                    >
                      {pkg.name}
                    </Link>
                    <div
                      className="font-mono text-xs"
                      style={{ color: "var(--text-secondary)" }}
                    >
                      {pkg.package_code}
                    </div>
                  </td>
                  <td>{pkg.total_items}</td>
                  <td>
                    {pkg.price == null
                      ? "—"
                      : Number(pkg.price).toLocaleString(undefined, {
                          style: "currency",
                          currency: "EUR",
                        })}
                  </td>
                  <td>
                    <Link to={`/products/packages/${pkg.package_id}/website`}>
                      {t(
                        pkg.website_visible
                          ? "catalog.websiteState.published"
                          : "catalog.websiteState.draft",
                      )}
                    </Link>
                  </td>
                  <td>
                    <Link
                      className="suite-button"
                      to={`/products/packages/${pkg.package_id}/overview`}
                    >
                      {t("catalog.details")}
                    </Link>
                  </td>
                </tr>
              ))}
              {!packages.length && (
                <tr>
                  <td colSpan={5}>{t("catalog.noneFound")}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
