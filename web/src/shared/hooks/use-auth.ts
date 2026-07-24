import { useContext } from 'react'

import { AuthContext } from '@shared/hocs/auth-provider/auth-provider'
import { removeToken } from '@shared/auth/token'

export function useAuth() {
    const ctx = useContext(AuthContext)
    if (!ctx) {
        throw new Error('useAuth must be used within AuthProvider')
    }
    return ctx
}

export function useLogout() {
    const { setIsAuthenticated } = useAuth()
    return () => {
        removeToken()
        setIsAuthenticated(false)
    }
}
