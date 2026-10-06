import { instance } from '../axios'
import { listRequest } from '../list'

export interface Namespace {
    name: string
    created_at: string
    updated_at?: string
    description?: string
    owner?: string
    labels?: Record<string, string>
}

export interface NamespaceMetadata {
    description?: string
    owner?: string
    labels?: Record<string, string>
}

/**
 * Object counts of a namespace. A field is absent (not zero) when the caller
 * may not list the objects it counts.
 */
export interface NamespaceStats {
    secrets?: number
    secrets_kv1?: number
    secrets_kv2?: number
    tokens?: number
    policies?: number
    auth_roles?: number
    /** a scan limit cut the token count short */
    truncated?: boolean
}

export interface NamespaceView extends Namespace {
    stats?: NamespaceStats
}

export async function listNamespaces(): Promise<string[]> {
    return listRequest('/v1/sys/namespaces/')
}

/** Namespaces with metadata and (with `stats`) object counts. */
export async function listNamespaceDetails(stats = true): Promise<NamespaceView[]> {
    const { data } = await instance.get<{
        keys?: string[]
        namespaces?: NamespaceView[]
    }>('/v1/sys/namespaces/', {
        params: { list: 'true', ...(stats ? { stats: 'true' } : {}) }
    })
    if (data.namespaces) return data.namespaces
    // older server without the detailed list: names only
    return (data.keys ?? []).map((name) => ({ name, created_at: '' }))
}

export async function createNamespace(name: string, meta: NamespaceMetadata = {}): Promise<Namespace> {
    const { data } = await instance.post<Namespace>('/v1/sys/namespaces', {
        name,
        ...meta
    })
    return data
}

/** Replaces description, owner and labels. */
export async function updateNamespace(name: string, meta: NamespaceMetadata): Promise<Namespace> {
    const { data } = await instance.put<Namespace>(`/v1/sys/namespaces/${name}`, meta)
    return data
}

export async function getNamespaceStats(name: string): Promise<NamespaceStats> {
    const { data } = await instance.get<{ stats: NamespaceStats }>(`/v1/sys/namespaces/${name}/stats`)
    return data.stats ?? {}
}

export async function deleteNamespace(name: string): Promise<void> {
    await instance.delete(`/v1/sys/namespaces/${name}`)
}
