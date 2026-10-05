// SPDX-License-Identifier: AGPL-3.0-only
import { FocusTrap } from '@mantine/core'
import { CSSProperties, ReactNode, useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'

import c from './console.module.css'
import { ShellIcon } from './icons'
import styles from './layer.module.css'

function useLayerEffects(opened: boolean, onClose: () => void) {
    const last = useRef<HTMLElement | null>(null)

    useEffect(() => {
        if (!opened) return
        last.current = document.activeElement as HTMLElement | null
        const prevOverflow = document.body.style.overflow
        document.body.style.overflow = 'hidden'
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose()
        }
        document.addEventListener('keydown', onKey)
        return () => {
            document.body.style.overflow = prevOverflow
            document.removeEventListener('keydown', onKey)
            try {
                last.current?.focus({ preventScroll: true })
            } catch {
                /* element is gone */
            }
        }
    }, [opened, onClose])
}

export function Scrim({ onClick, opened }: { onClick: () => void; opened: boolean }) {
    return <div className={styles.scrim} data-open={opened} onClick={onClick} />
}

/** Riveted-plate modal with scrim, focus trap, Escape and a close cross. */
export function ConsoleModal({
    children,
    closeLabel,
    labelledBy,
    onClose,
    opened,
    tone,
    width
}: {
    children: ReactNode
    closeLabel?: string
    labelledBy: string
    onClose: () => void
    opened: boolean
    tone?: 'red'
    width?: number
}) {
    useLayerEffects(opened, onClose)
    return createPortal(
        <>
            <Scrim onClick={onClose} opened={opened} />
            <div
                aria-hidden={!opened}
                aria-labelledby={labelledBy}
                aria-modal="true"
                className={`${c.plate} ${styles.modal}`}
                data-open={opened}
                data-tone={tone}
                role="dialog"
                style={{ '--w': `${width ?? 480}px` } as CSSProperties}
            >
                <i className={c.rivet} />
                <i className={c.rivet} />
                <i className={c.rivet} />
                <i className={c.rivet} />
                {closeLabel && (
                    <button aria-label={closeLabel} className={styles.x} onClick={onClose} type="button">
                        <ShellIcon name="x" />
                    </button>
                )}
                <FocusTrap active={opened}>
                    <div>{opened ? children : null}</div>
                </FocusTrap>
            </div>
        </>,
        document.body
    )
}

export function ConsoleDrawer({
    children,
    closeLabel,
    header,
    label,
    onClose,
    opened
}: {
    children: ReactNode
    closeLabel: string
    header: ReactNode
    label: string
    onClose: () => void
    opened: boolean
}) {
    useLayerEffects(opened, onClose)
    return createPortal(
        <>
            <Scrim onClick={onClose} opened={opened} />
            <aside aria-hidden={!opened} aria-label={label} className={styles.drawer} data-open={opened}>
                <FocusTrap active={opened}>
                    <div style={{ display: 'flex', flexDirection: 'column', minHeight: 0, flex: 1 }}>
                        <header className={styles.drawerHead}>
                            {header}
                            <button aria-label={closeLabel} className={styles.x} onClick={onClose} style={{ position: 'static' }} type="button">
                                <ShellIcon name="x" />
                            </button>
                        </header>
                        <div className={styles.drawerBody}>{children}</div>
                    </div>
                </FocusTrap>
            </aside>
        </>,
        document.body
    )
}

export const layerStyles = styles
