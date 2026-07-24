// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { SegmentedControl } from '@mantine/core'

import classes from './segmented-control.module.css'

export default {
    SegmentedControl: SegmentedControl.extend({
        classNames: {
            root: classes.root,
            indicator: classes.indicator,
            label: classes.label
        },
        defaultProps: {
            radius: 'md',
            transitionDuration: 200
        }
    })
}
