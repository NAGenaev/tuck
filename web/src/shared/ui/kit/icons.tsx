// SPDX-License-Identifier: AGPL-3.0-only
import { CSSProperties, ReactNode } from 'react'

/** The artifact's inline icon set: 24px grid, 1.8 stroke, square caps and miters. */
const PATHS = {
    eye: (
        <>
            <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12z" />
            <circle cx="12" cy="12" r="3" />
        </>
    ),
    eyeoff: (
        <path d="M3 3l18 18M10.6 5.1A9.7 9.7 0 0112 5c6 0 10 7 10 7a17 17 0 01-3.2 3.9M6.4 6.4A17 17 0 002 12s4 7 10 7a9.6 9.6 0 004.2-1" />
    ),
    copy: (
        <>
            <rect height="12" width="12" x="8" y="8" />
            <path d="M4 16V4h12" />
        </>
    ),
    check: <path d="M4 12.5l5 5L20 6.5" />,
    x: <path d="M5 5l14 14M19 5L5 19" />,
    plus: <path d="M12 4v16M4 12h16" />,
    minus: <path d="M4 12h16" />,
    search: (
        <>
            <circle cx="10.5" cy="10.5" r="6.5" />
            <path d="M15.5 15.5L21 21" />
        </>
    ),
    down: <path d="M5 9l7 7 7-7" />,
    up: <path d="M5 15l7-7 7 7" />,
    left: <path d="M15 5l-7 7 7 7" />,
    right: <path d="M9 5l7 7-7 7" />,
    trash: <path d="M4 7h16M9 7V4h6v3M6 7l1 14h10l1-14M10 11v6M14 11v6" />,
    info: (
        <>
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11v6M12 7.5v.01" />
        </>
    ),
    warn: <path d="M12 3L2 20h20L12 3zM12 10v5M12 17.5v.01" />,
    alert: (
        <>
            <circle cx="12" cy="12" r="9" />
            <path d="M12 7v6M12 16.5v.01" />
        </>
    ),
    lock: (
        <>
            <rect height="11" width="16" x="4" y="10" />
            <path d="M8 10V7a4 4 0 018 0v3" />
        </>
    ),
    key: (
        <>
            <circle cx="8" cy="15" r="4" />
            <path d="M11 12l9-9M16 7l3 3" />
        </>
    ),
    refresh: <path d="M20 11A8 8 0 006 7L4 9M4 4v5h5M4 13a8 8 0 0014 4l2-2M20 20v-5h-5" />,
    dots: <path d="M5 12h.01M12 12h.01M19 12h.01" strokeWidth="3" />,
    inbox: <path d="M3 13l3-8h12l3 8v7H3zM3 13h5l1 3h6l1-3h5" />,
    download: <path d="M12 3v12M6 10l6 6 6-6M4 20h16" />,
    gear: (
        <>
            <circle cx="12" cy="12" r="3" />
            <path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M19 5l-2 2M7 17l-2 2" />
        </>
    ),
    bolt: <path d="M13 2L4 14h7l-1 8 9-12h-7z" />,
    user: (
        <>
            <circle cx="12" cy="8" r="4" />
            <path d="M4 21c0-4 4-6 8-6s8 2 8 6" />
        </>
    ),
    ns: (
        <>
            <rect height="8" width="8" x="3" y="3" />
            <rect height="8" width="8" x="13" y="3" />
            <rect height="8" width="8" x="3" y="13" />
            <path d="M17 13v8M13 17h8" />
        </>
    ),
    braces: (
        <path d="M8 4c-3 0-3 2-3 4s0 4-2 4c2 0 2 2 2 4s0 4 3 4M16 4c3 0 3 2 3 4s0 4 2 4c-2 0-2 2-2 4s0 4-3 4" />
    ),
    wand: <path d="M4 20L16 8M14 6l4 4M18 3v3M16.5 4.5h3M6 5v2M5 6h2" />
} satisfies Record<string, ReactNode>

export type IconName = keyof typeof PATHS

export function Icon({
    name,
    size = 16,
    className,
    style
}: {
    className?: string
    name: IconName
    /** px; the glyph scales with currentColor */
    size?: number
    style?: CSSProperties
}) {
    return (
        <svg
            aria-hidden
            className={className}
            fill="none"
            height={size}
            stroke="currentColor"
            strokeLinecap="square"
            strokeLinejoin="miter"
            strokeWidth="1.8"
            style={style}
            viewBox="0 0 24 24"
            width={size}
        >
            {PATHS[name]}
        </svg>
    )
}
