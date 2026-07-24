import { instance } from '../axios'
import { listRequest } from '../list'

export interface DBConfig {
    plugin_name: string
    connection_url: string
    max_open_conns?: number
}

export interface DBRole {
    name: string
    db_name: string
    creation_statements?: string
    revocation_statements?: string
    default_ttl?: number
    max_ttl?: number
}

export interface DBRolePut {
    db_name: string
    creation_statements?: string
    revocation_statements?: string
    default_ttl?: string
    max_ttl?: string
}

export interface DBCredentials {
    lease_id: string
    username: string
    password: string
    expires_at: string
    ttl: number
}

export async function listDBConfigs(): Promise<string[]> {
    return listRequest('/v1/database/config/')
}

export async function getDBConfig(name: string): Promise<DBConfig> {
    const { data } = await instance.get<DBConfig>(`/v1/database/config/${name}`)
    return data
}

export async function putDBConfig(name: string, body: DBConfig): Promise<void> {
    await instance.put(`/v1/database/config/${name}`, body)
}

export async function deleteDBConfig(name: string): Promise<void> {
    await instance.delete(`/v1/database/config/${name}`)
}

export async function listDBRoles(): Promise<string[]> {
    return listRequest('/v1/database/role/')
}

export async function putDBRole(name: string, body: DBRolePut): Promise<void> {
    await instance.put(`/v1/database/role/${name}`, body)
}

export async function deleteDBRole(name: string): Promise<void> {
    await instance.delete(`/v1/database/role/${name}`)
}

export async function generateDBCreds(role: string): Promise<DBCredentials> {
    const { data } = await instance.post<DBCredentials>(`/v1/database/creds/${role}`)
    return data
}
