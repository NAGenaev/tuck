// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from '../cx'
import styles from './countdown.module.css'

const pad = (n: number) => String(n).padStart(2, '0')

/** "2d 04:12:09", "04:12:09" or "12:09" depending on how much is left. */
export function formatRemaining(ms: number, dayLabel = 'd') {
    const s = Math.max(0, Math.floor(ms / 1000))
    const d = Math.floor(s / 86400)
    const h = Math.floor((s % 86400) / 3600)
    const m = Math.floor((s % 3600) / 60)
    const sec = s % 60
    if (d > 0) return `${d}${dayLabel} ${pad(h)}:${pad(m)}:${pad(sec)}`
    if (h > 0) return `${pad(h)}:${pad(m)}:${pad(sec)}`
    return `${pad(m)}:${pad(sec)}`
}

/**
 * Live countdown to `until` (Date, epoch ms or ISO string) for token / lease / TTL expiry.
 * The text and the bar slide from brass to alarm as time runs out. `total` (ms) is the full
 * lifetime used for the bar; without it the remaining time at mount is used.
 */
export function Countdown({
    until,
    total,
    showBar = true,
    onExpire,
    className
}: {
    className?: string
    onExpire?: () => void
    showBar?: boolean
    /** full lifetime in ms, enables a bar that reflects real progress */
    total?: number
    until: Date | number | string
}) {
    const { t } = useTranslation('forms')
    const target = new Date(until).getTime()
    const [now, setNow] = useState(() => Date.now())
    const base = useRef(Math.max(1, total ?? target - Date.now()))
    const fired = useRef(false)

    useEffect(() => {
        base.current = Math.max(1, total ?? target - Date.now())
        fired.current = false
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [target, total])

    useEffect(() => {
        const id = window.setInterval(() => setNow(Date.now()), 1000)
        return () => window.clearInterval(id)
    }, [])

    const left = target - now
    const expired = left <= 0

    useEffect(() => {
        if (expired && !fired.current) {
            fired.current = true
            onExpire?.()
        }
    }, [expired, onExpire])

    const frac = Math.min(1, Math.max(0, left / base.current))
    // brass while plenty is left, drifting to alarm over the last 35%
    const heat = Math.round(Math.min(100, Math.max(0, (1 - frac / 0.35) * 100)))
    const color = `color-mix(in srgb, var(--alarm) ${heat}%, var(--brass))`

    return (
        <span
            className={cx(styles.cd, className)}
            data-expired={expired || undefined}
            style={{ ['--cd' as string]: expired ? 'var(--alarm)' : color }}
        >
            <span className={styles.time}>
                {expired ? t('countdown.expired') : formatRemaining(left, t('countdown.day'))}
            </span>
            {showBar && (
                <span aria-hidden className={styles.track}>
                    <i style={{ width: `${frac * 100}%` }} />
                </span>
            )}
        </span>
    )
}
