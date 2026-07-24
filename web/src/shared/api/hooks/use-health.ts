import { useQuery } from '@tanstack/react-query'

import { getHealth } from '../endpoints/sys'

export function useHealth() {
    return useQuery({
        queryKey: ['sys', 'health'],
        queryFn: getHealth,
        refetchInterval: 5_000
    })
}
