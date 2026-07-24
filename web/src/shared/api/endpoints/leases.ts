import { instance } from '../axios'

// NOTE: unlike every other list endpoint (which return {"keys":[...]}), the
// unified leases list wraps in {"leases": [...full Info objects]}.
export interface LeaseInfo {
    id: string
    backend: string
    internal_id: string
    expires_at: string
    revoked: boolean
}

export async function listLeases(): Promise<LeaseInfo[]> {
    const { data } = await instance.get<{ leases: LeaseInfo[] }>('/v1/sys/leases/', {
        params: { list: 'true' }
    })
    return data.leases ?? []
}

export async function renewLease(leaseId: string, increment?: string): Promise<void> {
    await instance.post('/v1/sys/leases/renew', { lease_id: leaseId, increment })
}

export async function revokeLease(leaseId: string): Promise<void> {
    await instance.delete(`/v1/sys/leases/${leaseId}`)
}
