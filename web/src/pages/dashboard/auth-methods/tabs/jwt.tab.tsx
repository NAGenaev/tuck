import { ActionIcon, Button, Card, Group, Stack, Table, Text, TextInput } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbPlus, TbTrash } from 'react-icons/tb'

import {
    deleteJWTRole,
    getJWTConfig,
    getJWTRole,
    listJWTRoles,
    putJWTConfig,
    putJWTRole
} from '@shared/api/endpoints/auth-jwt'
import { TableCard } from '@shared/ui/table-card/table-card'

function ConfigForm() {
    const { t } = useTranslation()
    const { data } = useQuery({ queryKey: ['jwt', 'config'], queryFn: getJWTConfig })
    const [jwksUri, setJwksUri] = useState('')
    const [issuer, setIssuer] = useState('')
    const [audience, setAudience] = useState('')

    useEffect(() => {
        if (data) {
            setJwksUri(data.jwks_uri ?? '')
            setIssuer(data.issuer ?? '')
            setAudience(data.audience ?? '')
        }
    }, [data])

    const mutation = useMutation({
        mutationFn: () => putJWTConfig({ jwks_uri: jwksUri, issuer, audience }),
        onSuccess: () =>
            notifications.show({ color: 'teal', message: t('authMethods.configSaved'), title: t('common.saved') }),
        onError: () => notifications.show({ color: 'red', message: t('common.saveFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('authMethods.jwt.providerConfig')}
                </Text>
                <TextInput
                    label={t('authMethods.jwt.jwksUri')}
                    onChange={(e) => setJwksUri(e.currentTarget.value)}
                    placeholder={t('authMethods.jwt.jwksUriPlaceholder')}
                    value={jwksUri}
                />
                <Group grow>
                    <TextInput
                        label={t('authMethods.jwt.issuer')}
                        onChange={(e) => setIssuer(e.currentTarget.value)}
                        value={issuer}
                    />
                    <TextInput
                        label={t('authMethods.jwt.audience')}
                        onChange={(e) => setAudience(e.currentTarget.value)}
                        value={audience}
                    />
                </Group>
                <Button
                    disabled={!jwksUri}
                    loading={mutation.isPending}
                    onClick={() => mutation.mutate()}
                >
                    {t('authMethods.jwt.saveConfig')}
                </Button>
            </Stack>
        </Card>
    )
}

function RoleForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [policies, setPolicies] = useState('')
    const [boundSubject, setBoundSubject] = useState('')
    const [ttl, setTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putJWTRole(name, {
                policies: policies
                    .split(',')
                    .map((p) => p.trim())
                    .filter(Boolean),
                bound_subject: boundSubject || undefined,
                ttl: ttl || undefined
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
                        label={t('authMethods.jwt.boundSubject')}
                        onChange={(e) => setBoundSubject(e.currentTarget.value)}
                        value={boundSubject}
                    />
                    <TextInput
                        label={t('tokens.ttl')}
                        onChange={(e) => setTtl(e.currentTarget.value)}
                        placeholder={t('authMethods.ttlPlaceholder1h')}
                        value={ttl}
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

export function JWTTab() {
    const { t } = useTranslation()
    const [creating, setCreating] = useState(false)
    const queryClient = useQueryClient()

    const { data: roles, isLoading } = useQuery({ queryKey: ['jwt', 'roles'], queryFn: listJWTRoles })

    const deleteMutation = useMutation({
        mutationFn: deleteJWTRole,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['jwt', 'roles'] })
            notifications.show({ color: 'teal', message: t('authMethods.roleDeleted'), title: t('common.deleted') })
        }
    })

    return (
        <Stack gap="md">
            <ConfigForm />

            <Group justify="space-between">
                <Text fw={700} size="sm">
                    {t('authMethods.rolesTitle')}
                </Text>
                <Button leftSection={<TbPlus size={16} />} onClick={() => setCreating((c) => !c)} variant="light">
                    {t('authMethods.newRole')}
                </Button>
            </Group>

            {creating && (
                <RoleForm
                    onDone={() => {
                        setCreating(false)
                        queryClient.invalidateQueries({ queryKey: ['jwt', 'roles'] })
                    }}
                />
            )}

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
                            <Table.Td>{name}</Table.Td>
                            <Table.Td>
                                <ActionIcon
                                    color="red"
                                    onClick={() =>
                                        modals.openConfirmModal({
                                            title: t('authMethods.deleteRoleTitle'),
                                            children: t('authMethods.deleteRoleConfirm', { method: 'JWT', name }),
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
