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

/** namespace picks one of the user's namespaces (the server answers 409 with the list when it is needed); '' is root. */
export async function loginLdap(username: string, password: string, namespace?: string): Promise<LdapLoginResult> {
    const { data } = await instance.post<LdapLoginResult>('/v1/auth/ldap/login', {
        namespace,
        password,
        username
    })
    return data
}

export async function getOidcStatus(): Promise<{ enabled: boolean }> {
    const { data } = await instance.get<{ enabled: boolean }>('/v1/auth/oidc/status')
    return data
}

export async function getOidcAuthUrl(): Promise<{ state: string; url: string }> {
    const { data } = await instance.get<{ state: string; url: string }>('/v1/auth/oidc/auth-url')
    return data
}

export async function oidcCallback(code: string, state: string): Promise<LdapLoginResult> {
    const { data } = await instance.post<LdapLoginResult>('/v1/auth/oidc/callback', { code, state })
    return data
}

export async function renewSelf(): Promise<Token> {
    const { data } = await instance.post<Token>('/v1/auth/token/renew-self', {})
    return data
}
