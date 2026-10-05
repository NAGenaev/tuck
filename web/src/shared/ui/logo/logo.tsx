// SPDX-License-Identifier: AGPL-3.0-only
import { Link } from 'react-router'

import { ROUTES } from '@shared/constants/routes'
import c from '@shared/ui/shell-controls/console.module.css'
import { ShellIcon } from '@shared/ui/shell-controls/icons'

/** TUCK wordmark with the reticle mark (rotates on hover). */
export function Logo({ to = ROUTES.DASHBOARD.STATUS }: { to?: string }) {
    return (
        <Link aria-label="Tuck" className={c.brand} to={to}>
            <ShellIcon name="reticle" size={24} stroke={2} />
            TUCK
        </Link>
    )
}
