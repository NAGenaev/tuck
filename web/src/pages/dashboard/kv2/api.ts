// SPDX-License-Identifier: AGPL-3.0-only
import { instance } from '@shared/api/axios'

/** PUT /v2/secret/{path}: same call as putKVv2, but keeps the version the server assigned. */
export async function writeSecret(path: string, value: string, cas?: number): Promise<number | undefined> {
    const { data } = await instance.put<{ path?: string; version?: number }>(`/v2/secret/${path}`, value, {
        params: cas !== undefined ? { cas } : undefined,
        headers: { 'Content-Type': 'text/plain' }
    })
    return data?.version
}

/** PUT /v2/secret/metadata/{path}: body {max_versions}. */
export async function updateMaxVersions(path: string, maxVersions: number): Promise<void> {
    await instance.put(`/v2/secret/metadata/${path}`, { max_versions: maxVersions })
}

export function errStatus(err: unknown): number | undefined {
    return (err as { response?: { status?: number } })?.response?.status
}

export function errMessage(err: unknown): string {
    const e = err as { message?: string; response?: { data?: { error?: string } } }
    return e?.response?.data?.error ?? e?.message ?? ''
}
