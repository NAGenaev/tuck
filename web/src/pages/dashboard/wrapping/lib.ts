// SPDX-License-Identifier: AGPL-3.0-only
import { isAxiosError } from 'axios'

import type { WrappedCapsule } from '@shared/api/endpoints/wrapping'

/** Circumference of the stage ring (r = 46). */
export const RING = 289.03

export type Status = 'active' | 'expired' | 'missing' | 'revoked' | 'used'
export type Op = 'lookup' | 'revoke' | 'unwrap' | 'wrap'
export type Tab = 'look' | 'unwrap' | 'wrap'

/** A capsule this browser session knows about: wrapped here, looked up, unwrapped or revoked. */
export interface Capsule {
    created?: number
    delivered?: number
    expires?: number
    revokedAt?: number
    status: Status
    token: string
    /** creation TTL, seconds */
    ttl?: number
    usedAt?: number
}

export interface Problem {
    message?: string
    status?: number
}

export const OP_PATH: Record<Op, string> = {
    lookup: 'sys/wrapping/lookup',
    revoke: 'sys/wrapping/revoke',
    unwrap: 'sys/wrapping/unwrap',
    wrap: 'sys/wrapping/wrap'
}

export const STATUS_TONE: Record<Status, 'b' | 'g' | 'n' | 'r'> = {
    active: 'g',
    expired: 'r',
    missing: 'n',
    revoked: 'r',
    used: 'b'
}

/** Effective status: an "active" capsule past its expiry is expired (clock-based, like the server). */
export function statusOf(c: Capsule | undefined, now: number): Status | undefined {
    if (!c) return undefined
    if (c.status === 'active' && c.expires !== undefined && now > c.expires) return 'expired'
    return c.status
}

/** The stage view of a capsule the server lists (no token: it is only known by its accessor). */
export function fromServer(c: WrappedCapsule): Capsule {
    return {
        created: Date.parse(c.creation_time),
        expires: Date.parse(c.expires_at),
        status: 'active',
        token: '',
        ttl: c.creation_ttl
    }
}

export const short = (t: string) => (t.length > 20 ? `${t.slice(0, 14)}…${t.slice(-4)}` : t)

const pad = (n: number) => String(n).padStart(2, '0')

export function fmtTime(t: number) {
    const d = new Date(t)
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export const fmtDate = (t: number, locale: string) => new Date(t).toLocaleString(locale)

/** m:ss or h:mm:ss for the stage clock. */
export function clock(ms: number) {
    const s = Math.max(0, Math.ceil(ms / 1000))
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    const r = s % 60
    return `${h ? `${h}:` : ''}${pad(m)}:${pad(r)}`
}

export function splitLeft(ms: number) {
    const s = Math.ceil(Math.max(0, ms) / 1000)
    return { h: Math.floor(s / 3600), m: Math.floor((s % 3600) / 60), s: s % 60 }
}

const UNIT_SEC: Record<string, number> = { h: 3600, m: 60, ms: 0.001, ns: 1e-9, s: 1, us: 1e-6, µs: 1e-6 }

/** Parses a Go duration ("30s", "5m", "1h30m") into seconds, as the server does. null if invalid. */
export function parseTtl(value: string): null | number {
    const v = value.trim()
    if (!v || !/^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/.test(v)) return null
    let total = 0
    for (const m of v.matchAll(/(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g)) total += parseFloat(m[1]) * UNIT_SEC[m[2]]
    return total >= 1 ? Math.round(total) : null
}

export function problemOf(err: unknown): Problem {
    if (isAxiosError(err)) {
        const d = err.response?.data as { error?: string } | undefined
        return { message: d?.error ?? (err.response ? undefined : err.message), status: err.response?.status }
    }
    return { message: err instanceof Error ? err.message : undefined }
}

/** Starter payloads. Values are obvious placeholders, replace them before wrapping. */
export const TEMPLATES = {
    api: { api_key: '<api-key>', scope: '<scope>', service: '<service-name>' },
    db: { db_host: '<host>', db_password: '<password>', db_port: 5432, db_user: '<user>' },
    otp: { force_change: true, one_time_password: '<one-time-password>', user: '<user>' }
} as const

export type TemplateKey = keyof typeof TEMPLATES

/** 512 B, 1.5 KB ... for a payload size. */
export function fmtBytes(n: number, unit: string) {
    if (n < 1024) return `${n} ${unit}`
    return `${(n / 1024).toFixed(n < 10240 ? 1 : 0)} K${unit}`
}

export const byteLength = (s: string) => new TextEncoder().encode(s).length

export function randomParticles(count = 44) {
    return Array.from({ length: count }, (_, i) => {
        const a = Math.random() * Math.PI * 2
        const d = 70 + Math.random() * 130
        return {
            delay: Math.floor(Math.random() * 200),
            id: i,
            left: Math.floor(Math.random() * 160 - 80),
            mint: Math.random() < 0.3,
            r: Math.floor(Math.random() * 360),
            top: 110 + Math.floor(Math.random() * 90),
            x: Math.floor(Math.cos(a) * d),
            y: Math.floor(Math.sin(a) * d - 40)
        }
    })
}

export type Particle = ReturnType<typeof randomParticles>[number]

/** "5 min 12 s" style remaining time; `t` is the wrapping namespace translator. */
export function fmtLeft(t: (key: string, o?: Record<string, unknown>) => string, ms: number) {
    const { h, m, s } = splitLeft(ms)
    if (h) return t('left.h', { h, m })
    if (m) return t('left.m', { m, s })
    return t('left.s', { s })
}
