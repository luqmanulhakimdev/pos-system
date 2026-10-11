DROP INDEX IF EXISTS customers_active_name_idx;
ALTER TABLE customers DROP COLUMN IF EXISTS active;
