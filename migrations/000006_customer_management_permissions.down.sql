DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name IN ('customer.update','customer.delete'));
DELETE FROM permissions WHERE name IN ('customer.update','customer.delete');
