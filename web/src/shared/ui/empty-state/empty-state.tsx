// SPDX-License-Identifier: AGPL-3.0-only
import { ElementType } from 'react'

import { StateBlock } from '../kit/state-block/state-block'

/** Quiet "nothing here" one-liner. For titled / retry / 403 variants use StateBlock directly. */
export function EmptyState({
    icon,
    label
}: {
    icon?: ElementType<{ size: number }>
    label: string
}) {
    return <StateBlock compact icon={icon} kind="empty" message={label} />
}
