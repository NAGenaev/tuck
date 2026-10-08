import { instance } from '../axios'

export type TeamAccess = 'admin' | 'developer' | 'reader'

export interface TeamK8sBinding {
    /** Trusted cluster name; empty = the default cluster */
    cluster?: string
    namespace: string
    service_account: string
    access: 'developer' | 'reader'
}

export interface TeamRequest {
    name: string
    description?: string
    owner?: string
    groups: Partial<Record<TeamAccess, string[]>>
    k8s?: TeamK8sBinding[]
    /** Go duration, e.g. "1h"; empty = server default */
    ttl?: string
    dry_run?: boolean
}

export interface TeamPlanItem {
    kind: 'k8s_role' | 'ldap_role' | 'namespace' | 'policy'
    name: string
    action: 'conflict' | 'create'
    detail?: string
}

export interface TeamK8sHint {
    cluster?: string
    namespace: string
    annotation: string
}

export interface TeamResult {
    namespace: string
    dry_run: boolean
    items: TeamPlanItem[]
    k8s_hints?: TeamK8sHint[]
}

/** Dry run: validates and lists what would be created. Changes nothing. */
export async function planTeam(req: TeamRequest): Promise<TeamResult> {
    const { data } = await instance.post<TeamResult>('/v1/sys/teams', { ...req, dry_run: true })
    return { ...data, items: data.items ?? [] }
}

/** Creates the namespace, policies and roles in one all-or-nothing step. */
export async function createTeam(req: TeamRequest): Promise<TeamResult> {
    const { data } = await instance.post<TeamResult>('/v1/sys/teams', { ...req, dry_run: false })
    return { ...data, items: data.items ?? [] }
}

export interface PolicySyncItem {
    action: 'unchanged' | 'updated' | 'would_update'
    namespace: string
    policy: string
}

/** Brings the team-admin/developer/reader policies of onboarded teams up to the current templates. */
export async function syncTeamPolicies(dryRun: boolean, name?: string): Promise<PolicySyncItem[]> {
    const { data } = await instance.post<{ policies?: PolicySyncItem[] }>('/v1/sys/teams/sync', {
        dry_run: dryRun,
        name: name || undefined
    })
    return data.policies ?? []
}

export async function listK8sClusters(): Promise<string[]> {
    const { data } = await instance.get<{ clusters?: string[] }>('/v1/auth/kubernetes/clusters')
    return data.clusters ?? []
}
