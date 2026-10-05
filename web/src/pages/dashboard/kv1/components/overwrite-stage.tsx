// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Icon, prefersReducedMotion } from '@shared/ui/kit'

import styles from '../kv1.module.css'
import { stageMask } from '../lib'

const LIM = 260
const trunc = (s: string) => (s.length > LIM ? `${s.slice(0, LIM)}…` : s)

type Phase = 'burn' | 'done' | 'write'

/**
 * The signature moment of the screen: the old value burns away, the new one writes in.
 * Both panes show masked previews (keys visible, values as bullets) so no secret is put on screen.
 * The real PUT is fired when the write phase starts; a failure aborts the stage via `onFail`.
 */
export function OverwriteStage({
    path,
    oldText,
    newText,
    perform,
    onDone,
    onFail
}: {
    newText: string
    oldText: null | string
    onDone: () => void
    onFail: (e: unknown) => void
    path: string
    perform: () => Promise<unknown>
}) {
    const { t } = useTranslation('kv1')
    const reduced = prefersReducedMotion()
    const T = (ms: number) => (reduced ? Math.min(ms, 30) : ms)

    const oldShown = useMemo(() => trunc(oldText === null ? '••••••••••••' : stageMask(oldText)), [oldText])
    const newShown = useMemo(() => trunc(stageMask(newText)), [newText])
    const oldChars = useMemo(
        () =>
            Array.from(oldShown).map((c, i, a) => ({
                c,
                d: (reduced ? 0 : (i / a.length) * 1.3 + Math.random() * 0.15).toFixed(3),
                r: (Math.random() * 80 - 40).toFixed(0)
            })),
        [oldShown, reduced]
    )
    const newChars = useMemo(() => Array.from(newShown), [newShown])
    const newCount = useMemo(() => newChars.filter((c) => c !== '\n').length, [newChars])

    const [phase, setPhase] = useState<Phase>('burn')
    const [written, setWritten] = useState(0)
    const [settled, setSettled] = useState(false)
    const [bar, setBar] = useState(0)
    const [finished, setFinished] = useState(false)

    const oldRef = useRef<HTMLDivElement>(null)
    const canvas = useRef<HTMLCanvasElement>(null)
    const cb = useRef({ onDone, onFail, perform })
    cb.current = { onDone, onFail, perform }
    const putRef = useRef<null | Promise<unknown>>(null)

    useEffect(() => {
        const timers: number[] = []
        const intervals: number[] = []
        let alive = true
        const later = (fn: () => void, ms: number) => timers.push(window.setTimeout(fn, ms))

        // phase 1: burn
        let bi = 0
        intervals.push(
            window.setInterval(() => {
                bi += 2
                setBar(Math.min(40, bi * 0.8))
                if (bi >= 50) intervals.forEach(window.clearInterval)
            }, T(40))
        )
        if (!reduced) startEmbers(oldRef.current, canvas.current, () => alive)

        // phase 2: write
        later(() => {
            setPhase('write')
            putRef.current ??= Promise.resolve().then(() => cb.current.perform())
            const putP = putRef.current
            let k = 0
            const step = Math.max(1, Math.ceil(newCount / 60))
            const per = Math.max(16, Math.min(40, 1500 / Math.max(1, newCount / step)))
            const wt = window.setInterval(() => {
                k = Math.min(newCount, k + step)
                setWritten(k)
                setBar(40 + 60 * (k / Math.max(1, newCount)))
                if (k >= newCount) {
                    window.clearInterval(wt)
                    putP.then(
                        () => {
                            if (!alive) return
                            later(() => {
                                setSettled(true)
                                setBar(100)
                                setPhase('done')
                                setFinished(true)
                                later(() => cb.current.onDone(), T(1100))
                            }, T(350))
                        },
                        (e) => {
                            if (alive) cb.current.onFail(e)
                        }
                    )
                }
            }, T(per))
            intervals.push(wt)
        }, T(2350))

        return () => {
            alive = false
            timers.forEach(window.clearTimeout)
            intervals.forEach(window.clearInterval)
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [])

    const status =
        phase === 'burn' ? t('stageErasing') : phase === 'write' ? t('stageWriting') : `✓ ${t('stageDone')}`
    const statusColor = phase === 'burn' ? 'var(--alarm)' : 'var(--mint)'

    let idx = -1
    return (
        <div aria-live="polite" className={styles.ow}>
            <h4>
                <Icon name="bolt" size={16} />
                {t('stageTitle')} <code>{`secret/${path}`}</code>
            </h4>
            <div className={styles.owGrid}>
                <div className={`${styles.pane} ${styles.paneOld}`}>
                    <label>{t('stageOld')}</label>
                    <div className={styles.flame} data-go />
                    <div ref={oldRef}>
                        {oldChars.map((o, i) =>
                            o.c === '\n' ? (
                                <br key={i} />
                            ) : (
                                <span
                                    className={styles.ch}
                                    data-burn
                                    key={i}
                                    style={{ ['--d' as string]: `${o.d}s`, ['--r' as string]: `${o.r}deg` }}
                                >
                                    {o.c}
                                </span>
                            )
                        )}
                    </div>
                    <canvas ref={canvas} />
                </div>
                <div className={styles.arrow}>
                    <i />
                    <span>{t('stageNoVersions')}</span>
                    <i />
                </div>
                <div className={`${styles.pane} ${styles.paneNew}`} data-live={phase !== 'burn' || undefined}>
                    <label>{t('stageNew')}</label>
                    <div>
                        {newChars.map((c, n) => {
                            if (c === '\n') return <br key={n} />
                            idx += 1
                            return (
                                <span
                                    className={styles.ch}
                                    data-in={idx < written || undefined}
                                    data-settled={settled || undefined}
                                    key={n}
                                >
                                    {c}
                                </span>
                            )
                        })}
                        {phase === 'write' && !finished && <span className={styles.caret} />}
                    </div>
                </div>
            </div>
            <div className={styles.owBar}>
                <b style={{ width: `${bar}%` }} />
            </div>
            <div className={styles.owStatus} style={{ color: statusColor }}>
                {status}
            </div>
        </div>
    )
}

/** Sparks that rise from the burning characters (port of the artifact's canvas pass). */
function startEmbers(host: HTMLElement | null, cv: HTMLCanvasElement | null, alive: () => boolean) {
    const pane = cv?.parentElement
    if (!host || !cv || !pane) return
    const r = pane.getBoundingClientRect()
    const dpr = window.devicePixelRatio || 1
    cv.width = r.width * dpr
    cv.height = r.height * dpr
    const ctx = cv.getContext('2d')
    if (!ctx) return
    ctx.scale(dpr, dpr)
    const cols = ['#ffb347', '#ff7a3c', '#FF5A4D', '#F0C772']
    const q: Array<{ t: number; x: number; y: number }> = []
    host.querySelectorAll<HTMLElement>('span').forEach((s, i) => {
        if (i % 2 || !s.textContent?.trim()) return
        const b = s.getBoundingClientRect()
        q.push({
            t: parseFloat(s.style.getPropertyValue('--d')) * 1000 + 260,
            x: b.left - r.left + b.width / 2,
            y: b.top - r.top + 4
        })
    })
    q.sort((a, b) => a.t - b.t)
    let ps: Array<{ c: string; l: number; L: number; vx: number; vy: number; x: number; y: number; z: number }> = []
    const t0 = performance.now()
    const frame = (now: number) => {
        if (!alive()) return
        const el = now - t0
        while (q.length && q[0].t <= el) {
            const e = q.shift()!
            const m = 1 + (Math.random() > 0.6 ? 1 : 0)
            for (let i = 0; i < m; i++) {
                ps.push({
                    c: cols[Math.floor(Math.random() * 4)],
                    l: 0,
                    L: 600 + Math.random() * 600,
                    vx: (Math.random() - 0.5) * 0.5,
                    vy: -(0.5 + Math.random() * 1.3),
                    x: e.x,
                    y: e.y,
                    z: 1 + Math.random() * 2.2
                })
            }
        }
        ctx.clearRect(0, 0, r.width, r.height)
        ps = ps.filter((p) => {
            p.l += 16
            if (p.l > p.L) return false
            p.x += p.vx + Math.sin(p.l / 90) * 0.25
            p.y += p.vy
            p.vy *= 0.992
            ctx.globalAlpha = 1 - p.l / p.L
            ctx.fillStyle = p.c
            ctx.fillRect(p.x, p.y, p.z, p.z)
            return true
        })
        if (q.length || ps.length) requestAnimationFrame(frame)
        else ctx.clearRect(0, 0, r.width, r.height)
    }
    requestAnimationFrame(frame)
}
