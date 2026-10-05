import { Badge } from '@mantine/core'

import classes from './badge.module.css'

export default {
    Badge: Badge.extend({
        classNames: { root: classes.root },
        defaultProps: {
            radius: 'md',
            variant: 'outline'
        }
    })
}
