// SPDX-License-Identifier: AGPL-3.0-only
import { Button } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { FormEvent, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { joinCluster } from '@shared/api/endpoints/cluster'
import { Modal, toast } from '@shared/ui/kit'

import { ADDR_RE, NodeVM, serverMessage } from '../lib'
import styles from '../cluster.module.css'

const cx = (...p: Array<false | null | string | undefined>) => p.filter(Boolean).join(' ')

export function JoinModal({
    isLeader,
    leaderAddr,
    nodes,
    onClose,
    onJoined,
    opened
}: {
    isLeader: boolean
    leaderAddr: string
    nodes: NodeVM[]
    onClose: () => void
    onJoined: (id: string, votersAfter: number) => void
    opened: boolean
}) {
    const { t } = useTranslation('cluster')
    const [id, setId] = useState('')
    const [addr, setAddr] = useState('')
    const [serverErr, setServerErr] = useState('')

    useEffect(() => {
        if (opened) {
            setId('')
            setAddr('')
            setServerErr('')
        }
    }, [opened])

    const nid = id.trim()
    const naddr = addr.trim()
    let idErr = ''
    if (nid) {
        if (/\s/.test(nid)) idErr = t('join.errIdSpaces')
        else if (nodes.some((n) => n.id === nid)) idErr = t('join.errIdExists')
    }
    let addrErr = ''
    if (naddr) {
        if (!ADDR_RE.test(naddr)) addrErr = t('join.errAddrFormat')
        else if (nodes.some((n) => n.address === naddr)) addrErr = t('join.errAddrExists')
    }
    const valid = !!nid && !!naddr && !idErr && !addrErr

    const mutation = useMutation({
        mutationFn: () => joinCluster(nid, naddr),
        onError: (err) => {
            const msg = serverMessage(err)
            setServerErr(msg ?? '')
            toast.error(t('join.failed'), msg ?? t('join.failedHint'))
        },
        onSuccess: () => {
            toast.success(t('join.done'), t('join.doneMsg', { id: nid }))
            onJoined(nid, nodes.filter((n) => n.suffrage !== 'Nonvoter').length + 1)
            onClose()
        }
    })

    const submit = (e?: FormEvent) => {
        e?.preventDefault()
        if (!valid || mutation.isPending) return
        setServerErr('')
        mutation.mutate()
    }

    const preview = JSON.stringify({ node_id: nid || '…', raft_addr: naddr || '…' }, null, 2)

    return (
        <Modal
            busy={mutation.isPending}
            footer={
                <>
                    <Button disabled={mutation.isPending} onClick={onClose} variant="default">
                        {t('join.cancel')}
                    </Button>
                    <Button disabled={!valid} loading={mutation.isPending} onClick={() => submit()}>
                        {t('join.submit')}
                    </Button>
                </>
            }
            icon="plus"
            onClose={onClose}
            opened={opened}
            title={t('join.title')}
        >
            <form className={styles.form} noValidate onSubmit={submit}>
                <p className={styles.hint}>{t('join.hint')}</p>
                {!isLeader && (
                    <div className={styles.warnbox}>
                        <b>{t('join.notLeaderTitle')}</b> {t('join.notLeader', { addr: leaderAddr || '—' })}
                    </div>
                )}
                <div className={styles.fld}>
                    <label htmlFor="cl-join-id">{t('join.nodeId')}</label>
                    <input
                        autoComplete="off"
                        autoFocus
                        className={cx(styles.inp, !!idErr && styles.bad)}
                        data-autofocus
                        id="cl-join-id"
                        onChange={(e) => {
                            setId(e.currentTarget.value)
                            setServerErr('')
                        }}
                        placeholder="node-4"
                        spellCheck={false}
                        value={id}
                    />
                    {idErr && <p className={styles.err}>{idErr}</p>}
                </div>
                <div className={styles.fld}>
                    <label htmlFor="cl-join-addr">{t('join.raftAddr')}</label>
                    <input
                        autoComplete="off"
                        className={cx(styles.inp, !!addrErr && styles.bad)}
                        id="cl-join-addr"
                        onChange={(e) => {
                            setAddr(e.currentTarget.value)
                            setServerErr('')
                        }}
                        placeholder="10.0.0.2:8201"
                        spellCheck={false}
                        value={addr}
                    />
                    {addrErr && <p className={styles.err}>{addrErr}</p>}
                </div>
                {serverErr && <p className={styles.err}>{serverErr}</p>}
                <div className={styles.cap}>POST /v1/sys/cluster/join</div>
                <pre className={styles.json}>{preview}</pre>
            </form>
        </Modal>
    )
}
