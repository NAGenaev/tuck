import { instance } from '../axios'
import { listRequest } from '../list'

export interface Token {
    id: string
    accessor?: string
    display_name?: string
    policies?: string[]
    ttl?: number
    max_ttl?: number
    max_uses?: number
    use_count?: number
    renewable?: boolean
    created_at?: string
    expires_at?: string
}

export interface CreateTokenBody {
    display_name?: string
    policies?: string[]
    ttl?: string
    no_parent?: boolean
}

export async function listTokens(): Promise<string[]> {
    return listRequest('/v1/auth/token/')
}

export async function createToken(body: CreateTokenBody): Promise<Token> {
    const { data } = await instance.post<Token>('/v1/auth/token', body)
    return data
}

export async function lookupToken(id: string): Promise<Token> {
    const { data } = await instance.get<Token>(`/v1/auth/token/${encodeURIComponent(id)}`)
    return data
}

export async function revokeToken(id: string): Promise<void> {
    await instance.delete(`/v1/auth/token/${encodeURIComponent(id)}`)
}
