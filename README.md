# TallyBridge Connector

Desktop application that bridges **Tally ERP** (Windows) with the [TallyBridge Platform](https://github.com/OneBook-ERP/tallybridge-platform) cloud middleware. Reads vouchers and master data from Tally via its XML API and pushes them to the cloud for sync with ERPNext.

## Architecture

```
Tally ERP (Desktop)
    ↕ XML API (HTTP port 9000)
TallyBridge Connector (this app)  ← Wails v2 + Go + React
    ↕ REST API + WebSocket
TallyBridge Platform (Cloud)
    ↕ REST API
ERPNext (Cloud SaaS)
```

## Tech Stack

- **Desktop Framework:** Wails v2 (Go backend + web frontend)
- **Backend:** Go 1.21+
- **Frontend:** React + TypeScript + Tailwind CSS + Vite
- **Database:** SQLite (local config & sync state)
- **Tally Integration:** XML-over-HTTP (NATIVEMETHOD collection API)

## Project Structure

```
├── app.go                    # Main app — all Go methods exposed to frontend
├── main.go                   # Wails app entry point
├── wails.json                # Wails project config
├── tally/
│   ├── client.go             # HTTP client for Tally XML server
│   ├── envelope.go           # XML request envelope builder (NATIVEMETHOD format)
│   ├── envelope_test.go      # Tests for envelope construction
│   ├── parser/               # Tally XML response parsers
│   ├── reader/               # Tally data readers (companies, vouchers, ledgers)
│   └── writer/               # Tally XML writer (reverse sync)
├── sync/
│   ├── delta.go              # Delta sync — reads vouchers since last sync
│   ├── scheduler.go          # Background sync scheduler (5-min interval)
│   └── reverse.go            # Reverse sync (ERPNext → Tally, planned)
├── store/
│   ├── store.go              # SQLite database wrapper
│   ├── config.go             # Config key-value store
│   ├── schema.sql            # Database schema
│   ├── sync_state.go         # Sync state tracking
│   ├── job_queue.go          # Local job queue
│   └── mapping_config.go     # Account mapping config
├── frontend/
│   └── src/
│       ├── App.tsx           # Main app with step-based setup wizard
│       ├── components/ui/    # Reusable UI components (button, card, badge)
│       └── lib/              # Wails bindings & utilities
├── build/                    # Build assets (icons, manifests)
└── build-windows-arm64.sh    # Cross-compilation script for Windows ARM64
```

## Setup

### Prerequisites
- Go 1.21+
- Node.js 20+
- [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- Tally ERP running with XML server enabled (default port 9000)

### Development

```bash
# Install frontend dependencies
cd frontend && npm install && cd ..

# Start dev mode (hot reload for both Go and React)
wails dev
```

### Building

```bash
# Build for current platform
wails build

# Cross-compile for Windows ARM64 (from macOS, requires zig)
chmod +x build-windows-arm64.sh
./build-windows-arm64.sh
```

### Tally XML Server Setup

1. Open Tally ERP → Gateway of Tally → Configure → Advanced Configuration
2. Set **Enable ODBC/XML Server** to `Yes`
3. Set **Port** to `9000` (default)
4. The connector auto-discovers Tally on `localhost:9000` or scans the local network

## How It Works

### Setup Wizard (4 steps)
1. **Connect to Tally** — Enter Tally XML server URL or auto-scan network
2. **Configure ERPNext** — Enter ERPNext URL + API credentials, test connection
3. **Select Company** — Pick a Tally company, set FY date range for initial sync
4. **Register with Cloud** — Register with TallyBridge Platform, get machine token

### Sync Flow
1. **Scheduler** fires every 5 minutes (configurable)
2. **DeltaSync** reads new/modified vouchers from Tally using `$$IsAlteredSince` filter
3. Vouchers are batched and POSTed to `POST /connector/sync-data` on the platform
4. Platform transforms and writes to ERPNext
5. Sync state (last-synced timestamp) is persisted in local SQLite

### Tally XML API Details
- Uses **NATIVEMETHOD** format (not FETCH) for collection requests
- `<TYPE>` is a child element inside `<COLLECTION>`, not an attribute
- Date range via `SVFROMDATE`/`SVTODATE` in `<STATICVARIABLES>`
- Company names include FY suffix: `"COVAI POLYMER TRADERS - (2023-24)"`

## App Methods (Go → Frontend)

| Method | Description |
|--------|-------------|
| `ConnectTally(url)` | Connect to Tally XML server |
| `ScanTally()` | Auto-discover Tally on network |
| `GetCompanies()` | List companies from connected Tally |
| `SaveConfig(...)` | Save ERPNext + cloud config to SQLite |
| `SaveSyncRange(from, to)` | Set FY date range for sync |
| `TestERPNextConnection(url, key, secret)` | Verify ERPNext API credentials |
| `RegisterWithCloud(cloudURL, company)` | Register connector with platform |
| `StartBackgroundSync()` | Start 5-minute sync scheduler |
| `ValidateERPNextRequirements()` | Check ERPNext has required doctypes (stub) |
| `DataQualityScan()` | Scan Tally data quality (stub) |

---

## Task Tracker

### Completed
- [x] Wails v2 project scaffold with React frontend
- [x] Tally XML client with NATIVEMETHOD envelope format
- [x] Company list reader from Tally
- [x] Voucher reader with date range and delta filters
- [x] 4-step setup wizard UI (Tally → ERPNext → Company → Cloud)
- [x] ERPNext connection test (live HTTP check)
- [x] SQLite config store (key-value)
- [x] Cloud registration (POST /connector/register, saves machine token)
- [x] Background sync scheduler (5-minute interval)
- [x] Delta sync — reads vouchers since last altered date
- [x] Sync state persistence in SQLite
- [x] Auto-detect FY date range from company name suffix
- [x] Date picker for manual sync range override
- [x] Windows ARM64 cross-compilation (zig + go build)

### In Progress
- [ ] Reverse sync (ERPNext → Tally via long-poll `/connector/jobs/pending`)
- [ ] Master data sync (ledgers, stock items, cost centres)

### To Do — High Priority
- [ ] Wire heartbeat calls to `/connector/heartbeat` (keep-alive)
- [ ] Ledger master sync (Tally → Cloud for account mapping UI)
- [ ] Stock item sync (with HSN codes)
- [ ] Retry logic for failed cloud uploads
- [ ] Error toast notifications in UI
- [ ] Sync status dashboard (last sync time, counts, errors)

### To Do — Medium Priority
- [ ] Make `ValidateERPNextRequirements()` real (check custom fields, doctypes)
- [ ] Make `DataQualityScan()` real (check for missing GSTINs, dates, etc.)
- [ ] Multi-company support (switch between companies)
- [ ] Tally connection health monitoring (auto-reconnect)
- [ ] Configurable sync interval (currently fixed at 5 min)
- [ ] Account mapping editor in UI
- [ ] Tally XML writer for reverse sync (create vouchers in Tally)

### To Do — Low Priority
- [ ] System tray mode (minimize to tray, run in background)
- [ ] Auto-start on Windows login
- [ ] Update checker / auto-updater
- [ ] Logs viewer in UI
- [ ] Export sync log (CSV)
- [ ] Unit tests for XML parsers
- [ ] Integration tests with live Tally
- [ ] NSIS installer for Windows
- [ ] Code signing for Windows builds

---

## License

Private — OneBook ERP
