// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Select, TextInput } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { KeyboardEvent, ReactNode, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cx, Icon, SkeletonLine } from '@shared/ui/kit'
import { Eyebrow, Plate } from '@shared/ui/tuck/tuck'

import { listNamespaces } from '../api'
import styles from '../auth-methods.module.css'

export const rich = { b: <b />, br: <br />, code: <code />, em: <em /> }

/** Field wrapper: caption, optional right-hand hint, message line that turns red on error. */
export function Field({
    label,
    hint,
    error,
    children,
    right
}: {
    children: ReactNode
    error?: string
    hint?: ReactNode
    label: ReactNode
    right?: ReactNode
}) {
    return (
        <div className={cx(styles.fld, error && styles.err)}>
            <div className={styles.lbl}>
                <span>{label}</span>
                {right && <i>{right}</i>}
            </div>
            {children}
            {(error || hint) && <span className={styles.msg}>{error || hint}</span>}
        </div>
    )
}

/**
 * Chips input. Enter commits the draft; with `comma` a comma (or space) does too. `pattern` rejects
 * malformed entries, the rejected text stays in the box so it can be fixed.
 */
export function TagInput({
    value,
    onChange,
    pattern,
    placeholder,
    comma = true,
    ariaLabel,
    onInvalid
}: {
    ariaLabel: string
    /** commit on , and space as well (off for values that legitimately contain them, e.g. LDAP DNs) */
    comma?: boolean
    onChange: (next: string[]) => void
    onInvalid?: (draft: string) => void
    pattern?: RegExp
    placeholder?: string
    value: string[]
}) {
    const [draft, setDraft] = useState('')
    const [bad, setBad] = useState(false)
    const ref = useRef<HTMLInputElement>(null)

    const commit = (raw: string) => {
        const parts = comma ? raw.split(/[,\s]+/) : [raw]
        const next = [...value]
        let rejected = ''
        for (const p of parts.map((x) => x.trim()).filter(Boolean)) {
            if (pattern && !pattern.test(p)) {
                rejected = rejected ? `${rejected} ${p}` : p
                continue
            }
            if (!next.includes(p)) next.push(p)
        }
        onChange(next)
        setDraft(rejected)
        setBad(!!rejected)
        if (rejected) onInvalid?.(rejected)
    }

    const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter' || (comma && (e.key === ',' || e.key === ' '))) {
            if (draft.trim()) {
                e.preventDefault()
                commit(draft)
            } else if (e.key !== 'Enter') {
                e.preventDefault()
            }
        } else if (e.key === 'Backspace' && !draft && value.length) {
            onChange(value.slice(0, -1))
        }
    }

    return (
        <div
            className={styles.tagbox}
            onClick={() => ref.current?.focus()}
            style={bad ? { borderColor: 'var(--alarm)' } : undefined}
        >
            {value.map((v) => (
                <span className={styles.chip} key={v}>
                    {v}
                    <button
                        aria-label={`${ariaLabel}: ${v} ×`}
                        onClick={(e) => {
                            e.stopPropagation()
                            onChange(value.filter((x) => x !== v))
                        }}
                        type="button"
                    >
                        ×
                    </button>
                </span>
            ))}
            <input
                aria-label={ariaLabel}
                onBlur={() => draft.trim() && commit(draft)}
                onChange={(e) => {
                    setDraft(e.target.value)
                    setBad(false)
                }}
                onKeyDown={onKey}
                placeholder={value.length ? '' : placeholder}
                ref={ref}
                value={draft}
            />
        </div>
    )
}

/** tuck_namespace picker: a Select over /v1/sys/namespaces, or a plain input if the list is denied. */
export function NamespaceField({
    value,
    onChange,
    error
}: {
    error?: string
    onChange: (v: string) => void
    value: string
}) {
    const { t } = useTranslation('auth')
    const id = useId()
    const { data, isError } = useQuery({
        queryKey: ['auth-methods', 'namespaces'],
        queryFn: listNamespaces,
        staleTime: 60_000,
        retry: false
    })
    const names = Array.from(new Set([...(data ?? []).map((n) => n.replace(/\/$/, '')), ...(value ? [value] : [])]))
    return (
        <Field error={error} hint={t('f.nsHint')} label={t('f.ns')}>
            {isError ? (
                <TextInput
                    aria-label={t('f.ns')}
                    id={id}
                    onChange={(e) => onChange(e.currentTarget.value.trim())}
                    placeholder={t('f.nsPh')}
                    value={value}
                />
            ) : (
                <Select
                    aria-label={t('f.ns')}
                    clearable
                    data={names}
                    id={id}
                    onChange={(v) => onChange(v ?? '')}
                    placeholder={t('f.nsPh')}
                    searchable
                    value={value || null}
                />
            )}
        </Field>
    )
}

export function PolicyField({
    value,
    onChange,
    error
}: {
    error?: string
    onChange: (v: string[]) => void
    value: string[]
}) {
    const { t } = useTranslation('auth')
    return (
        <Field error={error} hint={t('f.policiesHint')} label={t('f.policies')}>
            <TagInput
                ariaLabel={t('f.policies')}
                onChange={onChange}
                pattern={/^[A-Za-z0-9_.:/@-]+$/}
                placeholder={t('f.policiesPh')}
                value={value}
            />
        </Field>
    )
}

export function PolicyChips({ list, tone }: { list?: string[]; tone?: 'ice' }) {
    if (!list?.length) return <span className={styles.dim}>—</span>
    return (
        <div className={styles.chips}>
            {list.map((p) => (
                <span className={cx(styles.chip, tone === 'ice' && styles.ice)} key={p}>
                    {p}
                </span>
            ))}
        </div>
    )
}

export function ForbiddenPlate({
    request,
    need,
    onRetry,
    retrying
}: {
    need: string
    onRetry: () => void
    request: string
    retrying?: boolean
}) {
    const { t } = useTranslation('auth')
    return (
        <Plate variant="danger">
            <div className={styles.stageState}>
                <div className={styles.big403}>403</div>
                <div>
                    <Eyebrow>{t('forbidden.eyebrow')}</Eyebrow>
                    <h2>{t('forbidden.title')}</h2>
                    <p className={styles.stateP}>{t('forbidden.text')}</p>
                    <pre className={styles.req}>
                        <span className={styles.c}>{t('forbidden.request')}</span>
                        {'\n'}
                        {request}
                        {'\n'}
                        <span className={styles.c}>{t('forbidden.need')}</span>
                        {'\n'}
                        <span className={styles.k}>path</span> <span className={styles.s}>&quot;{need}&quot;</span>{' '}
                        {'{ capabilities = ["read", "list"] }'}
                    </pre>
                    <Button
                        leftSection={<Icon name="refresh" size={14} />}
                        loading={retrying}
                        onClick={onRetry}
                        variant="default"
                    >
                        {t('forbidden.retry')}
                    </Button>
                </div>
            </div>
        </Plate>
    )
}

export function PaneSkeleton({ request }: { request: string }) {
    const { t } = useTranslation('auth')
    return (
        <div aria-busy className={styles.skelGrid}>
            <SkeletonLine height={64} />
            <SkeletonLine height={180} />
            <div className={styles.boot}>
                <i className={styles.spin} />
                {request} … {t('loading')}
            </div>
        </div>
    )
}

export function SectionHead({
    eyebrow,
    title,
    text,
    actions,
    first
}: {
    actions?: ReactNode
    eyebrow: string
    first?: boolean
    text?: ReactNode
    title: string
}) {
    return (
        <div className={cx(styles.sechead, first && styles.firstSec)}>
            <div>
                <Eyebrow>{eyebrow}</Eyebrow>
                <h2>{title}</h2>
                {text && <p>{text}</p>}
            </div>
            {actions && <div className={styles.secActs}>{actions}</div>}
        </div>
    )
}
