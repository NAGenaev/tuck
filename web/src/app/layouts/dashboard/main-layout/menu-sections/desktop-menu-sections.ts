// SPDX-License-Identifier: AGPL-3.0-only
// Menu data structure follows the pattern forked from github.com/remnawave/frontend's
// desktop-menu-sections (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00); content is Tuck's own.
import { useTranslation } from 'react-i18next'

import { ROUTES } from '@shared/constants/routes'

import { MenuItem } from './interfaces'

export const useDesktopMenuSections = (): MenuItem[] => {
    const { t } = useTranslation()

    return [
        {
            id: 'overview',
            header: t('nav.status'),
            icon: 'activity',
            section: [{ id: 'status', name: t('nav.status'), href: ROUTES.DASHBOARD.STATUS, icon: 'activity' }]
        },
        {
            id: 'secrets',
            header: t('nav.secrets'),
            icon: 'folder',
            section: [
                { id: 'kv1', name: t('nav.kv1'), href: ROUTES.DASHBOARD.SECRETS_KV1, icon: 'folder' },
                { id: 'kv2', name: t('nav.kv2'), href: ROUTES.DASHBOARD.SECRETS_KV2, icon: 'folders' },
                { id: 'wrapping', name: t('nav.wrapping'), href: ROUTES.DASHBOARD.WRAPPING, icon: 'gift' }
            ]
        },
        {
            id: 'access',
            header: t('nav.access'),
            icon: 'shield',
            section: [
                { id: 'tokens', name: t('nav.tokens'), href: ROUTES.DASHBOARD.TOKENS, icon: 'key' },
                {
                    id: 'token-roles',
                    name: t('nav.tokenRoles'),
                    href: ROUTES.DASHBOARD.TOKEN_ROLES,
                    icon: 'usercog'
                },
                { id: 'policies', name: t('nav.policies'), href: ROUTES.DASHBOARD.POLICIES, icon: 'shield' },
                {
                    id: 'auth-methods',
                    name: t('nav.authMethods'),
                    href: ROUTES.DASHBOARD.AUTH_METHODS,
                    icon: 'login'
                },
                {
                    id: 'namespaces',
                    name: t('nav.namespaces'),
                    href: ROUTES.DASHBOARD.NAMESPACES,
                    icon: 'boxes'
                }
            ]
        },
        {
            id: 'engines',
            header: t('nav.engines'),
            icon: 'plug',
            section: [
                { id: 'mounts', name: t('nav.mounts'), href: ROUTES.DASHBOARD.MOUNTS, icon: 'plug' }
            ]
        },
        {
            id: 'ops',
            header: t('nav.operations'),
            icon: 'server',
            section: [
                { id: 'cluster', name: t('nav.cluster'), href: ROUTES.DASHBOARD.CLUSTER, icon: 'server' },
                { id: 'audit-sinks', name: t('nav.auditSinks'), href: ROUTES.DASHBOARD.AUDIT_SINKS, icon: 'file' }
            ]
        }
    ]
}
