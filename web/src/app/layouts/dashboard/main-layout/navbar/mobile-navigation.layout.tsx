// SPDX-License-Identifier: AGPL-3.0-only
import { useState } from 'react'
import { Link, useLocation } from 'react-router'

import { ShellIcon } from '@shared/ui/shell-controls/icons'

import { useDesktopMenuSections } from '../menu-sections/desktop-menu-sections'
import styles from '../shell.module.css'
import { isPathActive } from './desktop-navigation.layout'

/** Drawer accordion with the same groups and routes as the desktop menu. */
export const MobileNavigation = ({ onNavigate }: { onNavigate: () => void }) => {
    const { pathname } = useLocation()
    const menu = useDesktopMenuSections()
    const [openId, setOpenId] = useState<null | string>(
        () => menu.find((g) => g.section.length > 1 && g.section.some((i) => isPathActive(pathname, i.href)))?.id ?? null
    )

    return (
        <>
            {menu.map((group) => {
                const id = group.id ?? ''
                const active = group.section.some((item) => isPathActive(pathname, item.href))
                const single = group.section.length === 1 ? group.section[0] : null

                if (single) {
                    return (
                        <div className={styles.acc} key={id}>
                            <Link
                                className={styles.accBtn}
                                data-active={active}
                                onClick={onNavigate}
                                to={single.href}
                            >
                                <ShellIcon name={group.icon ?? single.icon} size={17} />
                                {group.header}
                            </Link>
                        </div>
                    )
                }

                const open = openId === id
                return (
                    <div className={styles.acc} data-open={open} key={id}>
                        <button
                            aria-expanded={open}
                            className={styles.accBtn}
                            data-active={active}
                            onClick={() => setOpenId(open ? null : id)}
                            type="button"
                        >
                            {group.icon && <ShellIcon name={group.icon} size={17} />}
                            {group.header}
                            <ShellIcon className={styles.car} name="car" size={12} />
                        </button>
                        <div className={styles.sub}>
                            <div>
                                {group.section.map((item) => (
                                    <Link
                                        data-active={isPathActive(pathname, item.href)}
                                        key={item.id}
                                        onClick={onNavigate}
                                        tabIndex={open ? 0 : -1}
                                        to={item.href}
                                    >
                                        <ShellIcon name={item.icon} size={17} />
                                        {item.name}
                                    </Link>
                                ))}
                            </div>
                        </div>
                    </div>
                )
            })}
        </>
    )
}
