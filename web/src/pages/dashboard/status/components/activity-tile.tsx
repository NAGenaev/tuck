// SPDX-License-Identifier: AGPL-3.0-only
import { useTranslation } from 'react-i18next'

import { CountUp, SkeletonLine, Sparkline } from '@shared/ui/kit'

import { useSamples } from '../lib'
import styles from '../status.module.css'

/** Counter tile; the sparkline is built from real polls made while the page was open. */
export function ActivityTile({
    label,
    value,
    stamp,
    loading,
    dead,
    className,
    total
}: {
    className?: string
    /** why there is no number: sealed vault or failed request */
    dead?: 'error' | 'sealed' | null
    label: string
    loading?: boolean
    /** timestamp of the last successful poll (react-query dataUpdatedAt) */
    stamp: number
    /** optional secondary count shown under the number */
    total?: number
    value: number | undefined
}) {
    const { t } = useTranslation('status')
    const samples = useSamples(dead ? undefined : value, stamp)
    const delta = samples.length > 1 && value !== undefined ? value - samples[0] : 0

    return (
        <div className={`${styles.tile} ${className ?? ''}`} data-dead={dead ? 'true' : undefined}>
            <small>{label}</small>
            <div className={styles.val} data-tone="brass">
                {dead ? (
                    '—'
                ) : loading || value === undefined ? (
                    <SkeletonLine height={30} width={110} />
                ) : (
                    <CountUp duration={1400} value={value} />
                )}
            </div>
            {!dead && value !== undefined && (
                <span className={styles.delta} data-tone={delta < 0 ? 'dn' : delta === 0 ? 'dim' : undefined}>
                    {delta === 0
                        ? samples.length > 1
                            ? t('activity.stable')
                            : total !== undefined
                              ? t('activity.total', { n: total })
                              : ''
                        : t('activity.sinceOpen', { delta: `${delta > 0 ? '▲ +' : '▼ '}${delta}` })}
                </span>
            )}
            <div className={styles.sparkBox}>
                {!dead && samples.length > 1 && <Sparkline data={samples} height={70} label={label} tone="brass" />}
            </div>
            {dead && (
                <div className={styles.nodata}>{dead === 'sealed' ? t('activity.noSealed') : t('activity.noError')}</div>
            )}
        </div>
    )
}
