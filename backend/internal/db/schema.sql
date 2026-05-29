CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS agents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    public_key   TEXT NOT NULL,
    arch         TEXT,
    os           TEXT,
    gpu_model    TEXT,
    cpu_cores    INT,
    ram_gb       FLOAT,
    gpu_layers   INT,
    cpu_score    FLOAT NOT NULL DEFAULT 0,
    mem_bw_gbps  FLOAT NOT NULL DEFAULT 0,
    gpu_tflops   FLOAT NOT NULL DEFAULT 0,
    bench_score  FLOAT NOT NULL DEFAULT 1.0,
    cpu_limit    FLOAT NOT NULL DEFAULT 50,
    ram_limit    FLOAT NOT NULL DEFAULT 25,
    gpu_layers_limit INT NOT NULL DEFAULT 0,
    status       TEXT NOT NULL DEFAULT 'offline',
    last_seen    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS token_ledger (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id   UUID NOT NULL REFERENCES agents(id),
    delta      FLOAT NOT NULL,
    reason     TEXT NOT NULL,
    job_id     UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS token_ledger_agent_idx ON token_ledger(agent_id);

CREATE TABLE IF NOT EXISTS jobs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submitter_agent_id  UUID NOT NULL REFERENCES agents(id),
    assigned_agent_id   UUID REFERENCES agents(id),
    type                TEXT NOT NULL DEFAULT 'llm_inference',
    payload             JSONB NOT NULL DEFAULT '{}',
    status              TEXT NOT NULL DEFAULT 'pending',
    token_cost          FLOAT NOT NULL DEFAULT 0,
    error               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS jobs_status_idx ON jobs(status);
CREATE INDEX IF NOT EXISTS jobs_submitter_idx ON jobs(submitter_agent_id);
