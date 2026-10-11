INSERT INTO permissions(name,description) VALUES
    ('customer.update','Update customers'),
    ('customer.delete','Deactivate customers')
ON CONFLICT(name) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.name IN ('admin','manager') AND p.name IN ('customer.update','customer.delete')
ON CONFLICT DO NOTHING;
