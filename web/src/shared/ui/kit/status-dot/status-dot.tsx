// SPDX-License-Identifier: AGPL-3.0-only
import { ReactNode } from 'react'

import { cx } from '../cx'
import styles from './status-dot.module.css'

export type DotTone = 'bad' | 'info' | 'off' | 'ok' | 'warn'

/** Glowing status dot with an optional mono label. `live` makes it pulse (a calm 2s breathe). */
export function StatusDot({
    tone = 'ok',
    live,
    children,
    className
}: {
    children?: ReactNode
    className?: string
    /** pulse: use for "this is happening now" states */
    live?: boolean
    tone?: DotTone
}) {
    return (
        <span className={cx(styles.dot, className)} data-live={live || undefined} data-tone={tone}>
            {children}
        </span>
    )
}
