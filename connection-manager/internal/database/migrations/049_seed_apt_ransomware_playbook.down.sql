-- Migration 049 down: Remove the APT Ransomware Containment playbook and rule

DELETE FROM automation_rules WHERE id = 'aaaaaaaa-bbbb-cccc-dddd-ffffffffffff';
DELETE FROM response_playbooks WHERE id = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee';
