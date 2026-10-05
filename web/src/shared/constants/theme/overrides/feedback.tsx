// SPDX-License-Identifier: AGPL-3.0-only
import { Alert, Badge, Notification, Progress, RingProgress, Skeleton } from '@mantine/core'

import classes from './feedback.module.css'
import { toneVar } from './tone'

export default {
    Notification: Notification.extend({
        classNames: {
            root: classes.toast,
            icon: classes.toastIcon,
            body: classes.toastBody,
            title: classes.toastTitle,
            description: classes.toastDescription,
            closeButton: classes.toastClose
        },
        defaultProps: { radius: 0, withBorder: true },
        vars: (_theme, props) => ({
            root: { '--notification-color': toneVar(props.color) }
        })
    }),
    Alert: Alert.extend({
        classNames: {
            root: classes.alert,
            icon: classes.alertIcon,
            title: classes.alertTitle,
            message: classes.alertMessage,
            closeButton: classes.alertClose
        },
        defaultProps: { radius: 0 },
        vars: (_theme, props) => ({
            root: { '--alert-color': toneVar(props.color) }
        })
    }),
    Skeleton: Skeleton.extend({
        classNames: { root: classes.skeleton },
        defaultProps: { radius: 0 }
    }),
    Progress: Progress.extend({
        classNames: {
            root: classes.progress,
            section: classes.progressSection,
            label: classes.progressLabel
        },
        defaultProps: { radius: 0, size: 'sm' }
    }),
    RingProgress: RingProgress.extend({
        classNames: { curve: classes.ringCurve, label: classes.ringLabel }
    }),
    Badge: Badge.extend({
        classNames: { root: classes.badge, label: classes.badgeLabel },
        defaultProps: { radius: 0, variant: 'light' }
    })
}
