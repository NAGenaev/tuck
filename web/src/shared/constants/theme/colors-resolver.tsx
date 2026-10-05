// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import {
    defaultVariantColorsResolver,
    parseThemeColor,
    rgba,
    VariantColorsResolver
} from '@mantine/core'

export const variantColorResolver: VariantColorsResolver = (input) => {
    const defaultResolvedColors = defaultVariantColorsResolver(input)

    if (input.variant === 'soft') {
        const parsed = parseThemeColor({
            color: input.color || 'gray',
            theme: input.theme,
            colorScheme: 'dark'
        })
        const base = input.theme.colors[parsed.color][5]
        return {
            background: rgba(base, 0.1),
            border: `1px solid ${rgba(base, 0.35)}`,
            color: `var(--mantine-color-${parsed.color}-4)`,
            hover: rgba(base, 0.18)
        }
    }

    if (input.variant === 'light') {
        const parsed = parseThemeColor({
            color: input.color || input.theme.primaryColor,
            theme: input.theme,
            colorScheme: 'dark'
        })
        const base = input.theme.colors[parsed.color]?.[5] ?? parsed.value
        return {
            ...defaultResolvedColors,
            background: rgba(base, 0.1),
            hover: rgba(base, 0.2),
            border: `1px solid ${rgba(base, 0.3)}`
        }
    }

    return defaultResolvedColors
}
