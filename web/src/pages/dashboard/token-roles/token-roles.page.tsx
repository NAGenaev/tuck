import {
    ActionIcon,
    Alert,
    Button,
    Checkbox,
    Group,
    NumberInput,
    SimpleGrid,
    Stack,
    Table,
    TextInput
} from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbPlus, TbTrash, TbUserCog, TbUserPlus } from 'react-icons/tb'

import {
    createTokenFromRole,
    deleteTokenRole,
    listTokenRoles,
    putTokenRole
} from '@shared/api/endpoints/token-roles'
import { Token } from '@shared/api/endpoints/token'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

function RoleForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [policies, setPolicies] = useState('')
    const [ttl, setTtl] = useState('')
    const [maxTtl, setMaxTtl] = useState('')
    const [maxUses, setMaxUses] = useState<number | string>('')
    const [renewable, setRenewable] = useState(true)

    const mutation = useMutation({
        mutationFn: () =>
            putTokenRole(name, {
                policies: policies
                    .split(',')
                    .map((p) => p.trim())
                    .filter(Boolean),
                ttl: ttl || undefined,
                max_ttl: maxTtl || undefined,
                max_uses: maxUses === '' ? undefined : Number(maxUses),
                renewable
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
            <TextInput autoFocus label={t('authMethods.roleName')} onChange={(e) => setName(e.currentTarget.value)} value={name} />
            <TextInput
                label={t('tokens.policiesLabel')}
                onChange={(e) => setPolicies(e.currentTarget.value)}
                value={policies}
            />
            <Group grow>
                <TextInput
                    label={t('tokens.ttl')}
                    onChange={(e) => setTtl(e.currentTarget.value)}
                    placeholder={t('authMethods.ttlPlaceholder1h')}
                    value={ttl}
                />
                <TextInput
                    label={t('cryptoEngines.pki.maxTtl')}
                    onChange={(e) => setMaxTtl(e.currentTarget.value)}
                    placeholder={t('tokens.ttlPlaceholder')}
                    value={maxTtl}
                />
                <NumberInput
                    label={t('tokenRoles.maxUses')}
                    onChange={setMaxUses}
                    placeholder={t('tokenRoles.maxUsesPlaceholder')}
                    value={maxUses}
                />
            </Group>
            <Checkbox
                checked={renewable}
                label={t('tokenRoles.renewable')}
                onChange={(e) => setRenewable(e.currentTarget.checked)}
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

export function TokenRolesPage() {
    const { t } = useTranslation()
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
    const [newToken, setNewToken] = useState<Token | null>(null)
    const queryClient = useQueryClient()

    const { data: roles, isLoading } = useQuery({
        queryKey: ['token-roles', 'list'],
        queryFn: listTokenRoles
    })

    const deleteMutation = useMutation({
        mutationFn: deleteTokenRole,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['token-roles', 'list'] })
    })

    const createTokenMutation = useMutation({
        mutationFn: (role: string) => createTokenFromRole(role),
        onSuccess: setNewToken,
        onError: () => notifications.show({ color: 'red', message: t('common.createFailed'), title: t('common.error') })
    })

    return (
        <Page title="Token Roles">
            <Stack gap="lg">
                <PageHeader
                    action={
                        <Button leftSection={<TbPlus size={16} />} onClick={openCreating}>
                            {t('common.new')}
                        </Button>
                    }
                    color="indigo"
                    icon={TbUserCog}
                    title={t('pages.tokenRoles')}
                />

                <SimpleGrid cols={{ base: 1, sm: 2 }}>
                    <MetricCard
                        IconComponent={TbUserCog}
                        iconColor="indigo"
                        isLoading={isLoading}
                        title={t('common.total')}
                        value={roles?.length ?? 0}
                    />
                </SimpleGrid>

                <EntityModal color="indigo" icon={TbUserCog} onClose={closeCreating} opened={creating} title={t('authMethods.newRole')}>
                    <RoleForm
                        onDone={() => {
                            closeCreating()
                            queryClient.invalidateQueries({ queryKey: ['token-roles', 'list'] })
                        }}
                    />
                </EntityModal>

                {newToken && (
                    <Alert
                        color="yellow"
                        onClose={() => setNewToken(null)}
                        title={t('tokens.copyNowTitle')}
                        variant="light"
                        withCloseButton
                    >
                        <CopyableField value={newToken.id} />
                    </Alert>
                )}

                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('authMethods.role')}</Table.Th>
                            <Table.Th w={100} />
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {!isLoading && (roles?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td colSpan={2}>
                                    <EmptyState icon={TbUserCog} label={t('authMethods.noRoles')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {roles?.map((name) => (
                            <Table.Tr key={name}>
                                <Table.Td>{name}</Table.Td>
                                <Table.Td>
                                    <Group gap="xs" justify="flex-end">
                                        <ActionIcon
                                            loading={createTokenMutation.isPending}
                                            onClick={() => createTokenMutation.mutate(name)}
                                            variant="subtle"
                                        >
                                            <TbUserPlus size={16} />
                                        </ActionIcon>
                                        <ActionIcon
                                            color="red"
                                            onClick={() =>
                                                modals.openConfirmModal({
                                                    title: t('authMethods.deleteRoleTitle'),
                                                    children: t('authMethods.deleteRoleConfirm', {
                                                        method: 'token',
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
