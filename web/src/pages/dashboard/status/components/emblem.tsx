// SPDX-License-Identifier: AGPL-3.0-only
import { CSSProperties, useEffect, useState } from 'react'

import { prefersReducedMotion } from '@shared/ui/kit'

import styles from '../status.module.css'

const CIRC = 766.5

interface Spark {
    a: number
    r: number
    sd: number
}

function makeSparks(): Spark[] {
    return Array.from({ length: 30 }, (_, j) => ({
        a: j * 12,
        r: Math.random() * 70,
        sd: Math.random() * 0.18
    }))
}

/**
 * The seal dial: concentric rotating rings, a progress arc (share collection), a lock glyph when
 * sealed, and a one-shot burst of rings + sparks whenever `burstKey` changes.
 */
export function Emblem({ burstKey, progress }: { burstKey: number; progress: number }) {
    const [sparks, setSparks] = useState<Spark[] | null>(null)

    useEffect(() => {
        if (!burstKey || prefersReducedMotion()) return
        setSparks(makeSparks())
        const id = window.setTimeout(() => setSparks(null), 2300)
        return () => window.clearTimeout(id)
    }, [burstKey])

    const offset = CIRC * (1 - Math.max(0, Math.min(1, progress)))

    return (
        <div aria-hidden className={styles.emblem}>
            <div className={styles.sweep} />
            <svg fill="none" stroke="currentColor" viewBox="0 0 300 300">
                <circle cx="150" cy="150" r="143" stroke="var(--line2)" strokeWidth="1" />
                <g className={`${styles.spinG} ${styles.spinR}`}>
                    <circle cx="150" cy="150" opacity=".55" r="134" strokeDasharray="1.5 9.3" strokeWidth="6" />
                </g>
                <circle cx="150" cy="150" r="122" stroke="var(--line)" strokeWidth="3" />
                <circle
                    className={styles.arc}
                    cx="150"
                    cy="150"
                    r="122"
                    strokeDasharray={CIRC}
                    strokeDashoffset={offset}
                    strokeWidth="3"
                    transform="rotate(-90 150 150)"
                />
                <g className={styles.spinG}>
                    <circle cx="150" cy="150" r="104" stroke="var(--brass)" strokeDasharray="14 9" strokeWidth="2" />
                </g>
                <g className={`${styles.spinG} ${styles.spinR} ${styles.spinF}`}>
                    <circle cx="150" cy="150" r="84" strokeDasharray="2 7" strokeWidth="2" />
                </g>
                <rect height="92" stroke="var(--line2)" strokeWidth="1.5" width="92" x="104" y="104" />
                <path d="M150 78v22M150 200v22M78 150h22M200 150h22" opacity=".6" stroke="var(--bone)" strokeWidth="2" />
                <circle cx="150" cy="150" fill="currentColor" opacity=".08" r="44" stroke="none" />
                <circle className={styles.core} cx="150" cy="150" fill="currentColor" r="26" stroke="none" />
                <circle className={styles.open} cx="150" cy="150" fill="var(--void)" r="7" stroke="none" />
                <g className={styles.lock} stroke="none">
                    <rect fill="var(--void)" height="17" width="24" x="138" y="150" />
                    <path d="M142.5 150v-6a7.5 7.5 0 0115 0v6" fill="none" stroke="var(--void)" strokeWidth="3.5" />
                </g>
            </svg>
            {sparks && (
                <div className={styles.burst}>
                    {[0, 1, 2].map((i) => (
                        <i key={`r${i}`} style={{ '--bd': `${i * 0.16}s` } as CSSProperties} />
                    ))}
                    {sparks.map((s, i) => (
                        <b
                            key={`s${i}`}
                            style={{ '--a': `${s.a}deg`, '--r': `${s.r}px`, '--sd': `${s.sd}s` } as CSSProperties}
                        />
                    ))}
                </div>
            )}
        </div>
    )
}
