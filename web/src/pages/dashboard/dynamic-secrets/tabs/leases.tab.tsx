import { ActionIcon, Badge, Group, Stack, Table, Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { TbRefresh, TbTrash } from 'react-icons/tb'

import { listLeases, renewLease, revokeLease } from '@shared/api/endpoints/leases'
import { TableCard } from '@shared/ui/table-card/table-card'

export function LeasesTab() {
    const { t } = useTranslation()
    const queryClient = useQueryClient()
    const { data: leases, isLoading } = useQuery({
        queryKey: ['leases', 'list'],
        queryFn: listLeases,
        refetchInterval: 10_000
    })

    const invalidate = () => queryClient.invalidateQueries({ queryKey: ['leases', 'list'] })

    const renewMutation = useMutation({
        mutationFn: (id: string) => renewLease(id),
        onSuccess: () => {
            invalidate()
            notifications.show({ color: 'teal', message: t('dynamicSecrets.leases.renewedMessage'), title: t('common.renewed') })
        },
        onError: () => notifications.show({ color: 'red', message: t('dynamicSecrets.leases.renewFailed'), title: t('common.error') })
    })

    const revokeMutation = useMutation({
        mutationFn: revokeLease,
        onSuccess: () => {
            invalidate()
            notifications.show({ color: 'teal', message: t('dynamicSecrets.leases.revokedMessage'), title: t('common.revoked') })
        }
    })

    return (
        <Stack gap="md">
            <Text c="dimmed" size="xs">
                {t('dynamicSecrets.leases.aggregatedNote')}
            </Text>
            <TableCard>
                <Table.Thead>
                    <Table.Tr>
                        <Table.Th>{t('dynamicSecrets.leases.leaseId')}</Table.Th>
                        <Table.Th>{t('dynamicSecrets.leases.backend')}</Table.Th>
                        <Table.Th>{t('dynamicSecrets.leases.expires')}</Table.Th>
                        <Table.Th>{t('dynamicSecrets.leases.status')}</Table.Th>
                        <Table.Th w={100} />
                    </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                    {!isLoading && (leases?.length ?? 0) === 0 && (
                        <Table.Tr>
                            <Table.Td colSpan={5}>
                                <Text c="dimmed" size="sm">
                                    {t('dynamicSecrets.leases.noActiveLeases')}
                                </Text>
                            </Table.Td>
                        </Table.Tr>
                    )}
                    {leases?.map((lease) => (
                        <Table.Tr key={lease.id}>
                            <Table.Td>
                                <Text ff="monospace" size="xs">
                                    {lease.id}
                                </Text>
                            </Table.Td>
                            <Table.Td>{lease.backend}</Table.Td>
                            <Table.Td>
                                <Text c="dimmed" size="xs">
                                    {new Date(lease.expires_at).toLocaleString()}
                                </Text>
                            </Table.Td>
                            <Table.Td>
                                <Badge color={lease.revoked ? 'red' : 'teal'} variant="light">
                                    {lease.revoked ? t('common.revoked') : t('kv2.active')}
                                </Badge>
                            </Table.Td>
                            <Table.Td>
                                <Group gap="xs" justify="flex-end">
                                    <ActionIcon
                                        disabled={lease.revoked}
                                        loading={renewMutation.isPending}
                                        onClick={() => renewMutation.mutate(lease.id)}
                                        variant="subtle"
                                    >
                                        <TbRefresh size={16} />
                                    </ActionIcon>
                                    <ActionIcon
                                        color="red"
                                        disabled={lease.revoked}
                                        onClick={() =>
                                            modals.openConfirmModal({
                                                title: t('dynamicSecrets.leases.revokeLeaseTitle'),
                                                children: t('dynamicSecrets.leases.revokeLeaseConfirm', { id: lease.id }),
                                                labels: { confirm: t('common.revoke'), cancel: t('common.cancel') },
                                                confirmProps: { color: 'red' },
                                                onConfirm: () => revokeMutation.mutate(lease.id)
                                            })
                                        }
                                        variant="subtle"
                                    >
                                        <TbTrash size={16} />
                                    </ActionIcon>
                                </Group>
                            </Table.Td>
                        </Table.Tr>
                    ))}
                </Table.Tbody>
            </TableCard>
        </Stack>
    )
}
