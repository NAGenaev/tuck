import { ActionIcon, Alert, Button, Group, SimpleGrid, Stack, Table, Text, TextInput, Textarea } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbBolt, TbDatabase, TbPlus, TbTrash } from 'react-icons/tb'

import {
    DBCredentials,
    deleteDBConfig,
    deleteDBRole,
    generateDBCreds,
    listDBConfigs,
    listDBRoles,
    putDBConfig,
    putDBRole
} from '@shared/api/endpoints/dynamic-database'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { TableCard } from '@shared/ui/table-card/table-card'

function ConfigForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [pluginName, setPluginName] = useState('postgresql')
    const [connectionUrl, setConnectionUrl] = useState('')
    const [database, setDatabase] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putDBConfig(name, {
                plugin_name: pluginName,
                connection_url: connectionUrl,
                database: database || undefined
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
                label={t('dynamicSecrets.database.configName')}
                onChange={(e) => setName(e.currentTarget.value)}
                value={name}
            />
            <TextInput
                label={t('dynamicSecrets.database.pluginName')}
                onChange={(e) => setPluginName(e.currentTarget.value)}
                placeholder={t('dynamicSecrets.database.pluginNamePlaceholder')}
                value={pluginName}
            />
            <TextInput
                label={t('dynamicSecrets.database.connectionUrl')}
                onChange={(e) => setConnectionUrl(e.currentTarget.value)}
                placeholder={t('dynamicSecrets.database.connectionUrlPlaceholder')}
                value={connectionUrl}
            />
            <TextInput
                description={t('dynamicSecrets.database.databaseNameHint')}
                label={t('dynamicSecrets.database.databaseName')}
                onChange={(e) => setDatabase(e.currentTarget.value)}
                placeholder={name || t('dynamicSecrets.database.configName')}
                value={database}
            />
            <Button
                disabled={!name || !connectionUrl}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('authMethods.jwt.saveConfig')}
            </Button>
        </Stack>
    )
}

function RoleForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [dbName, setDbName] = useState('')
    const [creation, setCreation] = useState('')
    const [defaultTtl, setDefaultTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putDBRole(name, {
                db_name: dbName,
                creation_statements: creation || undefined,
                default_ttl: defaultTtl || undefined
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
                label={t('dynamicSecrets.database.dbNameLabel')}
                onChange={(e) => setDbName(e.currentTarget.value)}
                value={dbName}
            />
            <Textarea
                label={t('dynamicSecrets.database.creationStatements')}
                onChange={(e) => setCreation(e.currentTarget.value)}
                placeholder={t('dynamicSecrets.database.creationStatementsPlaceholder')}
                value={creation}
            />
            <TextInput
                label={t('dynamicSecrets.defaultTtl')}
                onChange={(e) => setDefaultTtl(e.currentTarget.value)}
                placeholder={t('authMethods.ttlPlaceholder1h')}
                value={defaultTtl}
            />
            <Button
                disabled={!name || !dbName}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('authMethods.saveRole')}
            </Button>
        </Stack>
    )
}

export function DatabaseTab() {
    const { t } = useTranslation()
    const [creatingConfig, { open: openCreatingConfig, close: closeCreatingConfig }] = useDisclosure(false)
    const [creatingRole, { open: openCreatingRole, close: closeCreatingRole }] = useDisclosure(false)
    const [creds, setCreds] = useState<null | DBCredentials>(null)
    const queryClient = useQueryClient()

    const { data: configs, isLoading: configsLoading } = useQuery({ queryKey: ['db', 'configs'], queryFn: listDBConfigs })
    const { data: roles, isLoading: rolesLoading } = useQuery({ queryKey: ['db', 'roles'], queryFn: listDBRoles })

    const deleteConfigMutation = useMutation({
        mutationFn: deleteDBConfig,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['db', 'configs'] })
    })
    const deleteRoleMutation = useMutation({
        mutationFn: deleteDBRole,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['db', 'roles'] })
    })
    const genMutation = useMutation({
        mutationFn: generateDBCreds,
        onSuccess: setCreds,
        onError: () => notifications.show({ color: 'red', message: t('common.generateFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="md">
            <Group justify="space-between">
                <Text fw={700} size="sm">
                    {t('dynamicSecrets.connections')}
                </Text>
                <Button leftSection={<TbPlus size={16} />} onClick={openCreatingConfig}>
                    {t('dynamicSecrets.newConnection')}
                </Button>
            </Group>

            <SimpleGrid cols={{ base: 1, sm: 2 }}>
                <MetricCard
                    IconComponent={TbDatabase}
                    iconColor="cyan"
                    isLoading={configsLoading}
                    title={t('dynamicSecrets.connections')}
                    value={configs?.length ?? 0}
                />
            </SimpleGrid>

            <EntityModal color="cyan" icon={TbDatabase} onClose={closeCreatingConfig} opened={creatingConfig} title={t('dynamicSecrets.newConnection')}>
                <ConfigForm
                    onDone={() => {
                        closeCreatingConfig()
                        queryClient.invalidateQueries({ queryKey: ['db', 'configs'] })
                    }}
                />
            </EntityModal>

            <TableCard>
                <Table.Thead>
                    <Table.Tr>
                        <Table.Th>{t('common.name')}</Table.Th>
                        <Table.Th w={60} />
                    </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                    {!configsLoading && (configs?.length ?? 0) === 0 && (
                        <Table.Tr>
                            <Table.Td colSpan={2}>
                                <EmptyState icon={TbDatabase} label={t('dynamicSecrets.noConnections')} />
                            </Table.Td>
                        </Table.Tr>
                    )}
                    {configs?.map((name) => (
                        <Table.Tr key={name}>
                            <Table.Td>{name}</Table.Td>
                            <Table.Td>
                                <ActionIcon
                                    color="red"
                                    onClick={() =>
                                        modals.openConfirmModal({
                                            title: t('dynamicSecrets.deleteConnectionTitle'),
                                            children: t('dynamicSecrets.deleteConnectionConfirm', {
                                                backend: 'database',
                                                name
                                            }),
                                            labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
                                            confirmProps: { color: 'red' },
                                            onConfirm: () => deleteConfigMutation.mutate(name)
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

            <Group justify="space-between">
                <Text fw={700} size="sm">
                    {t('authMethods.rolesTitle')}
                </Text>
                <Button leftSection={<TbPlus size={16} />} onClick={openCreatingRole}>
                    {t('authMethods.newRole')}
                </Button>
            </Group>

            <SimpleGrid cols={{ base: 1, sm: 2 }}>
                <MetricCard
                    IconComponent={TbDatabase}
                    iconColor="grape"
                    isLoading={rolesLoading}
                    title={t('authMethods.rolesTitle')}
                    value={roles?.length ?? 0}
                />
            </SimpleGrid>

            <EntityModal color="grape" icon={TbDatabase} onClose={closeCreatingRole} opened={creatingRole} title={t('authMethods.newRole')}>
                <RoleForm
                    onDone={() => {
                        closeCreatingRole()
                        queryClient.invalidateQueries({ queryKey: ['db', 'roles'] })
                    }}
                />
            </EntityModal>

            {creds && (
                <Alert color="yellow" onClose={() => setCreds(null)} title={t('dynamicSecrets.generatedCredentials')} variant="light" withCloseButton>
                    <Stack gap="xs">
                        <CopyableField label={t('dynamicSecrets.username')} value={creds.username} />
                        <CopyableField label={t('dynamicSecrets.password')} maskable value={creds.password} />
                    </Stack>
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
                    {!rolesLoading && (roles?.length ?? 0) === 0 && (
                        <Table.Tr>
                            <Table.Td colSpan={2}>
                                <EmptyState icon={TbDatabase} label={t('authMethods.noRoles')} />
                            </Table.Td>
                        </Table.Tr>
                    )}
                    {roles?.map((name) => (
                        <Table.Tr key={name}>
                            <Table.Td>{name}</Table.Td>
                            <Table.Td>
                                <Group gap="xs" justify="flex-end">
                                    <ActionIcon
                                        loading={genMutation.isPending}
                                        onClick={() => genMutation.mutate(name)}
                                        variant="subtle"
                                    >
                                        <TbBolt size={16} />
                                    </ActionIcon>
                                    <ActionIcon
                                        color="red"
                                        onClick={() =>
                                            modals.openConfirmModal({
                                                title: t('authMethods.deleteRoleTitle'),
                                                children: t('authMethods.deleteRoleConfirm', {
                                                    method: 'Database',
                                                    name
                                                }),
                                                labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
                                                confirmProps: { color: 'red' },
                                                onConfirm: () => deleteRoleMutation.mutate(name)
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
