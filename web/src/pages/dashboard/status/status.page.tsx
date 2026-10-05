import { Button, PasswordInput, Stack, Text } from "@mantine/core";
import { modals } from "@mantine/modals";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { TbDatabaseExport, TbLock, TbRefresh } from "react-icons/tb";

import { listLeases } from "@shared/api/endpoints/leases";
import { rotate, seal, snapshot, unseal } from "@shared/api/endpoints/sys";
import { listTokens } from "@shared/api/endpoints/token";
import { useHealth } from "@shared/api/hooks/use-health";
import { useSealStatus } from "@shared/api/hooks/use-seal-status";
import { formatUptime } from "@shared/utils/format";
import { Page } from "@shared/ui/page/page";
import { PageHeader } from "@shared/ui/page-header/page-header";
import { Bento, Eyebrow, Plate, Tile } from "@shared/ui/tuck/tuck";

import styles from "./status.module.css";

export function StatusPage() {
  const { t } = useTranslation();
  const { data: sealStatus, isLoading: sealLoading } = useSealStatus();
  const { data: health, isLoading: healthLoading } = useHealth();
  const { data: leases, isError: leasesError } = useQuery({
    queryKey: ["leases", "list"],
    queryFn: listLeases,
  });
  const { data: tokens, isError: tokensError } = useQuery({
    queryKey: ["tokens", "list"],
    queryFn: listTokens,
  });
  const [shard, setShard] = useState("");
  const queryClient = useQueryClient();

  const invalidateSealStatus = () =>
    queryClient.invalidateQueries({ queryKey: ["sys", "seal-status"] });

  const unsealMutation = useMutation({
    mutationFn: unseal,
    onSuccess: () => {
      setShard("");
      invalidateSealStatus();
    },
    onError: () =>
      notifications.show({
        color: "red",
        message: t("status.unsealFailed"),
        title: t("common.error"),
      }),
  });

  const rotateMutation = useMutation({
    mutationFn: rotate,
    onSuccess: () =>
      notifications.show({
        color: "teal",
        message: t("status.keyRotated"),
        title: t("common.saved"),
      }),
    onError: () =>
      notifications.show({
        color: "red",
        message: t("status.rotateFailed"),
        title: t("common.error"),
      }),
  });

  const sealMutation = useMutation({
    mutationFn: seal,
    onSuccess: invalidateSealStatus,
    onError: () =>
      notifications.show({
        color: "red",
        message: t("status.sealFailed"),
        title: t("common.error"),
      }),
  });

  const handleSnapshot = async () => {
    const blob = await snapshot();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "tuck-snapshot.snap";
    a.click();
    URL.revokeObjectURL(url);
  };

  const confirmDestructive = (message: string, onConfirm: () => void) =>
    modals.openConfirmModal({
      title: t("common.areYouSure"),
      children: message,
      labels: { confirm: t("common.confirm"), cancel: t("common.cancel") },
      confirmProps: { color: "red" },
      onConfirm,
    });

  const sealed = sealStatus?.sealed;
  const required = sealStatus?.required_shards ?? 0;
  const received = sealStatus?.received_shards ?? 0;
  const tone = sealed ? "#ff5a4d" : "#49e0a4";

  return (
    <Page title="Status">
      <Stack gap={36}>
        <PageHeader eyebrow={t("status.seal")} title={t("pages.status")} />

        <Plate className={styles.hero}>
          <div className={styles.state}>
            <svg className={styles.seal} fill="none" viewBox="0 0 44 44">
              <circle
                cx="22"
                cy="22"
                r="19"
                stroke="#3b4154"
                strokeWidth="1.5"
              />
              <circle
                className={styles.ring}
                cx="22"
                cy="22"
                r="12"
                stroke={sealed ? "#ff5a4d" : "#d7a94b"}
                strokeDasharray="4 5"
                strokeWidth="1.5"
              />
              <circle cx="22" cy="22" fill={tone} r="3" />
            </svg>
            <div className={styles.stateText}>
              <Eyebrow>{t("status.sealState")}</Eyebrow>
              <div
                className={styles.big}
                data-sealed={sealed ? "true" : "false"}
              >
                {sealLoading
                  ? "…"
                  : sealed
                    ? t("header.sealed")
                    : t("header.unsealed")}
              </div>
              <div className={styles.meta}>
                <b>{t("status.sealType")}</b> · {sealStatus?.type ?? "—"}
              </div>
            </div>
          </div>

          <div className={styles.control}>
            {required > 0 && (
              <div>
                <div className={styles.shardsLabel}>
                  {t("status.shards")}
                  <b>
                    {sealStatus
                      ? `${sealed ? received : required} / ${required}`
                      : "—"}
                  </b>
                </div>
                <div className={styles.shards}>
                  {Array.from({ length: Math.max(required, 1) }, (_, i) => (
                    <i data-on={!sealed || i < received} key={i} />
                  ))}
                </div>
              </div>
            )}

            {sealed && sealStatus?.type === "shamir" && (
              <Stack gap="xs">
                <Text c="dimmed" size="sm">
                  {t("status.serverSealed")}
                </Text>
                <PasswordInput
                  onChange={(e) => setShard(e.currentTarget.value)}
                  placeholder={t("status.unsealKeyShard")}
                  value={shard}
                />
                <Button
                  loading={unsealMutation.isPending}
                  onClick={() => unsealMutation.mutate(shard)}
                  w="fit-content"
                >
                  {t("status.submitShard")}
                </Button>
              </Stack>
            )}

            {sealed && sealStatus?.type !== "shamir" && (
              <Stack gap="xs">
                <Text c="dimmed" size="sm">
                  {t("status.autoUnsealHint")}
                </Text>
                <Button
                  loading={unsealMutation.isPending}
                  onClick={() => unsealMutation.mutate("")}
                  w="fit-content"
                >
                  {t("status.unseal")}
                </Button>
              </Stack>
            )}

            <div className={styles.actions}>
              <Button
                leftSection={<TbDatabaseExport size={16} />}
                onClick={handleSnapshot}
                variant="default"
              >
                {t("status.downloadSnapshot")}
              </Button>
              <Button
                leftSection={<TbRefresh size={16} />}
                loading={rotateMutation.isPending}
                onClick={() =>
                  confirmDestructive(t("status.rotateConfirm"), () =>
                    rotateMutation.mutate(),
                  )
                }
                variant="default"
              >
                {t("status.rotateKey")}
              </Button>
              {!sealed && (
                <Button
                  color="red"
                  leftSection={<TbLock size={16} />}
                  loading={sealMutation.isPending}
                  onClick={() =>
                    confirmDestructive(t("status.sealConfirm"), () =>
                      sealMutation.mutate(),
                    )
                  }
                  variant="light"
                >
                  {t("status.seal")}
                </Button>
              )}
            </div>
          </div>
        </Plate>

        <div className={styles.section}>
          <Eyebrow>{t("status.server")}</Eyebrow>
          <Bento columns="repeat(auto-fit, minmax(210px, 1fr))">
            <Tile label={t("status.version")} value={health?.version ?? "—"} />
            <Tile
              label={t("status.commit")}
              value={health?.commit?.slice(0, 8) ?? "—"}
            />
            <Tile
              label={t("status.uptime")}
              value={health ? formatUptime(health.uptime_seconds) : "—"}
            />
            <Tile
              label={t("status.ha")}
              tone={health?.ha_enabled ? "ok" : undefined}
              value={
                health?.ha_enabled ? t("common.enabled") : t("common.disabled")
              }
            />
          </Bento>
        </div>

        <div className={styles.section}>
          <Eyebrow>{t("status.activity")}</Eyebrow>
          <Bento columns="repeat(auto-fit, minmax(260px, 1fr))">
            <Tile
              label={t("status.activeLeases")}
              tone="brass"
              value={
                leasesError
                  ? "—"
                  : (leases?.filter((l) => !l.revoked).length ?? 0)
              }
            />
            <Tile
              label={t("status.activeTokens")}
              tone="brass"
              value={tokensError ? "—" : (tokens?.length ?? 0)}
            />
          </Bento>
        </div>
      </Stack>
    </Page>
  );
}
