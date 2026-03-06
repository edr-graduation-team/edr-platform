-- Drop command tables and all dependencies
DROP TRIGGER IF EXISTS trigger_commands_queue ON commands;
DROP TRIGGER IF EXISTS trigger_commands_set_expires ON commands;
DROP FUNCTION IF EXISTS add_to_command_queue();
DROP FUNCTION IF EXISTS set_command_expires();

DROP TABLE IF EXISTS command_queue CASCADE;
DROP TABLE IF EXISTS commands CASCADE;

-- Drop indexes
DROP INDEX IF EXISTS idx_commands_agent_id;
DROP INDEX IF EXISTS idx_commands_status;
DROP INDEX IF EXISTS idx_commands_command_type;
DROP INDEX IF EXISTS idx_commands_issued_at;
DROP INDEX IF EXISTS idx_commands_issued_by;
DROP INDEX IF EXISTS idx_commands_pending;
DROP INDEX IF EXISTS idx_command_queue_agent_priority;
