// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Modal as MantineModal } from '@mantine/core'
import { ReactNode, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from '../cx'
import { Icon, IconName } from '../icons'
import styles from './modal.module.css'

/** Boxed icon + title used in the header of Modal and Drawer (the artifact's .m-ico + .m-head). */
export function OverlayTitle({
    icon,
    tone = 'brass',
    children
}: {
    children: ReactNode
    icon?: IconName
    tone?: 'brass' | 'danger'
}) {
    return (
        <span className={styles.head}>
            {icon && (
                <span className={styles.ico} data-tone={tone}>
                    <Icon name={icon} size={16} />
                </span>
            )}
            <span className={styles.headText}>{children}</span>
        </span>
    )
}

/**
 * Modal on the shared frame: header with optional boxed icon, padded body, footer strip with
 * right-aligned buttons. `tone="danger"` switches the frame and the icon to alarm.
 * While `busy` the modal cannot be dismissed.
 */
export function Modal({
    opened,
    onClose,
    title,
    icon,
    tone = 'brass',
    size,
    footer,
    busy,
    children
}: {
    /** disables Esc, outside click and the close button */
    busy?: boolean
    children?: ReactNode
    /** buttons row, rendered in the footer strip (put the primary action last) */
    footer?: ReactNode
    icon?: IconName
    onClose: () => void
    opened: boolean
    /** px or a Mantine size, default 520 from the theme */
    size?: number | string
    title: ReactNode
    tone?: 'brass' | 'danger'
}) {
    return (
        <MantineModal
            closeButtonProps={{ disabled: busy }}
            closeOnClickOutside={!busy}
            closeOnEscape={!busy}
            data-danger={tone === 'danger' || undefined}
            onClose={onClose}
            opened={opened}
            size={size}
            title={
                <OverlayTitle icon={icon} tone={tone}>
                    {title}
                </OverlayTitle>
            }
            trapFocus
        >
            <div className={cx(styles.main, !footer && styles.mainBare)}>{children}</div>
            {footer && <div className={styles.foot}>{footer}</div>}
        </MantineModal>
    )
}

/**
 * Confirmation dialog.
 * - `destructive` (default) uses the danger frame and a red confirm button.
 * - `confirmText` makes the user type that exact string before the button unlocks.
 * - `onConfirm` may return a promise: the button shows a spinner while it runs and the dialog
 *   closes itself when it resolves (a rejection leaves it open for the caller's own error toast).
 *   A plain void return leaves closing to the caller. `loading` forces the busy state from outside.
 */
export function ConfirmModal({
    opened,
    onClose,
    onConfirm,
    title,
    message,
    confirmLabel,
    cancelLabel,
    destructive = true,
    confirmText,
    loading,
    icon
}: {
    cancelLabel?: string
    confirmLabel?: string
    /** user must type this exact value to enable the confirm button (secret path, namespace name) */
    confirmText?: string
    destructive?: boolean
    icon?: IconName
    loading?: boolean
    message?: ReactNode
    onClose: () => void
    onConfirm: () => Promise<unknown> | unknown
    opened: boolean
    title: ReactNode
}) {
    const { t } = useTranslation('forms')
    const [typed, setTyped] = useState('')
    const [pending, setPending] = useState(false)
    const busy = !!loading || pending
    const matches = confirmText === undefined || typed === confirmText

    useEffect(() => {
        if (opened) setTyped('')
    }, [opened])

    const submit = async () => {
        if (!matches || busy) return
        const result = onConfirm()
        if (result && typeof (result as Promise<unknown>).then === 'function') {
            setPending(true)
            try {
                await result
                onClose()
            } catch {
                // the caller reports the error; keep the dialog open so the user can retry
            } finally {
                setPending(false)
            }
        }
    }

    return (
        <Modal
            busy={busy}
            footer={
                <>
                    <Button disabled={busy} onClick={onClose} variant="default">
                        {cancelLabel ?? t('confirm.cancel')}
                    </Button>
                    <Button
                        color={destructive ? 'red' : undefined}
                        data-autofocus={confirmText === undefined ? true : undefined}
                        disabled={!matches}
                        loading={busy}
                        onClick={submit}
                    >
                        {confirmLabel ?? (destructive ? t('confirm.delete') : t('confirm.ok'))}
                    </Button>
                </>
            }
            icon={icon ?? (destructive ? 'trash' : 'info')}
            onClose={onClose}
            opened={opened}
            size={480}
            title={title}
            tone={destructive ? 'danger' : 'brass'}
        >
            {message && <div className={styles.message}>{message}</div>}
            {confirmText !== undefined && (
                <div className={styles.confirm}>
                    <label className={styles.confirmLabel} htmlFor="tk-confirm-input">
                        {t('confirm.typeToConfirm')} <code>{confirmText}</code>
                    </label>
                    <input
                        autoComplete="off"
                        autoFocus
                        className={styles.confirmInput}
                        data-good={(typed.length > 0 && matches) || undefined}
                        disabled={busy}
                        id="tk-confirm-input"
                        onChange={(e) => setTyped(e.currentTarget.value)}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') void submit()
                        }}
                        spellCheck={false}
                        value={typed}
                    />
                    <span className={styles.confirmHint} data-good={matches || undefined}>
                        {matches ? (
                            <>
                                <Icon name="check" size={13} />
                                {t('confirm.matched')}
                            </>
                        ) : (
                            t('confirm.hint')
                        )}
                    </span>
                </div>
            )}
        </Modal>
    )
}
