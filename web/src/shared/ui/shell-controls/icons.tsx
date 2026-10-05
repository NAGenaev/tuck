// SPDX-License-Identifier: AGPL-3.0-only
import { CSSProperties, ReactNode } from 'react'

const PATHS = {
    lock: (
        <>
            <rect height="11" width="16" x="4" y="10" />
            <path d="M8 10V7a4 4 0 018 0v3" />
        </>
    ),
    unlock: (
        <>
            <rect height="11" width="16" x="4" y="10" />
            <path d="M8 10V7a4 4 0 017.5-2" />
        </>
    ),
    tag: (
        <>
            <path d="M3 12V4h8l10 10-8 8z" />
            <circle cx="7.5" cy="8.5" r="1" />
        </>
    ),
    max: <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />,
    min: <path d="M9 4v5H4M15 4v5h5M9 20v-5H4M15 20v-5h5" />,
    ns: (
        <>
            <rect height="8" width="8" x="3" y="3" />
            <rect height="8" width="8" x="13" y="3" />
            <rect height="8" width="8" x="3" y="13" />
            <path d="M13 17h8M17 13v8" />
        </>
    ),
    lang: <path d="M4 5h9M8.5 3v2M6 5c0 4 3 7 6 8M12 5c-1 4-4 7-8 9M12 21l4.5-10L21 21M13.5 18h6" />,
    gh: (
        <path d="M9 19c-4.3 1.4-4.3-2.5-6-3m12 5v-3.5c0-1 .1-1.4-.5-2 2.8-.3 5.5-1.4 5.5-6a4.6 4.6 0 00-1.3-3.2 4.2 4.2 0 00-.1-3.2s-1.1-.3-3.5 1.3a12.3 12.3 0 00-6.2 0C6.5 2.8 5.4 3.1 5.4 3.1a4.2 4.2 0 00-.1 3.2A4.6 4.6 0 004 9.5c0 4.6 2.7 5.7 5.5 6-.6.6-.6 1.2-.5 2V21" />
    ),
    out: <path d="M14 8V5a1 1 0 00-1-1H5a1 1 0 00-1 1v14a1 1 0 001 1h8a1 1 0 001-1v-3M9 12h12l-3-3M21 12l-3 3" />,
    car: <path d="M5 9l7 7 7-7" />,
    activity: <path d="M3 12h4l3-8 4 16 3-8h4" />,
    folder: <path d="M3 6a1 1 0 011-1h5l2 2h9a1 1 0 011 1v10a1 1 0 01-1 1H4a1 1 0 01-1-1z" />,
    folders: <path d="M8 4h4l2 2h6v9H8zM4 8v12h13" />,
    gift: (
        <>
            <rect height="4" width="18" x="3" y="8" />
            <path d="M5 12v8h14v-8M12 8v12M12 8c-2-4-6-3-5 0M12 8c2-4 6-3 5 0" />
        </>
    ),
    key: (
        <>
            <circle cx="8" cy="15" r="4" />
            <path d="M11 12l9-9M16 7l3 3M14 9l2 2" />
        </>
    ),
    usercog: (
        <>
            <circle cx="9" cy="7" r="4" />
            <path d="M3 21v-2a4 4 0 014-4h3M17.5 21a3 3 0 100-6 3 3 0 000 6zM17.5 13v2M17.5 21v2M21 17.5h-2M16 17.5h-2" />
        </>
    ),
    shield: (
        <>
            <path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z" />
            <path d="M9 12l2 2 4-4" />
        </>
    ),
    login: <path d="M15 4h4a1 1 0 011 1v14a1 1 0 01-1 1h-4M10 8l4 4-4 4M14 12H3" />,
    boxes: (
        <>
            <rect height="8" width="8" x="3" y="3" />
            <rect height="8" width="8" x="13" y="3" />
            <rect height="8" width="8" x="3" y="13" />
            <rect height="8" width="8" x="13" y="13" />
        </>
    ),
    bolt: <path d="M13 3L5 14h6l-1 7 8-11h-6z" />,
    plug: <path d="M9 3v5M15 3v5M6 8h12v4a6 6 0 01-12 0zM12 18v3" />,
    server: (
        <>
            <rect height="6" width="18" x="3" y="4" />
            <rect height="6" width="18" x="3" y="14" />
            <path d="M7 7h.01M7 17h.01" />
        </>
    ),
    file: <path d="M6 3h9l4 4v14H6zM9 12h7M9 16h7" />,
    eye: (
        <>
            <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12z" />
            <circle cx="12" cy="12" r="3" />
        </>
    ),
    eyeoff: (
        <path d="M3 3l18 18M10.6 6.1A9.6 9.6 0 0112 5c6 0 10 7 10 7a17 17 0 01-3.2 4M6.5 6.8C3.8 8.6 2 12 2 12s4 7 10 7c1.6 0 3-.4 4.3-1M9.9 9.9a3 3 0 004.2 4.2" />
    ),
    copy: (
        <>
            <rect height="12" width="12" x="8" y="8" />
            <path d="M4 16V4h12" />
        </>
    ),
    check: <path d="M5 12l5 5 9-10" />,
    menu: <path d="M4 7h16M4 12h16M4 17h16" />,
    x: <path d="M6 6l12 12M18 6L6 18" />,
    paste: (
        <>
            <rect height="17" width="14" x="5" y="4" />
            <path d="M9 4V3h6v1M9 11h6M9 15h4" />
        </>
    ),
    warn: (
        <>
            <path d="M12 3l10 18H2z" />
            <path d="M12 10v5M12 18h.01" />
        </>
    ),
    arrow: <path d="M5 12h14M13 6l6 6-6 6" />,
    clock: (
        <>
            <circle cx="12" cy="12" r="9" />
            <path d="M12 7v5l3 2" />
        </>
    ),
    reticle: (
        <>
            <rect height="18" width="18" x="3" y="3" />
            <circle cx="12" cy="12" r="4" />
            <path d="M12 3v5M12 16v5M3 12h5M16 12h5" />
        </>
    )
} satisfies Record<string, ReactNode>

export type ShellIconName = keyof typeof PATHS

export function ShellIcon({
    className,
    name,
    size = 18,
    stroke = 1.8,
    style
}: {
    className?: string
    name: ShellIconName
    size?: number
    stroke?: number
    style?: CSSProperties
}) {
    return (
        <svg
            aria-hidden="true"
            className={className}
            fill="none"
            height={size}
            stroke="currentColor"
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={stroke}
            style={{ display: 'block', flex: 'none', ...style }}
            viewBox="0 0 24 24"
            width={size}
        >
            {PATHS[name]}
        </svg>
    )
}
