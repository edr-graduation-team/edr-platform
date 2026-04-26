-- Down migration: Remove EDR Automation Infrastructure

-- إزالة المشغلات
DROP TRIGGER IF EXISTS update_response_playbooks_updated_at ON response_playbooks;
DROP TRIGGER IF EXISTS update_automation_rules_updated_at ON automation_rules;
DROP TRIGGER IF EXISTS update_alert_stats_updated_at ON alert_stats;

-- إزالة الدالة
DROP FUNCTION IF EXISTS update_updated_at_column();

-- إزالة الجداول بالترتيب الصحيح
DROP TABLE IF EXISTS automation_metrics;
DROP TABLE IF EXISTS alert_stats;
DROP TABLE IF EXISTS playbook_suggestions;
DROP TABLE IF EXISTS playbook_executions;
DROP TABLE IF EXISTS automation_rules;
DROP TABLE IF EXISTS response_playbooks;
