// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Pill } from '@shared/ui/tuck/tuck'

import { NodeVM, simulateLoss } from '../lib'
import styles from '../cluster.module.css'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

/**
 * Read-only "what if node X goes down" calculator. It does pure arithmetic over the real voter
 * list of the last status poll. It never calls the cluster and never changes anything.
 */
export function QuorumCalc({ nodes }: { nodes: NodeVM[] }) {
    const { t } = useTranslation('cluster')
    const voters = useMemo(() => nodes.filter((n) => n.suffrage !== 'Nonvoter'), [nodes])
    const [down, setDown] = useState<Set<string>>(new Set())

    // forget nodes that left the configuration
    useEffect(() => {
        setDown((d) => {
            const ids = new Set(voters.map((v) => v.id))
            const next = new Set([...d].filter((id) => ids.has(id)))
            return next.size === d.size ? d : next
        })
    }, [voters])

    if (voters.length === 0) return null
    const res = simulateLoss(nodes, down)
    const toggle = (id: string) =>
        setDown((d) => {
            const next = new Set(d)
            if (next.has(id)) next.delete(id)
            else next.add(id)
            return next
        })

    return (
        <div className={styles.calc}>
            <div className={styles.calcHead}>
                <b>{t('calc.title')}</b>
                <Pill size="sm" tone="mute">
                    {t('calc.badge')}
                </Pill>
            </div>
            <p className={styles.calcSub}>{t('calc.sub')}</p>
            <div className={styles.calcChips} role="group">
                {voters.map((n) => (
                    <button
                        aria-pressed={down.has(n.id)}
                        className={cx(styles.calcChip, down.has(n.id) && styles.off)}
                        key={n.id}
                        onClick={() => toggle(n.id)}
                        type="button"
                    >
                        <i />
                        {n.id}
                    </button>
                ))}
            </div>
            <div className={cx(styles.calcRes, res.hasQuorum ? styles.ok : styles.bad)} role="status">
                <b>{res.hasQuorum ? t('calc.okTitle') : t('calc.badTitle')}</b>
                <span>
                    {t('calc.math', { alive: res.alive, quorum: res.quorum, total: res.total })}{' '}
                    {res.hasQuorum
                        ? res.spare > 0
                            ? t('calc.spare', { count: res.spare })
                            : t('calc.noSpare')
                        : t('calc.stopped')}
                </span>
            </div>
            {down.size > 0 && (
                <button className={styles.calcReset} onClick={() => setDown(new Set())} type="button">
                    {t('calc.reset')}
                </button>
            )}
        </div>
    )
}
