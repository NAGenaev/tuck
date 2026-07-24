// Menu data structure follows the pattern forked from github.com/remnawave/frontend's
// desktop-menu-sections (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00); content is Tuck's own.
import { useTranslation } from 'react-i18next'
import {
    TbActivity,
    TbBolt,
    TbBoxMultiple,
    TbFileText,
    TbFolder,
    TbFolders,
    TbGift,
    TbKey,
    TbLock,
    TbLogin,
    TbPlugConnected,
    TbPuzzle,
    TbRefresh,
    TbServer2,
    TbShieldCheck,
    TbUserCog
} from 'react-icons/tb'

import { ROUTES } from '@shared/constants/routes'

import { MenuItem } from './interfaces'

export const useDesktopMenuSections = (): MenuItem[] => {
    const { t } = useTranslation()

    return [
        {
            id: 'overview',
            header: t('nav.status'),
            icon: TbActivity,
            section: [
                { id: 'status', name: t('nav.status'), href: ROUTES.DASHBOARD.STATUS, icon: TbActivity }
            ]
        },
        {
            id: 'secrets',
            header: t('nav.secrets'),
            icon: TbFolder,
            section: [
                { id: 'kv1', name: t('nav.kv1'), href: ROUTES.DASHBOARD.SECRETS_KV1, icon: TbFolder },
                { id: 'kv2', name: t('nav.kv2'), href: ROUTES.DASHBOARD.SECRETS_KV2, icon: TbFolders },
                { id: 'wrapping', name: t('nav.wrapping'), href: ROUTES.DASHBOARD.WRAPPING, icon: TbGift }
            ]
        },
        {
            id: 'access',
            header: t('nav.access'),
            icon: TbShieldCheck,
            section: [
                { id: 'tokens', name: t('nav.tokens'), href: ROUTES.DASHBOARD.TOKENS, icon: TbKey },
                {
                    id: 'token-roles',
                    name: t('nav.tokenRoles'),
                    href: ROUTES.DASHBOARD.TOKEN_ROLES,
                    icon: TbUserCog
                },
                {
                    id: 'policies',
                    name: t('nav.policies'),
                    href: ROUTES.DASHBOARD.POLICIES,
                    icon: TbShieldCheck
                },
                {
                    id: 'auth-methods',
                    name: t('nav.authMethods'),
                    href: ROUTES.DASHBOARD.AUTH_METHODS,
                    icon: TbLogin
                },
                {
                    id: 'namespaces',
                    name: t('nav.namespaces'),
                    href: ROUTES.DASHBOARD.NAMESPACES,
                    icon: TbBoxMultiple
                }
            ]
        },
        {
            id: 'engines',
            header: t('nav.engines'),
            icon: TbPlugConnected,
            section: [
                {
                    id: 'dynamic-secrets',
                    name: t('nav.dynamicSecrets'),
                    href: ROUTES.DASHBOARD.DYNAMIC_SECRETS,
                    icon: TbBolt
                },
                {
                    id: 'crypto-engines',
                    name: t('nav.cryptoEngines'),
                    href: ROUTES.DASHBOARD.CRYPTO_ENGINES,
                    icon: TbLock
                },
                { id: 'mounts', name: t('nav.mounts'), href: ROUTES.DASHBOARD.MOUNTS, icon: TbPlugConnected },
                { id: 'plugins', name: t('nav.plugins'), href: ROUTES.DASHBOARD.PLUGINS, icon: TbPuzzle }
            ]
        },
        {
            id: 'ops',
            header: t('nav.operations'),
            icon: TbServer2,
            section: [
                { id: 'cluster', name: t('nav.cluster'), href: ROUTES.DASHBOARD.CLUSTER, icon: TbServer2 },
                {
                    id: 'replication',
                    name: t('nav.replication'),
                    href: ROUTES.DASHBOARD.REPLICATION,
                    icon: TbRefresh
                },
                {
                    id: 'audit-sinks',
                    name: t('nav.auditSinks'),
                    href: ROUTES.DASHBOARD.AUDIT_SINKS,
                    icon: TbFileText
                }
            ]
        }
    ]
}
