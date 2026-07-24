// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00):
// keeps the mount-time bootstrap + logout-event-subscription shape, backed by
// Tuck's GET /v1/auth/token/lookup-self instead of Remnawave's auth-status call.
import { createContext, ReactNode, useEffect, useMemo, useState } from 'react'

import { lookupSelf } from '@shared/api/endpoints/auth'
import { getToken, removeToken } from '@shared/auth/token'
import { logoutEvents } from '@shared/emitters/emit-logout'

interface AuthContextValues {
    isAuthenticated: boolean
    isInitialized: boolean
    setIsAuthenticated: (isAuthenticated: boolean) => void
}

// eslint-disable-next-line react-refresh/only-export-components
export const AuthContext = createContext<AuthContextValues | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
    const [isAuthenticated, setIsAuthenticated] = useState(false)
    const [isInitialized, setIsInitialized] = useState(false)

    const logoutUser = () => {
        removeToken()
        setIsAuthenticated(false)
    }

    useEffect(() => {
        return logoutEvents.subscribe(logoutUser)
    }, [])

    useEffect(() => {
        ;(async () => {
            const token = getToken()
            if (!token) {
                setIsAuthenticated(false)
                setIsInitialized(true)
                return
            }

            try {
                await lookupSelf()
                setIsAuthenticated(true)
            } catch {
                logoutUser()
            } finally {
                setIsInitialized(true)
            }
        })()
    }, [])

    const value = useMemo(
        () => ({ isAuthenticated, isInitialized, setIsAuthenticated }),
        [isAuthenticated, isInitialized]
    )

    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
