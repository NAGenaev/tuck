// SPDX-License-Identifier: AGPL-3.0-only
import { useQueries, useQuery } from '@tanstack/react-query'

import { getPolicy, listPolicies, Policy } from '@shared/api/endpoints/policy'
import { listTokens, lookupToken } from '@shared/api/endpoints/token'

export const LIST_KEY = ['policy', 'list'] as const
export const policyKey = (name: string) => ['policy', 'get', name] as const

export function usePolicyList() {
    return useQuery({ queryFn: listPolicies, queryKey: LIST_KEY })
}

/** Loads every policy body so the list can show rule counts and capability bars. */
export function usePolicyDetails(names: string[]) {
    const results = useQueries({
        queries: names.map((name) => ({
            queryFn: () => getPolicy(name),
            queryKey: policyKey(name),
            retry: false,
            staleTime: 15_000
        }))
    })
    const byName: Record<string, { data?: Policy; error: unknown; isError: boolean; isLoading: boolean }> = {}
    names.forEach((name, i) => {
        const r = results[i]
        byName[name] = { data: r.data, error: r.error, isError: r.isError, isLoading: r.isLoading }
    })
    return byName
}

const TOKEN_SCAN_LIMIT = 100

export interface TokenUsage {
    /** tokens (of those scanned) that reference the policy */
    bound: Array<{ label: string }>
    scanned: number
    total: number
}

/**
 * Which tokens reference a policy. The API has no reverse index, so this lists tokens and looks
 * each one up (first 100 only). Meant to be enabled on demand, e.g. when a delete dialog opens.
 */
export function useTokenUsage(policy: null | string) {
    return useQuery<TokenUsage>({
        enabled: !!policy,
        gcTime: 0,
        queryFn: async () => {
            const ids = await listTokens()
            const slice = ids.slice(0, TOKEN_SCAN_LIMIT)
            const settled = await Promise.allSettled(slice.map((id) => lookupToken(id)))
            const bound: TokenUsage['bound'] = []
            settled.forEach((r, i) => {
                if (r.status === 'fulfilled' && r.value.policies?.includes(policy as string)) {
                    bound.push({ label: r.value.display_name || `${slice[i].slice(0, 8)}…` })
                }
            })
            return { bound, scanned: slice.length, total: ids.length }
        },
        queryKey: ['policy', 'token-usage', policy],
        retry: false
    })
}
