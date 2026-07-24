import {
    Anchor,
    Badge,
    Breadcrumbs,
    Button,
    Card,
    Group,
    NumberInput,
    Stack,
    Table,
    Text,
    TextInput,
    Textarea
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbFolder, TbFolders, TbPlus } from 'react-icons/tb'

import {
    destroyKVv2,
    getKVv2,
    getKVv2Meta,
    listKVv2,
    putKVv2,
    softDeleteKVv2,
    undeleteKVv2
} from '@shared/api/endpoints/kvv2'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

function useKVv2List(prefix: string) {
    return useQuery({ queryKey: ['kv2', 'list', prefix], queryFn: () => listKVv2(prefix) })
}

function WriteSecretForm({ prefix, onDone }: { prefix: string; onDone: () => void }) {
    const { t } = useTranslation()
    const [name, setName] = useState('')
    const [value, setValue] = useState('')
    const [cas, setCas] = useState<number | string>('')
    const [isSaving, setIsSaving] = useState(false)

    const handleSave = async () => {
        if (!name) return
        setIsSaving(true)
        try {
            await putKVv2(`${prefix}${name}`, value, cas === '' ? undefined : Number(cas))
            notifications.show({
                color: 'teal',
                message: t('kv.wroteMessage', { path: `${prefix}${name}` }),
                title: t('common.saved')
            })
            onDone()
        } catch (err: unknown) {
            const status = (err as { response?: { status?: number } })?.response?.status
            notifications.show({
                color: 'red',
                message: status === 409 ? t('kv2.casMismatch') : t('kv.writeFailed'),
                title: t('common.error')
            })
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
            <NumberInput
                description={t('kv2.casDescription')}
                label={t('kv2.casVersion')}
                onChange={setCas}
                value={cas}
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
    const [viewVersion, setViewVersion] = useState<number | undefined>(undefined)

    const { data: current } = useQuery({
        queryKey: ['kv2', 'get', path, viewVersion],
        queryFn: () => getKVv2(path, viewVersion)
    })
    const { data: meta } = useQuery({
        queryKey: ['kv2', 'meta', path],
        queryFn: () => getKVv2Meta(path)
    })

    const invalidate = () => {
        queryClient.invalidateQueries({ queryKey: ['kv2', 'get', path] })
        queryClient.invalidateQueries({ queryKey: ['kv2', 'meta', path] })
    }

    const versions = meta?.versions
        ? Object.entries(meta.versions).sort(([a], [b]) => Number(b) - Number(a))
        : []

    const confirmAction = (title: string, message: string, action: () => Promise<void>) =>
        modals.openConfirmModal({
            title,
            children: message,
            labels: { confirm: t('common.confirm'), cancel: t('common.cancel') },
            confirmProps: { color: 'red' },
            onConfirm: async () => {
                await action()
                invalidate()
            }
        })

    return (
        <Card>
            <Stack gap="sm">
                <Group justify="space-between">
                    <Text fw={600}>
                        {path} {viewVersion ? `(v${viewVersion})` : `(v${meta?.current_version ?? '—'})`}
                    </Text>
                    <Anchor c="dimmed" component="button" onClick={onClose} size="sm">
                        {t('common.close')}
                    </Anchor>
                </Group>
                <CopyableField label={t('kv.value')} maskable value={current?.value ?? ''} />

                <Text fw={600} mt="sm" size="sm">
                    {t('kv2.versions')}
                </Text>
                <Table verticalSpacing="xs">
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('kv2.version')}</Table.Th>
                            <Table.Th>{t('kv2.created')}</Table.Th>
                            <Table.Th>{t('kv2.status')}</Table.Th>
                            <Table.Th />
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {versions.map(([v, info]) => (
                            <Table.Tr key={v}>
                                <Table.Td>{v}</Table.Td>
                                <Table.Td>
                                    <Text c="dimmed" size="xs">
                                        {info.created_at ? new Date(info.created_at).toLocaleString() : '—'}
                                    </Text>
                                </Table.Td>
                                <Table.Td>
                                    {info.destroyed ? (
                                        <Badge color="red" variant="light">
                                            {t('kv2.destroyed')}
                                        </Badge>
                                    ) : info.deleted_at ? (
                                        <Badge color="yellow" variant="light">
                                            {t('kv2.deletedBadge')}
                                        </Badge>
                                    ) : (
                                        <Badge color="teal" variant="light">
                                            {t('kv2.active')}
                                        </Badge>
                                    )}
                                </Table.Td>
                                <Table.Td>
                                    <Group gap="xs" justify="flex-end">
                                        <Anchor
                                            component="button"
                                            onClick={() => setViewVersion(Number(v))}
                                            size="xs"
                                        >
                                            {t('kv2.view')}
                                        </Anchor>
                                        {!info.destroyed && !info.deleted_at && (
                                            <Anchor
                                                c="yellow"
                                                component="button"
                                                onClick={() =>
                                                    confirmAction(
                                                        t('kv2.softDeleteTitle'),
                                                        t('kv2.softDeleteConfirm', { version: v }),
                                                        () => softDeleteKVv2(path, [Number(v)])
                                                    )
                                                }
                                                size="xs"
                                            >
                                                {t('common.delete')}
                                            </Anchor>
                                        )}
                                        {info.deleted_at && !info.destroyed && (
                                            <Anchor
                                                c="teal"
                                                component="button"
                                                onClick={() => undeleteKVv2(path, [Number(v)]).then(invalidate)}
                                                size="xs"
                                            >
                                                {t('kv2.undelete')}
                                            </Anchor>
                                        )}
                                        {!info.destroyed && (
                                            <Anchor
                                                c="red"
                                                component="button"
                                                onClick={() =>
                                                    confirmAction(
                                                        t('kv2.destroyTitle'),
                                                        t('kv2.destroyConfirm', { version: v }),
                                                        () => destroyKVv2(path, [Number(v)])
                                                    )
                                                }
                                                size="xs"
                                            >
                                                {t('kv2.destroy')}
                                            </Anchor>
                                        )}
                                    </Group>
                                </Table.Td>
                            </Table.Tr>
                        ))}
                    </Table.Tbody>
                </Table>
            </Stack>
        </Card>
    )
}

export function KV2Page() {
    const { t } = useTranslation()
    const [prefix, setPrefix] = useState('')
    const [manualPath, setManualPath] = useState('')
    const [selected, setSelected] = useState<null | string>(null)
    const [writing, setWriting] = useState(false)
    const { data: keys, isLoading } = useKVv2List(prefix)
    const queryClient = useQueryClient()

    const crumbs = ['secret', ...prefix.split('/').filter(Boolean)]

    return (
        <Page title="KV v2">
            <Stack gap="lg">
                <PageHeader color="blue" icon={TbFolders} title={t('pages.kv2')} />

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
                    <Button
                        leftSection={<TbPlus size={16} />}
                        onClick={() => setWriting((w) => !w)}
                        variant="light"
                    >
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

                {writing && (
                    <Card>
                        <WriteSecretForm
                            onDone={() => {
                                setWriting(false)
                                queryClient.invalidateQueries({ queryKey: ['kv2', 'list', prefix] })
                            }}
                            prefix={prefix}
                        />
                    </Card>
                )}

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
                                    <Text c="dimmed" size="sm">
                                        {t('kv.empty')}
                                    </Text>
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
                                            {isFolder ? <TbFolder size={16} /> : <TbFolders size={16} />}
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
