// SPDX-License-Identifier: AGPL-3.0-only
import { NativeSelect } from '@mantine/core'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmModal } from '@shared/ui/kit'
import { Eyebrow, Pill, Plate } from '@shared/ui/tuck/tuck'

import { fmtAge, NodeVM } from '../lib'
import styles from '../cluster.module.css'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

/** Leadership transfer dialog: optional target voter, otherwise the most up-to-date follower. */
export function StepDownModal({
    nodes,
    onClose,
    onConfirm,
    opened
}: {
    nodes: NodeVM[]
    onClose: () => void
    onConfirm: (targetId: string) => void
    opened: boolean
}) {
    const { t } = useTranslation('cluster')
    const [target, setTarget] = useState('')
    const candidates = nodes.filter((n) => n.suffrage === 'Voter' && n.role !== 'leader')

    useEffect(() => {
        if (opened) setTarget('')
    }, [opened])

    return (
        <ConfirmModal
            confirmLabel={t('stepdown.confirm')}
            destructive={false}
            icon="refresh"
            message={
                <div className={styles.form}>
                    <p className={styles.hint}>{t('stepdown.text')}</p>
                    <div className={styles.fld}>
                        <label htmlFor="stepdown-target">{t('stepdown.target')}</label>
                        <NativeSelect
                            data={[
                                { label: t('stepdown.auto'), value: '' },
                                ...candidates.map((n) => ({
                                    label:
                                        n.metrics?.lag_entries !== undefined
                                            ? `${n.id} · ${n.address} · ${t('stepdown.lag', { count: n.metrics.lag_entries })}`
                                            : `${n.id} · ${n.address}`,
                                    value: n.id
                                }))
                            ]}
                            id="stepdown-target"
                            onChange={(e) => setTarget(e.currentTarget.value)}
                            value={target}
                        />
                    </div>
                    <div className={styles.warnbox}>{t('stepdown.warn')}</div>
                </div>
            }
            onClose={onClose}
            onConfirm={() => onConfirm(target)}
            opened={opened}
            title={t('stepdown.title')}
        />
    )
}

export interface Transition {
    from: string
    /** set when a leader other than `from` was seen */
    newLeader?: string
    since: number
}

/**
 * Live view of a leadership transfer. Every step is derived from the status polls, which run at
 * one second while this panel is visible; the elapsed time is the real wall clock.
 */
export function TransitionPanel({
    isLeader,
    leader,
    now,
    state,
    tr
}: {
    isLeader: boolean
    leader: string
    now: number
    state: string
    tr: Transition
}) {
    const { t } = useTranslation('cluster')
    const released = !isLeader
    const elected = !!tr.newLeader
    const steps: Array<{ done: boolean; key: string; params?: Record<string, string> }> = [
        { done: true, key: 'sent' },
        { done: released, key: 'released', params: { id: tr.from } },
        { done: elected, key: 'elected', params: { id: tr.newLeader ?? '…' } }
    ]
    const firstOpen = steps.findIndex((s) => !s.done)
    return (
        <Plate className={cx(styles.trans, elected && styles.transDone)} variant={elected ? 'default' : 'brass'}>
            <div className={styles.transHead}>
                <Eyebrow className={styles.eb0}>{t('stepdown.liveEyebrow')}</Eyebrow>
                <Pill dot tone={elected ? 'ok' : 'brass'}>
                    {elected ? t('stepdown.pillDone') : t('stepdown.pillLive')}
                </Pill>
                <span className={styles.transClock}>{fmtAge(now - tr.since)}</span>
            </div>
            <ol className={styles.transSteps}>
                {steps.map((s, i) => (
                    <li className={cx(s.done && styles.done, i === firstOpen && styles.cur)} key={s.key}>
                        <i>{s.done ? '✓' : i + 1}</i>
                        <span>{t(`stepdown.step.${s.key}`, s.params)}</span>
                    </li>
                ))}
            </ol>
            <p className={styles.transNote}>{t('stepdown.liveNote', { leader: leader || '—', state: state || '—' })}</p>
        </Plate>
    )
}
