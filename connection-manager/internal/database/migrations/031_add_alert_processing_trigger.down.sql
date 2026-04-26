-- Down migration: Remove Alert Processing Trigger

-- إزالة المشغل
DROP TRIGGER IF EXISTS trigger_process_alert_creation ON alerts;

-- إزالة الدالة
DROP FUNCTION IF EXISTS process_alert_on_create();
