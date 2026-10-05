// SPDX-License-Identifier: AGPL-3.0-only
import { useId } from 'react'

import { cx } from '@shared/ui/kit'

import { fmtDur, useTr } from '../lib'
import styles from './role-drawer.module.css'

const R = 70
const CIRC = 2 * Math.PI * R
const DEFAULT_CAP = 2764800 // 32 days, the system maximum

const TICKS = Array.from({ length: 60 }, (_, i) => {
    const a = (i / 60) * 2 * Math.PI
    const c = Math.cos(a)
    const s = Math.sin(a)
    const major = i % 5 === 0
    const r1 = major ? 92 : 95
    return { major, x1: 88 + c * r1, x2: 88 + c * 99, y1: 88 + s * r1, y2: 88 + s * 99 }
})

/** The brass dial of the drawer preview: how much of the cap the effective lifetime takes. */
export function LifetimeRing({
    ttl,
    max,
    period,
    uses,
    bad
}: {
    bad: boolean
    max: number
    period: number
    ttl: number
    uses: string
}) {
    const { tr } = useTr()
    const gid = useId()
    const eff = period || ttl
    const cap = max || DEFAULT_CAP
    const frac = eff ? Math.min(1, Math.sqrt(eff / cap)) : 0

    const rows: Array<[string, string, boolean]> = [
        ['TTL', ttl ? fmtDur(tr, ttl) : tr('byDefault', 'по умолчанию'), bad],
        [tr('legendMax', 'Макс.'), max ? fmtDur(tr, max) : tr('none', 'нет'), false],
        [tr('legendPeriod', 'Период'), period ? fmtDur(tr, period) : tr('none', 'нет'), false],
        [tr('legendUses', 'Использ.'), !uses || Number(uses) === 0 ? tr('unlimited', 'без лимита') : uses, false]
    ]

    return (
        <div className={styles.ringbox}>
            <div className={cx(styles.bigring, bad && styles.ringBad, !!period && styles.ringPer)}>
                <svg viewBox="0 0 176 176">
                    <defs>
                        <linearGradient id={gid} x1="0" x2="1" y1="0" y2="1">
                            <stop offset="0" stopColor="#F0C772" />
                            <stop offset="1" stopColor="#D7A94B" />
                        </linearGradient>
                    </defs>
                    {TICKS.map((t, i) => (
                        <line
                            key={i}
                            stroke={t.major ? 'var(--line2)' : 'var(--dim)'}
                            strokeWidth={t.major ? 1.5 : 1}
                            x1={t.x1}
                            x2={t.x2}
                            y1={t.y1}
                            y2={t.y2}
                        />
                    ))}
                    <circle className={styles.rTr} cx="88" cy="88" r={R} />
                    <circle className={styles.rMx} cx="88" cy="88" r="82" />
                    <circle
                        className={styles.rVl}
                        cx="88"
                        cy="88"
                        r={R}
                        stroke={bad ? undefined : period ? undefined : `url(#${gid})`}
                        strokeDasharray={period ? undefined : CIRC}
                        strokeDashoffset={period ? undefined : CIRC * (1 - frac)}
                    />
                </svg>
                <div className={styles.hand} style={{ transform: `rotate(${frac * 360}deg)` }} />
                <div className={styles.core}>
                    <b>{period ? '∞' : ttl ? fmtDur(tr, ttl) : '—'}</b>
                    <small>{period ? `${tr('period', 'период')} ${fmtDur(tr, period)}` : 'TTL'}</small>
                </div>
            </div>
            <div className={styles.legend}>
                {rows.map(([k, v, isBad]) => (
                    <div key={k}>
                        <span>{k}</span>
                        <b className={isBad ? styles.legendBad : undefined}>{v}</b>
                    </div>
                ))}
            </div>
        </div>
    )
}
