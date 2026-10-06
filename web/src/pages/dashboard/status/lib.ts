// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from 'react'

import { prefersReducedMotion } from '@shared/ui/kit'

export type ViewState = 'auto' | 'error' | 'loading' | 'ok' | 'sealed'

const GLYPHS = '0123456789ABCDEF#%$@'

/** Scrambles into `text` whenever it changes (the artifact's "decode" effect). */
export function useDecode(text: string): string {
    const [shown, setShown] = useState(text)

    useEffect(() => {
        if (prefersReducedMotion()) {
            setShown(text)
            return
        }
        const total = 22
        let frame = 0
        const id = window.setInterval(() => {
            frame += 1
            if (frame >= total) {
                window.clearInterval(id)
                setShown(text)
                return
            }
            let out = ''
            for (let i = 0; i < text.length; i++) {
                out +=
                    i < (text.length * frame) / total || text[i] === ' '
                        ? text[i]
                        : GLYPHS[Math.floor(Math.random() * GLYPHS.length)]
            }
            setShown(out)
        }, 36)
        return () => window.clearInterval(id)
    }, [text])

    return shown
}

const pad = (n: number) => (n < 10 ? '0' : '') + n

/** Uptime ticking every second between server refreshes. Returns { d, hms } or null. */
export function useUptime(seconds: number | undefined, updatedAt: number) {
    const [now, setNow] = useState(() => Date.now())

    useEffect(() => {
        if (seconds === undefined) return
        const id = window.setInterval(() => setNow(Date.now()), 1000)
        return () => window.clearInterval(id)
    }, [seconds])

    if (seconds === undefined) return null
    const u = Math.max(0, Math.floor(seconds + (now - updatedAt) / 1000))
    return {
        d: Math.floor(u / 86400),
        hms: `${pad(Math.floor((u % 86400) / 3600))}:${pad(Math.floor((u % 3600) / 60))}:${pad(u % 60)}`
    }
}

const MAX_SAMPLES = 60

/**
 * Real samples of a polled value taken while the page is open (one per refetch).
 * Nothing is invented: the chart only ever shows what the API returned during this visit.
 */
export function useSamples(value: number | undefined, stamp: number): number[] {
    const [samples, setSamples] = useState<number[]>([])

    useEffect(() => {
        if (value === undefined || !stamp) return
        setSamples((prev) => [...prev, value].slice(-MAX_SAMPLES))
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [stamp])

    return samples
}

export function formatClock(ts: number): string {
    const d = new Date(ts)
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}
