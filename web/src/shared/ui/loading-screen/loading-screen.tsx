// SPDX-License-Identifier: AGPL-3.0-only
// Forked (trimmed — no nprogress dependency) from github.com/remnawave/frontend
// (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00).
import { Center, Loader, Stack, Text } from '@mantine/core'

export function LoadingScreen({ height = '100dvh', text }: { height?: string; text?: string }) {
    return (
        <Center style={{ height }}>
            <Stack align="center" gap="xs">
                <Loader color="cyan" />
                {text && (
                    <Text c="dimmed" size="sm">
                        {text}
                    </Text>
                )}
            </Stack>
        </Center>
    )
}
