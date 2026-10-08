import { instance } from '../axios'

export type TuckSecretState = 'deleting' | 'error' | 'pending' | 'synced'

export interface TuckSecretEntry {
    key: string
    path: string
}

export interface TuckSecretCondition {
    type: string
    status: string
    reason?: string
    message?: string
}

export interface TuckSecretView {
    k8s_namespace: string
    name: string
    /** Tuck namespace the Kubernetes namespace is bound to; empty = root / unbound */
    tuck_namespace: string
    secret_name: string
    entries: TuckSecretEntry[]
    kv_version: string
    refresh_interval: string
    deletion_policy: string
    restart_targets?: { kind: string; name: string }[]
    state: TuckSecretState
    last_sync_time?: string
    last_sync_error?: string
    synced_revision?: string
    conditions?: TuckSecretCondition[]
}

export interface TuckSecretsResponse {
    items: TuckSecretView[]
    count: number
    truncated: boolean
}

/** Read-only list of the cluster's TuckSecret resources with their sync status. */
export async function listTuckSecrets(): Promise<TuckSecretsResponse> {
    const { data } = await instance.get<TuckSecretsResponse>('/v1/sys/k8s/tucksecrets')
    return { ...data, items: data.items ?? [] }
}
