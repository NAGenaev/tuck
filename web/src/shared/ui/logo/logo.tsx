import { Group, Text } from '@mantine/core'

const logoSrc = `${import.meta.env.BASE_URL}logo.png`

export function Logo() {
    return (
        <Group gap={10} wrap="nowrap">
            <img alt="Tuck" height={26} src={logoSrc} width={26} />
            <Text
                ff="Unbounded, sans-serif"
                fw={800}
                fz={16}
                style={{ letterSpacing: '0.02em' }}
                c="brass.5"
                tt="uppercase"
            >
                Tuck
            </Text>
        </Group>
    )
}
