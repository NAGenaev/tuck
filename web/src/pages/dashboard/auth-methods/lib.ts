// SPDX-License-Identifier: AGPL-3.0-only

/** Go time.Duration marshalled as JSON is an integer of nanoseconds. */
export const nsToSeconds = (ns?: number): number => (ns && ns > 0 ? Math.round(ns / 1e9) : 0)

/** "24h0m0s", "1h", "90m", "1h30m" -> seconds; null when it isn't a duration. */
export function parseGoDuration(s?: string): null | number {
    if (!s) return 0
    const re = /(\d+(?:\.\d+)?)(ms|h|m|s)/g
    let total = 0
    let used = 0
    let m: null | RegExpExecArray
    while ((m = re.exec(s))) {
        used += m[0].length
        const n = Number(m[1])
        total += m[2] === 'h' ? n * 3600 : m[2] === 'm' ? n * 60 : m[2] === 's' ? n : n / 1000
    }
    return used === s.length ? Math.round(total) : null
}

/** seconds -> compact "1d 2h" / "30m" / "45s"; 0 -> '' */
export function fmtTtl(sec: number): string {
    if (!sec || sec <= 0) return ''
    const d = Math.floor(sec / 86400)
    const h = Math.floor((sec % 86400) / 3600)
    const m = Math.floor((sec % 3600) / 60)
    const s = sec % 60
    const parts: string[] = []
    if (d) parts.push(`${d}d`)
    if (h) parts.push(`${h}h`)
    if (m) parts.push(`${m}m`)
    if (s && parts.length < 2) parts.push(`${s}s`)
    return parts.slice(0, 2).join(' ')
}

/** seconds -> value for the Go-duration wire fields; 0 -> undefined (server default) */
export const toDur = (sec: number): string | undefined => (sec > 0 ? `${Math.round(sec)}s` : undefined)

export const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/
export const K8S_PART_RE = /^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$/
export const TAG_RE = /^[A-Za-z0-9_.*:/@=-]+$/
export const NS_RE = /^[a-z0-9][a-z0-9-]*$/

export const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((v) => b.includes(v))
