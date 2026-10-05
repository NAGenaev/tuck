// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from '@tanstack/react-query'

import { getKVv2, getKVv2Meta, listKVv2 } from '@shared/api/endpoints/kvv2'

import { errStatus } from './api'

// query keys are the same as the old page used, so invalidations stay compatible
export function useKv2List(prefix: string) {
    return useQuery({
        queryKey: ['kv2', 'list', prefix],
        queryFn: async () => {
            try {
                return await listKVv2(prefix)
            } catch (err) {
                if (errStatus(err) === 404) return []
                throw err
            }
        },
        retry: (n, err) => errStatus(err) === undefined && n < 1
    })
}

export function useKv2Meta(path: string, enabled = true) {
    return useQuery({
        queryKey: ['kv2', 'meta', path],
        queryFn: () => getKVv2Meta(path),
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
