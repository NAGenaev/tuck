// SPDX-License-Identifier: AGPL-3.0-only
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { toast } from '@shared/ui/kit'

import { Capsule, LogRow, Op, short, statusOf } from './lib'

/**
 * What this browser session knows about its capsules. Tuck has no "list wrapping tokens"
 * endpoint, so the registry is built only from real calls (wrap, lookup, unwrap, revoke) and
 * lives in memory: tokens are bearer credentials and are never written to storage.
 */
export function useSession() {
    const { t } = useTranslation('wrapping')
    const [capsules, setCapsules] = useState<Record<string, Capsule>>({})
    const [log, setLog] = useState<LogRow[]>([])
    const [cur, setCur] = useState<null | string>(null)
    const [now, setNow] = useState(() => Date.now())
    const seq = useRef(0)
    const noted = useRef(new Set<string>())

    useEffect(() => {
        const id = window.setInterval(() => setNow(Date.now()), 1000)
        return () => window.clearInterval(id)
    }, [])

    const addLog = useCallback((op: Op, token: string, ok: boolean, text: string) => {
        const id = ++seq.current
        setLog((rows) => [{ fresh: true, id, ok, op, t: Date.now(), text, token }, ...rows])
        window.setTimeout(
            () => setLog((rows) => rows.map((r) => (r.id === id ? { ...r, fresh: false } : r))),
            1000
        )
    }, [])

    const upsert = useCallback((token: string, patch: Partial<Capsule>) => {
        setCapsules((all) => {
            const base: Capsule = all[token] ?? { status: 'active', token }
            return { ...all, [token]: { ...base, ...patch, token } }
        })
    }, [])

    // a capsule that outlived its TTL: say so once
    useEffect(() => {
        for (const c of Object.values(capsules)) {
            if (statusOf(c, now) === 'expired' && c.status === 'active' && !noted.current.has(c.token)) {
                noted.current.add(c.token)
                toast.error(t('toast.expired'), t('toast.expiredMsg', { token: short(c.token) }))
                addLog('expire', c.token, false, t('log.expired'))
            }
        }
    }, [now, capsules, addLog, t])

    return { addLog, capsules, cur, log, now, setCur, upsert }
}

export type Session = ReturnType<typeof useSession>
