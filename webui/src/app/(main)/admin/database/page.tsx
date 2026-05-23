"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
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
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import type { DatabaseBackup, DatabaseBackupInspectResult, DatabaseMigrationResult, DatabaseRestoreResult, DatabaseStatus, LegacySQLiteMapping } from "@/lib/api";

const DATABASE_MIGRATE_CONFIRM = "MIGRATE_DATABASE";

const FALLBACK_LEGACY_MAPPINGS: LegacySQLiteMapping[] = [
  { source_database: "users", source_table: "users", source_key: "users.users", target: "users", rows: 0, mapped: true },
  { source_database: "api_keys", source_table: "api_keys", source_key: "api_keys.api_keys", target: "api_keys", rows: 0, mapped: true },
  { source_database: "regcode", source_table: "regcode", source_key: "regcode.regcode", target: "regcodes", rows: 0, mapped: true },
  { source_database: "invites", source_table: "invite_codes", source_key: "invites.invite_codes", target: "invite_codes", rows: 0, mapped: true },
  { source_database: "bangumi", source_table: "require_bangumi", source_key: "bangumi.require_bangumi", target: "media_requests", rows: 0, mapped: true },
  { source_database: "bangumi", source_table: "require_tmdb", source_key: "bangumi.require_tmdb", target: "media_requests", rows: 0, mapped: true },
];

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
}: {
  title: string;
  description: string;
  icon: ComponentType<{ className?: string }>;
  active: boolean;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
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
            {active && <Badge>已选择</Badge>}
          </div>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{description}</p>
        </div>
      </div>
      <div className="mt-4 space-y-2 text-xs">{children}</div>
    </button>
  );
}

export default function AdminDatabaseMigrationPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const [dbStatus, setDbStatus] = useState<DatabaseStatus | null>(null);
  const [dbBackups, setDbBackups] = useState<DatabaseBackup[]>([]);
  const [source, setSource] = useState<"current" | "sqlite">("sqlite");
  const [target, setTarget] = useState<"postgres" | "json">("postgres");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [migrationResult, setMigrationResult] = useState<DatabaseMigrationResult | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [backupPreview, setBackupPreview] = useState<DatabaseBackupInspectResult | null>(null);
  const [backupPreviewOpen, setBackupPreviewOpen] = useState(false);
  const [restorePreview, setRestorePreview] = useState<DatabaseRestoreResult | null>(null);
  const [restoreOpen, setRestoreOpen] = useState(false);

  const loadDatabase = useCallback(async () => {
    setLoading(true);
    try {
      const [statusRes, backupsRes] = await Promise.all([
        api.getDatabaseStatus(),
        api.listDatabaseBackups(),
      ]);
      if (statusRes.success && statusRes.data) {
        setDbStatus(statusRes.data);
        if (!statusRes.data.legacy_sqlite_detected) {
          setSource("current");
        }
      }
      if (backupsRes.success && backupsRes.data) {
        setDbBackups(backupsRes.data.backups || []);
      }
    } catch (error: any) {
      toast({ title: "加载数据库状态失败", description: error.message || "请检查后端连接", variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void loadDatabase();
  }, [loadDatabase]);

  const latestBackup = dbBackups[0];
  const legacyDetected = Boolean(dbStatus?.legacy_sqlite_detected && dbStatus.legacy_sqlite);
  const postgresReady = target !== "postgres" || Boolean(dbStatus?.postgres_configured);
  const sourceReady = source !== "sqlite" || legacyDetected;
  const canPreview = sourceReady && postgresReady && !busy;

  const mappings = useMemo(() => {
    const fromPreview = migrationResult?.legacy_sqlite_import?.mappings || [];
    if (fromPreview.length > 0) return fromPreview;
    if (!dbStatus?.legacy_sqlite?.table_counts) return FALLBACK_LEGACY_MAPPINGS;
    return FALLBACK_LEGACY_MAPPINGS.map((item) => ({
      ...item,
      rows: dbStatus.legacy_sqlite?.table_counts?.[item.source_key] || 0,
    }));
  }, [dbStatus?.legacy_sqlite?.table_counts, migrationResult?.legacy_sqlite_import?.mappings]);

  const mappedCount = mappings.filter((item) => item.mapped).length;
  const unmappedCount = mappings.filter((item) => !item.mapped).length;

  const createBackup = async () => {
    setBusy(true);
    try {
      const res = await api.createDatabaseBackup();
      if (res.success) {
        toast({ title: "备份已创建", description: res.data?.backup?.name, variant: "success" });
        await loadDatabase();
      } else {
        toast({ title: "备份失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "备份失败", description: error.message || "请检查后端日志", variant: "destructive" });
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
        toast({ title: "读取备份失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "读取备份失败", description: error.message || "请检查后端日志", variant: "destructive" });
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
        toast({ title: "恢复预览失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "恢复预览失败", description: error.message || "请检查后端日志", variant: "destructive" });
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
          title: "数据库已恢复",
          description: res.data.pre_operation_backup ? `保护性备份：${res.data.pre_operation_backup.name}` : "恢复前已创建保护性备份",
          variant: "success",
        });
        await loadDatabase();
      } else {
        toast({ title: "恢复失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "恢复失败", description: error.message || "请检查后端日志", variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const deleteBackup = async (backup: DatabaseBackup) => {
    const accepted = await confirm({
      title: "删除数据库备份",
      description: `确认删除数据库备份 ${backup.name}？此操作不可恢复。`,
      tone: "danger",
      confirmLabel: "删除备份",
      confirmVariant: "destructive",
    });
    if (!accepted) return;
    setBusy(true);
    try {
      const res = await api.deleteDatabaseBackup(backup.name);
      if (res.success) {
        toast({ title: "备份已删除", description: backup.name, variant: "success" });
        await loadDatabase();
      } else {
        toast({ title: "删除失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "删除失败", description: error.message || "请检查后端日志", variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const previewMigration = async (openConfirm = false) => {
    setBusy(true);
    setMigrationResult(null);
    try {
      const res = await api.migrateDatabase({
        source_driver: source === "sqlite" ? "sqlite" : undefined,
        target_driver: target,
        dry_run: true,
      });
      if (res.success && res.data) {
        setMigrationResult(res.data);
        if (openConfirm) {
          setConfirmOpen(true);
        } else {
          toast({
            title: "迁移预检通过",
            description: `${res.data.users} 用户，${res.data.regcodes} 卡码，${res.data.invite_codes} 邀请码`,
            variant: "success",
          });
        }
      } else {
        toast({ title: "迁移预检失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "迁移预检失败", description: error.message || "请检查连接信息", variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  const executeMigration = async () => {
    setBusy(true);
    try {
      const res = await api.migrateDatabase({
        source_driver: source === "sqlite" ? "sqlite" : undefined,
        target_driver: target,
        dry_run: false,
        confirm: migrationResult?.confirm || DATABASE_MIGRATE_CONFIRM,
      });
      if (res.success && res.data) {
        setMigrationResult(res.data);
        setConfirmOpen(false);
        toast({
          title: "迁移完成",
          description: res.data.pre_operation_backup
            ? `已创建保护性备份 ${res.data.pre_operation_backup.name}`
            : `${res.data.users} 用户已写入目标后端`,
          variant: "success",
        });
        await loadDatabase();
      } else {
        toast({ title: "迁移失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "迁移失败", description: error.message || "请检查后端日志", variant: "destructive" });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold sm:text-3xl">数据库迁移</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            按“选择来源 → 选择目标 → 生成预览 → 二次确认”的顺序操作；先确认字段映射和备份状态，再执行写入。
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => void loadDatabase()} disabled={loading || busy}>
            {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
            刷新状态
          </Button>
          <Button variant="outline" onClick={() => void createBackup()} disabled={busy}>
            {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Archive className="mr-2 h-4 w-4" />}
            立即备份
          </Button>
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">当前运行后端</p>
            <p className="mt-1 text-xl font-semibold">{dbStatus?.active_driver || "-"}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">配置后端</p>
            <p className="mt-1 text-xl font-semibold">{dbStatus?.configured_driver || "-"}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">PostgreSQL</p>
            <p className="mt-1 text-xl font-semibold">{dbStatus?.postgres_configured ? "已配置" : "未配置"}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">可用备份</p>
            <p className="mt-1 text-xl font-semibold">{dbStatus?.backup_count ?? dbBackups.length}</p>
          </CardContent>
        </Card>
      </div>

      {!dbStatus?.postgres_configured && (
        <Alert className="border-amber-500/40 bg-amber-500/10">
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>PostgreSQL 尚未配置</AlertTitle>
          <AlertDescription>
            目标选择 PostgreSQL 前，请先在配置管理中填写 PostgreSQL 连接信息并保存。预检会验证连接，正式执行会自动创建保护性备份。
          </AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <CardTitle className="flex items-center gap-2"><Archive className="h-5 w-5" />数据库备份管理</CardTitle>
              <CardDescription>创建、查看、恢复和删除数据库状态快照。恢复前会自动创建保护性备份。</CardDescription>
            </div>
            <Button variant="outline" onClick={() => void createBackup()} disabled={busy}>
              {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Archive className="mr-2 h-4 w-4" />}
              创建备份
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {dbBackups.length === 0 ? (
            <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">暂无数据库备份</div>
          ) : (
            <div className="divide-y rounded-xl border">
              {dbBackups.map((backup) => (
                <div key={backup.name} className="flex flex-col gap-3 p-3 md:flex-row md:items-center md:justify-between">
                  <div className="min-w-0">
                    <p className="break-all text-sm font-medium">{backup.name}</p>
                    <p className="text-xs text-muted-foreground">{formatBytes(backup.size)} · {formatUnixTime(backup.created_at)}</p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button variant="outline" size="sm" onClick={() => void inspectBackup(backup)} disabled={busy}>
                      <Eye className="mr-2 h-4 w-4" />查看
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => void previewRestoreBackup(backup)} disabled={busy}>
                      <RotateCcw className="mr-2 h-4 w-4" />恢复
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => void deleteBackup(backup)} disabled={busy} className="text-destructive hover:text-destructive">
                      <Trash2 className="mr-2 h-4 w-4" />删除
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
        <Card className="overflow-hidden">
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><HardDrive className="h-5 w-5" />源数据库</CardTitle>
            <CardDescription>选择要读取的数据来源。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <EndpointCard
              title="当前 Go 状态"
              description="读取当前运行中的 JSON 或 PostgreSQL 状态快照。"
              icon={Database}
              active={source === "current"}
              onClick={() => { setSource("current"); setMigrationResult(null); }}
            >
              <div className="flex justify-between gap-3"><span className="text-muted-foreground">类型</span><strong>{dbStatus?.active_driver || "-"}</strong></div>
              <div className="flex justify-between gap-3"><span className="text-muted-foreground">用户</span><strong>{dbStatus?.user_count ?? "-"}</strong></div>
              <div className="break-all text-muted-foreground">状态文件：{dbStatus?.state_file || "-"}</div>
            </EndpointCard>

            <EndpointCard
              title="旧 SQLite"
              description="读取 db 目录下的旧版 SQLite/WAL 文件，并按字段映射导入。"
              icon={HardDrive}
              active={source === "sqlite"}
              disabled={!legacyDetected}
              onClick={() => { setSource("sqlite"); setMigrationResult(null); }}
            >
              <div className="flex flex-wrap gap-2">
                <StatusPill ok={legacyDetected} label={legacyDetected ? "已检测到" : "未检测到"} />
                <StatusPill ok={Boolean(dbStatus?.legacy_sqlite?.sqlite_available)} label={dbStatus?.legacy_sqlite?.sqlite_available ? "sqlite3 可用" : "sqlite3 不可用"} />
              </div>
              <div className="flex justify-between gap-3"><span className="text-muted-foreground">文件</span><strong>{dbStatus?.legacy_sqlite?.file_count ?? 0}</strong></div>
              <div className="flex justify-between gap-3"><span className="text-muted-foreground">体积</span><strong>{formatBytes(dbStatus?.legacy_sqlite?.total_size)}</strong></div>
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
            <CardTitle className="flex items-center gap-2"><UploadCloud className="h-5 w-5" />迁移目标</CardTitle>
            <CardDescription>选择写入目标；执行前只会写目标，不会自动切换运行后端。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <EndpointCard
              title="PostgreSQL"
              description="将快照写入 PostgreSQL 的 twilight_state JSONB 表。"
              icon={Database}
              active={target === "postgres"}
              disabled={!dbStatus?.postgres_configured}
              onClick={() => { setTarget("postgres"); setMigrationResult(null); }}
            >
              <div className="flex flex-wrap gap-2">
                <StatusPill ok={Boolean(dbStatus?.postgres_configured)} label={dbStatus?.postgres_configured ? "连接已配置" : "未配置"} />
                <StatusPill ok={dbStatus?.configured_driver === "postgres"} label={dbStatus?.configured_driver === "postgres" ? "配置目标一致" : "需重启切换"} />
              </div>
              <div className="text-muted-foreground">预检会连接 PostgreSQL，并在目标库不存在且权限允许时自动建库和准备状态表。</div>
            </EndpointCard>

            <EndpointCard
              title="JSON 状态文件"
              description="把源数据写回 Twilight JSON 状态文件，适合回退或离线校验。"
              icon={FileJson}
              active={target === "json"}
              onClick={() => { setTarget("json"); setMigrationResult(null); }}
            >
              <div className="flex justify-between gap-3"><span className="text-muted-foreground">类型</span><strong>json</strong></div>
              <div className="break-all text-muted-foreground">目标文件：{dbStatus?.state_file || "默认状态文件"}</div>
            </EndpointCard>
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-[0.9fr_1.1fr]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><ShieldCheck className="h-5 w-5" />安全检查</CardTitle>
            <CardDescription>确认备份、映射和目标状态。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <div className="rounded-xl border p-3">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium">保护性备份</span>
                <StatusPill ok label="执行时自动创建" />
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                正式迁移前后端会先备份当前状态；选择旧 SQLite 时还会备份 SQLite/WAL 文件。
              </p>
              <p className="mt-2 break-all text-xs text-muted-foreground">
                最近备份：{latestBackup ? `${latestBackup.name} · ${formatUnixTime(latestBackup.created_at)}` : "暂无"}
              </p>
            </div>
            <div className="rounded-xl border p-3">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium">字段映射</span>
                <Badge variant={unmappedCount ? "secondary" : "success"}>{mappedCount} 已映射 / {unmappedCount} 未映射</Badge>
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                预检后会显示每张旧表的目标结构、行数和字段转换。未知表不会写入目标，但会在预览中列出。
              </p>
            </div>
            <div className="rounded-xl border p-3">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium">目标状态</span>
                <StatusPill ok={postgresReady && sourceReady} label={postgresReady && sourceReady ? "可预检" : "待配置"} />
              </div>
              <p className="mt-2 break-all text-xs text-muted-foreground">
                {migrationResult?.target_ready ? compactJSON(migrationResult.target_ready) : "生成预检后显示连接与目标路径。"}
              </p>
              {migrationResult?.target_ready?.database_created === true && (
                <p className="mt-2 text-xs text-emerald-600 dark:text-emerald-400">
                  本次预检已自动创建 PostgreSQL 目标数据库。
                </p>
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><Info className="h-5 w-5" />字段映射</CardTitle>
            <CardDescription>从旧 SQLite 到 Twilight 状态结构的映射关系。</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="max-h-[420px] space-y-2 overflow-y-auto pr-1">
              {mappings.map((mapping) => (
                <div key={mapping.source_key} className="rounded-xl border bg-background/70 p-3 text-xs">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="min-w-0">
                      <p className="font-mono font-semibold">{mapping.source_key}</p>
                      <p className="mt-0.5 text-muted-foreground">目标：{mapping.target}</p>
                    </div>
                    <div className="flex items-center gap-2">
                      <Badge variant={mapping.mapped ? "success" : "secondary"}>{mapping.mapped ? "已映射" : "未映射"}</Badge>
                      <Badge variant="outline">{mapping.rows} 行</Badge>
                    </div>
                  </div>
                  {mapping.fields?.length ? (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {mapping.fields.slice(0, 8).map((field) => (
                        <Badge key={`${mapping.source_key}-${field.source}-${field.target}`} variant="outline" className="max-w-full truncate">
                          {field.source} → {field.target}{field.transform ? ` · ${field.transform}` : ""}
                        </Badge>
                      ))}
                      {mapping.fields.length > 8 && <Badge variant="secondary">+{mapping.fields.length - 8}</Badge>}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>

      <Card className="border-primary/20 bg-primary/[0.03]">
        <CardHeader>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <CardTitle className="flex items-center gap-2"><Database className="h-5 w-5" />底部预览</CardTitle>
              <CardDescription>预检只读取源数据并检查目标，不会写入目标数据库。</CardDescription>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => void previewMigration(false)} disabled={!canPreview}>
                {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <ShieldCheck className="mr-2 h-4 w-4" />}
                生成预览
              </Button>
              <Button onClick={() => void previewMigration(true)} disabled={!canPreview}>
                {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <UploadCloud className="mr-2 h-4 w-4" />}
                预览并执行
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {!migrationResult ? (
            <div className="rounded-xl border border-dashed bg-background/60 p-8 text-center text-sm text-muted-foreground">
              选择源端和目标后点击“生成预览”，这里会展示迁移规模、映射结果、备份策略和告警信息。
            </div>
          ) : (
            <>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">用户</p><p className="text-xl font-semibold">{migrationResult.users}</p></div>
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">卡码</p><p className="text-xl font-semibold">{migrationResult.regcodes}</p></div>
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">邀请码</p><p className="text-xl font-semibold">{migrationResult.invite_codes}</p></div>
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">求片</p><p className="text-xl font-semibold">{migrationResult.media_requests}</p></div>
                <div className="rounded-xl border bg-background/80 p-3"><p className="text-xs text-muted-foreground">快照</p><p className="text-xl font-semibold">{formatBytes(migrationResult.snapshot_bytes)}</p></div>
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
                  <AlertTitle>需要注意</AlertTitle>
                  <AlertDescription>{migrationResult.warnings.join("；")}</AlertDescription>
                </Alert>
              ) : null}
              {migrationResult.pre_operation_backup && (
                <Alert className="border-emerald-500/40 bg-emerald-500/10">
                  <CheckCircle2 className="h-4 w-4" />
                  <AlertTitle>迁移已执行</AlertTitle>
                  <AlertDescription>
                    保护性备份：{migrationResult.pre_operation_backup.name}；目标：{migrationResult.target_driver}。
                  </AlertDescription>
                </Alert>
              )}
            </>
          )}
        </CardContent>
      </Card>

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>确认执行数据库迁移</DialogTitle>
            <DialogDescription>后端已完成预检。确认后会先创建保护性备份，再把快照写入目标后端。</DialogDescription>
          </DialogHeader>
          {migrationResult && (
            <div className="space-y-3 text-sm">
              <Alert>
                <Info className="h-4 w-4" />
                <AlertTitle>迁移只写入目标</AlertTitle>
                <AlertDescription>如需正式切换运行后端，请确认 database.driver 已改为目标类型并重启服务。</AlertDescription>
              </Alert>
              <div className="grid gap-2 rounded-md border p-3 text-xs">
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">来源</span><strong>{migrationResult.source_driver}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">目标</span><strong>{migrationResult.target_driver}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">快照大小</span><strong>{formatBytes(migrationResult.snapshot_bytes)}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">用户 / 卡码 / 邀请码</span><strong>{migrationResult.users} / {migrationResult.regcodes} / {migrationResult.invite_codes}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">登录 / 播放 / 签到</span><strong>{countOf(migrationResult, "login_logs")} / {countOf(migrationResult, "playback_records")} / {countOf(migrationResult, "signin")}</strong></div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)} disabled={busy}>取消</Button>
            <Button onClick={() => void executeMigration()} disabled={busy || !migrationResult}>
              {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <UploadCloud className="mr-2 h-4 w-4" />}
              确认迁移
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={backupPreviewOpen} onOpenChange={setBackupPreviewOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>数据库备份详情</DialogTitle>
            <DialogDescription>查看备份大小、创建时间和快照数据量。</DialogDescription>
          </DialogHeader>
          {backupPreview && (
            <div className="space-y-3 text-sm">
              <div className="rounded-md border p-3 text-xs">
                <div className="break-all font-medium">{backupPreview.backup.name}</div>
                <div className="mt-1 text-muted-foreground">{formatBytes(backupPreview.backup.size)} · {formatUnixTime(backupPreview.backup.created_at)}</div>
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
            <Button variant="outline" onClick={() => setBackupPreviewOpen(false)}>关闭</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={restoreOpen} onOpenChange={setRestoreOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>确认恢复数据库备份</DialogTitle>
            <DialogDescription>恢复会覆盖当前数据库状态；执行前会自动创建保护性备份。</DialogDescription>
          </DialogHeader>
          {restorePreview && (
            <div className="space-y-3 text-sm">
              <Alert className="border-amber-500/40 bg-amber-500/10">
                <AlertTriangle className="h-4 w-4" />
                <AlertTitle>高风险操作</AlertTitle>
                <AlertDescription>请确认当前数据库已不再需要，或确认保护性备份可用于回退。</AlertDescription>
              </Alert>
              <div className="grid gap-2 rounded-md border p-3 text-xs">
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">目标备份</span><strong className="break-all text-right">{restorePreview.restored}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">当前用户数</span><strong>{restorePreview.current_counts?.users ?? "-"}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">恢复后用户数</span><strong>{restorePreview.counts?.users ?? restorePreview.users}</strong></div>
                <div className="flex justify-between gap-3"><span className="text-muted-foreground">卡码 / 邀请码</span><strong>{restorePreview.regcodes} / {restorePreview.invite_codes}</strong></div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestoreOpen(false)} disabled={busy}>取消</Button>
            <Button onClick={() => void restoreBackup()} disabled={busy || !restorePreview}>
              {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RotateCcw className="mr-2 h-4 w-4" />}
              确认恢复
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
