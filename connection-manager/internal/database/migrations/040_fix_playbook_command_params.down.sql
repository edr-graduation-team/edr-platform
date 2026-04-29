-- Migration 040 DOWN: Revert collect_forensics params back to event_types.
-- Note: this is a best-effort rollback; complex JSONB transformations are not
-- perfectly reversible without storing original values.

UPDATE response_playbooks
SET
    commands = (
        SELECT jsonb_agg(
            CASE
                WHEN step->>'type' = 'collect_forensics'
                 AND step->'params'->>'log_types' IS NOT NULL
                THEN (
                    (step - 'params')
                    || jsonb_build_object('params',
                        (step->'params' - 'log_types' - 'time_range')
                        || jsonb_build_object('event_types', step->'params'->>'log_types')
                    )
                )
                ELSE step
            END
        )
        FROM jsonb_array_elements(commands) AS step
    ),
    updated_at = NOW()
WHERE commands::text ILIKE '%"collect_forensics"%';
