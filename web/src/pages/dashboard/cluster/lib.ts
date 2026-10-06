// SPDX-License-Identifier: AGPL-3.0-only
import { isAxiosError } from 'axios'

import type { ClusterSample, ClusterServerMetrics, ClusterStatus } from '@shared/api/endpoints/cluster'

export type NodeRole = 'follower' | 'leader' | 'nonvoter'
export type Health = 'down' | 'electing' | 'nofault' | 'ok'
export type LogLevel = 'e' | 'i' | 'o' | 'v' | 'w'

export interface NodeVM {
    address: string
    id: string
    /** true only when the API node we talk to is the leader and this is it */
    isMe: boolean
    /** replication numbers the answering node really has for this server (absent when it has none) */
    metrics?: ClusterServerMetrics
    role: NodeRole
    suffrage: string
}

export interface ClusterModel {
    health: Health
    /** Raft voters in the configuration */
    voters: number
    /** votes needed for a write */
    quorum: number
    /** how many voters may fail before writes stop */
    tolerated: number
    nodes: NodeVM[]
}

export interface LogEntry {
    id: number
    key: string
    level: LogLevel
    params: Record<string, number | string>
    ts: number
}

export function buildModel(status: ClusterStatus): ClusterModel {
    const servers = status.servers ?? []
    const nodes: NodeVM[] = servers.map((s) => {
        const isLeader = !!status.leader && s.id === status.leader
        return {
            address: s.address,
            id: s.id,
            isMe: isLeader && status.is_leader,
            metrics: s.metrics,
            role: isLeader ? 'leader' : s.suffrage === 'Nonvoter' ? 'nonvoter' : 'follower',
            suffrage: s.suffrage
        }
    })
    const voters = servers.filter((s) => s.suffrage !== 'Nonvoter').length
    const quorum = Math.floor(voters / 2) + 1
    const tolerated = Math.max(0, voters - quorum)

    let health: Health = 'ok'
    if (status.state === 'Shutdown') health = 'down'
    else if (!status.leader || status.state === 'Candidate') health = 'electing'
    else if (tolerated < 1) health = 'nofault'

    return { health, nodes, quorum, tolerated, voters }
}

/** Order for display: leader first, then by id. */
export function sortNodes(nodes: NodeVM[]): NodeVM[] {
    return [...nodes].sort((a, b) => Number(b.role === 'leader') - Number(a.role === 'leader') || a.id.localeCompare(b.id))
}

/** Position of node i of n on the topology ring (viewBox 760x440). */
export function ringPos(i: number, n: number) {
    const a = ((-90 + (i * 360) / n) * Math.PI) / 180
    return { x: 380 + 270 * Math.cos(a), y: 215 + 140 * Math.sin(a) }
}

/** Events visible by comparing two consecutive polls of the cluster configuration. */
export function diffStatus(prev: ClusterStatus | null, next: ClusterStatus): Array<Omit<LogEntry, 'id' | 'ts'>> {
    const out: Array<Omit<LogEntry, 'id' | 'ts'>> = []
    const nextServers = next.servers ?? []
    if (!prev) {
        out.push({
            key: 'connected',
            level: 'i',
            params: { count: nextServers.length, leader: next.leader || '—' }
        })
        return out
    }
    const prevServers = prev.servers ?? []
    if (prev.leader !== next.leader) {
        if (!next.leader) out.push({ key: 'leaderLost', level: 'w', params: { id: prev.leader } })
        else if (!prev.leader) out.push({ key: 'leaderElected', level: 'o', params: { id: next.leader } })
        else out.push({ key: 'leaderChanged', level: 'v', params: { from: prev.leader, to: next.leader } })
    }
    if (prev.term !== undefined && next.term !== undefined && next.term > prev.term) {
        out.push({ key: 'termChanged', level: 'v', params: { from: prev.term, to: next.term } })
    }
    if (prev.state !== next.state) {
        out.push({
            key: 'stateChanged',
            level: next.state === 'Shutdown' ? 'e' : 'i',
            params: { from: prev.state, to: next.state }
        })
    }
    for (const s of nextServers) {
        const was = prevServers.find((p) => p.id === s.id)
        if (!was) out.push({ key: 'nodeJoined', level: 'o', params: { addr: s.address, id: s.id } })
        else if (was.suffrage !== s.suffrage)
            out.push({ key: 'suffrageChanged', level: 'i', params: { id: s.id, to: s.suffrage } })
    }
    for (const p of prevServers) {
        if (!nextServers.some((s) => s.id === p.id)) out.push({ key: 'nodeLeft', level: 'w', params: { id: p.id } })
    }
    return out
}

export function httpStatus(err: unknown): number | undefined {
    return isAxiosError(err) ? err.response?.status : undefined
}

export function serverMessage(err: unknown): string | undefined {
    if (isAxiosError(err)) {
        const d = err.response?.data as { error?: string; leader_addr?: string } | undefined
        if (d?.error) return d.leader_addr ? `${d.error} (${d.leader_addr})` : d.error
    }
    return undefined
}

export const ADDR_RE = /^[a-z0-9.[\]:-]+:\d{2,5}$/i

export function pad2(n: number) {
    return String(n).padStart(2, '0')
}

export function fmtTime(ts: number) {
    const d = new Date(ts)
    return `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
}

export function fmtBytes(n: number, locale: string) {
    const units = ['B', 'KB', 'MB', 'GB']
    let v = n
    let i = 0
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024
        i++
    }
    return `${v.toLocaleString(locale, { maximumFractionDigits: i === 0 ? 0 : 1 })} ${units[i]}`
}

// ---------------------------------------------------------------------------
// Quorum arithmetic (pure calculation over the real peer list, no cluster call)
// ---------------------------------------------------------------------------

export interface LossResult {
    /** voters still up in the scenario */
    alive: number
    hasQuorum: boolean
    quorum: number
    /** how many more voters could fail before quorum is lost (negative = already lost) */
    spare: number
    total: number
}

/** What the voter majority looks like if the voters in `down` were gone. Non-voters never count. */
export function simulateLoss(nodes: NodeVM[], down: ReadonlySet<string>): LossResult {
    const voters = nodes.filter((n) => n.suffrage !== 'Nonvoter')
    const total = voters.length
    const quorum = Math.floor(total / 2) + 1
    const alive = voters.filter((n) => !down.has(n.id)).length
    return { alive, hasQuorum: alive >= quorum, quorum, spare: alive - quorum, total }
}

// ---------------------------------------------------------------------------
// Series for the sparklines: every point is a real sample from the history ring
// ---------------------------------------------------------------------------

/** Committed entries per second between consecutive samples (negative deltas are skipped). */
export function commitRate(history: ClusterSample[]): number[] {
    const out: number[] = []
    for (let i = 1; i < history.length; i++) {
        const dt = (history[i].t - history[i - 1].t) / 1000
        const di = history[i].commit_index - history[i - 1].commit_index
        if (dt > 0 && di >= 0) out.push(di / dt)
    }
    return out
}

/** Worst follower lag (entries) in each sample taken while this node was the leader. */
export function maxLagSeries(history: ClusterSample[]): number[] {
    return history
        .filter((h) => h.is_leader && h.peers)
        .map((h) => Math.max(0, ...Object.values(h.peers ?? {}).map((p) => p.lag)))
}

/** Heartbeat round trip (ms) to one follower, from the samples that have it. */
export function peerRttSeries(history: ClusterSample[], id: string): number[] {
    return history.map((h) => h.peers?.[id]?.rtt_ms).filter((v): v is number => typeof v === 'number')
}

export function peerLagSeries(history: ClusterSample[], id: string): number[] {
    return history.filter((h) => h.peers?.[id]).map((h) => h.peers?.[id]?.lag ?? 0)
}

export function fmtMs(ms: number) {
    return ms < 10 ? ms.toFixed(1) : String(Math.round(ms))
}

/** Age of the last contact: "320 ms", "4.2 s", "3 min". */
export function fmtAge(ms: number) {
    if (ms < 1000) return `${Math.round(ms)} ms`
    if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`
    return `${Math.round(ms / 60_000)} min`
}

export type CertTone = 'bad' | 'brass' | 'mute' | 'ok'

export function certTone(daysLeft: number | undefined): CertTone {
    if (daysLeft === undefined) return 'mute'
    if (daysLeft < 0) return 'bad'
    if (daysLeft < 14) return 'brass'
    return 'ok'
}

/** Slowest follower heartbeat round trip (ms) in each sample that has any. */
export function maxRttSeries(history: ClusterSample[]): number[] {
    const out: number[] = []
    for (const h of history) {
        const v = Object.values(h.peers ?? {})
            .map((p) => p.rtt_ms)
            .filter((x): x is number => typeof x === 'number')
        if (v.length) out.push(Math.max(...v))
    }
    return out
}
