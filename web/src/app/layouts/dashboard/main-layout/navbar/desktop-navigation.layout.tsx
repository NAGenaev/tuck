// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Menu, Menubar, UnstyledButton } from '@mantine/core'
import { ElementType, Fragment } from 'react'
import { PiCaretDownBold } from 'react-icons/pi'
import { Link, matchPath, useLocation } from 'react-router'

import { useDesktopMenuSections } from '../menu-sections/desktop-menu-sections'
import classes from './desktop-navigation.module.css'

const NavIcon = ({ icon: Icon }: { icon?: ElementType }) =>
    Icon ? (
        <span className={classes.icon}>
            <Icon />
        </span>
    ) : null

const isPathActive = (pathname: string, href: string): boolean =>
    matchPath({ path: href, end: false }, pathname) !== null

export const DesktopNavigation = () => {
    const { pathname } = useLocation()
    const menu = useDesktopMenuSections()

    return (
        <Menubar className={classes.navBar} trigger="hover">
            {menu.map((section) => {
                const sectionActive = section.section.some((item) => {
                    const hrefs = [
                        item.href,
                        ...(item.dropdownItems?.map((child) => child.href) ?? [])
                    ]
                    return hrefs.some((href) => isPathActive(pathname, href))
                })

                const singleItem =
                    section.section.length === 1 && !section.section[0].dropdownItems
                        ? section.section[0]
                        : null

                if (singleItem) {
                    return (
                        <UnstyledButton
                            className={`${classes.navItem} ${sectionActive ? classes.navItemActive : ''}`}
                            component={Link}
                            key={section.id}
                            to={singleItem.href}
                        >
                            <NavIcon icon={section.icon ?? singleItem.icon} />
                            <span>{section.header}</span>
                        </UnstyledButton>
                    )
                }

                return (
                    <Menubar.Menu key={section.id} width={220} withinPortal>
                        <Menubar.Target
                            className={`${classes.navItem} ${sectionActive ? classes.navItemActive : ''}`}
                        >
                            <NavIcon icon={section.icon} />
                            <span>{section.header}</span>
                            <PiCaretDownBold className={classes.caret} size={11} />
                        </Menubar.Target>
                        <Menubar.Dropdown>
                            {section.section.map((item, index) =>
                                item.dropdownItems ? (
                                    <Fragment key={item.id}>
                                        {index > 0 && <Menu.Divider />}
                                        <Menu.Label>{item.name}</Menu.Label>
                                        {item.dropdownItems.map((dropdownItem) => (
                                            <Menu.Item
                                                className={
                                                    isPathActive(pathname, dropdownItem.href)
                                                        ? classes.menuItemActive
                                                        : undefined
                                                }
                                                component={Link}
                                                key={dropdownItem.id}
                                                leftSection={<NavIcon icon={dropdownItem.icon} />}
                                                to={dropdownItem.href}
                                            >
                                                {dropdownItem.name}
                                            </Menu.Item>
                                        ))}
                                    </Fragment>
                                ) : (
                                    <Menu.Item
                                        className={
                                            isPathActive(pathname, item.href)
                                                ? classes.menuItemActive
                                                : undefined
                                        }
                                        component={Link}
                                        key={item.id}
                                        leftSection={<NavIcon icon={item.icon} />}
                                        to={item.href}
                                    >
                                        {item.name}
                                    </Menu.Item>
                                )
                            )}
                        </Menubar.Dropdown>
                    </Menubar.Menu>
                )
            })}
        </Menubar>
    )
}
