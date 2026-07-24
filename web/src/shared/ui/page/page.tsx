// SPDX-License-Identifier: AGPL-3.0-only
// Forked (trimmed — no nprogress/branding) from github.com/remnawave/frontend
// (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Box, BoxProps } from '@mantine/core'
import { forwardRef, ReactNode, useEffect } from 'react'

interface PageProps extends BoxProps {
    children: ReactNode
    title: string
}

export const Page = forwardRef<HTMLDivElement, PageProps>(({ children, title, ...other }, ref) => {
    useEffect(() => {
        document.title = `${title} | Tuck`
    }, [title])

    return (
        <Box ref={ref} {...other}>
            {children}
        </Box>
    )
})
Page.displayName = 'Page'
