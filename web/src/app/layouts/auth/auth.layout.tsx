// SPDX-License-Identifier: AGPL-3.0-only
// Forked (structure) from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Center } from '@mantine/core'
import { Outlet } from 'react-router'

export function AuthLayout() {
    return (
        <Center h="100vh">
            <Outlet />
        </Center>
    )
}
