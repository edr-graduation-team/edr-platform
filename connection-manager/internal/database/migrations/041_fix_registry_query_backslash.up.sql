-- Migration 041: Fix run_cmd steps with double-escaped backslashes in registry queries
-- and update the Deep Malware Investigation run_cmd step to use the correct single-backslash
-- reg query path.
--
-- Root cause: JSON escaping of "HKLM\\Software\\..." results in literal double-backslash
-- strings being passed to reg.exe, which rejects them as "Invalid key name".
-- The correct stored JSON value should be "HKLM\Software\..." (one backslash).

UPDATE response_playbooks
SET
    commands = (
        SELECT jsonb_agg(
            CASE
                -- Fix any run_cmd step where the cmd contains double-backslash reg paths
                WHEN step->>'type' = 'run_cmd'
                 AND step->'params'->>'cmd' ILIKE '%reg query HKLM\\%'
                THEN jsonb_set(
                    step,
                    '{params,cmd}',
                    -- Replace double backslash with single backslash in the reg query
                    to_jsonb(replace(step->'params'->>'cmd', 'HKLM\\', 'HKLM\'))
                )
                ELSE step
            END
        )
        FROM jsonb_array_elements(commands) AS step
    ),
    updated_at = NOW()
WHERE commands::text ILIKE '%reg query HKLM\\\\%';
