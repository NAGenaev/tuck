import {
    ActionIcon,
    Anchor,
    Button,
    Card,
    Checkbox,
    Group,
    SimpleGrid,
    Stack,
    Table,
    Text,
    TextInput
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TbPlus, TbShieldCheck, TbTrash } from 'react-icons/tb'

import { PageHeader } from '@shared/ui/page-header/page-header'

import {
    deletePolicy,
    getPolicy,
    listPolicies,
    PolicyRule,
    putPolicy
} from '@shared/api/endpoints/policy'
import { EmptyState } from '@shared/ui/empty-state/empty-state'
import { EntityModal } from '@shared/ui/entity-modal/entity-modal'
import { MetricCard } from '@shared/ui/metrics/metric-card/metric-card'
import { Page } from '@shared/ui/page/page'
import { TableCard } from '@shared/ui/table-card/table-card'

// Capabilities are a string array on the wire (internal/api/policies.go's
// parseCaps/capsToStrings) — the generated openapi.json wrongly documents
// this as an integer bitmask; there is no "sudo" capability, only these five.
const CAPS = ['read', 'write', 'delete', 'list', 'deny']

function useTemplates(): Record<string, string[]> {
    const { t } = useTranslation()
    return {
        [t('policies.readOnly')]: ['read', 'list'],
        [t('policies.readWrite')]: ['read', 'write', 'list'],
        [t('policies.admin')]: ['read', 'write', 'delete', 'list']
    }
}

function RuleRow({
    rule,
    onChange,
    onRemove
}: {
    onChange: (rule: PolicyRule) => void
    onRemove: () => void
    rule: PolicyRule
}) {
    const { t } = useTranslation()
    const caps = rule.capabilities ?? []
    const toggle = (cap: string) =>
        onChange({
            ...rule,
            capabilities: caps.includes(cap) ? caps.filter((c) => c !== cap) : [...caps, cap]
        })

    return (
        <Card withBorder>
            <Stack gap="xs">
                <Group justify="space-between">
                    <TextInput
                        onChange={(e) => onChange({ ...rule, path: e.currentTarget.value })}
                        placeholder={t('policies.pathPlaceholder')}
                        style={{ flex: 1 }}
                        value={rule.path ?? ''}
                    />
                    <ActionIcon color="red" onClick={onRemove} variant="subtle">
                        <TbTrash size={16} />
                    </ActionIcon>
                </Group>
                <Group gap="md">
                    {CAPS.map((cap) => (
                        <Checkbox
                            checked={caps.includes(cap)}
                            key={cap}
                            label={cap}
                            onChange={() => toggle(cap)}
                            size="xs"
                        />
                    ))}
                </Group>
            </Stack>
        </Card>
    )
}

function PolicyEditor({
    name,
    onSaved,
    onCancel
}: {
    name: null | string
    onCancel: () => void
    onSaved: () => void
}) {
    const { t } = useTranslation()
    const isNew = name === null
    const [policyName, setPolicyName] = useState(name ?? '')
    const [rules, setRules] = useState<PolicyRule[]>([])
    const templates = useTemplates()

    const load = useQuery({
        queryKey: ['policy', 'edit', name],
        queryFn: () => getPolicy(name!),
        enabled: !isNew
    })

    useEffect(() => {
        if (load.data?.rules) {
            setRules(load.data.rules)
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [load.data])

    const saveMutation = useMutation({
        mutationFn: () => putPolicy({ name: policyName, rules }),
        onSuccess: () => {
            notifications.show({
                color: 'teal',
                message: t('policies.savedMessage', { name: policyName }),
                title: t('common.saved')
            })
            onSaved()
        },
        onError: () => notifications.show({ color: 'red', message: t('common.saveFailed'), title: t('common.error') })
    })

    const addTemplate = (caps: string[]) =>
        setRules((r) => [...r, { path: '*', capabilities: caps }])

    return (
        <Stack gap="md">
            <Text c="dimmed" size="xs">
                {t('policies.createHint')}
            </Text>
            <TextInput
                autoFocus
                disabled={!isNew}
                label={t('common.name')}
                onChange={(e) => setPolicyName(e.currentTarget.value)}
                value={policyName}
            />

            <Group gap="xs">
                <Text c="dimmed" size="xs">
                    {t('policies.quickFill')}
                </Text>
                {Object.entries(templates).map(([label, caps]) => (
                    <Button
                        key={label}
                        onClick={() => addTemplate(caps)}
                        size="xs"
                        variant="subtle"
                    >
                        {label}
                    </Button>
                ))}
            </Group>

            <Stack gap="xs">
                {rules.map((rule, i) => (
                    <RuleRow
                        key={i}
                        onChange={(r) => setRules((rs) => rs.map((x, idx) => (idx === i ? r : x)))}
                        onRemove={() => setRules((rs) => rs.filter((_, idx) => idx !== i))}
                        rule={rule}
                    />
                ))}
                <Button
                    onClick={() => setRules((r) => [...r, { path: '', capabilities: [] }])}
                    variant="subtle"
                >
                    {t('policies.addRule')}
                </Button>
            </Stack>

            <Group justify="flex-end">
                <Anchor component="button" onClick={onCancel} size="sm">
                    {t('common.cancel')}
                </Anchor>
                <Button
                    disabled={!policyName}
                    leftSection={<TbPlus size={16} />}
                    loading={saveMutation.isPending}
                    onClick={() => saveMutation.mutate()}
                >
                    {t('policies.savePolicy')}
                </Button>
            </Group>
        </Stack>
    )
}

export function PoliciesPage() {
    const { t } = useTranslation()
    const [editing, setEditing] = useState<null | string>(null)
    const [creatingNew, setCreatingNew] = useState(false)
    const queryClient = useQueryClient()

    const { data: policies, isLoading } = useQuery({
        queryKey: ['policy', 'list'],
        queryFn: listPolicies
    })

    const deleteMutation = useMutation({
        mutationFn: deletePolicy,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['policy', 'list'] })
            notifications.show({ color: 'teal', message: t('policies.deletedMessage'), title: t('common.deleted') })
        }
    })

    const confirmDelete = (name: string) =>
        modals.openConfirmModal({
            title: t('policies.deleteTitle'),
            children: t('policies.deleteConfirm', { name }),
            labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
            confirmProps: { color: 'red' },
            onConfirm: () => deleteMutation.mutate(name)
        })

    const closeEditor = () => {
        setEditing(null)
        setCreatingNew(false)
        queryClient.invalidateQueries({ queryKey: ['policy', 'list'] })
    }

    return (
        <Page title="Policies">
            <Stack gap="lg">
                <PageHeader
                    action={
                        <Button leftSection={<TbPlus size={16} />} onClick={() => setCreatingNew(true)}>
                            {t('common.new')}
                        </Button>
                    }
                    color="grape"
                    icon={TbShieldCheck}
                    title={t('pages.policies')}
                />

                <SimpleGrid cols={{ base: 1, sm: 2 }}>
                    <MetricCard
                        IconComponent={TbShieldCheck}
                        iconColor="grape"
                        isLoading={isLoading}
                        title={t('common.total')}
                        value={policies?.length ?? 0}
                    />
                </SimpleGrid>

                <EntityModal
                    color="grape"
                    icon={TbShieldCheck}
                    onClose={closeEditor}
                    opened={creatingNew || !!editing}
                    size="lg"
                    title={creatingNew ? t('common.new') : (editing ?? '')}
                >
                    {(creatingNew || editing) && (
                        <PolicyEditor name={creatingNew ? null : editing} onCancel={closeEditor} onSaved={closeEditor} />
                    )}
                </EntityModal>

                <TableCard>
                    <Table.Thead>
                        <Table.Tr>
                            <Table.Th>{t('common.name')}</Table.Th>
                            <Table.Th w={60} />
                        </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                        {!isLoading && (policies?.length ?? 0) === 0 && (
                            <Table.Tr>
                                <Table.Td colSpan={2}>
                                    <EmptyState icon={TbShieldCheck} label={t('policies.noPolicies')} />
                                </Table.Td>
                            </Table.Tr>
                        )}
                        {policies?.map((name) => (
                            <Table.Tr key={name}>
                                <Table.Td>
                                    <Anchor component="button" onClick={() => setEditing(name)} size="sm">
                                        {name}
                                    </Anchor>
                                </Table.Td>
                                <Table.Td>
                                    <ActionIcon
                                        color="red"
                                        onClick={() => confirmDelete(name)}
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
