// SPDX-License-Identifier: AGPL-3.0-only
import { MantineTransition } from '@mantine/core'

export const EASE = 'cubic-bezier(0.2, 0.8, 0.2, 1)'

// The artifact's dropdown / menu motion: slide down 6px and scale up from .98.
export const dropdownTransition: MantineTransition = {
    in: { opacity: 1, transform: 'translateY(0) scale(1)' },
    out: { opacity: 0, transform: 'translateY(-6px) scale(0.98)' },
    common: { transformOrigin: 'top' },
    transitionProperty: 'opacity, transform'
}

// Modal: rise 24px from .96.
export const modalTransition: MantineTransition = {
    in: { opacity: 1, transform: 'translateY(0) scale(1)' },
    out: { opacity: 0, transform: 'translateY(24px) scale(0.96)' },
    common: { transformOrigin: 'center' },
    transitionProperty: 'opacity, transform'
}

// Tooltip: grows out of the anchor sideways.
export const tooltipTransition: MantineTransition = {
    in: { opacity: 1, transform: 'translateY(0) scaleX(1)' },
    out: { opacity: 0, transform: 'translateY(6px) scaleX(0.6)' },
    common: { transformOrigin: 'center bottom' },
    transitionProperty: 'opacity, transform'
}
