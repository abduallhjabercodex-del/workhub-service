-- 0003_outbox.down.sql
BEGIN;

DROP TABLE IF EXISTS notification_deliveries CASCADE;
DROP TABLE IF EXISTS outbox_events           CASCADE;

COMMIT;
