import { instance } from '../axios'
import { listRequest } from '../list'
import type { components } from '../schema'

export type KVv1Response = components['schemas']['KVv1Response']

export async function listKVv1(prefix: string): Promise<string[]> {
    return listRequest(`/v1/secret/${prefix}`)
}

export async function getKVv1(path: string): Promise<KVv1Response> {
    const { data } = await instance.get<KVv1Response>(`/v1/secret/${path}`)
    return data
}

export async function putKVv1(path: string, value: string): Promise<void> {
    await instance.put(`/v1/secret/${path}`, value, {
        headers: { 'Content-Type': 'text/plain' }
    })
}

export async function deleteKVv1(path: string): Promise<void> {
    await instance.delete(`/v1/secret/${path}`)
}
