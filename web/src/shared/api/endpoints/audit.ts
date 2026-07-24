import { instance } from '../axios'

// NOTE: unified list across both sink types, wrapped as {"sinks":[...]} (not
// {"keys":[...]} like every other list endpoint), and typed options are
// stringified inside a map[string]string "options" bag — there is no per-sink GET.
export interface AuditSink {
    name: string
    type: 'file' | 'webhook'
    options: Record<string, string>
    errors: number
}

export async function listAuditSinks(): Promise<AuditSink[]> {
    const { data } = await instance.get<{ sinks: AuditSink[] }>('/v1/sys/audit/', {
        params: { list: 'true' }
    })
    return data.sinks ?? []
}

export async function putWebhookSink(name: string, url: string, timeoutSec?: number): Promise<void> {
    await instance.put(`/v1/sys/audit/webhook/${name}`, { url, timeout_sec: timeoutSec })
}

export async function putFileSink(
    name: string,
    path: string,
    maxSizeMb?: number,
    maxBackups?: number
): Promise<void> {
    await instance.put(`/v1/sys/audit/file/${name}`, {
        path,
        max_size_mb: maxSizeMb,
        max_backups: maxBackups
    })
}

export async function deleteAuditSink(name: string): Promise<void> {
    await instance.delete(`/v1/sys/audit/${name}`)
}
