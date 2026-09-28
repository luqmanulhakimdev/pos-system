ALTER TABLE customers ADD COLUMN active BOOLEAN NOT NULL DEFAULT TRUE;
CREATE INDEX customers_active_name_idx ON customers(name,id) WHERE active=TRUE;
