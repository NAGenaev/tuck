import { Stack, Tabs } from '@mantine/core'
import { useTranslation } from 'react-i18next'
import { TbLogin } from 'react-icons/tb'

import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'

import { AppRoleTab } from './tabs/approle.tab'
import { GitHubTab } from './tabs/github.tab'
import { JWTTab } from './tabs/jwt.tab'
import { KubernetesTab } from './tabs/kubernetes.tab'
import { LDAPTab } from './tabs/ldap.tab'

export function AuthMethodsPage() {
    const { t } = useTranslation()
    return (
        <Page title="Auth Methods">
            <Stack gap="lg">
                <PageHeader icon={TbLogin} title={t('pages.authMethods')} />
                <Tabs defaultValue="approle">
                    <Tabs.List>
                        <Tabs.Tab value="approle">AppRole</Tabs.Tab>
                        <Tabs.Tab value="jwt">JWT / OIDC</Tabs.Tab>
                        <Tabs.Tab value="ldap">LDAP</Tabs.Tab>
                        <Tabs.Tab value="kubernetes">Kubernetes</Tabs.Tab>
                        <Tabs.Tab value="github">GitHub Actions</Tabs.Tab>
                    </Tabs.List>

                    <Tabs.Panel pt="md" value="approle">
                        <AppRoleTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="jwt">
                        <JWTTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="ldap">
                        <LDAPTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="kubernetes">
                        <KubernetesTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="github">
                        <GitHubTab />
                    </Tabs.Panel>
                </Tabs>
            </Stack>
        </Page>
    )
}
