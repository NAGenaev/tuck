import { instance } from '../axios'

// No LIST endpoint exists for Kubernetes roles — they're looked up directly
// by namespace + service account, not browsed.
export interface K8sRole {
    namespace: string
    service_account: string
    policies: string[]
    ttl: string
}

export interface K8sRolePut {
    policies: string[]
    ttl?: string
}

export async function getK8sRole(namespace: string, sa: string): Promise<K8sRole> {
    const { data } = await instance.get<K8sRole>(`/v1/auth/kubernetes/role/${namespace}/${sa}`)
    return data
}

export async function putK8sRole(namespace: string, sa: string, body: K8sRolePut): Promise<void> {
    await instance.put(`/v1/auth/kubernetes/role/${namespace}/${sa}`, body)
}

export async function deleteK8sRole(namespace: string, sa: string): Promise<void> {
    await instance.delete(`/v1/auth/kubernetes/role/${namespace}/${sa}`)
}
