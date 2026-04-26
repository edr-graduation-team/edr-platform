-- Migration: Insert Standard Automation Rules
-- Description: Creates intelligent automation rules for automatic threat response

-- قاعدة الاحتواء التلقائي للبرمجيات الخبيثة الحرجة
INSERT INTO automation_rules (name, description, trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes) VALUES
('Critical Malware Auto-Containment', 
 'الاحتواء التلقائي الفوري للبرمجيات الخبيثة الحرجة', 
 '{
   "severity": ["critical"],
   "rule_patterns": ["malware", "trojan", "backdoor", "ransomware"],
   "confidence_threshold": 0.8,
   "logic_operator": "AND"
 }'::jsonb,
 (SELECT id FROM response_playbooks WHERE name = 'Malware Immediate Containment'),
 1, -- أعلى أولوية
 true, -- تنفيذ تلقائي
 30);

-- قاعدة التحقيق التلقائي في الوصول المشبوه
INSERT INTO automation_rules (name, description, trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes) VALUES
('Suspicious Access Auto-Investigation', 
 'التحقيق التلقائي في محاولات الوصول المشبوهة', 
 '{
   "severity": ["high"],
   "rule_patterns": ["unauthorized_access", "privilege_escalation", "lateral_movement"],
   "confidence_threshold": 0.7,
   "logic_operator": "OR"
 }'::jsonb,
 (SELECT id FROM response_playbooks WHERE name = 'Unauthorized Access Investigation'),
 2,
 true,
 15);

-- قاعدة الاستجابة لهجمات الفدية
INSERT INTO automation_rules (name, description, trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes) VALUES
('Ransomware Emergency Response', 
 'الاستجابة الطارئة لهجمات الفدية', 
 '{
   "severity": ["critical"],
   "rule_patterns": ["ransomware", "encryption", "file_encryption"],
   "confidence_threshold": 0.9,
   "logic_operator": "OR"
 }'::jsonb,
 (SELECT id FROM response_playbooks WHERE name = 'Ransomware Attack Response'),
 1,
 true,
 0); -- لا فترة تبريد للطوارئ

-- قاعدة التحليل المتقدم للتهديدات المتوسطة
INSERT INTO automation_rules (name, description, trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes) VALUES
('Medium Threat Analysis', 
 'التحليل المتقدم للتهديدات متوسطة الشدة', 
 '{
   "severity": ["medium"],
   "rule_patterns": ["suspicious", "anomalous", "unusual_behavior"],
   "confidence_threshold": 0.6,
   "logic_operator": "OR"
 }'::jsonb,
 (SELECT id FROM response_playbooks WHERE name = 'Advanced Malware Analysis'),
 3,
 false, -- تنفيذ يدوي فقط
 60);

-- قاعدة إزالة البرمجيات الخبيثة التلقائية
INSERT INTO automation_rules (name, description, trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes) VALUES
('Malware Auto-Removal', 
 'إزالة تلقائية للبرمجيات الخبيثة المكتشفة', 
 '{
   "severity": ["critical", "high"],
   "rule_patterns": ["malware_detected", "trojan_found", "backdoor_identified"],
   "confidence_threshold": 0.85,
   "logic_operator": "AND"
 }'::jsonb,
 (SELECT id FROM response_playbooks WHERE name = 'Malware Removal & Recovery'),
 2,
 true,
 45);

-- قاعدة التحقق من صحة النظام بعد الحوادث
INSERT INTO automation_rules (name, description, trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes) VALUES
('Post-Incident Validation', 
 'التحقق التلقائي من صحة النظام بعد الاستجابة للحوادث', 
 '{
   "severity": ["medium", "low"],
   "rule_patterns": ["post_incident", "validation", "health_check"],
   "confidence_threshold": 0.5,
   "logic_operator": "OR"
 }'::jsonb,
 (SELECT id FROM response_playbooks WHERE name = 'System Validation Check'),
 4,
 true,
 120);
