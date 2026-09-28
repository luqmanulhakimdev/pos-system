INSERT INTO permissions(name,description) VALUES
    ('customer.read','View customers'),
    ('customer.create','Create customers')
ON CONFLICT(name) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.name IN ('manager','cashier') AND p.name IN ('customer.read','customer.create')
ON CONFLICT DO NOTHING;
