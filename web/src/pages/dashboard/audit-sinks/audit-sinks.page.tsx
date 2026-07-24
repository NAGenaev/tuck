import {
    ActionIcon,
    Badge,
    Button,
    Card,
    Group,
    NumberInput,
    SegmentedControl,
    Stack,
    Table,
    Text,
    TextInput
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbFileText, TbPlus, TbTrash } from 'react-icons/tb'

import { deleteAuditSink, listAuditSinks, putFileSink, putWebhookSink } from '@shared/api/endpoints/audit'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

function CreateSinkForm({ onDone }: { onDone: () => void }) {
    const { t } = useTranslation()
    const [type, setType] = useState<'file' | 'webhook'>('webhook')
    const [name, setName] = useState('')
    const [url, setUrl] = useState('')
    const [timeoutSec, setTimeoutSec] = useState<number | string>(5)
    const [path, setPath] = useState('')
    const [maxSizeMb, setMaxSizeMb] = useState<number | string>(100)

    const mutation = useMutation({
        mutationFn: () =>
            type === 'webhook'
                ? putWebhookSink(name, url, Number(timeoutSec))
                : putFileSink(name, path, Number(maxSizeMb)),
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
                <SegmentedControl
                    data={[
                        { label: t('auditSinks.typeWebhook'), value: 'webhook' },
                        { label: t('auditSinks.typeFile'), value: 'file' }
                    ]}
                    onChange={(v) => setType(v as 'file' | 'webhook')}
                    value={type}
                />
                <TextInput label={t('auditSinks.sinkName')} onChange={(e) => setName(e.currentTarget.value)} value={name} />
                {type === 'webhook' ? (
                    <Group grow>
                        <TextInput
                            label={t('auditSinks.url')}
                            onChange={(e) => setUrl(e.currentTarget.value)}
                            placeholder={t('auditSinks.urlPlaceholder')}
                            value={url}
                        />
                        <NumberInput label={t('auditSinks.timeoutSec')} onChange={setTimeoutSec} value={timeoutSec} />
                    </Group>
                ) : (
                    <Group grow>
                        <TextInput
                            label={t('auditSinks.filePath')}
                            onChange={(e) => setPath(e.currentTarget.value)}
                            placeholder={t('auditSinks.filePathPlaceholder')}
                            value={path}
                        />
                        <NumberInput label={t('auditSinks.maxSizeMb')} onChange={setMaxSizeMb} value={maxSizeMb} />
                    </Group>
                )}
                <Button
                    disabled={!name || (type === 'webhook' ? !url : !path)}
                    loading={mutation.isPending}
                    onClick={() => mutation.mutate()}
                >
                    {t('auditSinks.saveSink')}
                </Button>
            </Stack>
        </Card>
    )
}

export function AuditSinksPage() {
    const { t } = useTranslation()
    const [creating, setCreating] = useState(false)
    const queryClient = useQueryClient()

    const { data: sinks, isLoading } = useQuery({ queryKey: ['audit', 'list'], queryFn: listAuditSinks })

    const deleteMutation = useMutation({
        mutationFn: deleteAuditSink,
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['audit', 'list'] })
    })

    return (
        <Page title="Audit Sinks">
            <Stack gap="lg">
                <PageHeader
                    action={
                        <Button
                            leftSection={<TbPlus size={16} />}
                            onClick={() => setCreating((c) => !c)}
                            variant="light"
                        >
                            {t('common.new')}
                        </Button>
                    }
                    color="yellow"
                    icon={TbFileText}
                    title={t('pages.auditSinks')}
                />

                {creating && (
                    <CreateSinkForm
                        onDone={() => {
                            setCreating(false)
                            queryClient.invalidateQueries({ queryKey: ['audit', 'list'] })
                        }}
                    />
                )}

                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('common.name')}</Table.Th>
                            <Table.Th>{t('common.type')}</Table.Th>
                            <Table.Th>{t('auditSinks.details')}</Table.Th>
                            <Table.Th>{t('auditSinks.errors')}</Table.Th>
                            <Table.Th w={60} />
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {!isLoading && (sinks?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td colSpan={5}>
                                    <EmptyState icon={TbFileText} label={t('common.noResults')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {sinks?.map((sink) => (
                            <Table.Tr key={sink.name}>
                                <Table.Td>{sink.name}</Table.Td>
                                <Table.Td>
                                    <Badge color={sink.type === 'webhook' ? 'blue' : 'teal'} variant="light">
                                        {sink.type}
                                    </Badge>
                                </Table.Td>
                                <Table.Td>
                                    <Text c="dimmed" size="xs">
                                        {sink.options.url ?? sink.options.path ?? '—'}
                                    </Text>
                                </Table.Td>
                                <Table.Td>
                                    <Text c={sink.errors > 0 ? 'red' : 'dimmed'} size="xs">
                                        {sink.errors}
                                    </Text>
                                </Table.Td>
                                <Table.Td>
                                    <ActionIcon
                                        color="red"
                                        onClick={() =>
                                            modals.openConfirmModal({
                                                title: t('auditSinks.deleteSinkTitle'),
                                                children: t('auditSinks.deleteSinkConfirm', { name: sink.name }),
                                                labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
                                                confirmProps: { color: 'red' },
                                                onConfirm: () => deleteMutation.mutate(sink.name)
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
            </Stack>
        </Page>
    )
}
