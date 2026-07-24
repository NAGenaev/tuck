import { instance } from '../axios'
import { listRequest } from '../list'
import type { components } from '../schema'

export type KVv2Response = components['schemas']['KVv2Response']
export type KVMeta = components['schemas']['KVMeta']

export async function listKVv2(prefix: string): Promise<string[]> {
    return listRequest(`/v2/secret/${prefix}`)
}

export async function getKVv2(path: string, version?: number): Promise<KVv2Response> {
    const { data } = await instance.get<KVv2Response>(`/v2/secret/${path}`, {
        params: version ? { version } : undefined
    })
    return data
}

export async function putKVv2(path: string, value: string, cas?: number): Promise<void> {
    await instance.put(`/v2/secret/${path}`, value, {
        params: cas !== undefined ? { cas } : undefined,
        headers: { 'Content-Type': 'text/plain' }
    })
}

export async function softDeleteKVv2(path: string, versions: number[]): Promise<void> {
    await instance.delete(`/v2/secret/${path}`, {
        params: { versions: versions.join(',') }
    })
}

export async function undeleteKVv2(path: string, versions: number[]): Promise<void> {
    await instance.post(`/v2/secret/undelete/${path}`, { versions })
}

export async function destroyKVv2(path: string, versions: number[]): Promise<void> {
    await instance.post(`/v2/secret/destroy/${path}`, { versions })
}

export async function getKVv2Meta(path: string): Promise<KVMeta> {
    const { data } = await instance.get<{ metadata: KVMeta; path: string }>(
        `/v2/secret/metadata/${path}`
    )
    return data.metadata
}
