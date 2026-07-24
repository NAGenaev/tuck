import { instance } from '../axios'

export interface MountEntry {
    path: string
    type: string
    accessor: string
    description?: string
    builtin: boolean
    created_at: string
}

export interface MountConfig {
    default_lease_ttl?: number
    max_lease_ttl?: number
    force_no_cache?: boolean
    allowed_response_headers?: string[]
    passthrough_request_headers?: string[]
    description?: string
    updated_at?: string
}

export async function listMounts(): Promise<MountEntry[]> {
    const { data } = await instance.get<{ mounts: MountEntry[] }>('/v1/sys/mounts')
    return data.mounts ?? []
}

export async function createMount(path: string, type: string, description?: string): Promise<MountEntry> {
    const { data } = await instance.post<MountEntry>(`/v1/sys/mounts/${path}`, { type, description })
    return data
}

export async function deleteMount(path: string): Promise<void> {
    await instance.delete(`/v1/sys/mounts/${path}`)
}

export async function getMountConfig(path: string): Promise<MountConfig> {
    const { data } = await instance.get<MountConfig>(`/v1/sys/mounts-tune/${path}`)
    return data
}

export async function putMountConfig(
    path: string,
    body: { default_lease_ttl?: string; max_lease_ttl?: string }
): Promise<void> {
    await instance.post(`/v1/sys/mounts-tune/${path}`, body)
}
