import { useCatalogWriteAccess } from "./useCatalog";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Search } from "lucide-react";
import { api } from "../../lib/api";
import { catalogError } from "../../lib/catalog";
import {
  WorkspaceDialog,
  WorkspaceField,
  WorkspaceHeader,
  WorkspaceLoading,
  WorkspaceNotice,
} from "./Workspace";
interface MasterRecord {
  id: string | number;
  name: string;
  entity: string;
  parent_id?: string | number | null;
  parent_name?: string | null;
  abbreviation?: string | null;
  website?: string | null;
  gender?: string | null;
  is_decimal?: boolean;
  lifecycle_status: string;
  product_count: number;
  version: string;
}
const groups = [
  "categories",
  "manufacturers",
  "brands",
  "units",
  "cable-types",
  "connectors",
];
export function MasterDataPage() {
  const { group = "categories" } = useParams();
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [records, setRecords] = useState<MasterRecord[]>([]);
  const [parents, setParents] = useState<MasterRecord[]>([]);
  const [search, setSearch] = useState("");
  const [lifecycle, setLifecycle] = useState("active");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<Partial<MasterRecord> | null>(null);
  const [saving, setSaving] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const entities =
        group === "categories"
          ? ["categories", "subcategories", "third-categories"]
          : [group];
      const result = await Promise.all(
        entities.map(async (entity) => {
          const { data } = await api.get<MasterRecord[]>(
            `/admin/catalog-master/${entity}`,
            { params: { lifecycle_status: "all" } },
          );
          return data.map((record) => ({ ...record, entity }));
        }),
      );
      setRecords(result.flat());
      if (group === "brands") {
        const { data } = await api.get<MasterRecord[]>(
          "/admin/catalog-master/manufacturers",
        );
        setParents(data);
      }
    } catch (err) {
      setError(catalogError(err, t("catalog.loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [group, t]);
  useEffect(() => {
    void load();
    setEditor(null);
  }, [load]);
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!editor?.entity) return;
    setSaving(true);
    setError("");
    try {
      const payload = {
        ...editor,
        parent_id:
          editor.parent_id == null || editor.parent_id === ""
            ? null
            : String(editor.parent_id),
        expected_updated_at: editor.version,
      };
      if (editor.id != null)
        await api.put(
          `/admin/catalog-master/${editor.entity}/${encodeURIComponent(String(editor.id))}`,
          payload,
        );
      else await api.post(`/admin/catalog-master/${editor.entity}`, payload);
      setEditor(null);
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  const archive = async (record: MasterRecord) => {
    try {
      await api.post(
        `/admin/catalog-master/${record.entity}/${encodeURIComponent(String(record.id))}/${record.lifecycle_status === "archived" ? "restore" : "archive"}`,
        { expected_updated_at: record.version },
      );
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    }
  };
  const matches = (record: MasterRecord) =>
    (lifecycle === "all" || record.lifecycle_status === lifecycle) &&
    `${record.name} ${record.abbreviation ?? ""} ${record.parent_name ?? ""}`
      .toLocaleLowerCase()
      .includes(search.trim().toLocaleLowerCase());
  const childrenOf = (record: MasterRecord) =>
    records.filter(
      (child) =>
        child.entity ===
          (record.entity === "categories"
            ? "subcategories"
            : "third-categories") &&
        String(child.parent_id) === String(record.id),
    );
  const visible = (record: MasterRecord): boolean =>
    matches(record) ||
    (record.entity !== "third-categories" && childrenOf(record).some(visible));
  const actions = (record: MasterRecord) => (
    <div className="flex flex-wrap gap-2">
      <Link
        className="suite-button"
        to={`/products?${record.entity === "categories" ? "category_id" : record.entity === "subcategories" ? "subcategory_id" : "third_category_id"}=${encodeURIComponent(String(record.id))}`}
      >
        {t("catalog.viewProducts")} ({record.product_count})
      </Link>
      {writable && (
        <>
          {record.lifecycle_status !== "archived" && (
            <button
              className="suite-button"
              onClick={() => setEditor({ ...record })}
            >
              {t("catalog.edit")}
            </button>
          )}
          <button className="suite-button" onClick={() => void archive(record)}>
            {t(
              record.lifecycle_status === "archived"
                ? "catalog.restore"
                : "catalog.archive",
            )}
          </button>
          {record.entity !== "third-categories" &&
            record.lifecycle_status === "active" && (
              <button
                className="suite-button"
                onClick={() =>
                  setEditor({
                    entity:
                      record.entity === "categories"
                        ? "subcategories"
                        : "third-categories",
                    name: "",
                    abbreviation: "",
                    parent_id: record.id,
                  })
                }
              >
                {t("catalog.addChildCategory")}
              </button>
            )}
        </>
      )}
    </div>
  );
  const tree = (record: MasterRecord) => {
    if (!visible(record)) return null;
    const children =
      record.entity !== "third-categories" ? childrenOf(record) : [];
    return (
      <details open key={`${record.entity}-${record.id}`} className="py-3">
        <summary className="cursor-pointer font-semibold">
          {record.name}{" "}
          <span
            className="text-xs font-normal"
            style={{ color: "var(--text-secondary)" }}
          >
            {record.abbreviation} · {record.product_count}{" "}
            {t("catalog.catalog")}
            {record.lifecycle_status === "archived"
              ? ` · ${t("catalog.archived")}`
              : ""}
          </span>
        </summary>
        <div className="pt-3 space-y-2">
          {actions(record)}
          {children.length > 0 && (
            <div
              style={{
                borderInlineStart: "1px solid var(--border-default)",
                paddingInlineStart: "var(--space-4)",
              }}
            >
              {children.map(tree)}
            </div>
          )}
        </div>
      </details>
    );
  };
  const parentEntity =
    editor?.entity === "subcategories"
      ? "categories"
      : editor?.entity === "third-categories"
        ? "subcategories"
        : editor?.entity === "brands"
          ? "manufacturers"
          : "";
  const parentOptions =
    group === "brands"
      ? parents
      : records.filter(
          (record) =>
            record.entity === parentEntity &&
            record.lifecycle_status === "active",
        );
  return (
    <div className="space-y-5">
      <WorkspaceHeader
        title={t("catalog.masterData")}
        description={t("catalog.masterDataHelp")}
      />
      <nav
        className="flex flex-wrap gap-2"
        aria-label={t("catalog.masterData")}
      >
        {groups.map((entity) => (
          <Link
            key={entity}
            className={`suite-button ${group === entity ? "suite-button--primary" : ""}`}
            aria-current={group === entity ? "page" : undefined}
            to={`/products/master-data/${entity}`}
          >
            {t(`catalog.masterGroups.${entity}`)}
          </Link>
        ))}
      </nav>
      <div className="flex flex-wrap gap-3">
        <div className="suite-search-field flex-1">
          <Search size={18} />
          <input
            type="search"
            value={search}
            placeholder={t("catalog.searchMasterData")}
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
          <button
            className="suite-button suite-button--primary"
            onClick={() =>
              setEditor({ entity: group, name: "", abbreviation: "" })
            }
          >
            {t("catalog.newMasterRecord")}
          </button>
        )}
      </div>
      {error && (
        <WorkspaceNotice error retry={() => void load()}>
          {error}
        </WorkspaceNotice>
      )}
      {loading ? (
        <WorkspaceLoading />
      ) : group === "categories" ? (
        <section className="suite-card p-5">
          {records.filter((record) => record.entity === "categories").map(tree)}
          {!records.some(visible) && <p>{t("catalog.noneFound")}</p>}
        </section>
      ) : (
        <div className="suite-table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("catalog.name")}</th>
                <th>{t("catalog.details")}</th>
                <th>{t("catalog.usage")}</th>
                <th>{t("catalog.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {records.filter(matches).map((record) => (
                <tr key={record.id}>
                  <td>{record.name}</td>
                  <td>
                    {[record.abbreviation, record.parent_name, record.gender]
                      .filter(Boolean)
                      .join(" · ")}
                    {record.website && (
                      <a href={record.website} target="_blank" rel="noreferrer">
                        {record.website}
                      </a>
                    )}
                  </td>
                  <td>{record.product_count}</td>
                  <td>
                    {writable && (
                      <div className="flex flex-wrap gap-2">
                        {record.lifecycle_status !== "archived" && (
                          <button
                            className="suite-button"
                            onClick={() => setEditor({ ...record })}
                          >
                            {t("catalog.edit")}
                          </button>
                        )}
                        <button
                          className="suite-button"
                          onClick={() => void archive(record)}
                        >
                          {t(
                            record.lifecycle_status === "archived"
                              ? "catalog.restore"
                              : "catalog.archive",
                          )}
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
              {!records.some(matches) && (
                <tr>
                  <td colSpan={4}>{t("catalog.noneFound")}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
      {editor && (
        <WorkspaceDialog
          title={t(
            editor.id == null ? "catalog.newMasterRecord" : "catalog.edit",
          )}
          onClose={() => setEditor(null)}
        >
          <form className="space-y-4" onSubmit={(event) => void save(event)}>
            <WorkspaceField label={t("catalog.name")}>
              <input
                required
                maxLength={editor.entity === "units" ? 50 : 255}
                value={editor.name ?? ""}
                onChange={(event) =>
                  setEditor({ ...editor, name: event.target.value })
                }
              />
            </WorkspaceField>
            {!["manufacturers", "brands", "cable-types"].includes(
              editor.entity ?? "",
            ) && (
              <WorkspaceField label={t("catalog.abbreviation")}>
                <input
                  required={
                    editor.entity === "categories" || editor.entity === "units"
                  }
                  maxLength={
                    [
                      "categories",
                      "subcategories",
                      "third-categories",
                    ].includes(editor.entity ?? "")
                      ? 3
                      : 10
                  }
                  value={editor.abbreviation ?? ""}
                  onChange={(event) =>
                    setEditor({ ...editor, abbreviation: event.target.value })
                  }
                />
              </WorkspaceField>
            )}
            {parentEntity && (
              <WorkspaceField label={t("catalog.parentRecord")}>
                <select
                  required={editor.entity !== "brands"}
                  value={editor.parent_id ?? ""}
                  onChange={(event) =>
                    setEditor({
                      ...editor,
                      parent_id: event.target.value || null,
                    })
                  }
                >
                  <option value="">{t("catalog.choose")}</option>
                  {parentOptions.map((record) => (
                    <option key={record.id} value={record.id}>
                      {record.name}
                    </option>
                  ))}
                </select>
              </WorkspaceField>
            )}
            {editor.entity === "manufacturers" && (
              <WorkspaceField label={t("catalog.website")}>
                <input
                  type="url"
                  value={editor.website ?? ""}
                  onChange={(event) =>
                    setEditor({ ...editor, website: event.target.value })
                  }
                />
              </WorkspaceField>
            )}
            {editor.entity === "connectors" && (
              <WorkspaceField label={t("catalog.connectorGender")}>
                <input
                  maxLength={20}
                  value={editor.gender ?? ""}
                  onChange={(event) =>
                    setEditor({ ...editor, gender: event.target.value })
                  }
                />
              </WorkspaceField>
            )}
            {editor.entity === "units" && (
              <label className="flex gap-2 items-center">
                <input
                  type="checkbox"
                  checked={editor.is_decimal ?? false}
                  onChange={(event) =>
                    setEditor({ ...editor, is_decimal: event.target.checked })
                  }
                />
                {t("catalog.decimalUnit")}
              </label>
            )}
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
