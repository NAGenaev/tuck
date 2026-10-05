// SPDX-License-Identifier: AGPL-3.0-only
import { Modal } from '@mantine/core'
import { ElementType, ReactNode } from 'react'

import modalStyles from '../kit/modal/modal.module.css'

/**
 * Legacy modal with a react-icons glyph in the header. Same frame as the kit's Modal;
 * new code should prefer `Modal` from '@shared/ui/kit' (footer slot, busy state, tones).
 */
export function EntityModal({
    opened,
    onClose,
    icon: Icon,
    color = 'cyan',
    title,
    size = 'md',
    children
}: {
    children: ReactNode
    color?: string
    icon: ElementType<{ size: number }>
    onClose: () => void
    opened: boolean
    size?: 'lg' | 'md' | 'sm' | 'xl'
    title: string
}) {
    const danger = color === 'red' || color === 'pink'
    return (
        <Modal
            data-danger={danger || undefined}
            onClose={onClose}
            opened={opened}
            size={size}
            title={
                <span className={modalStyles.head}>
                    <span className={modalStyles.ico} data-tone={danger ? 'danger' : 'brass'}>
                        <Icon size={16} />
                    </span>
                    <span className={modalStyles.headText}>{title}</span>
                </span>
            }
        >
            {children}
        </Modal>
    )
}
