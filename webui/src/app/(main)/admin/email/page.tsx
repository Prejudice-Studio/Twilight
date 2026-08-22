"use client";

import { useCallback, useEffect, useState } from "react";
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
import { useAsyncResource } from "@/hooks/use-async-resource";
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

  const [tab, setTab] = useState<"pending" | "accounts" | "config">("pending");
  const [listView, setListView] = useState<"pending" | "accounts">("pending");
  const [pendingSearch, setPendingSearch] = useState("");
  const [accountSearch, setAccountSearch] = useState("");
  const [verifiedFilter, setVerifiedFilter] = useState<VerifiedFilter>("all");
  const [pendingQuery, setPendingQuery] = useState("");
  const [accountQuery, setAccountQuery] = useState("");
  const [pendingPage, setPendingPage] = useState(1);
  const [accountPage, setAccountPage] = useState(1);
  const [revokingId, setRevokingId] = useState<string | null>(null);
  const [cleaning, setCleaning] = useState(false);
  const [clearingEmails, setClearingEmails] = useState(false);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setPendingQuery(pendingSearch.trim());
      setPendingPage(1);
    }, 250);
    return () => window.clearTimeout(timer);
  }, [pendingSearch]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setAccountQuery(accountSearch.trim());
      setAccountPage(1);
    }, 250);
    return () => window.clearTimeout(timer);
  }, [accountSearch]);

  const loadResource = useCallback(async (signal?: AbortSignal) => {
    const page = listView === "accounts" ? accountPage : pendingPage;
    const search = listView === "accounts" ? accountQuery : pendingQuery;
    const res = await api.adminGetEmailVerifications({
      view: listView,
      page,
      per_page: 25,
      search,
      verified: listView === "accounts" ? verifiedFilter : "all",
    }, signal);
    if (!res.success || !res.data) {
      throw new Error(res.message || t("emailAdmin.loadFailed"));
    }
    return res.data;
  }, [accountPage, accountQuery, listView, pendingPage, pendingQuery, t, verifiedFilter]);

  const {
    data: loadedData,
    isLoading: loading,
    error,
    execute: reload,
    setData: setLoadedData,
  } = useAsyncResource(loadResource, { immediate: true });

  const data: EmailAdminData | null = loadedData ?? null;
  const hasActiveData = !!data && (!data.view || data.view === listView);

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

  const filteredPending = hasActiveData && tab === "pending" ? (data?.pending ?? []) : [];
  const filteredAccounts = hasActiveData && tab === "accounts" ? (data?.accounts ?? []) : [];

  const summary = data?.summary;
  const mutationBusy = revokingId !== null || cleaning || clearingEmails;

  useEffect(() => {
    if (data?.view === "pending") {
      const lastPage = Math.max(1, data.pages?.pending ?? 1);
      if (pendingPage > lastPage) setPendingPage(lastPage);
    }
    if (data?.view === "accounts") {
      const lastPage = Math.max(1, data.pages?.accounts ?? 1);
      if (accountPage > lastPage) setAccountPage(lastPage);
    }
  }, [accountPage, data, pendingPage]);

  const handleTabChange = (value: string) => {
    const next = value as "pending" | "accounts" | "config";
    setTab(next);
    if (next !== "config" && next !== listView) {
      setLoadedData(undefined);
      setListView(next);
    }
  };

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
        <Tabs value={tab} onValueChange={handleTabChange}>
          <TabsList className="max-w-full overflow-x-auto">
            <TabsTrigger value="pending">
              {t("emailAdmin.tabPending")} ({summary?.total_pending ?? 0})
            </TabsTrigger>
            <TabsTrigger value="accounts">
              {t("emailAdmin.tabAccounts")} ({summary?.total_with_email ?? 0})
            </TabsTrigger>
            <TabsTrigger value="config">{t("emailAdmin.tabConfig")}</TabsTrigger>
          </TabsList>

          {/* Pending verification codes */}
          {tab === "pending" && (
            <div className="mt-3 space-y-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <div className="relative w-full sm:max-w-xs">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={pendingSearch}
                    onChange={(e) => setPendingSearch(e.target.value)}
                    placeholder={t("emailAdmin.searchPendingPlaceholder")}
                    className="pl-9"
                  />
                </div>
                <div className="flex w-full flex-wrap gap-2 sm:w-auto">
                  <Button
                    variant="outline"
                    size="sm"
                    className="min-h-9 flex-1 sm:flex-none"
                    onClick={() => void handleCleanup()}
                    disabled={mutationBusy || (summary?.expired_pending ?? 0) === 0}
                  >
                    {cleaning ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
                    {t("emailAdmin.cleanupExpired")}
                  </Button>
                  {(summary?.unverified ?? 0) > 0 && (
                    <Button
                      variant="destructive"
                      size="sm"
                      className="min-h-9 flex-1 sm:flex-none"
                      onClick={() => void handleClearUnverified()}
                      disabled={mutationBusy}
                    >
                      {clearingEmails ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
                      {t("emailAdmin.clearUnverifiedBtn")}
                    </Button>
                  )}
                </div>
              </div>
              {loading && !hasActiveData ? (
                <Card><CardContent className="flex items-center justify-center gap-2 p-8 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" />{t("common.loading")}</CardContent></Card>
              ) : (
                <>
                  <div className="space-y-2 lg:hidden">
                    {filteredPending.map((p) => {
                      const purposeKey = PURPOSE_LABEL[p.purpose];
                      return (
                        <div key={p.id} className="space-y-3 rounded-lg border p-3">
                          <div className="flex min-w-0 items-start justify-between gap-2">
                            <div className="min-w-0">
                              <Badge variant="secondary">{purposeKey ? t(purposeKey) : p.purpose}</Badge>
                              <p className="mt-2 break-all font-mono text-xs">{p.email}</p>
                            </div>
                            <Badge variant={p.expired ? "secondary" : "outline"} className={!p.expired ? "border-emerald-500/20 text-emerald-500" : ""}>
                              {p.expired ? t("emailAdmin.codeExpired") : t("emailAdmin.codeActive")}
                            </Badge>
                          </div>
                          <dl className="grid grid-cols-2 gap-x-3 gap-y-2 text-xs">
                            <div className="min-w-0"><dt className="text-muted-foreground">{t("emailAdmin.colUser")}</dt><dd className="mt-0.5 break-words">{p.username ? `${p.username} · UID ${p.uid}` : "—"}</dd></div>
                            <div><dt className="text-muted-foreground">{t("emailAdmin.colAttempts")}</dt><dd className="mt-0.5 tabular-nums">{p.attempts}/{p.max_attempts}</dd></div>
                            <div><dt className="text-muted-foreground">{t("emailAdmin.colCreated")}</dt><dd className="mt-0.5">{formatUnix(p.created_at, locale)}</dd></div>
                            <div><dt className="text-muted-foreground">{t("emailAdmin.colExpires")}</dt><dd className="mt-0.5">{formatUnix(p.expires_at, locale)}</dd></div>
                          </dl>
                          <Button
                            variant="outline"
                            size="sm"
                            className="min-h-9 w-full text-destructive hover:text-destructive"
                            onClick={() => void handleRevoke(p.id)}
                            disabled={mutationBusy}
                          >
                            {revokingId === p.id ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
                            {t("emailAdmin.revoke")}
                          </Button>
                        </div>
                      );
                    })}
                    {filteredPending.length === 0 && <div className="rounded-lg border p-6 text-center text-sm text-muted-foreground">{t("emailAdmin.emptyPending")}</div>}
                  </div>
                  <div className="hidden overflow-hidden rounded-lg border lg:block">
                    <div className="custom-scrollbar overflow-x-auto overscroll-x-contain">
                      <table className="w-full min-w-[760px] text-sm">
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
                                disabled={mutationBusy}
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
                </>
              )}
              {hasActiveData && tab === "pending" && (data?.pages?.pending ?? 0) > 1 && (
                <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
                  <span className="text-muted-foreground">{t("common.pageStatus", { page: pendingPage, pages: data?.pages?.pending ?? 0 })}</span>
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" disabled={pendingPage <= 1 || loading} onClick={() => setPendingPage((page) => Math.max(1, page - 1))}>{t("common.previousPage")}</Button>
                    <Button variant="outline" size="sm" disabled={pendingPage >= (data?.pages?.pending ?? 0) || loading} onClick={() => setPendingPage((page) => page + 1)}>{t("common.nextPage")}</Button>
                  </div>
                </div>
              )}
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
                <Select value={verifiedFilter} onValueChange={(v) => { setVerifiedFilter(v as VerifiedFilter); setAccountPage(1); }}>
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
              {loading && !hasActiveData ? (
                <Card><CardContent className="flex items-center justify-center gap-2 p-8 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" />{t("common.loading")}</CardContent></Card>
              ) : (
                <>
                  <div className="space-y-2 lg:hidden">
                    {filteredAccounts.map((acc) => (
                      <div key={acc.uid} className="space-y-3 rounded-lg border p-3">
                        <div className="flex min-w-0 items-start justify-between gap-2">
                          <div className="min-w-0">
                            <p className="break-words font-medium">{acc.username}</p>
                            <p className="text-xs text-muted-foreground">UID {acc.uid}</p>
                          </div>
                          <div className={acc.email_verified ? "flex items-center gap-1 text-emerald-500" : "flex items-center gap-1 text-amber-500"}>
                            {acc.email_verified ? <ShieldCheck className="h-4 w-4" /> : <ShieldAlert className="h-4 w-4" />}
                            <span className="text-xs">{acc.email_verified ? t("emailAdmin.verified") : t("emailAdmin.unverified")}</span>
                          </div>
                        </div>
                        <p className="break-all font-mono text-xs">{acc.email}</p>
                        <div className="flex flex-wrap items-center gap-2 text-xs">
                          <span className="text-muted-foreground">{t("emailAdmin.colTelegram")}:</span>
                          <span className="break-all font-mono">{acc.telegram_id != null ? (acc.telegram_username ? `@${acc.telegram_username}` : acc.telegram_id) : t("emailAdmin.notBound")}</span>
                        </div>
                        <div className="flex flex-wrap gap-1">
                          {acc.role === 0 && <Badge className="border-info/20 bg-info/10 text-info">{t("emailAdmin.adminBadge")}</Badge>}
                          {acc.active ? (
                            <Badge variant="outline" className="gap-1 border-emerald-500/20 text-emerald-500"><CheckCircle2 className="h-3 w-3" />{t("emailAdmin.enabledBadge")}</Badge>
                          ) : (
                            <Badge variant="destructive" className="gap-1"><XCircle className="h-3 w-3" />{t("emailAdmin.disabledBadge")}</Badge>
                          )}
                        </div>
                      </div>
                    ))}
                    {filteredAccounts.length === 0 && <div className="rounded-lg border p-6 text-center text-sm text-muted-foreground">{t("emailAdmin.emptyAccounts")}</div>}
                  </div>
                  <div className="hidden overflow-hidden rounded-lg border lg:block">
                    <div className="custom-scrollbar overflow-x-auto overscroll-x-contain">
                      <table className="w-full min-w-[680px] text-sm">
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
                                <Badge className="border-info/20 bg-info/10 text-info">
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
                </>
              )}
              {hasActiveData && tab === "accounts" && (data?.pages?.accounts ?? 0) > 1 && (
                <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
                  <span className="text-muted-foreground">{t("common.pageStatus", { page: accountPage, pages: data?.pages?.accounts ?? 0 })}</span>
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" disabled={accountPage <= 1 || loading} onClick={() => setAccountPage((page) => Math.max(1, page - 1))}>{t("common.previousPage")}</Button>
                    <Button variant="outline" size="sm" disabled={accountPage >= (data?.pages?.accounts ?? 0) || loading} onClick={() => setAccountPage((page) => page + 1)}>{t("common.nextPage")}</Button>
                  </div>
                </div>
              )}
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
