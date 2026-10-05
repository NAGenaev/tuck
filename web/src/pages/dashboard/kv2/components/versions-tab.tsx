// SPDX-License-Identifier: AGPL-3.0-only
import { useTranslation } from 'react-i18next'

import { Pill } from '@shared/ui/tuck/tuck'

import common from '../common.module.css'
import styles from '../detail.module.css'
import { fmtDate, fmtRel, VerRow, VerState } from '../utils'

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
    busyVersion
}: {
    busyVersion?: number
    current: number
    onCompare: (v: number) => void
    onDestroy: (v: number) => void
    onRead: (v: number) => void
    onSoft: (v: number) => void
    onUndelete: (v: number) => void
    rows: VerRow[]
    viewing?: number
}) {
    const { t, i18n } = useTranslation('kv')
    const list = [...rows].reverse()

    return (
        <div className={styles.tp}>
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
                                    <span className={styles.w} title={fmtRel(x.createdAt, i18n.language)}>
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
