// SPDX-License-Identifier: AGPL-3.0-only
import { Checkbox, Chip, Radio, Switch } from '@mantine/core'

import classes from './controls.module.css'

const inline = {
    root: classes.root,
    label: classes.label,
    description: classes.description,
    error: classes.error
}

export default {
    Switch: Switch.extend({
        classNames: {
            ...inline,
            input: classes.input,
            track: classes.track,
            thumb: classes.thumb,
            trackLabel: classes.trackLabel
        },
        // 44 x 24 square track with a 16px thumb, as in the artifact.
        vars: () => ({
            root: {
                '--switch-height': '24px',
                '--switch-width': '44px',
                '--switch-thumb-size': '16px',
                '--switch-radius': '0',
                '--switch-track-label-padding': '3px'
            }
        })
    }),
    Checkbox: Checkbox.extend({
        classNames: { ...inline, input: classes.box, icon: classes.boxIcon },
        defaultProps: { radius: 0, size: 'sm' },
        vars: () => ({
            root: { '--checkbox-color': 'var(--brass)', '--checkbox-icon-color': 'var(--ink)' }
        })
    }),
    Radio: Radio.extend({
        classNames: { ...inline, radio: classes.dot, icon: classes.dotIcon },
        vars: () => ({
            root: { '--radio-color': 'var(--brass)', '--radio-icon-color': 'var(--brass)' }
        })
    }),
    Chip: Chip.extend({
        classNames: { label: classes.chip, checkIcon: classes.chipIcon, input: classes.chipInput },
        defaultProps: { radius: 0, size: 'md' }
    })
}
