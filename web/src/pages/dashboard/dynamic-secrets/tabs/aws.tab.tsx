import {
    ActionIcon,
    Alert,
    Button,
    Card,
    Group,
    PasswordInput,
    Select,
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
import { TbBolt, TbBrandAws, TbPlus, TbTrash } from 'react-icons/tb'

import {
    AWSCreds,
    deleteAWSRole,
    generateAWSCreds,
    getAWSConfig,
    listAWSRoles,
    putAWSConfig,
    putAWSRole
} from '@shared/api/endpoints/dynamic-aws'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { TableCard } from '@shared/ui/table-card/table-card'

function ConfigForm() {
    const { t } = useTranslation()
    const { data } = useQuery({ queryKey: ['aws', 'config'], queryFn: getAWSConfig })
    const [accessKeyId, setAccessKeyId] = useState('')
    const [secretKey, setSecretKey] = useState('')
    const [region, setRegion] = useState('')

    useEffect(() => {
        if (data) {
            setAccessKeyId(data.access_key_id ?? '')
            setRegion(data.region ?? '')
        }
    }, [data])

    const mutation = useMutation({
        mutationFn: () =>
            putAWSConfig({ access_key_id: accessKeyId, secret_access_key: secretKey, region }),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('authMethods.configSaved'), title: t('common.saved') })
            setSecretKey('')
        },
        onError: () => notifications.show({ color: 'red', message: t('common.saveFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('dynamicSecrets.aws.iamConfig')}
                </Text>
                <Group grow>
                    <TextInput
                        label={t('dynamicSecrets.aws.accessKeyId')}
                        onChange={(e) => setAccessKeyId(e.currentTarget.value)}
                        value={accessKeyId}
                    />
                    <PasswordInput
                        label={t('dynamicSecrets.aws.secretAccessKey')}
                        onChange={(e) => setSecretKey(e.currentTarget.value)}
                        placeholder={t('dynamicSecrets.aws.secretAccessKeyPlaceholder')}
                        value={secretKey}
                    />
                    <TextInput
                        label={t('dynamicSecrets.aws.region')}
                        onChange={(e) => setRegion(e.currentTarget.value)}
                        placeholder={t('dynamicSecrets.aws.regionPlaceholder')}
                        value={region}
                    />
                </Group>
                <Button loading={mutation.isPending} onClick={() => mutation.mutate()}>
                    {t('authMethods.jwt.saveConfig')}
                </Button>
            </Stack>
        </Card>
    )
}

function RoleForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [credType, setCredType] = useState<'assumed_role' | 'iam_user'>('iam_user')
    const [arns, setArns] = useState('')
    const [defaultTtl, setDefaultTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putAWSRole(name, {
                credential_type: credType,
                policy_arns: credType === 'iam_user' ? arns.split(',').map((a) => a.trim()).filter(Boolean) : undefined,
                role_arns: credType === 'assumed_role' ? arns.split(',').map((a) => a.trim()).filter(Boolean) : undefined,
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
            <Select
                data={['iam_user', 'assumed_role']}
                label={t('dynamicSecrets.credentialType')}
                onChange={(v) => setCredType((v as 'assumed_role' | 'iam_user') ?? 'iam_user')}
                value={credType}
            />
            <TextInput
                label={credType === 'iam_user' ? t('dynamicSecrets.aws.policyArnsLabel') : t('dynamicSecrets.aws.roleArnsLabel')}
                onChange={(e) => setArns(e.currentTarget.value)}
                value={arns}
            />
            <TextInput
                label={t('dynamicSecrets.defaultTtl')}
                onChange={(e) => setDefaultTtl(e.currentTarget.value)}
                placeholder={t('authMethods.ttlPlaceholder1h')}
                value={defaultTtl}
            />
            <Button
                disabled={!name}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('authMethods.saveRole')}
            </Button>
        </Stack>
    )
}

export function AWSTab() {
    const { t } = useTranslation()
    const [creatingRole, { open: openCreatingRole, close: closeCreatingRole }] = useDisclosure(false)
    const [creds, setCreds] = useState<AWSCreds | null>(null)
    const queryClient = useQueryClient()

    const { data: roles, isLoading } = useQuery({ queryKey: ['aws', 'roles'], queryFn: listAWSRoles })

    const deleteRoleMutation = useMutation({
        mutationFn: deleteAWSRole,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['aws', 'roles'] })
    })
    const genMutation = useMutation({
        mutationFn: generateAWSCreds,
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
                    IconComponent={TbBrandAws}
                    iconColor="orange"
                    isLoading={isLoading}
                    title={t('common.total')}
                    value={roles?.length ?? 0}
                />
            </SimpleGrid>

            <EntityModal color="orange" icon={TbBrandAws} onClose={closeCreatingRole} opened={creatingRole} title={t('authMethods.newRole')}>
                <RoleForm
                    onDone={() => {
                        closeCreatingRole()
                        queryClient.invalidateQueries({ queryKey: ['aws', 'roles'] })
                    }}
                />
            </EntityModal>

            {creds && (
                <Alert color="yellow" onClose={() => setCreds(null)} title={t('dynamicSecrets.generatedCredentials')} variant="light" withCloseButton>
                    <Stack gap="xs">
                        <CopyableField label={t('dynamicSecrets.aws.accessKeyId')} value={creds.access_key_id} />
                        <CopyableField label={t('dynamicSecrets.aws.secretAccessKey')} maskable value={creds.secret_access_key} />
                        {creds.session_token && (
                            <CopyableField label={t('dynamicSecrets.aws.sessionToken')} maskable value={creds.session_token} />
                        )}
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
                                <EmptyState icon={TbBrandAws} label={t('authMethods.noRoles')} />
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
                                                children: t('authMethods.deleteRoleConfirm', { method: 'AWS', name }),
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
