import { useCatalogWriteAccess, useUnsavedChanges } from "./useCatalog";
import {
  useCallback,
  useEffect,
  useState,
  useRef,
  type FormEvent,
} from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api } from "../../lib/api";
import { catalogError, type CatalogProduct } from "../../lib/catalog";
import { WebsitePanel } from "./WebsitePanel";
import {
  WorkspaceField,
  WorkspaceHeader,
  WorkspaceLoading,
  WorkspaceNotice,
} from "./Workspace";
export interface CatalogPackage {
  package_id: number;
  package_code: string;
  name: string;
  description?: string | null;
  price?: number | null;
  category?: string | null;
  aliases?: string[];
  website_visible: boolean;
  is_active?: boolean;
  updated_at: string;
  version?: string;
  total_items: number;
  items?: {
    product_id: number;
    quantity: number;
    product_name?: string;
    is_optional?: boolean;
  }[];
}
export function PackageWorkspace() {
  const { packageId, section = "overview" } = useParams();
  const creating = packageId === "new";
  const navigate = useNavigate();
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [pkg, setPackage] = useState<CatalogPackage | null>(
    creating
      ? {
          package_id: 0,
          package_code: "",
          name: "",
          description: "",
          price: null,
          category: "",
          aliases: [],
          website_visible: false,
          is_active: true,
          updated_at: "",
          total_items: 0,
          items: [],
        }
      : null,
  );
  const [products, setProducts] = useState<CatalogProduct[]>([]);
  const [categories, setCategories] = useState<{ name: string }[]>([]);
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState("");
  const [original, setOriginal] = useState<string>(() =>
    pkg ? JSON.stringify(pkg) : "",
  );
  const releaseNavigationGuard = useUnsavedChanges(
    original !== "" && pkg !== null && JSON.stringify(pkg) !== original,
    t("catalog.confirmDiscard"),
    () => {
      if (original) setPackage(JSON.parse(original));
    },
  );
  const [loading, setLoading] = useState(!creating);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const route = useRef(packageId);
  const load = useCallback(async () => {
    if (creating || route.current !== packageId) return;
    setError("");
    try {
      const { data } = await api.get<CatalogPackage>(
        `/admin/product-packages/${packageId}`,
      );
      if (route.current !== packageId) return;
      setPackage(data);
      setOriginal(JSON.stringify(data));
    } catch (err) {
      if (route.current !== packageId) return;
      setError(catalogError(err));
    } finally {
      if (route.current === packageId) setLoading(false);
    }
  }, [creating, packageId]);
  useEffect(() => {
    route.current = packageId;
    if (!creating) {
      setPackage(null);
      setOriginal("");
      setLoading(true);
    }
    void load();
    return () => {
      route.current = undefined;
    };
  }, [load, creating, packageId]);
  useEffect(() => {
    let current = true;
    const timer = window.setTimeout(() => {
      void api
        .get<CatalogProduct[]>("/admin/products", {
          params: { search, limit: 50 },
        })
        .then(({ data }) => {
          if (current) setProducts(data);
        })
        .catch((err) => {
          if (current) setError(catalogError(err));
        });
    }, 250);
    return () => {
      current = false;
      window.clearTimeout(timer);
    };
  }, [search, t]);
  useEffect(() => {
    void api
      .get<{ name: string }[]>("/admin/categories")
      .then(({ data }) => setCategories(data ?? []))
      .catch(() => {});
  }, []);
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!pkg) return;
    setSaving(true);
    setError("");
    setMessage("");
    const payload = {
      ...pkg,
      expected_updated_at: pkg.version,
      items: pkg.items ?? [],
      website_visible: pkg.website_visible,
    };
    try {
      if (creating) {
        const { data } = await api.post<{ package_id: number }>(
          "/admin/product-packages",
          payload,
        );
        releaseNavigationGuard();
        navigate(`/products/packages/${data.package_id}/website`);
      } else {
        await api.put(`/admin/product-packages/${packageId}`, payload);
        setMessage(t("catalog.saved"));
        await load();
      }
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  const lifecycle = async () => {
    if (
      !pkg ||
      !window.confirm(
        t(
          pkg.is_active === false
            ? "catalog.confirmRestore"
            : "catalog.confirmPackageArchive",
        ),
      )
    )
      return;
    const operation = pkg.is_active === false ? "restore" : "archive";
    try {
      await api.post(`/admin/product-packages/${packageId}/${operation}`, {
        expected_updated_at: pkg.version,
        confirm_lifecycle: true,
        confirmation_text: `${operation.toUpperCase()} WAREHOUSE PACKAGE ${pkg.package_id}`,
      });
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    }
  };
  if (loading) return <WorkspaceLoading />;
  if (!pkg)
    return (
      <WorkspaceNotice error retry={() => void load()}>
        {error}
      </WorkspaceNotice>
    );
  return (
    <div className="space-y-5">
      <Link className="suite-button" to="/products/packages">
        ← {t("catalog.packages")}
      </Link>
      <WorkspaceHeader
        title={creating ? t("catalog.newPackage") : pkg.name}
        description={pkg.package_code}
      >
        {writable && !creating && (
          <button className="suite-button" onClick={() => void lifecycle()}>
            {t(pkg.is_active === false ? "catalog.restore" : "catalog.archive")}
          </button>
        )}
      </WorkspaceHeader>
      {!creating && (
        <nav
          className="flex flex-wrap gap-2"
          aria-label={t("catalog.packages")}
        >
          {["overview", "contents", "website"].map((tab) => (
            <Link
              className={`suite-button ${section === tab ? "suite-button--primary" : ""}`}
              key={tab}
              to={`/products/packages/${packageId}/${tab}`}
              aria-current={section === tab ? "page" : undefined}
            >
              {t(`catalog.${tab}`)}
            </Link>
          ))}
        </nav>
      )}
      {error && <WorkspaceNotice error>{error}</WorkspaceNotice>}
      {message && <WorkspaceNotice>{message}</WorkspaceNotice>}
      {section === "website" && !creating ? (
        <WebsitePanel
          kind="package"
          id={pkg.package_id}
          readOnly={pkg.is_active === false}
          onChanged={() => {
            void load();
          }}
        />
      ) : (
        <form className="space-y-5" onSubmit={(event) => void save(event)}>
          {(creating || section === "overview") && (
            <section className="suite-card p-5 space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <WorkspaceField label={t("catalog.name")}>
                  <input
                    required
                    value={pkg.name}
                    disabled={!writable || pkg.is_active === false || saving}
                    onChange={(event) =>
                      setPackage({ ...pkg, name: event.target.value })
                    }
                  />
                </WorkspaceField>
                <WorkspaceField label={t("catalog.dailyPrice")}>
                  <input
                    type="number"
                    min="0"
                    step="0.01"
                    value={pkg.price ?? ""}
                    disabled={!writable || pkg.is_active === false || saving}
                    onChange={(event) =>
                      setPackage({
                        ...pkg,
                        price:
                          event.target.value === ""
                            ? null
                            : Number(event.target.value),
                      })
                    }
                  />
                </WorkspaceField>
                <WorkspaceField label={t("catalog.category")}>
                  <select
                    value={pkg.category ?? ""}
                    disabled={!writable || pkg.is_active === false || saving}
                    onChange={(event) =>
                      setPackage({ ...pkg, category: event.target.value })
                    }
                  >
                    <option value="">{t("catalog.choose")}</option>
                    {pkg.category &&
                      !categories.some(
                        (category) => category.name === pkg.category,
                      ) && <option value={pkg.category}>{pkg.category}</option>}
                    {categories.map((category) => (
                      <option key={category.name} value={category.name}>
                        {category.name}
                      </option>
                    ))}
                  </select>
                </WorkspaceField>
                <WorkspaceField label={t("catalog.aliases")}>
                  <textarea
                    rows={3}
                    value={(pkg.aliases ?? []).join("\n")}
                    disabled={!writable || pkg.is_active === false || saving}
                    onChange={(event) =>
                      setPackage({
                        ...pkg,
                        aliases: event.target.value.split("\n"),
                      })
                    }
                  />
                </WorkspaceField>
              </div>
              <WorkspaceField label={t("catalog.description")}>
                <textarea
                  rows={4}
                  value={pkg.description ?? ""}
                  disabled={!writable || pkg.is_active === false || saving}
                  onChange={(event) =>
                    setPackage({ ...pkg, description: event.target.value })
                  }
                />
              </WorkspaceField>
            </section>
          )}
          {(creating || section === "contents") && (
            <section className="suite-card p-5 space-y-4">
              <h2 className="text-xl font-bold">{t("catalog.contents")}</h2>
              {writable && pkg.is_active !== false && (
                <div className="flex flex-wrap items-end gap-3">
                  <WorkspaceField label={t("catalog.search")}>
                    <input
                      value={search}
                      onChange={(event) => setSearch(event.target.value)}
                    />
                  </WorkspaceField>
                  <WorkspaceField label={t("catalog.product")}>
                    <select
                      value={selected}
                      onChange={(event) => setSelected(event.target.value)}
                    >
                      <option value="">{t("catalog.choose")}</option>
                      {products
                        .filter(
                          (product) =>
                            !(pkg.items ?? []).some(
                              (item) => item.product_id === product.product_id,
                            ),
                        )
                        .map((product) => (
                          <option
                            key={product.product_id}
                            value={product.product_id}
                          >
                            {product.name}
                          </option>
                        ))}
                    </select>
                  </WorkspaceField>
                  <button
                    type="button"
                    className="suite-button"
                    disabled={!selected}
                    onClick={() => {
                      const product = products.find(
                        (product) => product.product_id === Number(selected),
                      );
                      setPackage({
                        ...pkg,
                        items: [
                          ...(pkg.items ?? []),
                          {
                            product_id: Number(selected),
                            quantity: 1,
                            product_name: product?.name,
                          },
                        ],
                      });
                      setSelected("");
                    }}
                  >
                    {t("catalog.addProduct")}
                  </button>
                </div>
              )}
              <div className="suite-table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>{t("catalog.product")}</th>
                      <th>{t("catalog.quantityLabel")}</th>
                      <th>{t("catalog.optionalItem")}</th>
                      <th>{t("catalog.actions")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(pkg.items ?? []).map((item) => (
                      <tr key={item.product_id}>
                        <td>
                          <Link to={`/products/${item.product_id}/overview`}>
                            {item.product_name ||
                              products.find(
                                (product) =>
                                  product.product_id === item.product_id,
                              )?.name ||
                              item.product_id}
                          </Link>
                        </td>
                        <td>
                          <input
                            type="number"
                            aria-label={t("catalog.quantityLabel")}
                            min="1"
                            step="1"
                            value={item.quantity}
                            className="max-w-24"
                            disabled={
                              !writable || pkg.is_active === false || saving
                            }
                            onChange={(event) =>
                              setPackage({
                                ...pkg,
                                items: pkg.items?.map((entry) =>
                                  entry.product_id === item.product_id
                                    ? {
                                        ...entry,
                                        quantity: Number(event.target.value),
                                      }
                                    : entry,
                                ),
                              })
                            }
                          />
                        </td>
                        <td>
                          <input
                            type="checkbox"
                            aria-label={t("catalog.optionalItem")}
                            checked={item.is_optional ?? false}
                            disabled={!writable || pkg.is_active === false}
                            onChange={(event) =>
                              setPackage({
                                ...pkg,
                                items: pkg.items?.map((entry) =>
                                  entry.product_id === item.product_id
                                    ? {
                                        ...entry,
                                        is_optional: event.target.checked,
                                      }
                                    : entry,
                                ),
                              })
                            }
                          />
                        </td>
                        <td>
                          {writable && pkg.is_active !== false && (
                            <button
                              type="button"
                              className="suite-button"
                              onClick={() =>
                                setPackage({
                                  ...pkg,
                                  items: pkg.items?.filter(
                                    (entry) =>
                                      entry.product_id !== item.product_id,
                                  ),
                                })
                              }
                            >
                              {t("catalog.remove")}
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                    {!pkg.items?.length && (
                      <tr>
                        <td colSpan={4}>{t("catalog.noPackageContents")}</td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </section>
          )}
          {writable && pkg.is_active !== false && (
            <div className="flex justify-end">
              <button
                className="suite-button suite-button--primary"
                disabled={saving || !pkg.name.trim() || !pkg.items?.length}
              >
                {t(saving ? "catalog.saving" : "catalog.save")}
              </button>
            </div>
          )}
        </form>
      )}
    </div>
  );
}
