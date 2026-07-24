import {
    ActionIcon,
    Alert,
    Button,
    Card,
    Group,
    Select,
    Stack,
    Table,
    Text,
    TextInput,
    Textarea
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbBolt, TbPlus, TbTrash } from 'react-icons/tb'

import {
    deleteGCPRole,
    generateGCPCreds,
    GCPCreds,
    listGCPRoles,
    putGCPConfig,
    putGCPRole
} from '@shared/api/endpoints/dynamic-gcp'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { TableCard } from '@shared/ui/table-card/table-card'

function ConfigForm() {
    const { t } = useTranslation()
    const [credentialsJson, setCredentialsJson] = useState('')

    const mutation = useMutation({
        mutationFn: () => putGCPConfig({ credentials_json: credentialsJson }),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('authMethods.configSaved'), title: t('common.saved') })
            setCredentialsJson('')
        },
        onError: () => notifications.show({ color: 'red', message: t('common.saveFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('dynamicSecrets.gcp.serviceAccountConfig')}
                </Text>
                <Textarea
                    autosize
                    label={t('dynamicSecrets.gcp.credentialsJson')}
                    minRows={3}
                    onChange={(e) => setCredentialsJson(e.currentTarget.value)}
                    placeholder={t('dynamicSecrets.gcp.credentialsJsonPlaceholder')}
                    value={credentialsJson}
                />
                <Button
                    disabled={!credentialsJson}
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
    const [credType, setCredType] = useState<'access_token' | 'service_account_key'>('access_token')
    const [saEmail, setSaEmail] = useState('')
    const [defaultTtl, setDefaultTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putGCPRole(name, {
                credential_type: credType,
                service_account_email: saEmail,
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
        <Card>
            <Stack gap="sm">
                <TextInput label={t('authMethods.roleName')} onChange={(e) => setName(e.currentTarget.value)} value={name} />
                <Select
                    data={['access_token', 'service_account_key']}
                    label={t('dynamicSecrets.credentialType')}
                    onChange={(v) => setCredType((v as 'access_token' | 'service_account_key') ?? 'access_token')}
                    value={credType}
                />
                <TextInput
                    label={t('dynamicSecrets.gcp.serviceAccountEmail')}
                    onChange={(e) => setSaEmail(e.currentTarget.value)}
                    placeholder={t('dynamicSecrets.gcp.serviceAccountEmailPlaceholder')}
                    value={saEmail}
                />
                <TextInput
                    label={t('dynamicSecrets.defaultTtl')}
                    onChange={(e) => setDefaultTtl(e.currentTarget.value)}
                    placeholder={t('authMethods.ttlPlaceholder1h')}
                    value={defaultTtl}
                />
                <Button
                    disabled={!name || !saEmail}
                    loading={mutation.isPending}
                    onClick={() => mutation.mutate()}
                >
                    {t('authMethods.saveRole')}
                </Button>
            </Stack>
        </Card>
    )
}

export function GCPTab() {
    const { t } = useTranslation()
    const [creatingRole, setCreatingRole] = useState(false)
    const [creds, setCreds] = useState<GCPCreds | null>(null)
    const queryClient = useQueryClient()

    const { data: roles } = useQuery({ queryKey: ['gcp', 'roles'], queryFn: listGCPRoles })

    const deleteRoleMutation = useMutation({
        mutationFn: deleteGCPRole,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['gcp', 'roles'] })
    })
    const genMutation = useMutation({
        mutationFn: generateGCPCreds,
        onSuccess: setCreds,
        onError: () => notifications.show({ color: 'red', message: t('common.generateFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="lg">
            <ConfigForm />

            <Group justify="space-between">
                <Text fw={700} size="sm">
                    {t('authMethods.rolesTitle')}
                </Text>
                <Button leftSection={<TbPlus size={16} />} onClick={() => setCreatingRole((c) => !c)} variant="light">
                    {t('authMethods.newRole')}
                </Button>
            </Group>
            {creatingRole && (
                <RoleForm
                    onDone={() => {
                        setCreatingRole(false)
                        queryClient.invalidateQueries({ queryKey: ['gcp', 'roles'] })
                    }}
                />
            )}

            {creds && (
                <Alert color="yellow" onClose={() => setCreds(null)} title={t('dynamicSecrets.generatedCredentials')} variant="light" withCloseButton>
                    <Stack gap="xs">
                        {creds.access_token && <CopyableField label={t('dynamicSecrets.gcp.accessToken')} maskable value={creds.access_token} />}
                        {creds.private_key && <CopyableField label={t('dynamicSecrets.gcp.privateKey')} maskable value={creds.private_key} />}
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
                    {(roles?.length ?? 0) === 0 && (
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
                                                children: t('authMethods.deleteRoleConfirm', { method: 'GCP', name }),
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
