// SPDX-License-Identifier: AGPL-3.0-only
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from '../cx'
import { Icon } from '../icons'
import { toast } from '../toast/toast'
import styles from './copy.module.css'

/** Clipboard write that also works on plain http (execCommand fallback). Resolves true on success. */
export async function copyText(text: string): Promise<boolean> {
    try {
        if (navigator.clipboard && window.isSecureContext) {
            await navigator.clipboard.writeText(text)
            return true
        }
    } catch {
        // fall through to the legacy path
    }
    try {
        const area = document.createElement('textarea')
        area.value = text
        area.setAttribute('readonly', '')
        area.style.cssText = 'position:fixed;opacity:0;pointer-events:none'
        document.body.appendChild(area)
        area.select()
        const ok = document.execCommand('copy')
        area.remove()
        return ok
    } catch {
        return false
    }
}

/**
 * Copy with a transient `copied` flag and an optional toast.
 * The toast never echoes the value unless `showValue` is set (secrets stay off screen).
 */
export function useCopy({
    timeout = 1800,
    toast: withToast = true,
    showValue = false
}: { showValue?: boolean; timeout?: number; toast?: boolean } = {}) {
    const { t } = useTranslation('forms')
    const [copied, setCopied] = useState(false)
    const timer = useRef<number | undefined>(undefined)

    useEffect(() => () => window.clearTimeout(timer.current), [])

    const copy = useCallback(
        async (text: string) => {
            const ok = await copyText(text)
            if (!ok) {
                toast.error(t('copy.failed'))
                return false
            }
            setCopied(true)
            window.clearTimeout(timer.current)
            timer.current = window.setTimeout(() => setCopied(false), timeout)
            if (withToast) {
                toast.success(
                    t('copy.copied'),
                    showValue ? <code>{text.length > 40 ? `${text.slice(0, 38)}…` : text}</code> : undefined,
                    2600
                )
            }
            return true
        },
        [showValue, t, timeout, withToast]
    )

    return { copied, copy }
}

/** Square icon button that copies `value`; swaps to a mint check for ~2s. */
export function CopyButton({
    value,
    label,
    size = 32,
    toast: withToast = true,
    showValue,
    className,
    onCopied
}: {
    className?: string
    /** accessible name; defaults to "Copy" */
    label?: string
    onCopied?: () => void
    /** echo the value in the toast (off by default, do not enable for secrets) */
    showValue?: boolean
    /** px edge of the square */
    size?: number
    /** show a "Copied" toast (default true) */
    toast?: boolean
    value: string
}) {
    const { t } = useTranslation('forms')
    const { copied, copy } = useCopy({ showValue, toast: withToast })
    const name = label ?? t('copy.copy')

    return (
        <button
            aria-label={copied ? t('copy.copied') : name}
            className={cx(styles.btn, className)}
            data-done={copied || undefined}
            onClick={async (e) => {
                e.stopPropagation()
                if (await copy(value)) onCopied?.()
            }}
            style={{ height: size, width: size }}
            title={name}
            type="button"
        >
            <Icon name={copied ? 'check' : 'copy'} size={Math.round(size * 0.46)} />
        </button>
    )
}

/**
 * Inline mono value that copies itself on click (the artifact's .idc).
 * `display` lets you show a shortened form while copying the full `value`.
 */
export function CopyableValue({
    value,
    display,
    maxWidth,
    className,
    toast: withToast = false,
    showValue
}: {
    className?: string
    display?: string
    /** css max-width of the text part; longer text is ellipsised */
    maxWidth?: number | string
    showValue?: boolean
    /** show a toast on copy (default false: the inline check is feedback enough) */
    toast?: boolean
    value: string
}) {
    const { t } = useTranslation('forms')
    const { copied, copy } = useCopy({ showValue, toast: withToast })
    const shown = display ?? value

    return (
        <button
            aria-label={t('copy.copyValue', { value: shown })}
            className={cx(styles.value, className)}
            data-done={copied || undefined}
            onClick={(e) => {
                e.stopPropagation()
                void copy(value)
            }}
            title={value}
            type="button"
        >
            <span className={styles.valueText} style={maxWidth === undefined ? undefined : { maxWidth }}>
                {shown}
            </span>
            <Icon name={copied ? 'check' : 'copy'} size={12} />
        </button>
    )
}
