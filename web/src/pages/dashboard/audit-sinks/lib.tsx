// SPDX-License-Identifier: AGPL-3.0-only
import { TFunction } from 'i18next'
import { ReactNode } from 'react'

import { AuditSink } from '@shared/api/endpoints/audit'

export type SinkType = 'file' | 'webhook'

export interface SinkFields {
    maxBackups: number | string
    maxSizeMb: number | string
    name: string
    path: string
    timeoutSec: number | string
    type: SinkType
    url: string
}

export type SinkErrors = Partial<Record<'backups' | 'name' | 'path' | 'size' | 'timeout' | 'url', string>>

const NAME_RE = /^[a-z0-9][a-z0-9-]{1,31}$/

const isPosInt = (v: number | string, min: number, max = Infinity) => {
    if (v === '' || v === null) return false
    const n = Number(v)
    return Number.isInteger(n) && n >= min && n <= max
}

export function validateSink(f: SinkFields, t: TFunction): SinkErrors {
    const er: SinkErrors = {}
    const name = f.name.trim()
    if (!name) er.name = t('drawer.errNameRequired')
    else if (!NAME_RE.test(name)) er.name = t('drawer.errNameChars')

    if (f.type === 'file') {
        const p = f.path.trim()
        if (!p) er.path = t('drawer.errPathRequired')
        else if (!/^(\/|[A-Za-z]:[\\/])/.test(p)) er.path = t('drawer.errPathAbs')
        if (!isPosInt(f.maxSizeMb, 1)) er.size = t('drawer.errSize')
        if (!isPosInt(f.maxBackups, 1)) er.backups = t('drawer.errBackups')
    } else {
        const u = f.url.trim()
        if (!u) er.url = t('drawer.errUrlRequired')
        else if (!/^https?:\/\/[^\s/]+/i.test(u)) er.url = t('drawer.errUrlFormat')
        if (!isPosInt(f.timeoutSec, 1, 60)) er.timeout = t('drawer.errTimeout')
    }
    return er
}

/** Non-blocking hint: the server accepts plain http, a bank usually should not. */
export function urlWarning(url: string, t: TFunction): string {
    return /^http:\/\//i.test(url.trim()) ? t('drawer.warnHttp') : ''
}

/** The request the form will send, shown as a live preview. */
export function buildBody(f: SinkFields): { json: Record<string, number | string>; url: string } {
    const name = f.name.trim() || '…'
    if (f.type === 'file') {
        return {
            url: `/v1/sys/audit/file/${name}`,
            json: {
                path: f.path.trim() || '/var/log/tuck/audit.log',
                max_size_mb: Number(f.maxSizeMb) || 100,
                max_backups: Number(f.maxBackups) || 7
            }
        }
    }
    return {
        url: `/v1/sys/audit/webhook/${name}`,
        json: {
            url: f.url.trim() || 'https://siem.example.org/tuck/ingest',
            timeout_sec: Number(f.timeoutSec) || 5
        }
    }
}

/** Where the sink delivers to, as stored in its options bag. */
export function sinkTarget(s: AuditSink): string {
    const o = s.options ?? {}
    if (o.url) return o.url
    if (o.path) return o.path
    if (o.address) return o.network ? `${o.network}://${o.address}` : o.address
    return '—'
}

/** One-line settings summary of a sink (rotation, timeout ...). */
export function sinkMeta(s: AuditSink, t: TFunction): string {
    const o = s.options ?? {}
    if (s.type === 'file') {
        const size = Number(o.max_size_mb) || 100
        const backups = Number(o.max_backups) || 7
        return t('meta.rotation', { size, backups })
    }
    if (s.type === 'webhook') return t('meta.timeout', { sec: Number(o.timeout_sec) || 5 })
    return o.facility ? `facility ${o.facility}` : '—'
}

type JsonStyles = Record<string, string>

/** Minimal JSON syntax colouring (keys / strings / numbers / punctuation). */
export function highlightJson(src: string, styles: JsonStyles): ReactNode[] {
    const out: ReactNode[] = []
    const re = /("(?:[^"\\]|\\.)*")(\s*:)?|(-?\b\d+\b)|([{}[\],])/g
    let last = 0
    let m: null | RegExpExecArray
    let k = 0
    while ((m = re.exec(src))) {
        if (m.index > last) out.push(src.slice(last, m.index))
        const [, str, colon, num, punct] = m
        if (str) {
            if (colon) {
                out.push(
                    <span className={styles.jk} key={k++}>
                        {str}
                    </span>,
                    <span className={styles.jp} key={k++}>
                        {colon}
                    </span>
                )
            } else {
                out.push(
                    <span className={styles.js} key={k++}>
                        {str}
                    </span>
                )
            }
        } else if (num) {
            out.push(
                <span className={styles.jn} key={k++}>
                    {num}
                </span>
            )
        } else {
            out.push(
                <span className={styles.jp} key={k++}>
                    {punct}
                </span>
            )
        }
        last = re.lastIndex
    }
    if (last < src.length) out.push(src.slice(last))
    return out
}
