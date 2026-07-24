import { instance } from '../axios'
import { listRequest } from '../list'

export interface PKIRole {
    name: string
    allowed_domains?: string[]
    allow_subdomains?: boolean
    allow_ip_sans?: boolean
    allow_localhost?: boolean
    key_type?: 'ec' | 'rsa'
    key_bits?: number
    default_ttl?: number
    max_ttl?: number
    server_flag?: boolean
    client_flag?: boolean
}

export interface PKIRolePut {
    allowed_domains?: string[]
    allow_subdomains?: boolean
    key_type?: 'ec' | 'rsa'
    default_ttl?: string
    max_ttl?: string
    server_flag?: boolean
    client_flag?: boolean
}

export interface IssuedCert {
    serial: string
    certificate: string
    private_key: string
    issuing_ca: string
    expires_at: string
    ttl: number
}

export interface CertRecord {
    serial: string
    role_name: string
    common_name: string
    sans?: string[]
    issued_at: string
    expires_at: string
    revoked: boolean
    revoked_at?: string
}

export async function generateRootCA(body: {
    common_name: string
    ttl?: string
    key_type?: 'ec' | 'rsa'
}): Promise<{ certificate: string }> {
    const { data } = await instance.post<{ certificate: string }>('/v1/pki/generate/root', body)
    return data
}

export async function importCA(cert_pem: string, key_pem: string): Promise<void> {
    await instance.post('/v1/pki/import/ca', { cert_pem, key_pem })
}

export async function listPKIRoles(): Promise<string[]> {
    return listRequest('/v1/pki/roles/')
}

export async function putPKIRole(name: string, body: PKIRolePut): Promise<PKIRole> {
    const { data } = await instance.put<PKIRole>(`/v1/pki/roles/${name}`, body)
    return data
}

export async function deletePKIRole(name: string): Promise<void> {
    await instance.delete(`/v1/pki/roles/${name}`)
}

export async function issueCert(
    role: string,
    body: { common_name: string; alt_names?: string[]; ttl?: string }
): Promise<IssuedCert> {
    const { data } = await instance.post<IssuedCert>(`/v1/pki/issue/${role}`, body)
    return data
}

export async function revokeCert(serial: string): Promise<void> {
    await instance.post(`/v1/pki/revoke/${serial}`, {})
}

export async function listCerts(): Promise<string[]> {
    return listRequest('/v1/pki/certs/')
}

export async function getCert(serial: string): Promise<CertRecord> {
    const { data } = await instance.get<CertRecord>(`/v1/pki/certs/${serial}`)
    return data
}
