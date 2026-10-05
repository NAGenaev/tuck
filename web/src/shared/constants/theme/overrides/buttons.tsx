// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { ActionIcon, Button, CloseButton } from '@mantine/core'

import classes from './buttons.module.css'

export default {
    ActionIcon: ActionIcon.extend({
        classNames: { root: classes.actionIcon },
        defaultProps: {
            radius: 'md',
            variant: 'outline'
        }
    }),
    Button: Button.extend({
        classNames: { root: classes.button, label: classes.buttonLabel },
        defaultProps: {
            radius: 'md',
            size: 'md',
            variant: 'filled'
        }
    }),
    CloseButton: CloseButton.extend({
        classNames: { root: classes.closeButton },
        defaultProps: {
            size: 'lg'
        }
    })
}
