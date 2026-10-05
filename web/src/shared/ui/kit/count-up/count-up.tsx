// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { prefersReducedMotion } from '../cx'

/**
 * Number that eases from its previous value to the new one (ease-out cubic).
 * Locale-aware grouping by default; with reduced motion it just shows the value.
 */
export function CountUp({
    value,
    duration = 900,
    decimals = 0,
    format,
    className
}: {
    className?: string
    /** decimal places for the default formatter */
    decimals?: number
    /** ms */
    duration?: number
    /** custom formatter, e.g. (n) => `${n}%` */
    format?: (n: number) => string
    value: number
}) {
    const { i18n } = useTranslation()
    const [shown, setShown] = useState(prefersReducedMotion() ? value : 0)
    const from = useRef(shown)

    useEffect(() => {
        if (prefersReducedMotion() || duration <= 0) {
            from.current = value
            setShown(value)
            return
        }
        const start = performance.now()
        const origin = from.current
        let raf = 0
        const step = (now: number) => {
            const p = Math.min(1, (now - start) / duration)
            const eased = 1 - Math.pow(1 - p, 3)
            const v = origin + (value - origin) * eased
            from.current = v
            setShown(v)
            if (p < 1) raf = requestAnimationFrame(step)
        }
        raf = requestAnimationFrame(step)
        return () => cancelAnimationFrame(raf)
    }, [value, duration])

    const text = format
        ? format(shown)
        : new Intl.NumberFormat(i18n.language, {
              maximumFractionDigits: decimals,
              minimumFractionDigits: decimals
          }).format(shown)

    return (
        <span className={className} style={{ fontVariantNumeric: 'tabular-nums' }}>
            {text}
        </span>
    )
}
