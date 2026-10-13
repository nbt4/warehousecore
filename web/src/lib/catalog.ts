import { suiteTranslate } from './cores-design';
import { api } from "./api";

export interface CatalogProduct {
  version?: string;
  procurement_product_id?: number;
  manufacturer_name_input?: string;
  product_id: number;
  name: string;
  product_code: string;
  description?: string | null;
  category_id?: number | null;
  subcategory_id?: number | string | null;
  subbiercategory_id?: number | string | null;
  category_name?: string | null;
  subcategory_name?: string | null;
  subbiercategory_name?: string | null;
  manufacturer_id?: number | null;
  manufacturer_name?: string | null;
  brand_id?: number | null;
  brand_name?: string | null;
  product_type: "equipment" | "accessory" | "consumable";
  product_kind: "standard" | "cable" | "consumable" | "container" | "service";
  tracking_mode: "individual" | "quantity" | "none";
  lifecycle_status: "active" | "archived";
  model_number?: string | null;
  manufacturer_part_number?: string | null;
  ean?: string | null;
  generic_barcode?: string | null;
  attributes?: Record<string, unknown>;
  item_cost_per_day?: number | null;
  price_per_unit?: number | null;
  maintenance_interval?: number | null;
  weight?: number | null;
  height?: number | null;
  width?: number | null;
  depth?: number | null;
  power_consumption?: number | null;
  pos_in_category?: number | null;
  stock_quantity?: number | null;
  min_stock_level?: number | null;
  count_type_id?: number | null;
  count_type_abbr?: string;
  device_count?: number;
  is_accessory?: boolean;
  is_consumable?: boolean;
  website_visible?: boolean;
  website_featured?: boolean;
  website_thumbnail?: string | null;
  website_images?: string[];
}
export interface CatalogMetadata {
  categories: { category_id: number; name: string; abbreviation: string }[];
  subcategories: {
    subcategory_id: string | number;
    category_id: number;
    name: string;
    abbreviation: string;
  }[];
  thirdCategories: {
    subbiercategory_id: string | number;
    subcategory_id: string | number;
    name: string;
    abbreviation: string;
  }[];
  manufacturers: { manufacturer_id: number; name: string; website?: string }[];
  brands: { brand_id: number; manufacturer_id?: number; name: string }[];
  units: { count_type_id: number; name: string; abbreviation: string }[];
}
export async function loadCatalogMetadata(): Promise<CatalogMetadata> {
  const paths = [
    "categories",
    "subcategories",
    "subbiercategories",
    "manufacturers",
    "brands",
    "count-types",
  ];
  const results = await Promise.all(
    paths.map((path) => api.get(`/admin/${path}`)),
  );
  return {
    categories: results[0].data ?? [],
    subcategories: results[1].data ?? [],
    thirdCategories: results[2].data ?? [],
    manufacturers: results[3].data ?? [],
    brands: results[4].data ?? [],
    units: results[5].data ?? [],
  };
}
export function catalogError(error: unknown, fallback = suiteTranslate("Daten konnten nicht geladen werden.")): string {
  if (typeof error === "object" && error !== null && "response" in error) {
    const response = (error as { response?: { data?: { error?: string } } })
      .response;
    if (typeof response?.data?.error === "string") return response.data.error;
  }
  return fallback;
}
export function productCategory(product: CatalogProduct) {
  return [
    product.category_name,
    product.subcategory_name,
    product.subbiercategory_name,
  ]
    .filter(Boolean)
    .join(" › ");
}
