// SPDX-License-Identifier: AGPL-3.0-only
import { Op, Problem, Tab } from '../lib'
import { Session } from '../use-session'

/** What the three tool panels need from the page: the session, the stage and error slots. */
export interface WrapCtx {
    clearProblem: (op: Op) => void
    /** record how a failed lookup / unwrap / revoke changed the capsule (404 gone, 410 expired) */
    failed: (token: string, status?: number) => void
    goto: (tab: Tab, token?: string) => void
    playUnwrap: () => void
    problems: Partial<Record<Op, Problem>>
    /** show the sealing animation while the wrap request runs */
    seal: (on: boolean) => void
    session: Session
    setProblem: (op: Op, p: Problem) => void
    setToken: (tab: 'look' | 'unwrap', value: string) => void
    shake: () => void
    tokens: { look: string; unwrap: string }
}
