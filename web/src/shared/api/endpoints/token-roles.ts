import { instance } from '../axios'
import { listRequest } from '../list'
import type { Token } from './token'

export interface TokenRole {
    name: string
    policies?: string[]
    ttl: number
    max_ttl: number
    max_uses?: number
    renewable?: boolean
    period?: number
    created_at?: string
    updated_at?: string
}

export interface TokenRolePut {
    policies?: string[]
    ttl?: string
    max_ttl?: string
    max_uses?: number
    renewable?: boolean
    period?: string
}

export async function listTokenRoles(): Promise<string[]> {
    return listRequest('/v1/auth/token/roles/')
}

export async function getTokenRole(name: string): Promise<TokenRole> {
    const { data } = await instance.get<TokenRole>(`/v1/auth/token/roles/${name}`)
    return data
}

export async function putTokenRole(name: string, body: TokenRolePut): Promise<void> {
    await instance.put(`/v1/auth/token/roles/${name}`, body)
}

export async function deleteTokenRole(name: string): Promise<void> {
    await instance.delete(`/v1/auth/token/roles/${name}`)
}

export async function createTokenFromRole(role: string, displayName?: string): Promise<Token> {
    const { data } = await instance.post<Token>(`/v1/auth/token/roles/${role}/create`, {
        display_name: displayName
    })
    return data
}
