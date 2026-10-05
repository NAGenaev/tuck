// SPDX-License-Identifier: AGPL-3.0-only
import { KeyboardEvent, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { prefersReducedMotion } from '@shared/ui/kit'

import { ClusterModel, NodeVM, ringPos } from '../lib'
import styles from '../cluster.module.css'

const COLOR = { follower: '#49E0A4', leader: '#D7A94B', nonvoter: '#45C9E6' } as const

/** Eases a number toward `to` (ease-out cubic); instant with reduced motion. */
function useTween(to: number) {
    const [v, setV] = useState(prefersReducedMotion() ? to : 0)
    const from = useRef(v)
    useEffect(() => {
        if (prefersReducedMotion()) {
            from.current = to
            setV(to)
            return
        }
        const t0 = performance.now()
        const a = from.current
        let raf = 0
        const step = (now: number) => {
            const p = Math.min(1, (now - t0) / 700)
            const val = a + (to - a) * (1 - Math.pow(1 - p, 3))
            from.current = val
            setV(Math.round(val))
            if (p < 1) raf = requestAnimationFrame(step)
        }
        raf = requestAnimationFrame(step)
        return () => cancelAnimationFrame(raf)
    }, [to])
    return v
}

export function Topology({
    model,
    onOpen,
    openId,
    state
}: {
    model: ClusterModel
    onOpen: (id: string) => void
    openId: null | string
    state: string
}) {
    const { t } = useTranslation('cluster')
    const nodes = model.nodes
    const n = nodes.length
    const voters = useTween(model.voters)
    const pos = nodes.map((_, i) => ringPos(i, n))
    const leaderIdx = nodes.findIndex((x) => x.role === 'leader')
    const reduce = prefersReducedMotion()

    const onKey = (e: KeyboardEvent, id: string) => {
        if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onOpen(id)
        }
    }

    const label = (nd: NodeVM) =>
        nd.role === 'leader' ? t('role.leader') : nd.role === 'nonvoter' ? t('role.nonvoter') : t('role.follower')

    return (
        <svg aria-label={t('topo.aria')} role="img" viewBox="0 0 760 440">
            <defs>
                <radialGradient cx="50%" cy="50%" id="cl-cg" r="50%">
                    <stop offset="0" stopColor="#D7A94B" stopOpacity=".12" />
                    <stop offset="1" stopColor="#D7A94B" stopOpacity="0" />
                </radialGradient>
            </defs>
            <ellipse cx="380" cy="215" fill="url(#cl-cg)" rx="230" ry="150" />
            <ellipse className={styles.dial} cx="380" cy="215" rx="270" ry="140" />
            <ellipse className={styles.dial} cx="380" cy="215" rx="170" ry="90" style={{ opacity: 0.6 }} />
            <g className={styles.sweep}>
                <line stroke="rgba(215,169,75,.22)" x1="380" x2="380" y1="215" y2="60" />
            </g>

            <g>
                {nodes.map((a, i) =>
                    nodes.slice(i + 1).map((b, k) => {
                        const j = i + 1 + k
                        const lead = a.role === 'leader' || b.role === 'leader'
                        return (
                            <line
                                className={`${styles.edge} ${lead ? styles.live : ''}`}
                                key={`${a.id}-${b.id}`}
                                x1={pos[i].x}
                                x2={pos[j].x}
                                y1={pos[i].y}
                                y2={pos[j].y}
                            />
                        )
                    })
                )}
            </g>

            {/* decorative heartbeat: leader -> followers */}
            {!reduce && leaderIdx >= 0 && (
                <g>
                    {nodes.map((nd, i) =>
                        i === leaderIdx ? null : (
                            <circle
                                fill="#F0C772"
                                key={nd.id}
                                r="3.5"
                                style={{ filter: 'drop-shadow(0 0 5px #F0C772)' }}
                            >
                                <animate
                                    attributeName="cx"
                                    begin={`${(i % 4) * 0.15}s`}
                                    calcMode="spline"
                                    dur="1.4s"
                                    keySplines=".45 0 .55 1"
                                    keyTimes="0;1"
                                    repeatCount="indefinite"
                                    values={`${pos[leaderIdx].x};${pos[i].x}`}
                                />
                                <animate
                                    attributeName="cy"
                                    begin={`${(i % 4) * 0.15}s`}
                                    calcMode="spline"
                                    dur="1.4s"
                                    keySplines=".45 0 .55 1"
                                    keyTimes="0;1"
                                    repeatCount="indefinite"
                                    values={`${pos[leaderIdx].y};${pos[i].y}`}
                                />
                                <animate
                                    attributeName="opacity"
                                    begin={`${(i % 4) * 0.15}s`}
                                    dur="1.4s"
                                    keyTimes="0;.1;.85;1"
                                    repeatCount="indefinite"
                                    values="0;1;1;0"
                                />
                            </circle>
                        )
                    )}
                </g>
            )}

            <g textAnchor="middle">
                <text className={styles.cntLbl} x="380" y="176">
                    {t('topo.voters')}
                </text>
                <text className={styles.cntBig} x="380" y="208">
                    {voters}
                </text>
                <text className={styles.cntLbl} x="380" y="238">
                    {t('topo.quorum')}
                </text>
                <text className={styles.cntSmall} x="380" y="260">
                    {`≥ ${model.quorum}`}
                </text>
                <text className={styles.cntSmall} fill="#575C70" x="380" y="282">
                    {state || '—'}
                </text>
            </g>

            <g>
                {nodes.map((nd, i) => {
                    const p = pos[i]
                    const c = COLOR[nd.role]
                    const lead = nd.role === 'leader'
                    return (
                        <g
                            aria-label={`${nd.id}, ${label(nd)}`}
                            className={`${styles.nd} ${lead ? styles.leader : ''} ${openId === nd.id ? styles.sel : ''}`}
                            key={nd.id}
                            onClick={() => onOpen(nd.id)}
                            onKeyDown={(e) => onKey(e, nd.id)}
                            role="button"
                            tabIndex={0}
                            transform={`translate(${p.x - 84},${p.y - 42})`}
                        >
                            {lead && (
                                <rect
                                    className={styles.ring}
                                    fill="none"
                                    height="94"
                                    stroke="#D7A94B"
                                    strokeOpacity=".6"
                                    width="178"
                                    x="-5"
                                    y="-5"
                                />
                            )}
                            <rect className={styles.box} height="84" width="168" />
                            <circle cx="16" cy="19" fill={c} r="4">
                                {!reduce && (
                                    <animate
                                        attributeName="opacity"
                                        dur="2s"
                                        repeatCount="indefinite"
                                        values="1;.35;1"
                                    />
                                )}
                            </circle>
                            <text className={styles.nm} x="28" y="24">
                                {nd.id.length > 16 ? `${nd.id.slice(0, 15)}…` : nd.id}
                            </text>
                            <text className={styles.rl} fill={c} x="14" y="46">
                                {label(nd).toUpperCase()}
                            </text>
                            <text className={styles.ad} x="14" y="62">
                                {nd.address.length > 24 ? `${nd.address.slice(0, 23)}…` : nd.address}
                            </text>
                            <rect fill="#272B38" height="4" width="100" x="14" y="71" />
                            <rect fill={c} height="4" width={nd.role === 'nonvoter' ? 30 : 100} x="14" y="71" />
                            <text className={styles.ad} x="120" y="76">
                                {nd.suffrage.toLowerCase()}
                            </text>
                            {lead && (
                                <g className={styles.crown} transform="translate(60,-27)">
                                    <path
                                        d="M0 20L4 5L15 13L24 0L33 13L44 5L48 20Z"
                                        fill="#D7A94B"
                                        stroke="#F0C772"
                                        strokeWidth="1"
                                    />
                                    <rect fill="#D7A94B" height="3" width="48" y="22" />
                                </g>
                            )}
                        </g>
                    )
                })}
            </g>
        </svg>
    )
}
