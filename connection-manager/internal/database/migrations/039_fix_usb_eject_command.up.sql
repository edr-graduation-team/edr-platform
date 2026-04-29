-- Migration 039: Fix USB unmount command in all response playbooks
-- Replaces any PowerShell-based USB eject commands (Dismount-Volume, Win32_LogicalDisk,
-- mountvol via PS) with the native __EJECT_USB__ token, which the agent handles
-- directly in Go using wmic + mountvol — no PowerShell, no exit-code ambiguity.

UPDATE response_playbooks
SET
    commands = (
        SELECT jsonb_agg(
            CASE
                -- Match any step that is a USB eject attempt (old or intermediate variants)
                WHEN (
                    step->>'type' IN ('run_cmd', 'device_unmount')
                    AND (
                        step->'params'->>'cmd' ILIKE '%Dismount-Volume%'
                        OR step->'params'->>'cmd' ILIKE '%Win32_LogicalDisk%'
                        OR step->'params'->>'cmd' ILIKE '%mountvol%'
                        OR step->'params'->>'cmd' ILIKE '%EJECT_USB%'
                        OR step->>'type' = 'device_unmount'
                    )
                )
                THEN jsonb_set(
                    jsonb_set(step, '{type}', '"run_cmd"'),
                    '{params,cmd}', '"__EJECT_USB__"'
                )
                ELSE step
            END
        )
        FROM jsonb_array_elements(commands) AS step
    ),
    updated_at = NOW()
WHERE
    commands IS NOT NULL
    AND commands != 'null'::jsonb
    AND (
        commands::text ILIKE '%Dismount-Volume%'
        OR commands::text ILIKE '%Win32_LogicalDisk%'
        OR commands::text ILIKE '%mountvol%'
        OR commands::text ILIKE '%EJECT_USB%'
        OR commands::text ILIKE '%device_unmount%'
    );
