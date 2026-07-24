import {
    ActionIcon,
    Alert,
    Anchor,
    Button,
    Card,
    Group,
    Stack,
    Table,
    Text,
    TextInput
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbPlus, TbTrash } from 'react-icons/tb'

import {
    AppRoleRole,
    deleteAppRole,
    generateSecretId,
    getAppRole,
    listAppRoles,
    putAppRole,
    SecretID
} from '@shared/api/endpoints/auth-approle'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { TableCard } from '@shared/ui/table-card/table-card'

function RoleForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [policies, setPolicies] = useState('')
    const [tokenTtl, setTokenTtl] = useState('')
    const [secretIdTtl, setSecretIdTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putAppRole(name, {
                policies: policies
                    .split(',')
                    .map((p) => p.trim())
                    .filter(Boolean),
                token_ttl: tokenTtl || undefined,
                secret_id_ttl: secretIdTtl || undefined
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
        <Card>
            <Stack gap="sm">
                <TextInput
                    label={t('authMethods.roleName')}
                    onChange={(e) => setName(e.currentTarget.value)}
                    value={name}
                />
                <TextInput
                    label={t('tokens.policiesLabel')}
                    onChange={(e) => setPolicies(e.currentTarget.value)}
                    value={policies}
                />
                <Group grow>
                    <TextInput
                        label={t('authMethods.approle.tokenTtl')}
                        onChange={(e) => setTokenTtl(e.currentTarget.value)}
                        placeholder={t('authMethods.ttlPlaceholder1h')}
                        value={tokenTtl}
                    />
                    <TextInput
                        label={t('authMethods.approle.secretIdTtl')}
                        onChange={(e) => setSecretIdTtl(e.currentTarget.value)}
                        placeholder={t('tokens.ttlPlaceholder')}
                        value={secretIdTtl}
                    />
                </Group>
                <Button
                    disabled={!name}
                    fullWidth
                    loading={mutation.isPending}
                    onClick={() => mutation.mutate()}
                >
                    {t('authMethods.saveRole')}
                </Button>
            </Stack>
        </Card>
    )
}

function RoleDetail({ name, onClose }: { name: string; onClose: () => void }) {
    const { t } = useTranslation()
    const [secretId, setSecretId] = useState<null | SecretID>(null)
    const { data } = useQuery({ queryKey: ['approle', name], queryFn: () => getAppRole(name) })

    const genMutation = useMutation({
        mutationFn: () => generateSecretId(name),
        onSuccess: setSecretId,
        onError: () =>
            notifications.show({ color: 'red', message: t('common.generateFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Group justify="space-between">
                    <Text fw={600}>{name}</Text>
                    <Anchor component="button" onClick={onClose} size="sm">
                        {t('common.close')}
                    </Anchor>
                </Group>
                {data && <CopyableField label={t('authMethods.approle.roleId')} value={data.role_id} />}
                <Button loading={genMutation.isPending} onClick={() => genMutation.mutate()} variant="light">
                    {t('authMethods.approle.generateSecretId')}
                </Button>
                {secretId && (
                    <Alert color="yellow" title={t('authMethods.approle.copySecretIdTitle')} variant="light">
                        <CopyableField value={secretId.id} />
                    </Alert>
                )}
            </Stack>
        </Card>
    )
}

export function AppRoleTab() {
    const { t } = useTranslation()
    const [creating, setCreating] = useState(false)
    const [selected, setSelected] = useState<null | string>(null)
    const queryClient = useQueryClient()

    const { data: roles, isLoading } = useQuery({
        queryKey: ['approle', 'list'],
        queryFn: listAppRoles
    })

    const deleteMutation = useMutation({
        mutationFn: deleteAppRole,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['approle', 'list'] })
            notifications.show({ color: 'teal', message: t('authMethods.roleDeleted'), title: t('common.deleted') })
        }
    })

    return (
        <Stack gap="md">
            <Group justify="flex-end">
                <Button leftSection={<TbPlus size={16} />} onClick={() => setCreating((c) => !c)} variant="light">
                    {t('authMethods.newRole')}
                </Button>
            </Group>

            {creating && (
                <RoleForm
                    onDone={() => {
                        setCreating(false)
                        queryClient.invalidateQueries({ queryKey: ['approle', 'list'] })
                    }}
                />
            )}

            {selected && <RoleDetail name={selected} onClose={() => setSelected(null)} />}

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
                                <Text c="dimmed" size="sm">
                                    {t('authMethods.noRoles')}
                                </Text>
                            </Table.Td>
                        </Table.Tr>
                    )}
                    {roles?.map((name) => (
                        <Table.Tr key={name}>
                            <Table.Td>
                                <Anchor component="button" onClick={() => setSelected(name)} size="sm">
                                    {name}
                                </Anchor>
                            </Table.Td>
                            <Table.Td>
                                <ActionIcon
                                    color="red"
                                    onClick={() =>
                                        modals.openConfirmModal({
                                            title: t('authMethods.deleteRoleTitle'),
                                            children: t('authMethods.deleteRoleConfirm', { method: 'AppRole', name }),
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
