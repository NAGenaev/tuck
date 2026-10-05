// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '../copy/copy'
import { cx, prefersReducedMotion } from '../cx'
import { Icon } from '../icons'
import styles from './reveal.module.css'

const GLYPHS = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz0123456789#$%&*+=?@'
const SCRAMBLE_MS = 640

function scrambled(real: string, resolved: number) {
    let out = ''
    for (let i = 0; i < real.length; i++) {
        const ch = real[i]
        out += i < resolved || /\s/.test(ch) ? ch : GLYPHS[Math.floor(Math.random() * GLYPHS.length)]
    }
    return out
}

/**
 * Masked secret value. Click the eye to "decrypt" it (characters resolve left to right), and it
 * re-masks itself after `autoMask` seconds with a brass countdown line.
 * The copy button always copies the real value, masked or not.
 */
export function Reveal({
    value,
    autoMask = 20,
    scramble = true,
    copyable = true,
    maskLength = 24,
    onReveal,
    className
}: {
    /** seconds until the value hides again; 0 keeps it visible until toggled */
    autoMask?: number
    className?: string
    copyable?: boolean
    /** max bullets shown while masked (the real length is never leaked past it) */
    maskLength?: number
    /** called when the user reveals, e.g. to write an audit trail */
    onReveal?: () => void
    /** play the decrypt animation (ignored with prefers-reduced-motion) */
    scramble?: boolean
    value: string
}) {
    const { t } = useTranslation('forms')
    const [shown, setShown] = useState(false)
    const [text, setText] = useState('')
    const [left, setLeft] = useState(autoMask)
    const flat = value.replace(/\s+/g, ' ')
    const raf = useRef(0)

    // hide again if the underlying value changes
    useEffect(() => setShown(false), [value])

    // decrypt animation
    useEffect(() => {
        cancelAnimationFrame(raf.current)
        if (!shown) return
        if (!scramble || prefersReducedMotion() || !flat) {
            setText(flat)
            return
        }
        const start = performance.now()
        const step = (now: number) => {
            const p = Math.min(1, (now - start) / SCRAMBLE_MS)
            setText(p >= 1 ? flat : scrambled(flat, Math.floor(p * flat.length)))
            if (p < 1) raf.current = requestAnimationFrame(step)
        }
        raf.current = requestAnimationFrame(step)
        return () => cancelAnimationFrame(raf.current)
    }, [shown, flat, scramble])

    // auto re-mask
    useEffect(() => {
        if (!shown || autoMask <= 0) return
        setLeft(autoMask)
        const id = window.setInterval(() => {
            setLeft((n) => {
                if (n <= 1) {
                    window.clearInterval(id)
                    setShown(false)
                    return autoMask
                }
                return n - 1
            })
        }, 1000)
        return () => window.clearInterval(id)
    }, [shown, autoMask])

    const masked = '•'.repeat(Math.min(Math.max(flat.length, 1), maskLength))

    return (
        <div className={cx(styles.root, className)} data-shown={shown || undefined}>
            <span aria-label={shown ? value : t('reveal.hidden')} className={styles.text} title={shown ? value : undefined}>
                {shown ? text : masked}
            </span>
            {shown && autoMask > 0 && <span className={styles.left}>{left}s</span>}
            <button
                aria-label={shown ? t('reveal.hide') : t('reveal.show')}
                aria-pressed={shown}
                className={styles.eye}
                onClick={() => {
                    if (!shown) onReveal?.()
                    setShown((s) => !s)
                }}
                title={shown ? t('reveal.hide') : t('reveal.show')}
                type="button"
            >
                <Icon name={shown ? 'eyeoff' : 'eye'} size={15} />
            </button>
            {copyable && <CopyButton className={styles.copy} size={34} value={value} />}
            {shown && autoMask > 0 && (
                <i className={styles.bar} key={String(shown)} style={{ animationDuration: `${autoMask}s` }} />
            )}
        </div>
    )
}
