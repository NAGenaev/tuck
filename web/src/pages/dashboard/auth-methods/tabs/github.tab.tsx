import { ActionIcon, Button, Group, SimpleGrid, Stack, Table, Text, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbBrandGithub, TbPlus, TbTrash } from 'react-icons/tb'

import { deleteGitHubRole, listGitHubRoles, putGitHubRole } from '@shared/api/endpoints/auth-github'
import { humanToNs } from '@shared/utils/duration'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { TableCard } from '@shared/ui/table-card/table-card'

function RoleForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [repository, setRepository] = useState('')
    const [repoOwner, setRepoOwner] = useState('')
    const [ref, setRef] = useState('')
    const [policies, setPolicies] = useState('')
    const [ttl, setTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putGitHubRole(name, {
                repository: repository || undefined,
                repository_owner: repoOwner || undefined,
                ref: ref || undefined,
                policies: policies
                    .split(',')
                    .map((p) => p.trim())
                    .filter(Boolean),
                ttl: ttl ? humanToNs(ttl) : 0
            }),
        onSuccess: () => {
            notifications.show({
                color: 'teal',
                message: t('policies.savedMessage', { name }),
                title: t('common.saved')
            })
            onDone()
        },
        onError: () => notifications.show({ color: 'red', message: t('common.saveFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="sm">
            <Text c="dimmed" size="xs">
                {t('authMethods.github.bindNote')}
            </Text>
            <TextInput autoFocus label={t('authMethods.roleName')} onChange={(e) => setName(e.currentTarget.value)} value={name} />
            <Group grow>
                <TextInput
                    label={t('authMethods.github.repository')}
                    onChange={(e) => setRepository(e.currentTarget.value)}
                    placeholder={t('authMethods.github.repositoryPlaceholder')}
                    value={repository}
                />
                <TextInput
                    label={t('authMethods.github.repositoryOwner')}
                    onChange={(e) => setRepoOwner(e.currentTarget.value)}
                    value={repoOwner}
                />
                <TextInput
                    label={t('authMethods.github.ref')}
                    onChange={(e) => setRef(e.currentTarget.value)}
                    placeholder={t('authMethods.github.refPlaceholder')}
                    value={ref}
                />
            </Group>
            <TextInput
                label={t('tokens.policiesLabel')}
                onChange={(e) => setPolicies(e.currentTarget.value)}
                value={policies}
            />
            <TextInput
                label={t('tokens.ttl')}
                onChange={(e) => setTtl(e.currentTarget.value)}
                placeholder={t('authMethods.ttlPlaceholder1h')}
                value={ttl}
            />
            <Button
                disabled={!name}
                fullWidth
                leftSection={<TbPlus size={16} />}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('authMethods.saveRole')}
            </Button>
        </Stack>
    )
}

export function GitHubTab() {
    const { t } = useTranslation()
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
    const queryClient = useQueryClient()

    const { data: roles, isLoading } = useQuery({ queryKey: ['github', 'roles'], queryFn: listGitHubRoles })

    const deleteMutation = useMutation({
        mutationFn: deleteGitHubRole,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['github', 'roles'] })
            notifications.show({ color: 'teal', message: t('authMethods.roleDeleted'), title: t('common.deleted') })
        }
    })

    return (
        <Stack gap="md">
            <Group justify="flex-end">
                <Button leftSection={<TbPlus size={16} />} onClick={openCreating}>
                    {t('authMethods.newRole')}
                </Button>
            </Group>

            <SimpleGrid cols={{ base: 1, sm: 2 }}>
                <MetricCard
                    IconComponent={TbBrandGithub}
                    iconColor="dark"
                    isLoading={isLoading}
                    title={t('common.total')}
                    value={roles?.length ?? 0}
                />
            </SimpleGrid>

            <EntityModal color="dark" icon={TbBrandGithub} onClose={closeCreating} opened={creating} title={t('authMethods.newRole')}>
                <RoleForm
                    onDone={() => {
                        closeCreating()
                        queryClient.invalidateQueries({ queryKey: ['github', 'roles'] })
                    }}
                />
            </EntityModal>

            <TableCard>
                <Table.Thead>
                    <Table.Tr>
                        <Table.Th>{t('authMethods.role')}</Table.Th>
                        <Table.Th w={60} />
                    </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                    {!isLoading && (roles?.length ?? 0) === 0 && (
                        <Table.Tr>
                            <Table.Td colSpan={2}>
                                <EmptyState icon={TbBrandGithub} label={t('authMethods.noRoles')} />
                            </Table.Td>
                        </Table.Tr>
                    )}
                    {roles?.map((name) => (
                        <Table.Tr key={name}>
                            <Table.Td>{name}</Table.Td>
                            <Table.Td>
                                <ActionIcon
                                    color="red"
                                    onClick={() =>
                                        modals.openConfirmModal({
                                            title: t('authMethods.deleteRoleTitle'),
                                            children: t('authMethods.deleteRoleConfirm', {
                                                method: 'GitHub Actions',
                                                name
                                            }),
                                            labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
                                            confirmProps: { color: 'red' },
                                            onConfirm: () => deleteMutation.mutate(name)
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
    )
}
