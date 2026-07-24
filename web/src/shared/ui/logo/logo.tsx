import { Group, Text } from '@mantine/core'

const logoSrc = `${import.meta.env.BASE_URL}logo.png`

export function Logo() {
    return (
        <Group gap={10} wrap="nowrap">
            <img alt="Tuck" height={30} src={logoSrc} width={30} />
            <Text
                fw={800}
                fz={20}
                style={{ letterSpacing: '0.06em' }}
                tt="uppercase"
            >
                Tuck
            </Text>
        </Group>
    )
}
