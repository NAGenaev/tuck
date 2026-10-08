import { instance } from '../axios'
import type { components } from '../schema'

export type Token = components['schemas']['Token']

export async function lookupSelf(): Promise<Token> {
    const { data } = await instance.get<Token>('/v1/auth/token/lookup-self')
    return data
}

export interface LdapLoginResult {
    expires_at?: string
    policies?: string[]
    token: string
}

export async function loginLdap(username: string, password: string): Promise<LdapLoginResult> {
    const { data } = await instance.post<LdapLoginResult>('/v1/auth/ldap/login', { password, username })
    return data
}

export async function renewSelf(): Promise<Token> {
    const { data } = await instance.post<Token>('/v1/auth/token/renew-self', {})
    return data
}
