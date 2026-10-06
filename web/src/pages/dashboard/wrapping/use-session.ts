// SPDX-License-Identifier: AGPL-3.0-only
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { toast } from '@shared/ui/kit'

import { Capsule, short, statusOf } from './lib'

/**
 * The wrapping tokens this tab holds, i.e. the ones the user wrapped, looked up, unwrapped or
 * revoked here. A token is a bearer credential, so it lives in memory only and is never stored.
 * The server-side view of live capsules (metadata only, no tokens) is `useWrapped`.
 */
export function useSession() {
    const { t } = useTranslation('wrapping')
    const qc = useQueryClient()
    const [capsules, setCapsules] = useState<Record<string, Capsule>>({})
    const [cur, setCur] = useState<null | string>(null)
    const [now, setNow] = useState(() => Date.now())
    const noted = useRef(new Set<string>())

    useEffect(() => {
        const id = window.setInterval(() => setNow(Date.now()), 1000)
        return () => window.clearInterval(id)
    }, [])

    const upsert = useCallback(
        (token: string, patch: Partial<Capsule>) => {
            setCapsules((all) => {
                const base: Capsule = all[token] ?? { status: 'active', token }
                return { ...all, [token]: { ...base, ...patch, token } }
            })
            // a wrap / unwrap / revoke changed what the server holds: refresh the list and counters
            if ('status' in patch || 'created' in patch) void qc.invalidateQueries({ queryKey: ['wrapping'] })
        },
        [qc]
    )

    // a capsule that outlived its TTL: say so once
    useEffect(() => {
        for (const c of Object.values(capsules)) {
            if (statusOf(c, now) === 'expired' && c.status === 'active' && !noted.current.has(c.token)) {
                noted.current.add(c.token)
                toast.error(t('toast.expired'), t('toast.expiredMsg', { token: short(c.token) }))
                void qc.invalidateQueries({ queryKey: ['wrapping'] })
            }
        }
    }, [now, capsules, qc, t])

    return { capsules, cur, now, setCur, upsert }
}

export type Session = ReturnType<typeof useSession>
