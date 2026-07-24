import {
    ActionIcon,
    Anchor,
    Breadcrumbs,
    Button,
    Card,
    Group,
    Stack,
    Table,
    Text,
    TextInput,
    Textarea
} from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbFolder, TbKey, TbPlus, TbTrash } from 'react-icons/tb'

import { deleteKVv1, getKVv1, listKVv1, putKVv1 } from '@shared/api/endpoints/kv'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

function useKVv1List(prefix: string) {
    return useQuery({
        queryKey: ['kv1', 'list', prefix],
        queryFn: () => listKVv1(prefix)
    })
}

function WriteSecretForm({ prefix, onDone }: { prefix: string; onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [value, setValue] = useState('')
    const [isSaving, setIsSaving] = useState(false)

    const handleSave = async () => {
        if (!name) return
        setIsSaving(true)
        try {
            await putKVv1(`${prefix}${name}`, value)
            notifications.show({
                color: 'teal',
                message: t('kv.wroteMessage', { path: `${prefix}${name}` }),
                title: t('common.saved')
            })
            onDone()
        } catch {
            notifications.show({ color: 'red', message: t('kv.writeFailed'), title: t('common.error') })
        } finally {
            setIsSaving(false)
        }
    }

    return (
        <Stack gap="sm">
            <TextInput
                autoFocus
                label={t('common.name')}
                onChange={(e) => setName(e.currentTarget.value)}
                placeholder={t('kv.namePlaceholder')}
                value={name}
            />
            <Textarea
                autosize
                label={t('kv.value')}
                minRows={4}
                onChange={(e) => setValue(e.currentTarget.value)}
                value={value}
            />
            <Button fullWidth loading={isSaving} onClick={handleSave}>
                {t('common.save')}
            </Button>
        </Stack>
    )
}

function SecretDetail({ path, onClose }: { path: string; onClose: () => void }) {
    const { t } = useTranslation()
    const queryClient = useQueryClient()
    const { data, isLoading } = useQuery({
        queryKey: ['kv1', 'get', path],
        queryFn: () => getKVv1(path)
    })

    const handleDelete = () =>
        modals.openConfirmModal({
            title: t('kv.deleteSecretTitle'),
            children: t('kv.deleteSecretConfirm', { path }),
            labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
            confirmProps: { color: 'red' },
            onConfirm: async () => {
                await deleteKVv1(path)
                queryClient.invalidateQueries({ queryKey: ['kv1', 'list'] })
                notifications.show({
                    color: 'teal',
                    message: t('kv.deletedMessage', { path }),
                    title: t('common.deleted')
                })
                onClose()
            }
        })

    return (
        <Card>
            <Stack gap="sm">
                <Group justify="space-between">
                    <Text fw={600}>{path}</Text>
                    <Group gap="xs">
                        <ActionIcon color="red" onClick={handleDelete} variant="light">
                            <TbTrash size={16} />
                        </ActionIcon>
                        <Anchor c="dimmed" component="button" onClick={onClose} size="sm">
                            {t('common.close')}
                        </Anchor>
                    </Group>
                </Group>
                <CopyableField label={t('kv.value')} maskable value={isLoading ? '…' : (data?.value ?? '')} />
            </Stack>
        </Card>
    )
}

export function KV1Page() {
    const { t } = useTranslation()
    const [prefix, setPrefix] = useState('')
    const [manualPath, setManualPath] = useState('')
    const [selected, setSelected] = useState<null | string>(null)
    const [writing, { open: openWriting, close: closeWriting }] = useDisclosure(false)
    const { data: keys, isLoading } = useKVv1List(prefix)
    const queryClient = useQueryClient()

    const crumbs = ['secret', ...prefix.split('/').filter(Boolean)]

    return (
        <Page title="KV v1">
            <Stack gap="lg">
                <PageHeader color="blue" icon={TbFolder} title={t('pages.kv1')} />

                <Group justify="space-between">
                    <Breadcrumbs>
                        {crumbs.map((crumb, i) => (
                            <Anchor
                                component="button"
                                key={i}
                                onClick={() =>
                                    setPrefix(
                                        i === 0
                                            ? ''
                                            : crumbs
                                                  .slice(1, i + 1)
                                                  .join('/')
                                                  .concat('/')
                                    )
                                }
                                size="sm"
                            >
                                {crumb}
                            </Anchor>
                        ))}
                    </Breadcrumbs>
                    <Button leftSection={<TbPlus size={16} />} onClick={openWriting}>
                        {t('kv.writeSecret')}
                    </Button>
                </Group>

                <Group>
                    <TextInput
                        onChange={(e) => setManualPath(e.currentTarget.value)}
                        placeholder={t('kv.jumpToPrefix')}
                        style={{ flex: 1 }}
                        value={manualPath}
                    />
                    <Button
                        onClick={() => setPrefix(manualPath ? `${manualPath.replace(/\/$/, '')}/` : '')}
                        variant="light"
                    >
                        {t('kv.go')}
                    </Button>
                </Group>

                <EntityModal color="blue" icon={TbKey} onClose={closeWriting} opened={writing} title={t('kv.writeSecret')}>
                    <WriteSecretForm
                        onDone={() => {
                            closeWriting()
                            queryClient.invalidateQueries({ queryKey: ['kv1', 'list', prefix] })
                        }}
                        prefix={prefix}
                    />
                </EntityModal>

                {selected && <SecretDetail onClose={() => setSelected(null)} path={selected} />}

                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('common.name')}</Table.Th>
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {!isLoading && (keys?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td>
                                    <EmptyState icon={TbKey} label={t('kv.empty')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {keys?.map((key) => {
                            const isFolder = key.endsWith('/')
                            return (
                                <Table.Tr
                                    key={key}
                                    onClick={() =>
                                        isFolder ? setPrefix(`${prefix}${key}`) : setSelected(`${prefix}${key}`)
                                    }
                                    style={{ cursor: 'pointer' }}
                                >
                                    <Table.Td>
                                        <Group gap="xs">
                                            {isFolder ? <TbFolder size={16} /> : <TbKey size={16} />}
                                            <Text size="sm">{key}</Text>
                                        </Group>
                                    </Table.Td>
                                </Table.Tr>
                            )
                        })}
                    </Table.Tbody>
                </TableCard>
            </Stack>
        </Page>
    )
}
