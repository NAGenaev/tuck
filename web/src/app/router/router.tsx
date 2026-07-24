// SPDX-License-Identifier: AGPL-3.0-only
// Forked (history-mode createBrowserRouter shape) from github.com/remnawave/frontend
// (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00). Routes are Tuck's own.
import { createBrowserRouter, Navigate, RouterProvider } from 'react-router'

import { AuthLayout } from '@app/layouts/auth/auth.layout'
import { MainLayout } from '@app/layouts/dashboard/main-layout/layout'
import { AuthGuard } from '@shared/hocs/guards/auth-guard'
import { ROUTES } from '@shared/constants/routes'

import { LoginPage } from '@pages/auth/login/login.page'
import { AuditSinksPage } from '@pages/dashboard/audit-sinks/audit-sinks.page'
import { AuthMethodsPage } from '@pages/dashboard/auth-methods/auth-methods.page'
import { ClusterPage } from '@pages/dashboard/cluster/cluster.page'
import { CryptoEnginesPage } from '@pages/dashboard/crypto-engines/crypto-engines.page'
import { DynamicSecretsPage } from '@pages/dashboard/dynamic-secrets/dynamic-secrets.page'
import { KV1Page } from '@pages/dashboard/kv1/kv1.page'
import { KV2Page } from '@pages/dashboard/kv2/kv2.page'
import { MountsPage } from '@pages/dashboard/mounts/mounts.page'
import { NamespacesPage } from '@pages/dashboard/namespaces/namespaces.page'
import { PluginsPage } from '@pages/dashboard/plugins/plugins.page'
import { PoliciesPage } from '@pages/dashboard/policies/policies.page'
import { ReplicationPage } from '@pages/dashboard/replication/replication.page'
import { StatusPage } from '@pages/dashboard/status/status.page'
import { TokenRolesPage } from '@pages/dashboard/token-roles/token-roles.page'
import { TokensPage } from '@pages/dashboard/tokens/tokens.page'
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
                    children: [{ path: ROUTES.AUTH.LOGIN, element: <LoginPage /> }]
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
                        { path: ROUTES.DASHBOARD.DYNAMIC_SECRETS, element: <DynamicSecretsPage /> },
                        { path: ROUTES.DASHBOARD.CRYPTO_ENGINES, element: <CryptoEnginesPage /> },
                        { path: ROUTES.DASHBOARD.WRAPPING, element: <WrappingPage /> },
                        { path: ROUTES.DASHBOARD.NAMESPACES, element: <NamespacesPage /> },
                        { path: ROUTES.DASHBOARD.TOKEN_ROLES, element: <TokenRolesPage /> },
                        { path: ROUTES.DASHBOARD.AUDIT_SINKS, element: <AuditSinksPage /> },
                        { path: ROUTES.DASHBOARD.CLUSTER, element: <ClusterPage /> },
                        { path: ROUTES.DASHBOARD.MOUNTS, element: <MountsPage /> },
                        { path: ROUTES.DASHBOARD.PLUGINS, element: <PluginsPage /> },
                        { path: ROUTES.DASHBOARD.REPLICATION, element: <ReplicationPage /> }
                    ]
                }
            ]
        }
    ],
    { basename: '/ui' }
)

export function Router() {
    return <RouterProvider router={router} />
}
