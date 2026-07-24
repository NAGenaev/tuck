// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Drawer, DrawerOverlay } from '@mantine/core'

import classes from './drawer.module.css'

export default {
    Drawer: Drawer.extend({
        classNames: {
            header: classes.drawerHeader,
            body: classes.drawerBody
        },
        defaultProps: {
            radius: 'md'
        }
    }),
    DrawerOverlay: DrawerOverlay.extend({
        defaultProps: {
            backgroundOpacity: 0.6,
            blur: 0
        }
    })
}
