// SPDX-License-Identifier: AGPL-3.0-only
import { useLayoutEffect, useRef } from 'react'

import { prefersReducedMotion } from '@shared/ui/kit'

import styles from '../detail.module.css'
import { maskText } from '../utils'

const CH = 'ABCDEFGHJKLMNPQRSTUVWXYZ0123456789#$%&*+=<>?@~'

/**
 * A secret value that is masked by default. Revealing it decodes it with a short scramble
 * (instant with reduced motion, or when it is already revealed on mount).
 * The text is written straight into the node so the animation does not re-render React.
 */
export function SecretText({ text, shown, className }: { className?: string; shown: boolean; text: string }) {
    const ref = useRef<HTMLSpanElement>(null)
    const first = useRef(true)
    const raf = useRef(0)
    const settle = useRef(0)

    useLayoutEffect(() => {
        const el = ref.current
        if (!el) return
        cancelAnimationFrame(raf.current)
        window.clearTimeout(settle.current)
        const wasFirst = first.current
        first.current = false

        if (!shown) {
            el.textContent = maskText(text.length)
            el.dataset.s = 'masked'
            return
        }
        if (wasFirst || prefersReducedMotion() || text.length === 0) {
            el.textContent = text
            el.dataset.s = 'plain'
            return
        }

        const n = text.length
        const dur = Math.min(1200, 450 + n * 22)
        const t0 = performance.now()
        el.dataset.s = 'dec'
        const frame = (now: number) => {
            const k = Math.min(1, (now - t0) / dur)
            const fixed = Math.floor(Math.pow(k, 1.4) * n)
            let s = text.slice(0, fixed)
            for (let i = fixed; i < n; i++) {
                s += text[i] === ' ' || text[i] === '\n' ? text[i] : CH[Math.floor(Math.random() * CH.length)]
            }
            el.textContent = s
            if (k < 1) {
                raf.current = requestAnimationFrame(frame)
            } else {
                el.textContent = text
                settle.current = window.setTimeout(() => {
                    el.dataset.s = 'plain'
                }, 350)
            }
        }
        raf.current = requestAnimationFrame(frame)
        return () => {
            cancelAnimationFrame(raf.current)
            window.clearTimeout(settle.current)
        }
    }, [shown, text])

    return <span className={className ?? styles.v} ref={ref} />
}
