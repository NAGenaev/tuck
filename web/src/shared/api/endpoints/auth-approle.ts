import { instance } from '../axios'
import { listRequest } from '../list'

export interface AppRoleRole {
    name: string
    role_id: string
    policies: string[]
    secret_id_ttl: number
    secret_id_num_uses: number
    token_ttl: number
    bound_cidrs?: string[]
}

export interface AppRoleRolePut {
    policies: string[]
    token_ttl?: string
    secret_id_ttl?: string
    secret_id_num_uses?: number
}

export interface SecretID {
    id: string
    role_name: string
    created_at: string
    expires_at?: string
    num_uses: number
    uses_left: number
}

export async function listAppRoles(): Promise<string[]> {
    return listRequest('/v1/auth/approle/role/')
}

export async function getAppRole(name: string): Promise<AppRoleRole> {
    const { data } = await instance.get<AppRoleRole>(`/v1/auth/approle/role/${name}`)
    return data
}

export async function putAppRole(name: string, body: AppRoleRolePut): Promise<AppRoleRole> {
    const { data } = await instance.put<AppRoleRole>(`/v1/auth/approle/role/${name}`, body)
    return data
}

export async function deleteAppRole(name: string): Promise<void> {
    await instance.delete(`/v1/auth/approle/role/${name}`)
}

export async function generateSecretId(name: string): Promise<SecretID> {
    const { data } = await instance.post<SecretID>(`/v1/auth/approle/role/${name}/secret-id`, {})
    return data
}
