import { Stack, Text, ThemeIcon } from '@mantine/core'
import { ElementType } from 'react'
import { TbInbox } from 'react-icons/tb'

export function EmptyState({
    icon: Icon = TbInbox,
    label
}: {
    icon?: ElementType<{ size: number }>
    label: string
}) {
    return (
        <Stack align="center" gap={6} py="lg">
            <ThemeIcon color="gray" radius="xl" size={40} variant="light">
                <Icon size={20} />
            </ThemeIcon>
            <Text c="dimmed" size="sm">
                {label}
            </Text>
        </Stack>
    )
}
