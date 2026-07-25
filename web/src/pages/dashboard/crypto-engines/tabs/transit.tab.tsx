import { ActionIcon, Button, Card, Group, Select, SimpleGrid, Stack, Table, Text, TextInput, Textarea } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbLockSquare, TbPlus, TbRefresh, TbTrash } from 'react-icons/tb'

import {
    createTransitKey,
    decrypt,
    deleteTransitKey,
    encrypt,
    rotateTransitKey,
    sign,
    TransitKeyType,
    verify,
    listTransitKeys
} from '@shared/api/endpoints/transit'
import { fromBase64Url, toBase64Url } from '@shared/utils/base64url'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { TableCard } from '@shared/ui/table-card/table-card'

const KEY_TYPES: TransitKeyType[] = ['aes256-gcm96', 'ecdsa-p256', 'ed25519', 'rsa-2048', 'rsa-4096']

function CreateKeyForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [type, setType] = useState<TransitKeyType>('aes256-gcm96')

    const mutation = useMutation({
        mutationFn: () => createTransitKey(name, type),
        onSuccess: () => {
            notifications.show({
                color: 'teal',
                message: t('cryptoEngines.createdMessage', { name }),
                title: t('common.created')
            })
            onDone()
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
            <Select
                data={KEY_TYPES}
                label={t('common.type')}
                onChange={(v) => setType((v as TransitKeyType) ?? 'aes256-gcm96')}
                value={type}
            />
            <Button disabled={!name} loading={mutation.isPending} onClick={() => mutation.mutate()}>
                {t('cryptoEngines.createKey')}
            </Button>
        </Stack>
    )
}

function KeyOperations({ name }: { name: string }) {
    const { t } = useTranslation()
    const [plaintext, setPlaintext] = useState('')
    const [ciphertext, setCiphertext] = useState('')
    const [signature, setSignature] = useState('')
    const [verifyResult, setVerifyResult] = useState<boolean | null>(null)

    const encryptMutation = useMutation({
        mutationFn: () => encrypt(name, toBase64Url(plaintext)),
        onSuccess: setCiphertext,
        onError: () => notifications.show({ color: 'red', message: t('cryptoEngines.transit.encryptFailed'), title: t('common.error') })
    })
    const decryptMutation = useMutation({
        mutationFn: () => decrypt(name, ciphertext),
        onSuccess: (b64) => setPlaintext(fromBase64Url(b64)),
        onError: () => notifications.show({ color: 'red', message: t('cryptoEngines.transit.decryptFailed'), title: t('common.error') })
    })
    const signMutation = useMutation({
        mutationFn: () => sign(name, toBase64Url(plaintext)),
        onSuccess: setSignature,
        onError: () => notifications.show({ color: 'red', message: t('cryptoEngines.transit.signFailed'), title: t('common.error') })
    })
    const verifyMutation = useMutation({
        mutationFn: () => verify(name, toBase64Url(plaintext), signature),
        onSuccess: setVerifyResult,
        onError: () => notifications.show({ color: 'red', message: t('cryptoEngines.transit.verifyFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {name}
                </Text>
                <Textarea
                    label={t('cryptoEngines.transit.plaintext')}
                    onChange={(e) => setPlaintext(e.currentTarget.value)}
                    value={plaintext}
                />
                <Group>
                    <Button loading={encryptMutation.isPending} onClick={() => encryptMutation.mutate()} size="xs" variant="light">
                        {t('cryptoEngines.transit.encrypt')}
                    </Button>
                    <Button loading={signMutation.isPending} onClick={() => signMutation.mutate()} size="xs" variant="light">
                        {t('cryptoEngines.transit.sign')}
                    </Button>
                    <Button
                        disabled={!signature}
                        loading={verifyMutation.isPending}
                        onClick={() => verifyMutation.mutate()}
                        size="xs"
                        variant="light"
                    >
                        {t('cryptoEngines.transit.verifySignature')}
                    </Button>
                </Group>
                {verifyResult !== null && (
                    <Text c={verifyResult ? 'teal' : 'red'} size="sm">
                        {t('cryptoEngines.transit.signatureValid', { value: String(verifyResult) })}
                    </Text>
                )}
                <CopyableField label={t('cryptoEngines.transit.ciphertext')} value={ciphertext} />
                <Group align="end">
                    <TextInput
                        label={t('cryptoEngines.transit.ciphertextToDecrypt')}
                        onChange={(e) => setCiphertext(e.currentTarget.value)}
                        style={{ flex: 1 }}
                        value={ciphertext}
                    />
                    <Button loading={decryptMutation.isPending} onClick={() => decryptMutation.mutate()} variant="light">
                        {t('cryptoEngines.transit.decrypt')}
                    </Button>
                </Group>
                <CopyableField label={t('cryptoEngines.transit.signature')} value={signature} />
            </Stack>
        </Card>
    )
}

export function TransitTab() {
    const { t } = useTranslation()
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
    const [selected, setSelected] = useState<null | string>(null)
    const queryClient = useQueryClient()

    const { data: keys, isLoading } = useQuery({ queryKey: ['transit', 'keys'], queryFn: listTransitKeys })

    const deleteMutation = useMutation({
        mutationFn: deleteTransitKey,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['transit', 'keys'] })
    })
    const rotateMutation = useMutation({
        mutationFn: rotateTransitKey,
        onSuccess: () =>
            notifications.show({
                color: 'teal',
                message: t('cryptoEngines.transit.keyRotatedMessage'),
                title: t('cryptoEngines.transit.rotatedTitle')
            })
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
                    IconComponent={TbLockSquare}
                    iconColor="indigo"
                    isLoading={isLoading}
                    title={t('common.total')}
                    value={keys?.length ?? 0}
                />
            </SimpleGrid>

            <EntityModal color="indigo" icon={TbLockSquare} onClose={closeCreating} opened={creating} title={t('cryptoEngines.newKey')}>
                <CreateKeyForm
                    onDone={() => {
                        closeCreating()
                        queryClient.invalidateQueries({ queryKey: ['transit', 'keys'] })
                    }}
                />
            </EntityModal>

            {selected && <KeyOperations name={selected} />}

            <TableCard>
                <Table.Thead>
                    <Table.Tr>
                        <Table.Th>{t('cryptoEngines.key')}</Table.Th>
                        <Table.Th w={100} />
                    </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                    {!isLoading && (keys?.length ?? 0) === 0 && (
                        <Table.Tr>
                            <Table.Td colSpan={2}>
                                <EmptyState icon={TbLockSquare} label={t('cryptoEngines.noKeys')} />
                            </Table.Td>
                        </Table.Tr>
                    )}
                    {keys?.map((name) => (
                        <Table.Tr key={name} onClick={() => setSelected(name)} style={{ cursor: 'pointer' }}>
                            <Table.Td>{name}</Table.Td>
                            <Table.Td>
                                <Group gap="xs" justify="flex-end">
                                    <ActionIcon
                                        onClick={(e) => {
                                            e.stopPropagation()
                                            rotateMutation.mutate(name)
                                        }}
                                        variant="subtle"
                                    >
                                        <TbRefresh size={16} />
                                    </ActionIcon>
                                    <ActionIcon
                                        color="red"
                                        onClick={(e) => {
                                            e.stopPropagation()
                                            modals.openConfirmModal({
                                                title: t('cryptoEngines.deleteKeyTitle'),
                                                children: t('cryptoEngines.deleteKeyConfirm', { engine: 'transit', name }),
                                                labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
                                                confirmProps: { color: 'red' },
                                                onConfirm: () => deleteMutation.mutate(name)
                                            })
                                        }}
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
