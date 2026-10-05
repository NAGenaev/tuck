// SPDX-License-Identifier: AGPL-3.0-only
// Hand-off between the dashboard (which sees the 401) and the login screen
// (which shows the "session expired" variant). sessionStorage only — it
// never holds a token.
const KEY = 'tuck_session_expired'

export interface SessionExpiredInfo {
    /** ISO time at which the 401 was observed. */
    at: string
    /** Token expiry known from lookup-self before the 401, if any. */
    expiresAt?: string
}

export function markSessionExpired(info: Omit<SessionExpiredInfo, 'at'> = {}) {
    try {
        sessionStorage.setItem(KEY, JSON.stringify({ at: new Date().toISOString(), ...info }))
    } catch {
        /* storage unavailable: the plain login screen is shown instead */
    }
}

export function peekSessionExpired(): null | SessionExpiredInfo {
    try {
        const raw = sessionStorage.getItem(KEY)
        return raw ? (JSON.parse(raw) as SessionExpiredInfo) : null
    } catch {
        return null
    }
}

export function clearSessionExpired() {
    try {
        sessionStorage.removeItem(KEY)
    } catch {
        /* ignore */
    }
}
