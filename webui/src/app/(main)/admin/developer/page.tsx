"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  BookOpen,
  ChevronDown,
  ChevronRight,
  Code2,
  Copy,
  FileCode2,
  Loader2,
  Play,
  Plus,
  Save,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { JsCodeEditor } from "@/components/js-code-editor";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import type { DeveloperJSPreset, DeveloperJSDocs, DeveloperJSDocEntry } from "@/lib/api-types";
import { useI18n, type MessageKey } from "@/lib/i18n";

type DeveloperTemplate = {
  id: string;
  presetId?: number;
  title: MessageKey | string;
  description: MessageKey | string;
  code: string;
  builtin?: boolean;
  updatedAt?: number;
};

const helloTemplate = `// Greeting command
const name = user.username || "user";
reply("Hello " + name + ". Args: " + args.join(", "));`;

const statsTemplate = `// Compact user summary
const name = user.username || "user";
const uid = user.uid || "unknown";
reply([
  "User: " + name,
  "UID: " + uid,
  "Role: " + user.role,
  "Has Emby: " + (user.has_emby ? "yes" : "no")
].join("\\n"));`;

const adminGuardTemplate = `// Admin-only command
if (!auth("admin")) {
  reply("Permission denied");
  return;
}
log("admin command accepted for " + user.username);
reply("Admin command accepted");`;

const configTemplate = `// Read safe, non-secret system configuration
const siteName = config("app.name");
const inviteEnabled = config("invite.enabled");
const maxDepth = config("invite.max_depth");

reply([
  "Site: " + siteName,
  "Invite enabled: " + inviteEnabled,
  "Invite max depth: " + maxDepth
].join("\\n"));`;

const envTemplate = `// Read allowlisted non-secret environment variables
const host = env("TWILIGHT_HOST") || "not set";
const port = env("TWILIGHT_PORT") || "not set";
reply("API bind: " + host + ":" + port);`;

const argsRouterTemplate = `// Route by first argument
const action = (args[0] || "help").toLowerCase();
if (action === "ping") {
  reply("pong");
} else if (action === "me") {
  reply("You are " + (user.username || "unknown"));
} else {
  reply("Usage: /tool ping | me");
}`;

const dbAnnouncementsTemplate = `// Simple: list latest announcements
const list = db.listAnnouncements({ limit: 5 });
if (!list.length) {
  reply("No announcements");
  return;
}
const lines = list.map(function (a) {
  return "- " + a.title + (a.pinned ? " (pinned)" : "");
});
reply(lines.join("\\n"));`;

const dbMyRequestsTemplate = `// Simple: show my own media requests
const mine = db.listMediaRequests({ limit: 10 });
if (!mine.length) {
  reply("You have no media requests");
  return;
}
const lines = mine.map(function (r) {
  return "- " + r.title + " [" + r.status + "]";
});
reply(lines.join("\\n"));`;

const dbRegcodeReportTemplate = `// Complex (admin): summarize registration codes by status
if (!auth("admin")) {
  reply("Permission denied");
  return;
}
const codes = db.listRegcodes({ limit: 200 });
const buckets = {};
codes.forEach(function (c) {
  const key = c.status || "unknown";
  buckets[key] = (buckets[key] || 0) + 1;
});
const lines = Object.keys(buckets).sort().map(function (k) {
  return k + ": " + buckets[k];
});
reply(["RegCode report (" + codes.length + " scanned)"].concat(lines).join("\\n"));`;

const dbTicketTriageTemplate = `// Complex (admin): triage open tickets and ping the oldest
if (!auth("admin")) {
  reply("Permission denied");
  return;
}
const tickets = db.listTickets({ limit: 100 });
const open = tickets.filter(function (t) {
  return t.status !== "closed" && t.status !== "resolved";
});
if (!open.length) {
  reply("No open tickets");
  return;
}
open.sort(function (a, b) {
  return (a.created_at || 0) - (b.created_at || 0);
});
const oldest = open[0];
const lines = [
  "Open tickets: " + open.length,
  "Oldest: #" + oldest.id + " " + (oldest.title || ""),
  "Opened: " + time.formatUnix(oldest.created_at),
  "Type: " + (oldest.type || "n/a")
];
reply(lines.join("\\n"));`;

const builtInTemplates: DeveloperTemplate[] = [
  {
    id: "hello",
    title: "adminDeveloper.exampleHello",
    description: "adminDeveloper.exampleHelloDesc",
    code: helloTemplate,
    builtin: true,
  },
  {
    id: "stats",
    title: "adminDeveloper.exampleStats",
    description: "adminDeveloper.exampleStatsDesc",
    code: statsTemplate,
    builtin: true,
  },
  {
    id: "admin-guard",
    title: "adminDeveloper.exampleGuard",
    description: "adminDeveloper.exampleGuardDesc",
    code: adminGuardTemplate,
    builtin: true,
  },
  {
    id: "config",
    title: "adminDeveloper.exampleConfig",
    description: "adminDeveloper.exampleConfigDesc",
    code: configTemplate,
    builtin: true,
  },
  {
    id: "env",
    title: "adminDeveloper.exampleEnv",
    description: "adminDeveloper.exampleEnvDesc",
    code: envTemplate,
    builtin: true,
  },
  {
    id: "router",
    title: "adminDeveloper.exampleRouter",
    description: "adminDeveloper.exampleRouterDesc",
    code: argsRouterTemplate,
    builtin: true,
  },
  {
    id: "db-announcements",
    title: "adminDeveloper.exampleDbAnnouncements",
    description: "adminDeveloper.exampleDbAnnouncementsDesc",
    code: dbAnnouncementsTemplate,
    builtin: true,
  },
  {
    id: "db-my-requests",
    title: "adminDeveloper.exampleDbMyRequests",
    description: "adminDeveloper.exampleDbMyRequestsDesc",
    code: dbMyRequestsTemplate,
    builtin: true,
  },
  {
    id: "db-regcode-report",
    title: "adminDeveloper.exampleDbRegcodeReport",
    description: "adminDeveloper.exampleDbRegcodeReportDesc",
    code: dbRegcodeReportTemplate,
    builtin: true,
  },
  {
    id: "db-ticket-triage",
    title: "adminDeveloper.exampleDbTicketTriage",
    description: "adminDeveloper.exampleDbTicketTriageDesc",
    code: dbTicketTriageTemplate,
    builtin: true,
  },
];

const snippetRows = [
  {
    labelKey: "adminDeveloper.snippetReply",
    code: `reply("message");`,
  },
  {
    labelKey: "adminDeveloper.snippetLog",
    code: `log("debug message");`,
  },
  {
    labelKey: "adminDeveloper.snippetAdminGuard",
    code: `if (!auth("admin")) {
  reply("Permission denied");
  return;
}
`,
  },
  {
    labelKey: "adminDeveloper.snippetAuthAdmin",
    code: `if (!authAdmin()) {
  reply("Admin only");
  return;
}
`,
  },
  {
    labelKey: "adminDeveloper.snippetConfig",
    code: `const siteName = config("app.name");`,
  },
  {
    labelKey: "adminDeveloper.snippetEnv",
    code: `const host = env("TWILIGHT_HOST");`,
  },
  {
    labelKey: "adminDeveloper.snippetArgs",
    code: `const firstArg = args[0] || "";`,
  },
  {
    labelKey: "adminDeveloper.snippetCommandContext",
    code: `const me = users.current();
const lines = [
  "private_chat=" + ctx.private_chat,
  "preview=" + ctx.preview,
  "command_time=" + time.formatUnix(ctx.command_time),
  "args=" + JSON.stringify(args),
  "uid=" + me.uid,
  "username=" + (me.username || "unbound"),
  "role=" + me.role,
  "active=" + me.active,
  "has_emby=" + me.has_emby,
  "email_verified=" + me.email_verified,
  "telegram_bound=" + me.telegram_bound,
  "notify_tg=" + me.notify_on_login_telegram,
  "notify_email=" + me.notify_on_login_email
];
reply(text.truncate(text.joinLines(lines), 1200));`,
  },
  {
    labelKey: "adminDeveloper.snippetCurrentUser",
    code: `const me = users.current();`,
  },
  {
    labelKey: "adminDeveloper.snippetNotify",
    code: `const result = users.setLoginNotify({ telegram: true });`,
  },
  {
    labelKey: "adminDeveloper.snippetDbSchema",
    code: `const schema = db.schema();
reply(schema.users.fields.join(", "));`,
  },
  {
    labelKey: "adminDeveloper.snippetDbCount",
    code: `reply("Users: " + db.count("users"));`,
  },
  {
    labelKey: "adminDeveloper.snippetDbUpdate",
    code: `const result = db.updateCurrentUser({ notify_on_login_telegram: true });
reply(JSON.stringify(result));`,
  },
  {
    labelKey: "adminDeveloper.snippetFetch",
    code: `const res = fetch("https://example.com");
reply(res.ok ? text.truncate(res.text, 200) : ("fetch failed: " + (res.error || res.status)));`,
  },
  {
    labelKey: "adminDeveloper.snippetText",
    code: `reply(text.truncate(args.join(" "), 120));`,
  },
  {
    labelKey: "adminDeveloper.snippetArray",
    code: `const uniqueArgs = arrays.unique(arrays.compact(args));`,
  },
  {
    labelKey: "adminDeveloper.snippetInline",
    code: `interactions.inline("Choose an action", [
  { text: "Status", answer: "OK", edit: "Status acknowledged" },
  { text: "Help", reply: "Use /help for commands" }
]);`,
  },
  {
    labelKey: "adminDeveloper.snippetWaitText",
    code: `interactions.waitText({
  seconds: 30,
  prompt: "Send one line in 30 seconds",
  reply_prefix: "Received:",
  max_chars: 120
});`,
  },
  {
    labelKey: "adminDeveloper.snippetDbList",
    code: `const list = db.listAnnouncements({ limit: 5 });
reply(list.map(function (a) { return "- " + a.title; }).join("\\n"));`,
  },
] as const;

function templateText(value: MessageKey | string, t: (key: MessageKey) => string): string {
  return value.startsWith("adminDeveloper.") ? t(value as MessageKey) : value;
}

function presetToTemplate(preset: DeveloperJSPreset): DeveloperTemplate {
  return {
    id: `preset-${preset.id}`,
    presetId: preset.id,
    title: preset.name,
    description: preset.description || "",
    code: preset.code || "",
    updatedAt: preset.updated_at,
  };
}

export default function AdminDeveloperPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const editorRef = useRef<HTMLTextAreaElement | null>(null);
  const [code, setCode] = useState(helloTemplate);
  const [running, setRunning] = useState(false);
  const [savingTemplate, setSavingTemplate] = useState(false);
  const [templateName, setTemplateName] = useState("");
  const [templateDescription, setTemplateDescription] = useState("");
  const [serverPresets, setServerPresets] = useState<DeveloperJSPreset[]>([]);
  const [activeTemplateId, setActiveTemplateId] = useState("hello");
  const [result, setResult] = useState<Awaited<ReturnType<typeof api.previewDeveloperJSCommand>>["data"] | null>(null);
  const [docs, setDocs] = useState<DeveloperJSDocs | null>(null);
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({});
  const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);

  const loadPresets = useCallback(async () => {
    try {
      const res = await api.listDeveloperJSPresets();
      if (res.success && res.data) {
        setServerPresets(res.data.presets);
      }
    } catch (err) {
      toast({ title: t("adminDeveloper.templatesLoadFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    }
  }, [t, toast]);

  const loadDocs = useCallback(async () => {
    try {
      const res = await api.getDeveloperJSDocs();
      if (res.success && res.data) {
        setDocs(res.data);
      }
    } catch {
      // 符号树为辅助功能，加载失败时静默降级，不打断编辑器使用。
    }
  }, []);

  const toggleGroup = useCallback((key: string) => {
    setExpandedGroups((prev) => ({
      ...prev,
      [key]: !prev[key],
    }));
  }, []);

  const isExpanded = useCallback((key: string, defaultVal = false) => {
    if (expandedGroups[key] !== undefined) {
      return expandedGroups[key];
    }
    return defaultVal;
  }, [expandedGroups]);

  const toggleSymbolDetails = useCallback((name: string) => {
    setSelectedSymbol((prev) => (prev === name ? null : name));
  }, []);

  const groupedNamespaces = useMemo(() => {
    if (!docs || !docs.namespaces) return {};
    const groups: Record<string, DeveloperJSDocEntry[]> = {};
    docs.namespaces.forEach((entry) => {
      const parts = entry.name.split(".");
      const ns = parts[0] || "other";
      if (!groups[ns]) {
        groups[ns] = [];
      }
      groups[ns].push(entry);
    });
    return groups;
  }, [docs]);

  useEffect(() => {
    void loadPresets();
  }, [loadPresets]);

  useEffect(() => {
    void loadDocs();
  }, [loadDocs]);

  const customTemplates = useMemo(() => serverPresets.map(presetToTemplate), [serverPresets]);
  const allTemplates = useMemo(() => [...builtInTemplates, ...customTemplates], [customTemplates]);
  const activeTemplate = allTemplates.find((item) => item.id === activeTemplateId);

  const applyTemplate = useCallback((template: DeveloperTemplate) => {
    setActiveTemplateId(template.id);
    setCode(template.code);
    setTemplateName(template.builtin ? "" : String(template.title));
    setTemplateDescription(template.builtin ? "" : String(template.description || ""));
    setResult(null);
  }, []);

  const insertSnippet = useCallback((snippet: string) => {
    const textarea = editorRef.current;
    if (!textarea) {
      setCode((current) => `${current}\n${snippet}`);
      return;
    }
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const next = `${code.slice(0, start)}${snippet}${code.slice(end)}`;
    setCode(next);
    requestAnimationFrame(() => {
      textarea.focus();
      const cursor = start + snippet.length;
      textarea.setSelectionRange(cursor, cursor);
    });
  }, [code]);

  const newBlankTemplate = useCallback(() => {
    setActiveTemplateId("blank");
    setCode("");
    setTemplateName("");
    setTemplateDescription("");
    setResult(null);
    requestAnimationFrame(() => editorRef.current?.focus());
  }, []);

  const saveAsTemplate = useCallback(async () => {
    const name = templateName.trim();
    if (!name) {
      toast({ title: t("adminDeveloper.templateNameRequired"), variant: "destructive" });
      return;
    }
    setSavingTemplate(true);
    try {
      const res = await api.createDeveloperJSPreset({ name, description: templateDescription.trim(), code });
      if (!res.success || !res.data) throw new Error(res.message || t("adminDeveloper.templateSaveFailed"));
      await loadPresets();
      setActiveTemplateId(`preset-${res.data.id}`);
      toast({ title: t("adminDeveloper.templateSaved"), variant: "success" });
    } catch (err) {
      toast({ title: t("adminDeveloper.templateSaveFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setSavingTemplate(false);
    }
  }, [code, loadPresets, t, templateDescription, templateName, toast]);

  const updateTemplate = useCallback(async () => {
    const target = customTemplates.find((item) => item.id === activeTemplateId && item.presetId);
    if (!target?.presetId) return;
    const name = templateName.trim();
    if (!name) {
      toast({ title: t("adminDeveloper.templateNameRequired"), variant: "destructive" });
      return;
    }
    setSavingTemplate(true);
    try {
      const res = await api.updateDeveloperJSPreset(target.presetId, { name, description: templateDescription.trim(), code });
      if (!res.success) throw new Error(res.message || t("adminDeveloper.templateSaveFailed"));
      await loadPresets();
      toast({ title: t("adminDeveloper.templateUpdated"), variant: "success" });
    } catch (err) {
      toast({ title: t("adminDeveloper.templateSaveFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setSavingTemplate(false);
    }
  }, [activeTemplateId, code, customTemplates, loadPresets, t, templateDescription, templateName, toast]);

  const deleteTemplate = useCallback(async () => {
    const target = customTemplates.find((item) => item.id === activeTemplateId && item.presetId);
    if (!target?.presetId) return;
    setSavingTemplate(true);
    try {
      const res = await api.deleteDeveloperJSPreset(target.presetId);
      if (!res.success) throw new Error(res.message || t("adminDeveloper.templateSaveFailed"));
      await loadPresets();
      applyTemplate(builtInTemplates[0]);
      toast({ title: t("adminDeveloper.templateDeleted"), variant: "success" });
    } catch (err) {
      toast({ title: t("adminDeveloper.templateSaveFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setSavingTemplate(false);
    }
  }, [activeTemplateId, applyTemplate, customTemplates, loadPresets, t, toast]);

  const copyCommandReply = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(code.trim());
      toast({ title: t("common.copied"), variant: "success" });
    } catch {
      toast({ title: t("common.copyFailed"), variant: "destructive" });
    }
  }, [code, t, toast]);

  const preview = async () => {
    setRunning(true);
    setResult(null);
    try {
      const res = await api.previewDeveloperJSCommand(code);
      if (res.success && res.data) {
        setResult(res.data);
        toast({ title: res.data.ok ? t("adminDeveloper.previewPassed") : t("adminDeveloper.previewBlocked"), variant: res.data.ok ? "success" : "destructive" });
      } else {
        toast({ title: t("adminDeveloper.previewFailed"), description: res.message, variant: "destructive" });
      }
    } catch (err) {
      toast({ title: t("adminDeveloper.previewFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <Code2 className="h-6 w-6" />
            {t("adminDeveloper.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("adminDeveloper.description")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Badge variant="outline">{t("adminDeveloper.authBadge")}</Badge>
          <Badge variant="warning">{t("adminDeveloper.sandboxBadge")}</Badge>
        </div>
      </div>

      <div className="rounded-xl border-2 border-red-600 bg-red-500/10 p-6 shadow-lg">
        <div className="flex items-start gap-4">
          <div className="rounded-xl bg-red-600 p-3 text-white">
            <AlertTriangle className="h-6 w-6 animate-pulse" />
          </div>
          <div className="space-y-2 min-w-0">
            <h2 className="text-xl font-extrabold tracking-tight text-red-600 dark:text-red-400">
              最高级别安全警示与管理员免责声明 / SYSTEM SECURITY WARNING & DISCLAIMER
            </h2>
            <p className="text-sm font-black text-foreground uppercase tracking-wide leading-relaxed">
              ⚠️ 开发者模式（动态代码沙箱）目前处于「已开启 (ACTIVE)」状态！
            </p>
            <p className="text-xs font-bold text-muted-foreground leading-relaxed">
              重要提示：在此状态下，任何已被绑定的「js:」或「js:preset:」自定义脚本将可直接在线上被触发并调用 Goja 执行引擎。
              <span className="font-extrabold text-red-500 underline mx-1">
                特此声明：对于在开发者模式启用期间由于自定义执行脚本带来的所有系统稳定性异常、数据回写失误、Telegram 命令泛滥、以及由此可能引发的网络与权限防线穿透风险，系统管理员（您自己）自愿承诺承担一切直接和间接的技术及法律全部风险后果！
              </span>
              如果这些指令无需继续调试，请务必立即通过在仪表盘兑换框输入「DEBUGMODE」并进行密码验证以全面将本模式关闭、阻断此类调用！
            </p>
          </div>
        </div>
      </div>

      <Alert className="border-amber-500/40 bg-amber-500/10">
        <AlertTriangle className="h-4 w-4" />
        <AlertTitle>{t("adminDeveloper.riskTitle")}</AlertTitle>
        <AlertDescription>{t("adminDeveloper.riskDescription")}</AlertDescription>
      </Alert>

      <div className="grid gap-4 xl:grid-cols-[300px_minmax(0,1fr)_380px]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <FileCode2 className="h-4 w-4" />
              {t("adminDeveloper.templatesTitle")}
            </CardTitle>
            <CardDescription>{t("adminDeveloper.templatesDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <Button type="button" variant="outline" size="sm" className="w-full" onClick={newBlankTemplate}>
              <Plus className="mr-2 h-4 w-4" />
              {t("adminDeveloper.newBlankPreset")}
            </Button>
            <div className="space-y-2">
              {allTemplates.map((template) => (
                <button
                  key={template.id}
                  type="button"
                  onClick={() => applyTemplate(template)}
                  className={`w-full rounded-lg border p-3 text-left transition-colors ${
                    activeTemplate?.id === template.id ? "border-primary bg-primary/10" : "hover:bg-muted/40"
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <p className="truncate text-sm font-medium">{templateText(template.title, t)}</p>
                    <Badge variant={template.builtin ? "outline" : "secondary"} className="text-[10px]">
                      {template.builtin ? t("adminDeveloper.builtin") : t("adminDeveloper.custom")}
                    </Badge>
                  </div>
                  <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{templateText(template.description, t)}</p>
                </button>
              ))}
            </div>

            <div className="space-y-2 rounded-lg border bg-muted/20 p-3">
              <p className="text-sm font-medium">{t("adminDeveloper.saveTemplateTitle")}</p>
              <div className="space-y-1">
                <label className="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                  {t("adminDeveloper.templateName")}
                  <span className="text-destructive">*</span>
                </label>
                <Input
                  value={templateName}
                  onChange={(event) => setTemplateName(event.target.value)}
                  placeholder={t("adminDeveloper.templateNamePlaceholder")}
                  inputSize="sm"
                  maxLength={80}
                  aria-invalid={!templateName.trim()}
                />
              </div>
              <Input value={templateDescription} onChange={(event) => setTemplateDescription(event.target.value)} placeholder={t("adminDeveloper.templateDescription")} inputSize="sm" maxLength={500} />
              <div className="grid gap-2">
                <Button size="sm" onClick={saveAsTemplate} disabled={savingTemplate || !templateName.trim()} className="min-h-9 whitespace-normal leading-tight">
                  {savingTemplate ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Plus className="mr-2 h-4 w-4" />}
                  {t("adminDeveloper.saveAsTemplate")}
                </Button>
                {activeTemplate?.presetId && (
                  <div className="grid grid-cols-2 gap-2">
                    <Button size="sm" variant="outline" onClick={updateTemplate} disabled={savingTemplate || !templateName.trim()}>
                      {savingTemplate ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
                      {t("common.save")}
                    </Button>
                    <Button size="sm" variant="destructive" onClick={deleteTemplate} disabled={savingTemplate}>
                      <Trash2 className="mr-2 h-4 w-4" />
                      {t("common.delete")}
                    </Button>
                  </div>
                )}
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("adminDeveloper.editorTitle")}</CardTitle>
            <CardDescription>{t("adminDeveloper.editorDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex justify-end">
              <Button variant="outline" size="sm" onClick={copyCommandReply} className="whitespace-normal leading-tight">
                <Copy className="mr-2 h-4 w-4" />
                {t("adminDeveloper.copyCode")}
              </Button>
            </div>
            <JsCodeEditor
              ref={editorRef}
              value={code}
              onChange={setCode}
              className="min-h-[440px]"
              spellCheck={false}
              ariaLabel={t("adminDeveloper.editorTitle")}
            />
            <Alert className="border-sky-500/40 bg-sky-500/10">
              <BookOpen className="h-4 w-4" />
              <AlertDescription className="flex flex-col gap-2 text-xs">
                <span>{t("adminDeveloper.bindCommandNotice")}</span>
                <Button asChild variant="outline" size="sm" className="w-fit">
                  <Link href="/admin/telegram/commands">
                    {t("adminDeveloper.openCommandManager")}
                  </Link>
                </Button>
              </AlertDescription>
            </Alert>
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => void preview()} disabled={running} className="min-h-10 whitespace-normal leading-tight">
                {running ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Play className="mr-2 h-4 w-4" />}
                {t("adminDeveloper.runPreview")}
              </Button>
              {snippetRows.map((snippet) => (
                <Button key={snippet.labelKey} type="button" variant="outline" size="sm" onClick={() => insertSnippet(snippet.code)}>
                  {t(snippet.labelKey)}
                </Button>
              ))}
            </div>
          </CardContent>
        </Card>

        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <BookOpen className="h-5 w-5" />
                {t("adminDeveloper.docsPageTitle")}
              </CardTitle>
              <CardDescription>{t("adminDeveloper.docsPageDescription")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="text-sm text-muted-foreground">{t("adminDeveloper.docsStandaloneNotice")}</p>
              <Button asChild className="w-full">
                <Link href="/admin/developer/js-docs">
                  <BookOpen className="mr-2 h-4 w-4" />
                  {t("adminDeveloper.openDocsPage")}
                </Link>
              </Button>
            </CardContent>
          </Card>

          {docs && (
            <Card>
              <CardHeader className="pb-3 border-b">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Code2 className="h-4 w-4 text-primary" />
                  {t("adminDeveloper.symbolTreeTitle")}
                </CardTitle>
                <CardDescription className="text-xs">
                  {t("adminDeveloper.symbolTreeDescription")}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4 text-sm max-h-[600px] overflow-y-auto pr-1 pt-4">
                {/* 1. Global bindings */}
                <div className="space-y-1">
                  <button
                    type="button"
                    onClick={() => toggleGroup("bindings")}
                    className="flex w-full items-center justify-between py-1 text-xs font-semibold text-muted-foreground hover:text-foreground"
                  >
                    <span className="flex items-center gap-1">
                      {isExpanded("bindings", true) ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
                      {t("adminDeveloper.groupBindings")}
                    </span>
                    <Badge variant="secondary" className="px-1 text-[10px]">
                      {docs.bindings?.length || 0}
                    </Badge>
                  </button>
                  {isExpanded("bindings", true) && docs.bindings && (
                    <div className="pl-3 space-y-1 border-l ml-1.5 border-muted">
                      {docs.bindings.map((item) => (
                        <div key={item.name} className="group flex flex-col rounded hover:bg-muted/30 p-1">
                          <div className="flex items-center justify-between gap-2">
                            <button
                              type="button"
                              onClick={() => toggleSymbolDetails(item.name)}
                              className="font-mono text-xs text-left hover:text-primary break-all font-semibold flex-1"
                            >
                              {item.name}
                            </button>
                            <div className="flex gap-1 opacity-85 group-hover:opacity-100">
                              <Button
                                size="icon"
                                variant="ghost"
                                className="h-5 w-5 rounded p-0 text-[10px]"
                                title={t("adminDeveloper.btnInsert")}
                                onClick={() => insertSnippet(item.name)}
                              >
                                <Plus className="h-3 w-3" />
                              </Button>
                            </div>
                          </div>
                          {selectedSymbol === item.name && (
                            <div className="mt-1.5 text-xs text-muted-foreground space-y-1 bg-muted/20 p-2 rounded">
                              <p className="text-foreground">{item.description}</p>
                              {item.fields && item.fields.length > 0 && (
                                <p className="break-all font-mono text-[10px]">
                                  <span className="font-semibold text-primary">{t("adminDeveloper.fieldsLabel")}</span>
                                  {item.fields.join(", ")}
                                </p>
                              )}
                              {item.example && (
                                <div className="mt-1 space-y-1">
                                  <p className="font-semibold text-[10px] text-primary">{t("adminDeveloper.exampleLabel")}</p>
                                  <pre className="font-mono text-[10px] bg-background border rounded px-1.5 py-1 whitespace-pre-wrap leading-normal break-all">
                                    {item.example}
                                  </pre>
                                  <Button
                                    size="sm"
                                    type="button"
                                    variant="outline"
                                    className="h-5 text-[10px] px-1.5 py-0 mt-1"
                                    onClick={() => insertSnippet(item.example || "")}
                                  >
                                    {t("adminDeveloper.btnExample")}
                                  </Button>
                                </div>
                              )}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* 2. Global functions */}
                <div className="space-y-1">
                  <button
                    type="button"
                    onClick={() => toggleGroup("functions")}
                    className="flex w-full items-center justify-between py-1 text-xs font-semibold text-muted-foreground hover:text-foreground"
                  >
                    <span className="flex items-center gap-1">
                      {isExpanded("functions", true) ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
                      {t("adminDeveloper.groupFunctions")}
                    </span>
                    <Badge variant="secondary" className="px-1 text-[10px]">
                      {docs.functions?.length || 0}
                    </Badge>
                  </button>
                  {isExpanded("functions", true) && docs.functions && (
                    <div className="pl-3 space-y-1 border-l ml-1.5 border-muted">
                      {docs.functions.map((item) => {
                        return (
                          <div key={item.name} className="group flex flex-col rounded hover:bg-muted/30 p-1">
                            <div className="flex items-center justify-between gap-2">
                              <button
                                type="button"
                                onClick={() => toggleSymbolDetails(item.name)}
                                className="font-mono text-xs text-left hover:text-primary break-all font-semibold flex-1"
                              >
                                {item.name}
                              </button>
                              <div className="flex gap-1 opacity-85 group-hover:opacity-100">
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  className="h-5 w-5 rounded p-0 text-[10px]"
                                  title={t("adminDeveloper.btnInsert")}
                                  onClick={() => insertSnippet(item.name)}
                                >
                                  <Plus className="h-3 w-3" />
                                </Button>
                              </div>
                            </div>
                            {selectedSymbol === item.name && (
                              <div className="mt-1.5 text-xs text-muted-foreground space-y-1 bg-muted/20 p-2 rounded">
                                <p className="text-foreground">{item.description}</p>
                                {item.returns && (
                                  <p className="font-mono text-[10px]">
                                    <span className="font-semibold text-primary">{t("adminDeveloper.returnsLabel")}</span>
                                    {item.returns}
                                  </p>
                                )}
                                {item.params && item.params.length > 0 && (
                                  <div className="space-y-0.5">
                                    <span className="font-semibold text-[10px] text-primary">{t("adminDeveloper.paramsLabel")}</span>
                                    <ul className="list-disc list-inside space-y-0.5 mt-0.5 font-mono text-[10px]">
                                      {item.params.map(p => (
                                        <li key={p.name} className="list-item">
                                          <span className="font-semibold text-foreground">{p.name}</span>
                                          {p.type && <span className="text-muted-foreground"> ({p.type})</span>}
                                          {p.required && <span className="text-destructive font-semibold"> *</span>}
                                        </li>
                                      ))}
                                    </ul>
                                  </div>
                                )}
                                {item.example && (
                                  <div className="mt-1 space-y-1">
                                    <p className="font-semibold text-[10px] text-primary">{t("adminDeveloper.exampleLabel")}</p>
                                    <pre className="font-mono text-[10px] bg-background border rounded px-1.5 py-1 whitespace-pre-wrap leading-normal break-all">
                                      {item.example}
                                    </pre>
                                    <Button
                                      size="sm"
                                      type="button"
                                      variant="outline"
                                      className="h-5 text-[10px] px-1.5 py-0 mt-1"
                                      onClick={() => insertSnippet(item.example || "")}
                                    >
                                      {t("adminDeveloper.btnExample")}
                                    </Button>
                                  </div>
                                )}
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>

                {/* 3. Grouped Namespaces */}
                <div className="space-y-1">
                  <button
                    type="button"
                    onClick={() => toggleGroup("namespaces")}
                    className="flex w-full items-center justify-between py-1 text-xs font-semibold text-muted-foreground hover:text-foreground"
                  >
                    <span className="flex items-center gap-1">
                      {isExpanded("namespaces", false) ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
                      {t("adminDeveloper.groupNamespaces")}
                    </span>
                    <Badge variant="secondary" className="px-1 text-[10px]">
                      {docs.namespaces?.length || 0}
                    </Badge>
                  </button>
                  {isExpanded("namespaces", false) && (
                    <div className="pl-3 space-y-2 border-l ml-1.5 border-muted">
                      {Object.entries(groupedNamespaces).map(([ns, entries]) => (
                        <div key={ns} className="space-y-1">
                          <button
                            type="button"
                            onClick={() => toggleGroup(`ns-${ns}`)}
                            className="flex w-full items-center justify-between py-0.5 text-[11px] font-bold text-foreground/80 hover:text-foreground"
                          >
                            <span className="flex items-center gap-1 font-mono">
                              {isExpanded(`ns-${ns}`, false) ? <ChevronDown className="h-2.5 w-2.5" /> : <ChevronRight className="h-2.5 w-2.5" />}
                              {ns}
                            </span>
                            <span className="text-[10px] opacity-60">({entries.length})</span>
                          </button>
                          {isExpanded(`ns-${ns}`, false) && (
                            <div className="pl-2 space-y-1 border-l border-muted/50 ml-1">
                              {entries.map((item) => (
                                <div key={item.name} className="group flex flex-col rounded hover:bg-muted/30 p-1">
                                  <div className="flex items-center justify-between gap-2">
                                    <button
                                      type="button"
                                      onClick={() => toggleSymbolDetails(item.name)}
                                      className="font-mono text-[11px] text-left hover:text-primary break-all font-medium flex-1"
                                    >
                                      {item.name}
                                    </button>
                                    <div className="flex gap-1 opacity-85 group-hover:opacity-100">
                                      <Button
                                        size="icon"
                                        variant="ghost"
                                        className="h-4 w-4 rounded p-0 text-[10px]"
                                        title={t("adminDeveloper.btnInsert")}
                                        onClick={() => insertSnippet(item.name)}
                                      >
                                        <Plus className="h-2.5 w-2.5" />
                                      </Button>
                                    </div>
                                  </div>
                                  {selectedSymbol === item.name && (
                                    <div className="mt-1 text-xs text-muted-foreground space-y-1 bg-muted/20 p-2 rounded">
                                      <p className="text-foreground">{item.description}</p>
                                      {item.returns && (
                                        <p className="font-mono text-[10px]">
                                          <span className="font-semibold text-primary">{t("adminDeveloper.returnsLabel")}</span>
                                          {item.returns}
                                        </p>
                                      )}
                                      {item.params && item.params.length > 0 && (
                                        <div className="space-y-0.5">
                                          <span className="font-semibold text-[10px] text-primary">{t("adminDeveloper.paramsLabel")}</span>
                                          <ul className="list-disc list-inside space-y-0.5 mt-0.5 font-mono text-[10px]">
                                            {item.params.map(p => (
                                              <li key={p.name} className="list-item">
                                                <span className="font-semibold text-foreground">{p.name}</span>
                                                {p.type && <span className="text-muted-foreground font-light"> ({p.type})</span>}
                                              </li>
                                            ))}
                                          </ul>
                                        </div>
                                      )}
                                      {item.example && (
                                        <div className="mt-1 space-y-1">
                                          <p className="font-semibold text-[10px] text-primary">{t("adminDeveloper.exampleLabel")}</p>
                                          <pre className="font-mono text-[10px] bg-background border rounded px-1.5 py-1 whitespace-pre-wrap leading-normal break-all">
                                            {item.example}
                                          </pre>
                                          <Button
                                            size="sm"
                                            type="button"
                                            variant="outline"
                                            className="h-5 text-[10px] px-1.5 py-0 mt-1"
                                            onClick={() => insertSnippet(item.example || "")}
                                          >
                                            {t("adminDeveloper.btnExample")}
                                          </Button>
                                        </div>
                                      )}
                                    </div>
                                  )}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* 4. Configuration and Environment keys */}
                <div className="space-y-1">
                  <button
                    type="button"
                    onClick={() => toggleGroup("configEnv")}
                    className="flex w-full items-center justify-between py-1 text-xs font-semibold text-muted-foreground hover:text-foreground"
                  >
                    <span className="flex items-center gap-1">
                      {isExpanded("configEnv", false) ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
                      {t("adminDeveloper.groupConfigEnv")}
                    </span>
                    <Badge variant="secondary" className="px-1 text-[10px]">
                      {(docs.config_keys?.length || 0) + (docs.env_keys?.length || 0)}
                    </Badge>
                  </button>
                  {isExpanded("configEnv", false) && (
                    <div className="pl-3 space-y-3 border-l ml-1.5 border-muted">
                      {docs.config_keys && docs.config_keys.length > 0 && (
                        <div className="space-y-1">
                          <p className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">{t("adminDeveloper.configKeysTitle")}</p>
                          <div className="flex flex-wrap gap-1">
                            {docs.config_keys.map((k) => (
                              <button
                                key={k}
                                type="button"
                                title={t("adminDeveloper.clickToInsertKey")}
                                onClick={() => insertSnippet(`config("${k}")`)}
                                className="font-mono text-[10px] px-1.5 py-0.5 rounded border bg-background hover:bg-muted hover:text-primary transition-colors text-left truncate max-w-full"
                              >
                                {k}
                              </button>
                            ))}
                          </div>
                        </div>
                      )}

                      {docs.env_keys && docs.env_keys.length > 0 && (
                        <div className="space-y-1">
                          <p className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">{t("adminDeveloper.envKeysTitle")}</p>
                          <div className="flex flex-wrap gap-1">
                            {docs.env_keys.map((k) => (
                              <button
                                key={k}
                                type="button"
                                title={t("adminDeveloper.clickToInsertKey")}
                                onClick={() => insertSnippet(`env("${k}")`)}
                                className="font-mono text-[10px] px-1.5 py-0.5 rounded border bg-background hover:bg-muted hover:text-primary transition-colors text-left truncate max-w-full border-muted-foreground/30"
                              >
                                {k}
                              </button>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          )}

          {result && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <ShieldCheck className="h-5 w-5" />
                  {t("adminDeveloper.resultTitle")}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <Badge variant={result.ok ? "success" : "destructive"}>{result.ok ? t("adminDeveloper.resultPassed") : t("adminDeveloper.resultBlocked")}</Badge>
                {result.errors?.length > 0 && (
                  <div>
                    <p className="mb-1 font-medium">{t("adminDeveloper.errors")}</p>
                    <ul className="list-inside list-disc text-destructive">
                      {result.errors.map((item) => <li key={item}>{item}</li>)}
                    </ul>
                  </div>
                )}
                {result.warnings?.length > 0 && (
                  <div>
                    <p className="mb-1 font-medium">{t("adminDeveloper.warnings")}</p>
                    <ul className="list-inside list-disc text-muted-foreground">
                      {result.warnings.map((item) => <li key={item}>{item}</li>)}
                    </ul>
                  </div>
                )}
                {result.output && (
                  <div>
                    <p className="mb-1 font-medium">{t("adminDeveloper.output")}</p>
                    <pre className="whitespace-pre-wrap rounded-md bg-muted p-2 text-xs">{result.output}</pre>
                  </div>
                )}
                {result.logs && result.logs.length > 0 && (
                  <div>
                    <p className="mb-1 font-medium">{t("adminDeveloper.logs")}</p>
                    <pre className="whitespace-pre-wrap rounded-md bg-muted p-2 text-xs">{result.logs.join("\n")}</pre>
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
