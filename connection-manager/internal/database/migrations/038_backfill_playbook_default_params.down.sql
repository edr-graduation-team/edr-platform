-- Rollback 038: Restore ${template_variable} placeholders
UPDATE response_playbooks
SET commands = jsonb_set(commands, '{2,params,file_path}', '"${suspicious_file}"', true)
WHERE name = 'Malware Immediate Containment' AND commands->2->>'type' = 'quarantine_file';

UPDATE response_playbooks
SET commands = jsonb_set(commands, '{1,params,process_name}', '"${ransomware_process}"', true)
WHERE name = 'Ransomware Attack Response' AND commands->1->>'type' = 'terminate_process';

UPDATE response_playbooks
SET commands = jsonb_set(commands, '{0,params,process_name}', '"${malware_process}"', true)
WHERE name = 'Malware Removal & Recovery' AND commands->0->>'type' = 'terminate_process';

UPDATE response_playbooks
SET commands = jsonb_set(commands, '{1,params,file_path}', '"${malware_file}"', true)
WHERE name = 'Malware Removal & Recovery' AND commands->1->>'type' = 'quarantine_file';

UPDATE response_playbooks
SET commands = jsonb_set(commands, '{2,params,url}', '"${signature_url}"', true)
WHERE name = 'Malware Removal & Recovery' AND commands->2->>'type' = 'update_signatures';
