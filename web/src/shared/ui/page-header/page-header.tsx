import { Box, Group, Title } from '@mantine/core'
import { ElementType, ReactNode } from 'react'

import { Eyebrow } from '@shared/ui/tuck/tuck'

export function PageHeader({
    action,
    eyebrow = 'Tuck',
    title
}: {
    action?: ReactNode
    color?: string
    eyebrow?: string
    icon?: ElementType<{ size: number }>
    title: string
}) {
    return (
        <Box
            pb="lg"
            pt="md"
            style={{
                borderBottom: '1px solid var(--line)',
                position: 'relative'
            }}
        >
            <Group align="flex-end" justify="space-between" wrap="wrap">
                <Box>
                    <Eyebrow>{eyebrow}</Eyebrow>
                    <Title
                        mt={14}
                        order={1}
                        style={{
                            fontWeight: 900,
                            fontSize: 'clamp(28px, 4vw, 44px)',
                            letterSpacing: '-0.03em',
                            lineHeight: 1,
                            textTransform: 'uppercase'
                        }}
                    >
                        {title}
                    </Title>
                </Box>
                {action}
            </Group>
            <Box bg="brass.5" h={2} style={{ position: 'absolute', bottom: -1, left: 0 }} w={96} />
        </Box>
    )
}
