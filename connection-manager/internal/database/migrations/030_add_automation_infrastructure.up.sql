-- Migration: Add EDR Automation Infrastructure
-- Description: Creates tables for response playbooks, automation rules, executions, suggestions, and metrics

-- جدول كتيبات الإجراءات القياسية
CREATE TABLE IF NOT EXISTS response_playbooks (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    name varchar(255) NOT NULL,
    description text,
    category varchar(100) NOT NULL CHECK (category IN ('containment', 'investigation', 'remediation', 'validation')),
    severity_filter varchar[] DEFAULT '{}',
    rule_pattern varchar(255),
    commands jsonb NOT NULL DEFAULT '[]'::jsonb,
    mitre_techniques varchar[] DEFAULT '{}',
    enabled boolean DEFAULT true,
    created_by uuid REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- جدول قواعد الأتمتة الذكية
CREATE TABLE IF NOT EXISTS automation_rules (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    name varchar(255) NOT NULL,
    description text,
    trigger_conditions jsonb NOT NULL DEFAULT '{}'::jsonb,
    playbook_id uuid NOT NULL REFERENCES response_playbooks(id) ON DELETE CASCADE,
    priority integer DEFAULT 100,
    auto_execute boolean DEFAULT false,
    cooldown_minutes integer DEFAULT 30,
    enabled boolean DEFAULT true,
    success_rate float DEFAULT 0.0,
    last_execution TIMESTAMPTZ,
    created_by uuid REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- جدول تنفيذات كتيبات الإجراءات
CREATE TABLE IF NOT EXISTS playbook_executions (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    alert_id uuid NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    playbook_id uuid NOT NULL REFERENCES response_playbooks(id) ON DELETE CASCADE,
    rule_id uuid REFERENCES automation_rules(id) ON DELETE SET NULL,
    agent_id uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    status varchar(50) DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    started_at TIMESTAMPTZ DEFAULT now(),
    completed_at TIMESTAMPTZ,
    commands_executed integer DEFAULT 0,
    commands_total integer DEFAULT 0,
    result jsonb DEFAULT '{}'::jsonb,
    error_message text,
    created_by uuid REFERENCES users(id),
    execution_time_ms integer
);

-- جدول اقتراحات كتيبات الإجراءات الذكية
CREATE TABLE IF NOT EXISTS playbook_suggestions (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    alert_id uuid NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    playbook_id uuid NOT NULL REFERENCES response_playbooks(id) ON DELETE CASCADE,
    confidence float CHECK (confidence >= 0 AND confidence <= 1),
    reason text,
    mitre_match varchar[],
    created_at TIMESTAMPTZ DEFAULT now()
);

-- جدول إحصائيات الأتمتة
CREATE TABLE IF NOT EXISTS automation_metrics (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    rule_id uuid REFERENCES automation_rules(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    executions_count integer DEFAULT 0,
    successful_executions integer DEFAULT 0,
    failed_executions integer DEFAULT 0,
    avg_execution_time_ms integer DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- جدول إحصائيات التنبيهات
CREATE TABLE IF NOT EXISTS alert_stats (
    date DATE PRIMARY KEY,
    alert_count integer DEFAULT 0,
    critical_count integer DEFAULT 0,
    high_count integer DEFAULT 0,
    medium_count integer DEFAULT 0,
    low_count integer DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- فهارس محسنة للأداء
CREATE INDEX IF NOT EXISTS idx_automation_rules_enabled_priority ON automation_rules(enabled, priority);
CREATE INDEX IF NOT EXISTS idx_playbook_executions_alert_id ON playbook_executions(alert_id);
CREATE INDEX IF NOT EXISTS idx_playbook_executions_status ON playbook_executions(status);
CREATE INDEX IF NOT EXISTS idx_playbook_suggestions_alert_id ON playbook_suggestions(alert_id);
CREATE INDEX IF NOT EXISTS idx_response_playbooks_enabled ON response_playbooks(enabled);
CREATE INDEX IF NOT EXISTS idx_response_playbooks_category ON response_playbooks(category);
CREATE INDEX IF NOT EXISTS idx_automation_metrics_date ON automation_metrics(date);
CREATE INDEX IF NOT EXISTS idx_automation_metrics_rule_id ON automation_metrics(rule_id);
CREATE INDEX IF NOT EXISTS idx_playbook_executions_agent_id ON playbook_executions(agent_id);
CREATE INDEX IF NOT EXISTS idx_playbook_executions_playbook_id ON playbook_executions(playbook_id);

-- إنشاء مشغل لتحديث updated_at تلقائياً
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- إضافة المشغل لجدول response_playbooks
CREATE TRIGGER update_response_playbooks_updated_at 
    BEFORE UPDATE ON response_playbooks 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- إضافة المشغل لجدول automation_rules
CREATE TRIGGER update_automation_rules_updated_at 
    BEFORE UPDATE ON automation_rules 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- إضافة المشغل لجدول alert_stats
CREATE TRIGGER update_alert_stats_updated_at 
    BEFORE UPDATE ON alert_stats 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();
