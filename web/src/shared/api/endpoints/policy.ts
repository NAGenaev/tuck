import { instance } from '../axios'
import { listRequest } from '../list'

// Hand-written (not from schema.d.ts): the generated openapi.json documents
// `capabilities` as an integer bitmask, but internal/api/policies.go actually
// encodes/decodes it as a string array (["read","write","delete","list","deny"]).
export interface PolicyRule {
    path: string
    capabilities: string[]
}

export interface Policy {
    name: string
    rules: PolicyRule[]
    inheritable?: boolean
}

export async function listPolicies(): Promise<string[]> {
    return listRequest('/v1/policy/')
}

export async function getPolicy(name: string): Promise<Policy> {
    const { data } = await instance.get<Policy>(`/v1/policy/${name}`)
    return data
}

export async function putPolicy(policy: Policy): Promise<void> {
    await instance.put(`/v1/policy/${policy.name}`, policy)
}

export async function deletePolicy(name: string): Promise<void> {
    await instance.delete(`/v1/policy/${name}`)
}
