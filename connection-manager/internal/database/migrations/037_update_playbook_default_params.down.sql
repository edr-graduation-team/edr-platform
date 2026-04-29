-- Rollback: Remove the params field that was added to each command step.
-- This uses jsonb_set to set the params key to NULL is not straightforward,
-- so instead we use #- operator to remove the key.

UPDATE response_playbooks
SET commands = commands #- '{0,params}'
WHERE id = '22222222-2222-2222-2222-222222222222';

UPDATE response_playbooks
SET commands = commands #- '{1,params}'
WHERE id = '22222222-2222-2222-2222-222222222222';

UPDATE response_playbooks
SET commands = commands #- '{0,params}'
WHERE id = '33333333-3333-3333-3333-333333333333';

UPDATE response_playbooks
SET commands = commands #- '{1,params}'
WHERE id = '33333333-3333-3333-3333-333333333333';
