# TallyBridge Connector

Desktop application that bridges **Tally ERP** (Windows) with the [TallyBridge Platform](https://github.com/OneBook-ERP/tallybridge-platform) cloud middleware. Reads vouchers and master data from Tally via its XML API and pushes them to the cloud for sync with ERPNext.

[![CI](https://github.com/OneBook-ERP/tallybridge-connector/actions/workflows/ci.yml/badge.svg)](https://github.com/OneBook-ERP/tallybridge-connector/actions/workflows/ci.yml)

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
- **Backend:** Go 1.22+
- **Frontend:** React + TypeScript + Tailwind CSS + Vite
- **Database:** SQLite (local config & sync state)
- **Tally Integration:** XML-over-HTTP (NATIVEMETHOD collection API)
- **System Tray:** energye/systray (minimize to tray, background sync)

## Project Structure

```
├── app.go                    # Main app — all Go methods exposed to frontend
├── main.go                   # Wails app entry point
├── tray.go                   # System tray integration (minimize to tray)
├── wails.json                # Wails project config
├── tally/
│   ├── client.go             # HTTP client for Tally XML server (120s timeout)
│   ├── envelope.go           # XML request envelope builder (NATIVEMETHOD format)
│   ├── parser/               # Tally XML response parsers
│   ├── reader/               # Data readers (companies, vouchers, ledgers, stock items)
│   └── writer/               # Tally XML writer (reverse sync — create/alter/cancel)
├── sync/
│   ├── delta.go              # Delta sync with master data + monthly chunking
│   ├── scheduler.go          # Background sync scheduler (5-min interval + heartbeat)
│   └── reverse.go            # Reverse sync (ERPNext → Tally via job polling)
├── store/
│   ├── store.go              # SQLite database wrapper
│   ├── config.go             # Config key-value store
│   ├── schema.sql            # Database schema
│   ├── sync_state.go         # Sync watermark tracking
│   ├── master_hash.go        # Master data change detection (content hashing)
│   ├── job_queue.go          # Local job queue for reverse sync
│   └── mapping_config.go     # Account mapping config
├── cmd/
│   └── sync-once/            # CLI tool for one-off sync (bypasses GUI)
├── frontend/
│   └── src/
│       ├── App.tsx           # Main app with setup wizard + 5-tab dashboard
│       ├── components/ui/    # Reusable UI components (Card, Badge, Button)
│       └── lib/              # Wails bindings & utilities
├── build/                    # Build assets (icons, manifests)
├── build-windows.sh          # Cross-compile for Windows (ARM64, x64, x86)
└── build-windows-arm64.sh    # Cross-compile for Windows ARM64
```

## Setup

### Prerequisites
- Go 1.22+
- Node.js 20+
- [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- Tally ERP running with XML server enabled (default port 9000)

### Development

```bash
# Install frontend dependencies
cd frontend && npm install && cd ..

# Start dev mode (hot reload for both Go and React)
~/go/bin/wails dev
```

### Building

```bash
# Build for current platform
wails build

# Cross-compile for Windows (ARM64, x64, x86 — requires zig)
./build-windows.sh
```

### Running Tests

```bash
CGO_ENABLED=1 go test ./...
```

### Tally XML Server Setup

1. Open Tally ERP > Gateway of Tally > Configure > Advanced Configuration
2. Set **Enable ODBC/XML Server** to `Yes`
3. Set **Port** to `9000` (default)
4. The connector auto-discovers Tally on `localhost:9000` or scans the local network

## How It Works

### Setup Wizard (4 steps)
1. **Connect to Tally** — Enter Tally XML server URL or auto-scan network
2. **Configure ERPNext** — Enter ERPNext URL + API credentials, test connection
3. **Select Company** — Pick a Tally company, set FY date range for initial sync
4. **Register with Cloud** — Register with TallyBridge Platform, get machine token

### Dashboard (5 tabs)
1. **Overview** — Sync status, Sync Masters button, Auto Sync toggle, Sync Now
2. **Exceptions** — Errors from cloud with tier, error code, document ref
3. **Sync Log** — Sync records with status, ERPNext doctype/name, timestamps
4. **Mappings** — JSON editor for 11 mapping sections + visual ledger reconciliation with root-type filters (Asset/Liability/Income/Expense)
5. **Settings** — Config editor, auto-start toggle, diagnostics, delete company

### Sync Flow
1. **Scheduler** fires every 5 minutes (can be paused via Auto Sync toggle)
2. **Master data** (ledgers, stock items, UOMs, godowns) synced first with content-hash change detection
3. **DeltaSync** reads new/modified vouchers from Tally using `$$IsAlteredSince` filter
4. Large FY data is fetched in **monthly chunks** to avoid Tally timeouts
5. Vouchers are batched (500 per request) and POSTed to the platform
6. Sync watermark (last-synced timestamp) persisted in SQLite
7. **Heartbeat** sent every 30 seconds with connector status + Tally counts every 5 minutes

### Reverse Sync (ERPNext to Tally)
- Long-poll loop polls cloud for pending write jobs
- Transforms ERPNext documents to Tally XML format
- Writes vouchers to Tally via XML import API

### System Tray
- App minimizes to system tray on close (background sync continues)
- Left-click tray icon to restore window
- Right-click for Show/Quit menu

### Tally XML API Details
- Uses **NATIVEMETHOD** format (not FETCH) for collection requests
- `<TYPE>` is a child element inside `<COLLECTION>`, not an attribute
- Date range via `SVFROMDATE`/`SVTODATE` in `<STATICVARIABLES>`
- Company names include FY suffix: `"COMPANY NAME - (2023-24)"`
- `$$IsAlteredSince` filter for incremental sync
- `SVCURRENTCOMPANY` required in import XML to target correct company

## App Methods (Go to Frontend)

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
| `SyncMasters()` | Force full master data re-sync |
| `GetSyncEnabled()` / `SetSyncEnabled(bool)` | Toggle auto-sync on/off |
| `DeleteCompanyData()` | Wipe all local state for clean reset |

## Releasing

```bash
# Tag a new version and push — GitHub Actions builds Windows binaries
cd /Users/apple/Documents/onebook_tally/tallybridge-connector
git tag v0.X.X
git push origin v0.X.X
```
Replace v0.2.0 with your desired version number. GitHub Actions will build the Windows binaries automatically.
---

## License

Private — OneBook ERP
