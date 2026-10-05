// SPDX-License-Identifier: AGPL-3.0-only
// Token endpoints the shared client does not cover yet (extended create body, renew, lookup-self).
// All of them exist on the server (internal/api/tokens.go).
import { isAxiosError } from 'axios'

import { instance } from '@shared/api/axios'
import type { Token } from '@shared/api/endpoints/token'

export interface TokenInfo extends Token {
    namespace?: string
    parent_id?: string
    orphan?: boolean
    period?: number
}

export interface CreateTokenInput {
    display_name: string
    policies: string[]
    /** Go duration, e.g. "86400s" */
    ttl: string
    renewable: boolean
    max_uses: number
    no_parent: boolean
}

export async function createTokenFull(body: CreateTokenInput): Promise<TokenInfo> {
    const { data } = await instance.post<TokenInfo>('/v1/auth/token', body)
    return data
}

export async function lookupTokenInfo(id: string): Promise<TokenInfo> {
    const { data } = await instance.get<TokenInfo>(`/v1/auth/token/${encodeURIComponent(id)}`)
    return data
}

export async function renewTokenById(id: string, ttlSeconds?: number): Promise<TokenInfo> {
    const { data } = await instance.post<TokenInfo>(
        `/v1/auth/token/${encodeURIComponent(id)}/renew`,
        ttlSeconds ? { ttl: `${ttlSeconds}s` } : {}
    )
    return data
}

export async function lookupSelf(): Promise<TokenInfo> {
    const { data } = await instance.get<TokenInfo>('/v1/auth/token/lookup-self')
    return data
}

export async function renewSelf(ttlSeconds?: number): Promise<TokenInfo> {
    const { data } = await instance.post<TokenInfo>(
        '/v1/auth/token/renew-self',
        ttlSeconds ? { ttl: `${ttlSeconds}s` } : {}
    )
    return data
}

/** HTTP status of a failed request, if any */
export const httpStatus = (e: unknown): number | undefined => (isAxiosError(e) ? e.response?.status : undefined)

/** Server-provided error text of a failed request, if any */
export function apiError(e: unknown): string | undefined {
    if (!isAxiosError(e)) return undefined
    const d = e.response?.data as { error?: unknown } | undefined
    return typeof d?.error === 'string' ? d.error : undefined
}
