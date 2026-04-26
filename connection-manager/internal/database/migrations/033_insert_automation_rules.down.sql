-- Down migration: Remove Standard Automation Rules

-- إزالة قواعد الأتمتة القياسية
DELETE FROM automation_rules WHERE name IN (
    'Critical Malware Auto-Containment',
    'Suspicious Access Auto-Investigation',
    'Ransomware Emergency Response',
    'Medium Threat Analysis',
    'Malware Auto-Removal',
    'Post-Incident Validation'
);
