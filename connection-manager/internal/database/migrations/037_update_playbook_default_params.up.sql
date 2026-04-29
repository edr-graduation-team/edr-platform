-- Migration 037: Backfill default params into existing playbook run_cmd commands
-- so analysts no longer need to manually type PowerShell/reg commands when
-- executing the USB and Deep Malware playbooks from the dashboard.
--
-- The params are embedded inside the JSONB commands array using jsonb_set with
-- a path expression that targets the specific array element by index.

-- USB Playbook (id: 22222222-...): set run_cmd params on commands[0]
UPDATE response_playbooks
SET commands = jsonb_set(
    commands,
    '{0,params}',
    '{"cmd": "powershell -Command \"Get-Volume | Where-Object DriveType -eq Removable | Dismount-Volume\""}'::jsonb,
    true
)
WHERE id = '22222222-2222-2222-2222-222222222222'
  AND (commands->0->>'params') IS NULL;

-- USB Playbook: set collect_logs params on commands[1]
UPDATE response_playbooks
SET commands = jsonb_set(
    commands,
    '{1,params}',
    '{"log_types": "System,Security"}'::jsonb,
    true
)
WHERE id = '22222222-2222-2222-2222-222222222222'
  AND (commands->1->>'params') IS NULL;

-- Deep Malware Playbook (id: 33333333-...): set scan_file params on commands[0]
UPDATE response_playbooks
SET commands = jsonb_set(
    commands,
    '{0,params}',
    '{"file_path": "C:\\\\Windows\\\\Temp"}'::jsonb,
    true
)
WHERE id = '33333333-3333-3333-3333-333333333333'
  AND (commands->0->>'params') IS NULL;

-- Deep Malware Playbook: set run_cmd params on commands[1]
UPDATE response_playbooks
SET commands = jsonb_set(
    commands,
    '{1,params}',
    '{"cmd": "reg query HKLM\\\\Software\\\\Microsoft\\\\Windows\\\\CurrentVersion\\\\Run"}'::jsonb,
    true
)
WHERE id = '33333333-3333-3333-3333-333333333333'
  AND (commands->1->>'params') IS NULL;
