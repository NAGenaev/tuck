// SPDX-License-Identifier: AGPL-3.0-only
// Forked (trimmed — inline skeleton instead of a shared ShimmerSkeleton component)
// from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Card, Group, Skeleton, Stack, Text, ThemeIcon, ThemeIconProps } from '@mantine/core'

export interface MetricCardProps {
    iconColor?: ThemeIconProps['color']
    IconComponent?: React.ComponentType<{ size: number }>
    isLoading?: boolean
    subtitle?: string
    title: string
    value: number | string
}

export function MetricCard({
    iconColor,
    IconComponent,
    isLoading,
    title,
    value,
    subtitle
}: MetricCardProps) {
    return (
        <Card withBorder>
            <Group gap="md" wrap="nowrap">
                {IconComponent && (
                    <ThemeIcon color={iconColor} radius="md" size={40} variant="light">
                        <IconComponent size={20} />
                    </ThemeIcon>
                )}

                <Stack gap={0} miw={0}>
                    <Text c="dimmed" size="xs" truncate="end">
                        {title}
                    </Text>
                    {isLoading ? (
                        <Skeleton height={24} width={80} />
                    ) : (
                        <Text fw={600} size="lg" truncate="end">
                            {value}
                        </Text>
                    )}
                    {subtitle && (
                        <Text c="dimmed" size="xs">
                            {subtitle}
                        </Text>
                    )}
                </Stack>
            </Group>
        </Card>
    )
}
