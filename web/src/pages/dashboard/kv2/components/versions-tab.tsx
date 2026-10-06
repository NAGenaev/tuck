// SPDX-License-Identifier: AGPL-3.0-only
import { useTranslation } from 'react-i18next'

import { Icon } from '@shared/ui/kit'
import { Pill } from '@shared/ui/tuck/tuck'

import common from '../common.module.css'
import styles from '../detail.module.css'
import { authorLabel, fmtDate, fmtRel, VerRow, VerState } from '../utils'

export function StatePill({ state }: { state: VerState }) {
    const { t } = useTranslation('kv')
    if (state === 'active') return <Pill tone="ok">{t('kv2.state.active')}</Pill>
    if (state === 'deleted') return <Pill tone="brass">{t('kv2.state.deleted')}</Pill>
    return <Pill tone="bad">{t('kv2.state.destroyed')}</Pill>
}

export function VersionsTab({
    rows,
    current,
    viewing,
    onRead,
    onCompare,
    onSoft,
    onUndelete,
    onDestroy,
    busyVersion,
    oldestVersion,
    maxVersions
}: {
    busyVersion?: number
    current: number
    maxVersions?: number
    onCompare: (v: number) => void
    onDestroy: (v: number) => void
    onRead: (v: number) => void
    onSoft: (v: number) => void
    onUndelete: (v: number) => void
    /** lowest version still stored (server value); versions below it were evicted by max_versions */
    oldestVersion?: number
    rows: VerRow[]
    viewing?: number
}) {
    const { t, i18n } = useTranslation('kv')
    const list = [...rows].reverse()
    const evicted = oldestVersion !== undefined && oldestVersion > 1 ? oldestVersion - 1 : 0

    return (
        <div className={styles.tp}>
            {evicted > 0 && (
                <div className={common.banner} data-tone="b">
                    <Icon name="info" size={20} />
                    <h5>{t('kv2.ver.evictedTitle')}</h5>
                    <p>
                        {t('kv2.ver.evictedText', {
                            first: 1,
                            last: evicted,
                            max: maxVersions ? maxVersions : '∞'
                        })}
                    </p>
                </div>
            )}
            <div className={styles.tl}>
                {list.map((x, i) => {
                    const isCur = x.version === current
                    const busy = busyVersion === x.version
                    return (
                        <div
                            className={styles.tv}
                            data-cur={isCur || undefined}
                            data-st={x.state}
                            data-view={viewing === x.version || undefined}
                            key={x.version}
                            style={{ ['--i' as string]: Math.min(i, 12) }}
                        >
                            <span className={styles.nd} />
                            <div className={styles.tcard}>
                                <div className={styles.tc1}>
                                    <span className={common.ver} data-cur={isCur || undefined}>
                                        v{x.version}
                                    </span>
                                    {isCur && <Pill tone="brass">{t('kv2.state.current')}</Pill>}
                                    <StatePill state={x.state} />
                                    <span
                                        className={styles.w}
                                        title={[
                                            fmtRel(x.createdAt, i18n.language),
                                            x.author?.accessor && `accessor ${x.author.accessor}`,
                                            x.author?.fingerprint && `fingerprint ${x.author.fingerprint}`
                                        ]
                                            .filter(Boolean)
                                            .join(' · ')}
                                    >
                                        <b className={styles.by} data-none={!authorLabel(x.author) || undefined}>
                                            {authorLabel(x.author) || '—'}
                                        </b>
                                        {' · '}
                                        {fmtDate(x.createdAt, i18n.language)}
                                    </span>
                                </div>
                                <div className={styles.tact}>
                                    {x.state !== 'destroyed' && (
                                        <button className={styles.lnk} data-t="i" onClick={() => onRead(x.version)} type="button">
                                            {t('kv2.ver.read')}
                                        </button>
                                    )}
                                    {x.state === 'active' && !isCur && (
                                        <button className={styles.lnk} data-t="i" onClick={() => onCompare(x.version)} type="button">
                                            {t('kv2.ver.compare')}
                                        </button>
                                    )}
                                    {x.state === 'active' && (
                                        <button className={styles.lnk} data-t="y" disabled={busy} onClick={() => onSoft(x.version)} type="button">
                                            {t('kv2.ver.delete')}
                                        </button>
                                    )}
                                    {x.state === 'deleted' && (
                                        <button className={styles.lnk} data-t="g" disabled={busy} onClick={() => onUndelete(x.version)} type="button">
                                            {t('kv2.ver.undelete')}
                                        </button>
                                    )}
                                    {x.state !== 'destroyed' ? (
                                        <button className={styles.lnk} data-t="r" disabled={busy} onClick={() => onDestroy(x.version)} type="button">
                                            {t('kv2.ver.destroy')}
                                        </button>
                                    ) : (
                                        <span className={styles.gone}>{t('kv2.ver.gone')}</span>
                                    )}
                                </div>
                            </div>
                        </div>
                    )
                })}
            </div>
        </div>
    )
}
