// SPDX-License-Identifier: AGPL-3.0-only
// Layout shell forked (top navbar, not left sidebar) from github.com/remnawave/frontend's
// dashboard main-layout (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00); visual design from
// the "Консоль Tuck" artifact: sticky two-row header, hover dropdown nav, drawer on phones.
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Outlet, useLocation } from 'react-router'

import { markSessionExpired } from '@pages/auth/session-expired'
import { logoutEvents } from '@shared/emitters/emit-logout'
import type { Token } from '@shared/api/endpoints/auth'
import { Logo } from '@shared/ui/logo/logo'
import c from '@shared/ui/shell-controls/console.module.css'
import ctl from '@shared/ui/shell-controls/controls.module.css'
import { ShellIcon } from '@shared/ui/shell-controls/icons'
import { ConsoleDrawer } from '@shared/ui/shell-controls/layer'

import { HeaderControls, tokenExpiryMs } from './header-controls'
import { DesktopNavigation } from './navbar/desktop-navigation.layout'
import { MobileNavigation } from './navbar/mobile-navigation.layout'
import styles from './shell.module.css'

export function MainLayout() {
    const { t } = useTranslation('shell')
    const queryClient = useQueryClient()
    const { pathname } = useLocation()
    const [drawerOpen, setDrawerOpen] = useState(false)

    // A 401 from any request means the token is gone (the axios interceptor emits this and the
    // AuthProvider then drops the session). Leave a note for the login screen so it can show the
    // "session expired" variant instead of the plain form.
    useEffect(
        () =>
            logoutEvents.subscribe(() => {
                const state = queryClient.getQueryState<Token>(['auth', 'lookup-self'])
                const ms = tokenExpiryMs(state?.data, state?.dataUpdatedAt ?? Date.now())
                markSessionExpired({ expiresAt: ms ? new Date(ms).toISOString() : undefined })
            }),
        [queryClient]
    )

    useEffect(() => setDrawerOpen(false), [pathname])

    return (
        <div className={styles.root}>
            <div className={c.backdrop} />
            <header className={styles.shead}>
                <div className={styles.row1}>
                    <div className={styles.left}>
                        <button
                            aria-label={t('menu')}
                            className={`${ctl.ctrl} ${styles.burger}`}
                            onClick={() => setDrawerOpen(true)}
                            type="button"
                        >
                            <ShellIcon name="menu" />
                        </button>
                        <Logo />
                    </div>
                    <HeaderControls />
                </div>
                <div className={styles.row2}>
                    <DesktopNavigation />
                </div>
            </header>

            <ConsoleDrawer
                closeLabel={t('close')}
                header={<Logo />}
                label={t('menu')}
                onClose={() => setDrawerOpen(false)}
                opened={drawerOpen}
            >
                <MobileNavigation onNavigate={() => setDrawerOpen(false)} />
            </ConsoleDrawer>

            <main className={styles.main}>
                <Outlet />
            </main>
        </div>
    )
}
