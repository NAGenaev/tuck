import { Group, ThemeIcon, Title } from '@mantine/core'
import { ElementType, ReactNode } from 'react'

export function PageHeader({
    icon: Icon,
    color = 'cyan',
    title,
    action
}: {
    action?: ReactNode
    color?: string
    icon: ElementType<{ size: number }>
    title: string
}) {
    return (
        <Group justify="space-between" wrap="nowrap">
            <Group gap="sm">
                <ThemeIcon color={color} radius="md" size={36} variant="light">
                    <Icon size={20} />
                </ThemeIcon>
                <Title order={3}>{title}</Title>
            </Group>
            {action}
        </Group>
    )
}
