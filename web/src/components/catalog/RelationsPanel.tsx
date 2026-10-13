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
interface Relation {
  id: number;
  dependency_product_id: number;
  dependency_name: string;
  relation_type: string;
  assignment_scope: string;
  default_quantity: number;
  notes?: string;
  lifecycle_status: string;
  updated_at: string;
}
interface PreviewItem {
  product_id: number;
  name: string;
  quantity: number;
  unit: string;
  origins?: { name: string; quantity: number; relation_type: string }[];
}
const types = [
  "required",
  "recommended",
  "compatible",
  "consumes",
  "alternative",
  "included",
];
export function RelationsPanel({ product }: { product: CatalogProduct }) {
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [relations, setRelations] = useState<Relation[]>([]);
  const [available, setAvailable] = useState<CatalogProduct[]>([]);
  const [search, setSearch] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [editor, setEditor] = useState<Partial<Relation> | null>(null);
  const [saving, setSaving] = useState(false);
  const [quantity, setQuantity] = useState(1);
  const [cases, setCases] = useState(1);
  const [preview, setPreview] = useState<{
    items: PreviewItem[];
    warnings: string[];
  } | null>(null);
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const { data } = await api.get<Relation[]>(
        `/admin/products/${product.product_id}/dependencies`,
        { params: { lifecycle_status: "all" } },
      );
      setRelations(data);
    } catch (err) {
      setError(catalogError(err, t("catalog.loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [product.product_id, t]);
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (!editor || editor.id) return;
    let current = true;
    const timer = window.setTimeout(() => {
      void api
        .get<CatalogProduct[]>("/admin/products", {
          params: { search, limit: 50 },
        })
        .then(({ data }) => {
          if (current)
            setAvailable(
              data.filter((item) => item.product_id !== product.product_id),
            );
        })
        .catch((err) => {
          if (current) setError(catalogError(err, t("catalog.loadFailed")));
        });
    }, 250);
    return () => {
      current = false;
      window.clearTimeout(timer);
    };
  }, [editor, search, product.product_id, t]);
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!editor) return;
    setSaving(true);
    setError("");
    try {
      const payload = {
        dependency_product_id: editor.dependency_product_id,
        relation_type: editor.relation_type,
        assignment_scope: editor.assignment_scope,
        default_quantity: editor.default_quantity,
        notes: editor.notes || null,
        expectedUpdatedAt: editor.updated_at,
      };
      if (editor.id)
        await api.put(
          `/admin/products/${product.product_id}/dependencies/${editor.id}`,
          payload,
        );
      else
        await api.post(
          `/admin/products/${product.product_id}/dependencies`,
          payload,
        );
      setEditor(null);
      setPreview(null);
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  const lifecycle = async (relation: Relation) => {
    setError("");
    try {
      if (relation.lifecycle_status === "archived")
        await api.post(
          `/admin/products/${product.product_id}/dependencies/${relation.id}/restore`,
          { expectedUpdatedAt: relation.updated_at },
        );
      else
        await api.delete(
          `/admin/products/${product.product_id}/dependencies/${relation.id}`,
          { data: { expectedUpdatedAt: relation.updated_at } },
        );
      setPreview(null);
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    }
  };
  const showPreview = async () => {
    setError("");
    try {
      const { data } = await api.get<{
        items: PreviewItem[];
        warnings: string[];
      }>(`/admin/products/${product.product_id}/dependency-preview`, {
        params: { quantity, cases },
      });
      setPreview(data);
    } catch (err) {
      setError(catalogError(err, t("catalog.loadFailed")));
    }
  };
  return (
    <div className="space-y-5">
      {error && <WorkspaceNotice error>{error}</WorkspaceNotice>}
      <section className="suite-card p-5 flex flex-wrap items-center justify-between gap-3">
        <p
          className="text-sm max-w-2xl"
          style={{ color: "var(--text-secondary)" }}
        >
          {t("catalog.relationsHelp")}
        </p>
        {writable && product.lifecycle_status === "active" && (
          <button
            className="suite-button suite-button--primary"
            onClick={() => {
              setSearch("");
              setEditor({
                relation_type: "required",
                assignment_scope: "device",
                default_quantity: 1,
              });
            }}
          >
            {t("catalog.addRelation")}
          </button>
        )}
      </section>
      {loading ? (
        <WorkspaceLoading />
      ) : (
        <div className="suite-table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("catalog.product")}</th>
                <th>{t("catalog.relationType")}</th>
                <th>{t("catalog.quantityLabel")}</th>
                <th>{t("catalog.scope")}</th>
                <th>{t("catalog.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {relations.map((relation) => (
                <tr key={relation.id}>
                  <td>
                    <Link
                      to={`/products/${relation.dependency_product_id}/overview`}
                    >
                      {relation.dependency_name}
                    </Link>
                    {relation.notes && (
                      <p
                        className="text-xs"
                        style={{ color: "var(--text-secondary)" }}
                      >
                        {relation.notes}
                      </p>
                    )}
                  </td>
                  <td>
                    {t(`catalog.relation.${relation.relation_type}`)}
                    {relation.lifecycle_status === "archived" && (
                      <div className="text-xs">{t("catalog.archived")}</div>
                    )}
                  </td>
                  <td>{relation.default_quantity.toLocaleString()}</td>
                  <td>{t(`catalog.scopes.${relation.assignment_scope}`)}</td>
                  <td>
                    {writable && (
                      <div className="flex flex-wrap gap-2">
                        {relation.lifecycle_status !== "archived" && (
                          <button
                            className="suite-button"
                            onClick={() => setEditor({ ...relation })}
                          >
                            {t("catalog.edit")}
                          </button>
                        )}
                        <button
                          className="suite-button"
                          onClick={() => void lifecycle(relation)}
                        >
                          {t(
                            relation.lifecycle_status === "archived"
                              ? "catalog.restore"
                              : "catalog.archive",
                          )}
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
              {!relations.length && (
                <tr>
                  <td colSpan={5}>{t("catalog.noRelations")}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
      <section className="suite-card p-5 space-y-4">
        <h2 className="text-xl font-bold">{t("catalog.packingPreview")}</h2>
        <div className="flex flex-wrap items-end gap-4">
          <WorkspaceField label={t("catalog.quantityLabel")}>
            <input
              type="number"
              min="1"
              max="1000"
              step="1"
              value={quantity}
              onChange={(event) => setQuantity(Number(event.target.value))}
            />
          </WorkspaceField>
          <WorkspaceField label={t("catalog.numberOfCases")}>
            <input
              type="number"
              min="0"
              max="1000"
              step="1"
              value={cases}
              onChange={(event) => setCases(Number(event.target.value))}
            />
          </WorkspaceField>
          <button className="suite-button" onClick={() => void showPreview()}>
            {t("catalog.calculate")}
          </button>
        </div>
        {preview && (
          <>
            {preview.warnings.map((warning, index) => (
              <WorkspaceNotice key={index}>{warning}</WorkspaceNotice>
            ))}
            <div className="suite-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>{t("catalog.product")}</th>
                    <th>{t("catalog.quantityLabel")}</th>
                    <th>{t("catalog.source")}</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.items.map((item) => (
                    <tr key={item.product_id}>
                      <td>{item.name}</td>
                      <td>
                        {item.quantity.toLocaleString()} {item.unit}
                      </td>
                      <td>
                        {item.origins
                          ?.map(
                            (origin) =>
                              `${origin.name}: ${origin.quantity.toLocaleString()} (${t(`catalog.relation.${origin.relation_type}`)})`,
                          )
                          .join(" · ") || t("catalog.booking")}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </section>
      {editor && (
        <WorkspaceDialog
          title={t(editor.id ? "catalog.editRelation" : "catalog.addRelation")}
          onClose={() => setEditor(null)}
        >
          <form className="space-y-4" onSubmit={(event) => void save(event)}>
            {!editor.id && (
              <>
                <WorkspaceField label={t("catalog.search")}>
                  <input
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                  />
                </WorkspaceField>
                <WorkspaceField label={t("catalog.product")}>
                  <select
                    required
                    value={editor.dependency_product_id ?? ""}
                    onChange={(event) =>
                      setEditor({
                        ...editor,
                        dependency_product_id: Number(event.target.value),
                      })
                    }
                  >
                    <option value="">{t("catalog.choose")}</option>
                    {available
                      .filter(
                        (item) =>
                          !relations.some(
                            (relation) =>
                              relation.dependency_product_id ===
                              item.product_id,
                          ),
                      )
                      .map((item) => (
                        <option key={item.product_id} value={item.product_id}>
                          {item.name} · {item.product_code}
                        </option>
                      ))}
                  </select>
                </WorkspaceField>
              </>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <WorkspaceField label={t("catalog.relationType")}>
                <select
                  value={editor.relation_type}
                  onChange={(event) =>
                    setEditor({ ...editor, relation_type: event.target.value })
                  }
                >
                  {types.map((type) => (
                    <option key={type} value={type}>
                      {t(`catalog.relation.${type}`)}
                    </option>
                  ))}
                </select>
              </WorkspaceField>
              <WorkspaceField label={t("catalog.scope")}>
                <select
                  value={editor.assignment_scope}
                  onChange={(event) =>
                    setEditor({
                      ...editor,
                      assignment_scope: event.target.value,
                    })
                  }
                >
                  {["product", "device", "case"].map((scope) => (
                    <option key={scope} value={scope}>
                      {t(`catalog.scopes.${scope}`)}
                    </option>
                  ))}
                </select>
              </WorkspaceField>
              <WorkspaceField label={t("catalog.quantityLabel")}>
                <input
                  required
                  type="number"
                  min="0.01"
                  step="0.01"
                  value={editor.default_quantity ?? 1}
                  onChange={(event) =>
                    setEditor({
                      ...editor,
                      default_quantity: Number(event.target.value),
                    })
                  }
                />
              </WorkspaceField>
            </div>
            <WorkspaceField label={t("catalog.notes")}>
              <textarea
                rows={3}
                value={editor.notes ?? ""}
                onChange={(event) =>
                  setEditor({ ...editor, notes: event.target.value })
                }
              />
            </WorkspaceField>
            <p className="text-sm" style={{ color: "var(--text-secondary)" }}>
              {t(`catalog.relationHelp.${editor.relation_type}`)}
            </p>
            <button
              className="suite-button suite-button--primary"
              disabled={saving || (!editor.id && !editor.dependency_product_id)}
            >
              {t(saving ? "catalog.saving" : "catalog.save")}
            </button>
          </form>
        </WorkspaceDialog>
      )}
    </div>
  );
}
