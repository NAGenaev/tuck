import { Alert, Button, Group, PasswordInput, SimpleGrid, Stack, Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
    TbActivity,
    TbClock,
    TbClockHour4,
    TbDatabaseExport,
    TbFileText,
    TbGitCommit,
    TbLock,
    TbLockOpen,
    TbNetwork,
    TbRefresh,
    TbShieldLock,
    TbTag,
    TbUsers
} from 'react-icons/tb'

import { rotate, seal, snapshot, unseal } from '@shared/api/endpoints/sys'
import { useHealth } from '@shared/api/hooks/use-health'
import { useSealStatus } from '@shared/api/hooks/use-seal-status'
import { formatUptime } from '@shared/utils/format'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'
import { PlaceholderCard } from '@shared/ui/placeholder-card/placeholder-card'

function SectionTitle({ children }: { children: string }) {
    return (
        <Text fw={700} size="sm">
            {children}
        </Text>
    )
}

export function StatusPage() {
    const { t } = useTranslation()
    const { data: sealStatus, isLoading: sealLoading } = useSealStatus()
    const { data: health, isLoading: healthLoading } = useHealth()
    const [shard, setShard] = useState('')
    const queryClient = useQueryClient()

    const invalidateSealStatus = () =>
        queryClient.invalidateQueries({ queryKey: ['sys', 'seal-status'] })

    const unsealMutation = useMutation({
        mutationFn: unseal,
        onSuccess: () => {
            setShard('')
            invalidateSealStatus()
        },
        onError: () =>
            notifications.show({ color: 'red', message: t('status.unsealFailed'), title: t('common.error') })
    })

    const rotateMutation = useMutation({
        mutationFn: rotate,
        onSuccess: () =>
            notifications.show({ color: 'teal', message: t('status.keyRotated'), title: t('common.saved') }),
        onError: () =>
            notifications.show({ color: 'red', message: t('status.rotateFailed'), title: t('common.error') })
    })

    const sealMutation = useMutation({
        mutationFn: seal,
        onSuccess: invalidateSealStatus,
        onError: () =>
            notifications.show({ color: 'red', message: t('status.sealFailed'), title: t('common.error') })
    })

    const handleSnapshot = async () => {
        const blob = await snapshot()
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = 'tuck-snapshot.snap'
        a.click()
        URL.revokeObjectURL(url)
    }

    const confirmDestructive = (message: string, onConfirm: () => void) =>
        modals.openConfirmModal({
            title: t('common.areYouSure'),
            children: message,
            labels: { confirm: t('common.confirm'), cancel: t('common.cancel') },
            confirmProps: { color: 'red' },
            onConfirm
        })

    return (
        <Page title="Status">
            <Stack gap="xl">
                <PageHeader icon={TbActivity} title={t('pages.status')} />

                <Stack gap="sm">
                    <SectionTitle>{t('status.seal')}</SectionTitle>
                    <SimpleGrid cols={{ base: 1, sm: 3 }}>
                        <MetricCard
                            IconComponent={sealStatus?.sealed ? TbLock : TbLockOpen}
                            iconColor={sealStatus?.sealed ? 'red' : 'teal'}
                            isLoading={sealLoading}
                            title={t('status.sealState')}
                            value={sealStatus?.sealed ? t('header.sealed') : t('header.unsealed')}
                        />
                        <MetricCard
                            IconComponent={TbShieldLock}
                            iconColor="grape"
                            isLoading={sealLoading}
                            title={t('status.sealType')}
                            value={sealStatus?.type ?? '—'}
                        />
                        <MetricCard
                            IconComponent={TbDatabaseExport}
                            iconColor="orange"
                            isLoading={sealLoading}
                            title={t('status.shards')}
                            value={
                                sealStatus
                                    ? `${sealStatus.received_shards ?? 0} / ${sealStatus.required_shards ?? 0}`
                                    : '—'
                            }
                        />
                    </SimpleGrid>
                </Stack>

                <Stack gap="sm">
                    <SectionTitle>{t('status.server')}</SectionTitle>
                    <SimpleGrid cols={{ base: 1, sm: 3 }}>
                        <MetricCard
                            IconComponent={TbTag}
                            iconColor="blue"
                            isLoading={healthLoading}
                            title={t('status.version')}
                            value={health?.version ?? '—'}
                        />
                        <MetricCard
                            IconComponent={TbGitCommit}
                            iconColor="cyan"
                            isLoading={healthLoading}
                            title={t('status.commit')}
                            value={health?.commit ?? '—'}
                        />
                        <MetricCard
                            IconComponent={TbClock}
                            iconColor="teal"
                            isLoading={healthLoading}
                            title={t('status.uptime')}
                            value={health ? formatUptime(health.uptime_seconds) : '—'}
                        />
                        <MetricCard
                            IconComponent={TbNetwork}
                            iconColor="grape"
                            isLoading={healthLoading}
                            title={t('status.ha')}
                            value={health?.ha_enabled ? t('common.enabled') : t('common.disabled')}
                        />
                    </SimpleGrid>
                </Stack>

                <Stack gap="sm">
                    <SectionTitle>{t('status.activity')}</SectionTitle>
                    <SimpleGrid cols={{ base: 1, sm: 2, lg: 4 }}>
                        <PlaceholderCard IconComponent={TbClockHour4} title={t('status.activeLeases')} />
                        <PlaceholderCard IconComponent={TbUsers} title={t('status.activeTokens')} />
                        <PlaceholderCard IconComponent={TbFileText} title={t('status.recentAuditEvents')} />
                        <PlaceholderCard IconComponent={TbRefresh} title={t('status.replicationLag')} />
                    </SimpleGrid>
                </Stack>

                {sealStatus?.sealed && (
                    <Alert color="yellow" title={t('status.serverSealed')} variant="light">
                        <Stack gap="sm">
                            <PasswordInput
                                onChange={(e) => setShard(e.currentTarget.value)}
                                placeholder={t('status.unsealKeyShard')}
                                value={shard}
                            />
                            <Button
                                loading={unsealMutation.isPending}
                                onClick={() => unsealMutation.mutate(shard)}
                                w="fit-content"
                            >
                                {t('status.submitShard')}
                            </Button>
                        </Stack>
                    </Alert>
                )}

                <Group>
                    <Button
                        leftSection={<TbDatabaseExport size={16} />}
                        onClick={handleSnapshot}
                        variant="light"
                    >
                        {t('status.downloadSnapshot')}
                    </Button>
                    <Button
                        leftSection={<TbRefresh size={16} />}
                        loading={rotateMutation.isPending}
                        onClick={() => confirmDestructive(t('status.rotateConfirm'), () => rotateMutation.mutate())}
                        variant="light"
                    >
                        {t('status.rotateKey')}
                    </Button>
                    <Button
                        color="red"
                        leftSection={<TbLock size={16} />}
                        loading={sealMutation.isPending}
                        onClick={() => confirmDestructive(t('status.sealConfirm'), () => sealMutation.mutate())}
                        variant="light"
                    >
                        {t('status.seal')}
                    </Button>
                </Group>
            </Stack>
        </Page>
    )
}
