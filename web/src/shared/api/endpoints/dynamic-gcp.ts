import { instance } from '../axios'
import { listRequest } from '../list'

export interface GCPConfig {
    credentials_json: string
}

export interface GCPRolePut {
    credential_type: 'access_token' | 'service_account_key'
    service_account_email: string
    default_ttl?: string
    max_ttl?: string
}

export interface GCPCreds {
    lease_id: string
    private_key?: string
    access_token?: string
    token_type?: string
    expires_at: string
}

export async function getGCPConfig(): Promise<GCPConfig> {
    const { data } = await instance.get<GCPConfig>('/v1/gcp/config')
    return data
}

export async function putGCPConfig(body: GCPConfig): Promise<void> {
    await instance.put('/v1/gcp/config', body)
}

export async function listGCPRoles(): Promise<string[]> {
    return listRequest('/v1/gcp/roles/')
}

export async function putGCPRole(name: string, body: GCPRolePut): Promise<void> {
    await instance.put(`/v1/gcp/roles/${name}`, body)
}

export async function deleteGCPRole(name: string): Promise<void> {
    await instance.delete(`/v1/gcp/roles/${name}`)
}

export async function generateGCPCreds(role: string): Promise<GCPCreds> {
    const { data } = await instance.post<GCPCreds>(`/v1/gcp/creds/${role}`)
    return data
}
