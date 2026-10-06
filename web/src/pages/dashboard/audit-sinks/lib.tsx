// SPDX-License-Identifier: AGPL-3.0-only
import { TFunction } from 'i18next'
import { ReactNode } from 'react'

import { AuditEvent, AuditFilter, AuditSink } from '@shared/api/endpoints/audit'

export type SinkType = 'file' | 'syslog' | 'webhook'

export interface SinkFields {
    addr: string
    facility: string
    maxBackups: number | string
    maxSizeMb: number | string
    name: string
    network: string
    path: string
    timeoutSec: number | string
    type: SinkType
    url: string
}

export type SinkErrors = Partial<Record<'addr' | 'backups' | 'name' | 'path' | 'size' | 'timeout' | 'url', string>>

const NAME_RE = /^[a-z0-9][a-z0-9-]{1,31}$/

const isPosInt = (v: number | string, min: number, max = Infinity) => {
    if (v === '' || v === null) return false
    const n = Number(v)
    return Number.isInteger(n) && n >= min && n <= max
}

export function validateSink(f: SinkFields, t: TFunction): SinkErrors {
    const er: SinkErrors = {}
    const name = f.name.trim()
    if (!name) er.name = t('drawer.errNameRequired')
    else if (!NAME_RE.test(name)) er.name = t('drawer.errNameChars')

    if (f.type === 'file') {
        const p = f.path.trim()
        if (!p) er.path = t('drawer.errPathRequired')
        else if (!/^(\/|[A-Za-z]:[\\/])/.test(p)) er.path = t('drawer.errPathAbs')
        if (!isPosInt(f.maxSizeMb, 1)) er.size = t('drawer.errSize')
        if (!isPosInt(f.maxBackups, 1)) er.backups = t('drawer.errBackups')
    } else if (f.type === 'webhook') {
        const u = f.url.trim()
        if (!u) er.url = t('drawer.errUrlRequired')
        else if (!/^https?:\/\/[^\s/]+/i.test(u)) er.url = t('drawer.errUrlFormat')
        if (!isPosInt(f.timeoutSec, 1, 60)) er.timeout = t('drawer.errTimeout')
    } else {
        if (f.network && !f.addr.trim()) er.addr = t('drawer.errAddrRequired')
    }
    return er
}

/** Non-blocking hint: the server accepts plain http, a bank usually should not. */
export function urlWarning(url: string, t: TFunction): string {
    return /^http:\/\//i.test(url.trim()) ? t('drawer.warnHttp') : ''
}

/** The request the form will send, shown as a live preview. */
export function buildBody(f: SinkFields): { json: Record<string, number | string>; url: string } {
    const name = f.name.trim() || '…'
    if (f.type === 'file') {
        return {
            url: `/v1/sys/audit/file/${name}`,
            json: {
                path: f.path.trim() || '/var/log/tuck/audit.log',
                max_size_mb: Number(f.maxSizeMb) || 100,
                max_backups: Number(f.maxBackups) || 7
            }
        }
    }
    if (f.type === 'syslog') {
        return {
            url: `/v1/sys/audit/syslog/${name}`,
            json: {
                network: f.network || 'tcp',
                addr: f.addr.trim() || 'rsyslog.example.internal:6514',
                facility: f.facility || 'local0'
            }
        }
    }
    return {
        url: `/v1/sys/audit/webhook/${name}`,
        json: {
            url: f.url.trim() || 'https://siem.example.org/tuck/ingest',
            timeout_sec: Number(f.timeoutSec) || 5
        }
    }
}

/** Where the sink delivers to, as stored in its options bag. */
export function sinkTarget(s: AuditSink): string {
    const o = s.options ?? {}
    if (o.url) return o.url
    if (o.path) return o.path
    if (o.addr) return o.network ? `${o.network}://${o.addr}` : o.addr
    return '—'
}

/** One-line settings summary of a sink (rotation, timeout ...). */
export function sinkMeta(s: AuditSink, t: TFunction): string {
    const o = s.options ?? {}
    if (s.type === 'file') {
        const size = Number(o.max_size_mb) || 100
        const backups = Number(o.max_backups) || 7
        return t('meta.rotation', { size, backups })
    }
    if (s.type === 'webhook') return t('meta.timeout', { sec: Number(o.timeout_sec) || 5 })
    return o.facility ? t('meta.facility', { facility: o.facility }) : '—'
}

/** Bytes as a short, locale-agnostic string (B/KB/MB/GB). */
export function formatBytes(n: number): string {
    if (!n || n <= 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    let v = n
    let i = 0
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024
        i++
    }
    return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`
}

/** "3m ago" / "just now", from an RFC 3339 timestamp. */
export function relTime(iso: string | undefined, t: TFunction): string {
    if (!iso) return '—'
    const ms = Date.now() - new Date(iso).getTime()
    const s = Math.max(0, Math.round(ms / 1000))
    if (s < 2) return t('time.now')
    if (s < 60) return t('time.secAgo', { n: s })
    if (s < 3600) return t('time.minAgo', { n: Math.floor(s / 60) })
    if (s < 86400) return t('time.hourAgo', { n: Math.floor(s / 3600) })
    return t('time.dayAgo', { n: Math.floor(s / 86400) })
}

export type Severity = 'err' | 'ok' | 'warn'

export function severityOf(status: number): Severity {
    if (status >= 500) return 'err'
    if (status >= 400) return 'warn'
    return 'ok'
}

export function operationOf(e: AuditEvent): string {
    if (e.operation) return e.operation
    const m = e.method.toUpperCase()
    if (m === 'GET' || m === 'HEAD') return e.path.includes('login') ? 'login' : 'read'
    if (m === 'PUT' || m === 'POST' || m === 'PATCH') return 'write'
    if (m === 'DELETE') return 'delete'
    return 'other'
}

export const OPERATIONS = ['all', 'read', 'write', 'delete', 'list', 'login', 'other'] as const
export const STATUS_FILTERS = ['all', '2', '403', '404', '5', 'error'] as const

export type TimeRangeKey = '15m' | '1h' | '24h' | 'all'

export const TIME_RANGES: TimeRangeKey[] = ['15m', '1h', '24h', 'all']

/** Converts a quick time-range pick into the `since` value the API accepts. */
export function sinceFromRange(range: TimeRangeKey): string | undefined {
    switch (range) {
        case '15m':
            return '15m'
        case '1h':
            return '1h'
        case '24h':
            return '24h'
        default:
            return undefined
    }
}

export interface LogFilterState {
    namespace: string
    operation: string
    path: string
    q: string
    range: TimeRangeKey
    status: string
}

export function filterStateToApi(f: LogFilterState): AuditFilter {
    return {
        path: f.path.trim() || undefined,
        operation: f.operation,
        status: f.status,
        namespace: f.namespace,
        q: f.q.trim() || undefined,
        since: sinceFromRange(f.range)
    }
}

/** NDJSON (one JSON object per line) download of the events currently loaded in the UI. */
export function downloadEventsNdjson(events: AuditEvent[], filename = 'audit-events.ndjson') {
    const body = events.map((e) => JSON.stringify(e)).join('\n') + (events.length ? '\n' : '')
    const blob = new Blob([body], { type: 'application/x-ndjson' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
}

type JsonStyles = Record<string, string>

/** Minimal JSON syntax colouring (keys / strings / numbers / punctuation). */
export function highlightJson(src: string, styles: JsonStyles): ReactNode[] {
    const out: ReactNode[] = []
    const re = /("(?:[^"\\]|\\.)*")(\s*:)?|(-?\b\d+\b)|([{}[\],])/g
    let last = 0
    let m: null | RegExpExecArray
    let k = 0
    while ((m = re.exec(src))) {
        if (m.index > last) out.push(src.slice(last, m.index))
        const [, str, colon, num, punct] = m
        if (str) {
            if (colon) {
                out.push(
                    <span className={styles.jk} key={k++}>
                        {str}
                    </span>,
                    <span className={styles.jp} key={k++}>
                        {colon}
                    </span>
                )
            } else {
                out.push(
                    <span className={styles.js} key={k++}>
                        {str}
                    </span>
                )
            }
        } else if (num) {
            out.push(
                <span className={styles.jn} key={k++}>
                    {num}
                </span>
            )
        } else {
            out.push(
                <span className={styles.jp} key={k++}>
                    {punct}
                </span>
            )
        }
        last = re.lastIndex
    }
    if (last < src.length) out.push(src.slice(last))
    return out
}
