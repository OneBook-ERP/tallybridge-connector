import { useState, useEffect, useCallback } from 'react'
import {
    ConnectTally, GetCompanies, SaveConfig, SaveSyncRange,
    StartBackgroundSync, TestERPNextConnection, RegisterWithCloud,
    GetCompanyStats, GetTallyLedgers, GetTallyVoucherTypes,
    GetERPNextCompanies, GetERPNextFiscalYears, GetERPNextAccounts,
    ValidateSetup, CreateERPNextFiscalYear, SaveERPNextMapping,
    GetMappingConfig, SetMappingConfig, GetMappingDefaults, DataQualityScan,
    GetServiceStatus, GetSyncStats, SyncNow, GetAllConfig, SetConfigValue,
} from '../wailsjs/go/main/App'
import { Card, CardHeader, CardTitle, CardContent } from './components/ui/card'
import { Badge } from './components/ui/badge'
import { Button } from './components/ui/button'

// ─── Types ────────────────────────────────────────────────────────────────────

interface ActivationResult {
    companyId: string
    companyName: string
    erpCompany: string
    cloudUrl: string
    erpnextUrl: string
}

type DashboardTab = 'overview' | 'mappings' | 'settings'

// ─── App Root ─────────────────────────────────────────────────────────────────

export default function App() {
    const [activated, setActivated] = useState<ActivationResult | null>(null)
    return activated
        ? <Dashboard result={activated} onAddCompany={() => setActivated(null)} />
        : <SetupWizard onActivated={setActivated} />
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// suggestErpAccount finds the best ERPNext account for a Tally ledger group name
// by matching known Tally standard group names to ERPNext account keywords.
function suggestErpAccount(group: string, accounts: { name: string }[]): string {
    const g = group.toLowerCase()
    const matchers: [string[], string[]][] = [
        [['sundry debtor'], ['debtor', 'receivable']],
        [['sundry creditor'], ['creditor', 'payable']],
        [['bank account'], ['bank']],
        [['cash-in-hand', 'cash in hand'], ['cash']],
        [['duties & taxes', 'duties and taxes'], ['duties', 'tax']],
        [['sales account'], ['sales']],
        [['purchase account'], ['purchase']],
        [['direct expense'], ['direct expense']],
        [['indirect expense'], ['indirect expense']],
        [['direct income'], ['direct income']],
        [['indirect income'], ['indirect income']],
        [['fixed asset'], ['fixed asset']],
        [['capital account'], ['capital']],
        [['reserve', 'surplus'], ['reserve', 'surplus']],
        [['investment'], ['investment']],
        [['loans & advances (asset)', 'loan (asset)'], ['loan', 'advance']],
        [['loans (liability)', 'loan (liability)'], ['loan']],
    ]
    for (const [groupTerms, accountTerms] of matchers) {
        if (groupTerms.some(gt => g.includes(gt))) {
            for (const at of accountTerms) {
                const match = accounts.find(a => a.name.toLowerCase().includes(at))
                if (match) return match.name
            }
        }
    }
    return ''
}

// ─── Setup Wizard ─────────────────────────────────────────────────────────────

function SetupWizard({ onActivated }: { onActivated: (r: ActivationResult) => void }) {
    const [step, setStep] = useState(1)

    // Step 2
    const [tallyUrl, setTallyUrl] = useState('http://192.168.1.10:9000')
    const [tallyOnline, setTallyOnline] = useState(false)
    const [isScanning, setIsScanning] = useState(false)

    // Step 3
    const [companies, setCompanies] = useState<string[]>([])
    const [selectedCompany, setSelectedCompany] = useState<string | null>(null)
    const [syncFromDate, setSyncFromDate] = useState('')
    const [syncToDate, setSyncToDate] = useState('')
    const [companyStats, setCompanyStats] = useState<Record<string, number>>({})
    const [loadingStats, setLoadingStats] = useState(false)
    const [loadingCompanies, setLoadingCompanies] = useState(false)

    // Step 4
    const [erpnextUrl, setErpnextUrl] = useState('https://tally.zenpivot.in')
    const [apiKey, setApiKey] = useState('2015ad50ead346b')
    const [apiSecret, setApiSecret] = useState('fde5d28a4c3dbe1')
    const [cloudUrl, setCloudUrl] = useState('http://localhost:3000')
    const [isTestingErp, setIsTestingErp] = useState(false)
    const [erpConnectionError, setErpConnectionError] = useState('')

    // Step 5 — ERPNext company + fiscal year selection
    const [erpCompanies, setErpCompanies] = useState<string[]>([])
    const [selectedErpCompany, setSelectedErpCompany] = useState('')
    const [erpFiscalYears, setErpFiscalYears] = useState<{ name: string; yearStart: string; yearEnd: string }[]>([])
    const [fyCovers, setFyCovers] = useState(false)
    const [creatingFY, setCreatingFY] = useState(false)
    const [step5Loading, setStep5Loading] = useState(false)

    // Step 6 — Validation
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const [validationResults, setValidationResults] = useState<any[]>([])
    const [validating, setValidating] = useState(false)

    // Step 7 — Voucher type mapping
    const [tallyVTypes, setTallyVTypes] = useState<{ name: string; nature: string }[]>([])
    const [vtMapping, setVtMapping] = useState<Record<string, string>>({})
    const NATURE_TO_DOCTYPE: Record<string, string[]> = {
        'Sales': ['Sales Invoice', 'Delivery Note', 'Skip'],
        'Purchase': ['Purchase Invoice', 'Purchase Receipt', 'Skip'],
        'Payment': ['Payment Entry', 'Skip'],
        'Receipt': ['Payment Entry', 'Skip'],
        'Contra': ['Journal Entry', 'Skip'],
        'Journal': ['Journal Entry', 'Skip'],
        'Credit Note': ['Sales Invoice (Return)', 'Skip'],
        'Debit Note': ['Purchase Invoice (Return)', 'Skip'],
        'Delivery Note': ['Delivery Note', 'Skip'],
        'Sales Order': ['Sales Order', 'Skip'],
        'Purchase Order': ['Purchase Order', 'Skip'],
        'Stock Journal': ['Stock Entry', 'Skip'],
        'Attendance': ['Skip'],
        'Receipt Note': ['Purchase Receipt', 'Skip'],
    }
    const DEFAULT_DOCTYPE: Record<string, string> = {
        'Sales': 'Sales Invoice', 'Purchase': 'Purchase Invoice',
        'Payment': 'Payment Entry', 'Receipt': 'Payment Entry',
        'Contra': 'Journal Entry', 'Journal': 'Journal Entry',
        'Credit Note': 'Sales Invoice (Return)', 'Debit Note': 'Purchase Invoice (Return)',
        'Delivery Note': 'Delivery Note', 'Sales Order': 'Sales Order',
        'Purchase Order': 'Purchase Order', 'Stock Journal': 'Stock Entry',
        'Attendance': 'Skip', 'Receipt Note': 'Purchase Receipt',
    }

    // Step 8 — Group-based ledger mapping
    const [tallyLedgers, setTallyLedgers] = useState<{ name: string; parent: string; gstin: string; gstReg: string }[]>([])
    const [erpAccounts, setErpAccounts] = useState<{ name: string; accountType: string; rootType: string }[]>([])
    const [groupMapping, setGroupMapping] = useState<Record<string, string>>({})
    const [step8Loading, setStep8Loading] = useState(false)

    // Step 9 — Data quality
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const [qualityResults, setQualityResults] = useState<any>(null)

    // Step 10 — Activation
    const [isActivating, setIsActivating] = useState(false)
    const [activateStatus, setActivateStatus] = useState<'idle' | 'registering' | 'syncing' | 'done' | 'error'>('idle')
    const [activateError, setActivateError] = useState('')
    const [companyId, setCompanyId] = useState('')

    const TOTAL_STEPS = 10

    const detectFY = (name: string) => {
        const m = name.match(/\((\d{4})-(\d{2})\)/)
        if (!m) return null
        const fromYear = parseInt(m[1])
        const toYear = parseInt(m[1].slice(0, 2) + m[2])
        return { from: `${fromYear}-04-01`, to: `${toYear}-03-31` }
    }

    // ── Scan Tally ──
    const handleScanTally = async () => {
        setIsScanning(true)
        try {
            const res = await ConnectTally(tallyUrl)
            setTallyOnline(res.status === 'ok' && (res.online as boolean))
        } catch { setTallyOnline(false) }
        setIsScanning(false)
    }

    // ── Fetch companies ──
    const handleGetCompanies = async () => {
        setLoadingCompanies(true)
        try {
            const list = await GetCompanies()
            setCompanies(list || [])
            if (list?.length && !selectedCompany) {
                selectCompany(list[0])
            }
        } catch { /* ignore */ }
        setLoadingCompanies(false)
    }

    const selectCompany = (name: string) => {
        setSelectedCompany(name)
        setCompanyStats({})
        const fy = detectFY(name)
        if (fy) { setSyncFromDate(fy.from); setSyncToDate(fy.to) }
        // Lazy-load stats
        setLoadingStats(true)
        GetCompanyStats(name).then(stats => {
            setCompanyStats(stats as Record<string, number>)
        }).finally(() => setLoadingStats(false))
    }

    // ── Step 5: load ERPNext companies + fiscal years ──
    const loadStep5 = async () => {
        setStep5Loading(true)
        const [comps, fys] = await Promise.all([
            GetERPNextCompanies().catch(() => [] as string[]),
            GetERPNextFiscalYears().catch(() => [] as { name: string; yearStart: string; yearEnd: string }[]),
        ])
        setErpCompanies(comps || [])
        setErpFiscalYears((fys || []) as { name: string; yearStart: string; yearEnd: string }[])
        if (comps?.length && !selectedErpCompany) setSelectedErpCompany(comps[0])
        // Check coverage
        if (fys?.length && syncFromDate && syncToDate) {
            const covered = fys.some(fy =>
                fy.yearStart <= syncFromDate && fy.yearEnd >= syncToDate
            )
            setFyCovers(covered)
        }
        setStep5Loading(false)
    }

    // ── Step 6: validate ──
    const runValidation = async () => {
        setValidating(true)
        const results = await ValidateSetup(selectedErpCompany, syncFromDate, syncToDate).catch(() => [])
        setValidationResults(results || [])
        setValidating(false)
    }

    // ── Step 7: load voucher types ──
    const loadVoucherTypes = async () => {
        if (!selectedCompany) return
        const rawVts = await GetTallyVoucherTypes(selectedCompany).catch(() => [])
        const vts = (rawVts || []) as { name: string; nature: string }[]
        setTallyVTypes(vts)
        // Auto-fill defaults
        const defaults: Record<string, string> = {}
        for (const vt of vts) {
            defaults[vt.name] = DEFAULT_DOCTYPE[vt.nature] || 'Journal Entry'
        }
        setVtMapping(prev => ({ ...defaults, ...prev }))
    }

    // ── Step 8: load ledgers + accounts, auto-suggest group mapping ──
    const loadLedgerMapping = async () => {
        if (!selectedCompany || !selectedErpCompany) return
        setStep8Loading(true)
        const [ledgers, accounts] = await Promise.all([
            GetTallyLedgers(selectedCompany).catch(() => []),
            GetERPNextAccounts(selectedErpCompany).catch(() => []),
        ])
        const typedLedgers = (ledgers || []) as { name: string; parent: string; gstin: string; gstReg: string }[]
        const typedAccounts = (accounts || []) as { name: string; accountType: string; rootType: string }[]
        setTallyLedgers(typedLedgers)
        setErpAccounts(typedAccounts)
        // Auto-suggest group mapping based on group name keywords
        const suggestions: Record<string, string> = {}
        const uniqueGroups = [...new Set(typedLedgers.map(l => l.parent).filter(Boolean))]
        for (const g of uniqueGroups) {
            const s = suggestErpAccount(g, typedAccounts)
            if (s) suggestions[g] = s
        }
        setGroupMapping(prev => ({ ...suggestions, ...prev }))
        setStep8Loading(false)
    }

    // ── Step 9: data quality scan ──
    const runQualityScan = async () => {
        if (!selectedCompany) {
            setQualityResults({ ledgerTotal: 0, stockItemTotal: 0, missingGSTIN: 0, missingUOM: 0, missingHSN: 0, warnCount: 0, readyCount: 0 })
            return
        }
        const res = await DataQualityScan(selectedCompany).catch(() => null)
        if (!res) {
            setQualityResults({ ledgerTotal: 0, stockItemTotal: 0, missingGSTIN: 0, missingUOM: 0, missingHSN: 0, warnCount: 0, readyCount: 0 })
            return
        }
        setQualityResults(res)
    }

    useEffect(() => {
        if (step === 2 && !tallyOnline) handleScanTally()
        if (step === 3 && companies.length === 0) handleGetCompanies()
        if (step === 5) loadStep5()
        if (step === 6) runValidation()
        if (step === 7) loadVoucherTypes()
        if (step === 8) loadLedgerMapping()
        if (step === 9 && !qualityResults) runQualityScan()
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [step])

    // ── Navigation ──
    const next = async () => {
        // Step 3 → save sync range
        if (step === 3 && syncFromDate && syncToDate) {
            await SaveSyncRange(syncFromDate, syncToDate)
        }
        // Step 4 → test ERPNext
        if (step === 4) {
            setErpConnectionError('')
            setIsTestingErp(true)
            try {
                const res = await TestERPNextConnection(erpnextUrl, apiKey, apiSecret)
                if (res.status !== 'ok') {
                    setErpConnectionError(res.message as string)
                    setIsTestingErp(false)
                    return
                }
                await SaveConfig(erpnextUrl, apiKey, apiSecret, selectedCompany || '', cloudUrl)
            } catch (e) {
                setErpConnectionError(String(e))
                setIsTestingErp(false)
                return
            }
            setIsTestingErp(false)
        }
        // Step 5 → save ERPNext company mapping
        if (step === 5 && selectedErpCompany) {
            await SaveERPNextMapping(selectedErpCompany)
        }
        // Step 7 → save voucher type mapping
        if (step === 7) {
            await SetMappingConfig('voucher_type', JSON.stringify(vtMapping))
        }
        // Step 8 → save group-based ledger mapping
        if (step === 8) {
            await SetMappingConfig('account_coa', JSON.stringify({ _type: 'group', ...groupMapping }))
        }
        // Step 10 → activate
        if (step === 10) {
            setIsActivating(true)
            setActivateStatus('registering')
            setActivateError('')
            try {
                const regRes = await RegisterWithCloud(cloudUrl, selectedCompany || '')
                if (regRes.status !== 'ok') {
                    setActivateError(regRes.message as string); setActivateStatus('error'); setIsActivating(false); return
                }
                const cid = regRes.companyId as string
                setCompanyId(cid)
                setActivateStatus('syncing')
                const syncRes = await StartBackgroundSync()
                if (syncRes.status !== 'ok') {
                    setActivateError(syncRes.message as string); setActivateStatus('error'); setIsActivating(false); return
                }
                setActivateStatus('done')
                setTimeout(() => onActivated({ companyId: cid, companyName: selectedCompany || '', erpCompany: selectedErpCompany, cloudUrl, erpnextUrl }), 1500)
            } catch (e) {
                setActivateError(String(e)); setActivateStatus('error')
            }
            setIsActivating(false)
            return
        }
        setStep(s => Math.min(s + 1, TOTAL_STEPS))
    }
    const prev = () => setStep(s => Math.max(s - 1, 1))

    const STEP_LABELS = [
        'Welcome', 'Detect Tally', 'Select Company',
        'ERPNext Connect', 'Map Companies', 'Validate Setup',
        'Voucher Mapping', 'Ledger Mapping', 'Data Quality', 'Activate',
    ]

    // ── Can proceed? ──
    const canNext = () => {
        if (isTestingErp || isActivating || activateStatus === 'done') return false
        if (step === 2 && !tallyOnline) return false
        if (step === 3 && (!selectedCompany || !syncFromDate || !syncToDate)) return false
        if (step === 5 && (!selectedErpCompany || step5Loading)) return false
        if (step === 6 && (validating || validationResults.some(r => r.status === 'error'))) return false
        return true
    }

    const allDocTypes = ['Sales Invoice', 'Purchase Invoice', 'Payment Entry', 'Journal Entry', 'Delivery Note', 'Purchase Receipt', 'Sales Invoice (Return)', 'Purchase Invoice (Return)', 'Skip']

    return (
        <div className="flex h-screen bg-neutral-50 text-neutral-900 font-sans">
            {/* Sidebar */}
            <div className="w-52 bg-white border-r p-4 flex flex-col shadow-sm shrink-0">
                <div className="mb-5">
                    <h1 className="text-base font-bold text-blue-600">TallyBridge</h1>
                    <p className="text-xs text-neutral-400">Setup Wizard</p>
                </div>
                <div className="space-y-0.5 flex-1">
                    {STEP_LABELS.map((label, i) => {
                        const n = i + 1; const done = step > n; const active = step === n
                        return (
                            <div key={n} className={`flex items-center gap-2 px-2 py-1.5 rounded-lg text-xs font-medium ${active ? 'text-blue-700 bg-blue-50' : done ? 'text-green-700' : 'text-neutral-400'}`}>
                                <span className={`w-4 h-4 rounded-full text-[10px] flex items-center justify-center shrink-0 font-bold ${done ? 'bg-green-500 text-white' : active ? 'bg-blue-600 text-white' : 'bg-neutral-200 text-neutral-400'}`}>
                                    {done ? '✓' : n}
                                </span>
                                {label}
                            </div>
                        )
                    })}
                </div>
                <div className="text-xs text-neutral-300 mt-2">Step {step} of {TOTAL_STEPS}</div>
            </div>

            {/* Content */}
            <div className="flex-1 flex flex-col items-center justify-center p-8 overflow-y-auto">
                <div className="w-full max-w-xl bg-white rounded-2xl shadow-lg border border-gray-100 p-7 flex flex-col min-h-[480px]">
                    <div className="flex-1">

                        {/* ─ Step 1: Welcome ─ */}
                        {step === 1 && (
                            <div className="space-y-4">
                                <h2 className="text-2xl font-bold">Welcome to TallyBridge</h2>
                                <p className="text-neutral-500 text-sm">This wizard connects your Tally ERP to ERPNext. It takes ~10 minutes.</p>
                                <div className="space-y-2 mt-4">
                                    {['Detect Tally on your network', 'Select company + financial year', 'Connect ERPNext and map companies', 'Map voucher types and ledger accounts', 'Validate setup and activate sync'].map(t => (
                                        <div key={t} className="flex items-center gap-2 text-sm text-neutral-600">
                                            <span className="text-blue-400 font-bold">›</span> {t}
                                        </div>
                                    ))}
                                </div>
                            </div>
                        )}

                        {/* ─ Step 2: Detect Tally ─ */}
                        {step === 2 && (
                            <div className="space-y-5">
                                <h2 className="text-2xl font-bold">Detect Tally</h2>
                                <p className="text-neutral-500 text-sm">Ensure Tally ODBC server is running on port 9000.</p>
                                <div>
                                    <label className="block text-sm font-medium text-neutral-700 mb-1">Tally URL</label>
                                    <div className="flex gap-2">
                                        <input type="url" className="flex-1 border border-gray-300 rounded-lg px-3 py-2 text-sm font-mono focus:ring-2 focus:ring-blue-500 outline-none"
                                            value={tallyUrl} onChange={e => setTallyUrl(e.target.value)} />
                                        <Button size="sm" onClick={handleScanTally} disabled={isScanning}>
                                            {isScanning ? 'Pinging…' : 'Connect'}
                                        </Button>
                                    </div>
                                </div>
                                <StatusRow online={tallyOnline} loading={isScanning}
                                    label={isScanning ? 'Pinging Tally…' : tallyOnline ? 'Tally Detected' : 'Tally Not Found'} />
                            </div>
                        )}

                        {/* ─ Step 3: Select Company ─ */}
                        {step === 3 && (
                            <div className="space-y-4">
                                <div className="flex items-center justify-between">
                                    <h2 className="text-2xl font-bold">Select Company</h2>
                                    <Button size="sm" variant="outline" onClick={handleGetCompanies} disabled={loadingCompanies}>
                                        {loadingCompanies ? '…' : '↻ Refresh'}
                                    </Button>
                                </div>
                                <div className="space-y-2 max-h-44 overflow-y-auto pr-1">
                                    {companies.length === 0 && <p className="text-neutral-400 italic text-sm p-4 border border-dashed rounded-lg text-center">No companies open in Tally.</p>}
                                    {companies.map(c => (
                                        <button key={c} onClick={() => selectCompany(c)}
                                            className={`w-full text-left px-4 py-3 rounded-xl border-2 text-sm font-medium transition-colors ${selectedCompany === c ? 'border-blue-500 bg-blue-50 text-blue-800' : 'border-gray-200 hover:border-gray-300 text-neutral-700'}`}>
                                            <div>{c}</div>
                                            {selectedCompany === c && (
                                                <div className="flex gap-3 mt-1 text-xs text-blue-600">
                                                    {loadingStats ? <span>Loading stats…</span> : (
                                                        <>
                                                            <span>{companyStats.ledgerCount ?? '–'} ledgers</span>
                                                            <span>{companyStats.groupCount ?? '–'} groups</span>
                                                            <span>{companyStats.voucherTypeCount ?? '–'} voucher types</span>
                                                            <span>{companyStats.voucherCount ?? '–'} vouchers</span>
                                                        </>
                                                    )}
                                                </div>
                                            )}
                                        </button>
                                    ))}
                                </div>
                                <div className="border-t pt-4">
                                    <p className="text-sm font-medium text-neutral-700 mb-2">Sync Date Range <span className="text-neutral-400 font-normal text-xs">(auto-detected from company name)</span></p>
                                    <div className="flex items-center gap-3">
                                        <div className="flex-1">
                                            <label className="block text-xs text-neutral-500 mb-1">From</label>
                                            <input type="date" className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                                                value={syncFromDate} onChange={e => setSyncFromDate(e.target.value)} />
                                        </div>
                                        <span className="text-neutral-400 mt-4">→</span>
                                        <div className="flex-1">
                                            <label className="block text-xs text-neutral-500 mb-1">To</label>
                                            <input type="date" className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                                                value={syncToDate} onChange={e => setSyncToDate(e.target.value)} />
                                        </div>
                                    </div>
                                </div>
                            </div>
                        )}

                        {/* ─ Step 4: ERPNext Connect ─ */}
                        {step === 4 && (
                            <div className="space-y-4">
                                <h2 className="text-2xl font-bold">Connect ERPNext</h2>
                                {erpConnectionError && <ErrorBox msg={erpConnectionError} />}
                                <div className="space-y-3">
                                    <Field label="ERPNext URL" type="url" value={erpnextUrl} onChange={setErpnextUrl} disabled={isTestingErp} />
                                    <div className="grid grid-cols-2 gap-3">
                                        <Field label="API Key" type="password" mono value={apiKey} onChange={setApiKey} disabled={isTestingErp} />
                                        <Field label="API Secret" type="password" mono value={apiSecret} onChange={setApiSecret} disabled={isTestingErp} />
                                    </div>
                                    <Field label="TallyBridge Cloud URL" type="url" mono value={cloudUrl} onChange={setCloudUrl} disabled={isTestingErp}
                                        hint="Middleware server (default: http://localhost:3000)" />
                                </div>
                            </div>
                        )}

                        {/* ─ Step 5: Map Companies + Fiscal Year ─ */}
                        {step === 5 && (
                            <div className="space-y-4">
                                <h2 className="text-2xl font-bold">Map to ERPNext</h2>
                                <p className="text-neutral-500 text-sm">Select which ERPNext company this Tally company maps to, and verify fiscal year coverage.</p>
                                {step5Loading ? <Spinner label="Loading ERPNext data…" /> : (
                                    <>
                                        <div>
                                            <label className="block text-sm font-medium text-neutral-700 mb-1">Tally Company</label>
                                            <div className="px-3 py-2 border border-neutral-200 rounded-lg text-sm text-neutral-600 bg-neutral-50">{selectedCompany}</div>
                                        </div>
                                        <div>
                                            <label className="block text-sm font-medium text-neutral-700 mb-1">ERPNext Company</label>
                                            <select className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                                                value={selectedErpCompany} onChange={e => setSelectedErpCompany(e.target.value)}>
                                                <option value="">— Select company —</option>
                                                {erpCompanies.map(c => <option key={c} value={c}>{c}</option>)}
                                            </select>
                                        </div>
                                        <div>
                                            <label className="block text-sm font-medium text-neutral-700 mb-1">Fiscal Years in ERPNext</label>
                                            {erpFiscalYears.length === 0
                                                ? <p className="text-sm text-neutral-400 italic">No fiscal years found.</p>
                                                : <div className="space-y-1 max-h-28 overflow-y-auto">
                                                    {erpFiscalYears.map(fy => (
                                                        <div key={fy.name} className={`flex justify-between text-xs px-3 py-2 rounded-lg border ${fy.yearStart <= syncFromDate && fy.yearEnd >= syncToDate ? 'border-green-200 bg-green-50 text-green-700' : 'border-neutral-200 text-neutral-500'}`}>
                                                            <span className="font-medium">{fy.name}</span>
                                                            <span>{fy.yearStart} → {fy.yearEnd}</span>
                                                        </div>
                                                    ))}
                                                </div>
                                            }
                                            {!fyCovers && syncFromDate && syncToDate && (
                                                <div className="mt-2 flex items-center justify-between p-3 bg-yellow-50 border border-yellow-200 rounded-lg text-xs">
                                                    <span className="text-yellow-700">No fiscal year covers {syncFromDate} → {syncToDate}</span>
                                                    <Button size="sm" disabled={creatingFY} onClick={async () => {
                                                        setCreatingFY(true)
                                                        const yearName = syncFromDate.slice(0, 4) + '-' + syncToDate.slice(2, 4)
                                                        await CreateERPNextFiscalYear(yearName, syncFromDate, syncToDate, selectedErpCompany)
                                                        await loadStep5()
                                                        setCreatingFY(false)
                                                    }}>
                                                        {creatingFY ? 'Creating…' : 'Create FY'}
                                                    </Button>
                                                </div>
                                            )}
                                        </div>
                                    </>
                                )}
                            </div>
                        )}

                        {/* ─ Step 6: Validate Setup ─ */}
                        {step === 6 && (
                            <div className="space-y-4">
                                <div className="flex items-center justify-between">
                                    <h2 className="text-2xl font-bold">Validate Setup</h2>
                                    <Button size="sm" variant="outline" onClick={runValidation} disabled={validating}>
                                        {validating ? 'Running…' : '↻ Re-check'}
                                    </Button>
                                </div>
                                {validating ? <Spinner label="Checking ERPNext environment…" /> : (
                                    <div className="space-y-2">
                                        {validationResults.map((item, i) => (
                                            <div key={i} className="flex items-start justify-between p-3 border rounded-lg bg-neutral-50 text-sm gap-3">
                                                <div>
                                                    <div className="font-medium text-neutral-800">{item.label}</div>
                                                    {item.message && <div className="text-xs text-neutral-400 mt-0.5">{item.message}</div>}
                                                </div>
                                                <Badge variant={item.status === 'ok' ? 'success' : item.status === 'warn' ? 'warning' : 'error'}>
                                                    {item.status === 'ok' ? 'Pass' : item.status === 'warn' ? 'Warn' : 'Fail'}
                                                </Badge>
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>
                        )}

                        {/* ─ Step 7: Voucher Type Mapping ─ */}
                        {step === 7 && (
                            <div className="space-y-4">
                                <h2 className="text-2xl font-bold">Voucher Type Mapping</h2>
                                <p className="text-neutral-500 text-sm">Map each Tally voucher type to an ERPNext DocType. Defaults are pre-filled.</p>
                                <div className="max-h-72 overflow-y-auto border rounded-lg">
                                    <table className="w-full text-xs">
                                        <thead className="bg-neutral-50 border-b sticky top-0">
                                            <tr>
                                                <th className="px-3 py-2 text-left font-semibold text-neutral-600">Tally Type</th>
                                                <th className="px-3 py-2 text-left font-semibold text-neutral-600">Nature</th>
                                                <th className="px-3 py-2 text-left font-semibold text-neutral-600">ERPNext DocType</th>
                                            </tr>
                                        </thead>
                                        <tbody className="divide-y bg-white">
                                            {tallyVTypes.length === 0
                                                ? <tr><td colSpan={3} className="px-3 py-4 text-center text-neutral-400">Loading voucher types…</td></tr>
                                                : tallyVTypes.map(vt => (
                                                    <tr key={vt.name}>
                                                        <td className="px-3 py-2 font-medium text-neutral-800">{vt.name}</td>
                                                        <td className="px-3 py-2 text-neutral-400">{vt.nature || '—'}</td>
                                                        <td className="px-3 py-2">
                                                            <select className="border border-gray-300 rounded px-2 py-1 text-xs w-full focus:ring-1 focus:ring-blue-500 outline-none"
                                                                value={vtMapping[vt.name] || ''}
                                                                onChange={e => setVtMapping(m => ({ ...m, [vt.name]: e.target.value }))}>
                                                                <option value="">— Select —</option>
                                                                {(NATURE_TO_DOCTYPE[vt.nature] || allDocTypes).map(d => <option key={d} value={d}>{d}</option>)}
                                                            </select>
                                                        </td>
                                                    </tr>
                                                ))}
                                        </tbody>
                                    </table>
                                </div>
                            </div>
                        )}

                        {/* ─ Step 8: Ledger Reconciliation ─ */}
                        {step === 8 && (() => {
                            // Derive groups with ledger counts
                            const groupCount: Record<string, number> = {}
                            for (const l of tallyLedgers) {
                                groupCount[l.parent] = (groupCount[l.parent] || 0) + 1
                            }
                            const groups = Object.entries(groupCount).sort(([a], [b]) => a.localeCompare(b))
                            // Unmapped groups (no suggestion + user hasn't set one)
                            const exceptions = groups.filter(([g]) => !groupMapping[g])
                            return (
                                <div className="space-y-3">
                                    <h2 className="text-2xl font-bold">Ledger Group Mapping</h2>
                                    <p className="text-neutral-500 text-sm">Map each Tally ledger group to an ERPNext account. All ledgers in a group inherit this mapping. Smart defaults are pre-filled.</p>
                                    {step8Loading ? <Spinner label="Loading ledgers and accounts…" /> : (
                                        <>
                                            <div className="max-h-64 overflow-y-auto border rounded-lg">
                                                <table className="w-full text-xs">
                                                    <thead className="bg-neutral-50 border-b sticky top-0">
                                                        <tr>
                                                            <th className="px-3 py-2 text-left font-semibold text-neutral-600">Tally Group</th>
                                                            <th className="px-3 py-2 text-center font-semibold text-neutral-600 w-14">Ledgers</th>
                                                            <th className="px-3 py-2 text-left font-semibold text-neutral-600">ERPNext Account</th>
                                                        </tr>
                                                    </thead>
                                                    <tbody className="divide-y bg-white">
                                                        {groups.map(([group, count]) => (
                                                            <tr key={group} className={groupMapping[group] ? '' : 'bg-yellow-50/60'}>
                                                                <td className="px-3 py-1.5 font-medium text-neutral-800 max-w-[160px] truncate" title={group}>{group}</td>
                                                                <td className="px-3 py-1.5 text-center text-neutral-400">{count}</td>
                                                                <td className="px-3 py-1.5">
                                                                    <select className={`border rounded px-2 py-1 text-xs w-full focus:ring-1 focus:ring-blue-500 outline-none ${groupMapping[group] ? 'border-green-300 bg-green-50' : 'border-yellow-300'}`}
                                                                        value={groupMapping[group] || ''}
                                                                        onChange={e => setGroupMapping(m => ({ ...m, [group]: e.target.value }))}>
                                                                        <option value="">— select account —</option>
                                                                        {erpAccounts.map(a => <option key={a.name} value={a.name}>{a.name}</option>)}
                                                                    </select>
                                                                </td>
                                                            </tr>
                                                        ))}
                                                    </tbody>
                                                </table>
                                            </div>
                                            {exceptions.length > 0 && (
                                                <div className="p-2.5 bg-yellow-50 border border-yellow-200 rounded-lg text-xs text-yellow-800">
                                                    <strong>{exceptions.length} group{exceptions.length > 1 ? 's' : ''} unmapped</strong> — these will use ERPNext auto-detect: {exceptions.map(([g]) => g).join(', ')}
                                                </div>
                                            )}
                                            {exceptions.length === 0 && groups.length > 0 && (
                                                <div className="p-2.5 bg-green-50 border border-green-200 rounded-lg text-xs text-green-700">All {groups.length} groups mapped. You can still proceed with partial mappings.</div>
                                            )}
                                        </>
                                    )}
                                </div>
                            )
                        })()}

                        {/* ─ Step 9: Data Quality ─ */}
                        {step === 9 && (
                            <div className="space-y-4">
                                <h2 className="text-2xl font-bold">Data Quality Check</h2>
                                <p className="text-neutral-500 text-sm">Issues found in Tally data that may affect sync quality.</p>
                                {!qualityResults ? <Spinner label="Scanning Tally data…" /> : (
                                    <div className="space-y-4">
                                        <div className="grid grid-cols-2 gap-3">
                                            <StatCard label="Ledgers" value={qualityResults.ledgerTotal} color="blue" />
                                            <StatCard label="Stock Items" value={qualityResults.stockItemTotal} color="blue" />
                                        </div>
                                        <table className="w-full text-sm border border-gray-200 rounded-lg overflow-hidden">
                                            <thead className="bg-gray-50">
                                                <tr>
                                                    <th className="px-4 py-2 text-left text-neutral-600 font-semibold">Check</th>
                                                    <th className="px-4 py-2 text-right text-neutral-600 font-semibold">Issues</th>
                                                    <th className="px-4 py-2 text-left text-neutral-600 font-semibold">Action</th>
                                                </tr>
                                            </thead>
                                            <tbody className="divide-y divide-gray-100">
                                                <tr className={qualityResults.missingGSTIN > 0 ? 'bg-yellow-50' : ''}>
                                                    <td className="px-4 py-2">Missing GSTIN (GST-registered parties)</td>
                                                    <td className="px-4 py-2 text-right font-mono">{qualityResults.missingGSTIN > 0 ? <span className="text-yellow-700 font-bold">{qualityResults.missingGSTIN}</span> : <span className="text-green-600">0 ✓</span>}</td>
                                                    <td className="px-4 py-2 text-neutral-500 text-xs">{qualityResults.missingGSTIN > 0 ? 'Fill GSTIN in Tally Ledger master' : ''}</td>
                                                </tr>
                                                <tr className={qualityResults.missingUOM > 0 ? 'bg-yellow-50' : ''}>
                                                    <td className="px-4 py-2">Missing UOM on stock items</td>
                                                    <td className="px-4 py-2 text-right font-mono">{qualityResults.missingUOM > 0 ? <span className="text-yellow-700 font-bold">{qualityResults.missingUOM}</span> : <span className="text-green-600">0 ✓</span>}</td>
                                                    <td className="px-4 py-2 text-neutral-500 text-xs">{qualityResults.missingUOM > 0 ? 'Set Base Units in Stock Item master' : ''}</td>
                                                </tr>
                                                <tr className={qualityResults.missingHSN > 0 ? 'bg-yellow-50' : ''}>
                                                    <td className="px-4 py-2">Missing HSN code on stock items</td>
                                                    <td className="px-4 py-2 text-right font-mono">{qualityResults.missingHSN > 0 ? <span className="text-yellow-700 font-bold">{qualityResults.missingHSN}</span> : <span className="text-green-600">0 ✓</span>}</td>
                                                    <td className="px-4 py-2 text-neutral-500 text-xs">{qualityResults.missingHSN > 0 ? 'Add HSN/SAC code in Stock Item master' : ''}</td>
                                                </tr>
                                            </tbody>
                                        </table>
                                        {qualityResults.warnCount === 0
                                            ? <div className="p-3 bg-green-50 border border-green-200 rounded-lg text-sm text-green-700">All checks passed. Data quality is excellent.</div>
                                            : <div className="p-3 bg-yellow-50 border border-yellow-200 rounded-lg text-sm text-yellow-700"><strong>{qualityResults.warnCount}</strong> issues found. Fix them in Tally for best sync results. You can still proceed.</div>
                                        }
                                    </div>
                                )}
                            </div>
                        )}

                        {/* ─ Step 10: Activate ─ */}
                        {step === 10 && (
                            <div className="space-y-4">
                                {activateStatus === 'idle' && (
                                    <div className="text-center py-4 space-y-4">
                                        <div className="w-14 h-14 bg-blue-100 rounded-full flex items-center justify-center mx-auto">
                                            <svg className="w-7 h-7 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                                            </svg>
                                        </div>
                                        <h2 className="text-2xl font-bold">Ready to Activate</h2>
                                        <div className="bg-neutral-50 rounded-lg p-3 text-left text-xs space-y-1 border">
                                            <Row label="Tally" value={selectedCompany || ''} />
                                            <Row label="ERPNext" value={selectedErpCompany} />
                                            <Row label="Date Range" value={`${syncFromDate} → ${syncToDate}`} />
                                            <Row label="Cloud" value={cloudUrl} mono />
                                        </div>
                                    </div>
                                )}
                                {(activateStatus === 'registering' || activateStatus === 'syncing') && (
                                    <div className="text-center py-8 space-y-4">
                                        <Spinner label={activateStatus === 'registering' ? 'Registering with cloud…' : 'Starting sync scheduler…'} />
                                    </div>
                                )}
                                {activateStatus === 'done' && (
                                    <div className="text-center py-4 space-y-3">
                                        <div className="w-14 h-14 bg-green-100 rounded-full flex items-center justify-center mx-auto">
                                            <svg className="w-7 h-7 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
                                            </svg>
                                        </div>
                                        <h2 className="text-2xl font-bold">TallyBridge Active</h2>
                                        <p className="text-neutral-500 text-sm">Opening dashboard…</p>
                                        <div className="bg-green-50 rounded-lg p-3 text-xs border border-green-100 text-left space-y-0.5">
                                            <div className="text-green-700">Company ID: <span className="font-mono">{companyId}</span></div>
                                            <div className="text-green-700">Sync interval: every 5 minutes</div>
                                        </div>
                                    </div>
                                )}
                                {activateStatus === 'error' && (
                                    <div className="text-center py-4 space-y-3">
                                        <div className="w-14 h-14 bg-red-100 rounded-full flex items-center justify-center mx-auto">
                                            <svg className="w-7 h-7 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                            </svg>
                                        </div>
                                        <h2 className="text-xl font-bold">Activation Failed</h2>
                                        <ErrorBox msg={activateError} />
                                    </div>
                                )}
                            </div>
                        )}
                    </div>

                    {/* Navigation */}
                    <div className="mt-6 flex justify-between pt-5 border-t border-gray-100">
                        <Button variant="outline" onClick={prev} disabled={step === 1 || isTestingErp || isActivating}>Back</Button>
                        <Button onClick={next} disabled={!canNext()}>
                            {(isTestingErp || isActivating) && <span className="w-3 h-3 border-2 border-white border-t-transparent rounded-full animate-spin mr-2 inline-block"></span>}
                            {step === 10 ? (activateStatus === 'error' ? 'Retry' : 'Activate') : 'Next →'}
                        </Button>
                    </div>
                </div>
            </div>
        </div>
    )
}

// ─── Dashboard ────────────────────────────────────────────────────────────────

function Dashboard({ result, onAddCompany }: { result: ActivationResult; onAddCompany: () => void }) {
    const [tab, setTab] = useState<DashboardTab>('overview')

    return (
        <div className="flex h-screen bg-neutral-50 text-neutral-900 font-sans">
            <div className="w-52 bg-white border-r flex flex-col shadow-sm shrink-0">
                <div className="px-4 py-4 border-b">
                    <span className="text-base font-bold text-blue-600">TallyBridge</span>
                    <div className="flex items-center gap-1.5 mt-0.5">
                        <span className="w-2 h-2 rounded-full bg-green-500 inline-block animate-pulse"></span>
                        <span className="text-xs text-green-700 font-medium">Active</span>
                    </div>
                </div>
                <nav className="flex-1 p-3 space-y-0.5">
                    {(['overview', 'mappings', 'settings'] as DashboardTab[]).map(id => (
                        <button key={id} onClick={() => setTab(id)}
                            className={`w-full text-left px-3 py-2 rounded-lg text-sm font-medium capitalize transition-colors ${tab === id ? 'bg-blue-50 text-blue-700' : 'text-neutral-600 hover:bg-neutral-100'}`}>
                            {id}
                        </button>
                    ))}
                </nav>
                <div className="p-3 border-t space-y-2">
                    <div className="text-xs text-neutral-400 truncate" title={result.companyName}>{result.companyName}</div>
                    <div className="text-xs text-neutral-400 truncate" title={result.erpCompany}>→ {result.erpCompany}</div>
                    <Button size="sm" variant="outline" className="w-full text-xs" onClick={onAddCompany}>+ Add Company</Button>
                </div>
            </div>
            <div className="flex-1 overflow-y-auto">
                {tab === 'overview' && <OverviewTab result={result} />}
                {tab === 'mappings' && <MappingsTab result={result} />}
                {tab === 'settings' && <SettingsTab result={result} />}
            </div>
        </div>
    )
}

// ─── Overview Tab ─────────────────────────────────────────────────────────────

function OverviewTab({ result }: { result: ActivationResult }) {
    const [services, setServices] = useState<Record<string, { online: boolean; url?: string; company?: string }>>({})
    const [syncStats, setSyncStats] = useState<Record<string, string>>({})
    const [loading, setLoading] = useState(true)
    const [syncing, setSyncing] = useState(false)
    const [syncMsg, setSyncMsg] = useState<{ type: 'ok' | 'error'; text: string } | null>(null)

    const refresh = useCallback(async () => {
        setLoading(true)
        const [svc, stats] = await Promise.all([
            GetServiceStatus().catch(() => ({})),
            GetSyncStats().catch(() => ({})),
        ])
        setServices(svc as Record<string, { online: boolean; url?: string }>)
        setSyncStats(stats as Record<string, string>)
        setLoading(false)
    }, [])

    useEffect(() => { refresh() }, [refresh])

    const runSync = async (fullSync: boolean) => {
        setSyncing(true)
        setSyncMsg(null)
        try {
            const res = await SyncNow(fullSync)
            if (res.status === 'ok') {
                setSyncMsg({ type: 'ok', text: res.message as string })
                // Refresh stats after sync
                const stats = await GetSyncStats().catch(() => ({}))
                setSyncStats(stats as Record<string, string>)
            } else {
                setSyncMsg({ type: 'error', text: res.message as string })
            }
        } catch (e) {
            setSyncMsg({ type: 'error', text: String(e) })
        }
        setSyncing(false)
    }

    const formatTS = (ts: string) => {
        if (!ts || ts.length < 14) return '—'
        return `${ts.slice(0,4)}-${ts.slice(4,6)}-${ts.slice(6,8)} ${ts.slice(8,10)}:${ts.slice(10,12)}`
    }

    const tallyHeartbeatCompany = (services.tally as { company?: string })?.company || ''

    return (
        <div className="p-6 space-y-5 max-w-3xl">
            <div className="flex items-center justify-between">
                <h2 className="text-lg font-semibold">Overview</h2>
                <Button size="sm" variant="outline" onClick={refresh} disabled={loading}>↻ Refresh</Button>
            </div>

            {/* Service Status */}
            <div>
                <h3 className="text-xs font-semibold text-neutral-500 uppercase tracking-wide mb-2">Service Status</h3>
                <div className="grid grid-cols-3 gap-3">
                    {/* Tally card — shows configured company + heartbeat detail */}
                    {(() => {
                        const s = services['tally'] as { online?: boolean } | undefined
                        return (
                            <Card>
                                <CardContent>
                                    <div className="flex items-center gap-2 mb-1">
                                        <span className={`w-2 h-2 rounded-full ${loading ? 'bg-neutral-300' : s?.online ? 'bg-green-500' : 'bg-red-400'}`}></span>
                                        <span className="text-xs font-semibold text-neutral-700">Tally ERP</span>
                                    </div>
                                    <Badge variant={loading ? 'default' : s?.online ? 'success' : 'error'}>
                                        {loading ? '…' : s?.online ? 'Online' : 'Offline'}
                                    </Badge>
                                    {/* Show the TallyBridge-configured company, not the heartbeat window */}
                                    <p className="text-xs text-neutral-600 mt-1 truncate font-medium" title={result.companyName}>{result.companyName}</p>
                                    {tallyHeartbeatCompany && tallyHeartbeatCompany !== result.companyName && (
                                        <p className="text-[10px] text-neutral-400 mt-0.5 truncate" title={tallyHeartbeatCompany}>Active window: {tallyHeartbeatCompany}</p>
                                    )}
                                </CardContent>
                            </Card>
                        )
                    })()}
                    {[
                        { key: 'erpnext', label: 'ERPNext', detail: result.erpnextUrl },
                        { key: 'cloud', label: 'Cloud', detail: result.cloudUrl },
                    ].map(svc => {
                        const s = services[svc.key] as { online?: boolean } | undefined
                        return (
                            <Card key={svc.key}>
                                <CardContent>
                                    <div className="flex items-center gap-2 mb-1">
                                        <span className={`w-2 h-2 rounded-full ${loading ? 'bg-neutral-300' : s?.online ? 'bg-green-500' : 'bg-red-400'}`}></span>
                                        <span className="text-xs font-semibold text-neutral-700">{svc.label}</span>
                                    </div>
                                    <Badge variant={loading ? 'default' : s?.online ? 'success' : 'error'}>
                                        {loading ? '…' : s?.online ? 'Online' : 'Offline'}
                                    </Badge>
                                    {svc.detail && <p className="text-xs text-neutral-400 mt-1 truncate" title={svc.detail}>{svc.detail}</p>}
                                </CardContent>
                            </Card>
                        )
                    })}
                </div>
            </div>

            {/* Manual Sync */}
            <div>
                <h3 className="text-xs font-semibold text-neutral-500 uppercase tracking-wide mb-2">Manual Sync</h3>
                <Card>
                    <CardContent>
                        <div className="space-y-3">
                            <div className="flex items-center gap-3">
                                <Button size="sm" onClick={() => runSync(false)} disabled={syncing}>
                                    {syncing ? <><span className="w-3 h-3 border-2 border-white border-t-transparent rounded-full animate-spin inline-block mr-1"></span>Syncing…</> : '↑ Sync Now'}
                                </Button>
                                <Button size="sm" variant="outline" onClick={() => runSync(true)} disabled={syncing}>
                                    Full Sync (reset watermark)
                                </Button>
                                <span className="text-xs text-neutral-400 flex-1">
                                    Sync Now: push new/changed vouchers only. Full Sync: re-send all vouchers from {syncStats.companyName ? '' : 'the configured FY'}.
                                </span>
                            </div>
                            {syncMsg && (
                                <div className={`px-3 py-2 rounded-lg text-xs ${syncMsg.type === 'ok' ? 'bg-green-50 text-green-700 border border-green-200' : 'bg-red-50 text-red-600 border border-red-200'}`}>
                                    {syncMsg.text}
                                </div>
                            )}
                        </div>
                    </CardContent>
                </Card>
            </div>

            {/* Sync Stats */}
            <div>
                <h3 className="text-xs font-semibold text-neutral-500 uppercase tracking-wide mb-2">Sync Statistics</h3>
                <div className="grid grid-cols-2 gap-3">
                    <Card>
                        <CardHeader><CardTitle>Last Sync</CardTitle></CardHeader>
                        <CardContent>
                            <p className="text-sm font-mono text-neutral-700">{formatTS(syncStats.lastSyncAt)}</p>
                        </CardContent>
                    </Card>
                    <Card>
                        <CardHeader><CardTitle>Total Synced</CardTitle></CardHeader>
                        <CardContent>
                            <p className="text-2xl font-bold text-blue-600">{syncStats.totalSynced || '0'}</p>
                            <p className="text-xs text-neutral-400">vouchers pushed to cloud</p>
                        </CardContent>
                    </Card>
                </div>
            </div>

            {/* Company Info */}
            <div>
                <h3 className="text-xs font-semibold text-neutral-500 uppercase tracking-wide mb-2">Company</h3>
                <Card>
                    <CardContent>
                        <div className="flex flex-col gap-1.5 text-sm">
                            <Row label="Tally" value={result.companyName} />
                            <Row label="ERPNext" value={result.erpCompany} />
                            <Row label="Company ID" value={result.companyId} mono />
                        </div>
                    </CardContent>
                </Card>
            </div>
        </div>
    )
}

// ─── Mappings Tab ─────────────────────────────────────────────────────────────

const MAPPING_SECTIONS = [
    { key: 'account_coa', label: 'Account / CoA', description: 'Tally ledger → ERPNext account' },
    { key: 'ledger_group', label: 'Ledger Groups', description: 'Ledger group → account type' },
    { key: 'gst_ledger', label: 'GST Ledgers', description: 'GST component ledgers → tax accounts' },
    { key: 'voucher_type', label: 'Voucher Types', description: 'Voucher type → ERPNext DocType' },
    { key: 'item_hsn', label: 'Items / HSN', description: 'Stock item → ERPNext item + HSN' },
    { key: 'uom', label: 'Units of Measure', description: 'Tally UoM → ERPNext UoM' },
    { key: 'party_identity', label: 'Party Identity', description: 'GSTIN / PAN deduplication rules' },
    { key: 'sync_direction', label: 'Sync Direction', description: 'Voucher direction + series' },
    { key: 'conflict_resolution', label: 'Conflict Resolution', description: 'Strategy when both sides change' },
    { key: 'historical_import', label: 'Historical Import', description: 'Batch import config and filters' },
    { key: 'exception_handling', label: 'Exception Handling', description: 'Error and retry rules' },
]

function MappingsTab({ result }: { result: ActivationResult }) {
    const [activeSection, setActiveSection] = useState('account_coa')
    const [jsonValue, setJsonValue] = useState('')
    const [loading, setLoading] = useState(false)
    const [saving, setSaving] = useState(false)
    const [saveStatus, setSaveStatus] = useState<'idle' | 'saved' | 'error'>('idle')
    const [jsonError, setJsonError] = useState('')
    const [showReconcile, setShowReconcile] = useState(false)

    // Reconciliation state
    const [tallyLedgers, setTallyLedgers] = useState<{ name: string; parent: string; gstin: string }[]>([])
    const [erpAccounts, setErpAccounts] = useState<{ name: string; accountType: string; rootType: string }[]>([])
    const [reconcileMap, setReconcileMap] = useState<Record<string, string>>({})
    const [reconcileLoading, setReconcileLoading] = useState(false)
    const [reconcileSearch, setReconcileSearch] = useState('')

    const loadSection = useCallback(async (key: string) => {
        setLoading(true); setSaveStatus('idle'); setJsonError('')
        try {
            const raw = await GetMappingConfig(key)
            try { setJsonValue(JSON.stringify(JSON.parse(raw), null, 2)) }
            catch { setJsonValue(raw || '{}') }
        } catch { setJsonValue('{}') }
        setLoading(false)
    }, [])

    useEffect(() => { loadSection(activeSection) }, [activeSection, loadSection])

    const handleSave = async () => {
        setJsonError('')
        try { JSON.parse(jsonValue) } catch (e) { setJsonError('Invalid JSON: ' + String(e)); return }
        setSaving(true)
        try {
            await SetMappingConfig(activeSection, jsonValue)
            setSaveStatus('saved')
            setTimeout(() => setSaveStatus('idle'), 2000)
        } catch (e) { setJsonError('Save failed: ' + String(e)); setSaveStatus('error') }
        setSaving(false)
    }

    const loadReconcile = async () => {
        setReconcileLoading(true)
        const [ledgers, accounts] = await Promise.all([
            GetTallyLedgers(result.companyName).catch(() => []),
            GetERPNextAccounts(result.erpCompany).catch(() => []),
        ])
        setTallyLedgers((ledgers || []) as { name: string; parent: string; gstin: string }[])
        setErpAccounts((accounts || []) as { name: string; accountType: string; rootType: string }[])
        // Pre-fill from saved mapping
        try {
            const saved = await GetMappingConfig('account_coa')
            const parsed = JSON.parse(saved)
            if (typeof parsed === 'object') setReconcileMap(parsed)
        } catch { /* ignore */ }
        setReconcileLoading(false)
    }

    const saveReconcileMap = async () => {
        setSaving(true)
        await SetMappingConfig('account_coa', JSON.stringify(reconcileMap))
        setSaveStatus('saved')
        setTimeout(() => setSaveStatus('idle'), 2000)
        setSaving(false)
    }

    const section = MAPPING_SECTIONS.find(s => s.key === activeSection)

    return (
        <div className="flex h-full">
            {/* Section sidebar */}
            <div className="w-44 border-r bg-white shrink-0 overflow-y-auto">
                <div className="p-2.5 border-b">
                    <p className="text-xs font-semibold text-neutral-500 uppercase tracking-wide">Sections</p>
                </div>
                <nav className="p-2 space-y-0.5">
                    {MAPPING_SECTIONS.map(s => (
                        <button key={s.key} onClick={() => { setActiveSection(s.key); setShowReconcile(false) }}
                            className={`w-full text-left px-2.5 py-2 rounded-lg text-xs font-medium transition-colors ${activeSection === s.key && !showReconcile ? 'bg-blue-50 text-blue-700' : 'text-neutral-600 hover:bg-neutral-100'}`}>
                            {s.label}
                        </button>
                    ))}
                    <div className="pt-2 border-t mt-2">
                        <button onClick={() => { setShowReconcile(true); loadReconcile() }}
                            className={`w-full text-left px-2.5 py-2 rounded-lg text-xs font-medium transition-colors ${showReconcile ? 'bg-purple-50 text-purple-700' : 'text-neutral-600 hover:bg-neutral-100'}`}>
                            ⇄ Reconcile
                        </button>
                    </div>
                </nav>
            </div>

            {/* Editor / Reconcile */}
            <div className="flex-1 flex flex-col p-5 overflow-hidden">
                {!showReconcile ? (
                    <>
                        <div className="mb-3 flex items-start justify-between gap-4">
                            <div>
                                <h3 className="text-sm font-semibold text-neutral-800">{section?.label}</h3>
                                <p className="text-xs text-neutral-500 mt-0.5">{section?.description}</p>
                            </div>
                            <div className="flex items-center gap-2 shrink-0">
                                {saveStatus === 'saved' && <Badge variant="success">Saved</Badge>}
                                {saveStatus === 'error' && <Badge variant="error">Error</Badge>}
                                <Button size="sm" variant="outline" onClick={async () => {
                                    const defaults = await GetMappingDefaults(activeSection).catch(() => '{}')
                                    try { setJsonValue(JSON.stringify(JSON.parse(defaults), null, 2)) }
                                    catch { setJsonValue(defaults) }
                                    setSaveStatus('idle')
                                }} disabled={loading}>
                                    Load Defaults
                                </Button>
                                <Button size="sm" onClick={handleSave} disabled={saving || loading}>
                                    {saving ? 'Saving…' : 'Save'}
                                </Button>
                            </div>
                        </div>
                        {jsonError && <div className="mb-2 px-3 py-2 bg-red-50 border border-red-200 text-red-600 text-xs rounded-lg">{jsonError}</div>}
                        {loading
                            ? <div className="flex-1 flex items-center justify-center text-neutral-400 text-sm">Loading…</div>
                            : <textarea className="flex-1 w-full font-mono text-xs border border-neutral-200 rounded-lg p-3 resize-none focus:outline-none focus:ring-2 focus:ring-blue-500 bg-neutral-50 text-neutral-800"
                                value={jsonValue} onChange={e => { setJsonValue(e.target.value); setSaveStatus('idle') }} spellCheck={false} />
                        }
                        <p className="text-xs text-neutral-400 mt-2">Edit JSON config, then Save. Changes apply on next sync cycle.</p>
                    </>
                ) : (
                    /* Reconciliation view */
                    <>
                        <div className="mb-3 flex items-center justify-between">
                            <div>
                                <h3 className="text-sm font-semibold text-neutral-800">Ledger ⇄ Account Reconciliation</h3>
                                <p className="text-xs text-neutral-500 mt-0.5">Match Tally ledgers to ERPNext accounts</p>
                            </div>
                            <div className="flex items-center gap-2">
                                {saveStatus === 'saved' && <Badge variant="success">Saved</Badge>}
                                <Button size="sm" onClick={saveReconcileMap} disabled={saving || reconcileLoading}>
                                    {saving ? 'Saving…' : 'Save Mapping'}
                                </Button>
                            </div>
                        </div>
                        <input type="text" placeholder="Search ledger…"
                            className="mb-2 w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                            value={reconcileSearch} onChange={e => setReconcileSearch(e.target.value)} />
                        {reconcileLoading ? <Spinner label="Loading Tally ledgers and ERPNext accounts…" /> : (
                            <div className="flex-1 overflow-y-auto border rounded-lg">
                                <table className="w-full text-xs">
                                    <thead className="bg-neutral-50 border-b sticky top-0">
                                        <tr>
                                            <th className="px-3 py-2 text-left font-semibold text-neutral-600 w-1/3">Tally Ledger</th>
                                            <th className="px-3 py-2 text-left font-semibold text-neutral-600 w-1/4">Group</th>
                                            <th className="px-3 py-2 text-left font-semibold text-neutral-600">ERPNext Account</th>
                                        </tr>
                                    </thead>
                                    <tbody className="divide-y bg-white">
                                        {tallyLedgers
                                            .filter(l => !reconcileSearch || l.name.toLowerCase().includes(reconcileSearch.toLowerCase()))
                                            .slice(0, 100)
                                            .map(l => (
                                                <tr key={l.name} className={reconcileMap[l.name] ? 'bg-blue-50/30' : ''}>
                                                    <td className="px-3 py-1.5 font-medium text-neutral-800 max-w-[140px]">
                                                        <div className="truncate" title={l.name}>{l.name}</div>
                                                        {l.gstin && <div className="text-neutral-400 font-mono text-[10px]">{l.gstin}</div>}
                                                    </td>
                                                    <td className="px-3 py-1.5 text-neutral-400 max-w-[100px] truncate" title={l.parent}>{l.parent}</td>
                                                    <td className="px-3 py-1.5">
                                                        <select className={`border rounded px-2 py-1 text-xs w-full focus:ring-1 focus:ring-blue-500 outline-none ${reconcileMap[l.name] ? 'border-blue-300 bg-blue-50' : 'border-gray-200'}`}
                                                            value={reconcileMap[l.name] || ''}
                                                            onChange={e => setReconcileMap(m => ({ ...m, [l.name]: e.target.value }))}>
                                                            <option value="">— Auto-detect —</option>
                                                            {erpAccounts.map(a => <option key={a.name} value={a.name}>{a.name}</option>)}
                                                        </select>
                                                    </td>
                                                </tr>
                                            ))}
                                    </tbody>
                                </table>
                                {tallyLedgers.length > 100 && (
                                    <p className="text-xs text-neutral-400 text-center py-2">Showing 100 of {tallyLedgers.length} — search to filter</p>
                                )}
                            </div>
                        )}
                    </>
                )}
            </div>
        </div>
    )
}

// ─── Settings Tab ─────────────────────────────────────────────────────────────

function SettingsTab({ result }: { result: ActivationResult }) {
    const [config, setConfig] = useState<Record<string, string>>({})
    const [editing, setEditing] = useState<Record<string, string>>({})
    const [saved, setSaved] = useState<Record<string, boolean>>({})
    const [loading, setLoading] = useState(true)

    useEffect(() => {
        GetAllConfig().then(cfg => {
            setConfig(cfg as Record<string, string>)
            setLoading(false)
        }).catch(() => setLoading(false))
    }, [result])

    type SettingGroup = { title: string; keys: string[]; labels: Record<string, string>; editable: Record<string, boolean>; mask?: Record<string, boolean> }
    const SETTING_GROUPS: SettingGroup[] = [
        {
            title: 'Connection',
            keys: ['erpnext_url', 'cloud_url'],
            labels: { erpnext_url: 'ERPNext URL', cloud_url: 'Cloud URL' },
            editable: { erpnext_url: true, cloud_url: true },
        },
        {
            title: 'Authentication',
            keys: ['erpnext_api_key', 'erpnext_api_secret'],
            labels: { erpnext_api_key: 'API Key', erpnext_api_secret: 'API Secret' },
            editable: { erpnext_api_key: true, erpnext_api_secret: true },
            mask: { erpnext_api_key: false, erpnext_api_secret: true },
        },
        {
            title: 'Company Mapping',
            keys: ['company_tally_name', 'erpnext_company', 'company_id'],
            labels: { company_tally_name: 'Tally Company', erpnext_company: 'ERPNext Company', company_id: 'Cloud Company ID' },
            editable: { erpnext_company: true },
        },
        {
            title: 'Sync Config',
            keys: ['sync_from_date', 'sync_to_date', 'poll_interval_minutes'],
            labels: { sync_from_date: 'Sync From', sync_to_date: 'Sync To', poll_interval_minutes: 'Poll Interval (min)' },
            editable: { sync_from_date: true, sync_to_date: true },
        },
        {
            title: 'System',
            keys: ['machine_id', 'last_registered_at'],
            labels: { machine_id: 'Machine ID', last_registered_at: 'Registered At' },
            editable: {},
        },
    ]

    const handleSave = async (key: string) => {
        const val = editing[key] ?? config[key]
        await SetConfigValue(key, val)
        setConfig(c => ({ ...c, [key]: val }))
        setEditing(e => { const n = { ...e }; delete n[key]; return n })
        setSaved(s => ({ ...s, [key]: true }))
        setTimeout(() => setSaved(s => ({ ...s, [key]: false })), 2000)
    }

    if (loading) return <div className="p-6"><Spinner label="Loading settings…" /></div>

    return (
        <div className="p-6 space-y-5 max-w-2xl">
            <h2 className="text-lg font-semibold">Settings</h2>
            {SETTING_GROUPS.map(group => (
                <Card key={group.title}>
                    <CardHeader><CardTitle>{group.title}</CardTitle></CardHeader>
                    <CardContent>
                        <div className="space-y-3">
                            {group.keys.map(key => {
                                const label = group.labels[key] || key
                                const isEditable = !!group.editable[key]
                                const isMasked = !!group.mask?.[key]
                                const editVal = editing[key]
                                const currentVal = config[key] || '—'
                                return (
                                    <div key={key} className="flex items-center justify-between gap-4 text-sm">
                                        <span className="text-neutral-500 shrink-0 w-40">{label}</span>
                                        {isEditable && editVal !== undefined ? (
                                            <div className="flex items-center gap-2 flex-1">
                                                <input
                                                    type={isMasked ? 'password' : 'text'}
                                                    className="flex-1 border border-gray-300 rounded px-2 py-1 text-xs font-mono focus:ring-2 focus:ring-blue-500 outline-none"
                                                    value={editVal}
                                                    onChange={e => setEditing(ev => ({ ...ev, [key]: e.target.value }))}
                                                />
                                                <Button size="sm" onClick={() => handleSave(key)}>Save</Button>
                                                <Button size="sm" variant="ghost" onClick={() => setEditing(e => { const n = { ...e }; delete n[key]; return n })}>✕</Button>
                                            </div>
                                        ) : (
                                            <div className="flex items-center gap-2 flex-1 justify-end">
                                                {saved[key] && <Badge variant="success">Saved</Badge>}
                                                <span className={`font-mono text-xs text-neutral-700 max-w-xs truncate ${isMasked ? 'select-none blur-sm hover:blur-none transition-all' : ''}`}>
                                                    {currentVal}
                                                </span>
                                                {isEditable && (
                                                    <button className="text-xs text-blue-500 hover:text-blue-700 shrink-0"
                                                        onClick={() => setEditing(e => ({ ...e, [key]: config[key] || '' }))}>
                                                        Edit
                                                    </button>
                                                )}
                                            </div>
                                        )}
                                    </div>
                                )
                            })}
                        </div>
                    </CardContent>
                </Card>
            ))}

            <Card>
                <CardHeader><CardTitle>About</CardTitle></CardHeader>
                <CardContent>
                    <p className="text-sm text-neutral-500">TallyBridge Connector — local bridge between Tally ERP and ERPNext via TallyBridge Cloud.</p>
                </CardContent>
            </Card>
        </div>
    )
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

function StatusRow({ online, loading, label }: { online: boolean; loading: boolean; label: string }) {
    return (
        <div className={`p-3 rounded-lg flex items-center gap-3 border text-sm ${online ? 'bg-green-50 border-green-100 text-green-800' : loading ? 'bg-blue-50 border-blue-100 text-blue-700' : 'bg-red-50 border-red-100 text-red-700'}`}>
            {loading
                ? <div className="w-3 h-3 border-2 border-blue-400 border-t-transparent rounded-full animate-spin shrink-0"></div>
                : <div className={`w-2.5 h-2.5 rounded-full shrink-0 ${online ? 'bg-green-500' : 'bg-red-400'}`}></div>
            }
            <span className="font-medium">{label}</span>
        </div>
    )
}

function Spinner({ label }: { label: string }) {
    return (
        <div className="flex flex-col items-center justify-center py-8 gap-3 text-neutral-400">
            <div className="w-7 h-7 rounded-full animate-spin" style={{ border: '3px solid #e5e7eb', borderTopColor: '#3b82f6' }}></div>
            <span className="text-sm">{label}</span>
        </div>
    )
}

function ErrorBox({ msg }: { msg: string }) {
    return <div className="p-3 bg-red-50 border border-red-200 text-red-600 rounded-lg text-xs">{msg}</div>
}

function Field({ label, type = 'text', value, onChange, disabled, hint, mono }: {
    label: string; type?: string; value: string; onChange: (v: string) => void;
    disabled?: boolean; hint?: string; mono?: boolean
}) {
    return (
        <div>
            <label className="block text-sm font-medium text-neutral-700 mb-1">{label}</label>
            <input type={type}
                className={`w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 outline-none ${mono ? 'font-mono' : ''}`}
                value={value} onChange={e => onChange(e.target.value)} disabled={disabled} />
            {hint && <p className="text-xs text-neutral-400 mt-1">{hint}</p>}
        </div>
    )
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
    return (
        <div className="flex justify-between items-start gap-4">
            <span className="text-neutral-400 shrink-0">{label}</span>
            <span className={`text-neutral-700 text-right break-all ${mono ? 'font-mono text-xs' : 'text-sm font-medium'}`}>{value || '—'}</span>
        </div>
    )
}

function StatCard({ label, value, color }: { label: string; value: number; color: 'green' | 'yellow' | 'blue' | 'red' }) {
    const colors = {
        green: 'text-green-600 bg-green-50 border-green-100',
        yellow: 'text-yellow-600 bg-yellow-50 border-yellow-100',
        blue: 'text-blue-600 bg-blue-50 border-blue-100',
        red: 'text-red-600 bg-red-50 border-red-100',
    }
    return (
        <div className={`border rounded-xl p-4 text-center ${colors[color]}`}>
            <div className="text-2xl font-bold">{value}</div>
            <div className="text-xs font-medium mt-1">{label}</div>
        </div>
    )
}
