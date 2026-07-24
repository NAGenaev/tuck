import { Card, ScrollArea, Table } from '@mantine/core'
import { ReactNode } from 'react'

export function TableCard({ children }: { children: ReactNode }) {
    return (
        <Card p={0}>
            <ScrollArea>
                <Table miw={480} verticalSpacing="sm">
                    {children}
                </Table>
            </ScrollArea>
        </Card>
    )
}
