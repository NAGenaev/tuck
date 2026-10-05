import { MantineColorsTuple } from '@mantine/core'

const toRgb = (hex: string): [number, number, number] => [
    parseInt(hex.slice(1, 3), 16),
    parseInt(hex.slice(3, 5), 16),
    parseInt(hex.slice(5, 7), 16)
]

const mix = (a: string, b: string, t: number): string => {
    const [ar, ag, ab] = toRgb(a)
    const [br, bg, bb] = toRgb(b)
    const ch = (x: number, y: number) =>
        Math.round(x + (y - x) * t)
            .toString(16)
            .padStart(2, '0')
    return `#${ch(ar, br)}${ch(ag, bg)}${ch(ab, bb)}`
}

// Ten shades with `base` at index 5 (the primary shade), lighter below, darker above.
export const ramp = (base: string): MantineColorsTuple => {
    const light = [0.92, 0.82, 0.68, 0.5, 0.25]
    const dark = [0.18, 0.36, 0.54, 0.72]
    return [
        ...light.map((t) => mix(base, '#ffffff', t)),
        base,
        ...dark.map((t) => mix(base, '#0A0B0E', t))
    ] as unknown as MantineColorsTuple
}

export const tokens = {
    void: '#0A0B0E',
    vault: '#121420',
    vault2: '#1A1D29',
    line: '#272B38',
    line2: '#3B4154',
    bone: '#EDEAE2',
    mute: '#9096A8',
    dim: '#575C70',
    brass: '#D7A94B',
    brassL: '#F0C772',
    alarm: '#FF5A4D',
    mint: '#49E0A4',
    ice: '#45C9E6',
    violet: '#9C8CFF'
}
