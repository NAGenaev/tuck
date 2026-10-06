// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from '@tanstack/react-query'
import { isAxiosError } from 'axios'

import { getWrappingStats, listWrapped } from '@shared/api/endpoints/wrapping'

/** How often the server-side list and counters are re-read. */
export const WRAPPED_POLL = 10_000

/**
 * What the server holds: live wrapped responses (metadata only) and lifetime counters.
 * The counters need read capability on sys/wrapping/stats; without it `statsDenied` is set
 * and the page shows only what the list itself proves.
 */
export function useWrapped() {
    const list = useQuery({
        queryFn: listWrapped,
        queryKey: ['wrapping', 'list'],
        refetchInterval: WRAPPED_POLL,
        retry: false
    })
    const stats = useQuery({
        queryFn: getWrappingStats,
        queryKey: ['wrapping', 'stats'],
        refetchInterval: WRAPPED_POLL,
        retry: false
    })
    const statsDenied = isAxiosError(stats.error) && stats.error.response?.status === 403
    return {
        capsules: list.data?.capsules ?? [],
        error: list.isError,
        loading: list.isLoading,
        refetch: () => Promise.all([list.refetch(), stats.refetch()]),
        refreshing: list.isFetching || stats.isFetching,
        scope: list.data?.scope,
        stats: stats.data,
        statsDenied,
        updatedAt: Math.max(list.dataUpdatedAt, 0)
    }
}

export type Wrapped = ReturnType<typeof useWrapped>
