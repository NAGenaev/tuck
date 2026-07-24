import { Group, Modal, Text, ThemeIcon } from '@mantine/core'
import { ElementType, ReactNode } from 'react'

export function EntityModal({
    opened,
    onClose,
    icon: Icon,
    color = 'cyan',
    title,
    size = 'md',
    children
}: {
    children: ReactNode
    color?: string
    icon: ElementType<{ size: number }>
    onClose: () => void
    opened: boolean
    size?: 'lg' | 'md' | 'sm' | 'xl'
    title: string
}) {
    return (
        <Modal
            centered
            onClose={onClose}
            opened={opened}
            padding="lg"
            size={size}
            title={
                <Group gap="sm">
                    <ThemeIcon color={color} radius="md" size={32} variant="light">
                        <Icon size={18} />
                    </ThemeIcon>
                    <Text fw={700} size="lg">
                        {title}
                    </Text>
                </Group>
            }
        >
            {children}
        </Modal>
    )
}
