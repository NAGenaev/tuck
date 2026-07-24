// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Tooltip } from '@mantine/core'

export default {
    Tooltip: Tooltip.extend({
        defaultProps: {
            radius: 'md',
            withArrow: true,
            transitionProps: { transition: 'scale-x', duration: 300 },
            arrowSize: 2,
            color: 'dark.6',
            styles: {
                tooltip: {
                    border: '1px solid var(--mantine-color-dark-4)'
                }
            }
        }
    })
}
