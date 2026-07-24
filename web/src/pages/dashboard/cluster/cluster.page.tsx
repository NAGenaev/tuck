import { ActionIcon, Badge, Button, Card, Group, SimpleGrid, Stack, Table, Text, TextInput } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbCrown, TbServer2, TbTrash } from 'react-icons/tb'

import { getClusterStatus, joinCluster, removeClusterNode } from '@shared/api/endpoints/cluster'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

function JoinForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [nodeId, setNodeId] = useState('')
    const [raftAddr, setRaftAddr] = useState('')

    const mutation = useMutation({
        mutationFn: () => joinCluster(nodeId, raftAddr),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('cluster.joinedMessage', { nodeId }), title: t('common.joined') })
            onDone()
        },
        onError: () => notifications.show({ color: 'red', message: t('cluster.joinFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('cluster.joinNode')}
                </Text>
                <Group grow>
                    <TextInput label={t('cluster.nodeId')} onChange={(e) => setNodeId(e.currentTarget.value)} value={nodeId} />
                    <TextInput
                        label={t('cluster.raftAddress')}
                        onChange={(e) => setRaftAddr(e.currentTarget.value)}
                        placeholder={t('cluster.raftAddressPlaceholder')}
                        value={raftAddr}
                    />
                </Group>
                <Button
                    disabled={!nodeId || !raftAddr}
                    loading={mutation.isPending}
                    onClick={() => mutation.mutate()}
                    variant="light"
                >
                    {t('cluster.join')}
                </Button>
            </Stack>
        </Card>
    )
}

export function ClusterPage() {
    const { t } = useTranslation()
    const queryClient = useQueryClient()

    const { data: status, isLoading } = useQuery({
        queryKey: ['cluster', 'status'],
        queryFn: getClusterStatus,
        refetchInterval: 5_000,
        retry: false
    })

    const removeMutation = useMutation({
        mutationFn: removeClusterNode,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['cluster', 'status'] })
    })

    if (!isLoading && !status) {
        return (
            <Page title="Cluster">
                <Stack gap="lg">
                    <PageHeader color="grape" icon={TbServer2} title={t('pages.cluster')} />
                    <EmptyState icon={TbServer2} label={t('cluster.notEnabled')} />
                </Stack>
            </Page>
        )
    }

    return (
        <Page title="Cluster">
            <Stack gap="lg">
                <PageHeader color="grape" icon={TbServer2} title={t('pages.cluster')} />

                <SimpleGrid cols={{ base: 1, sm: 3 }}>
                    <MetricCard
                        IconComponent={TbCrown}
                        iconColor={status?.is_leader ? 'yellow' : 'gray'}
                        isLoading={isLoading}
                        title={t('authMethods.role')}
                        value={status?.is_leader ? t('cluster.leader') : t('cluster.follower')}
                    />
                    <MetricCard
                        IconComponent={TbServer2}
                        iconColor="blue"
                        isLoading={isLoading}
                        title={t('cluster.leaderAddress')}
                        value={status?.leader_addr ?? '—'}
                    />
                    <MetricCard
                        IconComponent={TbServer2}
                        iconColor="teal"
                        isLoading={isLoading}
                        title={t('cluster.state')}
                        value={status?.state ?? '—'}
                    />
                </SimpleGrid>

                <JoinForm onDone={() => queryClient.invalidateQueries({ queryKey: ['cluster', 'status'] })} />

                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('cluster.nodeId')}</Table.Th>
                            <Table.Th>{t('cluster.address')}</Table.Th>
                            <Table.Th>{t('cluster.suffrage')}</Table.Th>
                            <Table.Th w={60} />
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {(status?.servers?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td colSpan={4}>
                                    <EmptyState icon={TbServer2} label={t('common.noResults')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {status?.servers.map((server) => (
                            <Table.Tr key={server.id}>
                                <Table.Td>{server.id}</Table.Td>
                                <Table.Td>{server.address}</Table.Td>
                                <Table.Td>
                                    <Badge color="gray" variant="light">
                                        {server.suffrage}
                                    </Badge>
                                </Table.Td>
                                <Table.Td>
                                    <ActionIcon
                                        color="red"
                                        onClick={() =>
                                            modals.openConfirmModal({
                                                title: t('cluster.removeNodeTitle'),
                                                children: t('cluster.removeNodeConfirm', { id: server.id }),
                                                labels: { confirm: t('cluster.remove'), cancel: t('common.cancel') },
                                                confirmProps: { color: 'red' },
                                                onConfirm: () => removeMutation.mutate(server.id)
                                            })
                                        }
                                        variant="subtle"
                                    >
                                        <TbTrash size={16} />
                                    </ActionIcon>
                                </Table.Td>
                            </Table.Tr>
                        ))}
                    </Table.Tbody>
                </TableCard>
            </Stack>
        </Page>
    )
}
