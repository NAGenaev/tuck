// SPDX-License-Identifier: AGPL-3.0-only
// Query keys/options shared by the page (method rail) and the panels, so both read one cache entry.
import { queryOptions } from '@tanstack/react-query'

import {
    getJwtConfig,
    getLdapConfig,
    listAppRoles,
    listJwtRoles,
    listK8sClusters,
    listLdapRoles,
    retryUnlessDenied
} from './api'

const root = ['auth-methods'] as const

export const qk = {
    approleList: [...root, 'approle', 'list'] as const,
    approleRole: (name: string) => [...root, 'approle', 'role', name] as const,
    jwtConfig: [...root, 'jwt', 'config'] as const,
    jwtRole: (name: string) => [...root, 'jwt', 'role', name] as const,
    jwtRoles: [...root, 'jwt', 'roles'] as const,
    k8sClusters: [...root, 'k8s', 'clusters'] as const,
    ldapConfig: [...root, 'ldap', 'config'] as const,
    ldapRole: (name: string) => [...root, 'ldap', 'role', name] as const,
    ldapRoles: [...root, 'ldap', 'roles'] as const
}

export const approleListQ = () =>
    queryOptions({ queryKey: qk.approleList, queryFn: listAppRoles, retry: retryUnlessDenied })
export const jwtConfigQ = () => queryOptions({ queryKey: qk.jwtConfig, queryFn: getJwtConfig, retry: retryUnlessDenied })
export const jwtRolesQ = () => queryOptions({ queryKey: qk.jwtRoles, queryFn: listJwtRoles, retry: retryUnlessDenied })
export const ldapConfigQ = () =>
    queryOptions({ queryKey: qk.ldapConfig, queryFn: getLdapConfig, retry: retryUnlessDenied })
export const ldapRolesQ = () =>
    queryOptions({ queryKey: qk.ldapRoles, queryFn: listLdapRoles, retry: retryUnlessDenied })
export const k8sClustersQ = () =>
    queryOptions({ queryKey: qk.k8sClusters, queryFn: listK8sClusters, retry: retryUnlessDenied })
