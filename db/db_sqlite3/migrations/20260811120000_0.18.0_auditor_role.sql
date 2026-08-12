-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds a read-only "auditor" role for accounts that need to review campaign
-- results without being able to run campaigns - reviewers, compliance staff,
-- or the client an engagement is run for.
--
-- No new permission is needed: the RBAC schema from 20190105192341_0.8.0_rbac
-- already splits view_objects from modify_objects, and the EnforceViewOnly
-- middleware already rejects every state-changing API request from an account
-- lacking modify_objects. Until now no shipped role exercised that path, since
-- both 'admin' and 'user' hold modify_objects. This migration only adds the
-- missing role, granting it view_objects alone.
INSERT INTO roles (slug, name, description)
VALUES ('auditor', 'Auditor', 'Read-only role with view access to objects and campaign results');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles AS r, permissions AS p
WHERE r.slug = 'auditor' AND p.slug = 'view_objects';

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
--
-- Any account still assigned to the auditor role is deliberately left with a
-- dangling role_id rather than being reassigned to 'user': a rollback must
-- never silently hand write access to accounts that were created read-only.
-- Such an account keeps no permissions at all and an admin has to reassign it
-- explicitly.
DELETE FROM role_permissions WHERE role_id = (SELECT id FROM roles WHERE slug = 'auditor');
DELETE FROM roles WHERE slug = 'auditor';
