-- Migration 039 DOWN: Restore USB eject steps to use the WMI+mountvol PowerShell command.
-- Note: this restores the intermediate fix, not the original broken Dismount-Volume command.

UPDATE response_playbooks
SET
    commands = (
        SELECT jsonb_agg(
            CASE
                WHEN step->>'type' = 'run_cmd'
                 AND step->'params'->>'cmd' = '__EJECT_USB__'
                THEN jsonb_set(
                    step,
                    '{params,cmd}',
                    '"powershell -Command \"$ErrorActionPreference=''SilentlyContinue''; $drives=Get-WmiObject Win32_LogicalDisk|Where-Object{$_.DriveType -eq 2}; if($drives){$drives|ForEach-Object{$d=$_.DeviceID+''\''; mountvol $d /D}}; exit 0\""'
                )
                ELSE step
            END
        )
        FROM jsonb_array_elements(commands) AS step
    ),
    updated_at = NOW()
WHERE commands::text ILIKE '%__EJECT_USB__%';
