// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00), adapted for Tuck.
import { createTheme } from '@mantine/core'

import { variantColorResolver } from './colors-resolver'
import components from './overrides'
import { ramp, tokens } from './palette'

const brass = ramp(tokens.brass)

export const theme = createTheme({
    variantColorResolver,
    components,
    cursorType: 'pointer',
    fontFamily: "'IBM Plex Sans', system-ui, -apple-system, 'Segoe UI', sans-serif",
    fontFamilyMonospace: "'JetBrains Mono', ui-monospace, Consolas, monospace",
    breakpoints: {
        xs: '30em',
        sm: '40em',
        md: '48em',
        lg: '64em',
        xl: '80em',
        '2xl': '96em',
        '3xl': '120em',
        '4xl': '160em'
    },
    scale: 1,
    fontSmoothing: true,
    focusRing: 'auto',
    white: tokens.bone,
    black: tokens.void,
    colors: {
        dark: [
            tokens.bone,
            '#C4C2BA',
            tokens.mute,
            '#6B7084',
            '#2F3443',
            tokens.vault2,
            tokens.vault,
            tokens.void,
            '#07080A',
            '#050608'
        ],
        brass,
        cyan: brass,
        teal: brass,
        yellow: brass,
        orange: ramp('#E8883F'),
        red: ramp(tokens.alarm),
        green: ramp(tokens.mint),
        blue: ramp(tokens.ice),
        violet: ramp(tokens.violet),
        grape: ramp(tokens.violet),
        indigo: ramp(tokens.ice),
        gray: ramp(tokens.mute)
    },
    primaryShade: 5,
    primaryColor: 'brass',
    autoContrast: true,
    luminanceThreshold: 0.4,
    radius: { xs: '0', sm: '0', md: '0', lg: '0', xl: '0' },
    defaultRadius: 'md',
    headings: {
        fontFamily: "'Unbounded', 'Arial Black', Impact, sans-serif",
        fontWeight: '700',
        sizes: {
            h1: { fontSize: '2rem', lineHeight: '1.1' },
            h2: { fontSize: '1.5rem', lineHeight: '1.15' },
            h3: { fontSize: '1.125rem', lineHeight: '1.2' },
            h4: { fontSize: '1rem', lineHeight: '1.25' },
            h5: { fontSize: '0.875rem', lineHeight: '1.3' },
            h6: { fontSize: '0.75rem', lineHeight: '1.3' }
        }
    }
})
