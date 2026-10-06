// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { ClusterSample } from '@shared/api/endpoints/cluster'
import { CopyButton, Drawer, MetaGrid, Sparkline } from '@shared/ui/kit'
import { Pill } from '@shared/ui/tuck/tuck'

import { fmtAge, fmtMs, NodeVM, peerLagSeries, peerRttSeries } from '../lib'
import styles from '../cluster.module.css'

export function RolePill({ role }: { role: NodeVM['role'] }) {
    const { t } = useTranslation('cluster')
    if (role === 'leader') return <Pill tone="brass">♛ {t('role.leader')}</Pill>
    if (role === 'nonvoter') return <Pill tone="mute">{t('role.nonvoter')}</Pill>
    return <Pill tone="ice">{t('role.follower')}</Pill>
}

export function NodeDrawer({
    history,
    node,
    onClose,
    onRemove
}: {
    history: ClusterSample[]
    node: NodeVM | null
    onClose: () => void
    onRemove: (id: string) => void
}) {
    const { i18n, t } = useTranslation('cluster')
    const m = node?.metrics
    const num = (n: number | undefined) => (n === undefined ? '—' : n.toLocaleString(i18n.language))
    const rtt = node ? peerRttSeries(history, node.id) : []
    const lag = node ? peerLagSeries(history, node.id) : []
    return (
        <Drawer
            footer={
                node && (
                    <Button color="red" onClick={() => onRemove(node.id)} variant="outline">
                        {t('drawer.remove')}
                    </Button>
                )
            }
            icon="gear"
            onClose={onClose}
            opened={node !== null}
            title={node?.id ?? ''}
        >
            {node && (
                <div className={styles.dBody}>
                    <div className={styles.dPills}>
                        <RolePill role={node.role} />
                        {node.isMe && <Pill tone="ok">{t('drawer.thisNode')}</Pill>}
                    </div>
                    <MetaGrid
                        items={[
                            [t('table.id'), <b key="id">{node.id}</b>],
                            [
                                t('table.addr'),
                                <span className={styles.dAddr} key="addr">
                                    {node.address}
                                    <CopyButton size={26} value={node.address} />
                                </span>
                            ],
                            [t('table.suffrage'), node.suffrage],
                            [t('table.role'), t(`role.${node.role}`)]
                        ]}
                    />
                    <h3 className={styles.dSub}>{t('drawer.repl')}</h3>
                    {m ? (
                        <MetaGrid
                            items={[
                                [t('drawer.source'), m.source === 'local' ? t('drawer.srcLocal') : t('drawer.srcHeartbeat')],
                                [t('repl.term'), num(m.term)],
                                [t('table.lastLog'), num(m.last_log_index)],
                                ...(m.commit_index !== undefined ? [[t('repl.commit'), num(m.commit_index)] as [string, string]] : []),
                                ...(m.applied_index !== undefined ? [[t('repl.applied'), num(m.applied_index)] as [string, string]] : []),
                                ...(m.lag_entries !== undefined ? [[t('table.lag'), num(m.lag_entries)] as [string, string]] : []),
                                ...(m.last_contact_ms !== undefined
                                    ? [[t('table.contact'), fmtAge(m.last_contact_ms)] as [string, string]]
                                    : []),
                                ...(m.heartbeat_rtt_ms !== undefined
                                    ? [[t('table.rtt'), `${fmtMs(m.heartbeat_rtt_ms)} ms`] as [string, string]]
                                    : []),
                                ...(m.reachable !== undefined
                                    ? [[t('drawer.reachable'), m.reachable ? t('drawer.yes') : t('drawer.no')] as [string, string]]
                                    : [])
                            ]}
                        />
                    ) : (
                        <p className={styles.dNote}>{t('drawer.noMetrics')}</p>
                    )}
                    {(rtt.length > 1 || lag.length > 1) && (
                        <div className={styles.dSpark}>
                            {rtt.length > 1 && (
                                <div>
                                    <small>{t('drawer.rttHist')}</small>
                                    <Sparkline data={rtt} height={34} label={t('drawer.rttHist')} tone="ice" />
                                </div>
                            )}
                            {lag.length > 1 && (
                                <div>
                                    <small>{t('drawer.lagHist')}</small>
                                    <Sparkline data={lag} height={34} label={t('drawer.lagHist')} tone="ok" />
                                </div>
                            )}
                        </div>
                    )}
                    <p className={styles.dNote}>
                        {node.role === 'leader' ? t('drawer.leaderNote') : t('drawer.followerNote')}
                    </p>
                </div>
            )}
        </Drawer>
    )
}
