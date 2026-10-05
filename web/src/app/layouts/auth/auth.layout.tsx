// SPDX-License-Identifier: AGPL-3.0-only
// Forked (structure) from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Outlet } from 'react-router'

import styles from './auth.module.css'

export function AuthLayout() {
    return (
        <div className={styles.root}>
            <div className={styles.glow} />
            <Outlet />
        </div>
    )
}
