-- 0004_membership_roles.up.sql
-- يوسّع قائمة الأدوار لتشمل viewer.
-- ملاحظة: لا حاجة لـ memberships_active_role — لم يُضَف في 0001.
BEGIN;

ALTER TABLE memberships DROP CONSTRAINT IF EXISTS memberships_role_valid;

ALTER TABLE memberships ADD CONSTRAINT memberships_role_valid
    CHECK (role IN ('owner', 'admin', 'member', 'viewer'));

COMMIT;
