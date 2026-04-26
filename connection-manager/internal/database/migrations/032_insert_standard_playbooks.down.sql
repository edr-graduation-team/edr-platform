-- Down migration: Remove Standard Response Playbooks

-- إزالة كتيبات الإجراءات القياسية
DELETE FROM response_playbooks WHERE name IN (
    'Malware Immediate Containment',
    'Ransomware Attack Response', 
    'Advanced Malware Analysis',
    'Unauthorized Access Investigation',
    'Malware Removal & Recovery',
    'System Validation Check'
);
