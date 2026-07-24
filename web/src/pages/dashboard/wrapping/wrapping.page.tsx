import { Alert, Button, Card, Group, Stack, Text, TextInput, Textarea } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbGift } from 'react-icons/tb'

import { lookupWrap, revokeWrap, unwrap, wrap, WrapLookup } from '@shared/api/endpoints/wrapping'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'

function WrapCard() {
    const { t } = useTranslation()
    const [payload, setPayload] = useState('')
    const [ttl, setTtl] = useState('5m')
    const [result, setResult] = useState<null | { token: string; expires_at: string }>(null)

    const mutation = useMutation({
        mutationFn: () => wrap(JSON.parse(payload || '{}'), ttl),
        onSuccess: setResult,
        onError: () => notifications.show({ color: 'red', message: t('wrapping.wrapFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('wrapping.wrap')}
                </Text>
                <Textarea
                    autosize
                    label={t('wrapping.payloadJson')}
                    minRows={3}
                    onChange={(e) => setPayload(e.currentTarget.value)}
                    placeholder={t('wrapping.payloadJsonPlaceholder')}
                    value={payload}
                />
                <TextInput label={t('tokens.ttl')} onChange={(e) => setTtl(e.currentTarget.value)} value={ttl} />
                <Button loading={mutation.isPending} onClick={() => mutation.mutate()} variant="light">
                    {t('wrapping.wrap')}
                </Button>
                {result && (
                    <Alert color="teal" title={t('wrapping.wrapToken')} variant="light">
                        <CopyableField value={result.token} />
                        <Text c="dimmed" mt={4} size="xs">
                            {t('wrapping.expiresMessage', { date: new Date(result.expires_at).toLocaleString() })}
                        </Text>
                    </Alert>
                )}
            </Stack>
        </Card>
    )
}

function UnwrapCard() {
    const { t } = useTranslation()
    const [token, setToken] = useState('')
    const [data, setData] = useState<null | string>(null)

    const mutation = useMutation({
        mutationFn: () => unwrap(token),
        onSuccess: (res) => setData(JSON.stringify(res, null, 2)),
        onError: () => notifications.show({ color: 'red', message: t('wrapping.unwrapFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('wrapping.unwrap')}
                </Text>
                <TextInput label={t('tokens.token')} onChange={(e) => setToken(e.currentTarget.value)} value={token} />
                <Button disabled={!token} loading={mutation.isPending} onClick={() => mutation.mutate()} variant="light">
                    {t('wrapping.unwrap')}
                </Button>
                {data && (
                    <Textarea autosize label={t('wrapping.payload')} minRows={3} readOnly value={data} />
                )}
            </Stack>
        </Card>
    )
}

function LookupRevokeCard() {
    const { t } = useTranslation()
    const [token, setToken] = useState('')
    const [info, setInfo] = useState<WrapLookup | null>(null)

    const lookupMutation = useMutation({
        mutationFn: () => lookupWrap(token),
        onSuccess: setInfo,
        onError: () => notifications.show({ color: 'red', message: t('wrapping.lookupFailed'), title: t('common.error') })
    })
    const revokeMutation = useMutation({
        mutationFn: () => revokeWrap(token),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('tokens.revokedMessage'), title: t('common.revoked') })
            setInfo(null)
        },
        onError: () => notifications.show({ color: 'red', message: t('wrapping.revokeFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <Text fw={600} size="sm">
                    {t('wrapping.lookupRevoke')}
                </Text>
                <TextInput label={t('tokens.token')} onChange={(e) => setToken(e.currentTarget.value)} value={token} />
                <Group>
                    <Button
                        disabled={!token}
                        loading={lookupMutation.isPending}
                        onClick={() => lookupMutation.mutate()}
                        variant="light"
                    >
                        {t('common.lookUp')}
                    </Button>
                    <Button
                        color="red"
                        disabled={!token}
                        loading={revokeMutation.isPending}
                        onClick={() => revokeMutation.mutate()}
                        variant="light"
                    >
                        {t('common.revoke')}
                    </Button>
                </Group>
                {info && (
                    <Stack gap={2}>
                        <Text size="xs">{t('wrapping.created', { date: new Date(info.creation_time).toLocaleString() })}</Text>
                        <Text size="xs">{t('wrapping.expires', { date: new Date(info.expires_at).toLocaleString() })}</Text>
                        <Text size="xs">{t('wrapping.creationTtl', { ttl: info.creation_ttl })}</Text>
                    </Stack>
                )}
            </Stack>
        </Card>
    )
}

export function WrappingPage() {
    const { t } = useTranslation()
    return (
        <Page title="Wrapping">
            <Stack gap="lg">
                <PageHeader color="pink" icon={TbGift} title={t('pages.wrapping')} />
                <WrapCard />
                <UnwrapCard />
                <LookupRevokeCard />
            </Stack>
        </Page>
    )
}
