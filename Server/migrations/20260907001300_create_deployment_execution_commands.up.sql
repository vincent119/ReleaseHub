CREATE TABLE deployment_execution_commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_version_id UUID NOT NULL REFERENCES deployment_request_versions(id),
    execution_id UUID NOT NULL REFERENCES deployment_executions(id),
    result_execution_id UUID NULL REFERENCES deployment_executions(id),
    command_type TEXT NOT NULL CHECK (command_type IN ('Retry', 'Terminate', 'Unlock')),
    actor_id UUID NOT NULL REFERENCES users(id),
    idempotency_key TEXT NOT NULL CHECK (length(btrim(idempotency_key)) BETWEEN 1 AND 255),
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    reason TEXT NOT NULL DEFAULT '',
    actual_states JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(actual_states) = 'array'),
    occurred_at TIMESTAMPTZ NOT NULL,
    UNIQUE (request_version_id, command_type, idempotency_key),
    CHECK (command_type = 'Retry' OR length(btrim(reason)) > 0),
    CHECK ((command_type = 'Retry' AND result_execution_id IS NOT NULL) OR
           (command_type <> 'Retry' AND result_execution_id IS NULL)),
    CHECK ((command_type = 'Unlock' AND jsonb_array_length(actual_states) > 0) OR
           (command_type <> 'Unlock' AND jsonb_array_length(actual_states) = 0))
);

CREATE INDEX deployment_execution_commands_execution_idx
ON deployment_execution_commands (execution_id, occurred_at DESC);

CREATE TRIGGER deployment_execution_commands_immutable
BEFORE UPDATE OR DELETE ON deployment_execution_commands
FOR EACH ROW EXECUTE FUNCTION releasehub_reject_immutable_row_mutation();
