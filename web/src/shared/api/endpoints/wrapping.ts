import { instance } from '../axios'

export async function wrap(data: unknown, ttl?: string): Promise<{ token: string; expires_at: string }> {
    const { data: res } = await instance.post<{ token: string; expires_at: string }>(
        '/v1/sys/wrapping/wrap',
        { data, ttl }
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
