"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, BookOpen, Code2, Loader2, Plus, RotateCcw, Save, Shield, Trash2 } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import type { DeveloperJSPreset } from "@/lib/api-types";
import { useI18n } from "@/lib/i18n";

type CommandType = "text" | "js";

type CommandRow = {
  id: string;
  command: string;
  type: CommandType;
  text: string;
  presetId: string;
  inlineCode?: string;
};

const jsPrefix = "js:";
const jsPresetPrefix = "preset:";
const nonePreset = "__none";

// 内置指令列表与分类
const BUILTIN_COMMANDS: { command: string; label: string; admin: boolean; description: string }[] = [
  { command: "bind", label: "/bind", admin: false, description: "绑定 Telegram 账号到 Web 账户" },
  { command: "about", label: "/about", admin: false, description: "服务说明" },
  { command: "cancel", label: "/cancel", admin: false, description: "取消当前操作" },
  { command: "me", label: "/me", admin: false, description: "查看当前绑定信息" },
  { command: "emby", label: "/emby", admin: false, description: "查看 Emby 状态" },
  { command: "resetpwd", label: "/resetpwd", admin: false, description: "密码重置指引" },
  { command: "delaccount", label: "/delaccount", admin: false, description: "自助销号" },
  { command: "version", label: "/version", admin: false, description: "显示版本号" },
  { command: "ping", label: "/ping", admin: false, description: "连通性测试" },
  { command: "notice", label: "/notice", admin: false, description: "查看最新公告" },
  { command: "stats", label: "/stats", admin: true, description: "服务统计" },
  { command: "admin", label: "/admin", admin: true, description: "管理员查询入口" },
  { command: "userinfo", label: "/userinfo", admin: true, description: "查看指定用户详情" },
  { command: "twfind", label: "/twfind", admin: true, description: "搜索用户" },
  { command: "twishelp", label: "/twishelp", admin: true, description: "管理员帮助" },
  { command: "banweb", label: "/banweb", admin: true, description: "禁用 Web 账号" },
  { command: "banemby", label: "/banemby", admin: true, description: "禁用 Emby 账号" },
  { command: "broadcast", label: "/broadcast", admin: true, description: "向用户广播消息" },
];

function rowID() {
  return `cmd-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function normalizeCommand(value: string) {
  const trimmed = value.trim().replace(/^\/+/, "").toLowerCase();
  return trimmed ? `/${trimmed}` : "/";
}

function commandRows(value: unknown, presets: DeveloperJSPreset[]): CommandRow[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => {
    const row = item as Record<string, unknown>;
    const command = normalizeCommand(String(row.command ?? ""));
    const reply = String(row.reply ?? "");
    const trimmed = reply.trim();
    if (trimmed.toLowerCase().startsWith(jsPrefix)) {
      const code = trimmed.slice(jsPrefix.length).trim();
      if (code.toLowerCase().startsWith(jsPresetPrefix)) {
        const presetId = code.slice(jsPresetPrefix.length).trim();
        return {
          id: rowID(),
          command,
          type: "js",
          text: "",
          presetId,
        };
      }
      const preset = presets.find((candidate) => candidate.code.trim() === code);
      return {
        id: rowID(),
        command,
        type: "js",
        text: "",
        presetId: preset ? String(preset.id) : "",
        inlineCode: preset ? undefined : code,
      };
    }
    return { id: rowID(), command, type: "text", text: reply, presetId: "" };
  });
}

function rowsToConfig(rows: CommandRow[], presets: DeveloperJSPreset[]) {
  return rows
    .map((row) => {
      const command = normalizeCommand(row.command);
      if (command === "/") return null;
      if (row.type === "text") {
        const reply = row.text.trim();
        return reply ? { command, reply } : null;
      }
      const preset = presets.find((item) => String(item.id) === row.presetId);
      if (preset) return { command, reply: `${jsPrefix}${jsPresetPrefix}${preset.id}` };
      const code = row.inlineCode?.trim() || "";
      return code ? { command, reply: `${jsPrefix}${code}` } : null;
    })
    .filter(Boolean);
}

export default function AdminTelegramCommandsPage() {
  const { t } = useI18n();
  const { toast } = useToast();
  const [presets, setPresets] = useState<DeveloperJSPreset[]>([]);
  const [rows, setRows] = useState<CommandRow[]>([]);
  const [original, setOriginal] = useState<CommandRow[]>([]);
  const [disabledCommands, setDisabledCommands] = useState<string[]>([]);
  const [originalDisabled, setOriginalDisabled] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [schemaRes, presetRes] = await Promise.all([
        api.getConfigSchema(),
        api.listDeveloperJSPresets(),
      ]);
      if (!schemaRes.success || !schemaRes.data) throw new Error(schemaRes.message || t("adminTelegramCommands.loadFailed"));
      const nextPresets = presetRes.success && presetRes.data ? presetRes.data.presets : [];
      const telegram = schemaRes.data.sections.find((section) => section.key === "Telegram");
      const field = telegram?.fields.find((item) => item.key === "bot_custom_commands");
      const nextRows = commandRows(field?.value ?? [], nextPresets);
      const disabledField = telegram?.fields.find((item) => item.key === "disabled_commands");
      const nextDisabled = Array.isArray(disabledField?.value) ? (disabledField!.value as string[]).map((s: string) => s.trim().toLowerCase()).filter(Boolean) : [];
      setPresets(nextPresets);
      setRows(nextRows);
      setOriginal(JSON.parse(JSON.stringify(nextRows)));
      setDisabledCommands(nextDisabled);
      setOriginalDisabled([...nextDisabled]);
    } catch (err) {
      toast({ title: t("adminTelegramCommands.loadFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, [t, toast]);

  useEffect(() => {
    void load();
  }, [load]);

  const changed = useMemo(() => JSON.stringify(rows) !== JSON.stringify(original) || JSON.stringify(disabledCommands) !== JSON.stringify(originalDisabled), [rows, original, disabledCommands, originalDisabled]);

  const updateRow = (id: string, patch: Partial<CommandRow>) => {
    setRows((current) => current.map((row) => (row.id === id ? { ...row, ...patch } : row)));
  };

  const addRow = () => {
    setRows((current) => [...current, { id: rowID(), command: "/", type: "text", text: "", presetId: "" }]);
  };

  const changedDisabled = useMemo(() => JSON.stringify(disabledCommands) !== JSON.stringify(originalDisabled), [disabledCommands, originalDisabled]);

  const save = async () => {
    setSaving(true);
    try {
      const payload = rowsToConfig(rows, presets);
      const res = await api.updateConfigBySchema({ Telegram: { bot_custom_commands: payload, disabled_commands: disabledCommands } });
      if (!res.success) throw new Error(res.message || t("adminTelegramCommands.saveFailed"));
      toast({ title: t("adminTelegramCommands.saved"), variant: "success" });
      await load();
    } catch (err) {
      toast({ title: t("adminTelegramCommands.saveFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <div className="flex h-64 items-center justify-center"><Loader2 className="h-8 w-8 animate-spin text-muted-foreground" /></div>;
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <Code2 className="h-6 w-6" />
            {t("adminTelegramCommands.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("adminTelegramCommands.description")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button asChild variant="outline">
            <Link href="/admin/developer">
              <BookOpen className="mr-2 h-4 w-4" />
              {t("adminTelegramCommands.openDeveloper")}
            </Link>
          </Button>
          <Button variant="outline" onClick={() => setRows(JSON.parse(JSON.stringify(original)))} disabled={!changed || saving}>
            <RotateCcw className="mr-2 h-4 w-4" />
            {t("common.reset")}
          </Button>
          <Button onClick={() => void save()} disabled={!changed || saving}>
            {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
            {t("common.save")}
          </Button>
        </div>
      </div>

      <Alert className="border-amber-500/40 bg-amber-500/10">
        <AlertTriangle className="h-4 w-4" />
        <AlertTitle>{t("adminTelegramCommands.noticeTitle")}</AlertTitle>
        <AlertDescription>{t("adminTelegramCommands.notice")}</AlertDescription>
      </Alert>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><Shield className="h-5 w-5" />{t("adminTelegramCommands.builtinTitle")}</CardTitle>
          <CardDescription>{t("adminTelegramCommands.builtinDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {BUILTIN_COMMANDS.map((cmd) => {
              const isDisabled = disabledCommands.includes(cmd.command);
              return (
                <div key={cmd.command} className="flex items-center justify-between gap-2 rounded-lg border p-2.5">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5">
                      <code className="text-sm font-medium">{cmd.label}</code>
                      {cmd.admin && <Badge variant="outline" className="text-[9px] px-1 py-0">{t("adminTelegramCommands.adminBadge")}</Badge>}
                    </div>
                    <p className="text-[11px] text-muted-foreground truncate">{cmd.description}</p>
                  </div>
                  <Switch
                    checked={!isDisabled}
                    onCheckedChange={(v) => {
                      setDisabledCommands((prev) => {
                        const next = new Set(prev);
                        if (v) next.delete(cmd.command);
                        else next.add(cmd.command);
                        return Array.from(next).sort();
                      });
                    }}
                  />
                </div>
              );
            })}
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <Card>
          <CardHeader>
            <CardTitle>{t("adminTelegramCommands.listTitle")}</CardTitle>
            <CardDescription>{t("adminTelegramCommands.listDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {rows.map((row, index) => (
              <div key={row.id} className="grid gap-3 rounded-lg border p-3 lg:grid-cols-[160px_140px_minmax(0,1fr)_auto]">
                <div className="space-y-2">
                  <Label>{t("adminTelegramCommands.command")}</Label>
                  <Input value={row.command} onChange={(event) => updateRow(row.id, { command: event.target.value })} onBlur={() => updateRow(row.id, { command: normalizeCommand(row.command) })} placeholder="/hello" />
                </div>
                <div className="space-y-2">
                  <Label>{t("adminTelegramCommands.type")}</Label>
                  <Select value={row.type} onValueChange={(value) => updateRow(row.id, { type: value as CommandType })}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="text">{t("adminTelegramCommands.typeText")}</SelectItem>
                      <SelectItem value="js">{t("adminTelegramCommands.typeJs")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{row.type === "text" ? t("adminTelegramCommands.replyText") : t("adminTelegramCommands.jsPreset")}</Label>
                  {row.type === "text" ? (
                    <Textarea value={row.text} onChange={(event) => updateRow(row.id, { text: event.target.value })} className="min-h-24" placeholder={t("adminTelegramCommands.textPlaceholder")} />
                  ) : (
                    <div className="space-y-2">
                      <Select value={row.presetId || nonePreset} onValueChange={(value) => updateRow(row.id, { presetId: value === nonePreset ? "" : value, inlineCode: undefined })}>
                        <SelectTrigger>
                          <SelectValue placeholder={t("adminTelegramCommands.choosePreset")} />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value={nonePreset}>{t("adminTelegramCommands.choosePreset")}</SelectItem>
                          {presets.map((preset) => (
                            <SelectItem key={preset.id} value={String(preset.id)}>{preset.name}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {row.inlineCode && !row.presetId ? (
                        <p className="text-xs text-amber-600">{t("adminTelegramCommands.inlineJsWarning")}</p>
                      ) : null}
                    </div>
                  )}
                </div>
                <div className="flex items-start justify-between gap-2 lg:flex-col">
                  <Badge variant="outline">#{index + 1}</Badge>
                  <Button type="button" variant="ghost" size="icon" onClick={() => setRows((current) => current.filter((item) => item.id !== row.id))}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))}
            <Button type="button" variant="outline" className="w-full" onClick={addRow}>
              <Plus className="mr-2 h-4 w-4" />
              {t("adminTelegramCommands.add")}
            </Button>
          </CardContent>
        </Card>

        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>{t("adminTelegramCommands.examplesTitle")}</CardTitle>
              <CardDescription>{t("adminTelegramCommands.examplesDescription")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <div className="rounded-md border bg-muted/20 p-3">
                <p className="font-medium">{t("adminTelegramCommands.textExampleTitle")}</p>
                <code className="mt-2 block whitespace-pre-wrap text-xs">/hello = {t("adminTelegramCommands.textExample")}</code>
              </div>
              <div className="rounded-md border bg-muted/20 p-3">
                <p className="font-medium">{t("adminTelegramCommands.jsExampleTitle")}</p>
                <code className="mt-2 block whitespace-pre-wrap text-xs">/hello = js:preset:1</code>
              </div>
              <div className="flex flex-wrap gap-2">
                {["{server_name}", "{bot_username}", "{user_name}"].map((item) => (
                  <Badge key={item} variant="outline">{item}</Badge>
                ))}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("adminTelegramCommands.presetTitle")}</CardTitle>
              <CardDescription>{t("adminTelegramCommands.presetDescription")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              {presets.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t("adminTelegramCommands.noPresets")}</p>
              ) : presets.map((preset) => (
                <div key={preset.id} className="rounded-md border p-2">
                  <p className="truncate text-sm font-medium">{preset.name}</p>
                  {preset.description ? <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{preset.description}</p> : null}
                </div>
              ))}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
