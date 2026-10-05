// SPDX-License-Identifier: AGPL-3.0-only
import { RefObject, useCallback, useEffect, useRef, useState } from 'react'

/** Open/close state for a dropdown that closes on outside click and Escape. */
export function usePopover(): {
    close: () => void
    open: boolean
    ref: RefObject<HTMLDivElement | null>
    toggle: () => void
} {
    const [open, setOpen] = useState(false)
    const ref = useRef<HTMLDivElement | null>(null)

    useEffect(() => {
        if (!open) return
        const onDown = (e: MouseEvent | TouchEvent) => {
            if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
        }
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') setOpen(false)
        }
        document.addEventListener('mousedown', onDown)
        document.addEventListener('touchstart', onDown)
        document.addEventListener('keydown', onKey)
        return () => {
            document.removeEventListener('mousedown', onDown)
            document.removeEventListener('touchstart', onDown)
            document.removeEventListener('keydown', onKey)
        }
    }, [open])

    const close = useCallback(() => setOpen(false), [])
    const toggle = useCallback(() => setOpen((v) => !v), [])
    return { close, open, ref, toggle }
}
