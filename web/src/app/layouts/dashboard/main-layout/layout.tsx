// SPDX-License-Identifier: AGPL-3.0-only
// Layout shell forked (top navbar, not left sidebar) from github.com/remnawave/frontend's
// dashboard main-layout (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00); mobile/compact
// variants not carried over — revisit if Tuck needs them later.
import { AppShell, Group } from '@mantine/core'
import { Outlet } from 'react-router'

import { Logo } from '@shared/ui/logo/logo'

import { HeaderControls } from './header-controls'
import { DesktopNavigation } from './navbar/desktop-navigation.layout'

export function MainLayout() {
    return (
        <AppShell header={{ height: 96 }} padding="md">
            <AppShell.Header>
                <Group h={44} justify="space-between" px="md">
                    <Logo />
                    <HeaderControls />
                </Group>
                <Group h={52} px="md" style={{ borderTop: '1px solid var(--mantine-color-dark-4)' }}>
                    <DesktopNavigation />
                </Group>
            </AppShell.Header>

            <AppShell.Main>
                <Outlet />
            </AppShell.Main>
        </AppShell>
    )
}
