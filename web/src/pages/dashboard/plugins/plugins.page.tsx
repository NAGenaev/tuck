import { DonutChart } from '@mantine/charts'
import { ActionIcon, Badge, Button, Card, Group, Select, SimpleGrid, Stack, Table, Text, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbPlus, TbPuzzle, TbTrash } from 'react-icons/tb'

import { deletePlugin, listPlugins, PluginType, registerPlugin } from '@shared/api/endpoints/plugins'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

const TYPES: PluginType[] = ['secret', 'auth', 'database']
const TYPE_COLORS: Record<string, string> = { auth: 'grape.6', database: 'orange.6', secret: 'cyan.6' }

function RegisterForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [type, setType] = useState<PluginType>('secret')
    const [name, setName] = useState('')
    const [command, setCommand] = useState('')
    const [sha256, setSha256] = useState('')

    const mutation = useMutation({
        mutationFn: () => registerPlugin(type, name, { command, sha256 }),
        onSuccess: () => {
            notifications.show({
                color: 'teal',
                message: t('plugins.registeredMessage', { name }),
                title: t('common.registered')
            })
            onDone()
        },
        onError: () => notifications.show({ color: 'red', message: t('plugins.registerFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="sm">
            <Text c="dimmed" size="xs">
                {t('plugins.createHint')}
            </Text>
            <Group grow>
                <Select
                    data={TYPES}
                    label={t('common.type')}
                    onChange={(v) => setType((v as PluginType) ?? 'secret')}
                    value={type}
                />
                <TextInput autoFocus label={t('common.name')} onChange={(e) => setName(e.currentTarget.value)} value={name} />
            </Group>
            <TextInput
                label={t('plugins.command')}
                onChange={(e) => setCommand(e.currentTarget.value)}
                placeholder={t('plugins.commandPlaceholder')}
                value={command}
            />
            <TextInput
                label={t('plugins.sha256')}
                onChange={(e) => setSha256(e.currentTarget.value)}
                placeholder={t('plugins.sha256Placeholder')}
                value={sha256}
            />
            <Button
                disabled={!name || !command || !sha256}
                fullWidth
                leftSection={<TbPlus size={16} />}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('plugins.registerPlugin')}
            </Button>
        </Stack>
    )
}

export function PluginsPage() {
    const { t } = useTranslation()
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
    const [filterType, setFilterType] = useState<PluginType | null>(null)
    const queryClient = useQueryClient()

    const { data: plugins, isLoading } = useQuery({
        queryKey: ['plugins', 'list', filterType],
        queryFn: () => listPlugins(filterType ?? undefined)
    })

    const deleteMutation = useMutation({
        mutationFn: ({ type, name }: { name: string; type: PluginType }) => deletePlugin(type, name),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['plugins', 'list'] })
    })

    const byType = useMemo(() => {
        const counts = new Map<string, number>()
        for (const plugin of plugins ?? []) {
            counts.set(plugin.type, (counts.get(plugin.type) ?? 0) + 1)
        }
        return Array.from(counts.entries()).map(([name, value]) => ({
            color: TYPE_COLORS[name] ?? 'gray.6',
            name,
            value
        }))
    }, [plugins])

    return (
        <Page title="Plugins">
            <Stack gap="lg">
                <PageHeader
                    action={
                        <Button leftSection={<TbPlus size={16} />} onClick={openCreating}>
                            {t('common.new')}
                        </Button>
                    }
                    color="cyan"
                    icon={TbPuzzle}
                    title={t('pages.plugins')}
                />

                <SimpleGrid cols={{ base: 1, md: 2 }}>
                    <MetricCard
                        IconComponent={TbPuzzle}
                        iconColor="cyan"
                        isLoading={isLoading}
                        title={t('common.total')}
                        value={plugins?.length ?? 0}
                    />
                    {byType.length > 0 && (
                        <Card withBorder>
                            <Group justify="space-between" wrap="nowrap">
                                <DonutChart data={byType} size={110} thickness={16} withTooltip />
                                <Stack gap={4}>
                                    {byType.map((entry) => (
                                        <Group gap={6} key={entry.name} wrap="nowrap">
                                            <Badge color={entry.color} size="xs" variant="filled" w={8} />
                                            <Text size="xs">
                                                {entry.name} · {entry.value}
                                            </Text>
                                        </Group>
                                    ))}
                                </Stack>
                            </Group>
                        </Card>
                    )}
                </SimpleGrid>

                <EntityModal color="cyan" icon={TbPuzzle} onClose={closeCreating} opened={creating} title={t('plugins.registerPlugin')}>
                    <RegisterForm
                        onDone={() => {
                            closeCreating()
                            queryClient.invalidateQueries({ queryKey: ['plugins', 'list'] })
                        }}
                    />
                </EntityModal>

                <Select
                    clearable
                    data={TYPES}
                    label={t('plugins.filterByType')}
                    onChange={(v) => setFilterType((v as PluginType) ?? null)}
                    placeholder={t('plugins.allTypes')}
                    value={filterType}
                    w={200}
                />

                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('common.name')}</Table.Th>
                            <Table.Th>{t('common.type')}</Table.Th>
                            <Table.Th>{t('plugins.version')}</Table.Th>
                            <Table.Th w={60} />
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {!isLoading && (plugins?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td colSpan={4}>
                                    <EmptyState icon={TbPuzzle} label={t('common.noResults')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {plugins?.map((plugin) => (
                            <Table.Tr key={`${plugin.type}-${plugin.name}`}>
                                <Table.Td>{plugin.name}</Table.Td>
                                <Table.Td>
                                    <Badge color="cyan" variant="light">
                                        {plugin.type}
                                    </Badge>
                                </Table.Td>
                                <Table.Td>{plugin.version ?? '—'}</Table.Td>
                                <Table.Td>
                                    {!plugin.builtin && (
                                        <ActionIcon
                                            color="red"
                                            onClick={() =>
                                                modals.openConfirmModal({
                                                    title: t('plugins.deletePluginTitle'),
                                                    children: t('plugins.deletePluginConfirm', { name: plugin.name }),
                                                    labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
                                                    confirmProps: { color: 'red' },
                                                    onConfirm: () =>
                                                        deleteMutation.mutate({
                                                            type: plugin.type,
                                                            name: plugin.name
                                                        })
                                                })
                                            }
                                            variant="subtle"
                                        >
                                            <TbTrash size={16} />
                                        </ActionIcon>
                                    )}
                                </Table.Td>
                            </Table.Tr>
                        ))}
                    </Table.Tbody>
                </TableCard>
            </Stack>
        </Page>
    )
}
