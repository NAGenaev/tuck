// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { CopyButton, Drawer, MetaGrid } from '@shared/ui/kit'
import { Pill } from '@shared/ui/tuck/tuck'

import { NodeVM } from '../lib'
import styles from '../cluster.module.css'

export function RolePill({ role }: { role: NodeVM['role'] }) {
    const { t } = useTranslation('cluster')
    if (role === 'leader') return <Pill tone="brass">♛ {t('role.leader')}</Pill>
    if (role === 'nonvoter') return <Pill tone="mute">{t('role.nonvoter')}</Pill>
    return <Pill tone="ice">{t('role.follower')}</Pill>
}

export function NodeDrawer({
    node,
    onClose,
    onRemove
}: {
    node: NodeVM | null
    onClose: () => void
    onRemove: (id: string) => void
}) {
    const { t } = useTranslation('cluster')
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
                    <p className={styles.dNote}>
                        {node.role === 'leader' ? t('drawer.leaderNote') : t('drawer.followerNote')}
                    </p>
                </div>
            )}
        </Drawer>
    )
}
