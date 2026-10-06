// SPDX-License-Identifier: AGPL-3.0-only
import { instance } from '@shared/api/axios'
import type { components } from '@shared/api/schema'

export type KvMetaResponse = components['schemas']['KVMetaResponse']
export type KvListEntry = components['schemas']['KVListEntry']
export type KvTotals = components['schemas']['KVTotals']
export type KvAuthor = components['schemas']['KVAuthor']

/** Limits enforced by the server (internal/kvv2). */
export const LIMITS = { customKey: 128, customPairs: 64, customValue: 512, maxVersions: 1000 }

/** PUT /v2/secret/{path}: same call as putKVv2, but keeps the version the server assigned. */
export async function writeSecret(path: string, value: string, cas?: number): Promise<number | undefined> {
    const { data } = await instance.put<{ path?: string; version?: number }>(`/v2/secret/${path}`, value, {
        params: cas !== undefined ? { cas } : undefined,
        headers: { 'Content-Type': 'text/plain' }
    })
    return data?.version
}

/** GET /v2/secret/metadata/{path}: the whole response (metadata + effective cas + mount options). */
export async function getMetaFull(path: string): Promise<KvMetaResponse> {
    const { data } = await instance.get<KvMetaResponse>(`/v2/secret/metadata/${path}`)
    return data
}

export interface MetaPatch {
    cas_required?: boolean
    /** PATCH merges: a null value deletes the key. PUT replaces the whole map. */
    custom_metadata?: Record<string, null | string>
    max_versions?: number
}

/** PATCH /v2/secret/metadata/{path}: only the fields present are changed. */
export async function patchMetadata(path: string, body: MetaPatch): Promise<void> {
    await instance.patch(`/v2/secret/metadata/${path}`, body)
}

/** PUT /v2/secret/metadata/{path}: custom_metadata, when present, replaces the stored map. */
export async function putMetadata(path: string, body: MetaPatch): Promise<void> {
    await instance.put(`/v2/secret/metadata/${path}`, body)
}

export interface DetailedList {
    entries: KvListEntry[]
    totals?: KvTotals
}

/**
 * GET /v2/secret/metadata/{prefix}?list=true&detailed=true[&totals=true].
 * Metadata only, never values. The prefix is part of the URL path.
 */
export async function listDetailed(prefix: string, totals = false): Promise<DetailedList> {
    const { data } = await instance.get<{ entries?: KvListEntry[]; keys?: string[]; totals?: KvTotals }>(
        `/v2/secret/metadata/${prefix}`,
        { params: { detailed: 'true', list: 'true', ...(totals ? { totals: 'true' } : {}) } }
    )
    return { entries: data.entries ?? [], totals: data.totals }
}

/** Totals of the whole mount (root prefix), without per-key entries. */
export async function getTotals(): Promise<KvTotals> {
    const { data } = await instance.get<{ totals?: KvTotals }>('/v2/secret/metadata/', {
        params: { list: 'true', totals: 'true' }
    })
    return data.totals ?? { deleted: 0, destroyed: 0, folders: 0, secrets: 0, versions: 0 }
}

export function errStatus(err: unknown): number | undefined {
    return (err as { response?: { status?: number } })?.response?.status
}

export function errMessage(err: unknown): string {
    const e = err as { message?: string; response?: { data?: { error?: string } } }
    return e?.response?.data?.error ?? e?.message ?? ''
}

/** the server answers 400 {cas_required: true} when a write lacks the cas parameter it demands */
export function isCasRequiredError(err: unknown): boolean {
    return errStatus(err) === 400 && (err as { response?: { data?: { cas_required?: boolean } } })?.response?.data?.cas_required === true
}
