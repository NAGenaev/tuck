import { instance } from '../axios'
import { listRequest } from '../list'

export interface AzureConfig {
    tenant_id: string
    client_id: string
    client_secret: string
}

export interface AzureRolePut {
    application_object_id: string
    application_id: string
    default_ttl?: string
    max_ttl?: string
}

export interface AzureCreds {
    lease_id: string
    tenant_id: string
    client_id: string
    client_secret: string
    expires_at: string
}

export async function getAzureConfig(): Promise<AzureConfig> {
    const { data } = await instance.get<AzureConfig>('/v1/azure/config')
    return data
}

export async function putAzureConfig(body: AzureConfig): Promise<void> {
    await instance.put('/v1/azure/config', body)
}

export async function listAzureRoles(): Promise<string[]> {
    return listRequest('/v1/azure/roles/')
}

export async function putAzureRole(name: string, body: AzureRolePut): Promise<void> {
    await instance.put(`/v1/azure/roles/${name}`, body)
}

export async function deleteAzureRole(name: string): Promise<void> {
    await instance.delete(`/v1/azure/roles/${name}`)
}

export async function generateAzureCreds(role: string): Promise<AzureCreds> {
    const { data } = await instance.post<AzureCreds>(`/v1/azure/creds/${role}`)
    return data
}
