// SPDX-License-Identifier: AGPL-3.0-only
import { Drawer, HoverCard, Menu, Modal, Popover, Tooltip } from '@mantine/core'

import classes from './overlays.module.css'
import { dropdownTransition, EASE, modalTransition, tooltipTransition } from './transitions'

const frame = {
    header: classes.header,
    title: classes.title,
    body: classes.body,
    close: classes.close,
    overlay: classes.overlay
}

const overlayProps = { color: '#050608', backgroundOpacity: 0.7, blur: 3 }

const dropdownProps = {
    transitionProps: { transition: dropdownTransition, duration: 200, timingFunction: EASE }
}

export default {
    Modal: Modal.extend({
        classNames: { ...frame, content: classes.content },
        defaultProps: {
            centered: true,
            radius: 0,
            shadow: 'none',
            size: 520,
            overlayProps,
            transitionProps: { transition: modalTransition, duration: 300, timingFunction: EASE }
        }
    }),
    Drawer: Drawer.extend({
        classNames: { ...frame, content: `${classes.content} ${classes.drawerContent}` },
        defaultProps: {
            position: 'right',
            radius: 0,
            shadow: 'none',
            size: 440,
            overlayProps,
            transitionProps: { duration: 400, timingFunction: EASE }
        }
    }),
    Menu: Menu.extend({
        classNames: {
            dropdown: classes.dropdown,
            item: classes.item,
            itemSection: classes.itemSection,
            label: classes.menuLabel,
            divider: classes.divider,
            arrow: classes.arrow
        },
        defaultProps: { radius: 0, shadow: 'none', offset: 6, ...dropdownProps }
    }),
    Popover: Popover.extend({
        classNames: { dropdown: `${classes.dropdown} ${classes.popover}`, arrow: classes.arrow },
        defaultProps: { radius: 0, shadow: 'none', offset: 8, ...dropdownProps }
    }),
    HoverCard: HoverCard.extend({
        classNames: { dropdown: `${classes.dropdown} ${classes.popover}`, arrow: classes.arrow },
        defaultProps: { radius: 0, shadow: 'none', ...dropdownProps }
    }),
    Tooltip: Tooltip.extend({
        classNames: { tooltip: classes.tooltip, arrow: classes.tooltipArrow },
        defaultProps: {
            radius: 0,
            withinPortal: true,
            transitionProps: { transition: tooltipTransition, duration: 150, timingFunction: EASE }
        }
    })
}
