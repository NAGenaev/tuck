import { ActionIcon, Alert, Button, Card, Group, Select, SimpleGrid, Stack, Table, Text, TextInput, Textarea } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbKey, TbPlus, TbTrash } from 'react-icons/tb'

import {
    deleteSSHRole,
    generateSSHCA,
    listSSHRoles,
    putSSHRole,
    signSSHKey,
    SignedCert
} from '@shared/api/endpoints/ssh'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { TableCard } from '@shared/ui/table-card/table-card'

function CAForm() {
    const { t } = useTranslation()
    const [ca, setCa] = useState<null | string>(null)
    const mutation = useMutation({
        mutationFn: () => generateSSHCA('ed25519'),
        onSuccess: (res) => setCa(res.public_key),
        onError: () => notifications.show({ color: 'red', message: t('common.generateFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('cryptoEngines.ssh.sshCa')}
                </Text>
                <Button loading={mutation.isPending} onClick={() => mutation.mutate()} variant="light">
                    {t('cryptoEngines.ssh.generateCa')}
                </Button>
                {ca && <CopyableField label={t('cryptoEngines.ssh.caPublicKey')} value={ca} />}
            </Stack>
        </Card>
    )
}

function RoleForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [allowedUsers, setAllowedUsers] = useState('')
    const [certType, setCertType] = useState<'host' | 'user'>('user')
    const [maxTtl, setMaxTtl] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            putSSHRole(name, {
                allowed_users: allowedUsers
                    .split(',')
                    .map((u) => u.trim())
                    .filter(Boolean),
                cert_type: certType,
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
        <Stack gap="sm">
            <TextInput
                autoFocus
                label={t('authMethods.roleName')}
                onChange={(e) => setName(e.currentTarget.value)}
                value={name}
            />
            <TextInput
                label={t('cryptoEngines.ssh.allowedUsers')}
                onChange={(e) => setAllowedUsers(e.currentTarget.value)}
                placeholder={t('cryptoEngines.ssh.allowedUsersPlaceholder')}
                value={allowedUsers}
            />
            <Group grow>
                <Select
                    data={['user', 'host']}
                    label={t('cryptoEngines.ssh.certType')}
                    onChange={(v) => setCertType((v as 'host' | 'user') ?? 'user')}
                    value={certType}
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
    )
}

function SignForm({ role }: { role: string }) {
    const { t } = useTranslation()
    const [publicKey, setPublicKey] = useState('')
    const [signed, setSigned] = useState<SignedCert | null>(null)

    const mutation = useMutation({
        mutationFn: () => signSSHKey(role, { public_key: publicKey }),
        onSuccess: setSigned,
        onError: () => notifications.show({ color: 'red', message: t('cryptoEngines.transit.signFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('cryptoEngines.ssh.signPublicKeyWith', { role })}
                </Text>
                <Textarea
                    onChange={(e) => setPublicKey(e.currentTarget.value)}
                    placeholder={t('cryptoEngines.ssh.publicKeyPlaceholder')}
                    value={publicKey}
                />
                <Button disabled={!publicKey} loading={mutation.isPending} onClick={() => mutation.mutate()} variant="light">
                    {t('cryptoEngines.transit.sign')}
                </Button>
                {signed && (
                    <Alert color="teal" title={t('cryptoEngines.ssh.signedCertificate')} variant="light">
                        <CopyableField value={signed.signed_key} />
                    </Alert>
                )}
            </Stack>
        </Card>
    )
}

export function SSHTab() {
    const { t } = useTranslation()
    const [creatingRole, { open: openCreatingRole, close: closeCreatingRole }] = useDisclosure(false)
    const [signingRole, setSigningRole] = useState<null | string>(null)
    const queryClient = useQueryClient()

    const { data: roles, isLoading } = useQuery({ queryKey: ['ssh', 'roles'], queryFn: listSSHRoles })

    const deleteMutation = useMutation({
        mutationFn: deleteSSHRole,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['ssh', 'roles'] })
    })

    return (
        <Stack gap="md">
            <CAForm />

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
                    IconComponent={TbKey}
                    iconColor="teal"
                    isLoading={isLoading}
                    title={t('common.total')}
                    value={roles?.length ?? 0}
                />
            </SimpleGrid>

            <EntityModal color="teal" icon={TbKey} onClose={closeCreatingRole} opened={creatingRole} title={t('authMethods.newRole')}>
                <RoleForm
                    onDone={() => {
                        closeCreatingRole()
                        queryClient.invalidateQueries({ queryKey: ['ssh', 'roles'] })
                    }}
                />
            </EntityModal>

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
                                <EmptyState icon={TbKey} label={t('authMethods.noRoles')} />
                            </Table.Td>
                        </Table.Tr>
                    )}
                    {roles?.map((name) => (
                        <Table.Tr key={name}>
                            <Table.Td>{name}</Table.Td>
                            <Table.Td>
                                <Group gap="xs" justify="flex-end">
                                    <ActionIcon
                                        onClick={() => setSigningRole(signingRole === name ? null : name)}
                                        variant="subtle"
                                    >
                                        <TbKey size={16} />
                                    </ActionIcon>
                                    <ActionIcon
                                        color="red"
                                        onClick={() =>
                                            modals.openConfirmModal({
                                                title: t('authMethods.deleteRoleTitle'),
                                                children: t('authMethods.deleteRoleConfirm', { method: 'SSH', name }),
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

            {signingRole && <SignForm role={signingRole} />}
        </Stack>
    )
}
