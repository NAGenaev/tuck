// SPDX-License-Identifier: AGPL-3.0-only
import { ReactNode } from 'react'

const PATHS = {
    clock: (
        <>
            <circle cx="12" cy="12" r="9" />
            <path d="M12 7v5l3 2" />
        </>
    ),
    lock: (
        <>
            <rect height="11" width="16" x="4" y="10" />
            <path d="M8 10V7a4 4 0 018 0v3" />
        </>
    ),
    look: (
        <>
            <circle cx="10.5" cy="10.5" r="6" />
            <path d="M15 15l6 6" />
        </>
    ),
    send: <path d="M3 12L21 4l-6 17-3-7z" />,
    unwrap: (
        <>
            <rect height="11" width="16" x="4" y="10" />
            <path d="M8 10V7a4 4 0 017.5-2" />
        </>
    ),
    wrap: (
        <>
            <rect height="13" width="18" x="3" y="6" />
            <path d="M3 6l9 7 9-7" />
        </>
    ),
    check: <path d="M4 12l5 5 11-11" />,
    x: <path d="M5 5l14 14M19 5L5 19" />
} satisfies Record<string, ReactNode>

export type WrapIconName = keyof typeof PATHS

export function WIcon({ name, size = 16 }: { name: WrapIconName; size?: number }) {
    return (
        <svg
            aria-hidden
            fill="none"
            height={size}
            stroke="currentColor"
            strokeWidth="1.8"
            viewBox="0 0 24 24"
            width={size}
        >
            {PATHS[name]}
        </svg>
    )
}
