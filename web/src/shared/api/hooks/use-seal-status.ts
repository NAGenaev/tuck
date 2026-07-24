import { useQuery } from '@tanstack/react-query'

import { getSealStatus } from '../endpoints/sys'

export function useSealStatus() {
    return useQuery({
        queryKey: ['sys', 'seal-status'],
        queryFn: getSealStatus,
        refetchInterval: 5_000
    })
}
