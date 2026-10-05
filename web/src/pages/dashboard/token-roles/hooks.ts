// SPDX-License-Identifier: AGPL-3.0-only
import { useQueries, useQuery } from '@tanstack/react-query'
import axios from 'axios'

import { getTokenRole, listTokenRoles } from '@shared/api/endpoints/token-roles'

import { RoleRow, toView } from './lib'

/** The list endpoint returns names only, so each role's details are fetched in parallel. */
export function useRoles() {
    const list = useQuery({ queryFn: listTokenRoles, queryKey: ['token-roles', 'list'] })
    const names = list.data ?? []
    const details = useQueries({
        queries: names.map((name) => ({
            queryFn: () => getTokenRole(name),
            queryKey: ['token-roles', 'get', name],
            staleTime: 15_000
        }))
    })

    const rows: RoleRow[] = names.map((name, i) => {
        const d = details[i]
        return {
            failed: !!d?.isError,
            loading: !d || d.isLoading,
            name,
            role: d?.data ? toView(name, d.data) : undefined
        }
    })

    const status = axios.isAxiosError(list.error) ? list.error.response?.status : undefined
    return {
        forbidden: status === 403,
        isError: list.isError,
        isLoading: list.isLoading,
        list,
        rows
    }
}

export function errorMessage(e: unknown): string | undefined {
    if (axios.isAxiosError(e)) {
        const msg = (e.response?.data as { error?: string } | undefined)?.error
        return msg ?? e.message
    }
    return e instanceof Error ? e.message : undefined
}
