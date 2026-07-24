import { instance } from '../axios'
import { listRequest } from '../list'

export interface SSHRole {
    name: string
    allowed_users?: string[]
    default_extensions?: Record<string, string>
    cert_type?: 'host' | 'user'
    default_ttl?: number
    max_ttl?: number
}

export interface SSHRolePut {
    allowed_users?: string[]
    cert_type?: 'host' | 'user'
    default_ttl?: string
    max_ttl?: string
}

export interface SignedCert {
    serial: number
    signed_key: string
    valid_after: string
    valid_before: string
    ttl: number
}

export async function generateSSHCA(keyType?: string): Promise<{ public_key: string }> {
    const { data } = await instance.post<{ public_key: string }>('/v1/ssh/generate/ca', {
        key_type: keyType
    })
    return data
}

export async function importSSHCA(privateKey: string): Promise<void> {
    await instance.post('/v1/ssh/import/ca', { private_key: privateKey })
}

export async function listSSHRoles(): Promise<string[]> {
    return listRequest('/v1/ssh/roles/')
}

export async function putSSHRole(name: string, body: SSHRolePut): Promise<SSHRole> {
    const { data } = await instance.put<SSHRole>(`/v1/ssh/roles/${name}`, body)
    return data
}

export async function deleteSSHRole(name: string): Promise<void> {
    await instance.delete(`/v1/ssh/roles/${name}`)
}

export async function signSSHKey(
    role: string,
    body: { public_key: string; valid_principals?: string[]; ttl?: string }
): Promise<SignedCert> {
    const { data } = await instance.post<SignedCert>(`/v1/ssh/sign/${role}`, body)
    return data
}
