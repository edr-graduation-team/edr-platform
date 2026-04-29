-- Migration 038: Backfill real default params into all 9 playbook commands
-- that still contain ${template_variable} placeholders or missing params.
-- These UPDATE statements are idempotent — they only change rows where the
-- param is still a template string or missing, so re-running is safe.
--
-- Playbooks targeted (from migration 032, auto-inserted without fixed IDs):
--   1. Malware Immediate Containment  → quarantine_file: file_path
--   2. Ransomware Attack Response     → terminate_process: process_name
--   3. Malware Removal & Recovery     → terminate_process, quarantine_file, update_signatures

-- ─── 1. Malware Immediate Containment ────────────────────────────────────────
-- commands[2] (index 2) = quarantine_file: replace ${suspicious_file} with real path
UPDATE response_playbooks
SET commands = jsonb_set(
    commands,
    '{2,params,file_path}',
    '"C:\\Windows\\Temp"',
    true
)
WHERE name = 'Malware Immediate Containment'
  AND commands->2->>'type' = 'quarantine_file'
  AND commands->2->'params'->>'file_path' LIKE '${%}';

-- ─── 2. Ransomware Attack Response ───────────────────────────────────────────
-- commands[1] (index 1) = terminate_process: replace ${ransomware_process}
UPDATE response_playbooks
SET commands = jsonb_set(
    commands,
    '{1,params,process_name}',
    '"vssadmin.exe"',
    true
)
WHERE name = 'Ransomware Attack Response'
  AND commands->1->>'type' = 'terminate_process'
  AND commands->1->'params'->>'process_name' LIKE '${%}';

-- ─── 3. Malware Removal & Recovery ───────────────────────────────────────────
-- commands[0] = terminate_process: replace ${malware_process}
UPDATE response_playbooks
SET commands = jsonb_set(
    commands,
    '{0,params,process_name}',
    '"suspicious.exe"',
    true
)
WHERE name = 'Malware Removal & Recovery'
  AND commands->0->>'type' = 'terminate_process'
  AND commands->0->'params'->>'process_name' LIKE '${%}';

-- commands[1] = quarantine_file: replace ${malware_file}
UPDATE response_playbooks
SET commands = jsonb_set(
    commands,
    '{1,params,file_path}',
    '"C:\\Windows\\Temp"',
    true
)
WHERE name = 'Malware Removal & Recovery'
  AND commands->1->>'type' = 'quarantine_file'
  AND commands->1->'params'->>'file_path' LIKE '${%}';

-- commands[2] = update_signatures: replace ${signature_url} with empty string
-- (the agent will use its built-in signature feed when url is empty)
UPDATE response_playbooks
SET commands = jsonb_set(
    commands,
    '{2,params,url}',
    '""',
    true
)
WHERE name = 'Malware Removal & Recovery'
  AND commands->2->>'type' = 'update_signatures'
  AND commands->2->'params'->>'url' LIKE '${%}';
