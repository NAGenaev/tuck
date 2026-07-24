// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
type Listener = () => void

function createEmitter() {
    const listeners = new Set<Listener>()
    return {
        emit() {
            listeners.forEach((listener) => listener())
        },
        subscribe(listener: Listener) {
            listeners.add(listener)
            return () => {
                listeners.delete(listener)
            }
        }
    }
}

export const logoutEvents = createEmitter()
