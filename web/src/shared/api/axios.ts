// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00):
// keeps the axios instance shape and the 401/403 -> decoupled-logout-event pattern,
// swaps Authorization-bearer for Tuck's X-Tuck-Token/X-Tuck-Namespace headers.
import axios from 'axios'

import { logoutEvents } from '../emitters/emit-logout'
import { getNamespace, getToken } from '../auth/token'

export const instance = axios.create({
    baseURL: window.location.origin,
    headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json'
    }
})

instance.interceptors.request.use((config) => {
    const token = getToken()
    if (token) {
        config.headers.set('X-Tuck-Token', token)
    }
    const namespace = getNamespace()
    if (namespace) {
        config.headers.set('X-Tuck-Namespace', namespace)
    }
    return config
})

instance.interceptors.response.use(
    (response) => response,
    (error) => {
        // 401 = not authenticated (invalid/expired token) -> drop the session.
        // 403 = authenticated but not authorized for this specific action ->
        // let the caller handle it (toast, hide widget); do NOT log out, or
        // any token with a policy short of root gets kicked out of the
        // dashboard the moment it hits a single restricted endpoint.
        if (error.response && error.response.status === 401) {
            logoutEvents.emit()
        }
        return Promise.reject(error)
    }
)
