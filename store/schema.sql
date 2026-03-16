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
-- The connector is the source of truth for all sync mappings.
-- These are pushed to the cloud config_profiles on registration.
CREATE TABLE IF NOT EXISTS mapping_config (
    section  TEXT PRIMARY KEY,  -- 'account_coa' | 'ledger_group' | 'gst_ledger' |
                                 -- 'voucher_type' | 'item_hsn' | 'uom' |
                                 -- 'party_identity' | 'sync_direction' |
                                 -- 'conflict_resolution' | 'historical_import' | 'exception_handling'
    config   TEXT NOT NULL DEFAULT '{}'  -- JSON string
);

-- Seed all config sections with empty JSON defaults.
INSERT OR IGNORE INTO mapping_config (section) VALUES ('account_coa');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('ledger_group');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('gst_ledger');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('voucher_type');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('item_hsn');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('uom');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('party_identity');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('sync_direction');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('conflict_resolution');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('historical_import');
INSERT OR IGNORE INTO mapping_config (section) VALUES ('exception_handling');

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
