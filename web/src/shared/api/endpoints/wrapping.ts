import { instance } from '../axios'

export async function wrap(
    data: unknown,
    ttl?: string,
    creationPath?: string
): Promise<{ token: string; expires_at: string }> {
    const { data: res } = await instance.post<{ token: string; expires_at: string }>(
        '/v1/sys/wrapping/wrap',
        { creation_path: creationPath || undefined, data, ttl }
    )
    return res
}

export async function unwrap(token: string): Promise<unknown> {
    const { data } = await instance.post<{ data: unknown }>('/v1/sys/wrapping/unwrap', { token })
    return data.data
}

export interface WrapLookup {
    creation_time: string
    expires_at: string
    creation_ttl: number
}

export async function lookupWrap(token: string): Promise<WrapLookup> {
    const { data } = await instance.post<WrapLookup>('/v1/sys/wrapping/lookup', { token })
    return data
}

export async function revokeWrap(token: string): Promise<void> {
    await instance.delete('/v1/sys/wrapping/revoke', { data: { token } })
}

/** Metadata of a live wrapped response. Never carries the token or the payload. */
export interface WrappedCapsule {
    /** listing / revoke handle; it cannot be used to unwrap */
    accessor: string
    creation_time: string
    creation_ttl: number
    creator_accessor?: string
    creator_fingerprint?: string
    creator_name?: string
    creation_path?: string
    expires_at: string
    namespace: string
    /** payload size, bytes */
    size: number
}

export interface WrappedList {
    capsules: WrappedCapsule[]
    /** "own": only capsules created by the caller; "all": the caller may see everyone's */
    scope: 'all' | 'own'
}

export async function listWrapped(): Promise<WrappedList> {
    const { data } = await instance.get<WrappedList>('/v1/sys/wrapping/list')
    return { capsules: data.capsules ?? [], scope: data.scope }
}

export interface WrappingStats {
    /** live capsules right now */
    active: number
    expired: number
    revoked: number
    since: string
    unwrapped: number
    wrapped: number
}

/** Lifetime counters. 403 for tokens without read capability on sys/wrapping/stats. */
export async function getWrappingStats(): Promise<WrappingStats> {
    const { data } = await instance.get<WrappingStats>('/v1/sys/wrapping/stats')
    return data
}

export async function revokeWrapByAccessor(accessor: string): Promise<void> {
    await instance.delete(`/v1/sys/wrapping/revoke-accessor/${encodeURIComponent(accessor)}`)
}
