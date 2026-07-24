import { instance } from '../axios'
import { listRequest } from '../list'

export interface TOTPKeyInfo {
    name: string
    issuer: string
    account: string
    algorithm: string
    digits: number
    period: number
    skew: number
}

export interface TOTPCreateResult extends TOTPKeyInfo {
    url: string
    secret: string
}

export async function listTOTPKeys(): Promise<string[]> {
    return listRequest('/v1/totp/keys/')
}

export async function getTOTPKey(name: string): Promise<TOTPKeyInfo> {
    const { data } = await instance.get<TOTPKeyInfo>(`/v1/totp/keys/${name}`)
    return data
}

export async function createTOTPKey(
    name: string,
    body: { issuer: string; account: string; digits?: number; period?: number }
): Promise<TOTPCreateResult> {
    const { data } = await instance.post<TOTPCreateResult>(`/v1/totp/keys/${name}`, body)
    return data
}

export async function deleteTOTPKey(name: string): Promise<void> {
    await instance.delete(`/v1/totp/keys/${name}`)
}

export async function generateTOTPCode(name: string): Promise<{ code: string; valid_until: string }> {
    const { data } = await instance.get<{ code: string; valid_until: string }>(
        `/v1/totp/code/${name}`
    )
    return data
}

export async function validateTOTPCode(name: string, code: string): Promise<boolean> {
    const { data } = await instance.post<{ valid: boolean }>(`/v1/totp/code/${name}`, { code })
    return data.valid
}
