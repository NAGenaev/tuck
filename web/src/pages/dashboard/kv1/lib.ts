// SPDX-License-Identifier: AGPL-3.0-only
import { isAxiosError } from 'axios'

export type NameIssue = 'chars' | 'empty' | 'slashes' | 'space'

export function errStatus(e: unknown): number | undefined {
    return isAxiosError(e) ? e.response?.status : undefined
}

/** "403 Forbidden" style line for toasts / state details; falls back to the error message. */
export function errLine(e: unknown): string {
    if (isAxiosError(e)) {
        if (!e.response) return e.message
        const s = e.response.status
        const msg = (e.response.data as { error?: string; errors?: string[] } | undefined)?.error
        return msg ? `${s} · ${msg}` : `${s} ${e.response.statusText ?? ''}`.trim()
    }
    return e instanceof Error ? e.message : String(e)
}

export function dirOf(path: string): string {
    const i = path.lastIndexOf('/')
    return i < 0 ? '' : path.slice(0, i + 1)
}

/** Parses a JSON object (not array / scalar); null otherwise. */
export function parseObject(v: string): null | Record<string, unknown> {
    try {
        const o: unknown = JSON.parse(v)
        return o && typeof o === 'object' && !Array.isArray(o) ? (o as Record<string, unknown>) : null
    } catch {
        return null
    }
}

export function stringify(v: unknown): string {
    return typeof v === 'string' ? v : JSON.stringify(v)
}

export const bullets = (n: number) => '•'.repeat(n)

export function maskValue(v: string): string {
    return bullets(Math.max(6, Math.min(v.length, 18)))
}

/** Same-shape masked preview of a value (keys stay, values become bullets); never exposes the secret. */
export function stageMask(text: string): string {
    try {
        const parsed: unknown = JSON.parse(text)
        if (parsed && typeof parsed === 'object') {
            return JSON.stringify(
                parsed,
                (_k, v) =>
                    v && typeof v === 'object' ? v : bullets(Math.max(4, Math.min(String(v).length, 14))),
                2
            )
        }
    } catch {
        // not JSON: fall through
    }
    return text.replace(/\S/g, '•')
}

/** Lenient path check: the server decides what it accepts, we only block what breaks the URL. */
export function validateName(n: string): NameIssue | null {
    if (!n) return 'empty'
    if (/\s/.test(n)) return 'space'
    if (/[?#\%]/.test(n)) return 'chars'
    if (/^\/|\/\/|\/$/.test(n)) return 'slashes'
    return null
}

export function byteLength(v: string): number {
    return new TextEncoder().encode(v).length
}

export const sleep = (ms: number) => new Promise<void>((r) => window.setTimeout(r, ms))
