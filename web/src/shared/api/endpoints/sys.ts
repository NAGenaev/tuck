import { instance } from '../axios'
import type { components } from '../schema'

export type SealStatus = components['schemas']['SealStatus']

export async function getSealStatus(): Promise<SealStatus> {
    const { data } = await instance.get<SealStatus>('/v1/sys/seal-status')
    return data
}

export async function unseal(key: string): Promise<SealStatus> {
    const { data } = await instance.post<SealStatus>('/v1/sys/unseal', { key })
    return data
}

export async function seal(): Promise<void> {
    await instance.post('/v1/sys/seal', {})
}

export async function rotate(): Promise<void> {
    await instance.post('/v1/sys/rotate', {})
}

export async function snapshot(): Promise<Blob> {
    const { data } = await instance.get('/v1/sys/snapshot', { responseType: 'blob' })
    return data
}

export interface Health {
    version: string
    commit: string
    build_date: string
    sealed: boolean
    ha_enabled: boolean
    uptime_seconds: number
}

export async function getHealth(): Promise<Health> {
    const { data } = await instance.get<Health>('/v1/health')
    return data
}
