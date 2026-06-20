"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  BookOpen,
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import type { DeveloperJSPreset } from "@/lib/api-types";
import { useI18n, type MessageKey } from "@/lib/i18n";

type DeveloperTemplate = {
  id: string;
  presetId?: number;
  title: MessageKey | string;
  description: MessageKey | string;
  command: string;
  code: string;
  builtin?: boolean;
  updatedAt?: number;
};

type DocRow = {
  name: string;
  descriptionKey: MessageKey;
  example?: string;
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

const builtInTemplates: DeveloperTemplate[] = [
  {
    id: "hello",
    title: "adminDeveloper.exampleHello",
    description: "adminDeveloper.exampleHelloDesc",
    command: "/hello",
    code: helloTemplate,
    builtin: true,
  },
  {
    id: "stats",
    title: "adminDeveloper.exampleStats",
    description: "adminDeveloper.exampleStatsDesc",
    command: "/me",
    code: statsTemplate,
    builtin: true,
  },
  {
    id: "admin-guard",
    title: "adminDeveloper.exampleGuard",
    description: "adminDeveloper.exampleGuardDesc",
    command: "/admin_tool",
    code: adminGuardTemplate,
    builtin: true,
  },
  {
    id: "config",
    title: "adminDeveloper.exampleConfig",
    description: "adminDeveloper.exampleConfigDesc",
    command: "/site",
    code: configTemplate,
    builtin: true,
  },
  {
    id: "env",
    title: "adminDeveloper.exampleEnv",
    description: "adminDeveloper.exampleEnvDesc",
    command: "/runtime",
    code: envTemplate,
    builtin: true,
  },
  {
    id: "router",
    title: "adminDeveloper.exampleRouter",
    description: "adminDeveloper.exampleRouterDesc",
    command: "/tool",
    code: argsRouterTemplate,
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
] as const;

const bindingRows: DocRow[] = [
  { name: "ctx.private_chat", descriptionKey: "adminDeveloper.bindingCtxPrivate", example: "true" },
  { name: "ctx.command_time", descriptionKey: "adminDeveloper.bindingCtxTime", example: "Unix seconds" },
  { name: "args", descriptionKey: "adminDeveloper.bindingArgs", example: '["a", "b"]' },
  { name: "user.uid", descriptionKey: "adminDeveloper.bindingUserUid", example: "10001" },
  { name: "user.username", descriptionKey: "adminDeveloper.bindingUserName", example: "alice" },
  { name: "user.role", descriptionKey: "adminDeveloper.bindingUserRole", example: "0 admin, 1 user, 2 whitelist" },
  { name: "user.active", descriptionKey: "adminDeveloper.bindingUserActive", example: "true" },
  { name: "user.has_emby", descriptionKey: "adminDeveloper.bindingUserHasEmby", example: "true" },
];

const constantRows: DocRow[] = [
  { name: "constants.roles.admin", descriptionKey: "adminDeveloper.constantRoleAdmin", example: "0" },
  { name: "constants.roles.user", descriptionKey: "adminDeveloper.constantRoleUser", example: "1" },
  { name: "constants.roles.whitelist", descriptionKey: "adminDeveloper.constantRoleWhitelist", example: "2" },
  { name: "constants.limits.max_replies", descriptionKey: "adminDeveloper.constantMaxReplies", example: "4" },
  { name: "constants.limits.max_logs", descriptionKey: "adminDeveloper.constantMaxLogs", example: "8" },
];

const functionRows: DocRow[] = [
  { name: "reply(text)", descriptionKey: "adminDeveloper.functionReply", example: 'reply("hello")' },
  { name: "log(text)", descriptionKey: "adminDeveloper.functionLog", example: 'log("step reached")' },
  { name: "auth(role)", descriptionKey: "adminDeveloper.functionAuth", example: 'auth("admin")' },
  { name: "config(key)", descriptionKey: "adminDeveloper.functionConfig", example: 'config("invite.enabled")' },
  { name: "env(key)", descriptionKey: "adminDeveloper.functionEnv", example: 'env("TWILIGHT_HOST")' },
];

const configRows: DocRow[] = [
  { name: "app.name", descriptionKey: "adminDeveloper.configAppName" },
  { name: "app.version", descriptionKey: "adminDeveloper.configAppVersion" },
  { name: "telegram.enabled", descriptionKey: "adminDeveloper.configTelegramEnabled" },
  { name: "telegram.require_membership", descriptionKey: "adminDeveloper.configTelegramMembership" },
  { name: "telegram.ban_on_leave", descriptionKey: "adminDeveloper.configTelegramBan" },
  { name: "invite.enabled", descriptionKey: "adminDeveloper.configInviteEnabled" },
  { name: "invite.max_depth", descriptionKey: "adminDeveloper.configInviteDepth" },
  { name: "email.enabled", descriptionKey: "adminDeveloper.configEmailEnabled" },
  { name: "signin.enabled", descriptionKey: "adminDeveloper.configSigninEnabled" },
  { name: "ticket.enabled", descriptionKey: "adminDeveloper.configTicketEnabled" },
];

const envRows: DocRow[] = [
  { name: "TWILIGHT_APP_NAME", descriptionKey: "adminDeveloper.envAppName" },
  { name: "TWILIGHT_SERVER_NAME", descriptionKey: "adminDeveloper.envServerName" },
  { name: "TWILIGHT_HOST", descriptionKey: "adminDeveloper.envHost" },
  { name: "TWILIGHT_PORT", descriptionKey: "adminDeveloper.envPort" },
  { name: "TWILIGHT_DATABASE_DRIVER", descriptionKey: "adminDeveloper.envDatabaseDriver" },
  { name: "TWILIGHT_EMAIL_ENABLED", descriptionKey: "adminDeveloper.envEmailEnabled" },
  { name: "TWILIGHT_INVITE_ENABLED", descriptionKey: "adminDeveloper.envInviteEnabled" },
];

function templateText(value: MessageKey | string, t: (key: MessageKey) => string): string {
  return value.startsWith("adminDeveloper.") ? t(value as MessageKey) : value;
}

function commandReply(command: string, code: string): string {
  const normalized = command.trim().startsWith("/") ? command.trim() : `/${command.trim() || "custom"}`;
  return `${normalized} = js:${code.trim()}`;
}

function presetToTemplate(preset: DeveloperJSPreset): DeveloperTemplate {
  return {
    id: `preset-${preset.id}`,
    presetId: preset.id,
    title: preset.name,
    description: preset.description || "",
    command: "/custom",
    code: preset.code || "",
    updatedAt: preset.updated_at,
  };
}

function DocRows({ rows }: { rows: DocRow[] }) {
  const { t } = useI18n();
  return (
    <div className="space-y-2">
      {rows.map((row) => (
        <div key={row.name} className="rounded-md border bg-muted/20 p-2">
          <div className="flex flex-wrap items-center gap-2">
            <code className="font-mono text-xs">{row.name}</code>
            {row.example ? <Badge variant="outline" className="text-[10px]">{row.example}</Badge> : null}
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{t(row.descriptionKey)}</p>
        </div>
      ))}
    </div>
  );
}

export default function AdminDeveloperPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const editorRef = useRef<HTMLTextAreaElement | null>(null);
  const [code, setCode] = useState(helloTemplate);
  const [command, setCommand] = useState("/hello");
  const [running, setRunning] = useState(false);
  const [savingTemplate, setSavingTemplate] = useState(false);
  const [templateName, setTemplateName] = useState("");
  const [templateDescription, setTemplateDescription] = useState("");
  const [serverPresets, setServerPresets] = useState<DeveloperJSPreset[]>([]);
  const [activeTemplateId, setActiveTemplateId] = useState("hello");
  const [result, setResult] = useState<Awaited<ReturnType<typeof api.previewDeveloperJSCommand>>["data"] | null>(null);

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

  useEffect(() => {
    void loadPresets();
  }, [loadPresets]);

  const customTemplates = useMemo(() => serverPresets.map(presetToTemplate), [serverPresets]);
  const allTemplates = useMemo(() => [...builtInTemplates, ...customTemplates], [customTemplates]);
  const activeTemplate = allTemplates.find((item) => item.id === activeTemplateId);
  const commandPreview = useMemo(() => commandReply(command, code), [code, command]);

  const applyTemplate = useCallback((template: DeveloperTemplate) => {
    setActiveTemplateId(template.id);
    setCode(template.code);
    setCommand(template.command || "/custom");
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
    setCommand("/custom");
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
      await navigator.clipboard.writeText(commandPreview);
      toast({ title: t("common.copied"), variant: "success" });
    } catch {
      toast({ title: t("common.copyFailed"), variant: "destructive" });
    }
  }, [commandPreview, t, toast]);

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
                  <code className="mt-2 block truncate text-[11px] text-muted-foreground">{template.command}</code>
                </button>
              ))}
            </div>

            <div className="space-y-2 rounded-lg border bg-muted/20 p-3">
              <p className="text-sm font-medium">{t("adminDeveloper.saveTemplateTitle")}</p>
              <Input value={templateName} onChange={(event) => setTemplateName(event.target.value)} placeholder={t("adminDeveloper.templateName")} inputSize="sm" />
              <Input value={templateDescription} onChange={(event) => setTemplateDescription(event.target.value)} placeholder={t("adminDeveloper.templateDescription")} inputSize="sm" />
              <div className="grid gap-2">
                <Button size="sm" onClick={saveAsTemplate} disabled={savingTemplate} className="min-h-9 whitespace-normal leading-tight">
                  {savingTemplate ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Plus className="mr-2 h-4 w-4" />}
                  {t("adminDeveloper.saveAsTemplate")}
                </Button>
                {activeTemplate?.presetId && (
                  <div className="grid grid-cols-2 gap-2">
                    <Button size="sm" variant="outline" onClick={updateTemplate} disabled={savingTemplate}>
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
            <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_180px]">
              <Input value={command} onChange={(event) => setCommand(event.target.value)} placeholder="/hello" />
              <Button variant="outline" onClick={copyCommandReply} className="min-h-10 whitespace-normal leading-tight">
                <Copy className="mr-2 h-4 w-4" />
                {t("adminDeveloper.copyCommandReply")}
              </Button>
            </div>
            <Textarea
              ref={editorRef}
              value={code}
              onChange={(event) => setCode(event.target.value)}
              className="min-h-[440px] font-mono text-sm"
              spellCheck={false}
            />
            <div className="rounded-lg border bg-muted/20 p-3">
              <p className="mb-2 text-xs font-medium text-muted-foreground">{t("adminDeveloper.commandReplyPreview")}</p>
              <pre className="max-h-28 overflow-auto whitespace-pre-wrap break-words rounded-md bg-background p-2 text-xs">{commandPreview}</pre>
            </div>
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
                {t("adminDeveloper.docsTitle")}
              </CardTitle>
              <CardDescription>{t("adminDeveloper.docsDescription")}</CardDescription>
            </CardHeader>
            <CardContent>
              <Tabs defaultValue="bindings" className="space-y-3">
                <TabsList className="i18n-stable-tabs grid h-auto w-full grid-cols-3">
                  <TabsTrigger value="bindings">{t("adminDeveloper.bindingsTitle")}</TabsTrigger>
                  <TabsTrigger value="functions">{t("adminDeveloper.functionsTitle")}</TabsTrigger>
                  <TabsTrigger value="config">{t("adminDeveloper.configEnvTitle")}</TabsTrigger>
                </TabsList>
                <TabsContent value="bindings" className="space-y-3">
                  <DocRows rows={bindingRows} />
                  <DocRows rows={constantRows} />
                </TabsContent>
                <TabsContent value="functions">
                  <DocRows rows={functionRows} />
                </TabsContent>
                <TabsContent value="config" className="space-y-3">
                  <p className="text-xs text-muted-foreground">{t("adminDeveloper.configEnvNotice")}</p>
                  <DocRows rows={configRows} />
                  <DocRows rows={envRows} />
                </TabsContent>
              </Tabs>
            </CardContent>
          </Card>

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
