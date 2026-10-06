// SPDX-License-Identifier: AGPL-3.0-only
import { useTranslation } from 'react-i18next'

import type { ClusterStatus } from '@shared/api/endpoints/cluster'
import { Sparkline } from '@shared/ui/kit'
import { Pill, Plate } from '@shared/ui/tuck/tuck'

import { certTone, commitRate, fmtMs, maxLagSeries, maxRttSeries } from '../lib'
import styles from '../cluster.module.css'

function last(a: number[]) {
    return a.length ? a[a.length - 1] : undefined
}

/**
 * Raft indexes of the answering node plus sparklines drawn from the ring of samples that the node
 * itself records. Nothing here is interpolated: each point is one real sample.
 */
export function ReplicationPanel({ status }: { status: ClusterStatus }) {
    const { i18n, t } = useTranslation('cluster')
    const fmt = (n: number | undefined) => (n === undefined ? '—' : n.toLocaleString(i18n.language))
    const history = status.history ?? []
    const span = history.length > 1 ? (history[history.length - 1].t - history[0].t) / 1000 : 0
    const rate = commitRate(history)
    const lag = maxLagSeries(history)
    const rtt = maxRttSeries(history)

    const rows: Array<{
        data: number[]
        key: string
        tone: 'brass' | 'ice' | 'ok'
        unit: string
        value: string
        wait?: string
    }> = [
        {
            data: rate,
            key: 'rate',
            tone: 'brass',
            unit: t('repl.unitRate'),
            value: last(rate) === undefined ? '—' : (last(rate) as number).toFixed(1)
        },
        {
            data: lag,
            key: 'lag',
            tone: 'ok',
            unit: t('repl.unitEntries'),
            value: last(lag) === undefined ? '—' : fmt(last(lag)),
            wait: status.is_leader ? undefined : t('repl.leaderOnly')
        },
        {
            data: rtt,
            key: 'rtt',
            tone: 'ice',
            unit: 'ms',
            value: last(rtt) === undefined ? '—' : fmtMs(last(rtt) as number),
            wait: status.is_leader ? undefined : t('repl.leaderOnly')
        }
    ]

    return (
        <Plate className={styles.panel}>
            <div className={styles.secHead} style={{ margin: '0 0 4px' }}>
                <h2 className={styles.sec}>{t('repl.title')}</h2>
                <Pill tone="mute">{status.node_id ?? '—'}</Pill>
            </div>
            <p className={styles.sub}>{t('repl.sub')}</p>
            <div className={styles.rtiles}>
                <div>
                    <small>{t('repl.term')}</small>
                    <strong className={styles.num}>{fmt(status.term)}</strong>
                </div>
                <div>
                    <small>{t('repl.commit')}</small>
                    <strong className={styles.num}>{fmt(status.commit_index)}</strong>
                </div>
                <div>
                    <small>{t('repl.applied')}</small>
                    <strong className={styles.num}>{fmt(status.applied_index)}</strong>
                </div>
                <div>
                    <small>{t('repl.lastLog')}</small>
                    <strong className={styles.num}>{fmt(status.last_log_index)}</strong>
                </div>
            </div>
            {rows.map((r) => (
                <div className={styles.sparkRow} key={r.key}>
                    <div className={styles.sparkL}>
                        <small>{t(`repl.${r.key}`)}</small>
                        <strong className={styles.num}>
                            {r.value} <em>{r.unit}</em>
                        </strong>
                    </div>
                    <div className={styles.sparkR}>
                        {r.data.length > 1 ? (
                            <Sparkline data={r.data} height={40} label={t(`repl.${r.key}`)} tone={r.tone} />
                        ) : (
                            <span className={styles.sparkWait}>{r.wait ?? t('repl.collecting')}</span>
                        )}
                    </div>
                </div>
            ))}
            <p className={styles.logNote}>
                {history.length > 1
                    ? t('repl.window', { count: history.length, secs: Math.round(span) })
                    : t('repl.collecting')}
            </p>
        </Plate>
    )
}

/** Certificate of this node on the Raft transport, or a plain-TCP warning. */
export function TlsPanel({ status }: { status: ClusterStatus }) {
    const { i18n, t } = useTranslation('cluster')
    const tls = status.tls
    const fmtDate = (iso?: string) =>
        iso ? new Date(iso).toLocaleDateString(i18n.language, { day: 'numeric', month: 'short', year: 'numeric' }) : '—'

    return (
        <Plate className={styles.panel} variant={tls ? 'default' : 'brass'}>
            <div className={styles.secHead} style={{ margin: '0 0 4px' }}>
                <h2 className={styles.sec}>{t('tls.title')}</h2>
                <Pill dot tone={tls && !tls.error ? 'ok' : 'brass'}>
                    {tls ? t('tls.pillOn') : t('tls.pillOff')}
                </Pill>
            </div>
            {!tls ? (
                <>
                    <p className={styles.sub}>{t('tls.offText')}</p>
                    <p className={styles.qwarn}>{t('tls.offWarn')}</p>
                </>
            ) : (
                <>
                    <p className={styles.sub}>{t('tls.sub', { id: status.node_id ?? '—' })}</p>
                    {tls.error ? (
                        <p className={styles.qwarn}>{tls.error}</p>
                    ) : (
                        <dl className={styles.certList}>
                            <div>
                                <dt>{t('tls.subject')}</dt>
                                <dd>{tls.subject ?? '—'}</dd>
                            </div>
                            <div>
                                <dt>{t('tls.issuer')}</dt>
                                <dd>{tls.issuer ?? '—'}</dd>
                            </div>
                            <div>
                                <dt>{t('tls.notAfter')}</dt>
                                <dd>
                                    {fmtDate(tls.not_after)}{' '}
                                    {tls.days_left !== undefined && (
                                        <Pill size="sm" tone={certTone(tls.days_left)}>
                                            {tls.days_left < 0
                                                ? t('tls.expired')
                                                : t('tls.daysLeft', { count: tls.days_left })}
                                        </Pill>
                                    )}
                                </dd>
                            </div>
                            {!!(tls.dns_names?.length || tls.ip_addresses?.length) && (
                                <div>
                                    <dt>{t('tls.san')}</dt>
                                    <dd>{[...(tls.dns_names ?? []), ...(tls.ip_addresses ?? [])].join(', ')}</dd>
                                </div>
                            )}
                            <div>
                                <dt>{t('tls.ca')}</dt>
                                <dd>{t('tls.caCount', { count: tls.ca_certs })}</dd>
                            </div>
                            {tls.server_name && (
                                <div>
                                    <dt>{t('tls.serverName')}</dt>
                                    <dd>{tls.server_name}</dd>
                                </div>
                            )}
                        </dl>
                    )}
                    <p className={styles.logNote}>{t('tls.localOnly')}</p>
                </>
            )}
        </Plate>
    )
}
