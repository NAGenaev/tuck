// SPDX-License-Identifier: AGPL-3.0-only
import { CSSProperties } from 'react'

import { Particle, RING } from '../lib'
import styles from '../wrapping.module.css'

export type StageState = 'breaking' | 'dissolved' | 'forbidden' | 'idle' | 'low' | 'sealed' | 'sealing' | 'void'

const GRAD = 'tuck-wrapping-waxg'
const LINES = [
    [112, 188],
    [120, 180],
    [104, 196],
    [126, 174]
]

/** The capsule: an envelope with a wax seal, a countdown ring and a state machine in CSS. */
export function Stage({
    label,
    offset,
    particles,
    shake,
    state,
    tag,
    time,
    timeSub
}: {
    label: string
    /** 0..RING, how much of the ring has burnt away */
    offset: number
    particles: Particle[]
    shake: number
    state: StageState
    tag: string
    time: string
    timeSub: string
}) {
    return (
        <div
            aria-live="polite"
            className={`${styles.stage} ${shake ? styles.stageShake : ''}`}
            data-s={state}
            key={`s${shake}`}
        >
            <div className={styles.stageTag}>{tag}</div>
            <svg aria-hidden className={styles.cap} viewBox="0 0 300 250">
                <defs>
                    <radialGradient cx="35%" cy="30%" id={GRAD} r="80%">
                        <stop offset="0" stopColor="#F0C772" />
                        <stop offset=".6" stopColor="#D7A94B" />
                        <stop offset="1" stopColor="#9a7424" />
                    </radialGradient>
                </defs>
                <g className={styles.lines}>
                    {LINES.map(([a, b], i) => (
                        <g key={i} style={{ '--d': i } as CSSProperties}>
                            <line x1={a} x2={b} y1="22" y2="22" />
                        </g>
                    ))}
                </g>
                <rect className={styles.envBody} height="150" width="260" x="20" y="62" />
                <path className={styles.envFold} d="M20 212l95-72M280 212l-95-72" />
                <path className={styles.envFlap} d="M20 62h260l-130 76z" />
                <circle className={styles.ticks} cx="150" cy="138" r="56" />
                <circle className={styles.ringBg} cx="150" cy="138" r="46" />
                <circle
                    className={styles.ring}
                    cx="150"
                    cy="138"
                    r="46"
                    strokeDasharray={RING}
                    strokeDashoffset={offset}
                />
                <g className={styles.wax}>
                    <circle className={styles.waxDisc} cx="150" cy="138" fill={`url(#${GRAD})`} r="32" />
                    <circle
                        cx="150"
                        cy="138"
                        fill="none"
                        r="24"
                        stroke="#8a6a24"
                        strokeDasharray="3 3"
                        strokeWidth="1"
                    />
                    <rect className={styles.waxIco} height="13" width="18" x="141" y="136" />
                    <path className={styles.waxIco} d="M144.5 136v-4a5.5 5.5 0 0111 0v4" />
                    <path className={styles.crack} d="M150 106l-6 14 9 8-7 12 8 8-4 12" />
                </g>
                <text className={styles.time} x="150" y="206">
                    {time}
                </text>
                <text className={styles.timeS} x="150" y="232">
                    {timeSub}
                </text>
            </svg>
            {particles.map((p) => (
                <i
                    className={styles.part}
                    key={p.id}
                    style={
                        {
                            '--r': `${p.r}deg`,
                            '--x': `${p.x}px`,
                            '--y': `${p.y}px`,
                            animationDelay: `${p.delay}ms`,
                            background: p.mint ? 'var(--mint)' : 'var(--brass)',
                            left: `calc(50% + ${p.left}px)`,
                            top: p.top
                        } as CSSProperties
                    }
                />
            ))}
            <div className={styles.stageSt}>{label}</div>
        </div>
    )
}
