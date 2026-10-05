// SPDX-License-Identifier: AGPL-3.0-only
// Maps the Mantine color names the app already passes to notifications / alerts
// (`color: 'red' | 'teal' | ...`) onto the artifact's semantic tones.
export const toneVar = (color?: string): string => {
    switch (color) {
        case 'red':
        case 'pink':
            return 'var(--alarm)'
        case 'teal':
        case 'green':
        case 'lime':
            return 'var(--mint)'
        case 'blue':
        case 'cyan':
        case 'indigo':
            return 'var(--ice)'
        case 'violet':
        case 'grape':
            return 'var(--violet)'
        case 'gray':
        case 'dark':
            return 'var(--mute)'
        default:
            // yellow / orange / brass / primary
            return 'var(--brass)'
    }
}
