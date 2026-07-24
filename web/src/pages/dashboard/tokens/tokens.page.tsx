import {
    ActionIcon,
    Alert,
    Button,
    Card,
    Group,
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
import { TbKey, TbPlus, TbTrash } from 'react-icons/tb'

import { createToken, listTokens, revokeToken, Token } from '@shared/api/endpoints/token'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

function CreateTokenForm({
    onCreated
}: {
    onCreated: (token: Token) => void
}) {
    const { t } = useTranslation()
    const [displayName, setDisplayName] = useState('')
    const [ttl, setTtl] = useState('')
    const [policies, setPolicies] = useState('')

    const mutation = useMutation({
        mutationFn: () =>
            createToken({
                display_name: displayName || undefined,
                ttl: ttl || undefined,
                policies: policies
                    ? policies
                          .split(',')
                          .map((p) => p.trim())
                          .filter(Boolean)
                    : undefined
            }),
        onSuccess: (token) => {
            onCreated(token)
            setDisplayName('')
            setTtl('')
            setPolicies('')
        },
        onError: () =>
            notifications.show({ color: 'red', message: t('tokens.createFailed'), title: t('common.error') })
    })

    return (
        <Card>
            <Stack gap="sm">
                <TextInput
                    label={t('tokens.displayName')}
                    onChange={(e) => setDisplayName(e.currentTarget.value)}
                    placeholder={t('tokens.displayNamePlaceholder')}
                    value={displayName}
                />
                <TextInput
                    label={t('tokens.ttl')}
                    onChange={(e) => setTtl(e.currentTarget.value)}
                    placeholder={t('tokens.ttlPlaceholder')}
                    value={ttl}
                />
                <TextInput
                    label={t('tokens.policiesLabel')}
                    onChange={(e) => setPolicies(e.currentTarget.value)}
                    placeholder={t('tokens.policiesPlaceholder')}
                    value={policies}
                />
                <Button
                    fullWidth
                    loading={mutation.isPending}
                    onClick={() => mutation.mutate()}
                >
                    {t('tokens.createToken')}
                </Button>
            </Stack>
        </Card>
    )
}

export function TokensPage() {
    const { t } = useTranslation()
    const [creating, setCreating] = useState(false)
    const [newToken, setNewToken] = useState<null | Token>(null)
    const queryClient = useQueryClient()

    const { data: tokens, isLoading } = useQuery({
        queryKey: ['tokens', 'list'],
        queryFn: listTokens
    })

    const revokeMutation = useMutation({
        mutationFn: revokeToken,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['tokens', 'list'] })
            notifications.show({ color: 'teal', message: t('tokens.revokedMessage'), title: t('common.revoked') })
        }
    })

    const confirmRevoke = (id: string) =>
        modals.openConfirmModal({
            title: t('tokens.revokeTitle'),
            children: t('tokens.revokeConfirm'),
            labels: { confirm: t('common.revoke'), cancel: t('common.cancel') },
            confirmProps: { color: 'red' },
            onConfirm: () => revokeMutation.mutate(id)
        })

    return (
        <Page title="Tokens">
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
                    icon={TbKey}
                    title={t('pages.tokens')}
                />

                {creating && (
                    <CreateTokenForm
                        onCreated={(token) => {
                            setCreating(false)
                            setNewToken(token)
                            queryClient.invalidateQueries({ queryKey: ['tokens', 'list'] })
                        }}
                    />
                )}

                {newToken && (
                    <Alert
                        color="yellow"
                        onClose={() => setNewToken(null)}
                        title={t('tokens.copyNowTitle')}
                        variant="light"
                        withCloseButton
                    >
                        <CopyableField value={newToken.id} />
                    </Alert>
                )}

                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('tokens.token')}</Table.Th>
                            <Table.Th w={60} />
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {!isLoading && (tokens?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td colSpan={2}>
                                    <Text c="dimmed" size="sm">
                                        {t('tokens.noTokens')}
                                    </Text>
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {tokens?.map((id) => (
                            <Table.Tr key={id}>
                                <Table.Td>
                                    <CopyableField maskable size="xs" value={id} />
                                </Table.Td>
                                <Table.Td>
                                    <ActionIcon
                                        color="red"
                                        onClick={() => confirmRevoke(id)}
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
