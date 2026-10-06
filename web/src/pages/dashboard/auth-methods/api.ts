// SPDX-License-Identifier: AGPL-3.0-only
// Local API layer for the auth-methods screen. The shared clients under @shared/api/endpoints/auth-*
// predate tuck_namespace, multi-cluster Kubernetes and the full LDAP config; this file speaks to the
// same endpoints with the complete wire shape so that saving never drops fields the form doesn't show.
import { isAxiosError } from 'axios'

import { instance } from '@shared/api/axios'
import { listRequest } from '@shared/api/list'

// ---------- shared ----------

export const status = (err: unknown): number | undefined => (isAxiosError(err) ? err.response?.status : undefined)

export function serverMessage(err: unknown): string | undefined {
    if (isAxiosError(err)) {
        const d = err.response?.data as { error?: string } | undefined
        return d?.error ?? err.message
    }
    return err instanceof Error ? err.message : undefined
}

export const retryUnlessDenied = (count: number, err: unknown) => {
    const s = status(err)
    return s !== 403 && s !== 404 && count < 2
}

// ---------- namespaces (for the tuck_namespace picker) ----------

export const listNamespaces = (): Promise<string[]> => listRequest('/v1/sys/namespaces/')

// ---------- AppRole ----------

export interface AppRoleFull {
    bound_cidrs?: string[]
    name: string
    policies: string[]
    role_id: string
    /** nanoseconds (Go time.Duration) */
    secret_id_num_uses?: number
    secret_id_ttl?: number
    token_ttl?: number
    tuck_namespace?: string
}

export interface AppRoleBody {
    policies: string[]
    role_id?: string
    secret_id_num_uses?: number
    secret_id_ttl?: string
    token_ttl?: string
    tuck_namespace?: string
}

export interface SecretIdResp {
    created_at: string
    expires_at?: string
    id: string
    num_uses: number
    role_name: string
    uses_left: number
}

export const listAppRoles = (): Promise<string[]> => listRequest('/v1/auth/approle/role/')

export async function getAppRole(name: string): Promise<AppRoleFull> {
    const { data } = await instance.get<AppRoleFull>(`/v1/auth/approle/role/${encodeURIComponent(name)}`)
    return data
}

export async function putAppRole(name: string, body: AppRoleBody): Promise<AppRoleFull> {
    const { data } = await instance.put<AppRoleFull>(`/v1/auth/approle/role/${encodeURIComponent(name)}`, body)
    return data
}

export async function deleteAppRole(name: string): Promise<void> {
    await instance.delete(`/v1/auth/approle/role/${encodeURIComponent(name)}`)
}

export async function generateSecretId(name: string): Promise<SecretIdResp> {
    const { data } = await instance.post<SecretIdResp>(`/v1/auth/approle/role/${encodeURIComponent(name)}/secret-id`, {})
    return data
}

// ---------- JWT ----------

export interface JwtConfigFull {
    audience?: string
    default_ttl?: number
    issuer?: string
    jwks_uri: string
}

export interface JwtRoleFull {
    bound_audiences?: string[]
    bound_claims?: Record<string, string>
    bound_subject?: string
    name: string
    policies: string[]
    ttl?: number
    tuck_namespace?: string
}

export interface JwtRoleBody {
    bound_audiences?: string[]
    bound_claims?: Record<string, string>
    bound_subject?: string
    policies: string[]
    ttl?: string
    tuck_namespace?: string
}

export async function getJwtConfig(): Promise<JwtConfigFull> {
    const { data } = await instance.get<JwtConfigFull>('/v1/auth/jwt/config')
    return data
}

export async function putJwtConfig(body: {
    audience?: string
    default_ttl?: string
    issuer?: string
    jwks_uri: string
}): Promise<void> {
    await instance.put('/v1/auth/jwt/config', body)
}

export const listJwtRoles = (): Promise<string[]> => listRequest('/v1/auth/jwt/role/')

export async function getJwtRole(name: string): Promise<JwtRoleFull> {
    const { data } = await instance.get<JwtRoleFull>(`/v1/auth/jwt/role/${encodeURIComponent(name)}`)
    return data
}

export async function putJwtRole(name: string, body: JwtRoleBody): Promise<void> {
    await instance.put(`/v1/auth/jwt/role/${encodeURIComponent(name)}`, body)
}

export async function deleteJwtRole(name: string): Promise<void> {
    await instance.delete(`/v1/auth/jwt/role/${encodeURIComponent(name)}`)
}

// ---------- LDAP ----------

export interface LdapConfigFull {
    bind_dn?: string
    bind_password?: string
    group_attr?: string
    group_dn?: string
    group_filter?: string
    starttls?: boolean
    tls_insecure?: boolean
    urls: string[]
    user_attr?: string
    user_dn: string
}

export interface LdapRoleFull {
    groups?: string[]
    name: string
    policies: string[]
    ttl?: number
    tuck_namespace?: string
    users?: string[]
}

export interface LdapRoleBody {
    groups?: string[]
    policies: string[]
    ttl?: string
    tuck_namespace?: string
    users?: string[]
}

export async function getLdapConfig(): Promise<LdapConfigFull> {
    const { data } = await instance.get<LdapConfigFull>('/v1/auth/ldap/config')
    return data
}

export async function putLdapConfig(body: LdapConfigFull): Promise<void> {
    await instance.put('/v1/auth/ldap/config', body)
}

export const listLdapRoles = (): Promise<string[]> => listRequest('/v1/auth/ldap/role/')

export async function getLdapRole(name: string): Promise<LdapRoleFull> {
    const { data } = await instance.get<LdapRoleFull>(`/v1/auth/ldap/role/${encodeURIComponent(name)}`)
    return data
}

export async function putLdapRole(name: string, body: LdapRoleBody): Promise<void> {
    await instance.put(`/v1/auth/ldap/role/${encodeURIComponent(name)}`, body)
}

export async function deleteLdapRole(name: string): Promise<void> {
    await instance.delete(`/v1/auth/ldap/role/${encodeURIComponent(name)}`)
}

// ---------- Kubernetes ----------

export interface K8sRoleFull {
    cluster?: string
    namespace: string
    policies: string[]
    service_account: string
    /** Go duration string, e.g. "24h0m0s", or "" */
    ttl: string
    tuck_namespace?: string
}

export interface K8sRoleBody {
    policies: string[]
    ttl?: string
    tuck_namespace?: string
}

/** '' = the default cluster (kubernetes.api in the server config) */
const k8sBase = (cluster: string) =>
    cluster ? `/v1/auth/kubernetes/clusters/${encodeURIComponent(cluster)}/role` : '/v1/auth/kubernetes/role'

export async function listK8sClusters(): Promise<string[]> {
    const { data } = await instance.get<{ clusters?: string[] }>('/v1/auth/kubernetes/clusters')
    return data.clusters ?? []
}

export async function getK8sRole(cluster: string, ns: string, sa: string): Promise<K8sRoleFull> {
    const { data } = await instance.get<K8sRoleFull>(
        `${k8sBase(cluster)}/${encodeURIComponent(ns)}/${encodeURIComponent(sa)}`
    )
    return data
}

export async function putK8sRole(cluster: string, ns: string, sa: string, body: K8sRoleBody): Promise<void> {
    await instance.put(`${k8sBase(cluster)}/${encodeURIComponent(ns)}/${encodeURIComponent(sa)}`, body)
}

export async function deleteK8sRole(cluster: string, ns: string, sa: string): Promise<void> {
    await instance.delete(`${k8sBase(cluster)}/${encodeURIComponent(ns)}/${encodeURIComponent(sa)}`)
}

export const k8sRolePath = (cluster: string, ns: string, sa: string) => `${k8sBase(cluster)}/${ns}/${sa}`
