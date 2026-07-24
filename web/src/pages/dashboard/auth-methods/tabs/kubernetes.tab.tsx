import { ActionIcon, Button, Card, Group, Stack, Text, TextInput } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbTrash } from 'react-icons/tb'

import { deleteK8sRole, getK8sRole, K8sRole, putK8sRole } from '@shared/api/endpoints/auth-k8s'

export function KubernetesTab() {
    const { t } = useTranslation()
    const [namespace, setNamespace] = useState('')
    const [sa, setSa] = useState('')
    const [policies, setPolicies] = useState('')
    const [ttl, setTtl] = useState('')
    const [found, setFound] = useState<K8sRole | null>(null)
    const [searched, setSearched] = useState(false)

    const lookupMutation = useMutation({
        mutationFn: () => getK8sRole(namespace, sa),
        onSuccess: (role) => {
            setFound(role)
            setPolicies((role.policies ?? []).join(', '))
            setTtl(role.ttl ?? '')
            setSearched(true)
        },
        onError: () => {
            setFound(null)
            setSearched(true)
        }
    })

    const saveMutation = useMutation({
        mutationFn: () =>
            putK8sRole(namespace, sa, {
                policies: policies
                    .split(',')
                    .map((p) => p.trim())
                    .filter(Boolean),
                ttl: ttl || undefined
            }),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('authMethods.kubernetes.roleSaved'), title: t('common.saved') })
            lookupMutation.mutate()
        },
        onError: () => notifications.show({ color: 'red', message: t('common.saveFailed'), title: t('common.error') })
    })

    const deleteMutation = useMutation({
        mutationFn: () => deleteK8sRole(namespace, sa),
        onSuccess: () => {
            notifications.show({ color: 'teal', message: t('authMethods.roleDeleted'), title: t('common.deleted') })
            setFound(null)
            setSearched(false)
        }
    })

    return (
        <Stack gap="md">
            <Card>
                <Stack gap="sm">
                    <Text fw={600} size="sm">
                        {t('authMethods.kubernetes.lookupOrCreate')}
                    </Text>
                    <Text c="dimmed" size="xs">
                        {t('authMethods.kubernetes.noListEndpointNote')}
                    </Text>
                    <Group grow>
                        <TextInput
                            label={t('authMethods.kubernetes.namespace')}
                            onChange={(e) => setNamespace(e.currentTarget.value)}
                            value={namespace}
                        />
                        <TextInput
                            label={t('authMethods.kubernetes.serviceAccount')}
                            onChange={(e) => setSa(e.currentTarget.value)}
                            value={sa}
                        />
                    </Group>
                    <Button
                        disabled={!namespace || !sa}
                        loading={lookupMutation.isPending}
                        onClick={() => lookupMutation.mutate()}
                        variant="light"
                    >
                        {t('common.lookUp')}
                    </Button>
                </Stack>
            </Card>

            {searched && (
                <Card>
                    <Stack gap="sm">
                        <Group justify="space-between">
                            <Text fw={600} size="sm">
                                {found ? t('authMethods.kubernetes.editRole') : t('authMethods.kubernetes.roleNotFoundCreate')}
                            </Text>
                            {found && (
                                <ActionIcon
                                    color="red"
                                    onClick={() =>
                                        modals.openConfirmModal({
                                            title: t('authMethods.deleteRoleTitle'),
                                            children: t('authMethods.kubernetes.deleteRoleConfirm', { namespace, sa }),
                                            labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
                                            confirmProps: { color: 'red' },
                                            onConfirm: () => deleteMutation.mutate()
                                        })
                                    }
                                    variant="subtle"
                                >
                                    <TbTrash size={16} />
                                </ActionIcon>
                            )}
                        </Group>
                        <TextInput
                            label={t('tokens.policiesLabel')}
                            onChange={(e) => setPolicies(e.currentTarget.value)}
                            value={policies}
                        />
                        <TextInput
                            label={t('tokens.ttl')}
                            onChange={(e) => setTtl(e.currentTarget.value)}
                            placeholder={t('tokens.ttlPlaceholder')}
                            value={ttl}
                        />
                        <Button loading={saveMutation.isPending} onClick={() => saveMutation.mutate()}>
                            {t('authMethods.saveRole')}
                        </Button>
                    </Stack>
                </Card>
            )}
        </Stack>
    )
}
