// SPDX-License-Identifier: AGPL-3.0-only
import { useId } from 'react'

import { cx } from '../cx'
import styles from './sparkline.module.css'

export type SparkTone = 'bad' | 'brass' | 'ice' | 'ok' | 'violet'

const COLOR: Record<SparkTone, string> = {
    brass: 'var(--brass)',
    ok: 'var(--mint)',
    bad: 'var(--alarm)',
    ice: 'var(--ice)',
    violet: 'var(--violet)'
}

const W = 100

/**
 * Tiny SVG area chart. Pass raw numbers; it scales min..max into the box and stretches to the
 * parent's width (set `width` for a fixed px size). A glowing dot marks the latest value.
 */
export function Sparkline({
    data,
    tone = 'brass',
    width,
    height = 36,
    fill = true,
    label,
    className
}: {
    className?: string
    data: number[]
    /** area under the line */
    fill?: boolean
    height?: number
    /** accessible description, e.g. "Requests per minute" */
    label?: string
    tone?: SparkTone
    /** px; omit to fill the container */
    width?: number
}) {
    const gid = useId()
    const pad = 3
    const pts = data.length > 1 ? data : [data[0] ?? 0, data[0] ?? 0]
    const min = Math.min(...pts)
    const max = Math.max(...pts)
    const span = max - min || 1
    const x = (i: number) => (i / (pts.length - 1)) * W
    const y = (v: number) => height - pad - ((v - min) / span) * (height - pad * 2)
    const line = pts.map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(2)} ${y(v).toFixed(2)}`).join(' ')
    const lastY = y(pts[pts.length - 1])

    return (
        <span
            className={cx(styles.spark, className)}
            style={{ height, width: width ?? '100%', ['--c' as string]: COLOR[tone] }}
        >
            <svg
                aria-label={label}
                height="100%"
                preserveAspectRatio="none"
                role={label ? 'img' : undefined}
                viewBox={`0 0 ${W} ${height}`}
                width="100%"
                aria-hidden={label ? undefined : true}
            >
                <defs>
                    <linearGradient id={gid} x1="0" x2="0" y1="0" y2="1">
                        <stop offset="0" stopColor="var(--c)" stopOpacity="0.32" />
                        <stop offset="1" stopColor="var(--c)" stopOpacity="0" />
                    </linearGradient>
                </defs>
                {fill && <path d={`${line} L${W} ${height} L0 ${height} Z`} fill={`url(#${gid})`} />}
                <path
                    className={styles.line}
                    d={line}
                    fill="none"
                    pathLength={1}
                    stroke="var(--c)"
                    strokeLinejoin="round"
                    strokeWidth="1.6"
                    vectorEffect="non-scaling-stroke"
                />
            </svg>
            <i className={styles.dot} style={{ top: `${(lastY / height) * 100}%` }} />
        </span>
    )
}
