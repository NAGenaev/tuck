// SPDX-License-Identifier: AGPL-3.0-only
// Forked (menu data) from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00);
// rendering follows the "Консоль Tuck" design: hover/click dropdowns with an underline sweep.
import { useEffect, useRef, useState } from 'react'
import { Link, matchPath, useLocation } from 'react-router'

import ctl from '@shared/ui/shell-controls/controls.module.css'
import { ShellIcon } from '@shared/ui/shell-controls/icons'

import { useDesktopMenuSections } from '../menu-sections/desktop-menu-sections'
import styles from '../shell.module.css'

export const isPathActive = (pathname: string, href: string): boolean =>
    matchPath({ path: href, end: false }, pathname) !== null

export const DesktopNavigation = () => {
    const { pathname } = useLocation()
    const menu = useDesktopMenuSections()
    const [openId, setOpenId] = useState<null | string>(null)
    const rootRef = useRef<HTMLElement>(null)
    const hoverTimer = useRef<number | undefined>(undefined)

    // close on route change
    useEffect(() => setOpenId(null), [pathname])

    useEffect(() => {
        if (!openId) return
        const onDown = (e: MouseEvent | TouchEvent) => {
            if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpenId(null)
        }
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') setOpenId(null)
        }
        document.addEventListener('mousedown', onDown)
        document.addEventListener('touchstart', onDown)
        document.addEventListener('keydown', onKey)
        return () => {
            document.removeEventListener('mousedown', onDown)
            document.removeEventListener('touchstart', onDown)
            document.removeEventListener('keydown', onKey)
        }
    }, [openId])

    useEffect(() => () => window.clearTimeout(hoverTimer.current), [])

    const canHover = () => window.matchMedia('(hover: hover)').matches

    return (
        <nav className={styles.nav} ref={rootRef}>
            {menu.map((group) => {
                const id = group.id ?? ''
                const active = group.section.some((item) => isPathActive(pathname, item.href))
                const single = group.section.length === 1 ? group.section[0] : null

                if (single) {
                    return (
                        <div className={styles.ni} key={id}>
                            <Link
                                aria-current={active ? 'page' : undefined}
                                className={styles.nt}
                                data-active={active}
                                to={single.href}
                            >
                                <ShellIcon name={group.icon ?? single.icon} size={17} />
                                <span>{group.header}</span>
                            </Link>
                        </div>
                    )
                }

                const open = openId === id
                return (
                    <div
                        className={styles.ni}
                        key={id}
                        onMouseEnter={() => {
                            if (!canHover()) return
                            window.clearTimeout(hoverTimer.current)
                            setOpenId(id)
                        }}
                        onMouseLeave={() => {
                            if (!canHover()) return
                            hoverTimer.current = window.setTimeout(() => setOpenId(null), 160)
                        }}
                    >
                        <button
                            aria-expanded={open}
                            aria-haspopup="menu"
                            className={styles.nt}
                            data-active={active}
                            data-open={open}
                            onClick={() => setOpenId(open ? null : id)}
                            onKeyDown={(e) => {
                                if (e.key === 'ArrowDown') {
                                    e.preventDefault()
                                    setOpenId(id)
                                    const host = e.currentTarget.parentElement
                                    window.requestAnimationFrame(() =>
                                        host?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
                                    )
                                }
                            }}
                            type="button"
                        >
                            {group.icon && <ShellIcon name={group.icon} size={17} />}
                            <span>{group.header}</span>
                            <ShellIcon className={styles.car} name="car" size={11} />
                        </button>
                        <div className={`${ctl.menu} ${styles.navMenu}`} data-open={open} role="menu">
                            {group.section.map((item) => (
                                <Link
                                    aria-current={isPathActive(pathname, item.href) ? 'page' : undefined}
                                    className={ctl.item}
                                    data-active={isPathActive(pathname, item.href)}
                                    key={item.id}
                                    onClick={() => setOpenId(null)}
                                    role="menuitem"
                                    tabIndex={open ? 0 : -1}
                                    to={item.href}
                                >
                                    <ShellIcon name={item.icon} size={17} />
                                    {item.name}
                                </Link>
                            ))}
                        </div>
                    </div>
                )
            })}
        </nav>
    )
}
