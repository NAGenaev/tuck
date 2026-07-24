import { ActionIcon, Alert, Button, Card, Group, Select, Stack, Table, Text, TextInput } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbCertificate, TbPlus, TbTrash } from 'react-icons/tb'

import {
    deletePKIRole,
    generateRootCA,
    IssuedCert,
    issueCert,
    listPKIRoles,
    putPKIRole,
    revokeCert
} from '@shared/api/endpoints/pki'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { TableCard } from '@shared/ui/table-card/table-card'

function CAForm() {
    const { t } = useTranslation()
    const [commonName, setCommonName] = useState('')
    const [ttl, setTtl] = useState('87600h')
    const [ca, setCa] = useState<null | string>(null)

    const mutation = useMutation({
        mutationFn: () => generateRootCA({ common_name: commonName, ttl }),
        onSuccess: (res) => setCa(res.certificate),
        onError: () => notifications.show({ color: 'red', message: t('common.generateFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('cryptoEngines.pki.rootCa')}
                </Text>
                <Group grow>
                    <TextInput
                        label={t('cryptoEngines.pki.commonName')}
                        onChange={(e) => setCommonName(e.currentTarget.value)}
                        placeholder={t('cryptoEngines.pki.commonNamePlaceholder')}
                        value={commonName}
                    />
                    <TextInput label={t('tokens.ttl')} onChange={(e) => setTtl(e.currentTarget.value)} value={ttl} />
                </Group>
                <Button
                    disabled={!commonName}
                    loading={mutation.isPending}
                    onClick={() => mutation.mutate()}
                    variant="light"
                >
                    {t('cryptoEngines.pki.generateRootCa')}
                </Button>
                {ca && (
                    <Alert color="teal" title={t('cryptoEngines.pki.caCertificate')} variant="light">
                        <CopyableField value={ca} />
                    </Alert>
                )}
            </Stack>
        </Card>
    )
}

function RoleForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [allowedDomains, setAllowedDomains] = useState('')
    const [keyType, setKeyType] = useState<'ec' | 'rsa'>('ec')
    const [maxTtl, setMaxTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putPKIRole(name, {
                allowed_domains: allowedDomains
                    .split(',')
                    .map((d) => d.trim())
                    .filter(Boolean),
                allow_subdomains: true,
                key_type: keyType,
                max_ttl: maxTtl || undefined
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
                <TextInput
                    label={t('cryptoEngines.pki.allowedDomains')}
                    onChange={(e) => setAllowedDomains(e.currentTarget.value)}
                    placeholder={t('cryptoEngines.pki.allowedDomainsPlaceholder')}
                    value={allowedDomains}
                />
                <Group grow>
                    <Select
                        data={['ec', 'rsa']}
                        label={t('cryptoEngines.pki.keyType')}
                        onChange={(v) => setKeyType((v as 'ec' | 'rsa') ?? 'ec')}
                        value={keyType}
                    />
                    <TextInput
                        label={t('cryptoEngines.pki.maxTtl')}
                        onChange={(e) => setMaxTtl(e.currentTarget.value)}
                        placeholder={t('cryptoEngines.pki.maxTtlPlaceholder')}
                        value={maxTtl}
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

function IssueForm({ role }: { role: string }) {
    const { t } = useTranslation()
    const [commonName, setCommonName] = useState('')
    const [cert, setCert] = useState<IssuedCert | null>(null)

    const mutation = useMutation({
        mutationFn: () => issueCert(role, { common_name: commonName }),
        onSuccess: setCert,
        onError: () => notifications.show({ color: 'red', message: t('cryptoEngines.pki.issueFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="xs">
            <Group>
                <TextInput
                    onChange={(e) => setCommonName(e.currentTarget.value)}
                    placeholder="host.example.com"
                    size="xs"
                    style={{ flex: 1 }}
                    value={commonName}
                />
                <Button disabled={!commonName} loading={mutation.isPending} onClick={() => mutation.mutate()} size="xs">
                    {t('cryptoEngines.pki.issueCertBtn')}
                </Button>
            </Group>
            {cert && (
                <Alert color="yellow" title={t('cryptoEngines.pki.certAndKeyCopyNow')} variant="light">
                    <Stack gap="xs">
                        <CopyableField label={t('cryptoEngines.pki.serial')} value={cert.serial} />
                        <CopyableField label={t('cryptoEngines.pki.certificate')} value={cert.certificate} />
                        <CopyableField label={t('cryptoEngines.pki.privateKey')} maskable value={cert.private_key} />
                    </Stack>
                </Alert>
            )}
        </Stack>
    )
}

export function PKITab() {
    const { t } = useTranslation()
    const [creatingRole, setCreatingRole] = useState(false)
    const [issuingRole, setIssuingRole] = useState<null | string>(null)
    const [revokeSerial, setRevokeSerial] = useState('')
    const queryClient = useQueryClient()

    const { data: roles } = useQuery({ queryKey: ['pki', 'roles'], queryFn: listPKIRoles })

    const deleteMutation = useMutation({
        mutationFn: deletePKIRole,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['pki', 'roles'] })
    })

    const revokeMutation = useMutation({
        mutationFn: revokeCert,
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('cryptoEngines.pki.certRevokedMessage'), title: t('common.revoked') })
            setRevokeSerial('')
        },
        onError: () => notifications.show({ color: 'red', message: t('cryptoEngines.pki.revokeFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="lg">
            <CAForm />

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
                        queryClient.invalidateQueries({ queryKey: ['pki', 'roles'] })
                    }}
                />
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
                                        onClick={() => setIssuingRole(issuingRole === name ? null : name)}
                                        variant="subtle"
                                    >
                                        <TbCertificate size={16} />
                                    </ActionIcon>
                                    <ActionIcon
                                        color="red"
                                        onClick={() =>
                                            modals.openConfirmModal({
                                                title: t('authMethods.deleteRoleTitle'),
                                                children: t('authMethods.deleteRoleConfirm', { method: 'PKI', name }),
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

            {issuingRole && (
                <Card>
                    <Text fw={600} mb="xs" size="sm">
                        {t('cryptoEngines.pki.issueCertFrom', { role: issuingRole })}
                    </Text>
                    <IssueForm role={issuingRole} />
                </Card>
            )}

            <Card>
                <Stack gap="sm">
                    <Text fw={600} size="sm">
                        {t('cryptoEngines.pki.revokeCertificate')}
                    </Text>
                    <Group>
                        <TextInput
                            onChange={(e) => setRevokeSerial(e.currentTarget.value)}
                            placeholder={t('cryptoEngines.pki.serialNumberPlaceholder')}
                            style={{ flex: 1 }}
                            value={revokeSerial}
                        />
                        <Button
                            color="red"
                            disabled={!revokeSerial}
                            loading={revokeMutation.isPending}
                            onClick={() => revokeMutation.mutate(revokeSerial)}
                            variant="light"
                        >
                            {t('common.revoke')}
                        </Button>
                    </Group>
                </Stack>
            </Card>
        </Stack>
    )
}
