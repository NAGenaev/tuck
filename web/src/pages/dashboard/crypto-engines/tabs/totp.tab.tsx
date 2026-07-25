import { ActionIcon, Alert, Button, Group, SimpleGrid, Stack, Table, Text, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbPlayerPlay, TbPlayerStop, TbPlus, TbShieldLock, TbTrash } from 'react-icons/tb'

import {
    createTOTPKey,
    deleteTOTPKey,
    generateTOTPCode,
    listTOTPKeys,
    TOTPCreateResult,
    validateTOTPCode
} from '@shared/api/endpoints/totp'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { TableCard } from '@shared/ui/table-card/table-card'

function CreateKeyForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [issuer, setIssuer] = useState('')
    const [account, setAccount] = useState('')
    const [created, setCreated] = useState<TOTPCreateResult | null>(null)

    const mutation = useMutation({
        mutationFn: () => createTOTPKey(name, { issuer, account }),
        onSuccess: (res) => {
            setCreated(res)
            notifications.show({
                color: 'teal',
                message: t('cryptoEngines.createdMessage', { name }),
                title: t('common.created')
            })
        },
        onError: () => notifications.show({ color: 'red', message: t('common.createFailed'), title: t('common.error') })
    })

    return (
        <Stack gap="sm">
            <TextInput
                autoFocus
                label={t('cryptoEngines.keyName')}
                onChange={(e) => setName(e.currentTarget.value)}
                value={name}
            />
            <Group grow>
                <TextInput
                    label={t('authMethods.jwt.issuer')}
                    onChange={(e) => setIssuer(e.currentTarget.value)}
                    placeholder={t('cryptoEngines.totp.issuerPlaceholder')}
                    value={issuer}
                />
                <TextInput
                    label={t('cryptoEngines.totp.account')}
                    onChange={(e) => setAccount(e.currentTarget.value)}
                    placeholder={t('cryptoEngines.totp.accountPlaceholder')}
                    value={account}
                />
            </Group>
            <Button
                disabled={!name || !issuer || !account}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('cryptoEngines.createKey')}
            </Button>
            {created && (
                <Alert color="yellow" onClose={() => setCreated(null)} title={t('cryptoEngines.totp.secretCopyNowTitle')} variant="light" withCloseButton>
                    <Stack gap="xs">
                        <CopyableField label={t('cryptoEngines.totp.secretBase32')} maskable value={created.secret} />
                        <CopyableField label={t('cryptoEngines.totp.otpauthUrl')} maskable value={created.url} />
                    </Stack>
                </Alert>
            )}
            <Button onClick={onDone} variant="subtle">
                {t('cryptoEngines.totp.done')}
            </Button>
        </Stack>
    )
}

function KeyRow({ name, onDelete }: { name: string; onDelete: () => void }) {
    const { t } = useTranslation()
    const [autoRefresh, setAutoRefresh] = useState(false)
    const [validateCode, setValidateCode] = useState('')
    const [validateResult, setValidateResult] = useState<boolean | null>(null)

    const { data: code } = useQuery({
        queryKey: ['totp', 'code', name],
        queryFn: () => generateTOTPCode(name),
        refetchInterval: autoRefresh ? 5_000 : false
    })

    const validateMutation = useMutation({
        mutationFn: () => validateTOTPCode(name, validateCode),
        onSuccess: setValidateResult
    })

    return (
        <Table.Tr>
            <Table.Td>{name}</Table.Td>
            <Table.Td>
                <Text ff="monospace" fw={600}>
                    {code?.code ?? '——————'}
                </Text>
            </Table.Td>
            <Table.Td>
                <ActionIcon onClick={() => setAutoRefresh((a) => !a)} variant="subtle">
                    {autoRefresh ? <TbPlayerStop size={16} /> : <TbPlayerPlay size={16} />}
                </ActionIcon>
            </Table.Td>
            <Table.Td>
                <Group gap={4} wrap="nowrap">
                    <TextInput
                        onChange={(e) => setValidateCode(e.currentTarget.value)}
                        placeholder={t('cryptoEngines.totp.validateCodePlaceholder')}
                        size="xs"
                        value={validateCode}
                    />
                    <Button loading={validateMutation.isPending} onClick={() => validateMutation.mutate()} size="xs" variant="light">
                        {t('cryptoEngines.totp.check')}
                    </Button>
                    {validateResult !== null && (
                        <Text c={validateResult ? 'teal' : 'red'} size="xs">
                            {validateResult ? t('cryptoEngines.totp.valid') : t('cryptoEngines.totp.invalid')}
                        </Text>
                    )}
                </Group>
            </Table.Td>
            <Table.Td>
                <ActionIcon color="red" onClick={onDelete} variant="subtle">
                    <TbTrash size={16} />
                </ActionIcon>
            </Table.Td>
        </Table.Tr>
    )
}

export function TOTPTab() {
    const { t } = useTranslation()
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
    const queryClient = useQueryClient()

    const { data: keys, isLoading } = useQuery({ queryKey: ['totp', 'keys'], queryFn: listTOTPKeys })

    const deleteMutation = useMutation({
        mutationFn: deleteTOTPKey,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['totp', 'keys'] })
    })

    return (
        <Stack gap="md">
            <Group justify="flex-end">
                <Button leftSection={<TbPlus size={16} />} onClick={openCreating}>
                    {t('cryptoEngines.newKey')}
                </Button>
            </Group>

            <SimpleGrid cols={{ base: 1, sm: 2 }}>
                <MetricCard
                    IconComponent={TbShieldLock}
                    iconColor="grape"
                    isLoading={isLoading}
                    title={t('common.total')}
                    value={keys?.length ?? 0}
                />
            </SimpleGrid>

            <EntityModal color="grape" icon={TbShieldLock} onClose={closeCreating} opened={creating} title={t('cryptoEngines.newKey')}>
                <CreateKeyForm
                    onDone={() => {
                        closeCreating()
                        queryClient.invalidateQueries({ queryKey: ['totp', 'keys'] })
                    }}
                />
            </EntityModal>

            <TableCard>
                <Table.Thead>
                    <Table.Tr>
                        <Table.Th>{t('cryptoEngines.key')}</Table.Th>
                        <Table.Th>{t('cryptoEngines.totp.code')}</Table.Th>
                        <Table.Th>{t('cryptoEngines.totp.autoRefresh')}</Table.Th>
                        <Table.Th>{t('cryptoEngines.totp.validate')}</Table.Th>
                        <Table.Th w={50} />
                    </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                    {!isLoading && (keys?.length ?? 0) === 0 && (
                        <Table.Tr>
                            <Table.Td colSpan={5}>
                                <EmptyState icon={TbShieldLock} label={t('cryptoEngines.noKeys')} />
                            </Table.Td>
                        </Table.Tr>
                    )}
                    {keys?.map((name) => (
                        <KeyRow
                            key={name}
                            name={name}
                            onDelete={() =>
                                modals.openConfirmModal({
                                    title: t('cryptoEngines.deleteKeyTitle'),
                                    children: t('cryptoEngines.deleteKeyConfirm', { engine: 'TOTP', name }),
                                    labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
                                    confirmProps: { color: 'red' },
                                    onConfirm: () => deleteMutation.mutate(name)
                                })
                            }
                        />
                    ))}
                </Table.Tbody>
            </TableCard>
        </Stack>
    )
}
