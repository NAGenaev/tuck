// SPDX-License-Identifier: AGPL-3.0-only
import { Chip, Input, NumberInput, SegmentedControl } from '@mantine/core'
import { ReactNode, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cx } from '../cx'
import { Icon } from '../icons'
import styles from './ttl-field.module.css'

const UNITS = [86400, 3600, 60, 1] as const
const UNIT_KEY: Record<number, string> = { 1: 'second', 60: 'minute', 3600: 'hour', 86400: 'day' }

export const TTL_DEFAULT_MAX = 2592000 // 30 days
export const TTL_DEFAULT_PRESETS = [900, 3600, 28800, 86400, 2592000]

/** Largest unit that divides `seconds` evenly (minutes for 0, so the field starts sensibly). */
const bestUnit = (seconds: number) => (seconds > 0 ? UNITS.find((u) => seconds % u === 0) : 60) ?? 1

/**
 * TTL input. The value is always a number of seconds; the user types in sec / min / h / d, taps a
 * preset chip, and sees the humanized result ("1 hour 30 minutes"), the exact seconds, the expiry
 * moment and a bar against `max`. Values over `max` are flagged (the server clamps them).
 */
export function TtlField({
    value,
    onChange,
    max = TTL_DEFAULT_MAX,
    presets = TTL_DEFAULT_PRESETS,
    label,
    description,
    disabled,
    className
}: {
    className?: string
    description?: ReactNode
    disabled?: boolean
    label?: ReactNode
    /** seconds, the policy / role ceiling (default 30 days) */
    max?: number
    onChange: (seconds: number) => void
    /** quick-pick values in seconds */
    presets?: number[]
    /** seconds */
    value: number
}) {
    const { t, i18n } = useTranslation('forms')
    const [unit, setUnit] = useState<number>(() => bestUnit(value))
    const [raw, setRaw] = useState<'' | number>(() => value / bestUnit(value))

    // follow outside changes (reset, loaded data) without fighting the user's typing
    useEffect(() => {
        if ((raw === '' ? 0 : raw) * unit !== value) {
            const u = value % unit === 0 ? unit : bestUnit(value)
            setUnit(u)
            setRaw(value / u)
        }
    }, [value, raw, unit])

    const human = (s: number) => {
        if (s <= 0) return t('ttl.human.zero')
        const parts: string[] = []
        let rest = s
        for (const u of UNITS) {
            const n = Math.floor(rest / u)
            if (n > 0 && parts.length < 2) parts.push(t(`ttl.human.${UNIT_KEY[u]}`, { count: n }))
            rest -= n * u
        }
        return parts.join(' ')
    }

    const over = value > max
    const until =
        value > 0
            ? new Date(Date.now() + value * 1000).toLocaleString(i18n.language, {
                  day: '2-digit',
                  hour: '2-digit',
                  minute: '2-digit',
                  month: 'short'
              })
            : ''

    const setNumber = (n: number | string) => {
        const num = typeof n === 'number' ? n : n === '' ? '' : Number(n) || 0
        setRaw(num)
        onChange((num === '' ? 0 : num) * unit)
    }

    const pickUnit = (u: string) => {
        const next = Number(u)
        setUnit(next)
        onChange((raw === '' ? 0 : raw) * next)
    }

    const pickPreset = (s: number) => {
        const u = bestUnit(s)
        setUnit(u)
        setRaw(s / u)
        onChange(s)
    }

    return (
        <Input.Wrapper className={className} description={description} label={label}>
            <div className={styles.ttl}>
                <div className={styles.row}>
                    <NumberInput
                        allowDecimal={false}
                        allowNegative={false}
                        aria-label={t('ttl.amount')}
                        className={styles.num}
                        disabled={disabled}
                        hideControls={false}
                        min={0}
                        onChange={setNumber}
                        value={raw}
                    />
                    <SegmentedControl
                        aria-label={t('ttl.unit.label')}
                        data={UNITS.slice()
                            .reverse()
                            .map((u) => ({ label: t(`ttl.unit.${UNIT_KEY[u]}`), value: String(u) }))}
                        disabled={disabled}
                        onChange={pickUnit}
                        size="sm"
                        value={String(unit)}
                    />
                </div>
                <div className={styles.presets} role="group">
                    {presets.map((s) => (
                        <Chip
                            checked={value === s}
                            disabled={disabled}
                            key={s}
                            onChange={() => pickPreset(s)}
                            size="sm"
                        >
                            {human(s)}
                        </Chip>
                    ))}
                </div>
                <div className={cx(styles.out)} data-over={over || undefined}>
                    <b className={styles.human}>{human(value)}</b>
                    <code className={styles.secs}>{value}s</code>
                    {until && <span className={styles.until}>{t('ttl.until', { date: until })}</span>}
                </div>
                <div aria-hidden className={styles.track}>
                    <i style={{ width: `${Math.min(100, (value / max) * 100)}%` }} />
                </div>
                <div className={styles.hint} data-err={over || undefined}>
                    {over && <Icon name="alert" size={13} />}
                    {over ? t('ttl.over', { max: human(max) }) : t('ttl.max', { max: human(max) })}
                </div>
            </div>
        </Input.Wrapper>
    )
}
