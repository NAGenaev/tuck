// SPDX-License-Identifier: AGPL-3.0-only
import { Table } from '@mantine/core'

import classes from './table.module.css'

export default {
    Table: Table.extend({
        classNames: {
            table: classes.table,
            th: classes.th,
            td: classes.td,
            tr: classes.tr,
            caption: classes.caption
        },
        defaultProps: {
            highlightOnHover: true,
            withRowBorders: true,
            verticalSpacing: 'sm',
            horizontalSpacing: 'md'
        }
    })
}
