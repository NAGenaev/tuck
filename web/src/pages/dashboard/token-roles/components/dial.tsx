// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from 'react'

import { fmtShort, RoleView } from '../lib'
import styles from '../token-roles.module.css'

/** Small ring: how much of the role's explicit maximum its TTL takes (full + mint for periodic roles). */
export function Dial({ role, big }: { big?: boolean; role: RoleView }) {
    const [ready, setReady] = useState(false)
    useEffect(() => {
        const id = requestAnimationFrame(() => requestAnimationFrame(() => setReady(true)))
        return () => cancelAnimationFrame(id)
    }, [])

    const size = big ? 64 : 30
    const r = big ? 28 : 12
    const sw = big ? 4 : 3
    const c = 2 * Math.PI * r
    const per = role.period > 0
    const frac = per ? 1 : role.maxTtl ? Math.min(1, role.ttl / role.maxTtl) : role.ttl ? 1 : 0
    const mid = size / 2

    return (
        <svg className={big ? styles.dialBig : styles.dial} viewBox={`0 0 ${size} ${size}`}>
            <circle cx={mid} cy={mid} fill="none" r={r} stroke="var(--line2)" strokeWidth={sw} />
            <circle
                className={styles.dialV}
                cx={mid}
                cy={mid}
                fill="none"
                r={r}
                stroke={per ? 'var(--mint)' : 'var(--brass)'}
                strokeDasharray={c}
                strokeDashoffset={ready ? c * (1 - frac) : c}
                strokeWidth={sw}
            />
            {big && (
                <text fill="var(--bone)" fontFamily="JetBrains Mono, monospace" fontSize="11" fontWeight="500" textAnchor="middle" x={mid} y={mid + 4}>
                    {per ? '∞' : fmtShort(role.ttl) || '—'}
                </text>
            )}
        </svg>
    )
}
