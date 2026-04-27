-- Migration: 031_pos_in_category_trigger.sql
-- Description: Auto-assign pos_in_category on product insert when NULL
-- Each product within a subcategory gets a unique sequential position number,
-- which becomes part of the generated device ID (e.g. LGT5001 = LGT + pos5 + counter001)

DROP TRIGGER IF EXISTS products_before_insert_pos ON products;
DROP FUNCTION IF EXISTS assign_pos_in_category();

CREATE OR REPLACE FUNCTION assign_pos_in_category()
RETURNS TRIGGER AS $$
DECLARE
    next_pos INT;
BEGIN
    IF NEW.pos_in_category IS NULL AND NEW.subcategoryID IS NOT NULL THEN
        SELECT COALESCE(MAX(pos_in_category), 0) + 1
          INTO next_pos
          FROM products
         WHERE subcategoryID = NEW.subcategoryID;
        NEW.pos_in_category := next_pos;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER products_before_insert_pos
    BEFORE INSERT ON products
    FOR EACH ROW
    EXECUTE FUNCTION assign_pos_in_category();

-- Backfill existing NULL pos_in_category values per subcategory
DO $$
DECLARE
    rec RECORD;
    counter INT;
BEGIN
    FOR rec IN
        SELECT DISTINCT subcategoryID FROM products WHERE pos_in_category IS NULL AND subcategoryID IS NOT NULL
    LOOP
        counter := (SELECT COALESCE(MAX(pos_in_category), 0) FROM products WHERE subcategoryID = rec.subcategoryID AND pos_in_category IS NOT NULL);
        UPDATE products
           SET pos_in_category = sub.new_pos
          FROM (
              SELECT productID,
                     counter + ROW_NUMBER() OVER (ORDER BY productID) AS new_pos
                FROM products
               WHERE subcategoryID = rec.subcategoryID
                 AND pos_in_category IS NULL
          ) sub
         WHERE products.productID = sub.productID;
    END LOOP;
END;
$$;
