import { instance } from '../axios'

export interface ClusterStatus {
    is_leader: boolean
    leader: string
    leader_addr: string
    state: string
    servers: { id: string; address: string; suffrage: string }[]
}

export async function getClusterStatus(): Promise<ClusterStatus> {
    const { data } = await instance.get<ClusterStatus>('/v1/sys/cluster')
    return data
}

export async function joinCluster(nodeId: string, raftAddr: string): Promise<void> {
    await instance.post('/v1/sys/cluster/join', { node_id: nodeId, raft_addr: raftAddr })
}

export async function removeClusterNode(id: string): Promise<void> {
    await instance.delete(`/v1/sys/cluster/node/${id}`)
}
