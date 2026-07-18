# Product Packages OCR Integration Notes

## Overview
Product Packages have been implemented in WarehouseCore with full CRUD functionality. The OCR integration for job creation mentioned in legacy tracker issue #19 requires implementation in **RentalCore**.

## What's Implemented in WarehouseCore

### Database Schema
- `product_packages` table - stores package information (ID, package_code, name, description, price)
- `product_package_items` table - links products to packages with quantities
- `product_package_aliases` table - stores OCR keywords/aliases per package for fast mapping
- Migrations: `019_create_product_packages.sql` and `020_add_package_codes_and_aliases.sql`

### Backend API Endpoints
All admin endpoints require authentication (`/api/v1/admin/...`):

**Read Operations:**
- `GET /admin/product-packages` - List all product packages (supports search parameter)
- `GET /admin/product-packages/{id}` - Get detailed package info with items

**Write Operations:**
- `POST /admin/product-packages` - Create new package
- `PUT /admin/product-packages/{id}` - Update existing package
- `DELETE /admin/product-packages/{id}` - Delete package
- `POST /admin/product-packages/{id}/items` - Add item to package
- `DELETE /admin/product-packages/{package_id}/items/{item_id}` - Remove item from package

**Alias / OCR Mapping Endpoint (authenticated)**:
- `GET /api/v1/product-packages/alias-map` - Returns flat list of `{ alias, package_id, package_code, package_name, price }`

### Frontend UI
- Location: Products page (`/products`) with "Produktpakete" tab
- Features:
  - List all packages with search
  - Create/edit packages with product selection
  - View package details
  - Delete packages
  - Add/remove products from packages
  - Maintain OCR keyword/alias list per package (used by alias-map endpoint)

## Required RentalCore Integration

### OCR Job Creation Mapping
The OCR job creation feature in RentalCore should be extended to support mapping scanned items to product packages:

1. **Database Access / API**:
   - RentalCore shares the same MySQL database, so it can read from `product_packages`, `product_package_items`, and `product_package_aliases`.
   - Alternatively, call WarehouseCore's authenticated endpoint `GET /api/v1/product-packages/alias-map` to obtain all alias-to-package mappings in one payload (recommended for OCR services).

3. **UI Integration**: During OCR job creation in RentalCore:
   - Show option to select product packages (in addition to individual products)
   - When a package is selected, automatically add all its products with specified quantities
   - Display package price if available

### Suggested Implementation Approach

1. Add a product packages selector in RentalCore's job creation form
2. Query available packages from `product_packages` table (or `GET /admin/product-packages`)
3. When a package is selected (or matched via alias), expand it to individual items:
   ```sql
   SELECT ppi.product_id, ppi.quantity, p.name, pp.price
   FROM product_package_items ppi
   JOIN products p ON ppi.product_id = p.productID
   JOIN product_packages pp ON ppi.package_id = pp.package_id
   WHERE ppi.package_id = ?
   ```
4. Add these items to the job with the package reference for tracking
5. During OCR parsing, compare normalized text tokens against `product_package_aliases.alias` values (or the alias-map endpoint) to resolve a package.

### Database Compatibility
Since both cores use the same MySQL database (`RentalCore`), no additional synchronization is needed. Changes made in WarehouseCore are immediately available to RentalCore.

## Future Enhancements

1. **Package Templates**: Create common package templates for frequent job types
2. **Package Pricing**: Use package price for job calculations instead of individual item prices
3. **Package History**: Track which jobs used which packages
4. **OCR Recognition**: Train OCR to recognize package codes/names directly
5. **Barcode Support**: Add optional barcode field to packages for quick scanning
