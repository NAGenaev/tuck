// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Breadcrumbs, px } from '@mantine/core'
import { GoDotFill as BreadcrumbsSeparator } from 'react-icons/go'

export default {
    Breadcrumbs: Breadcrumbs.extend({
        defaultProps: {
            separator: (
                <BreadcrumbsSeparator
                    color="var(--mantine-color-dimmed)"
                    opacity={0.4}
                    size={px('0.5rem')}
                />
            )
        }
    })
}
