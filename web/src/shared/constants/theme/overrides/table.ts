import { Table } from '@mantine/core'

import classes from './table.module.css'

export default {
    Table: Table.extend({
        classNames: { th: classes.th, tr: classes.tr, td: classes.td },
        defaultProps: {
            highlightOnHover: true
        }
    })
}
