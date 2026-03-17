-- TallyBridge Connector — Local SQLite Schema
-- Run once on first launch; subsequent launches are no-ops (IF NOT EXISTS).

CREATE TABLE IF NOT EXISTS sync_state (
    collection   TEXT PRIMARY KEY,   -- 'vouchers' | 'ledgers' | 'stock_items'
    last_sync_at TEXT NOT NULL DEFAULT '',  -- yyyymmddHHmmss (Tally format)
    last_hash    TEXT NOT NULL DEFAULT ''
);

-- Seed default rows so callers can always do a simple UPDATE.
INSERT OR IGNORE INTO sync_state (collection) VALUES ('vouchers');
INSERT OR IGNORE INTO sync_state (collection) VALUES ('ledgers');
INSERT OR IGNORE INTO sync_state (collection) VALUES ('stock_items');
INSERT OR IGNORE INTO sync_state (collection) VALUES ('masters');

CREATE TABLE IF NOT EXISTS job_queue (
    id           TEXT PRIMARY KEY,          -- UUID
    job_type     TEXT NOT NULL,             -- 'CREATE' | 'ALTER' | 'CANCEL'
    payload      TEXT NOT NULL,             -- JSON
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    retry_count  INTEGER NOT NULL DEFAULT 0,
    status       TEXT NOT NULL DEFAULT 'PENDING'  -- PENDING | SENT | FAILED
);

CREATE TABLE IF NOT EXISTS config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- ── Sync mapping config (per company, one row per section) ──────────────────
-- Cloud (config_profiles) is the source of truth for all mappings.
-- This table is a read-through cache populated by GetCloudConfig.
-- Scoped by company_id so multiple companies on the same machine are isolated.
CREATE TABLE IF NOT EXISTS mapping_config (
    company_id  TEXT NOT NULL,   -- matches config.company_id
    section     TEXT NOT NULL,   -- 'account_coa' | 'gst_ledger' | 'voucher_type' | 'godown_warehouse'
    config      TEXT NOT NULL DEFAULT '{}',  -- JSON string pulled from cloud
    PRIMARY KEY (company_id, section)
);

CREATE TABLE IF NOT EXISTS master_hashes (
    entity_type  TEXT NOT NULL,  -- 'ledger' | 'stock_item' | 'stock_group' | 'uom' | 'godown'
    entity_guid  TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    synced_at    DATETIME NOT NULL,
    PRIMARY KEY (entity_type, entity_guid)
);

-- Seed known config keys with empty defaults.
INSERT OR IGNORE INTO config (key) VALUES ('company_tally_name');
INSERT OR IGNORE INTO config (key) VALUES ('machine_token');
INSERT OR IGNORE INTO config (key) VALUES ('cloud_url');
INSERT OR IGNORE INTO config (key) VALUES ('poll_interval_minutes');
INSERT OR IGNORE INTO config (key) VALUES ('last_registered_at');
INSERT OR IGNORE INTO config (key) VALUES ('company_id');
INSERT OR IGNORE INTO config (key) VALUES ('machine_id');
INSERT OR IGNORE INTO config (key) VALUES ('sync_from_date');  -- YYYY-MM-DD, FY start
INSERT OR IGNORE INTO config (key) VALUES ('sync_to_date');    -- YYYY-MM-DD, FY end
