import { Badge, Button, Card, Group, SimpleGrid, Stack, Table, Text, TextInput } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbRefresh } from 'react-icons/tb'

import {
    disableReplication,
    enablePrimary,
    enableSecondary,
    getReplicationStatus,
    getWALEntries
} from '@shared/api/endpoints/replication'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

export function ReplicationPage() {
    const { t } = useTranslation()
    const [primaryAddr, setPrimaryAddr] = useState('')
    const queryClient = useQueryClient()

    const { data: status, isLoading } = useQuery({
        queryKey: ['replication', 'status'],
        queryFn: getReplicationStatus,
        refetchInterval: 5_000
    })

    const { data: wal } = useQuery({
        queryKey: ['replication', 'wal'],
        queryFn: () => getWALEntries(0),
        enabled: status?.mode !== 'disabled'
    })

    const invalidate = () => queryClient.invalidateQueries({ queryKey: ['replication', 'status'] })

    const primaryMutation = useMutation({
        mutationFn: enablePrimary,
        onSuccess: () => {
            invalidate()
            notifications.show({ color: 'teal', message: t('replication.primaryEnabledMessage'), title: t('common.enabled') })
        },
        onError: () => notifications.show({ color: 'red', message: t('replication.enableFailed'), title: t('common.error') })
    })

    const secondaryMutation = useMutation({
        mutationFn: () => enableSecondary(primaryAddr),
        onSuccess: () => {
            invalidate()
            notifications.show({ color: 'teal', message: t('replication.secondaryEnabledMessage'), title: t('common.enabled') })
        },
        onError: () => notifications.show({ color: 'red', message: t('replication.enableFailed'), title: t('common.error') })
    })

    const disableMutation = useMutation({
        mutationFn: disableReplication,
        onSuccess: () => {
            invalidate()
            notifications.show({ color: 'teal', message: t('replication.disabledMessage'), title: t('common.disabled') })
        }
    })

    return (
        <Page title="Replication">
            <Stack gap="lg">
                <PageHeader color="indigo" icon={TbRefresh} title={t('pages.replication')} />

                <SimpleGrid cols={{ base: 1, sm: 3 }}>
                    <MetricCard
                        IconComponent={TbRefresh}
                        iconColor={status?.mode === 'disabled' ? 'gray' : 'teal'}
                        isLoading={isLoading}
                        title={t('replication.mode')}
                        value={status?.mode ?? '—'}
                    />
                    <MetricCard
                        IconComponent={TbRefresh}
                        iconColor="blue"
                        isLoading={isLoading}
                        title={t('replication.lastSequence')}
                        value={status?.last_sequence ?? 0}
                    />
                    <MetricCard
                        IconComponent={TbRefresh}
                        iconColor="grape"
                        isLoading={isLoading}
                        title={t('replication.primaryAddress')}
                        value={status?.primary_addr ?? '—'}
                    />
                </SimpleGrid>

                <Card>
                    <Stack gap="sm">
                        <Text fw={600} size="sm">
                            {t('replication.configure')}
                        </Text>
                        <Group>
                            <Button
                                disabled={status?.mode === 'primary'}
                                loading={primaryMutation.isPending}
                                onClick={() => primaryMutation.mutate()}
                                variant="light"
                            >
                                {t('replication.enablePrimary')}
                            </Button>
                            <Button
                                color="red"
                                disabled={status?.mode === 'disabled'}
                                loading={disableMutation.isPending}
                                onClick={() =>
                                    modals.openConfirmModal({
                                        title: t('replication.disableTitle'),
                                        children: t('replication.disableConfirm'),
                                        labels: { confirm: t('replication.disable'), cancel: t('common.cancel') },
                                        confirmProps: { color: 'red' },
                                        onConfirm: () => disableMutation.mutate()
                                    })
                                }
                                variant="light"
                            >
                                {t('replication.disable')}
                            </Button>
                        </Group>
                        <Group align="end">
                            <TextInput
                                label={t('replication.primaryAddressForSecondary')}
                                onChange={(e) => setPrimaryAddr(e.currentTarget.value)}
                                placeholder={t('replication.primaryAddrPlaceholder')}
                                style={{ flex: 1 }}
                                value={primaryAddr}
                            />
                            <Button
                                disabled={!primaryAddr}
                                loading={secondaryMutation.isPending}
                                onClick={() => secondaryMutation.mutate()}
                                variant="light"
                            >
                                {t('replication.enableSecondary')}
                            </Button>
                        </Group>
                    </Stack>
                </Card>

                <Text fw={700} size="sm">
                    {t('replication.walEntries')}
                </Text>
                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('replication.seq')}</Table.Th>
                            <Table.Th>{t('replication.operation')}</Table.Th>
                            <Table.Th>{t('cryptoEngines.key')}</Table.Th>
                            <Table.Th>{t('replication.timestamp')}</Table.Th>
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {(wal?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td colSpan={4}>
                                    <EmptyState icon={TbRefresh} label={t('common.noResults')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {wal?.map((entry) => (
                            <Table.Tr key={entry.seq}>
                                <Table.Td>{entry.seq}</Table.Td>
                                <Table.Td>
                                    <Badge color={entry.operation === 'put' ? 'teal' : 'red'} variant="light">
                                        {entry.operation}
                                    </Badge>
                                </Table.Td>
                                <Table.Td>
                                    <Text ff="monospace" size="xs">
                                        {entry.key}
                                    </Text>
                                </Table.Td>
                                <Table.Td>
                                    <Text c="dimmed" size="xs">
                                        {new Date(entry.timestamp).toLocaleString()}
                                    </Text>
                                </Table.Td>
                            </Table.Tr>
                        ))}
                    </Table.Tbody>
                </TableCard>
            </Stack>
        </Page>
    )
}
