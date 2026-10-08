// SPDX-License-Identifier: AGPL-3.0-only
// Forked (history-mode createBrowserRouter shape) from github.com/remnawave/frontend
// (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00). Routes are Tuck's own.
import { createBrowserRouter, Navigate, RouterProvider } from 'react-router'

import { AuthLayout } from '@app/layouts/auth/auth.layout'
import { MainLayout } from '@app/layouts/dashboard/main-layout/layout'
import { AuthGuard } from '@shared/hocs/guards/auth-guard'
import { ROUTES } from '@shared/constants/routes'

import { LoginPage } from '@pages/auth/login/login.page'
import { OidcCallbackPage } from '@pages/auth/oidc-callback/oidc-callback.page'
import { NotFoundPage } from '@pages/not-found/not-found.page'
import { AuditSinksPage } from '@pages/dashboard/audit-sinks/audit-sinks.page'
import { AuthMethodsPage } from '@pages/dashboard/auth-methods/auth-methods.page'
import { ClusterPage } from '@pages/dashboard/cluster/cluster.page'
import { KV1Page } from '@pages/dashboard/kv1/kv1.page'
import { KV2Page } from '@pages/dashboard/kv2/kv2.page'
import { MountsPage } from '@pages/dashboard/mounts/mounts.page'
import { NamespacesPage } from '@pages/dashboard/namespaces/namespaces.page'
import { PoliciesPage } from '@pages/dashboard/policies/policies.page'
import { StatusPage } from '@pages/dashboard/status/status.page'
import { TeamsPage } from '@pages/dashboard/teams/teams.page'
import { TokenRolesPage } from '@pages/dashboard/token-roles/token-roles.page'
import { TokensPage } from '@pages/dashboard/tokens/tokens.page'
import { TuckSecretsPage } from '@pages/dashboard/tucksecrets/tucksecrets.page'
import { WrappingPage } from '@pages/dashboard/wrapping/wrapping.page'

const router = createBrowserRouter(
    [
        {
            element: <AuthGuard />,
            children: [
                { index: true, element: <Navigate replace to={ROUTES.DASHBOARD.STATUS} /> },
                {
                    path: ROUTES.AUTH.ROOT,
                    element: <AuthLayout />,
                    children: [
                        { path: ROUTES.AUTH.LOGIN, element: <LoginPage /> },
                        { path: ROUTES.AUTH.OIDC_CALLBACK, element: <OidcCallbackPage /> }
                    ]
                },
                {
                    path: ROUTES.DASHBOARD.ROOT,
                    element: <MainLayout />,
                    children: [
                        { path: ROUTES.DASHBOARD.STATUS, element: <StatusPage /> },
                        { path: ROUTES.DASHBOARD.SECRETS_KV1, element: <KV1Page /> },
                        { path: ROUTES.DASHBOARD.SECRETS_KV2, element: <KV2Page /> },
                        { path: ROUTES.DASHBOARD.TOKENS, element: <TokensPage /> },
                        { path: ROUTES.DASHBOARD.POLICIES, element: <PoliciesPage /> },
                        { path: ROUTES.DASHBOARD.AUTH_METHODS, element: <AuthMethodsPage /> },
                        { path: ROUTES.DASHBOARD.WRAPPING, element: <WrappingPage /> },
                        { path: ROUTES.DASHBOARD.NAMESPACES, element: <NamespacesPage /> },
                        { path: ROUTES.DASHBOARD.TOKEN_ROLES, element: <TokenRolesPage /> },
                        { path: ROUTES.DASHBOARD.AUDIT_SINKS, element: <AuditSinksPage /> },
                        { path: ROUTES.DASHBOARD.CLUSTER, element: <ClusterPage /> },
                        { path: ROUTES.DASHBOARD.MOUNTS, element: <MountsPage /> },
                        { path: ROUTES.DASHBOARD.TEAMS, element: <TeamsPage /> },
                        { path: ROUTES.DASHBOARD.TUCKSECRETS, element: <TuckSecretsPage /> }
                    ]
                }
            ]
        },
        { path: '*', element: <NotFoundPage /> }
    ],
    { basename: '/ui' }
)

export function Router() {
    return <RouterProvider router={router} />
}
