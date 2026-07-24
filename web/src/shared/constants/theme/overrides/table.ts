// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Table } from '@mantine/core'

export default {
    Table: Table.extend({
        defaultProps: {
            highlightOnHover: true
        }
    })
}
