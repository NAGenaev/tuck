import { Stack, Tabs } from '@mantine/core'
import { useTranslation } from 'react-i18next'
import { TbLock } from 'react-icons/tb'

import { Page } from '@shared/ui/page/page'
import { PageHeader } from '@shared/ui/page-header/page-header'

import { PKITab } from './tabs/pki.tab'
import { SSHTab } from './tabs/ssh.tab'
import { TOTPTab } from './tabs/totp.tab'
import { TransitTab } from './tabs/transit.tab'

export function CryptoEnginesPage() {
    const { t } = useTranslation()
    return (
        <Page title="Crypto Engines">
            <Stack gap="lg">
                <PageHeader color="violet" icon={TbLock} title={t('pages.cryptoEngines')} />
                <Tabs defaultValue="pki">
                    <Tabs.List>
                        <Tabs.Tab value="pki">PKI</Tabs.Tab>
                        <Tabs.Tab value="transit">Transit</Tabs.Tab>
                        <Tabs.Tab value="ssh">SSH</Tabs.Tab>
                        <Tabs.Tab value="totp">TOTP</Tabs.Tab>
                    </Tabs.List>

                    <Tabs.Panel pt="md" value="pki">
                        <PKITab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="transit">
                        <TransitTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="ssh">
                        <SSHTab />
                    </Tabs.Panel>
                    <Tabs.Panel pt="md" value="totp">
                        <TOTPTab />
                    </Tabs.Panel>
                </Tabs>
            </Stack>
        </Page>
    )
}
