import { instance } from '../axios'

/**
 * Replication state of one server as seen by the node that answered. Exact for the
 * answering node ("local"); observed through heartbeats for followers, and only
 * when the answering node is the leader.
 */
export interface ClusterServerMetrics {
    applied_index?: number
    commit_index?: number
    failures?: number
    heartbeat_rtt_ms?: number
    lag_entries?: number
    last_contact_ms?: number
    last_log_index: number
    reachable?: boolean
    source: 'heartbeat' | 'local'
    term: number
}

export interface ClusterServer {
    address: string
    id: string
    metrics?: ClusterServerMetrics
    self?: boolean
    suffrage: string
}

/** Certificate this node presents on the Raft transport (absent for plain TCP). */
export interface ClusterTLS {
    ca_certs: number
    days_left?: number
    dns_names?: string[]
    error?: string
    ip_addresses?: string[]
    issuer?: string
    not_after?: string
    not_before?: string
    serial?: string
    server_name?: string
    subject?: string
}

/** One real sample taken by the answering node (every few seconds, ring of 60). */
export interface ClusterSample {
    applied_index: number
    commit_index: number
    is_leader: boolean
    last_log_index: number
    peers?: Record<string, { lag: number; rtt_ms?: number }>
    /** Unix time, milliseconds */
    t: number
    term: number
}

export interface ClusterStatus {
    advertise_addr?: string
    applied_index?: number
    commit_index?: number
    fsm_pending?: number
    heartbeat_timeout_ms?: number
    history?: ClusterSample[]
    is_leader: boolean
    last_contact_ms?: null | number
    last_log_index?: number
    last_log_term?: number
    last_snapshot_index?: number
    last_snapshot_term?: number
    leader: string
    leader_addr: string
    node_id?: string
    servers: ClusterServer[]
    state: string
    term?: number
    tls?: ClusterTLS
}

export interface ClusterSnapshot {
    created_at?: string
    id: string
    index: number
    size: number
    term: number
    voters: number
}

export interface ClusterSnapshotResult {
    created: boolean
    reason?: string
    snapshot?: ClusterSnapshot | null
}

export async function getClusterStatus(): Promise<ClusterStatus> {
    const { data } = await instance.get<ClusterStatus>('/v1/sys/cluster')
    return data
}

export async function joinCluster(nodeId: string, raftAddr: string): Promise<void> {
    await instance.post('/v1/sys/cluster/join', { node_id: nodeId, raft_addr: raftAddr })
}

export interface ClusterJoinToken {
    accessor: string
    advertise_addr?: string
    expires_at: string
    leader_addr: string
    node_id?: string
    num_uses: number
    policy: string
    token: string
    ttl_seconds: number
}

/**
 * Root/sudo only. Mints a short-lived, few-use token scoped to the built-in
 * cluster-join policy (sys/cluster read+write only), so a new node can be joined
 * without handing out the root token. Shown once — the server never returns the
 * raw value again. Omit either field for the server defaults (600s / 1 use).
 */
export async function createClusterJoinToken(opts?: { numUses?: number; ttlSeconds?: number }): Promise<ClusterJoinToken> {
    const { data } = await instance.post<ClusterJoinToken>('/v1/sys/cluster/join-token', {
        num_uses: opts?.numUses,
        ttl_seconds: opts?.ttlSeconds
    })
    return data
}

export async function removeClusterNode(id: string): Promise<void> {
    await instance.delete(`/v1/sys/cluster/node/${id}`)
}

/** Leader only: hands leadership to `targetId`, or to the most up-to-date follower when empty. */
export async function stepDownCluster(targetId?: string): Promise<void> {
    await instance.post('/v1/sys/cluster/step-down', targetId ? { target_id: targetId } : {})
}

export async function listClusterSnapshots(): Promise<ClusterSnapshot[]> {
    const { data } = await instance.get<{ snapshots: ClusterSnapshot[] | null }>('/v1/sys/cluster/snapshot')
    return data.snapshots ?? []
}

export async function createClusterSnapshot(): Promise<ClusterSnapshotResult> {
    const { data } = await instance.post<ClusterSnapshotResult>('/v1/sys/cluster/snapshot')
    return data
}
