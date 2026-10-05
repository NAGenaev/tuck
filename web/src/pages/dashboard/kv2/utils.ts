// SPDX-License-Identifier: AGPL-3.0-only
import type { KVMeta } from '@shared/api/endpoints/kvv2'

export type VerState = 'active' | 'deleted' | 'destroyed'

export interface VerRow {
    createdAt?: string
    state: VerState
    version: number
}

export function versionRows(meta?: KVMeta): VerRow[] {
    if (!meta?.versions) return []
    return Object.entries(meta.versions)
        .map(([v, info]) => ({
            createdAt: info.created_at,
            state: (info.destroyed ? 'destroyed' : info.deleted_at ? 'deleted' : 'active') as VerState,
            version: Number(v)
        }))
        .sort((a, b) => a.version - b.version)
}

export type ParsedValue = { kind: 'pairs'; pairs: Array<[string, string]> } | { kind: 'text'; text: string }

/**
 * A secret is one opaque string on the server. When it is a flat JSON object of strings we show
 * it as key / value pairs (a view only; it is stored and written back as the same JSON).
 */
export function parseValue(raw: string): ParsedValue {
    const s = raw.trim()
    if (s.startsWith('{')) {
        try {
            const obj: unknown = JSON.parse(s)
            if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
                const entries = Object.entries(obj as Record<string, unknown>)
                if (entries.length > 0 && entries.every(([, v]) => typeof v === 'string')) {
                    return { kind: 'pairs', pairs: entries as Array<[string, string]> }
                }
            }
        } catch {
            // plain text
        }
    }
    return { kind: 'text', text: raw }
}

export function pairsToJson(pairs: Array<{ key: string; value: string }>): string {
    const obj: Record<string, string> = {}
    for (const p of pairs) obj[p.key.trim()] = p.value
    return JSON.stringify(obj)
}

export const maskText = (len: number) => '•'.repeat(Math.min(Math.max(len, 8), 22))

/** returns a reason or null when the name is acceptable */
export function validateName(name: string): null | 'empty' | 'format' {
    const n = name.trim()
    if (!n) return 'empty'
    if (/\s/.test(n) || n.startsWith('/') || n.endsWith('/') || n.includes('//') || /(^|\/)\.\.?(\/|$)/.test(n)) {
        return 'format'
    }
    return null
}

export const KEY_PATTERN = /^[A-Za-z0-9_.-]+$/

// ---------- line diff (LCS) ----------

export interface DiffLine {
    key?: string
    sig: string
    txt: string
}

export interface DiffRow {
    a?: number
    b?: number
    line: DiffLine
    t: 'add' | 'del' | 'eq'
}

export function valueLines(raw: string, show: boolean): DiffLine[] {
    const parsed = parseValue(raw)
    if (parsed.kind === 'pairs') {
        const sorted = [...parsed.pairs].sort(([a], [b]) => a.localeCompare(b))
        return sorted.map(([k, v], i) => ({
            key: k,
            sig: `${k}=${v}`,
            txt: `  ${JSON.stringify(k)}: "${show ? v : maskText(v.length)}"${i < sorted.length - 1 ? ',' : ''}`
        }))
    }
    return raw.split('\n').map((l) => ({ sig: l, txt: show ? l : maskText(l.length) }))
}

export function lcsDiff(A: DiffLine[], B: DiffLine[]): DiffRow[] {
    const n = A.length
    const m = B.length
    const L = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
    for (let i = n - 1; i >= 0; i--) {
        for (let j = m - 1; j >= 0; j--) {
            L[i][j] = A[i].sig === B[j].sig ? L[i + 1][j + 1] + 1 : Math.max(L[i + 1][j], L[i][j + 1])
        }
    }
    const out: DiffRow[] = []
    let i = 0
    let j = 0
    while (i < n && j < m) {
        if (A[i].sig === B[j].sig) {
            out.push({ a: i + 1, b: j + 1, line: B[j], t: 'eq' })
            i++
            j++
        } else if (A[i].key !== undefined && A[i].key === B[j].key) {
            out.push({ a: i + 1, line: A[i], t: 'del' })
            out.push({ b: j + 1, line: B[j], t: 'add' })
            i++
            j++
        } else if (L[i + 1][j] >= L[i][j + 1]) {
            out.push({ a: i + 1, line: A[i], t: 'del' })
            i++
        } else {
            out.push({ b: j + 1, line: B[j], t: 'add' })
            j++
        }
    }
    while (i < n) {
        out.push({ a: i + 1, line: A[i], t: 'del' })
        i++
    }
    while (j < m) {
        out.push({ b: j + 1, line: B[j], t: 'add' })
        j++
    }
    return out
}

// ---------- time ----------

export function fmtDate(iso: string | undefined, lng: string): string {
    if (!iso) return '—'
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return '—'
    return d.toLocaleString(lng === 'ru' ? 'ru-RU' : 'en-GB', {
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        month: '2-digit',
        year: 'numeric'
    })
}

export function fmtRel(iso: string | undefined, lng: string): string {
    if (!iso) return '—'
    const ms = new Date(iso).getTime() - Date.now()
    if (Number.isNaN(ms)) return '—'
    const rtf = new Intl.RelativeTimeFormat(lng === 'ru' ? 'ru' : 'en', { numeric: 'auto' })
    const min = Math.round(ms / 60000)
    if (Math.abs(min) < 1) return rtf.format(0, 'minute')
    if (Math.abs(min) < 60) return rtf.format(min, 'minute')
    const h = Math.round(min / 60)
    if (Math.abs(h) < 24) return rtf.format(h, 'hour')
    const d = Math.round(h / 24)
    if (Math.abs(d) < 30) return rtf.format(d, 'day')
    return rtf.format(Math.round(d / 30), 'month')
}

export const dirName = (p: string) => p.slice(0, p.lastIndexOf('/') + 1)
