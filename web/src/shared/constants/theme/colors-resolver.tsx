// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import {
    defaultVariantColorsResolver,
    parseThemeColor,
    rgba,
    VariantColorsResolver
} from '@mantine/core'

import { tokens } from './palette'

const INK = '#1A1200'
const INK_ON_ALARM = '#1A0402'

// Colors whose filled button carries dark "ink" text (everything bright). Neutral
// colors (gray/dark) keep the light text picked by Mantine.
const NEUTRAL = new Set(['gray', 'dark'])

export const variantColorResolver: VariantColorsResolver = (input) => {
    const defaultResolvedColors = defaultVariantColorsResolver(input)

    const parsed = parseThemeColor({
        color: input.color || input.theme.primaryColor,
        theme: input.theme,
        colorScheme: 'dark'
    })
    const base = input.theme.colors[parsed.color]?.[5] ?? parsed.value

    if (input.variant === 'soft') {
        return {
            background: rgba(base, 0.1),
            border: `1px solid ${rgba(base, 0.35)}`,
            color: `var(--mantine-color-${parsed.color}-4)`,
            hover: rgba(base, 0.18)
        }
    }

    if (input.variant === 'light') {
        return {
            ...defaultResolvedColors,
            background: rgba(base, 0.1),
            hover: rgba(base, 0.2),
            border: `1px solid ${rgba(base, 0.3)}`
        }
    }

    // "danger": the artifact's red button is a tinted outline that fills with
    // alarm on hover.
    if (input.variant === 'filled' && parsed.color === 'red') {
        return {
            background: rgba(tokens.alarm, 0.06),
            border: `1px solid ${rgba(tokens.alarm, 0.55)}`,
            color: tokens.alarm,
            hover: tokens.alarm,
            hoverColor: INK_ON_ALARM
        }
    }

    if (input.variant === 'filled' && !NEUTRAL.has(parsed.color)) {
        const isBrass = parsed.color === 'brass'
        return {
            ...defaultResolvedColors,
            background: base,
            border: `1px solid ${base}`,
            color: INK,
            hover: isBrass ? tokens.brassL : `var(--mantine-color-${parsed.color}-4)`,
            hoverColor: INK
        }
    }

    if (input.variant === 'outline' && parsed.color === 'red') {
        return {
            ...defaultResolvedColors,
            border: `1px solid ${rgba(tokens.alarm, 0.55)}`,
            hover: rgba(tokens.alarm, 0.1)
        }
    }

    return defaultResolvedColors
}
