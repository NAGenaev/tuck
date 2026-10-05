// SPDX-License-Identifier: AGPL-3.0-only
import { Pagination, SegmentedControl, Tabs } from '@mantine/core'

import classes from './navigation.module.css'

export default {
    Tabs: Tabs.extend({
        classNames: {
            root: classes.tabsRoot,
            list: classes.tabsList,
            tab: classes.tab,
            tabLabel: classes.tabLabel,
            tabSection: classes.tabSection,
            panel: classes.panel
        },
        defaultProps: { radius: 0 }
    }),
    SegmentedControl: SegmentedControl.extend({
        classNames: {
            root: classes.segRoot,
            indicator: classes.segIndicator,
            label: classes.segLabel,
            input: classes.segInput
        },
        defaultProps: { radius: 0 }
    }),
    Pagination: Pagination.extend({
        classNames: { control: classes.pgControl, dots: classes.pgDots },
        defaultProps: { radius: 0, gap: 4 }
    })
}
