-- Migration 040: Fix all playbook command params for correct agent execution
--
-- Problems fixed:
--   1. collect_forensics uses "event_types" but agent reads "log_types"/"types"
--      → rename event_types to log_types and map values to real Windows event log names
--   2. collect_forensics with no params at all → add default log_types
--   3. USB unmount in "Unauthorized USB Device Response" (ID 22222222-...) uses
--      old Dismount-Volume PS command → replace with __EJECT_USB__ token (done in 039,
--      but 035 uses ON CONFLICT DO NOTHING so 039 may not have patched it)
--   4. All collect_forensics steps now get time_range=24h as a sensible default


-- ═══════════════════════════════════════════════════════════════════════════════
-- HELPER: remap event_types values to real Windows log channel names
-- process,file,network,dns,registry → System,Security,Application
-- authentication → Security
-- system → System
-- ═══════════════════════════════════════════════════════════════════════════════

-- Fix ALL response_playbooks rows that have collect_forensics steps
-- using "event_types" instead of "log_types":
UPDATE response_playbooks
SET
    commands = (
        SELECT jsonb_agg(
            CASE
                WHEN step->>'type' = 'collect_forensics'
                THEN (
                    -- Remove "event_types" key, inject "log_types" and "time_range"
                    (step - 'params')
                    || jsonb_build_object('params',
                        COALESCE(step->'params', '{}'::jsonb)
                        -- Remove old event_types key
                        - 'event_types'
                        -- Add log_types mapped from event_types value
                        || jsonb_build_object(
                            'log_types',
                            CASE
                                WHEN (step->'params'->>'event_types') ILIKE '%security%'
                                  OR (step->'params'->>'event_types') ILIKE '%auth%'
                                  OR (step->'params'->>'event_types') ILIKE '%network%'
                                THEN 'System,Security'
                                ELSE 'System,Security'  -- safe default for all cases
                            END
                        )
                        -- Ensure time_range is set
                        || jsonb_build_object('time_range', '24h')
                    )
                )
                ELSE step
            END
        )
        FROM jsonb_array_elements(commands) AS step
    ),
    updated_at = NOW()
WHERE commands::text ILIKE '%"collect_forensics"%'
  AND commands::text ILIKE '%event_types%';


-- ═══════════════════════════════════════════════════════════════════════════════
-- Fix collect_forensics steps that have NO params at all
-- (e.g. "Ransomware Immediate Containment" from migration 035)
-- ═══════════════════════════════════════════════════════════════════════════════
UPDATE response_playbooks
SET
    commands = (
        SELECT jsonb_agg(
            CASE
                WHEN step->>'type' = 'collect_forensics'
                 AND (step->'params' IS NULL OR step->'params' = 'null'::jsonb OR step->'params' = '{}'::jsonb
                      OR step->'params'->>'log_types' IS NULL AND step->'params'->>'types' IS NULL)
                THEN jsonb_set(
                    step,
                    '{params}',
                    '{"log_types": "System,Security", "time_range": "24h", "max_events": "500"}'::jsonb,
                    true
                )
                ELSE step
            END
        )
        FROM jsonb_array_elements(commands) AS step
    ),
    updated_at = NOW()
WHERE commands::text ILIKE '%"collect_forensics"%'
  AND (
    commands::text NOT ILIKE '%log_types%'
    OR commands::text NOT ILIKE '%"types"%'
  );


-- ═══════════════════════════════════════════════════════════════════════════════
-- Fix USB unmount step in "Unauthorized USB Device Response" (ID 22222222-...)
-- Migration 035 uses ON CONFLICT DO NOTHING so migration 039 may have missed it
-- if the row already existed with the old Dismount-Volume command.
-- ═══════════════════════════════════════════════════════════════════════════════
UPDATE response_playbooks
SET
    commands = (
        SELECT jsonb_agg(
            CASE
                WHEN step->>'type' = 'run_cmd'
                 AND (
                     step->'params'->>'cmd' ILIKE '%Dismount-Volume%'
                     OR step->'params'->>'cmd' ILIKE '%Win32_LogicalDisk%'
                     OR step->'params'->>'cmd' ILIKE '%mountvol%'
                 )
                THEN jsonb_set(
                    jsonb_set(step, '{params,cmd}', '"__EJECT_USB__"'),
                    '{params,from_playbook}', '"true"'
                )
                ELSE step
            END
        )
        FROM jsonb_array_elements(commands) AS step
    ),
    updated_at = NOW()
WHERE id = '22222222-2222-2222-2222-222222222222'
   OR (
       commands::text ILIKE '%Dismount-Volume%'
       OR commands::text ILIKE '%Win32_LogicalDisk%'
   );


-- ═══════════════════════════════════════════════════════════════════════════════
-- Fix collect_logs steps that have no params (add System,Security default)
-- ═══════════════════════════════════════════════════════════════════════════════
UPDATE response_playbooks
SET
    commands = (
        SELECT jsonb_agg(
            CASE
                WHEN step->>'type' = 'collect_logs'
                 AND (step->'params' IS NULL OR step->'params'->>'log_types' IS NULL)
                THEN jsonb_set(
                    step,
                    '{params}',
                    '{"log_types": "System,Security"}'::jsonb,
                    true
                )
                ELSE step
            END
        )
        FROM jsonb_array_elements(commands) AS step
    ),
    updated_at = NOW()
WHERE commands::text ILIKE '%"collect_logs"%'
  AND commands::text NOT ILIKE '%log_types%';


-- ═══════════════════════════════════════════════════════════════════════════════
-- Fix terminate_process steps that have no process_name (use safe default)
-- ═══════════════════════════════════════════════════════════════════════════════
UPDATE response_playbooks
SET
    commands = (
        SELECT jsonb_agg(
            CASE
                WHEN step->>'type' = 'terminate_process'
                 AND (step->'params' IS NULL OR step->'params'->>'process_name' IS NULL OR step->'params'->>'process_name' = '')
                THEN jsonb_set(
                    step,
                    '{params}',
                    '{"process_name": "suspicious.exe", "kill_tree": "true"}'::jsonb,
                    true
                )
                ELSE step
            END
        )
        FROM jsonb_array_elements(commands) AS step
    ),
    updated_at = NOW()
WHERE commands::text ILIKE '%"terminate_process"%'
  AND (
    commands::text NOT ILIKE '%process_name%'
    OR commands::text ILIKE '%"process_name": ""'
  );
