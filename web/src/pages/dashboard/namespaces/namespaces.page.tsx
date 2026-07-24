import { ActionIcon, Button, SimpleGrid, Stack, Table, Text, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbBoxMultiple, TbPlus, TbTrash } from 'react-icons/tb'

import { createNamespace, deleteNamespace, listNamespaces } from '@shared/api/endpoints/namespace'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

export function NamespacesPage() {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
    const queryClient = useQueryClient()

    const { data: namespaces, isLoading } = useQuery({
        queryKey: ['namespaces', 'list'],
        queryFn: listNamespaces
    })

    const createMutation = useMutation({
        mutationFn: () => createNamespace(name),
        onSuccess: () => {
            setName('')
            closeCreating()
            queryClient.invalidateQueries({ queryKey: ['namespaces', 'list'] })
            notifications.show({ color: 'teal', message: t('namespaces.createdMessage'), title: t('common.created') })
        },
        onError: () => notifications.show({ color: 'red', message: t('common.createFailed'), title: t('common.error') })
    })

    const deleteMutation = useMutation({
        mutationFn: deleteNamespace,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['namespaces', 'list'] })
            notifications.show({ color: 'teal', message: t('namespaces.deletedMessage'), title: t('common.deleted') })
        }
    })

    return (
        <Page title="Namespaces">
            <Stack gap="lg">
                <PageHeader
                    action={
                        <Button leftSection={<TbPlus size={16} />} onClick={openCreating}>
                            {t('common.new')}
                        </Button>
                    }
                    color="grape"
                    icon={TbBoxMultiple}
                    title={t('pages.namespaces')}
                />

                <SimpleGrid cols={{ base: 1, sm: 2 }}>
                    <MetricCard
                        IconComponent={TbBoxMultiple}
                        iconColor="grape"
                        isLoading={isLoading}
                        title={t('common.total')}
                        value={namespaces?.length ?? 0}
                    />
                </SimpleGrid>

                <EntityModal color="grape" icon={TbBoxMultiple} onClose={closeCreating} opened={creating} title={t('common.new')}>
                    <Stack gap="sm">
                        <Text c="dimmed" size="xs">
                            {t('namespaces.createHint')}
                        </Text>
                        <TextInput
                            autoFocus
                            label={t('common.name')}
                            onChange={(e) => setName(e.currentTarget.value)}
                            placeholder={t('namespaces.namePlaceholder')}
                            value={name}
                        />
                        <Button
                            disabled={!name}
                            fullWidth
                            leftSection={<TbPlus size={16} />}
                            loading={createMutation.isPending}
                            onClick={() => createMutation.mutate()}
                        >
                            {t('common.create')}
                        </Button>
                    </Stack>
                </EntityModal>

                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('common.name')}</Table.Th>
                            <Table.Th w={60} />
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {!isLoading && (namespaces?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td colSpan={2}>
                                    <EmptyState icon={TbBoxMultiple} label={t('namespaces.noNamespaces')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {namespaces?.map((ns) => (
                            <Table.Tr key={ns}>
                                <Table.Td>{ns}</Table.Td>
                                <Table.Td>
                                    <ActionIcon
                                        color="red"
                                        onClick={() =>
                                            modals.openConfirmModal({
                                                title: t('namespaces.deleteTitle'),
                                                children: t('namespaces.deleteConfirm', { name: ns }),
                                                labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
                                                confirmProps: { color: 'red' },
                                                onConfirm: () => deleteMutation.mutate(ns)
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
