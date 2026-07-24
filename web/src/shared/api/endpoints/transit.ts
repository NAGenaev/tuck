import { instance } from '../axios'
import { listRequest } from '../list'

export type TransitKeyType = 'aes256-gcm96' | 'ecdsa-p256' | 'ed25519' | 'rsa-2048' | 'rsa-4096'

export interface TransitKey {
    name: string
    type: TransitKeyType
    latest_version: number
    min_decryption_version: number
    deletable: boolean
    created_at: string
    versions: Record<string, { created_at: string }>
}

export async function listTransitKeys(): Promise<string[]> {
    return listRequest('/v1/transit/keys/')
}

export async function getTransitKey(name: string): Promise<TransitKey> {
    const { data } = await instance.get<TransitKey>(`/v1/transit/keys/${name}`)
    return data
}

export async function createTransitKey(name: string, type?: TransitKeyType): Promise<TransitKey> {
    const { data } = await instance.post<TransitKey>(`/v1/transit/keys/${name}`, { type })
    return data
}

export async function deleteTransitKey(name: string): Promise<void> {
    await instance.delete(`/v1/transit/keys/${name}`)
}

export async function rotateTransitKey(name: string): Promise<void> {
    await instance.post(`/v1/transit/keys/${name}/rotate`, {})
}

export async function encrypt(name: string, plaintextB64: string): Promise<string> {
    const { data } = await instance.post<{ ciphertext: string }>(`/v1/transit/encrypt/${name}`, {
        plaintext: plaintextB64
    })
    return data.ciphertext
}

export async function decrypt(name: string, ciphertext: string): Promise<string> {
    const { data } = await instance.post<{ plaintext: string }>(`/v1/transit/decrypt/${name}`, {
        ciphertext
    })
    return data.plaintext
}

export async function sign(name: string, inputB64: string): Promise<string> {
    const { data } = await instance.post<{ signature: string }>(`/v1/transit/sign/${name}`, {
        input: inputB64,
        hash_algorithm: 'sha2-256'
    })
    return data.signature
}

export async function verify(name: string, inputB64: string, signature: string): Promise<boolean> {
    const { data } = await instance.post<{ valid: boolean }>(`/v1/transit/verify/${name}`, {
        input: inputB64,
        signature,
        hash_algorithm: 'sha2-256'
    })
    return data.valid
}

export async function hmac(name: string, inputB64: string): Promise<string> {
    const { data } = await instance.post<{ hmac: string }>(`/v1/transit/hmac/${name}`, {
        input: inputB64,
        algorithm: 'sha2-256'
    })
    return data.hmac
}
