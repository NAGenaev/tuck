export const ROUTES = {
    AUTH: {
        ROOT: '/auth',
        LOGIN: '/auth/login',
        OIDC_CALLBACK: '/auth/oidc/callback'
    },
    DASHBOARD: {
        ROOT: '/dashboard',
        STATUS: '/dashboard/status',
        SECRETS_KV1: '/dashboard/secrets/kv1',
        SECRETS_KV2: '/dashboard/secrets/kv2',
        TOKENS: '/dashboard/tokens',
        POLICIES: '/dashboard/policies',
        AUTH_METHODS: '/dashboard/auth-methods',
        WRAPPING: '/dashboard/wrapping',
        NAMESPACES: '/dashboard/namespaces',
        TOKEN_ROLES: '/dashboard/token-roles',
        AUDIT_SINKS: '/dashboard/audit-sinks',
        CLUSTER: '/dashboard/cluster',
        MOUNTS: '/dashboard/mounts',
        TEAMS: '/dashboard/teams',
        TUCKSECRETS: '/dashboard/tucksecrets'
    }
} as const
