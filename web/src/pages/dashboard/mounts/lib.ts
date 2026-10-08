// SPDX-License-Identifier: AGPL-3.0-only
import { isAxiosError } from 'axios'

import type { MountEntry } from '@shared/api/endpoints/mounts'

export type Cat = 'auth' | 'engine' | 'sys'
export type Filter = 'all' | 'auth' | 'secrets' | 'sys'

export const CAT_COLOR: Record<Cat, string> = {
    auth: 'var(--ice)',
    engine: 'var(--brass)',
    sys: 'var(--violet)'
}

export const CAT_PILL: Record<Cat, 'b' | 'c' | 'g' | 'v'> = {
    auth: 'c',
    engine: 'b',
    sys: 'v'
}

const AUTH = new Set(['approle', 'auth', 'cert', 'jwt', 'kubernetes', 'ldap', 'oidc', 'token', 'userpass'])
const SYS = new Set(['identity', 'system'])

export function catOf(m: Pick<MountEntry, 'path' | 'type'>): Cat {
    if (SYS.has(m.type)) return 'sys'
    if (AUTH.has(m.type) || m.path.startsWith('auth/')) return 'auth'
    return 'engine'
}

export function matchesFilter(m: MountEntry, f: Filter): boolean {
    if (f === 'all') return true
    const c = catOf(m)
    if (f === 'secrets') return c === 'engine'
    return c === f
}

/** Mount types offered in the wizard (suggestion only: the server accepts any type string). */
export interface CatalogType {
    auth?: boolean
    icon: string
    id: string
    suggest: string
}

export const CATALOG: CatalogType[] = [
    { icon: 'kv', id: 'kv', suggest: 'my-kv' },
    { auth: true, icon: 'k8s', id: 'kubernetes', suggest: 'auth/kubernetes-spb' },
    { auth: true, icon: 'key', id: 'approle', suggest: 'auth/approle-ci' },
    { auth: true, icon: 'jwt', id: 'jwt', suggest: 'auth/jwt-sso' }
]

export const PATH_RE = /^[a-z0-9][a-z0-9_-]*(\/[a-z0-9][a-z0-9_-]*)*$/
export const TYPE_RE = /^[a-z0-9][a-z0-9_-]*$/

export const normPath = (p: string) => p.trim().replace(/^\/+|\/+$/g, '')
export const withSlash = (p: string) => `${normPath(p)}/`

export function serverMessage(err: unknown): string | undefined {
    if (isAxiosError(err)) {
        const d = err.response?.data as { error?: string } | undefined
        return d?.error
    }
    return undefined
}

export const isForbidden = (err: unknown) => isAxiosError(err) && err.response?.status === 403

/** nanoseconds (Go time.Duration) to whole seconds */
export const nsToSec = (ns?: number) => (ns ? Math.round(ns / 1e9) : 0)

/** 3600 -> 1h, 90 -> 90s, 172800 -> 2d, 0 -> — */
export function fmtTtl(s: number): string {
    if (!s) return '—'
    if (s % 86400 === 0 && s >= 86400 * 2) return `${s / 86400}d`
    if (s % 3600 === 0) return `${s / 3600}h`
    if (s % 60 === 0) return `${s / 60}m`
    return `${s}s`
}

export const reducedMotion = () =>
    typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
