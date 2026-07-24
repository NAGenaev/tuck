import { instance } from '../axios'
import { listRequest } from '../list'

export interface JWTConfig {
    jwks_uri: string
    issuer?: string
    audience?: string
    default_ttl?: number
}

export interface JWTConfigPut {
    jwks_uri: string
    issuer?: string
    audience?: string
    default_ttl?: string
}

export interface JWTRole {
    name: string
    bound_subject?: string
    bound_claims?: Record<string, string>
    bound_audiences?: string[]
    policies: string[]
    ttl: number
}

export interface JWTRolePut {
    policies: string[]
    bound_subject?: string
    bound_audiences?: string[]
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
