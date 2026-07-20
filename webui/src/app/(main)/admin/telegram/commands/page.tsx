"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, BookOpen, Code2, Loader2, Plus, RotateCcw, Save, Search, Shield, Trash2 } from "lucide-react";
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
import type { DeveloperJSPreset, TelegramCommandCatalogItem } from "@/lib/api-types";
import { deepClone } from "@/lib/deep-clone";
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

function rowID() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return `cmd-${crypto.randomUUID()}`;
  }
  return `cmd-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function normalizeCommand(value: string) {
  const trimmed = value.trim().replace(/^\/+/, "").toLowerCase().replace(/[^a-z0-9_]/g, "").slice(0, 32);
  return trimmed ? `/${trimmed}` : "/";
}

function commandName(value: string) {
  return normalizeCommand(value).replace(/^\/+/, "");
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
  const [catalog, setCatalog] = useState<TelegramCommandCatalogItem[]>([]);
  const [rows, setRows] = useState<CommandRow[]>([]);
  const [original, setOriginal] = useState<CommandRow[]>([]);
  const [disabledCommands, setDisabledCommands] = useState<string[]>([]);
  const [originalDisabled, setOriginalDisabled] = useState<string[]>([]);
  const [builtinQuery, setBuiltinQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [schemaRes, presetRes, commandCatalogRes] = await Promise.all([
        api.getConfigSchema(),
        api.listDeveloperJSPresets(),
        api.getTelegramCommandCatalog(),
      ]);
      if (!schemaRes.success || !schemaRes.data) throw new Error(schemaRes.message || t("adminTelegramCommands.loadFailed"));
      const nextPresets = presetRes.success && presetRes.data ? presetRes.data.presets : [];
      if (!commandCatalogRes.success || !commandCatalogRes.data) throw new Error(commandCatalogRes.message || t("adminTelegramCommands.loadFailed"));
      const telegram = schemaRes.data.sections.find((section) => section.key === "Telegram");
      const field = telegram?.fields.find((item) => item.key === "bot_custom_commands");
      const nextRows = commandRows(field?.value ?? [], nextPresets);
      const disabledField = telegram?.fields.find((item) => item.key === "disabled_commands");
      const schemaDisabled = Array.isArray(disabledField?.value) ? (disabledField!.value as string[]).map((s: string) => commandName(s)).filter(Boolean) : [];
      const disableableNames = new Set(commandCatalogRes.data.commands.filter((item) => item.disableable).map((item) => commandName(item.command)));
      const catalogDisabled = commandCatalogRes.data.commands
        .filter((item) => item.disableable && item.disabled)
        .map((item) => commandName(item.command))
        .filter(Boolean);
      const nextDisabled = (catalogDisabled.length > 0 ? catalogDisabled : schemaDisabled).filter((name) => disableableNames.has(name));
      setPresets(nextPresets);
      setCatalog(commandCatalogRes.data.commands);
      setRows(nextRows);
      setOriginal(deepClone(nextRows));
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

  const builtinNames = useMemo(() => new Set(catalog.map((item) => commandName(item.command))), [catalog]);
  const filteredCatalog = useMemo(() => {
    const builtinQueryText = builtinQuery.trim().toLowerCase();
    if (!builtinQueryText) return catalog;
    return catalog.filter((item) => {
      const haystack = `${item.command} ${item.name} ${item.description} ${item.usage} ${item.category}`.toLowerCase();
      return haystack.includes(builtinQueryText);
    });
  }, [builtinQuery, catalog]);

  const rowErrors = useMemo(() => {
    const errors = new Map<string, string>();
    const seen = new Map<string, string>();
    for (const row of rows) {
      const name = commandName(row.command);
      if (!name) continue;
      if (builtinNames.has(name)) {
        errors.set(row.id, t("adminTelegramCommands.errorBuiltinConflict"));
        continue;
      }
      if (seen.has(name)) {
        errors.set(row.id, t("adminTelegramCommands.errorDuplicate"));
        const first = seen.get(name);
        if (first) errors.set(first, t("adminTelegramCommands.errorDuplicate"));
        continue;
      }
      seen.set(name, row.id);
      if (row.type === "text" && !row.text.trim()) {
        errors.set(row.id, t("adminTelegramCommands.errorEmptyReply"));
      }
      if (row.type === "js" && !row.presetId && !row.inlineCode?.trim()) {
        errors.set(row.id, t("adminTelegramCommands.errorMissingScript"));
      }
    }
    return errors;
  }, [builtinNames, rows, t]);

  const validationError = rowErrors.size > 0 ? Array.from(rowErrors.values())[0] : "";

  const categoryLabel = useCallback((category: string) => {
    switch (category) {
      case "user":
        return t("adminTelegramCommands.category.user");
      case "admin":
        return t("adminTelegramCommands.category.admin");
      case "system":
        return t("adminTelegramCommands.category.system");
      case "group":
        return t("adminTelegramCommands.category.group");
      default:
        return category;
    }
  }, [t]);

  const save = async () => {
    if (validationError) {
      toast({ title: t("adminTelegramCommands.validationFailed"), description: validationError, variant: "destructive" });
      return;
    }
    setSaving(true);
    try {
      const payload = rowsToConfig(rows, presets);
      const disableableNames = new Set(catalog.filter((item) => item.disableable).map((item) => commandName(item.command)));
      const normalizedDisabled = Array.from(new Set(disabledCommands.map(commandName).filter((name) => name && disableableNames.has(name)))).sort();
      const res = await api.updateConfigBySchema({ Telegram: { bot_custom_commands: payload, disabled_commands: normalizedDisabled } });
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
          <Button
            variant="outline"
            onClick={() => {
              setRows(deepClone(original));
              setDisabledCommands([...originalDisabled]);
            }}
            disabled={!changed || saving}
          >
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
          <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div>
              <CardTitle className="flex items-center gap-2"><Shield className="h-5 w-5" />{t("adminTelegramCommands.builtinTitle")}</CardTitle>
              <CardDescription>{t("adminTelegramCommands.builtinDescription")}</CardDescription>
            </div>
            <div className="relative w-full md:w-72">
              <Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                value={builtinQuery}
                onChange={(event) => setBuiltinQuery(event.target.value)}
                className="pl-8"
                placeholder={t("adminTelegramCommands.searchBuiltin")}
              />
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {filteredCatalog.map((cmd) => {
              const name = commandName(cmd.command);
              const isDisabled = disabledCommands.includes(name);
              return (
                <div key={cmd.command} className="flex min-h-24 items-start justify-between gap-2 rounded-lg border p-2.5">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <code className="text-sm font-medium">{cmd.label}</code>
                      {cmd.admin && <Badge variant="outline" className="text-[9px] px-1 py-0">{t("adminTelegramCommands.adminBadge")}</Badge>}
                      <Badge variant="secondary" className="px-1 py-0 text-[9px]">{categoryLabel(cmd.category)}</Badge>
                      {!cmd.disableable && <Badge variant="outline" className="px-1 py-0 text-[9px]">{t("adminTelegramCommands.fixedBadge")}</Badge>}
                    </div>
                    <p className="mt-1 line-clamp-2 text-[11px] text-muted-foreground">{cmd.description}</p>
                    <code className="mt-1 block truncate text-[11px] text-muted-foreground">{cmd.usage}</code>
                  </div>
                  <Switch
                    checked={!isDisabled}
                    disabled={!cmd.disableable}
                    onCheckedChange={(v) => {
                      setDisabledCommands((prev) => {
                        const next = new Set(prev);
                        if (v) next.delete(name);
                        else next.add(name);
                        return Array.from(next).sort();
                      });
                    }}
                    aria-label={`${cmd.label} ${t("adminTelegramCommands.enabled")}`}
                  />
                </div>
              );
            })}
            {filteredCatalog.length === 0 ? (
              <p className="col-span-full rounded-md border border-dashed p-4 text-sm text-muted-foreground">{t("adminTelegramCommands.noBuiltinMatches")}</p>
            ) : null}
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
                  {rowErrors.get(row.id) ? <p className="text-xs text-destructive">{rowErrors.get(row.id)}</p> : null}
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
                  <Button type="button" variant="ghost" size="icon" aria-label={t("common.delete")} onClick={() => setRows((current) => current.filter((item) => item.id !== row.id))}>
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
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("adminTelegramCommands.placeholderTitle")}</CardTitle>
              <CardDescription>{t("adminTelegramCommands.placeholderDescription")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              <div className="space-y-2 text-xs">
                <p className="font-medium text-muted-foreground">{t("adminTelegramCommands.placeholderCategoryBasic")}</p>
                <div className="flex flex-wrap gap-1.5">
                  {["{server_name}", "{bot_username}", "{user_name}"].map((item) => (
                    <button
                      key={item}
                      type="button"
                      className="inline-flex items-center rounded-md border bg-muted/30 px-2 py-0.5 font-mono text-[11px] hover:bg-muted cursor-pointer"
                      onClick={() => {
                        const active = document.activeElement;
                        if (active instanceof HTMLTextAreaElement) {
                          const start = active.selectionStart;
                          const end = active.selectionEnd;
                          const val = active.value;
                          active.value = val.slice(0, start) + item + val.slice(end);
                          active.selectionStart = active.selectionEnd = start + item.length;
                          active.dispatchEvent(new Event("input", { bubbles: true }));
                          active.focus();
                        }
                      }}
                      title={t("adminTelegramCommands.clickToInsert")}
                    >
                      {item}
                    </button>
                  ))}
                </div>
                <p className="font-medium text-muted-foreground mt-3">{t("adminTelegramCommands.placeholderCategoryTG")}</p>
                <div className="flex flex-wrap gap-1.5">
                  {["{tg_chat_id}", "{tg_from_id}", "{tg_username}", "{tg_first_name}"].map((item) => (
                    <button
                      key={item}
                      type="button"
                      className="inline-flex items-center rounded-md border bg-muted/30 px-2 py-0.5 font-mono text-[11px] hover:bg-muted cursor-pointer"
                      onClick={() => {
                        const active = document.activeElement;
                        if (active instanceof HTMLTextAreaElement) {
                          const start = active.selectionStart;
                          const end = active.selectionEnd;
                          const val = active.value;
                          active.value = val.slice(0, start) + item + val.slice(end);
                          active.selectionStart = active.selectionEnd = start + item.length;
                          active.dispatchEvent(new Event("input", { bubbles: true }));
                          active.focus();
                        }
                      }}
                      title={t("adminTelegramCommands.clickToInsert")}
                    >
                      {item}
                    </button>
                  ))}
                </div>
                <p className="font-medium text-muted-foreground mt-3">{t("adminTelegramCommands.placeholderCategoryUser")}</p>
                <div className="flex flex-wrap gap-1.5">
                  {["{web_status}", "{expire_status}", "{emby_status}", "{emby_enabled_status}", "{role}", "{register_time}", "{registration_source}", "{bgm_sync_status}"].map((item) => (
                    <button
                      key={item}
                      type="button"
                      className="inline-flex items-center rounded-md border bg-muted/30 px-2 py-0.5 font-mono text-[11px] hover:bg-muted cursor-pointer"
                      onClick={() => {
                        const active = document.activeElement;
                        if (active instanceof HTMLTextAreaElement) {
                          const start = active.selectionStart;
                          const end = active.selectionEnd;
                          const val = active.value;
                          active.value = val.slice(0, start) + item + val.slice(end);
                          active.selectionStart = active.selectionEnd = start + item.length;
                          active.dispatchEvent(new Event("input", { bubbles: true }));
                          active.focus();
                        }
                      }}
                      title={t("adminTelegramCommands.clickToInsert")}
                    >
                      {item}
                    </button>
                  ))}
                </div>
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
