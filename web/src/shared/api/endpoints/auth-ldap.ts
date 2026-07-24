import { instance } from '../axios'
import { listRequest } from '../list'

export interface LDAPConfig {
    urls: string[]
    bind_dn?: string
    bind_password?: string
    user_dn: string
    user_attr?: string
    group_dn?: string
    group_attr?: string
}

export interface LDAPRole {
    name: string
    groups?: string[]
    users?: string[]
    policies: string[]
    ttl: number
}

export interface LDAPRolePut {
    groups?: string[]
    users?: string[]
    policies: string[]
    ttl?: string
}

export async function getLDAPConfig(): Promise<LDAPConfig> {
    const { data } = await instance.get<LDAPConfig>('/v1/auth/ldap/config')
    return data
}

export async function putLDAPConfig(body: LDAPConfig): Promise<void> {
    await instance.put('/v1/auth/ldap/config', body)
}

export async function listLDAPRoles(): Promise<string[]> {
    return listRequest('/v1/auth/ldap/role/')
}

export async function getLDAPRole(name: string): Promise<LDAPRole> {
    const { data } = await instance.get<LDAPRole>(`/v1/auth/ldap/role/${name}`)
    return data
}

export async function putLDAPRole(name: string, body: LDAPRolePut): Promise<void> {
    await instance.put(`/v1/auth/ldap/role/${name}`, body)
}

export async function deleteLDAPRole(name: string): Promise<void> {
    await instance.delete(`/v1/auth/ldap/role/${name}`)
}
