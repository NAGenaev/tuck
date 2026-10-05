// SPDX-License-Identifier: AGPL-3.0-only
import { notifications } from '@mantine/notifications'
import { CSSProperties, ReactNode } from 'react'

import { Icon, IconName } from '../icons'

type Tone = 'error' | 'info' | 'success' | 'warning'

// color = the legacy Mantine color name; the theme maps it to a brand token (see overrides/tone.ts),
// so these toasts look the same as old `notifications.show({ color: 'red' | 'teal', ... })` calls.
const META: Record<Tone, { color: string; icon: IconName; ms: number }> = {
    success: { color: 'teal', icon: 'check', ms: 4000 },
    error: { color: 'red', icon: 'alert', ms: 6000 },
    warning: { color: 'yellow', icon: 'warn', ms: 5000 },
    info: { color: 'blue', icon: 'info', ms: 4000 }
}

function show(tone: Tone, title: ReactNode, message?: ReactNode, autoClose?: false | number) {
    const meta = META[tone]
    const ms = autoClose === undefined ? meta.ms : autoClose
    return notifications.show({
        autoClose: ms,
        color: meta.color,
        icon: <Icon name={meta.icon} size={15} />,
        message: message ?? '',
        // the theme paints a shrinking life line from --tk-life (omitted for sticky toasts)
        style: ms === false ? undefined : ({ '--tk-life': `${ms}ms` } as CSSProperties),
        title,
        withCloseButton: true
    })
}

/**
 * Toast helpers over Mantine notifications.
 * `message` may be a ReactNode, wrap values in <code> to get the mono highlight.
 * `autoClose` is in ms; pass false to keep the toast until it is dismissed.
 */
export const toast = {
    success: (title: ReactNode, message?: ReactNode, autoClose?: false | number) =>
        show('success', title, message, autoClose),
    error: (title: ReactNode, message?: ReactNode, autoClose?: false | number) =>
        show('error', title, message, autoClose),
    warning: (title: ReactNode, message?: ReactNode, autoClose?: false | number) =>
        show('warning', title, message, autoClose),
    info: (title: ReactNode, message?: ReactNode, autoClose?: false | number) =>
        show('info', title, message, autoClose),
    hide: (id: string) => notifications.hide(id)
}
