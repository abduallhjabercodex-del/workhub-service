-- 0004_membership_roles.down.sql
BEGIN;

-- احذف كل صفوف viewer قبل تقليص الـ CHECK
DELETE FROM memberships WHERE role = 'viewer';

ALTER TABLE memberships DROP CONSTRAINT IF EXISTS memberships_role_valid;

ALTER TABLE memberships ADD CONSTRAINT memberships_role_valid
    CHECK (role IN ('owner', 'admin', 'member'));

COMMIT;
