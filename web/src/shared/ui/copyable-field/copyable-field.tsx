// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00),
// extended with a mask/reveal toggle (a UX feature from Tuck's previous UI that must be preserved).
import { ActionIcon, CopyButton, Group, Input } from '@mantine/core'
import { useState } from 'react'
import { PiCheck, PiCopy, PiEye, PiEyeSlash } from 'react-icons/pi'

export const CopyableField = ({
    label,
    value,
    maskable,
    size
}: {
    label?: React.ReactNode | string
    maskable?: boolean
    size?: 'lg' | 'md' | 'sm' | 'xl' | 'xs'
    value: number | string
}) => {
    const [revealed, setRevealed] = useState(!maskable)
    const stringValue = value.toString()
    const displayValue = revealed ? stringValue : '•'.repeat(Math.min(stringValue.length, 32))

    return (
        <CopyButton timeout={2000} value={stringValue}>
            {({ copied, copy }) => (
                <Input.Wrapper label={label}>
                    <Input
                        onClick={copy}
                        readOnly
                        rightSection={
                            <Group gap={4} wrap="nowrap">
                                {maskable && (
                                    <ActionIcon
                                        onClick={(e) => {
                                            e.stopPropagation()
                                            setRevealed((r) => !r)
                                        }}
                                        variant="subtle"
                                    >
                                        {revealed ? <PiEyeSlash size="16px" /> : <PiEye size="16px" />}
                                    </ActionIcon>
                                )}
                                <ActionIcon
                                    color={copied ? 'teal' : 'gray'}
                                    onClick={copy}
                                    variant="subtle"
                                >
                                    {copied ? <PiCheck size="16px" /> : <PiCopy size="16px" />}
                                </ActionIcon>
                            </Group>
                        }
                        rightSectionWidth={maskable ? 64 : 32}
                        size={size}
                        value={displayValue}
                    />
                </Input.Wrapper>
            )}
        </CopyButton>
    )
}
