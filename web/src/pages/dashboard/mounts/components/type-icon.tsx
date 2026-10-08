// SPDX-License-Identifier: AGPL-3.0-only
import { ReactNode } from 'react'

/** The artifact's engine glyphs (24px grid, 1.8 stroke, square caps). */
const PATHS: Record<string, ReactNode> = {
    kv: (
        <>
            <rect height="11" width="16" x="4" y="10" />
            <path d="M8 10V7a4 4 0 018 0v3M12 14v3" />
        </>
    ),
    k8s: (
        <>
            <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9z" />
            <circle cx="12" cy="12" r="2.5" />
            <path d="M12 3v6.5M4 7.5l5.8 3.3M20 7.5l-5.8 3.3" />
        </>
    ),
    key: (
        <>
            <circle cx="8" cy="12" r="4" />
            <path d="M12 12h9M18 12v4M15 12v3" />
        </>
    ),
    jwt: (
        <>
            <path d="M4 6h16v12H4z" />
            <path d="M8 10h8M8 14h5" />
        </>
    ),
    db: (
        <>
            <ellipse cx="12" cy="6" rx="7" ry="3" />
            <path d="M5 6v12c0 1.7 3.1 3 7 3s7-1.3 7-3V6M5 12c0 1.7 3.1 3 7 3s7-1.3 7-3" />
        </>
    ),
    plug: <path d="M9 3v5M15 3v5M6 8h12v4a6 6 0 01-12 0zM12 18v3" />
}

export function TypeIcon({ name, size = 19 }: { name: string; size?: number }) {
    return (
        <svg
            aria-hidden
            fill="none"
            height={size}
            stroke="currentColor"
            strokeLinecap="square"
            strokeWidth="1.8"
            viewBox="0 0 24 24"
            width={size}
        >
            {PATHS[name] ?? PATHS.plug}
        </svg>
    )
}
