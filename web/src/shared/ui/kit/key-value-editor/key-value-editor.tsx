// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Input } from '@mantine/core'
import { ReactNode, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from '../cx'
import { Icon } from '../icons'
import styles from './key-value-editor.module.css'

export interface KvPair {
    key: string
    value: string
}

export type KvIssue = 'badKey' | 'duplicate' | 'noKey'

export interface KvValidation {
    /** row index -> problem with that row's key */
    issues: Record<number, KvIssue>
    /** first problem found, in the order duplicate > badKey > noKey */
    problem?: KvIssue
    valid: boolean
}

export const KV_KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_.-]*$/

/** Pure validation, reusable before submit. Rows with neither key nor value are ignored. */
export function validateKv(pairs: KvPair[], pattern: RegExp = KV_KEY_PATTERN): KvValidation {
    const issues: Record<number, KvIssue> = {}
    const seen = new Set<string>()
    pairs.forEach((p, i) => {
        const key = p.key.trim()
        if (!key && !p.value) return
        if (!key) issues[i] = 'noKey'
        else if (!pattern.test(key)) issues[i] = 'badKey'
        else if (seen.has(key)) issues[i] = 'duplicate'
        seen.add(key)
    })
    const kinds = Object.values(issues)
    const problem = (['duplicate', 'badKey', 'noKey'] as const).find((k) => kinds.includes(k))
    return { issues, problem, valid: !problem }
}

let uid = 0
const nextId = () => `kv${++uid}`

/**
 * Editable list of key/value rows with live validation (bad / duplicate / orphan keys),
 * animated add and remove, and a status footer. Controlled: `value` + `onChange`.
 * The component only reports; call `validateKv(value)` (or use `onValidityChange`) to gate saving.
 */
export function KeyValueEditor({
    value,
    onChange,
    onValidityChange,
    label,
    description,
    addLabel,
    keyPattern,
    maxPairs,
    disabled,
    footerExtra,
    className
}: {
    addLabel?: string
    className?: string
    description?: ReactNode
    disabled?: boolean
    /** extra node on the right side of the footer, e.g. a "Format .env" button */
    footerExtra?: ReactNode
    keyPattern?: RegExp
    label?: ReactNode
    maxPairs?: number
    onChange: (pairs: KvPair[]) => void
    onValidityChange?: (valid: boolean) => void
    value: KvPair[]
}) {
    const { t } = useTranslation('forms')
    const [ids, setIds] = useState<string[]>(() => value.map(nextId))
    const [leaving, setLeaving] = useState<null | string>(null)
    const [focusId, setFocusId] = useState<null | string>(null)

    // keep the id list aligned with the value when the parent replaces it
    const rowIds = value.map((_, i) => ids[i] ?? `kv-pending-${i}`)
    useEffect(() => {
        if (ids.length !== value.length) {
            setIds((prev) => value.map((_, i) => prev[i] ?? nextId()))
        }
    }, [ids.length, value])

    const check = validateKv(value, keyPattern)
    const lastValid = useRef<boolean | undefined>(undefined)
    useEffect(() => {
        if (lastValid.current !== check.valid) {
            lastValid.current = check.valid
            onValidityChange?.(check.valid)
        }
    }, [check.valid, onValidityChange])

    const patch = (i: number, part: Partial<KvPair>) =>
        onChange(value.map((p, n) => (n === i ? { ...p, ...part } : p)))

    const add = () => {
        const id = nextId()
        setIds([...rowIds.filter((r) => !r.startsWith('kv-pending')), id])
        setFocusId(id)
        onChange([...value, { key: '', value: '' }])
    }

    const remove = (i: number) => {
        const id = rowIds[i]
        setLeaving(id)
        window.setTimeout(() => {
            setLeaving(null)
            setIds(rowIds.filter((_, n) => n !== i))
            onChange(value.filter((_, n) => n !== i))
        }, 240)
    }

    const n = value.length
    const full = maxPairs !== undefined && n >= maxPairs

    return (
        <Input.Wrapper className={className} description={description} label={label}>
            <div className={styles.kv} data-disabled={disabled || undefined}>
                <div aria-hidden className={styles.head}>
                    <span>{t('kv.key')}</span>
                    <span>{t('kv.value')}</span>
                    <span />
                </div>
                {value.length === 0 && <div className={styles.none}>{t('kv.empty')}</div>}
                {value.map((p, i) => {
                    const id = rowIds[i]
                    const issue = check.issues[i]
                    return (
                        <div className={cx(styles.row, leaving === id && styles.out)} key={id}>
                            <input
                                aria-invalid={!!issue}
                                aria-label={t('kv.key')}
                                autoComplete="off"
                                autoFocus={focusId === id}
                                className={cx(styles.k, issue && styles.bad)}
                                disabled={disabled}
                                onChange={(e) => patch(i, { key: e.currentTarget.value })}
                                placeholder="KEY"
                                spellCheck={false}
                                value={p.key}
                            />
                            <input
                                aria-label={t('kv.value')}
                                autoComplete="off"
                                className={styles.val}
                                disabled={disabled}
                                onChange={(e) => patch(i, { value: e.currentTarget.value })}
                                placeholder={t('kv.valuePlaceholder')}
                                spellCheck={false}
                                value={p.value}
                            />
                            <button
                                aria-label={t('kv.remove')}
                                className={styles.rm}
                                disabled={disabled}
                                onClick={() => remove(i)}
                                title={t('kv.remove')}
                                type="button"
                            >
                                <Icon name="trash" size={15} />
                            </button>
                        </div>
                    )
                })}
                <div className={styles.foot}>
                    <Button
                        disabled={disabled || full}
                        leftSection={<Icon name="plus" size={14} />}
                        onClick={add}
                        size="xs"
                        variant="default"
                    >
                        {addLabel ?? t('kv.add')}
                    </Button>
                    <span
                        aria-live="polite"
                        className={styles.msg}
                        data-state={check.valid ? 'good' : 'err'}
                    >
                        {check.problem ? t(`kv.problem.${check.problem}`) : t('kv.ok', { count: n })}
                    </span>
                    {footerExtra}
                </div>
            </div>
        </Input.Wrapper>
    )
}
