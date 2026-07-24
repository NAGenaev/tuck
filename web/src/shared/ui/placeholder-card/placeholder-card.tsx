import { Card, Group, Stack, Text, ThemeIcon } from '@mantine/core'
import { ElementType } from 'react'
import { useTranslation } from 'react-i18next'

export interface PlaceholderCardProps {
    IconComponent: ElementType<{ size: number }>
    subtitle?: string
    title: string
}

export function PlaceholderCard({
    IconComponent,
    title,
    subtitle
}: PlaceholderCardProps) {
    const { t } = useTranslation()
    return (
        <Card
            style={{ borderStyle: 'dashed', opacity: 0.6 }}
            withBorder
        >
            <Group gap="md" wrap="nowrap">
                <ThemeIcon color="gray" radius="md" size={40} variant="light">
                    <IconComponent size={20} />
                </ThemeIcon>
                <Stack gap={0} miw={0}>
                    <Text fw={600} size="sm" truncate="end">
                        {title}
                    </Text>
                    <Text c="dimmed" size="xs">
                        {subtitle ?? t('common.comingSoon')}
                    </Text>
                </Stack>
            </Group>
        </Card>
    )
}
