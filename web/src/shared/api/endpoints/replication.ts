import { instance } from '../axios'

export interface ReplicationState {
    mode: 'disabled' | 'primary' | 'secondary'
    last_sequence: number
    updated_at: string
    primary_addr?: string
}

export interface WALEntry {
    seq: number
    timestamp: string
    operation: 'delete' | 'put'
    key: string
    value?: string
}

export async function getReplicationStatus(): Promise<ReplicationState> {
    const { data } = await instance.get<ReplicationState>('/v1/sys/replication/status')
    return data
}

export async function enablePrimary(): Promise<void> {
    await instance.post('/v1/sys/replication/primary/enable', {})
}

export async function enableSecondary(primaryAddr: string): Promise<void> {
    await instance.post('/v1/sys/replication/secondary/enable', { primary_addr: primaryAddr })
}

export async function disableReplication(): Promise<void> {
    await instance.post('/v1/sys/replication/disable', {})
}

export async function getWALEntries(after = 0): Promise<WALEntry[]> {
    const { data } = await instance.get<{ entries: WALEntry[] }>('/v1/sys/replication/wal', {
        params: { after }
    })
    return data.entries ?? []
}
