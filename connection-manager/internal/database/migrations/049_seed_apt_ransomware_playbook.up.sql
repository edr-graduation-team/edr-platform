-- Migration 049: Seed APT Ransomware Containment playbook for Chapter 5 testing
-- This playbook is designed for the multi-stage APT attack simulation scenario
-- using Atomic Red Team. It contains 3 sequential containment commands:
--   1. process_tree_snapshot — capture full process ancestry before isolation
--   2. collect_forensics     — pull Security + Sysmon logs for timeline analysis
--   3. isolate_network       — cut the host from the LAN (C2 channel stays open)
--
-- MITRE ATT&CK coverage: T1059.001, T1082, T1547.001, T1562.001, T1003.001, T1490
-- NIST 800-61 phase: Containment, Eradication & Recovery

INSERT INTO response_playbooks (
    id, name, description, category, commands,
    mitre_techniques, severity_filter, rule_pattern,
    enabled, created_at, updated_at
) VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee',
    'APT Ransomware Containment',
    'Automated response playbook for multi-stage APT ransomware attacks. Captures a process tree snapshot to preserve the attack chain, collects forensic event logs (Security & Sysmon) for timeline reconstruction, then isolates the host from the network to prevent lateral movement. Designed for use with Atomic Red Team test scenarios covering T1059→T1082→T1547→T1562→T1003→T1490.',
    'containment',
    '[
        {
            "type": "process_tree_snapshot",
            "timeout": 120,
            "description": "Capture complete process tree to preserve parent-child attack chain before isolation"
        },
        {
            "type": "collect_forensics",
            "timeout": 300,
            "description": "Collect Security and Sysmon event logs for forensic timeline analysis",
            "params": {
                "log_types": "Security,Microsoft-Windows-Sysmon/Operational",
                "max_events": "1000"
            }
        },
        {
            "type": "isolate_network",
            "timeout": 30,
            "description": "Isolate host from network to prevent lateral movement (C2 channel maintained)"
        }
    ]'::jsonb,
    ARRAY['T1059.001', 'T1082', 'T1547.001', 'T1562.001', 'T1003.001', 'T1490'],
    ARRAY['critical', 'high'],
    'Atomic Red Team',
    true,
    NOW(),
    NOW()
)
ON CONFLICT (id) DO NOTHING;

-- Also create a matching automation rule that can auto-trigger this playbook
-- when credential dumping (T1003) or ransomware impact (T1486/T1490) alerts fire
INSERT INTO automation_rules (
    id, name, description, trigger_conditions,
    playbook_id, priority, auto_execute,
    cooldown_minutes, enabled, success_rate,
    created_at, updated_at
) VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-ffffffffffff',
    'Auto-Respond APT Ransomware Chain',
    'Automatically triggers the APT Ransomware Containment playbook when critical-severity alerts matching credential dumping (T1003) or system recovery inhibition (T1490) patterns are detected. Designed for Atomic Red Team validation scenarios.',
    '{"severity": "critical", "rule_name": "T1003|T1490|T1486|Credential Dumping|Inhibit System Recovery"}'::jsonb,
    'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee',
    1,
    false,
    10,
    true,
    0.0,
    NOW(),
    NOW()
)
ON CONFLICT (id) DO NOTHING;
