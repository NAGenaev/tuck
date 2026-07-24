import { Stack, Tabs } from '@mantine/core'
import { useTranslation } from 'react-i18next'
import { TbBolt } from 'react-icons/tb'

import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'

import { AWSTab } from './tabs/aws.tab'
import { AzureTab } from './tabs/azure.tab'
import { DatabaseTab } from './tabs/database.tab'
import { GCPTab } from './tabs/gcp.tab'
import { LeasesTab } from './tabs/leases.tab'

export function DynamicSecretsPage() {
    const { t } = useTranslation()
    return (
        <Page title="Dynamic Secrets">
            <Stack gap="lg">
                <PageHeader color="orange" icon={TbBolt} title={t('pages.dynamicSecrets')} />
                <Tabs defaultValue="database">
                    <Tabs.List>
                        <Tabs.Tab value="database">Database</Tabs.Tab>
                        <Tabs.Tab value="aws">AWS</Tabs.Tab>
                        <Tabs.Tab value="gcp">GCP</Tabs.Tab>
                        <Tabs.Tab value="azure">Azure</Tabs.Tab>
                        <Tabs.Tab value="leases">Leases</Tabs.Tab>
                    </Tabs.List>

                    <Tabs.Panel pt="md" value="database">
                        <DatabaseTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="aws">
                        <AWSTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="gcp">
                        <GCPTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="azure">
                        <AzureTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="leases">
                        <LeasesTab />
                    </Tabs.Panel>
                </Tabs>
            </Stack>
        </Page>
    )
}
