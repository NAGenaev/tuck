// SPDX-License-Identifier: AGPL-3.0-only
import { ReactNode, useEffect } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { Icon, useCopy } from '@shared/ui/kit'

import { Op, Problem } from '../lib'
import styles from '../wrapping.module.css'
import { WIcon, WrapIconName } from './icons'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

export const rich = { b: <b />, code: <code /> }

/** Token in a mono well with a labelled copy button (the artifact's .tokbox). */
export function TokBox({
    emptyText,
    onCopied,
    value
}: {
    emptyText?: string
    onCopied?: () => void
    value?: null | string
}) {
    const { t } = useTranslation('wrapping')
    const { copied, copy } = useCopy({ toast: false })
    return (
        <div className={styles.tokbox}>
            <code className={value ? undefined : styles.tokEmpty} title={value ?? undefined}>
                {value || emptyText}
            </code>
            <button
                className={copied ? styles.done : undefined}
                disabled={!value}
                onClick={async () => {
                    if (value && (await copy(value))) onCopied?.()
                }}
                type="button"
            >
                <Icon name={copied ? 'check' : 'copy'} size={14} />
                <span>{copied ? t('copied') : t('copy')}</span>
            </button>
        </div>
    )
}

/** Inline field error (.msg). */
export function Msg({ children }: { children?: ReactNode }) {
    if (!children) return null
    return (
        <div className={cx(styles.msg, styles.msgErr)} role="alert">
            {children}
        </div>
    )
}

/** Alert box in one of three tones, optionally shaking on arrival. */
export function Alert({
    children,
    icon,
    shake,
    title,
    tone
}: {
    children?: ReactNode
    icon: WrapIconName | 'warn'
    shake?: boolean
    title: ReactNode
    tone: 'b' | 'i' | 'r'
}) {
    return (
        <div
            className={cx(styles.alert, styles[`alert_${tone}`], shake && styles.shake)}
            role="alert"
        >
            {icon === 'warn' ? <Icon name="warn" size={22} /> : <WIcon name={icon} size={22} />}
            <h4>{title}</h4>
            {children && <p>{children}</p>}
        </div>
    )
}

const CAP: Record<Op, string> = {
    lookup: 'update',
    revoke: 'delete',
    unwrap: 'update',
    wrap: 'update'
}

/** Real API failure of an operation, in the artifact's alert styling (403 / 404 / 410 / other). */
export function ProblemAlert({ op, problem }: { op: Op; problem: Problem }) {
    const { t } = useTranslation('wrapping')
    const s = problem.status
    if (s === 403) {
        return (
            <Alert icon="lock" shake title={t('problem.forbidden.title')} tone="r">
                <Trans
                    components={rich}
                    i18nKey="problem.forbidden.text"
                    ns="wrapping"
                    values={{ cap: CAP[op], path: `sys/wrapping/${op}` }}
                />
            </Alert>
        )
    }
    if (s === 410) {
        return (
            <Alert icon="clock" shake title={t('problem.expired.title')} tone="b">
                {t('problem.expired.text')}
            </Alert>
        )
    }
    if (s === 404) {
        return (
            <Alert icon="warn" shake title={t(`problem.notFound.${op === 'unwrap' ? 'unwrap' : 'other'}Title`)} tone="r">
                <Trans
                    components={rich}
                    i18nKey={`problem.notFound.${op === 'unwrap' ? 'unwrapText' : 'otherText'}`}
                    ns="wrapping"
                />
            </Alert>
        )
    }
    return (
        <Alert
            icon="warn"
            shake
            title={s ? t('problem.server.title', { status: s }) : t('problem.network.title')}
            tone="r"
        >
            {problem.message ?? t(s ? 'problem.server.text' : 'problem.network.text')}
        </Alert>
    )
}

const TOKEN =
    /("(?:\\.|[^"\\])*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|\b(true|false|null)\b/g

/** Pretty JSON where keys stay readable and every value is blurred until the viewer reveals. */
export function MaskedJson({ revealed, text }: { revealed: boolean; text: string }) {
    const parts: ReactNode[] = []
    let last = 0
    let n = 0
    for (const m of text.matchAll(TOKEN)) {
        const at = m.index ?? 0
        if (at > last) parts.push(text.slice(last, at))
        if (m[1] && m[2]) {
            parts.push(
                <span className={styles.jk} key={n++}>
                    {m[1]}
                </span>,
                m[2]
            )
        } else {
            parts.push(
                <span className={cx(m[1] ? styles.js : styles.jn, styles.mask)} key={n++}>
                    {m[0]}
                </span>
            )
        }
        last = at + m[0].length
    }
    if (last < text.length) parts.push(text.slice(last))
    return (
        <div className={cx(styles.json, revealed && styles.jsonReveal)} data-testid="wrap-json">
            {parts}
        </div>
    )
}

/** Hides again by itself after `seconds` once revealed. */
export function useAutoMask(revealed: boolean, hide: () => void, seconds = 30) {
    useEffect(() => {
        if (!revealed) return
        const id = window.setTimeout(hide, seconds * 1000)
        return () => window.clearTimeout(id)
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [revealed, seconds])
}
