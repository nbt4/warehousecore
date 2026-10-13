import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { Search } from "lucide-react";
import { api } from "../../lib/api";
import { catalogError } from "../../lib/catalog";
import {
  WorkspaceDialog,
  WorkspaceField,
  WorkspaceLoading,
  WorkspaceNotice,
} from "../catalog/Workspace";
import {
  useCatalogWriteAccess,
  useDebouncedValue,
} from "../catalog/useCatalog";
interface FieldDefinition {
  id: number;
  name: string;
  field_type: "text" | "number" | "dropdown";
  unit: string | null;
  dropdown_options: string | null;
  is_active: boolean;
}
interface FieldValue {
  field_definition_id: number;
  value: string;
  sort_order: number;
}
interface RentalEquipment {
  equipment_id?: number;
  product_name: string;
  supplier_name: string;
  rental_price: number;
  customer_price: number;
  category?: string | null;
  description?: string | null;
  notes?: string | null;
  is_active: boolean;
  updated_at?: string;
  field_values: FieldValue[];
}
const empty: RentalEquipment = {
  product_name: "",
  supplier_name: "",
  rental_price: 0,
  customer_price: 0,
  category: "",
  description: "",
  notes: "",
  is_active: true,
  field_values: [],
};
function fieldOptions(raw: string | null): string[] {
  try {
    const value: unknown = JSON.parse(raw ?? "[]");
    if (Array.isArray(value)) return value.map(String);
  } catch {
    /* Older definitions contain comma-separated values. */
  }
  return (raw ?? "")
    .split(",")
    .map((value) => value.trim())
    .filter(Boolean);
}
export function RentedProductsTab() {
  const { t } = useTranslation();
  const writable = useCatalogWriteAccess();
  const [items, setItems] = useState<RentalEquipment[]>([]);
  const [suppliers, setSuppliers] = useState<string[]>([]);
  const [categories, setCategories] = useState<string[]>([]);
  const [fields, setFields] = useState<FieldDefinition[]>([]);
  const [contacts, setContacts] = useState<
    { id: number; display_name: string }[]
  >([]);
  const [search, setSearch] = useState("");
  const query = useDebouncedValue(search);
  const [supplier, setSupplier] = useState("");
  const [lifecycle, setLifecycle] = useState("active");
  const [editor, setEditor] = useState<RentalEquipment | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const supplierQuery = useDebouncedValue(editor?.supplier_name ?? "");
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const { data } = await api.get<RentalEquipment[]>(
        "/admin/rental-equipment",
        { params: { search: query, supplier: supplier || undefined } },
      );
      setItems(data ?? []);
    } catch (err) {
      setError(catalogError(err, t("catalog.loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [query, supplier, t]);
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    let current = true;
    void Promise.all([
      api.get<string[]>("/admin/rental-equipment/suppliers"),
      api.get<FieldDefinition[]>(
        "/admin/rental-field-definitions?active_only=true",
      ),
      api.get<{ name: string }[]>("/admin/categories"),
    ])
      .then(([a, b, c]) => {
        if (current) {
          setSuppliers(a.data ?? []);
          setFields(b.data ?? []);
          setCategories(c.data.map((item) => item.name));
        }
      })
      .catch((err) => {
        if (current) setError(catalogError(err, t("catalog.loadFailed")));
      });
    return () => {
      current = false;
    };
  }, [t]);
  useEffect(() => {
    const abort = new AbortController();
    if (supplierQuery.trim().length < 2) {
      setContacts([]);
      return;
    }
    void api
      .get<{ id: number; display_name: string }[]>(
        "/admin/rental-equipment/supplier-contacts",
        { params: { q: supplierQuery }, signal: abort.signal },
      )
      .then(({ data }) => setContacts(data ?? []))
      .catch(() => {});
    return () => abort.abort();
  }, [supplierQuery]);
  const categoryOptions = useMemo(
    () =>
      [
        ...new Set([
          ...categories,
          ...items
            .map((item) => item.category)
            .filter((name): name is string => Boolean(name)),
        ]),
      ].sort(),
    [categories, items],
  );
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!editor || !writable) return;
    setSaving(true);
    setError("");
    try {
      const payload = {
        ...editor,
        product_name: editor.product_name.trim(),
        supplier_name: editor.supplier_name.trim(),
        expected_updated_at: editor.updated_at,
      };
      if (editor.equipment_id)
        await api.put(
          `/admin/rental-equipment/${editor.equipment_id}`,
          payload,
        );
      else await api.post("/admin/rental-equipment", payload);
      setEditor(null);
      await load();
    } catch (err) {
      setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      setSaving(false);
    }
  };
  const setField = (definition: number, value: string) => {
    if (!editor) return;
    setEditor({
      ...editor,
      field_values: [
        ...editor.field_values.filter(
          (field) => field.field_definition_id !== definition,
        ),
        { field_definition_id: definition, value, sort_order: 0 },
      ],
    });
  };
  const visible = items.filter(
    (item) =>
      lifecycle === "all" || (lifecycle === "active") === item.is_active,
  );
  return (
    <div className="space-y-5">
      <WorkspaceNotice>{t("catalog.externalHelp")}</WorkspaceNotice>
      <div className="flex flex-wrap gap-3">
        <div className="suite-search-field flex-1">
          <Search size={18} />
          <input
            type="search"
            placeholder={t("catalog.externalSearch")}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        <select
          value={supplier}
          aria-label={t("catalog.supplier")}
          onChange={(event) => setSupplier(event.target.value)}
        >
          <option value="">{t("catalog.allSuppliers")}</option>
          {suppliers.map((name) => (
            <option key={name}>{name}</option>
          ))}
        </select>
        <select
          value={lifecycle}
          aria-label={t("catalog.lifecycle")}
          onChange={(event) => setLifecycle(event.target.value)}
        >
          {["active", "archived", "all"].map((value) => (
            <option value={value} key={value}>
              {t(`catalog.${value}`)}
            </option>
          ))}
        </select>
        {writable && (
          <button
            className="suite-button suite-button--primary"
            onClick={() => setEditor({ ...empty })}
          >
            {t("catalog.newExternal")}
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
      ) : (
        <div className="suite-table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("catalog.product")}</th>
                <th>{t("catalog.supplier")}</th>
                <th>{t("catalog.category")}</th>
                <th>{t("catalog.purchaseRentalPrice")}</th>
                <th>{t("catalog.customerPrice")}</th>
                <th>{t("catalog.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((item) => (
                <tr key={item.equipment_id}>
                  <td>
                    {item.product_name}
                    {!item.is_active && (
                      <div className="text-xs">{t("catalog.archived")}</div>
                    )}
                  </td>
                  <td>{item.supplier_name}</td>
                  <td>{item.category ?? "—"}</td>
                  <td>
                    {item.rental_price.toLocaleString(undefined, {
                      style: "currency",
                      currency: "EUR",
                    })}
                  </td>
                  <td>
                    {item.customer_price.toLocaleString(undefined, {
                      style: "currency",
                      currency: "EUR",
                    })}
                  </td>
                  <td>
                    <button
                      className="suite-button"
                      onClick={() =>
                        setEditor({
                          ...item,
                          field_values: item.field_values ?? [],
                        })
                      }
                    >
                      {t(writable ? "catalog.edit" : "catalog.details")}
                    </button>
                  </td>
                </tr>
              ))}
              {!visible.length && (
                <tr>
                  <td colSpan={6}>{t("catalog.noneFound")}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
      {editor && (
        <WorkspaceDialog
          title={t(
            editor.equipment_id ? "catalog.external" : "catalog.newExternal",
          )}
          onClose={() => {
            if (!saving) setEditor(null);
          }}
        >
          <form className="space-y-4" onSubmit={(event) => void save(event)}>
            <fieldset
              disabled={!writable || saving}
              className="grid gap-4 sm:grid-cols-2"
            >
              <WorkspaceField label={t("catalog.name")}>
                <input
                  required
                  maxLength={255}
                  value={editor.product_name}
                  onChange={(event) =>
                    setEditor({ ...editor, product_name: event.target.value })
                  }
                />
              </WorkspaceField>
              <WorkspaceField
                label={t("catalog.supplier")}
                hint={t("catalog.supplierHelp")}
              >
                <input
                  list="catalog-supplier-contacts"
                  required
                  value={editor.supplier_name}
                  onChange={(event) =>
                    setEditor({ ...editor, supplier_name: event.target.value })
                  }
                />
                <datalist id="catalog-supplier-contacts">
                  {[
                    ...new Set([
                      ...suppliers,
                      ...contacts.map((item) => item.display_name),
                    ]),
                  ].map((name) => (
                    <option key={name} value={name} />
                  ))}
                </datalist>
              </WorkspaceField>
              <WorkspaceField label={t("catalog.category")}>
                <select
                  value={editor.category ?? ""}
                  onChange={(event) =>
                    setEditor({ ...editor, category: event.target.value })
                  }
                >
                  <option value="">{t("catalog.noCategory")}</option>
                  {categoryOptions.map((name) => (
                    <option key={name}>{name}</option>
                  ))}
                </select>
              </WorkspaceField>
              {(["rental_price", "customer_price"] as const).map((key) => (
                <WorkspaceField
                  label={t(
                    key === "rental_price"
                      ? "catalog.purchaseRentalPrice"
                      : "catalog.customerPrice",
                  )}
                  key={key}
                >
                  <input
                    required
                    type="number"
                    min="0"
                    step="0.01"
                    value={editor[key]}
                    onChange={(event) =>
                      setEditor({
                        ...editor,
                        [key]: Number(event.target.value),
                      })
                    }
                  />
                </WorkspaceField>
              ))}
              <WorkspaceField label={t("catalog.lifecycle")}>
                <select
                  value={editor.is_active ? "active" : "archived"}
                  onChange={(event) =>
                    setEditor({
                      ...editor,
                      is_active: event.target.value === "active",
                    })
                  }
                >
                  <option value="active">{t("catalog.active")}</option>
                  <option value="archived">{t("catalog.archived")}</option>
                </select>
              </WorkspaceField>
              <WorkspaceField label={t("catalog.description")}>
                <textarea
                  value={editor.description ?? ""}
                  onChange={(event) =>
                    setEditor({ ...editor, description: event.target.value })
                  }
                />
              </WorkspaceField>
              <WorkspaceField label={t("catalog.notes")}>
                <textarea
                  value={editor.notes ?? ""}
                  onChange={(event) =>
                    setEditor({ ...editor, notes: event.target.value })
                  }
                />
              </WorkspaceField>
              {fields.map((field) => (
                <WorkspaceField
                  key={field.id}
                  label={`${field.name}${field.unit ? ` (${field.unit})` : ""}`}
                >
                  {field.field_type === "dropdown" ? (
                    <select
                      value={
                        editor.field_values.find(
                          (value) => value.field_definition_id === field.id,
                        )?.value ?? ""
                      }
                      onChange={(event) =>
                        setField(field.id, event.target.value)
                      }
                    >
                      <option value="">{t("catalog.choose")}</option>
                      {fieldOptions(field.dropdown_options).map((option) => (
                        <option key={option}>{option}</option>
                      ))}
                    </select>
                  ) : (
                    <input
                      type={field.field_type === "number" ? "number" : "text"}
                      step="any"
                      value={
                        editor.field_values.find(
                          (value) => value.field_definition_id === field.id,
                        )?.value ?? ""
                      }
                      onChange={(event) =>
                        setField(field.id, event.target.value)
                      }
                    />
                  )}
                </WorkspaceField>
              ))}
            </fieldset>
            {error && <WorkspaceNotice error>{error}</WorkspaceNotice>}
            {writable && (
              <button
                className="suite-button suite-button--primary"
                disabled={saving}
              >
                {t(saving ? "catalog.saving" : "catalog.save")}
              </button>
            )}
          </form>
        </WorkspaceDialog>
      )}
    </div>
  );
}
