import { ActionIcon, Badge, Button, Card, Group, Select, Stack, Table, Text, TextInput } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbPlus, TbPuzzle, TbTrash } from 'react-icons/tb'

import { deletePlugin, listPlugins, PluginType, registerPlugin } from '@shared/api/endpoints/plugins'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

const TYPES: PluginType[] = ['secret', 'auth', 'database']

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
        <Card>
            <Stack gap="sm">
                <Group grow>
                    <Select
                        data={TYPES}
                        label={t('common.type')}
                        onChange={(v) => setType((v as PluginType) ?? 'secret')}
                        value={type}
                    />
                    <TextInput label={t('common.name')} onChange={(e) => setName(e.currentTarget.value)} value={name} />
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
                    loading={mutation.isPending}
                    onClick={() => mutation.mutate()}
                >
                    {t('plugins.registerPlugin')}
                </Button>
            </Stack>
        </Card>
    )
}

export function PluginsPage() {
    const { t } = useTranslation()
    const [creating, setCreating] = useState(false)
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

    return (
        <Page title="Plugins">
            <Stack gap="lg">
                <PageHeader
                    action={
                        <Button
                            leftSection={<TbPlus size={16} />}
                            onClick={() => setCreating((c) => !c)}
                            variant="light"
                        >
                            {t('common.new')}
                        </Button>
                    }
                    color="cyan"
                    icon={TbPuzzle}
                    title={t('pages.plugins')}
                />

                {creating && (
                    <RegisterForm
                        onDone={() => {
                            setCreating(false)
                            queryClient.invalidateQueries({ queryKey: ['plugins', 'list'] })
                        }}
                    />
                )}

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
