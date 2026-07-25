import {
    ActionIcon,
    Alert,
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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbBolt, TbBrandAzure, TbPlus, TbTrash } from 'react-icons/tb'

import {
    AzureCreds,
    deleteAzureRole,
    generateAzureCreds,
    listAzureRoles,
    putAzureConfig,
    putAzureRole
} from '@shared/api/endpoints/dynamic-azure'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { TableCard } from '@shared/ui/table-card/table-card'

function ConfigForm() {
    const { t } = useTranslation()
    const [tenantId, setTenantId] = useState('')
    const [clientId, setClientId] = useState('')
    const [clientSecret, setClientSecret] = useState('')

    const mutation = useMutation({
        mutationFn: () => putAzureConfig({ tenant_id: tenantId, client_id: clientId, client_secret: clientSecret }),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('authMethods.configSaved'), title: t('common.saved') })
            setClientSecret('')
        },
        onError: () => notifications.show({ color: 'red', message: t('common.saveFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('dynamicSecrets.azure.servicePrincipalConfig')}
                </Text>
                <Group grow>
                    <TextInput label={t('dynamicSecrets.azure.tenantId')} onChange={(e) => setTenantId(e.currentTarget.value)} value={tenantId} />
                    <TextInput label={t('dynamicSecrets.azure.clientId')} onChange={(e) => setClientId(e.currentTarget.value)} value={clientId} />
                    <PasswordInput
                        label={t('dynamicSecrets.azure.clientSecret')}
                        onChange={(e) => setClientSecret(e.currentTarget.value)}
                        value={clientSecret}
                    />
                </Group>
                <Button
                    disabled={!tenantId || !clientId}
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
    const [appObjectId, setAppObjectId] = useState('')
    const [appId, setAppId] = useState('')
    const [defaultTtl, setDefaultTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putAzureRole(name, {
                application_object_id: appObjectId,
                application_id: appId,
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
                label={t('dynamicSecrets.azure.appObjectId')}
                onChange={(e) => setAppObjectId(e.currentTarget.value)}
                value={appObjectId}
            />
            <TextInput label={t('dynamicSecrets.azure.appId')} onChange={(e) => setAppId(e.currentTarget.value)} value={appId} />
            <TextInput
                label={t('dynamicSecrets.defaultTtl')}
                onChange={(e) => setDefaultTtl(e.currentTarget.value)}
                placeholder={t('authMethods.ttlPlaceholder1h')}
                value={defaultTtl}
            />
            <Button
                disabled={!name || !appObjectId || !appId}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('authMethods.saveRole')}
            </Button>
        </Stack>
    )
}

export function AzureTab() {
    const { t } = useTranslation()
    const [creatingRole, { open: openCreatingRole, close: closeCreatingRole }] = useDisclosure(false)
    const [creds, setCreds] = useState<AzureCreds | null>(null)
    const queryClient = useQueryClient()

    const { data: roles, isLoading } = useQuery({ queryKey: ['azure', 'roles'], queryFn: listAzureRoles })

    const deleteRoleMutation = useMutation({
        mutationFn: deleteAzureRole,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['azure', 'roles'] })
    })
    const genMutation = useMutation({
        mutationFn: generateAzureCreds,
        onSuccess: setCreds,
        onError: () => notifications.show({ color: 'red', message: t('common.generateFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="md">
            <ConfigForm />

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
                    IconComponent={TbBrandAzure}
                    iconColor="blue"
                    isLoading={isLoading}
                    title={t('common.total')}
                    value={roles?.length ?? 0}
                />
            </SimpleGrid>

            <EntityModal color="blue" icon={TbBrandAzure} onClose={closeCreatingRole} opened={creatingRole} title={t('authMethods.newRole')}>
                <RoleForm
                    onDone={() => {
                        closeCreatingRole()
                        queryClient.invalidateQueries({ queryKey: ['azure', 'roles'] })
                    }}
                />
            </EntityModal>

            {creds && (
                <Alert color="yellow" onClose={() => setCreds(null)} title={t('dynamicSecrets.generatedCredentials')} variant="light" withCloseButton>
                    <Stack gap="xs">
                        <CopyableField label={t('dynamicSecrets.azure.clientId')} value={creds.client_id} />
                        <CopyableField label={t('dynamicSecrets.azure.clientSecret')} maskable value={creds.client_secret} />
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
                    {!isLoading && (roles?.length ?? 0) === 0 && (
                        <Table.Tr>
                            <Table.Td colSpan={2}>
                                <EmptyState icon={TbBrandAzure} label={t('authMethods.noRoles')} />
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
                                                children: t('authMethods.deleteRoleConfirm', { method: 'Azure', name }),
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
