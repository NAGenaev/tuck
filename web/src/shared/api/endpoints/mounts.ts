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

/**
 * Usage counters of a mount for the calling namespace. A field is absent when
 * it does not apply to the mount or the caller may not list what it counts.
 */
export interface MountStats {
    /** false for a mount-table-only entry: nothing is stored under it */
    engine_backed: boolean
    secrets?: number
    leases?: number
    tokens?: number
    secrets_kv1?: number
    secrets_kv2?: number
    versions?: number
    deleted_versions?: number
    destroyed_versions?: number
    hidden_secrets?: number
    last_write_at?: string
    truncated?: boolean
    roles?: number
    connections?: number
    issued_certs?: number
}

export interface MountDetails {
    config: MountConfig
    stats?: MountStats
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
    body: { default_lease_ttl?: string; force_no_cache?: boolean; max_lease_ttl?: string }
): Promise<void> {
    await instance.post(`/v1/sys/mounts-tune/${path}`, body)
}

/** Tune config together with the usage counters of the mount. */
export async function getMountDetails(path: string): Promise<MountDetails> {
    const { data } = await instance.get<MountConfig & { stats?: MountStats }>(`/v1/sys/mounts-tune/${path}`, {
        params: { stats: 'true' }
    })
    const { stats, ...config } = data
    return { config, stats }
}
