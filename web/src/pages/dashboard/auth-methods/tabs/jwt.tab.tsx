import { ActionIcon, Button, Card, Group, SimpleGrid, Stack, Table, Text, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbPlus, TbTrash, TbUserCog } from 'react-icons/tb'

import {
    deleteJWTRole,
    getJWTConfig,
    getJWTRole,
    listJWTRoles,
    putJWTConfig,
    putJWTRole
} from '@shared/api/endpoints/auth-jwt'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
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
        <Stack gap="sm">
            <TextInput
                autoFocus
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
                leftSection={<TbPlus size={16} />}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('authMethods.saveRole')}
            </Button>
        </Stack>
    )
}

export function JWTTab() {
    const { t } = useTranslation()
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
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

            <SimpleGrid cols={{ base: 1, sm: 2 }}>
                <MetricCard
                    IconComponent={TbUserCog}
                    iconColor="cyan"
                    isLoading={isLoading}
                    title={t('common.total')}
                    value={roles?.length ?? 0}
                />
            </SimpleGrid>

            <Group justify="space-between">
                <Text fw={700} size="sm">
                    {t('authMethods.rolesTitle')}
                </Text>
                <Button leftSection={<TbPlus size={16} />} onClick={openCreating}>
                    {t('authMethods.newRole')}
                </Button>
            </Group>

            <EntityModal color="cyan" icon={TbUserCog} onClose={closeCreating} opened={creating} title={t('authMethods.newRole')}>
                <RoleForm
                    onDone={() => {
                        closeCreating()
                        queryClient.invalidateQueries({ queryKey: ['jwt', 'roles'] })
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
                                <EmptyState icon={TbUserCog} label={t('authMethods.noRoles')} />
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
