import { Box, Group, ThemeIcon, Title } from '@mantine/core'
import { ElementType, ReactNode } from 'react'

export function PageHeader({
    icon: Icon,
    color = 'brass',
    title,
    action
}: {
    action?: ReactNode
    color?: string
    icon: ElementType<{ size: number }>
    title: string
}) {
    return (
        <Box
            pb="md"
            style={{
                borderBottom: '1px solid var(--mantine-color-dark-4)',
                position: 'relative'
            }}
        >
            <Group justify="space-between" wrap="nowrap">
                <Group gap="md">
                    <ThemeIcon
                        c={`${color}.5`}
                        radius="md"
                        size={40}
                        style={{ borderColor: 'var(--mantine-color-dark-3)' }}
                        variant="default"
                    >
                        <Icon size={20} />
                    </ThemeIcon>
                    <Title order={2}>{title}</Title>
                </Group>
                {action}
            </Group>
            <Box
                bg="brass.5"
                h={2}
                style={{ position: 'absolute', bottom: -1, left: 0 }}
                w={72}
            />
        </Box>
    )
}
