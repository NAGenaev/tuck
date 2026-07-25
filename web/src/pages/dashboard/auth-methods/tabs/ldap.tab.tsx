import {
    ActionIcon,
    Button,
    Card,
    Group,
    PasswordInput,
    SimpleGrid,
    Stack,
    Table,
    Text,
    TextInput
} from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbAddressBook, TbPlus, TbTrash } from 'react-icons/tb'

import {
    deleteLDAPRole,
    getLDAPConfig,
    listLDAPRoles,
    putLDAPConfig,
    putLDAPRole
} from '@shared/api/endpoints/auth-ldap'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { TableCard } from '@shared/ui/table-card/table-card'

function ConfigForm() {
    const { t } = useTranslation()
    const { data } = useQuery({ queryKey: ['ldap', 'config'], queryFn: getLDAPConfig })
    const [urls, setUrls] = useState('')
    const [userDn, setUserDn] = useState('')
    const [bindDn, setBindDn] = useState('')
    const [bindPassword, setBindPassword] = useState('')
    const [groupDn, setGroupDn] = useState('')

    useEffect(() => {
        if (data) {
            setUrls((data.urls ?? []).join(', '))
            setUserDn(data.user_dn ?? '')
            setBindDn(data.bind_dn ?? '')
            setGroupDn(data.group_dn ?? '')
        }
    }, [data])

    const mutation = useMutation({
        mutationFn: () =>
            putLDAPConfig({
                urls: urls
                    .split(',')
                    .map((u) => u.trim())
                    .filter(Boolean),
                user_dn: userDn,
                bind_dn: bindDn || undefined,
                bind_password: bindPassword || undefined,
                group_dn: groupDn || undefined
            }),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('authMethods.configSaved'), title: t('common.saved') })
            setBindPassword('')
        },
        onError: () => notifications.show({ color: 'red', message: t('common.saveFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('authMethods.ldap.ldapConfig')}
                </Text>
                <TextInput
                    label={t('authMethods.ldap.serverUrls')}
                    onChange={(e) => setUrls(e.currentTarget.value)}
                    placeholder={t('authMethods.ldap.serverUrlsPlaceholder')}
                    value={urls}
                />
                <TextInput
                    label={t('authMethods.ldap.userDn')}
                    onChange={(e) => setUserDn(e.currentTarget.value)}
                    placeholder={t('authMethods.ldap.userDnPlaceholder')}
                    value={userDn}
                />
                <Group grow>
                    <TextInput
                        label={t('authMethods.ldap.bindDn')}
                        onChange={(e) => setBindDn(e.currentTarget.value)}
                        value={bindDn}
                    />
                    <PasswordInput
                        label={t('authMethods.ldap.bindPassword')}
                        onChange={(e) => setBindPassword(e.currentTarget.value)}
                        placeholder={t('authMethods.ldap.bindPasswordPlaceholder')}
                        value={bindPassword}
                    />
                </Group>
                <TextInput
                    label={t('authMethods.ldap.groupDn')}
                    onChange={(e) => setGroupDn(e.currentTarget.value)}
                    value={groupDn}
                />
                <Button
                    disabled={!userDn || !urls}
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
    const [groups, setGroups] = useState('')
    const [policies, setPolicies] = useState('')
    const [ttl, setTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putLDAPRole(name, {
                groups: groups
                    .split(',')
                    .map((g) => g.trim())
                    .filter(Boolean),
                policies: policies
                    .split(',')
                    .map((p) => p.trim())
                    .filter(Boolean),
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
                label={t('authMethods.ldap.groupsLabel')}
                onChange={(e) => setGroups(e.currentTarget.value)}
                value={groups}
            />
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

export function LDAPTab() {
    const { t } = useTranslation()
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
    const queryClient = useQueryClient()

    const { data: roles, isLoading } = useQuery({ queryKey: ['ldap', 'roles'], queryFn: listLDAPRoles })

    const deleteMutation = useMutation({
        mutationFn: deleteLDAPRole,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['ldap', 'roles'] })
            notifications.show({ color: 'teal', message: t('authMethods.roleDeleted'), title: t('common.deleted') })
        }
    })

    return (
        <Stack gap="md">
            <ConfigForm />

            <SimpleGrid cols={{ base: 1, sm: 2 }}>
                <MetricCard
                    IconComponent={TbAddressBook}
                    iconColor="indigo"
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

            <EntityModal color="indigo" icon={TbAddressBook} onClose={closeCreating} opened={creating} title={t('authMethods.newRole')}>
                <RoleForm
                    onDone={() => {
                        closeCreating()
                        queryClient.invalidateQueries({ queryKey: ['ldap', 'roles'] })
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
                                <EmptyState icon={TbAddressBook} label={t('authMethods.noRoles')} />
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
                                            children: t('authMethods.deleteRoleConfirm', { method: 'LDAP', name }),
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
