// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from '@tanstack/react-query'

import { getKVv2 } from '@shared/api/endpoints/kvv2'

import { errStatus, getMetaFull, getTotals, listDetailed } from './api'

// Query keys are the same as the old page used, so invalidations stay compatible.
// ['kv2','list',prefix] holds the detailed list of one level, ['kv2','list','totals'] the
// totals of the whole mount (a prefix always ends with "/" or is empty, so no key can collide).
export function useKv2List(prefix: string) {
    return useQuery({
        queryKey: ['kv2', 'list', prefix],
        queryFn: async () => {
            try {
                return (await listDetailed(prefix)).entries
            } catch (err) {
                if (errStatus(err) === 404) return []
                throw err
            }
        },
        retry: (n, err) => errStatus(err) === undefined && n < 1
    })
}

/** counts for the stats strip; a token that cannot list the root simply gets no strip */
export function useKv2Totals() {
    return useQuery({
        queryKey: ['kv2', 'list', 'totals'],
        queryFn: getTotals,
        retry: false,
        staleTime: 15000
    })
}

/** data is the whole metadata response: {metadata, cas_required_effective, mount_options} */
export function useKv2Meta(path: string, enabled = true) {
    return useQuery({
        queryKey: ['kv2', 'meta', path],
        queryFn: () => getMetaFull(path),
        enabled,
        retry: false
    })
}

export function useKv2Version(path: string, version: number | undefined, enabled = true) {
    return useQuery({
        queryKey: ['kv2', 'get', path, version],
        queryFn: () => getKVv2(path, version),
        enabled,
        retry: false
    })
}
