// SPDX-License-Identifier: AGPL-3.0-only
import { CSSProperties, ReactNode, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Icon } from '../kit/icons'
import styles from './tuck.module.css'

const cx = (...parts: Array<false | null | string | undefined>) => parts.filter(Boolean).join(' ')

export function Eyebrow({ children, className }: { children: ReactNode; className?: string }) {
    return <div className={cx(styles.eyebrow, className)}>{children}</div>
}

export type PlateVariant = 'brass' | 'danger' | 'default' | 'flat'

export function Plate({
    children,
    className,
    style,
    variant = 'default',
    padded
}: {
    children: ReactNode
    className?: string
    /** pad the plate like the artifact's .plate (clamp 20..32px) */
    padded?: boolean
    style?: CSSProperties
    /** default = rivets, brass halo; flat = no halo; brass = brass frame; danger = alarm frame */
    variant?: PlateVariant
}) {
    return (
        <div
            className={cx(styles.plate, padded && styles.platePadded, className)}
            data-variant={variant}
            style={style}
        >
            <i className={styles.rivet} />
            <i className={styles.rivet} />
            <i className={styles.rivet} />
            <i className={styles.rivet} />
            {children}
        </div>
    )
}

export function PlateBar({ children }: { children?: ReactNode }) {
    return (
        <div className={styles.plateBar}>
            <span />
            <span />
            <span />
            {children && <em>{children}</em>}
        </div>
    )
}

/** Small caps title of a plate with a hairline running to the right (.plate-t). */
export function PlateTitle({ children, right }: { children: ReactNode; right?: ReactNode }) {
    return (
        <h3 className={styles.plateTitle}>
            <span>{children}</span>
            <i />
            {right}
        </h3>
    )
}

/** Sub-heading with a brass square (.sub). */
export function Sub({ children, className }: { children: ReactNode; className?: string }) {
    return <div className={cx(styles.sub, className)}>{children}</div>
}

export function Term({ children }: { children: ReactNode }) {
    return <div className={styles.term}>{children}</div>
}

export function Cursor() {
    return <span className={styles.cursor} />
}

/**
 * Terminal line that types itself out, then keeps a blinking cursor.
 * With reduced motion (or speed 0) the whole text is shown at once.
 */
export function TypeLine({
    text,
    speed = 38,
    prompt = '$',
    onDone
}: {
    onDone?: () => void
    /** prompt glyph shown in brass before the text; pass null to hide */
    prompt?: null | string
    /** ms per character; 0 disables the animation */
    speed?: number
    text: string
}) {
    const [count, setCount] = useState(0)

    useEffect(() => {
        const reduce =
            typeof window !== 'undefined' &&
            window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
        if (!speed || reduce) {
            setCount(text.length)
            onDone?.()
            return
        }
        setCount(0)
        let i = 0
        const id = window.setInterval(() => {
            i += 1
            setCount(i)
            if (i >= text.length) {
                window.clearInterval(id)
                onDone?.()
            }
        }, speed)
        return () => window.clearInterval(id)
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [text, speed])

    return (
        <div className={styles.typeLine} aria-label={text}>
            {prompt && <span className={styles.typePrompt}>{prompt}</span>}
            <span aria-hidden>{text.slice(0, count)}</span>
            <Cursor />
        </div>
    )
}

/** Page-wide faint grid, fades out away from the top-right corner. Put once near the root. */
export function GridBackdrop({ className }: { className?: string }) {
    return <div aria-hidden className={cx(styles.gridBackdrop, className)} />
}

/** Breathing brass halo in the top-right corner. Put once near the root. */
export function Glow({ className }: { className?: string }) {
    return <div aria-hidden className={cx(styles.glow, className)} />
}

export function Bento({
    children,
    columns,
    style
}: {
    children: ReactNode
    columns: string
    style?: CSSProperties
}) {
    return (
        <div className={styles.bento} style={{ gridTemplateColumns: columns, ...style }}>
            {children}
        </div>
    )
}

export function Tile({
    label,
    value,
    hint,
    tone,
    children
}: {
    children?: ReactNode
    hint?: string
    label: string
    tone?: 'bad' | 'brass' | 'ok'
    value?: number | string
}) {
    return (
        <div className={styles.tile}>
            <span className={styles.tileLabel}>{label}</span>
            {value !== undefined && (
                <span className={styles.tileValue} data-tone={tone} title={String(value)}>
                    {value}
                </span>
            )}
            {children}
            {hint && <span className={styles.tileHint}>{hint}</span>}
        </div>
    )
}

export type PillTone = 'bad' | 'brass' | 'ice' | 'mute' | 'ok' | 'violet'

// the artifact's short tone codes (.pill.g .b .c .r .v .n) work too
const TONE_ALIAS = { b: 'brass', c: 'ice', g: 'ok', n: 'mute', r: 'bad', v: 'violet' } as const

export function Pill({
    children,
    tone = 'brass',
    variant = 'outline',
    size = 'md',
    dot,
    count,
    onRemove,
    title
}: {
    children: ReactNode
    /** trailing mono counter (.pill.cnt) */
    count?: number | string
    /** leading glowing dot (.pill.dt) */
    dot?: boolean
    /** renders a remove button (.pill.rm) */
    onRemove?: () => void
    size?: 'lg' | 'md' | 'sm'
    title?: string
    tone?: keyof typeof TONE_ALIAS | PillTone
    /** outline = line only; soft = tinted; fill = solid with ink text */
    variant?: 'fill' | 'outline' | 'soft'
}) {
    const { t } = useTranslation('forms')
    const resolved = tone in TONE_ALIAS ? TONE_ALIAS[tone as keyof typeof TONE_ALIAS] : tone
    return (
        <span
            className={styles.pill}
            data-dot={dot || undefined}
            data-size={size}
            data-tone={resolved}
            data-variant={variant}
            title={title}
        >
            {children}
            {count !== undefined && <b>{count}</b>}
            {onRemove && (
                <button
                    aria-label={t('pill.remove')}
                    className={styles.pillRemove}
                    onClick={onRemove}
                    type="button"
                >
                    <Icon name="x" size={9} />
                </button>
            )}
        </span>
    )
}
