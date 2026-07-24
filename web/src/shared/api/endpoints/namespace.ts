import { instance } from '../axios'
import { listRequest } from '../list'

export interface Namespace {
    name: string
    created_at: string
}

export async function listNamespaces(): Promise<string[]> {
    return listRequest('/v1/sys/namespaces/')
}

export async function createNamespace(name: string): Promise<Namespace> {
    const { data } = await instance.post<Namespace>('/v1/sys/namespaces', { name })
    return data
}

export async function deleteNamespace(name: string): Promise<void> {
    await instance.delete(`/v1/sys/namespaces/${name}`)
}
