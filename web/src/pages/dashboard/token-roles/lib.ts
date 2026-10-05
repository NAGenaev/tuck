// SPDX-License-Identifier: AGPL-3.0-only
import { useTranslation } from 'react-i18next'

import type { TokenRole } from '@shared/api/endpoints/token-roles'

/** Role as the UI sees it: durations in seconds (the API echoes Go durations as nanoseconds). */
export interface RoleView {
    maxTtl: number
    maxUses: number
    name: string
    period: number
    policies: string[]
    renewable: boolean
    ttl: number
    updatedAt?: string
}

export interface RoleRow {
    failed: boolean
    loading: boolean
    name: string
    role?: RoleView
}

export const NAME_RE = /^[a-z0-9][a-z0-9_-]{0,62}$/
export const POLICY_RE = /^[^\s,]+$/

const nsToSec = (ns: number | undefined) => (ns ? Math.round(ns / 1e9) : 0)

export function toView(name: string, r: TokenRole): RoleView {
    const updated = r.updated_at && new Date(r.updated_at).getFullYear() > 2000 ? r.updated_at : undefined
    return {
        maxTtl: nsToSec(r.max_ttl),
        maxUses: r.max_uses ?? 0,
        name,
        period: nsToSec(r.period),
        policies: r.policies ?? [],
        renewable: !!r.renewable,
        ttl: nsToSec(r.ttl),
        updatedAt: updated
    }
}

/** "30s" / "15m" / "1h" / "7d" -> seconds; '' -> null; anything else -> NaN. */
export function parseDur(v: string): null | number {
    const s = v.trim().toLowerCase()
    if (!s) return null
    const m = s.match(/^(\d+)\s*(s|m|h|d)?$/)
    if (!m) return NaN
    const unit = { d: 86400, h: 3600, m: 60, s: 1 }[(m[2] ?? 's') as 'd' | 'h' | 'm' | 's']
    return Number(m[1]) * unit
}

/** Compact input form: 3600 -> "1h", 90 -> "90s". */
export function fmtShort(s: number): string {
    if (!s) return ''
    if (s % 86400 === 0) return `${s / 86400}d`
    if (s % 3600 === 0) return `${s / 3600}h`
    if (s % 60 === 0) return `${s / 60}m`
    return `${s}s`
}

/** Go time.ParseDuration string (no days). */
export function toGoDuration(s: number): string | undefined {
    if (!s) return undefined
    if (s % 3600 === 0) return `${s / 3600}h`
    if (s % 60 === 0) return `${s / 60}m`
    return `${s}s`
}

export type Tr = (key: string, def: string, opts?: Record<string, unknown>) => string

export function fmtDur(tr: Tr, s: null | number | undefined): string {
    if (s == null || Number.isNaN(s)) return '—'
    if (s === 0) return '0'
    if (s % 86400 === 0) return `${s / 86400} ${tr('u.d', 'д')}`
    if (s % 3600 === 0) return `${s / 3600} ${tr('u.h', 'ч')}`
    if (s % 60 === 0) return `${s / 60} ${tr('u.m', 'мин')}`
    return `${s} ${tr('u.s', 'с')}`
}

/** Translation helper: every string of this screen lives under `tokens:roles.*`. */
export function useTr() {
    const { t, i18n } = useTranslation(['tokens', 'translation'])
    const tr: Tr = (key, def, opts) => t(`roles.${key}`, { defaultValue: def, ...opts }) as string
    return { lang: i18n.language, t, tr }
}

export function fmtDate(iso: string | undefined, lang: string): string {
    if (!iso) return ''
    try {
        return new Intl.DateTimeFormat(lang, { day: '2-digit', hour: '2-digit', minute: '2-digit', month: 'short', year: 'numeric' }).format(
            new Date(iso)
        )
    } catch {
        return iso
    }
}
