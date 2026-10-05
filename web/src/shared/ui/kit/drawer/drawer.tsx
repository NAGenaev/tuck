// SPDX-License-Identifier: AGPL-3.0-only
import { Drawer as MantineDrawer } from '@mantine/core'
import { ReactNode } from 'react'

import { cx } from '../cx'
import { IconName } from '../icons'
import { OverlayTitle } from '../modal/modal'
import styles from './drawer.module.css'

/**
 * Side panel on the shared frame (440px, slides from the right, full width on phones).
 * `footer` is pinned to the bottom of the panel with equal-width buttons.
 */
export function Drawer({
    opened,
    onClose,
    title,
    icon,
    size,
    footer,
    busy,
    children
}: {
    busy?: boolean
    children?: ReactNode
    footer?: ReactNode
    icon?: IconName
    onClose: () => void
    opened: boolean
    /** px or a Mantine size, default 440 from the theme */
    size?: number | string
    title: ReactNode
}) {
    return (
        <MantineDrawer
            closeButtonProps={{ disabled: busy }}
            closeOnClickOutside={!busy}
            closeOnEscape={!busy}
            onClose={onClose}
            opened={opened}
            size={size}
            title={<OverlayTitle icon={icon}>{title}</OverlayTitle>}
            trapFocus
        >
            <div className={styles.shell}>
                <div className={cx(styles.main, !footer && styles.mainBare)}>{children}</div>
                {footer && <div className={styles.foot}>{footer}</div>}
            </div>
        </MantineDrawer>
    )
}

/** Mono definition grid for a drawer body (the artifact's .d-meta): [label, value] pairs. */
export function MetaGrid({ items }: { items: Array<[ReactNode, ReactNode]> }) {
    return (
        <dl className={styles.meta}>
            {items.map(([label, value], i) => (
                <div className={styles.metaRow} key={i}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                </div>
            ))}
        </dl>
    )
}
