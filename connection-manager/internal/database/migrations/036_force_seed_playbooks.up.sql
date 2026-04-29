-- Migration 036: Ensure default playbooks and rules exist
-- Uses ON CONFLICT DO NOTHING so it is safe to re-run

INSERT INTO response_playbooks (id, name, description, category, commands, mitre_techniques, enabled, created_at, updated_at) VALUES
('11111111-1111-1111-1111-111111111111',
 'Ransomware Immediate Containment',
 'Isolates the host from the network, terminates suspected encryption processes (like vssadmin or unknown encrypters), and captures a forensic memory dump.',
 'containment',
 '[
    {"type": "isolate_network", "timeout": 30, "description": "Isolate agent from network (keep C2 channel open)"},
    {"type": "terminate_process", "timeout": 45, "description": "Terminate processes matching ransomware behavior heuristics"},
    {"type": "collect_forensics", "timeout": 300, "description": "Capture volatile memory snapshot for analysis"}
 ]'::jsonb,
 ARRAY['T1486', 'T1490'],
 true, NOW(), NOW()),

('22222222-2222-2222-2222-222222222222',
 'Unauthorized USB Device Response',
 'Automatically unmounts unauthorized mass storage devices and pulls recent file system logs to track potential exfiltration.',
 'remediation',
 '[
    {"type": "run_cmd", "timeout": 15, "description": "Force unmount untrusted USB storage volume", "params": {"cmd": "powershell -Command \"Get-Volume | Where-Object DriveType -eq Removable | Dismount-Volume\""}},
    {"type": "collect_logs", "timeout": 60, "description": "Retrieve Windows Event Logs for Device insertions", "params": {"log_types": "System,Security"}}
 ]'::jsonb,
 ARRAY['T1091', 'T1052'],
 true, NOW(), NOW()),

('33333333-3333-3333-3333-333333333333',
 'Deep Malware Investigation',
 'Executes a comprehensive YARA scan and queries the registry for persistence mechanisms.',
 'investigation',
 '[
    {"type": "scan_file", "timeout": 600, "description": "Run full YARA signature scan on recent file modifications", "params": {"file_path": "C:\\Windows\\Temp"}},
    {"type": "run_cmd", "timeout": 120, "description": "Analyze Run/RunOnce keys and Scheduled Tasks", "params": {"cmd": "reg query HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"}}
 ]'::jsonb,
 ARRAY['T1547', 'T1053'],
 true, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

INSERT INTO automation_rules (id, name, description, trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes, enabled, success_rate, created_at, updated_at) VALUES
('44444444-4444-4444-4444-444444444444',
 'Auto-Contain Suspected Ransomware',
 'Triggers the Ransomware Containment Playbook when highly suspicious encryption behavior is detected on any agent.',
 '{"rule_name": "Ransomware Behavior Detected"}'::jsonb,
 '11111111-1111-1111-1111-111111111111',
 1, true, 5, true, 0.98, NOW(), NOW()),

('55555555-5555-5555-5555-555555555555',
 'Block Untrusted Mass Storage',
 'Triggers the USB response playbook to unmount unknown devices immediately upon detection.',
 '{"rule_name": "Unauthorized USB Inserted"}'::jsonb,
 '22222222-2222-2222-2222-222222222222',
 3, true, 5, true, 0.95, NOW(), NOW()),

('66666666-6666-6666-6666-666666666666',
 'High Risk Process Termination',
 'Terminates known credential dumping utilities and triggers an advanced memory scan playbook.',
 '{"rule_name": "Credential Dumping Tool"}'::jsonb,
 '33333333-3333-3333-3333-333333333333',
 2, false, 5, true, 0.85, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
