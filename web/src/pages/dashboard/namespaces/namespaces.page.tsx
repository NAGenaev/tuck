import { ActionIcon, Button, Group, Stack, Table, Text, TextInput } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbBoxMultiple, TbPlus, TbTrash } from 'react-icons/tb'

import { createNamespace, deleteNamespace, listNamespaces } from '@shared/api/endpoints/namespace'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

export function NamespacesPage() {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const queryClient = useQueryClient()

    const { data: namespaces, isLoading } = useQuery({
        queryKey: ['namespaces', 'list'],
        queryFn: listNamespaces
    })

    const createMutation = useMutation({
        mutationFn: () => createNamespace(name),
        onSuccess: () => {
            setName('')
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
                <PageHeader color="grape" icon={TbBoxMultiple} title={t('pages.namespaces')} />

                <Group>
                    <TextInput
                        onChange={(e) => setName(e.currentTarget.value)}
                        placeholder={t('namespaces.namePlaceholder')}
                        style={{ flex: 1 }}
                        value={name}
                    />
                    <Button
                        disabled={!name}
                        leftSection={<TbPlus size={16} />}
                        loading={createMutation.isPending}
                        onClick={() => createMutation.mutate()}
                    >
                        {t('common.create')}
                    </Button>
                </Group>

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
                                    <Text c="dimmed" size="sm">
                                        {t('namespaces.noNamespaces')}
                                    </Text>
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
