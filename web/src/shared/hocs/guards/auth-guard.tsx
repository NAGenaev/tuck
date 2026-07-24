// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Navigate, Outlet, useLocation } from 'react-router'

import { ROUTES } from '@shared/constants/routes'
import { useAuth } from '@shared/hooks/use-auth'
import { LoadingScreen } from '@shared/ui/loading-screen/loading-screen'

export function AuthGuard() {
    const location = useLocation()
    const { isAuthenticated, isInitialized } = useAuth()

    if (!isInitialized) {
        return <LoadingScreen />
    }

    if (!isAuthenticated) {
        if (location.pathname.startsWith(ROUTES.AUTH.ROOT)) {
            return <Outlet />
        }
        return <Navigate replace to={ROUTES.AUTH.LOGIN} />
    }

    if (location.pathname.startsWith(ROUTES.AUTH.ROOT)) {
        return <Navigate replace to={ROUTES.DASHBOARD.STATUS} />
    }

    return <Outlet />
}
