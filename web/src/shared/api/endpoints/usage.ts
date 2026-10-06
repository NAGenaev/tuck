import { instance } from '../axios'

export interface SecretsUsage {
    /** KV v1 + KV v2 */
    secrets: number
    secrets_kv1: number
    secrets_kv2: number
}

/** Number of KV secrets in the caller's namespace (needs list on secret/). */
export async function getSecretsUsage(): Promise<SecretsUsage> {
    const { data } = await instance.get<SecretsUsage>('/v1/sys/usage/secrets')
    return data
}
