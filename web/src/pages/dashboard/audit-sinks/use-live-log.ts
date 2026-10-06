// SPDX-License-Identifier: AGPL-3.0-only
import { useCallback, useEffect, useRef, useState } from 'react'

import { AuditEvent, AuditFilter, auditStreamUrl } from '@shared/api/endpoints/audit'
import { getNamespace, getToken } from '@shared/auth/token'

/** Rows kept in memory; older rows are dropped once the cap is hit. */
export const LIVE_LOG_CAP = 500
const RECONNECT_MS = 3000

export type LiveLogStatus = 'connecting' | 'live' | 'offline' | 'paused'

interface SseEvent {
    data: string
    event: string
    id?: string
}

/** Splits a raw SSE byte stream into {event,data,id} records; "comment" lines (heartbeats) are dropped. */
function parseBlock(block: string): SseEvent | null {
    let event = 'message'
    let id: string | undefined
    const data: string[] = []
    for (const raw of block.split('\n')) {
        if (!raw || raw.startsWith(':')) continue
        const idx = raw.indexOf(':')
        const field = idx === -1 ? raw : raw.slice(0, idx)
        const value = idx === -1 ? '' : raw.slice(idx + 1).replace(/^ /, '')
        if (field === 'event') event = value
        else if (field === 'id') id = value
        else if (field === 'data') data.push(value)
    }
    if (data.length === 0 && event === 'message') return null
    return { event, data: data.join('\n'), id }
}

/**
 * Live tail of GET /v1/sys/audit/events/stream via fetch + a manual reader
 * (no EventSource: it cannot send the X-Tuck-Token header). Reconnects with
 * since_seq set to the last seen sequence number so a pause/filter-preserving
 * reconnect never re-plays history and never silently drops a gap either
 * (the "gap" server event is surfaced as a banner by the caller).
 */
export function useLiveLog(filter: AuditFilter, enabled: boolean) {
    const [rows, setRows] = useState<AuditEvent[]>([])
    const [status, setStatus] = useState<LiveLogStatus>('connecting')
    const [gap, setGap] = useState(false)
    const [paused, setPaused] = useState(false)
    const lastSeq = useRef<number | undefined>(undefined)
    const abortRef = useRef<AbortController | null>(null)
    const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
    const filterKey = JSON.stringify(filter)

    const stop = useCallback(() => {
        abortRef.current?.abort()
        abortRef.current = null
        if (timerRef.current) {
            clearTimeout(timerRef.current)
            timerRef.current = null
        }
    }, [])

    const connect = useCallback(() => {
        stop()
        if (!enabled || paused) return
        const controller = new AbortController()
        abortRef.current = controller
        setStatus('connecting')

        const url = auditStreamUrl(JSON.parse(filterKey), lastSeq.current)
        const headers: Record<string, string> = { Accept: 'text/event-stream' }
        const token = getToken()
        if (token) headers['X-Tuck-Token'] = token
        const ns = getNamespace()
        if (ns) headers['X-Tuck-Namespace'] = ns

        fetch(url, { headers, signal: controller.signal })
            .then(async (res) => {
                if (!res.ok || !res.body) throw new Error(`stream ${res.status}`)
                setStatus('live')
                const reader = res.body.getReader()
                const decoder = new TextDecoder()
                let buf = ''
                for (;;) {
                    const { done, value } = await reader.read()
                    if (done) break
                    buf += decoder.decode(value, { stream: true })
                    let idx: number
                    // eslint-disable-next-line no-cond-assign
                    while ((idx = buf.indexOf('\n\n')) !== -1) {
                        const block = buf.slice(0, idx)
                        buf = buf.slice(idx + 2)
                        const ev = parseBlock(block)
                        if (!ev) continue
                        if (ev.event === 'audit') {
                            try {
                                const entry = JSON.parse(ev.data) as AuditEvent
                                if (entry.seq) lastSeq.current = entry.seq
                                setRows((prev) => [entry, ...prev].slice(0, LIVE_LOG_CAP))
                            } catch {
                                // ignore a malformed line rather than killing the stream
                            }
                        } else if (ev.event === 'gap') {
                            setGap(true)
                        } else if (ev.event === 'hello') {
                            try {
                                const hello = JSON.parse(ev.data) as { head_seq?: number }
                                if (lastSeq.current === undefined && hello.head_seq) lastSeq.current = hello.head_seq
                            } catch {
                                // ignore
                            }
                        }
                    }
                }
                throw new Error('stream closed')
            })
            .catch(() => {
                if (controller.signal.aborted) return
                setStatus('offline')
                timerRef.current = setTimeout(connect, RECONNECT_MS)
            })
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [enabled, paused, filterKey, stop])

    useEffect(() => {
        if (!enabled || paused) {
            stop()
            setStatus(paused ? 'paused' : 'offline')
            return
        }
        connect()
        return stop
        // reconnect fresh (no replay) whenever the filter changes
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [enabled, paused, filterKey])

    const togglePause = useCallback(() => setPaused((p) => !p), [])
    const clearGap = useCallback(() => setGap(false), [])
    const clearRows = useCallback(() => setRows([]), [])

    return { rows, status, paused, togglePause, gap, clearGap, clearRows }
}
