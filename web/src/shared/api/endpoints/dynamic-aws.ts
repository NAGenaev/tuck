import { instance } from '../axios'
import { listRequest } from '../list'

export interface AWSConfig {
    access_key_id: string
    secret_access_key: string
    region: string
}

export interface AWSRolePut {
    credential_type: 'assumed_role' | 'iam_user'
    policy_arns?: string[]
    role_arns?: string[]
    default_ttl?: string
    max_ttl?: string
}

export interface AWSCreds {
    lease_id: string
    access_key_id: string
    secret_access_key: string
    session_token?: string
    expires_at: string
}

export async function getAWSConfig(): Promise<AWSConfig> {
    const { data } = await instance.get<AWSConfig>('/v1/aws/config')
    return data
}

export async function putAWSConfig(body: AWSConfig): Promise<void> {
    await instance.put('/v1/aws/config', body)
}

export async function listAWSRoles(): Promise<string[]> {
    return listRequest('/v1/aws/roles/')
}

export async function putAWSRole(name: string, body: AWSRolePut): Promise<void> {
    await instance.put(`/v1/aws/roles/${name}`, body)
}

export async function deleteAWSRole(name: string): Promise<void> {
    await instance.delete(`/v1/aws/roles/${name}`)
}

export async function generateAWSCreds(role: string): Promise<AWSCreds> {
    const { data } = await instance.post<AWSCreds>(`/v1/aws/creds/${role}`)
    return data
}
