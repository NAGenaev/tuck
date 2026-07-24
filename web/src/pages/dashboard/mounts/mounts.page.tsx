import { DonutChart } from '@mantine/charts'
import { ActionIcon, Badge, Button, Card, Group, SimpleGrid, Stack, Table, Text, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbPlugConnected, TbPlus, TbSettings, TbTrash } from 'react-icons/tb'

import {
    createMount,
    deleteMount,
    getMountConfig,
    listMounts,
    putMountConfig
} from '@shared/api/endpoints/mounts'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

const CHART_COLORS = ['cyan.6', 'grape.6', 'teal.6', 'orange.6', 'indigo.6', 'pink.6']

function CreateMountForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [path, setPath] = useState('')
    const [type, setType] = useState('')
    const [description, setDescription] = useState('')

    const mutation = useMutation({
        mutationFn: () => createMount(path, type, description || undefined),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('mounts.mountedMessage', { path }), title: t('mounts.mountedTitle') })
            onDone()
        },
        onError: () => notifications.show({ color: 'red', message: t('mounts.mountFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="sm">
            <Text c="dimmed" size="xs">
                {t('mounts.createHint')}
            </Text>
            <Group grow>
                <TextInput
                    autoFocus
                    label={t('mounts.path')}
                    onChange={(e) => setPath(e.currentTarget.value)}
                    placeholder={t('mounts.pathPlaceholder')}
                    value={path}
                />
                <TextInput
                    label={t('common.type')}
                    onChange={(e) => setType(e.currentTarget.value)}
                    placeholder={t('mounts.typePlaceholder')}
                    value={type}
                />
            </Group>
            <TextInput
                label={t('mounts.description')}
                onChange={(e) => setDescription(e.currentTarget.value)}
                value={description}
            />
            <Button
                disabled={!path || !type}
                fullWidth
                leftSection={<TbPlus size={16} />}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('mounts.registerMount')}
            </Button>
        </Stack>
    )
}

function TuneForm({ path, onClose }: { onClose: () => void; path: string }) {
    const { t } = useTranslation()
    const [defaultTtl, setDefaultTtl] = useState('')
    const [maxTtl, setMaxTtl] = useState('')

    const { data } = useQuery({
        queryKey: ['mounts', 'config', path],
        queryFn: () => getMountConfig(path)
    })

    const mutation = useMutation({
        mutationFn: () =>
            putMountConfig(path, {
                default_lease_ttl: defaultTtl || undefined,
                max_lease_ttl: maxTtl || undefined
            }),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('mounts.tuningSaved'), title: t('common.saved') })
            onClose()
        },
        onError: () => notifications.show({ color: 'red', message: t('common.saveFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="sm">
            <Text c="dimmed" size="xs">
                {t('mounts.currentTuning', { default: formatNs(data?.default_lease_ttl), max: formatNs(data?.max_lease_ttl) })}
            </Text>
            <Group grow>
                <TextInput
                    label={t('mounts.defaultLeaseTtl')}
                    onChange={(e) => setDefaultTtl(e.currentTarget.value)}
                    placeholder={t('authMethods.ttlPlaceholder1h')}
                    value={defaultTtl}
                />
                <TextInput
                    label={t('mounts.maxLeaseTtl')}
                    onChange={(e) => setMaxTtl(e.currentTarget.value)}
                    placeholder={t('tokens.ttlPlaceholder')}
                    value={maxTtl}
                />
            </Group>
            <Button fullWidth loading={mutation.isPending} onClick={() => mutation.mutate()}>
                {t('mounts.saveTuning')}
            </Button>
        </Stack>
    )
}

function formatNs(ns?: number): string {
    if (!ns) return '—'
    return `${(ns / 1e9 / 3600).toFixed(1)}h`
}

export function MountsPage() {
    const { t } = useTranslation()
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
    const [tuning, setTuning] = useState<null | string>(null)
    const queryClient = useQueryClient()

    const { data: mounts, isLoading } = useQuery({ queryKey: ['mounts', 'list'], queryFn: listMounts })

    const deleteMutation = useMutation({
        mutationFn: deleteMount,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['mounts', 'list'] })
    })

    const byType = useMemo(() => {
        const counts = new Map<string, number>()
        for (const mount of mounts ?? []) {
            counts.set(mount.type, (counts.get(mount.type) ?? 0) + 1)
        }
        return Array.from(counts.entries()).map(([name, value], i) => ({
            color: CHART_COLORS[i % CHART_COLORS.length],
            name,
            value
        }))
    }, [mounts])

    return (
        <Page title="Mounts">
            <Stack gap="lg">
                <PageHeader
                    action={
                        <Button leftSection={<TbPlus size={16} />} onClick={openCreating}>
                            {t('common.new')}
                        </Button>
                    }
                    color="teal"
                    icon={TbPlugConnected}
                    title={t('pages.mounts')}
                />

                <SimpleGrid cols={{ base: 1, md: 2 }}>
                    <MetricCard
                        IconComponent={TbPlugConnected}
                        iconColor="teal"
                        isLoading={isLoading}
                        title={t('common.total')}
                        value={mounts?.length ?? 0}
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

                <EntityModal color="teal" icon={TbPlugConnected} onClose={closeCreating} opened={creating} title={t('mounts.registerMount')}>
                    <CreateMountForm
                        onDone={() => {
                            closeCreating()
                            queryClient.invalidateQueries({ queryKey: ['mounts', 'list'] })
                        }}
                    />
                </EntityModal>

                <EntityModal
                    color="teal"
                    icon={TbSettings}
                    onClose={() => setTuning(null)}
                    opened={!!tuning}
                    title={tuning ? t('mounts.tuneTitle', { path: tuning }) : ''}
                >
                    {tuning && <TuneForm onClose={() => setTuning(null)} path={tuning} />}
                </EntityModal>

                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('mounts.path')}</Table.Th>
                            <Table.Th>{t('common.type')}</Table.Th>
                            <Table.Th>{t('mounts.accessor')}</Table.Th>
                            <Table.Th w={100} />
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {!isLoading && (mounts?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td colSpan={4}>
                                    <EmptyState icon={TbPlugConnected} label={t('common.noResults')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {mounts?.map((mount) => (
                            <Table.Tr key={mount.path}>
                                <Table.Td>{mount.path}</Table.Td>
                                <Table.Td>
                                    <Badge color="blue" variant="light">
                                        {mount.type}
                                    </Badge>
                                </Table.Td>
                                <Table.Td>
                                    <Text c="dimmed" ff="monospace" size="xs">
                                        {mount.accessor}
                                    </Text>
                                </Table.Td>
                                <Table.Td>
                                    <Group gap="xs" justify="flex-end">
                                        <ActionIcon onClick={() => setTuning(mount.path)} variant="subtle">
                                            <TbSettings size={16} />
                                        </ActionIcon>
                                        {!mount.builtin && (
                                            <ActionIcon
                                                color="red"
                                                onClick={() =>
                                                    modals.openConfirmModal({
                                                        title: t('mounts.unmountTitle'),
                                                        children: t('mounts.unmountConfirm', { path: mount.path }),
                                                        labels: { confirm: t('mounts.unmount'), cancel: t('common.cancel') },
                                                        confirmProps: { color: 'red' },
                                                        onConfirm: () => deleteMutation.mutate(mount.path)
                                                    })
                                                }
                                                variant="subtle"
                                            >
                                                <TbTrash size={16} />
                                            </ActionIcon>
                                        )}
                                    </Group>
                                </Table.Td>
                            </Table.Tr>
                        ))}
                    </Table.Tbody>
                </TableCard>
            </Stack>
        </Page>
    )
}
