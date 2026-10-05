// SPDX-License-Identifier: AGPL-3.0-only
import { CSSProperties } from 'react'

import { cx } from '../cx'
import styles from './skeleton.module.css'

/** One shimmering bar (the artifact's .sk). */
export function SkeletonLine({
    width = '100%',
    height = 12,
    className,
    style
}: {
    className?: string
    height?: number | string
    style?: CSSProperties
    width?: number | string
}) {
    return <span aria-hidden className={cx(styles.sk, className)} style={{ height, width, ...style }} />
}

/** Bordered card with a title bar and `lines` text bars of descending width (.sk-card). */
export function SkeletonCard({ lines = 3, title = true }: { lines?: number; title?: boolean }) {
    return (
        <div aria-busy className={styles.card}>
            {title && <SkeletonLine height={14} width="40%" />}
            {Array.from({ length: lines }, (_, i) => (
                <SkeletonLine key={i} width={`${100 - ((i * 17) % 55)}%`} />
            ))}
        </div>
    )
}

/** Placeholder rows for a table body; render inside <tbody>. */
export function SkeletonRows({ rows = 5, cols = 4 }: { cols?: number; rows?: number }) {
    return (
        <>
            {Array.from({ length: rows }, (_, r) => (
                <tr aria-busy key={r}>
                    {Array.from({ length: cols }, (_, c) => (
                        <td key={c}>
                            <SkeletonLine width={`${55 + ((r * 7 + c * 13) % 40)}%`} />
                        </td>
                    ))}
                </tr>
            ))}
        </>
    )
}

/** Spinning oval (2px line ring with a brass top); inherits size, 16px by default. */
export function Spinner({ size = 16, className }: { className?: string; size?: number }) {
    return <i aria-hidden className={cx(styles.spin, className)} style={{ height: size, width: size }} />
}
