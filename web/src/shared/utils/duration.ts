// Most TTL fields accept a Go duration string ("1h") on write but echo back
// as raw nanoseconds (int) on read — GitHub's role.ttl is the one exception
// that requires nanoseconds on both read AND write (no wire-struct conversion
// server-side). These helpers bridge nanoseconds <-> a human string for display.
export function nsToHuman(ns: number | undefined): string {
    if (!ns) return '—'
    const seconds = ns / 1e9
    if (seconds >= 3600) return `${(seconds / 3600).toFixed(1)}h`
    if (seconds >= 60) return `${(seconds / 60).toFixed(1)}m`
    return `${seconds.toFixed(0)}s`
}

const UNIT_SECONDS: Record<string, number> = { h: 3600, m: 60, s: 1 }

export function humanToNs(input: string): number {
    const match = input.trim().match(/^(\d+(?:\.\d+)?)\s*(h|m|s)$/i)
    if (!match) return 0
    const [, amount, unit] = match
    return Math.round(Number(amount) * UNIT_SECONDS[unit.toLowerCase()] * 1e9)
}
