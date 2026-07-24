import {
    ActionIcon,
    Alert,
    Button,
    Group,
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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbKey, TbPlus, TbTrash } from 'react-icons/tb'

import { createToken, listTokens, revokeToken, Token } from '@shared/api/endpoints/token'
import { CopyableField } from '@shared/ui/copyable-field/copyable-field'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { TableCard } from '@shared/ui/table-card/table-card'

function CreateTokenForm({ onCreated }: { onCreated: (token: Token) => void }) {
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
        <Stack gap="sm">
            <Text c="dimmed" size="xs">
                {t('tokens.createHint')}
            </Text>
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
                leftSection={<TbPlus size={16} />}
                loading={mutation.isPending}
                onClick={() => mutation.mutate()}
            >
                {t('tokens.createToken')}
            </Button>
        </Stack>
    )
}

export function TokensPage() {
    const { t } = useTranslation()
    const [creating, { open: openCreating, close: closeCreating }] = useDisclosure(false)
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
                        <Button leftSection={<TbPlus size={16} />} onClick={openCreating}>
                            {t('common.new')}
                        </Button>
                    }
                    icon={TbKey}
                    title={t('pages.tokens')}
                />

                <SimpleGrid cols={{ base: 1, sm: 2 }}>
                    <MetricCard
                        IconComponent={TbKey}
                        iconColor="cyan"
                        isLoading={isLoading}
                        title={t('common.total')}
                        value={tokens?.length ?? 0}
                    />
                </SimpleGrid>

                <EntityModal color="cyan" icon={TbKey} onClose={closeCreating} opened={creating} title={t('tokens.createToken')}>
                    <CreateTokenForm
                        onCreated={(token) => {
                            closeCreating()
                            setNewToken(token)
                            queryClient.invalidateQueries({ queryKey: ['tokens', 'list'] })
                        }}
                    />
                </EntityModal>

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
                                    <EmptyState icon={TbKey} label={t('tokens.noTokens')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {tokens?.map((id) => (
                            <Table.Tr key={id}>
                                <Table.Td>
                                    <CopyableField maskable size="xs" value={id} />
                                </Table.Td>
                                <Table.Td>
                                    <Group gap="xs" justify="flex-end">
                                        <ActionIcon
                                            color="red"
                                            onClick={() => confirmRevoke(id)}
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
        </Page>
    )
}
