DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE name IN ('customer.read','customer.create'));
DELETE FROM permissions WHERE name IN ('customer.read','customer.create');
