-- Migration: Insert Standard Response Playbooks
-- Description: Creates standard playbooks for common security scenarios

-- كتيب الاحتواء الفوري للبرمجيات الخبيثة
INSERT INTO response_playbooks (name, description, category, severity_filter, rule_pattern, commands, mitre_techniques) VALUES
('Malware Immediate Containment', 
 'الاحتواء الفوري للأنظمة المصابة بالبرمجيات الخبيثة مع عزل الشبكة وجمع الأدلة', 
 'containment', 
 ARRAY['critical', 'high'], 
 'malware|trojan|backdoor|ransomware', 
 '[
   {
     "type": "isolate_network",
     "timeout": 300,
     "description": "Isolate endpoint from network immediately",
     "on_failure": "stop"
   },
   {
     "type": "collect_forensics",
     "params": {"event_types": "process,file,network,dns,registry", "max_events": "1000"},
     "timeout": 900,
     "description": "Collect digital forensic evidence",
     "on_failure": "continue"
   },
   {
     "type": "quarantine_file",
     "params": {"file_path": "C:\\Windows\\Temp"},
     "timeout": 300,
     "description": "Quarantine suspicious file (override path as needed)",
     "on_failure": "continue"
   }
 ]'::jsonb,
 ARRAY['T1055', 'T1059', 'T1566']);

-- كتيب الاستجابة لهجمات الفدية
INSERT INTO response_playbooks (name, description, category, severity_filter, rule_pattern, commands, mitre_techniques) VALUES
('Ransomware Attack Response', 
 'الاستجابة الشاملة لهجمات الفدية مع الاحتواء والتحقيق وجمع الأدلة الكاملة', 
 'containment', 
 ARRAY['critical'], 
 'ransomware|encryption|file_encryption', 
 '[
   {
     "type": "isolate_network",
     "timeout": 300,
     "description": "Isolate endpoint immediately",
     "on_failure": "stop"
   },
   {
     "type": "terminate_process",
     "params": {"process_name": "vssadmin.exe", "kill_tree": "true"},
     "timeout": 300,
     "description": "Terminate ransomware process (override name as needed)",
     "on_failure": "continue"
   },
   {
     "type": "collect_forensics",
     "params": {"event_types": "process,file,registry", "max_events": "2000"},
     "timeout": 1200,
     "description": "Collect comprehensive forensic evidence",
     "on_failure": "continue"
   },
   {
     "type": "memory_dump",
     "timeout": 1800,
     "description": "Capture full memory snapshot",
     "on_failure": "continue"
   }
 ]'::jsonb,
 ARRAY['T1486', 'T1059', 'T1055']);

-- كتيب التحليل المتقدم للبرمجيات الخبيثة
INSERT INTO response_playbooks (name, description, category, severity_filter, rule_pattern, commands, mitre_techniques) VALUES
('Advanced Malware Analysis', 
 'التحليل المتعمق للبرمجيات الخبيثة المشتبه بها مع فحص شامل للنظام', 
 'investigation', 
 ARRAY['high', 'medium'], 
 'suspicious|malware_behavior|anomalous', 
 '[
   {
     "type": "collect_forensics",
     "params": {"event_types": "process,file,network,dns,registry", "max_events": "1500"},
     "timeout": 900,
     "description": "جمع الأدلة التفصيلية",
     "on_failure": "continue"
   },
   {
     "type": "process_tree_snapshot",
     "timeout": 600,
     "description": "أخذ لقطة شجرة العمليات",
     "on_failure": "continue"
   },
   {
     "type": "persistence_scan",
     "timeout": 900,
     "description": "فحص آليات الاستمرار",
     "on_failure": "continue"
   },
   {
     "type": "filesystem_timeline",
     "params": {"window_hours": "24"},
     "timeout": 600,
     "description": "إنشاء خط زمني للملفات",
     "on_failure": "continue"
   }
 ]'::jsonb,
 ARRAY['T1055', 'T1543', 'T1547']);

-- كتيب التحقيق في الوصول غير المصرح به
INSERT INTO response_playbooks (name, description, category, severity_filter, rule_pattern, commands, mitre_techniques) VALUES
('Unauthorized Access Investigation', 
 'التحقيق في محاولات الوصول غير المصرح به مع مراجعة شاملة للنظام', 
 'investigation', 
 ARRAY['high', 'medium'], 
 'unauthorized_access|privilege_escalation|lateral_movement', 
 '[
   {
     "type": "collect_forensics",
     "params": {"event_types": "process,network,authentication", "max_events": "1000"},
     "timeout": 600,
     "description": "جمع بيانات المصادقة",
     "on_failure": "continue"
   },
   {
     "type": "lsass_access_audit",
     "timeout": 300,
     "description": "مراجعة الوصول إلى LSASS",
     "on_failure": "continue"
   },
   {
     "type": "network_last_seen",
     "timeout": 300,
     "description": "آخر اتصالات الشبكة",
     "on_failure": "continue"
   },
   {
     "type": "agent_integrity_check",
     "timeout": 300,
     "description": "فحص تكامل الوكيل",
     "on_failure": "continue"
   }
 ]'::jsonb,
 ARRAY['T1078', 'T1110', 'T1003']);

-- كتيب إزالة البرمجيات الخبيثة واستعادة النظام
INSERT INTO response_playbooks (name, description, category, severity_filter, rule_pattern, commands, mitre_techniques) VALUES
('Malware Removal & Recovery', 
 'إزالة البرمجيات الخبيثة واستعادة النظام إلى الحالة الآمنة', 
 'remediation', 
 ARRAY['critical', 'high'], 
 'malware_detected|trojan_found|backdoor_identified', 
 '[
   {
     "type": "terminate_process",
     "params": {"process_name": "suspicious.exe", "kill_tree": "true"},
     "timeout": 300,
     "description": "Terminate malicious process (override name as needed)",
     "on_failure": "continue"
   },
   {
     "type": "quarantine_file",
     "params": {"file_path": "C:\\Windows\\Temp"},
     "timeout": 300,
     "description": "Quarantine malicious file (override path as needed)",
     "on_failure": "continue"
   },
   {
     "type": "update_signatures",
     "params": {"url": ""},
     "timeout": 600,
     "description": "Update threat signature database",
     "on_failure": "continue"
   },
   {
     "type": "unisolate_network",
     "timeout": 300,
     "description": "Restore network connectivity after remediation",
     "on_failure": "continue"
   }
 ]'::jsonb,
 ARRAY['T1059', 'T1055', 'T1566']);

-- كتيب التحقق من صحة النظام
INSERT INTO response_playbooks (name, description, category, severity_filter, rule_pattern, commands, mitre_techniques) VALUES
('System Validation Check', 
 'التحقق من صحة النظام بعد الاستجابة للتهديد', 
 'validation', 
 ARRAY['medium', 'low'], 
 'post_incident|validation|health_check', 
 '[
   {
     "type": "collect_forensics",
     "params": {"event_types": "process,system", "max_events": "500"},
     "timeout": 300,
     "description": "جمع بيانات صحة النظام",
     "on_failure": "continue"
   },
   {
     "type": "agent_integrity_check",
     "timeout": 300,
     "description": "فحص تكامل الوكيل",
     "on_failure": "continue"
   },
   {
     "type": "update_signatures",
     "params": {"url": "https://example.com/latest-signatures.ndjson"},
     "timeout": 600,
     "description": "تحديث التوقيعات",
     "on_failure": "continue"
   }
 ]'::jsonb,
 ARRAY['T1082', 'T1018']);
