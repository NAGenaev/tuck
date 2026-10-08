import { instance } from '../axios'

// NOTE: unified list across sink types, wrapped as {"sinks":[...]} (not
// {"keys":[...]} like every other list endpoint), and typed options are
// stringified inside a map[string]string "options" bag — there is no per-sink GET.
export interface SinkStats {
    sent: number
    bytes: number
    errors: number
    retries: number
    dropped: number
    skipped: number
    queue: number
    consecutive_failures: number
    paused: boolean
    last_error?: string
    last_error_at?: string
    last_success_at?: string
    next_retry_at?: string
}

export interface AuditSink {
    name: string
    type: 'file' | 'syslog' | 'webhook'
    options: Record<string, string>
    errors: number
    enabled: boolean
    /** false when configured but not currently running (e.g. its file could not be opened) */
    active: boolean
    health: 'failing' | 'inactive' | 'ok' | 'paused'
    stats?: SinkStats
}

export interface AuditCapabilities {
    /** false on platforms without syslog support (e.g. Windows) */
    syslog: boolean
    pause: boolean
}

export interface AuditSinkList {
    capabilities: AuditCapabilities
    sinks: AuditSink[]
}

export async function listAuditSinks(): Promise<AuditSinkList> {
    const { data } = await instance.get<{ capabilities?: AuditCapabilities; sinks: AuditSink[] }>(
        '/v1/sys/audit/',
        { params: { list: 'true' } }
    )
    return { sinks: data.sinks ?? [], capabilities: data.capabilities ?? { syslog: false, pause: true } }
}

export async function putWebhookSink(name: string, url: string, timeoutSec?: number): Promise<void> {
    await instance.put(`/v1/sys/audit/webhook/${name}`, { url, timeout_sec: timeoutSec })
}

export async function putFileSink(
    name: string,
    path: string,
    maxSizeMb?: number,
    maxBackups?: number
): Promise<void> {
    await instance.put(`/v1/sys/audit/file/${name}`, {
        path,
        max_size_mb: maxSizeMb,
        max_backups: maxBackups
    })
}

export async function putSyslogSink(name: string, network: string, addr: string, facility: string): Promise<void> {
    await instance.put(`/v1/sys/audit/syslog/${name}`, { network, addr, facility })
}

export async function putSinkState(name: string, enabled: boolean): Promise<void> {
    await instance.put(`/v1/sys/audit/state/${name}`, { enabled })
}

export async function deleteAuditSink(name: string): Promise<void> {
    await instance.delete(`/v1/sys/audit/${name}`)
}

// ─── readable log ───────────────────────────────────────────────────────────

/** One record of the audit log. Never carries a secret value or a token ID. */
export interface AuditEvent {
    time: string
    seq?: number
    request_id?: string
    method: string
    path: string
    operation?: string
    namespace?: string
    status: number
    client_ip?: string
    fingerprint?: string
    /** who the request is attributed to, e.g. "ldap:alice"; a claimed login for a failed sign-in */
    identity?: string
    entity_id?: string
    duration_ms: number
    prev_hash: string
    hash: string
}

export interface AuditWindow {
    size: number
    capacity: number
    oldest_seq: number
    newest_seq: number
}

export interface AuditEventsResponse {
    events: AuditEvent[]
    matched: number
    truncated: boolean
    window: AuditWindow
    head_hash: string
    head_seq: number
}

export interface AuditFilter {
    path?: string
    operation?: string
    status?: string
    namespace?: string
    q?: string
    since?: string
    until?: string
}

export interface AuditEventsQuery extends AuditFilter {
    afterSeq?: number
    limit?: number
}

function filterParams(f: AuditFilter): Record<string, string> {
    const p: Record<string, string> = {}
    if (f.path) p.path = f.path
    if (f.operation && f.operation !== 'all') p.operation = f.operation
    if (f.status && f.status !== 'all') p.status = f.status
    if (f.namespace && f.namespace !== 'all') p.namespace = f.namespace
    if (f.q) p.q = f.q
    if (f.since) p.since = f.since
    if (f.until) p.until = f.until
    return p
}

export async function getAuditEvents(q: AuditEventsQuery = {}): Promise<AuditEventsResponse> {
    const params: Record<string, number | string> = { ...filterParams(q) }
    if (q.limit) params.limit = q.limit
    if (q.afterSeq) params.after_seq = q.afterSeq
    const { data } = await instance.get<AuditEventsResponse>('/v1/sys/audit/events', { params })
    return data
}

export interface MinuteCount {
    time: string
    count: number
    errors: number
}

export interface AuditStatsSnapshot {
    since: string
    total: number
    today: number
    today_errors: number
    last_minute: number
    per_minute: MinuteCount[]
    by_operation: Record<string, number>
    window_minutes: number
}

export interface AuditStatsResponse {
    stats: AuditStatsSnapshot
    window: AuditWindow
    head_hash: string
    head_seq: number
}

export async function getAuditStats(minutes = 60): Promise<AuditStatsResponse> {
    const { data } = await instance.get<AuditStatsResponse>('/v1/sys/audit/stats', { params: { minutes } })
    return data
}

export interface AuditVerifyResult {
    ok: boolean
    checked: number
    first_broken: number
    broken_seq?: number
    reason?: string
    legacy?: number
    from_genesis: boolean
    head_hash?: string
}

export interface AuditVerifyResponse {
    ok: boolean
    verified_at: string
    memory?: AuditVerifyResult
    file?: AuditVerifyResult
    file_source?: string
    file_error?: string
    head_hash: string
    head_seq: number
}

export async function postAuditVerify(source: 'all' | 'file' | 'memory' = 'all', sink?: string): Promise<AuditVerifyResponse> {
    const { data } = await instance.post<AuditVerifyResponse>('/v1/sys/audit/verify', { source, sink })
    return data
}

/** Builds the query string for the SSE stream endpoint (same filters as getAuditEvents, plus a replay cursor). */
export function auditStreamUrl(filter: AuditFilter, sinceSeq?: number): string {
    const params = new URLSearchParams(filterParams(filter))
    if (sinceSeq !== undefined) params.set('since_seq', String(sinceSeq))
    const qs = params.toString()
    return `/v1/sys/audit/events/stream${qs ? `?${qs}` : ''}`
}
