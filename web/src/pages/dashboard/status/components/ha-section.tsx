// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'

import { getClusterStatus } from '@shared/api/endpoints/cluster'
import { SkeletonLine } from '@shared/ui/kit'
import { Eyebrow } from '@shared/ui/tuck/tuck'

import styles from '../status.module.css'

/** HA block: real Raft membership from GET /v1/sys/cluster (only when the server reports HA). */
export function HaSection({
    haEnabled,
    sealed,
    healthLoaded
}: {
    haEnabled: boolean | undefined
    healthLoaded: boolean
    sealed: boolean
}) {
    const { t } = useTranslation('status')
    const queryEnabled = haEnabled === true && !sealed

    const {
        data: cluster,
        error,
        isLoading
    } = useQuery({
        queryKey: ['sys', 'cluster'],
        queryFn: getClusterStatus,
        enabled: queryEnabled,
        refetchInterval: 10_000,
        retry: false
    })

    const servers = cluster?.servers ?? []
    const forbidden = isAxiosError(error) && error.response?.status === 403

    let note: string
    if (!healthLoaded) note = ''
    else if (!haEnabled) note = t('ha.single')
    else if (sealed) note = t('ha.sealedNote')
    else if (cluster) note = `${t('ha.quorum', { n: servers.length })}\n${t('ha.state', { state: cluster.state })}`
    else note = t('ha.unknown')

    const isLeader = (id: string, address: string) =>
        (!!cluster?.leader && cluster.leader === id) || (!!cluster?.leader_addr && cluster.leader_addr === address)

    return (
        <section className={`${styles.sec} ${styles.rv}`} style={{ '--d': 5 } as CSSProperties}>
            <div className={styles.sech}>
                <Eyebrow>{t('ha.eyebrow')}</Eyebrow>
                <em>{t('ha.tag')}</em>
            </div>
            <div className={styles.raft}>
                <div className={styles.raftH}>
                    <small>{t('ha.label')}</small>
                    {!healthLoaded ? (
                        <SkeletonLine height={22} width={120} />
                    ) : (
                        <div className={styles.val} data-tone={haEnabled ? 'ok' : undefined} style={{ fontSize: 22 }}>
                            {haEnabled ? t('ha.enabled') : t('ha.disabled')}
                        </div>
                    )}
                    <p style={{ whiteSpace: 'pre-line' }}>{note}</p>
                </div>
                <div className={styles.peers}>
                    {queryEnabled && isLoading && (
                        <>
                            {[0, 1, 2].map((i) => (
                                <div className={styles.peer} key={i}>
                                    <SkeletonLine height={14} width="60%" />
                                    <SkeletonLine width="80%" />
                                </div>
                            ))}
                        </>
                    )}
                    {servers.map((s) => {
                        const leader = isLeader(s.id, s.address)
                        return (
                            <div className={styles.peer} data-role={leader ? 'leader' : 'follower'} key={s.id}>
                                <div className={styles.nm}>
                                    <span className={styles.pd} />
                                    <span className={styles.nmText} title={s.id}>
                                        {s.id}
                                    </span>
                                    <span className={styles.role}>
                                        {leader
                                            ? t('ha.leader')
                                            : s.suffrage && s.suffrage.toLowerCase() === 'nonvoter'
                                              ? t('ha.nonvoter')
                                              : t('ha.follower')}
                                    </span>
                                </div>
                                <div className={styles.mt}>
                                    <b>{s.address}</b>
                                    {s.suffrage ? ` · ${s.suffrage.toLowerCase()}` : ''}
                                </div>
                                <div className={styles.lag}>
                                    <i />
                                </div>
                            </div>
                        )
                    })}
                    {queryEnabled && error && (
                        <div className={styles.peersState}>
                            {forbidden ? t('ha.peerForbidden') : t('ha.peerFailed')}
                        </div>
                    )}
                    {healthLoaded && !queryEnabled && (
                        <div className={styles.peersState}>{!haEnabled ? t('ha.single') : t('ha.sealedNote')}</div>
                    )}
                </div>
            </div>
        </section>
    )
}
