// SPDX-License-Identifier: AGPL-3.0-only
import type { PillTone } from '@shared/ui/tuck/tuck'

import type { TokenInfo } from './tokens.api'

export interface TokenRow {
    id: string
    accessor?: string
    name: string
    policies: string[]
    /** epoch ms */
    createdAt?: number
    /** epoch ms; null = never expires */
    expiresAt: null | number
    /** lifetime in seconds (created -> expires), 0 when unknown / infinite */
    total: number
    ns: string
    uses: number
    maxUses: number
    renewable: boolean
    /** false while the lookup is pending or failed: only the id is known */
    resolved: boolean
    loading: boolean
}

const ts = (s?: string): null | number => {
    if (!s) return null
    const d = new Date(s)
    // Go's zero time ("0001-01-01...") means "never"
    return Number.isNaN(d.getTime()) || d.getFullYear() <= 1 ? null : d.getTime()
}

export function toRow(id: string, info: TokenInfo | undefined, loading: boolean): TokenRow {
    if (!info) {
        return {
            id,
            name: '',
            policies: [],
            expiresAt: null,
            total: 0,
            ns: '',
            uses: 0,
            maxUses: 0,
            renewable: false,
            resolved: false,
            loading
        }
    }
    const createdAt = ts(info.created_at) ?? undefined
    const expiresAt = ts(info.expires_at)
    return {
        id,
        accessor: info.accessor,
        name: info.display_name ?? '',
        policies: info.policies ?? [],
        createdAt,
        expiresAt,
        total: createdAt && expiresAt ? Math.max(1, Math.round((expiresAt - createdAt) / 1000)) : 0,
        ns: info.namespace || 'root',
        uses: info.use_count ?? 0,
        maxUses: info.max_uses ?? 0,
        renewable: !!info.renewable,
        resolved: true,
        loading: false
    }
}

export const leftSeconds = (row: { expiresAt: null | number }, now: number) =>
    row.expiresAt === null ? Infinity : Math.max(0, (row.expiresAt - now) / 1000)

const pad = (n: number) => (n < 10 ? `0${n}` : String(n))

export function fmtLeft(s: number, d: string, h: string, m: string): string {
    if (!Number.isFinite(s)) return '∞'
    s = Math.max(0, Math.floor(s))
    const days = Math.floor(s / 86400)
    const hours = Math.floor((s % 86400) / 3600)
    const mins = Math.floor((s % 3600) / 60)
    if (days > 0) return `${days}${d} ${pad(hours)}${h}`
    if (hours > 0) return `${hours}${h} ${pad(mins)}${m}`
    return `${pad(mins)}:${pad(s % 60)}`
}

export function fmtTotal(s: number, d: string, h: string, m: string): string {
    if (!s) return '∞'
    return s >= 86400 ? `${Math.round(s / 86400)}${d}` : s >= 3600 ? `${Math.round(s / 3600)}${h}` : `${Math.round(s / 60)}${m}`
}

/** "partial" bar colour: brass while plenty is left, drifting to alarm below 35% */
export function barColor(p: number): string {
    if (p >= 0.35) return 'rgb(215,169,75)'
    const k = Math.min(1, (0.35 - p) / 0.27)
    return `rgb(${Math.round(215 + 40 * k)},${Math.round(169 - 79 * k)},${Math.round(75 + 2 * k)})`
}

export function policyTone(p: string): PillTone {
    if (p === 'admin' || p === 'root') return 'bad'
    if (/-read$/.test(p) || /^read/.test(p)) return 'ok'
    if (/-(write|rw)$/.test(p)) return 'brass'
    if (/deploy|ci-/.test(p)) return 'violet'
    if (/audit|backup/.test(p)) return 'ice'
    return 'mute'
}

export const shortAcc = (a: string) => (a.length > 14 ? `${a.slice(0, 10)}…${a.slice(-4)}` : a)
/** the token value is a secret: never show more than its last 4 characters */
export const maskId = (id: string) => `••••••••${id.slice(-4)}`

type T = (key: string, opts?: Record<string, unknown>) => string

export const fmtDateTime = (ms: number, lang: string) =>
    new Date(ms).toLocaleString(lang, { day: '2-digit', hour: '2-digit', minute: '2-digit', month: 'short' })

/** "today, 14:05" / "yesterday, 09:30" / "12 Oct, 14:05" */
export function fmtCreated(ms: number, now: number, lang: string, t: T): string {
    const d = new Date(ms)
    const n = new Date(now)
    const clock = d.toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' })
    const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
    const diff = Math.round((day(n) - day(d)) / 86400000)
    if (diff === 0) return `${t('time.today')}, ${clock}`
    if (diff === 1) return `${t('time.yesterday')}, ${clock}`
    return fmtDateTime(ms, lang)
}

export function agoText(ms: number, now: number, t: T): string {
    const s = Math.max(0, Math.floor((now - ms) / 1000))
    if (s < 60) return t('time.justNow')
    if (s < 3600) return t('time.minAgo', { n: Math.floor(s / 60) })
    if (s < 86400) return t('time.hAgo', { n: Math.floor(s / 3600) })
    return t('time.dAgo', { n: Math.floor(s / 86400) })
}
