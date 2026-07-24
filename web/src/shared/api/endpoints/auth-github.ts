import { instance } from '../axios'
import { listRequest } from '../list'

// NOTE: unlike every other auth engine, GitHub's role.ttl (time.Duration) is
// decoded straight from JSON with no wire-struct conversion — it must be sent
// AND is returned as raw nanoseconds, not a "1h"-style duration string.
export interface GitHubRole {
    name: string
    repository?: string
    repository_owner?: string
    ref?: string
    environment?: string
    workflow_ref?: string
    actor?: string
    audience?: string
    policies: string[]
    ttl: number
}

export type GitHubRolePut = Omit<GitHubRole, 'name'>

export async function listGitHubRoles(): Promise<string[]> {
    return listRequest('/v1/auth/github/role/')
}

export async function getGitHubRole(name: string): Promise<GitHubRole> {
    const { data } = await instance.get<GitHubRole>(`/v1/auth/github/role/${name}`)
    return data
}

export async function putGitHubRole(name: string, body: GitHubRolePut): Promise<void> {
    await instance.put(`/v1/auth/github/role/${name}`, body)
}

export async function deleteGitHubRole(name: string): Promise<void> {
    await instance.delete(`/v1/auth/github/role/${name}`)
}
