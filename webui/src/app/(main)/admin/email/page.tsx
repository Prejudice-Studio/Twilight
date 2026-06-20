"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Mail,
  RefreshCw,
  Loader2,
  AlertTriangle,
  Search,
  Trash2,
  ShieldCheck,
  ShieldAlert,
  CheckCircle2,
  XCircle,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminConfigSections } from "@/components/admin/config-section-editor";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { api } from "@/lib/api";
import type { EmailAdminData } from "@/lib/api-types";

type VerifiedFilter = "all" | "verified" | "unverified";

const PURPOSE_LABEL: Record<string, MessageKey> = {
  bind: "emailAdmin.purposeBind",
  reset_password: "emailAdmin.purposeReset",
  change_password: "emailAdmin.purposeChangePw",
  change_emby_password: "emailAdmin.purposeChangeEmby",
};

function formatUnix(seconds: number | null | undefined, locale: string): string {
  if (!seconds || seconds <= 0) return "—";
  return new Date(seconds * 1000).toLocaleString(locale);
}

export default function AdminEmailPage() {
  const { t, locale } = useI18n();
  const { toast } = useToast();
  const { confirm } = useConfirm();

  const [data, setData] = useState<EmailAdminData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [tab, setTab] = useState<"pending" | "accounts" | "config">("pending");
  const [pendingSearch, setPendingSearch] = useState("");
  const [accountSearch, setAccountSearch] = useState("");
  const [verifiedFilter, setVerifiedFilter] = useState<VerifiedFilter>("all");
  const [revokingId, setRevokingId] = useState<string | null>(null);
  const [cleaning, setCleaning] = useState(false);
  const [clearingEmails, setClearingEmails] = useState(false);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.adminGetEmailVerifications();
      if (res.success && res.data) {
        setData(res.data);
      } else {
        throw new Error(res.message || t("emailAdmin.loadFailed"));
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : t("emailAdmin.loadFailed");
      setError(message);
      toast({ title: t("emailAdmin.loadFailed"), description: message, variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, [t, toast]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const handleRevoke = useCallback(
    async (id: string) => {
      const okConfirm = await confirm({
        title: t("emailAdmin.revokeTitle"),
        description: t("emailAdmin.revokeDesc"),
        tone: "danger",
        confirmLabel: t("emailAdmin.revoke"),
      });
      if (!okConfirm) return;
      setRevokingId(id);
      try {
        const res = await api.adminRevokeEmailVerification(id);
        if (res.success) {
          toast({ title: t("emailAdmin.revokeDone"), variant: "success" });
          await reload();
        } else {
          toast({ title: t("emailAdmin.revokeFailed"), description: res.message, variant: "destructive" });
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : t("emailAdmin.revokeFailed");
        toast({ title: t("emailAdmin.revokeFailed"), description: message, variant: "destructive" });
      } finally {
        setRevokingId(null);
      }
    },
    [confirm, reload, t, toast],
  );

  const handleCleanup = useCallback(async () => {
    setCleaning(true);
    try {
      const res = await api.adminCleanupEmailVerifications();
      if (res.success && res.data) {
        toast({ title: t("emailAdmin.cleanupDone", { count: res.data.deleted }), variant: "success" });
        await reload();
      } else {
        toast({ title: t("emailAdmin.cleanupFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : t("emailAdmin.cleanupFailed");
      toast({ title: t("emailAdmin.cleanupFailed"), description: message, variant: "destructive" });
    } finally {
      setCleaning(false);
    }
  }, [reload, t, toast]);

  const filteredPending = useMemo(() => {
    if (!data) return [];
    const q = pendingSearch.trim().toLowerCase();
    if (!q) return data.pending;
    return data.pending.filter((p) =>
      [p.email, p.username ?? "", p.uid != null ? String(p.uid) : "", p.purpose].some((h) =>
        h.toLowerCase().includes(q),
      ),
    );
  }, [data, pendingSearch]);

  const filteredAccounts = useMemo(() => {
    if (!data) return [];
    const q = accountSearch.trim().toLowerCase();
    return data.accounts.filter((acc) => {
      if (verifiedFilter === "verified" && !acc.email_verified) return false;
      if (verifiedFilter === "unverified" && acc.email_verified) return false;
      if (q) {
        const hay = [
          acc.username,
          String(acc.uid),
          acc.email,
          acc.telegram_username ?? "",
          acc.telegram_id != null ? String(acc.telegram_id) : "",
        ];
        if (!hay.some((h) => h.toLowerCase().includes(q))) return false;
      }
      return true;
    });
  }, [data, accountSearch, verifiedFilter]);

  const summary = data?.summary;

  const handleClearUnverified = useCallback(async () => {
    const ok = await confirm({
      title: t("emailAdmin.clearUnverifiedTitle"),
      description: t("emailAdmin.clearUnverifiedDesc", { count: summary?.unverified ?? 0 }),
      tone: "danger",
      confirmLabel: t("emailAdmin.clearUnverifiedConfirm"),
    });
    if (!ok) return;
    setClearingEmails(true);
    try {
      const res = await api.adminClearUnverifiedEmails();
      if (res.success && res.data) {
        toast({ title: t("emailAdmin.clearUnverifiedDone", { count: res.data.cleared }), variant: "success" });
        await reload();
      } else {
        toast({ title: t("emailAdmin.clearUnverifiedFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : t("emailAdmin.clearUnverifiedFailed");
      toast({ title: t("emailAdmin.clearUnverifiedFailed"), description: message, variant: "destructive" });
    } finally {
      setClearingEmails(false);
    }
  }, [confirm, reload, summary?.unverified, t, toast]);

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <Mail className="h-5 w-5" />
            {t("emailAdmin.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("emailAdmin.description")}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void reload()} disabled={loading}>
          {loading ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <RefreshCw className="mr-2 h-4 w-4" />
          )}
          {t("common.refresh")}
        </Button>
      </div>

      {/* Summary */}
      {summary && data && (
        <Card>
          <CardContent className="flex flex-wrap gap-2 p-4 text-xs">
            <Badge
              variant="outline"
              className={data.smtp_configured ? "border-emerald-500/30 text-emerald-500" : "border-amber-500/30 text-amber-500"}
            >
              {data.smtp_configured ? t("emailAdmin.smtpConfigured") : t("emailAdmin.smtpNotConfigured")}
            </Badge>
            <Badge variant="outline">
              {data.force_bind ? t("emailAdmin.forceBindOn") : t("emailAdmin.forceBindOff")}
            </Badge>
            <Badge variant="outline">{t("emailAdmin.summaryPending", { count: summary.total_pending })}</Badge>
            {summary.expired_pending > 0 && (
              <Badge variant="outline" className="border-amber-500/30 text-amber-500">
                {t("emailAdmin.summaryExpired", { count: summary.expired_pending })}
              </Badge>
            )}
            <Badge variant="outline">{t("emailAdmin.summaryWithEmail", { count: summary.total_with_email })}</Badge>
            <Badge variant="outline" className="border-emerald-500/30 text-emerald-500">
              {t("emailAdmin.summaryVerified", { count: summary.verified })}
            </Badge>
            <Badge variant="outline" className="border-amber-500/30 text-amber-500">
              {t("emailAdmin.summaryUnverified", { count: summary.unverified })}
            </Badge>
          </CardContent>
        </Card>
      )}

      {error ? (
        <Card className="border-destructive/40">
          <CardContent className="flex items-center gap-2 p-4 text-sm text-destructive">
            <AlertTriangle className="h-4 w-4" />
            {error}
          </CardContent>
        </Card>
      ) : loading && !data ? (
        <Card>
          <CardContent className="flex items-center justify-center gap-2 p-10 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t("common.loading")}
          </CardContent>
        </Card>
      ) : (
        <Tabs value={tab} onValueChange={(v) => setTab(v as "pending" | "accounts" | "config")}>
          <TabsList>
            <TabsTrigger value="pending">
              {t("emailAdmin.tabPending")} ({data?.pending.length ?? 0})
            </TabsTrigger>
            <TabsTrigger value="accounts">
              {t("emailAdmin.tabAccounts")} ({data?.accounts.length ?? 0})
            </TabsTrigger>
            <TabsTrigger value="config">{t("emailAdmin.tabConfig")}</TabsTrigger>
          </TabsList>

          {/* Pending verification codes */}
          {tab === "pending" && (
            <div className="mt-3 space-y-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <div className="relative sm:max-w-xs">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={pendingSearch}
                    onChange={(e) => setPendingSearch(e.target.value)}
                    placeholder={t("emailAdmin.searchPendingPlaceholder")}
                    className="pl-9"
                  />
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => void handleCleanup()}
                  disabled={cleaning || (summary?.expired_pending ?? 0) === 0}
                >
                  {cleaning ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Trash2 className="mr-2 h-4 w-4" />
                  )}
                  {t("emailAdmin.cleanupExpired")}
                </Button>
                {(summary?.unverified ?? 0) > 0 && (
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => void handleClearUnverified()}
                    disabled={clearingEmails}
                  >
                    {clearingEmails ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Trash2 className="mr-2 h-4 w-4" />
                    )}
                    {t("emailAdmin.clearUnverifiedBtn")}
                  </Button>
                )}
              </div>
              <div className="overflow-hidden rounded-lg border">
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead className="bg-muted/50">
                      <tr>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colPurpose")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colEmail")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colUser")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colAttempts")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colCreated")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colExpires")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colActions")}</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {filteredPending.map((p) => {
                        const purposeKey = PURPOSE_LABEL[p.purpose];
                        return (
                          <tr key={p.id} className="hover:bg-muted/30">
                            <td className="p-3">
                              <Badge variant="secondary">{purposeKey ? t(purposeKey) : p.purpose}</Badge>
                            </td>
                            <td className="p-3 font-mono text-xs">{p.email}</td>
                            <td className="p-3">
                              {p.username ? (
                                <span>
                                  {p.username}
                                  <span className="ml-1 text-xs text-muted-foreground">UID {p.uid}</span>
                                </span>
                              ) : (
                                <span className="text-muted-foreground">—</span>
                              )}
                            </td>
                            <td className="p-3 tabular-nums">
                              {p.attempts}/{p.max_attempts}
                            </td>
                            <td className="p-3 text-xs text-muted-foreground">{formatUnix(p.created_at, locale)}</td>
                            <td className="p-3 text-xs">
                              <div className="text-muted-foreground">{formatUnix(p.expires_at, locale)}</div>
                              {p.expired ? (
                                <Badge variant="secondary" className="mt-1">{t("emailAdmin.codeExpired")}</Badge>
                              ) : (
                                <Badge className="mt-1 border-emerald-500/20 bg-emerald-500/10 text-emerald-500">
                                  {t("emailAdmin.codeActive")}
                                </Badge>
                              )}
                            </td>
                            <td className="p-3">
                              <Button
                                variant="ghost"
                                size="sm"
                                className="text-destructive hover:text-destructive"
                                onClick={() => void handleRevoke(p.id)}
                                disabled={revokingId === p.id}
                              >
                                {revokingId === p.id ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <Trash2 className="h-4 w-4" />
                                )}
                                <span className="ml-1">{t("emailAdmin.revoke")}</span>
                              </Button>
                            </td>
                          </tr>
                        );
                      })}
                      {filteredPending.length === 0 && (
                        <tr>
                          <td colSpan={7} className="p-6 text-center text-muted-foreground">
                            {t("emailAdmin.emptyPending")}
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}

          {/* Email accounts */}
          {tab === "accounts" && (
            <div className="mt-3 space-y-3">
              <div className="grid gap-2 sm:grid-cols-[minmax(220px,1fr)_200px]">
                <div className="relative">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={accountSearch}
                    onChange={(e) => setAccountSearch(e.target.value)}
                    placeholder={t("emailAdmin.searchAccountPlaceholder")}
                    className="pl-9"
                  />
                </div>
                <Select value={verifiedFilter} onValueChange={(v) => setVerifiedFilter(v as VerifiedFilter)}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t("emailAdmin.filterAll")}</SelectItem>
                    <SelectItem value="verified">{t("emailAdmin.filterVerified")}</SelectItem>
                    <SelectItem value="unverified">{t("emailAdmin.filterUnverified")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="overflow-hidden rounded-lg border">
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead className="bg-muted/50">
                      <tr>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colUser")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colEmail")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colVerifyStatus")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colTelegram")}</th>
                        <th className="p-3 text-left font-medium">{t("emailAdmin.colAccountStatus")}</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y">
                      {filteredAccounts.map((acc) => (
                        <tr key={acc.uid} className="hover:bg-muted/30">
                          <td className="p-3">
                            <div className="font-medium">{acc.username}</div>
                            <div className="text-xs text-muted-foreground">UID {acc.uid}</div>
                          </td>
                          <td className="p-3 font-mono text-xs">{acc.email}</td>
                          <td className="p-3">
                            {acc.email_verified ? (
                              <div className="flex items-center gap-1 text-emerald-500">
                                <ShieldCheck className="h-4 w-4" />
                                <span className="text-xs">{t("emailAdmin.verified")}</span>
                              </div>
                            ) : (
                              <div className="flex items-center gap-1 text-amber-500">
                                <ShieldAlert className="h-4 w-4" />
                                <span className="text-xs">{t("emailAdmin.unverified")}</span>
                              </div>
                            )}
                            {acc.email_verified && acc.email_verified_at && (
                              <div className="mt-0.5 text-[11px] text-muted-foreground">
                                {formatUnix(acc.email_verified_at, locale)}
                              </div>
                            )}
                          </td>
                          <td className="p-3 text-xs">
                            {acc.telegram_id != null ? (
                              <span className="font-mono">
                                {acc.telegram_username ? `@${acc.telegram_username}` : acc.telegram_id}
                              </span>
                            ) : (
                              <span className="text-muted-foreground">{t("emailAdmin.notBound")}</span>
                            )}
                          </td>
                          <td className="p-3">
                            <div className="flex flex-wrap gap-1">
                              {acc.role === 0 && (
                                <Badge className="border-purple-500/20 bg-purple-500/10 text-purple-500">
                                  {t("emailAdmin.adminBadge")}
                                </Badge>
                              )}
                              {acc.active ? (
                                <Badge variant="outline" className="gap-1 border-emerald-500/20 text-emerald-500">
                                  <CheckCircle2 className="h-3 w-3" />
                                </Badge>
                              ) : (
                                <Badge variant="destructive" className="gap-1">
                                  <XCircle className="h-3 w-3" />
                                  {t("emailAdmin.disabledBadge")}
                                </Badge>
                              )}
                            </div>
                          </td>
                        </tr>
                      ))}
                      {filteredAccounts.length === 0 && (
                        <tr>
                          <td colSpan={5} className="p-6 text-center text-muted-foreground">
                            {t("emailAdmin.emptyAccounts")}
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}

          {tab === "config" && (
            <div className="mt-4">
              <AdminConfigSections
                sectionKeys={["Email", "Scheduler", "Notification", "RateLimit"]}
                sectionFieldKeys={{
                  Email: [
                    "enabled",
                    "smtp_host",
                    "smtp_port",
                    "smtp_username",
                    "smtp_password",
                    "smtp_encryption",
                    "smtp_from_address",
                    "smtp_from_name",
                    "smtp_timeout_seconds",
                    "force_bind",
                    "code_length",
                    "code_type",
                    "code_ttl_minutes",
                    "resend_cooldown_seconds",
                    "max_attempts",
                    "auto_cleanup_expired_verifications",
                    "auto_cleanup_unverified",
                    "auto_cleanup_unverified_days",
                    "email_validation_mode",
                    "email_whitelist",
                    "email_blacklist",
                    "subject_template",
                    "body_template",
                  ],
                  Scheduler: ["session_cleanup_interval"],
                  RateLimit: ["email_code_ip_per_10m", "email_code_uid_per_10m", "email_code_addr_per_10m"],
                }}
                title={t("emailAdmin.configTitle")}
                description={t("emailAdmin.configDescription")}
                notice={t("emailAdmin.configNotice")}
              />
            </div>
          )}
        </Tabs>
      )}
    </div>
  );
}
