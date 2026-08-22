"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { ComponentType, ReactNode } from "react";
import {
  AlertTriangle,
  Archive,
  ArrowRight,
  CheckCircle2,
  Database,
  Eye,
  FileJson,
  HardDrive,
  Info,
  Loader2,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Trash2,
  UploadCloud,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import type { DatabaseBackup, DatabaseBackupInspectResult, DatabaseMigrationResult, DatabaseRestoreResult, DatabaseStatus } from "@/lib/api-types";

const DATABASE_MIGRATE_CONFIRM = "MIGRATE_DATABASE";

function formatBytes(value?: number): string {
  const size = Number(value || 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(2)} MB`;
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function formatUnixTime(value?: number): string {
  if (!value) return "-";
  return new Date(value * 1000).toLocaleString("zh-CN");
}

function countOf(result: DatabaseMigrationResult | null, key: string): number {
  return Number(result?.counts?.[key] ?? 0);
}

function compactJSON(value?: Record<string, unknown>): string {
  if (!value) return "-";
  return JSON.stringify(value);
}

function isAbortError(error: unknown): boolean {
  return (error instanceof DOMException && error.name === "AbortError") ||
    (error instanceof Error && error.name === "AbortError");
}

function StatusPill({ ok, label }: { ok: boolean; label: string }) {
  return (
    <Badge variant={ok ? "success" : "secondary"} className="gap-1.5">
      {ok ? <CheckCircle2 className="h-3.5 w-3.5" /> : <AlertTriangle className="h-3.5 w-3.5" />}
      {label}
    </Badge>
  );
}

function EndpointCard({
  title,
  description,
  icon: Icon,
  active,
  disabled,
  onClick,
  children,
  selectedLabel,
}: {
  title: string;
  description: string;
  icon: ComponentType<{ className?: string }>;
  active: boolean;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
  selectedLabel: string;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={cn(
        "w-full rounded-xl border bg-card/70 p-4 text-left transition-all",
        active ? "border-primary/70 shadow-lg shadow-primary/10" : "hover:border-primary/40",
        disabled && "cursor-not-allowed opacity-55 hover:border-border",
      )}
    >
      <div className="flex items-start gap-3">
        <div className={cn("grid h-10 w-10 shrink-0 place-items-center rounded-xl", active ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground")}>
          <Icon className="h-5 w-5" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-2">
            <h3 className="font-semibold">{title}</h3>
            {active && <Badge>{selectedLabel}</Badge>}
          </div>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{description}</p>
        </div>
      </div>
      <div className="mt-4 space-y-2 text-xs">{children}</div>
    </button>
  );
}

export default function AdminDatabaseMigrationPage() {
  const { t } = useI18n();
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const [dbStatus, setDbStatus] = useState<DatabaseStatus | null>(null);
  const [dbBackups, setDbBackups] = useState<DatabaseBackup[]>([]);
  const [source, setSource] = useState<"current">("current");
  const [target, setTarget] = useState<"postgres" | "json">("postgres");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [migrationResult, setMigrationResult] = useState<DatabaseMigrationResult | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [backupPreview, setBackupPreview] = useState<DatabaseBackupInspectResult | null>(null);
  const [backupPreviewOpen, setBackupPreviewOpen] = useState(false);
  const [restorePreview, setRestorePreview] = useState<DatabaseRestoreResult | null>(null);
  const [restoreOpen, setRestoreOpen] = useState(false);
  const [backupNote, setBackupNote] = useState("");
  const loadAbortRef = useRef<AbortController | null>(null);
  const loadSequenceRef = useRef(0);

  const loadDatabase = useCallback(async () => {
    loadAbortRef.current?.abort();
    const controller = new AbortController();
    loadAbortRef.current = controller;
    const sequence = ++loadSequenceRef.current;
    setLoading(true);
    try {
      const [statusRes, backupsRes] = await Promise.all([
        api.getDatabaseStatus(controller.signal),
        api.listDatabaseBackups(controller.signal),
      ]);
      if (controller.signal.aborted || sequence !== loadSequenceRef.current) return;
      if (statusRes.success && statusRes.data) {
        setDbStatus(statusRes.data);
      }
      if (backupsRes.success && backupsRes.data) {
        setDbBackups(backupsRes.data.backups || []);
      }
    } catch (error: any) {
      if (!isAbortError(error) && sequence === loadSequenceRef.current) {
        toast({ title: t("adminDatabase.loadStatusFailed"), description: error.message || t("adminDatabase.checkBackend"), variant: "destructive" });
      }
    } finally {
      if (sequence === loadSequenceRef.current) setLoading(false);
      if (loadAbortRef.current === controller) loadAbortRef.current = null;
    }
  }, [toast, t]);

  useEffect(() => {
    const sequenceRef = loadSequenceRef;
    const abortRef = loadAbortRef;
    void loadDatabase();
    return () => {
      sequenceRef.current++;
      abortRef.current?.abort();
    };
  }, [loadDatabase]);

  const latestBackup = dbBackups[0];
  const migrationEnabled = Boolean(dbStatus?.migration_panel_enabled);
  const postgresReady = target !== "postgres" || Boolean(dbStatus?.postgres_configured);
  const sourceReady = true;
  const canPreview = migrationEnabled && sourceReady && postgresReady && !busy;

  const createBackup = async () => {
    setBusy(true);
    try {
      const res = await api.createDatabaseBackup(backupNote);
      if (res.success) {
        toast({ title: t("adminDatabase.backupCreated"), description: res.data?.backup?.name, variant: "success" });
        setBackupNote("");
        await loadDatabase();
      } else {
        toast({ title: t("adminDatabase.backupFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminDatabase.backupFailed"), description: error.message || t("adminDatabase.checkBackendLogs"), variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const inspectBackup = async (backup: DatabaseBackup) => {
    setBusy(true);
    try {
      const res = await api.inspectDatabaseBackup(backup.name);
      if (res.success && res.data) {
        setBackupPreview(res.data);
        setBackupPreviewOpen(true);
      } else {
        toast({ title: t("adminDatabase.inspectFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminDatabase.inspectFailed"), description: error.message || t("adminDatabase.checkBackendLogs"), variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const previewRestoreBackup = async (backup: DatabaseBackup) => {
    setBusy(true);
    try {
      const res = await api.previewDatabaseRestore(backup.name);
      if (res.success && res.data) {
        setRestorePreview(res.data);
        setRestoreOpen(true);
      } else {
        toast({ title: t("adminDatabase.restorePreviewFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminDatabase.restorePreviewFailed"), description: error.message || t("adminDatabase.checkBackendLogs"), variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const restoreBackup = async () => {
    if (!restorePreview?.restored) return;
    setBusy(true);
    try {
      const res = await api.restoreDatabaseBackup(restorePreview.restored, { confirm: restorePreview.confirm || "RESTORE_DATABASE_BACKUP" });
      if (res.success && res.data) {
        setRestorePreview(res.data);
        setRestoreOpen(false);
        toast({
          title: t("adminDatabase.dbRestored"),
          description: res.data.pre_operation_backup ? t("adminDatabase.protectiveBackupDesc", { name: res.data.pre_operation_backup.name }) : t("adminDatabase.protectiveBackupCreated"),
          variant: "success",
        });
        await loadDatabase();
      } else {
        toast({ title: t("adminDatabase.restoreFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminDatabase.restoreFailed"), description: error.message || t("adminDatabase.checkBackendLogs"), variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const deleteBackup = async (backup: DatabaseBackup) => {
    const accepted = await confirm({
      title: t("adminDatabase.deleteBackupTitle"),
      description: t("adminDatabase.deleteBackupDesc", { name: backup.name }),
      tone: "danger",
      confirmLabel: t("adminDatabase.deleteBackupConfirmLabel"),
      confirmVariant: "destructive",
    });
    if (!accepted) return;
    setBusy(true);
    try {
      const res = await api.deleteDatabaseBackup(backup.name);
      if (res.success) {
        toast({ title: t("adminDatabase.backupDeleted"), description: backup.name, variant: "success" });
        await loadDatabase();
      } else {
        toast({ title: t("adminDatabase.deleteFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminDatabase.deleteFailed"), description: error.message || t("adminDatabase.checkBackendLogs"), variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const previewMigration = async (openConfirm = false) => {
    setBusy(true);
    setMigrationResult(null);
    try {
      const res = await api.migrateDatabase({
        source_driver: undefined,
        target_driver: target,
        dry_run: true,
      });
      if (res.success && res.data) {
        setMigrationResult(res.data);
        if (openConfirm) {
          setConfirmOpen(true);
        } else {
          toast({
            title: t("adminDatabase.migratePreflightOk"),
            description: t("adminDatabase.migratePreflightDesc", { users: res.data.users, regcodes: res.data.regcodes, inviteCodes: res.data.invite_codes }),
            variant: "success",
          });
        }
      } else {
        toast({ title: t("adminDatabase.migratePreflightFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminDatabase.migratePreflightFailed"), description: error.message || t("adminDatabase.checkConnInfo"), variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const executeMigration = async () => {
    setBusy(true);
    try {
      const res = await api.migrateDatabase({
        source_driver: undefined,
        target_driver: target,
        dry_run: false,
        confirm: migrationResult?.confirm || DATABASE_MIGRATE_CONFIRM,
      });
      if (res.success && res.data) {
        setMigrationResult(res.data);
        setConfirmOpen(false);
        toast({
          title: t("adminDatabase.migrateComplete"),
          description: res.data.pre_operation_backup
            ? t("adminDatabase.migrateCompleteBackup", { name: res.data.pre_operation_backup.name })
            : t("adminDatabase.migrateCompleteWritten", { users: res.data.users }),
          variant: "success",
        });
        await loadDatabase();
      } else {
        toast({ title: t("adminDatabase.migrateFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminDatabase.migrateFailed"), description: error.message || t("adminDatabase.checkBackendLogs"), variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold sm:text-3xl">{t("adminDatabase.title")}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("adminDatabase.description")}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => void loadDatabase()} disabled={loading || busy}>
            {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
            {t("adminDatabase.refreshStatus")}
          </Button>
          <Button variant="outline" onClick={() => void createBackup()} disabled={busy}>
            {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Archive className="mr-2 h-4 w-4" />}
            {t("adminDatabase.backupNow")}
          </Button>
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">{t("adminDatabase.activeBackend")}</p>
            <p className="mt-1 text-xl font-semibold">{dbStatus?.active_driver || "-"}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">{t("adminDatabase.configuredBackend")}</p>
            <p className="mt-1 text-xl font-semibold">{dbStatus?.configured_driver || "-"}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">{t("adminDatabase.postgres")}</p>
            <p className="mt-1 text-xl font-semibold">{dbStatus?.postgres_configured ? t("adminDatabase.postgresConfigured") : t("adminDatabase.postgresNotConfigured")}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">{t("adminDatabase.migrationFeature")}</p>
            <p className="mt-1 text-xl font-semibold">{migrationEnabled ? t("adminDatabase.migrationOn") : t("adminDatabase.migrationOff")}</p>
          </CardContent>
        </Card>
      </div>

      {!migrationEnabled && (
        <Alert>
          <Info className="h-4 w-4" />
          <AlertTitle>{t("adminDatabase.migrationOffTitle")}</AlertTitle>
          <AlertDescription>
            {t("adminDatabase.migrationOffDesc")}
          </AlertDescription>
        </Alert>
      )}

      {migrationEnabled && !dbStatus?.postgres_configured && (
        <Alert className="border-amber-500/40 bg-amber-500/10">
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>{t("adminDatabase.postgresNotReadyTitle")}</AlertTitle>
          <AlertDescription>
            {t("adminDatabase.postgresNotReadyDesc")}
          </AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <CardTitle className="flex items-center gap-2"><Archive className="h-5 w-5" />{t("adminDatabase.backupMgmtTitle")}</CardTitle>
              <CardDescription>{t("adminDatabase.backupMgmtDesc")}</CardDescription>
            </div>
            <div className="flex w-full flex-col gap-2 sm:w-80">
              <Textarea
                value={backupNote}
                onChange={(event) => setBackupNote(event.target.value.slice(0, 200))}
                placeholder={t("adminDatabase.notePlaceholder")}
                className="min-h-20 resize-none text-sm"
              />
              <Button variant="outline" onClick={() => void createBackup()} disabled={busy}>
                {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Archive className="mr-2 h-4 w-4" />}
                {t("adminDatabase.createBackup")}
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {dbBackups.length === 0 ? (
            <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">{t("adminDatabase.noBackups")}</div>
          ) : (
            <div className="custom-scrollbar max-h-[min(55dvh,640px)] divide-y overflow-y-auto overscroll-contain rounded-xl border">
              {dbBackups.map((backup) => (
                <div key={backup.name} className="flex flex-col gap-3 p-3 md:flex-row md:items-center md:justify-between">
                  <div className="min-w-0">
                    <p className="break-all text-sm font-medium">{backup.name}</p>
                    <p className="text-xs text-muted-foreground">{formatBytes(backup.size)} · {formatUnixTime(backup.created_at)}</p>
                    {backup.note && <p className="mt-1 break-words text-xs text-foreground/80">{t("adminDatabase.notePrefix")}{backup.note}</p>}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button variant="outline" size="sm" onClick={() => void inspectBackup(backup)} disabled={busy}>
                      <Eye className="mr-2 h-4 w-4" />{t("adminDatabase.inspect")}
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => void previewRestoreBackup(backup)} disabled={busy}>
                      <RotateCcw className="mr-2 h-4 w-4" />{t("adminDatabase.restore")}
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => void deleteBackup(backup)} disabled={busy} className="text-destructive hover:text-destructive">
                      <Trash2 className="mr-2 h-4 w-4" />{t("adminDatabase.delete")}
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {migrationEnabled && (
      <>
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
        <Card className="overflow-hidden">
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><HardDrive className="h-5 w-5" />{t("adminDatabase.sourceTitle")}</CardTitle>
            <CardDescription>{t("adminDatabase.sourceDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <EndpointCard
              title={t("adminDatabase.currentGoState")}
              description={t("adminDatabase.currentGoStateDesc")}
              icon={Database}
              active={source === "current"}
              selectedLabel={t("adminDatabase.selected")}
              onClick={() => { setSource("current"); setMigrationResult(null); }}
            >
              <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.typeLabel")}</span><strong>{dbStatus?.active_driver || "-"}</strong></div>
              <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.userLabel")}</span><strong>{dbStatus?.user_count ?? "-"}</strong></div>
              <div className="break-all text-muted-foreground">{t("adminDatabase.stateFilePrefix")}{dbStatus?.state_file || "-"}</div>
            </EndpointCard>
          </CardContent>
        </Card>

        <div className="hidden items-center justify-center xl:flex">
          <div className="grid h-12 w-12 place-items-center rounded-full border bg-background shadow-sm">
            <ArrowRight className="h-5 w-5 text-muted-foreground" />
          </div>
        </div>

        <Card className="overflow-hidden">
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><UploadCloud className="h-5 w-5" />{t("adminDatabase.targetTitle")}</CardTitle>
            <CardDescription>{t("adminDatabase.targetDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <EndpointCard
              title="PostgreSQL"
              description={t("adminDatabase.pgTargetDesc")}
              icon={Database}
              active={target === "postgres"}
              disabled={!dbStatus?.postgres_configured}
              selectedLabel={t("adminDatabase.selected")}
              onClick={() => { setTarget("postgres"); setMigrationResult(null); }}
            >
              <div className="flex flex-wrap gap-2">
                <StatusPill ok={Boolean(dbStatus?.postgres_configured)} label={dbStatus?.postgres_configured ? t("adminDatabase.pgConnConfigured") : t("adminDatabase.pgNotConfigured")} />
                <StatusPill ok={dbStatus?.configured_driver === "postgres"} label={dbStatus?.configured_driver === "postgres" ? t("adminDatabase.pgTargetMatch") : t("adminDatabase.pgNeedRestart")} />
              </div>
              <div className="text-muted-foreground">{t("adminDatabase.pgPreflightHint")}</div>
            </EndpointCard>

            <EndpointCard
              title={t("adminDatabase.jsonTarget")}
              description={t("adminDatabase.jsonTargetDesc")}
              icon={FileJson}
              active={target === "json"}
              selectedLabel={t("adminDatabase.selected")}
              onClick={() => { setTarget("json"); setMigrationResult(null); }}
            >
              <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.typeLabel")}</span><strong>json</strong></div>
              <div className="break-all text-muted-foreground">{t("adminDatabase.targetFilePrefix")}{dbStatus?.state_file || t("adminDatabase.defaultStateFile")}</div>
            </EndpointCard>
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-[0.9fr_1.1fr]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><ShieldCheck className="h-5 w-5" />{t("adminDatabase.safetyTitle")}</CardTitle>
            <CardDescription>{t("adminDatabase.safetyDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <div className="rounded-xl border p-3">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium">{t("adminDatabase.protectiveBackup")}</span>
                <StatusPill ok label={t("adminDatabase.autoCreateOnRun")} />
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                {t("adminDatabase.protectiveBackupHint")}
              </p>
              <p className="mt-2 break-all text-xs text-muted-foreground">
                {t("adminDatabase.latestBackupPrefix")}{latestBackup ? `${latestBackup.name} · ${formatUnixTime(latestBackup.created_at)}` : t("adminDatabase.none")}
              </p>
            </div>
            <div className="rounded-xl border p-3">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium">{t("adminDatabase.migrationSource")}</span>
                <Badge variant="success">{t("adminDatabase.currentRunningState")}</Badge>
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                {t("adminDatabase.migrationSourceHint")}
              </p>
            </div>
            <div className="rounded-xl border p-3">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium">{t("adminDatabase.targetState")}</span>
                <StatusPill ok={postgresReady && sourceReady} label={postgresReady && sourceReady ? t("adminDatabase.canPreflight") : t("adminDatabase.pendingConfig")} />
              </div>
              <p className="mt-2 break-all text-xs text-muted-foreground">
                {migrationResult?.target_ready ? compactJSON(migrationResult.target_ready) : t("adminDatabase.targetStateHint")}
              </p>
              {migrationResult?.target_ready?.database_created === true && (
                <p className="mt-2 text-xs text-emerald-600 dark:text-emerald-400">
                  {t("adminDatabase.databaseCreatedHint")}
                </p>
              )}
            </div>
          </CardContent>
        </Card>
      </div>

      <Card className="border-primary/20 bg-primary/[0.03]">
        <CardHeader>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <CardTitle className="flex items-center gap-2"><Database className="h-5 w-5" />{t("adminDatabase.bottomPreviewTitle")}</CardTitle>
              <CardDescription>{t("adminDatabase.bottomPreviewDesc")}</CardDescription>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => void previewMigration(false)} disabled={!canPreview}>
                {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <ShieldCheck className="mr-2 h-4 w-4" />}
                {t("adminDatabase.generatePreview")}
              </Button>
              <Button onClick={() => void previewMigration(true)} disabled={!canPreview}>
                {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <UploadCloud className="mr-2 h-4 w-4" />}
                {t("adminDatabase.previewAndExecute")}
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {!migrationResult ? (
            <div className="rounded-xl border border-dashed bg-background/60 p-8 text-center text-sm text-muted-foreground">
              {t("adminDatabase.previewEmptyHint")}
            </div>
          ) : (
            <>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">{t("adminDatabase.metricUsers")}</p><p className="text-xl font-semibold">{migrationResult.users}</p></div>
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">{t("adminDatabase.metricRegcodes")}</p><p className="text-xl font-semibold">{migrationResult.regcodes}</p></div>
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">{t("adminDatabase.metricInviteCodes")}</p><p className="text-xl font-semibold">{migrationResult.invite_codes}</p></div>
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">{t("adminDatabase.metricMediaRequests")}</p><p className="text-xl font-semibold">{migrationResult.media_requests}</p></div>
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">{t("adminDatabase.metricSnapshot")}</p><p className="text-xl font-semibold">{formatBytes(migrationResult.snapshot_bytes)}</p></div>
              </div>
              <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                {Object.entries(migrationResult.counts || {}).filter(([, value]) => Number(value) > 0).map(([key, value]) => (
                  <div key={key} className="flex justify-between gap-3 rounded-lg border bg-background/70 px-3 py-2 text-xs">
                    <span className="text-muted-foreground">{key}</span>
                    <strong>{Number(value)}</strong>
                  </div>
                ))}
              </div>
              {migrationResult.warnings?.length ? (
                <Alert className="border-amber-500/40 bg-amber-500/10">
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle>{t("adminDatabase.warningsTitle")}</AlertTitle>
                  <AlertDescription>{migrationResult.warnings.join("；")}</AlertDescription>
                </Alert>
              ) : null}
              {migrationResult.pre_operation_backup && (
                <Alert className="border-emerald-500/40 bg-emerald-500/10">
                  <CheckCircle2 className="h-4 w-4" />
                  <AlertTitle>{t("adminDatabase.migrationExecutedTitle")}</AlertTitle>
                  <AlertDescription>
                    {t("adminDatabase.migrationExecutedDesc", { backup: migrationResult.pre_operation_backup.name, target: migrationResult.target_driver })}
                  </AlertDescription>
                </Alert>
              )}
            </>
          )}
        </CardContent>
      </Card>
      </>
      )}

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent className="custom-scrollbar max-h-[85dvh] max-w-lg overflow-y-auto overscroll-contain">
          <DialogHeader>
            <DialogTitle>{t("adminDatabase.confirmMigrateTitle")}</DialogTitle>
            <DialogDescription>{t("adminDatabase.confirmMigrateDesc")}</DialogDescription>
          </DialogHeader>
          {migrationResult && (
            <div className="space-y-3 text-sm">
              <Alert>
                <Info className="h-4 w-4" />
                <AlertTitle>{t("adminDatabase.writeTargetOnlyTitle")}</AlertTitle>
                <AlertDescription>{t("adminDatabase.writeTargetOnlyDesc")}</AlertDescription>
              </Alert>
              <div className="grid gap-2 rounded-md border p-3 text-xs">
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.sourceField")}</span><strong>{migrationResult.source_driver}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.targetField")}</span><strong>{migrationResult.target_driver}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.snapshotSize")}</span><strong>{formatBytes(migrationResult.snapshot_bytes)}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.usersCodesInvites")}</span><strong>{migrationResult.users} / {migrationResult.regcodes} / {migrationResult.invite_codes}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.loginsPlaybackSignin")}</span><strong>{countOf(migrationResult, "login_logs")} / {countOf(migrationResult, "playback_records")} / {countOf(migrationResult, "signin")}</strong></div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)} disabled={busy}>{t("common.cancel")}</Button>
            <Button onClick={() => void executeMigration()} disabled={busy || !migrationResult}>
              {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <UploadCloud className="mr-2 h-4 w-4" />}
              {t("adminDatabase.confirmMigrate")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={backupPreviewOpen} onOpenChange={setBackupPreviewOpen}>
        <DialogContent className="custom-scrollbar max-h-[85dvh] max-w-lg overflow-y-auto overscroll-contain">
          <DialogHeader>
            <DialogTitle>{t("adminDatabase.backupDetailTitle")}</DialogTitle>
            <DialogDescription>{t("adminDatabase.backupDetailDesc")}</DialogDescription>
          </DialogHeader>
          {backupPreview && (
            <div className="space-y-3 text-sm">
              <div className="rounded-md border p-3 text-xs">
                <div className="break-all font-medium">{backupPreview.backup.name}</div>
                <div className="mt-1 text-muted-foreground">{formatBytes(backupPreview.backup.size)} · {formatUnixTime(backupPreview.backup.created_at)}</div>
                {backupPreview.backup.note && <div className="mt-2 break-words">{t("adminDatabase.notePrefix")}{backupPreview.backup.note}</div>}
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                {Object.entries(backupPreview.counts || {}).map(([key, value]) => (
                  <div key={key} className="flex justify-between gap-3 rounded-md border px-3 py-2 text-xs">
                    <span className="text-muted-foreground">{key}</span>
                    <strong>{Number(value)}</strong>
                  </div>
                ))}
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setBackupPreviewOpen(false)}>{t("adminDatabase.close")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={restoreOpen} onOpenChange={setRestoreOpen}>
        <DialogContent className="custom-scrollbar max-h-[85dvh] max-w-lg overflow-y-auto overscroll-contain">
          <DialogHeader>
            <DialogTitle>{t("adminDatabase.confirmRestoreTitle")}</DialogTitle>
            <DialogDescription>{t("adminDatabase.confirmRestoreDesc")}</DialogDescription>
          </DialogHeader>
          {restorePreview && (
            <div className="space-y-3 text-sm">
              <Alert className="border-amber-500/40 bg-amber-500/10">
                <AlertTriangle className="h-4 w-4" />
                <AlertTitle>{t("adminDatabase.highRiskTitle")}</AlertTitle>
                <AlertDescription>{t("adminDatabase.highRiskDesc")}</AlertDescription>
              </Alert>
              <div className="grid gap-2 rounded-md border p-3 text-xs">
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.targetBackupField")}</span><strong className="break-all text-right">{restorePreview.restored}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.currentUserCount")}</span><strong>{restorePreview.current_counts?.users ?? "-"}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.restoredUserCount")}</span><strong>{restorePreview.counts?.users ?? restorePreview.users}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">{t("adminDatabase.codesInvites")}</span><strong>{restorePreview.regcodes} / {restorePreview.invite_codes}</strong></div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestoreOpen(false)} disabled={busy}>{t("common.cancel")}</Button>
            <Button onClick={() => void restoreBackup()} disabled={busy || !restorePreview}>
              {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RotateCcw className="mr-2 h-4 w-4" />}
              {t("adminDatabase.confirmRestore")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
