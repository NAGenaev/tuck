import { instance } from '../axios'
import { listRequest } from '../list'

export interface JWTConfig {
    jwks_uri: string
    issuer?: string
    audience?: string
    default_ttl?: number
    oidc_authorization_url?: string
    oidc_token_url?: string
    oidc_client_id?: string
    oidc_client_secret_set?: boolean
    oidc_redirect_uri?: string
    oidc_scopes?: string
}

export interface JWTConfigPut {
    jwks_uri: string
    issuer?: string
    audience?: string
    default_ttl?: string
    oidc_authorization_url?: string
    oidc_token_url?: string
    oidc_client_id?: string
    /** omitted keeps the stored secret, '' clears it */
    oidc_client_secret?: string
    oidc_redirect_uri?: string
    oidc_scopes?: string
}

export interface JWTRole {
    name: string
    bound_subject?: string
    bound_claims?: Record<string, string>
    bound_audiences?: string[]
    bound_groups?: string[]
    policies: string[]
    ttl: number
}

export interface JWTRolePut {
    policies: string[]
    bound_subject?: string
    bound_audiences?: string[]
    bound_groups?: string[]
    ttl?: string
}

export async function getJWTConfig(): Promise<JWTConfig> {
    const { data } = await instance.get<JWTConfig>('/v1/auth/jwt/config')
    return data
}

export async function putJWTConfig(body: JWTConfigPut): Promise<void> {
    await instance.put('/v1/auth/jwt/config', body)
}

export async function listJWTRoles(): Promise<string[]> {
    return listRequest('/v1/auth/jwt/role/')
}

export async function getJWTRole(name: string): Promise<JWTRole> {
    const { data } = await instance.get<JWTRole>(`/v1/auth/jwt/role/${name}`)
    return data
}

export async function putJWTRole(name: string, body: JWTRolePut): Promise<void> {
    await instance.put(`/v1/auth/jwt/role/${name}`, body)
}

export async function deleteJWTRole(name: string): Promise<void> {
    await instance.delete(`/v1/auth/jwt/role/${name}`)
}
