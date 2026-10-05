// SPDX-License-Identifier: AGPL-3.0-only
import { CSSProperties, ReactNode } from 'react'

import styles from './tuck.module.css'

export function Eyebrow({ children }: { children: ReactNode }) {
    return <div className={styles.eyebrow}>{children}</div>
}

export function Plate({
    children,
    className,
    style
}: {
    children: ReactNode
    className?: string
    style?: CSSProperties
}) {
    return (
        <div className={[styles.plate, className].filter(Boolean).join(' ')} style={style}>
            <i className={styles.rivet} />
            <i className={styles.rivet} />
            <i className={styles.rivet} />
            <i className={styles.rivet} />
            {children}
        </div>
    )
}

export function PlateBar({ children }: { children?: ReactNode }) {
    return (
        <div className={styles.plateBar}>
            <span />
            <span />
            <span />
            {children && <em>{children}</em>}
        </div>
    )
}

export function Term({ children }: { children: ReactNode }) {
    return <div className={styles.term}>{children}</div>
}

export function Cursor() {
    return <span className={styles.cursor} />
}

export function Bento({
    children,
    columns,
    style
}: {
    children: ReactNode
    columns: string
    style?: CSSProperties
}) {
    return (
        <div className={styles.bento} style={{ gridTemplateColumns: columns, ...style }}>
            {children}
        </div>
    )
}

export function Tile({
    label,
    value,
    hint,
    tone,
    children
}: {
    children?: ReactNode
    hint?: string
    label: string
    tone?: 'bad' | 'brass' | 'ok'
    value?: number | string
}) {
    return (
        <div className={styles.tile}>
            <span className={styles.tileLabel}>{label}</span>
            {value !== undefined && (
                <span className={styles.tileValue} data-tone={tone} title={String(value)}>
                    {value}
                </span>
            )}
            {children}
            {hint && <span className={styles.tileHint}>{hint}</span>}
        </div>
    )
}

export function Pill({ children, tone = 'brass' }: { children: ReactNode; tone?: 'bad' | 'brass' | 'ice' | 'ok' }) {
    return (
        <span className={styles.pill} data-tone={tone}>
            {children}
        </span>
    )
}
