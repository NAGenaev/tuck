// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { ElementType, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from '../cx'
import { Icon, IconName } from '../icons'
import { SkeletonLine, Spinner } from '../skeleton/skeleton'
import styles from './state-block.module.css'

export type StateKind = 'empty' | 'error' | 'forbidden' | 'loading'

const DEFAULT_ICON: Record<Exclude<StateKind, 'loading'>, IconName> = {
    empty: 'inbox',
    error: 'alert',
    forbidden: 'lock'
}

/**
 * One block for every non-happy path of a screen: empty list, load error, 403, loading.
 * Dashed striped frame with a boxed icon (the artifact's .empty).
 * Without `title` it collapses to a compact one-liner, which is what EmptyState uses.
 */
export function StateBlock({
    kind = 'empty',
    icon,
    title,
    message,
    details,
    action,
    onRetry,
    retrying,
    compact,
    className
}: {
    /** extra node under the text, e.g. a "Create" button */
    action?: ReactNode
    className?: string
    /** smaller paddings, for use inside a card or table */
    compact?: boolean
    /** mono line, typically the raw server error or status code */
    details?: string
    /** a react-icons component (EmptyState legacy) or a kit icon name */
    icon?: ElementType<{ size: number }> | IconName
    kind?: StateKind
    message?: ReactNode
    /** shows a Retry button (error and forbidden kinds) */
    onRetry?: () => void
    retrying?: boolean
    /** defaults to a localized heading for error / forbidden / loading */
    title?: ReactNode
}) {
    const { t } = useTranslation('forms')
    const heading =
        title ?? (kind === 'empty' ? undefined : (t(`state.${kind}.title`) as string))
    const text =
        message ?? (kind === 'empty' || title !== undefined ? undefined : (t(`state.${kind}.message`) as string))

    let glyph: ReactNode
    if (kind === 'loading') {
        glyph = <Spinner size={22} />
    } else if (typeof icon === 'string') {
        glyph = <Icon name={icon as IconName} size={compact ? 18 : 22} />
    } else if (icon) {
        const Custom = icon
        glyph = <Custom size={compact ? 18 : 22} />
    } else {
        glyph = <Icon name={DEFAULT_ICON[kind]} size={compact ? 18 : 22} />
    }

    return (
        <div
            aria-busy={kind === 'loading' || undefined}
            className={cx(styles.state, className)}
            data-compact={compact || !heading || undefined}
            data-kind={kind}
            role={kind === 'error' ? 'alert' : 'status'}
        >
            <span className={styles.ico}>{glyph}</span>
            {heading && <h4 className={styles.title}>{heading}</h4>}
            {text && <p className={styles.text}>{text}</p>}
            {details && <code className={styles.details}>{details}</code>}
            {kind === 'loading' && !text && !heading && <SkeletonLine width={140} />}
            {(onRetry || action) && (
                <div className={styles.actions}>
                    {onRetry && kind !== 'loading' && (
                        <Button
                            leftSection={<Icon name="refresh" size={14} />}
                            loading={retrying}
                            onClick={onRetry}
                            size="sm"
                            variant="default"
                        >
                            {t('state.retry')}
                        </Button>
                    )}
                    {action}
                </div>
            )}
        </div>
    )
}
