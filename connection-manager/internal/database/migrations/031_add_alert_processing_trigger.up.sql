-- Migration: Add Alert Processing Trigger
-- Description: Creates trigger and function for automatic alert processing

-- دالة معالجة التنبيهات المتقدمة
CREATE OR REPLACE FUNCTION process_alert_on_create()
RETURNS TRIGGER AS $$
DECLARE
    alert_record RECORD;
    matching_rules INTEGER;
BEGIN
    -- الحصول على بيانات التنبيه الجديد
    SELECT * INTO alert_record FROM alerts WHERE id = NEW.id;
    
    -- تسجيل التنبيه للمعالجة
    RAISE NOTICE 'Processing alert %: severity=%, rule=%, agent=%', 
        NEW.id, NEW.severity, NEW.rule_name, NEW.agent_id;
    
    -- تحديث إحصائيات التنبيهات
    INSERT INTO alert_stats (date, alert_count, critical_count, high_count, medium_count, low_count)
    VALUES (CURRENT_DATE, 1, 
        CASE WHEN NEW.severity = 'critical' THEN 1 ELSE 0 END,
        CASE WHEN NEW.severity = 'high' THEN 1 ELSE 0 END,
        CASE WHEN NEW.severity = 'medium' THEN 1 ELSE 0 END,
        CASE WHEN NEW.severity = 'low' THEN 1 ELSE 0 END)
    ON CONFLICT (date) DO UPDATE SET
        alert_count = alert_stats.alert_count + 1,
        critical_count = alert_stats.critical_count + 
            CASE WHEN NEW.severity = 'critical' THEN 1 ELSE 0 END,
        high_count = alert_stats.high_count + 
            CASE WHEN NEW.severity = 'high' THEN 1 ELSE 0 END,
        medium_count = alert_stats.medium_count + 
            CASE WHEN NEW.severity = 'medium' THEN 1 ELSE 0 END,
        low_count = alert_stats.low_count + 
            CASE WHEN NEW.severity = 'low' THEN 1 ELSE 0 END,
        updated_at = now();
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة المشغل إلى جدول alerts الموجود
DROP TRIGGER IF EXISTS trigger_process_alert_creation ON alerts;
CREATE TRIGGER trigger_process_alert_creation
    AFTER INSERT ON alerts
    FOR EACH ROW
    EXECUTE FUNCTION process_alert_on_create();
