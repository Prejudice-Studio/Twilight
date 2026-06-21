"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { BookOpen, Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import type { DeveloperJSDocEntry, DeveloperJSDocs } from "@/lib/api-types";
import { useI18n, type Locale } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type LocalizedDocEntry = DeveloperJSDocEntry & {
  description: string;
};

interface DeveloperJSDocsPanelProps {
  className?: string;
  onInsertSnippet?: (code: string) => void;
}

const entryText: Record<Exclude<Locale, "en-US">, Record<string, string>> = {
  "zh-Hans": {
    "ctx.private_chat": "当前指令是否来自私聊。群聊里建议只做只读工具，写操作应额外做管理员判断。",
    "ctx.command_time": "指令进入沙箱时的 Unix 秒级时间戳，可用 time.formatUnix 格式化。",
    "ctx.preview": "是否来自后台预览。预览模式下写入类 API 只返回 dry_run，不会真实修改数据。",
    args: "指令参数数组，不包含指令本身。例如 /tool ping now 会得到 [\"ping\", \"now\"]。",
    user: "当前 Telegram 绑定的本地用户脱敏快照。不包含邮箱、Telegram ID、Emby ID、Token 或密码。",
    "constants.roles": "角色常量：admin=0、user=1、whitelist=2。",
    "constants.limits": "运行期收集限制，例如 reply/log 最大次数。",
    "reply(text)": "追加一段回复文本。最多收集 4 段，最终用换行合并发送。",
    "log(text)": "追加一条本次执行日志。最多收集 8 条，敏感内容会脱敏截断。",
    "auth(role)": "检查当前绑定用户角色。支持 admin、whitelist、user 或数字角色字符串。",
    "getUser(uid)": "按精确 UID 读取脱敏用户快照。读取他人需要当前 Telegram 绑定用户是管理员；普通用户只能读取自己。",
    "config(key)": "读取白名单内的非敏感配置。未允许或敏感键返回空字符串。",
    "env(key)": "读取白名单内的非敏感 TWILIGHT_* 环境变量。未允许或敏感键返回空字符串。",
    "users.current()": "返回当前 Telegram 绑定用户的脱敏快照。",
    "users.describe()": "users.current() 的可读别名。",
    "users.get(uid)": "getUser(uid) 的命名空间形式，按精确 UID 返回脱敏用户快照或 null。",
    "users.byUID(uid)": "users.get(uid) 的别名。",
    "users.hasRole(role)": "按 auth(role) 相同规则检查当前用户角色。",
    "users.requireActive()": "仅当当前 Telegram 已绑定本地用户且账号启用时返回 true。",
    "users.setLoginNotify(options)": "修改当前绑定用户的登录通知偏好，只接受 telegram/email 布尔字段。",
    "text.truncate(value, max)": "按最大字符数截断文本。",
    "text.joinLines(values)": "把数组连接为多行文本。",
    "text.escape(value)": "转义基础 HTML 敏感字符，适合纯文本输出。",
    "text.numberLines(values)": "把数组转换为 1. / 2. 格式的编号文本。",
    "arrays.first(values)": "返回数组第一项，没有则返回 undefined。",
    "arrays.compact(values)": "移除 null 和空字符串。",
    "arrays.unique(values)": "按首次出现顺序去重字符串数组。",
    "arrays.take(values, count)": "截取数组前 count 项。",
    "time.now()": "返回当前 Unix 秒级时间戳。",
    "time.formatUnix(ts)": "把 Unix 秒级时间戳格式化为 UTC RFC3339 文本。",
    "interactions.inline(text, actions)": "发送静态 inline keyboard。按钮只执行预定义 answer/edit/reply，不会再次运行 JS。",
    "interactions.waitText(options)": "等待同一用户在 1-60 秒内发送下一条非命令文本，并按限制截断、编号或回复。",
    Object: "Goja 提供的原生 JavaScript Object。",
    Array: "Goja 提供的原生 JavaScript Array。常见输出处理优先使用 arrays.*。",
    JSON: "原生 JSON parse/stringify 支持。",
    Math: "原生 Math 工具。",
    Date: "原生 Date 支持。命令输出建议优先使用 time.now/time.formatUnix。",
    "String / Number / Boolean": "原生基础类型包装与原型方法。",
  },
  "zh-Hant": {
    "ctx.private_chat": "目前指令是否來自私聊。群聊中建議只做唯讀工具，寫操作應額外做管理員判斷。",
    "ctx.command_time": "指令進入沙箱時的 Unix 秒級時間戳，可用 time.formatUnix 格式化。",
    "ctx.preview": "是否來自後台預覽。預覽模式下寫入類 API 只返回 dry_run，不會真實修改資料。",
    args: "指令參數陣列，不包含指令本身。例如 /tool ping now 會得到 [\"ping\", \"now\"]。",
    user: "目前 Telegram 綁定的本地使用者脫敏快照。不包含信箱、Telegram ID、Emby ID、Token 或密碼。",
    "constants.roles": "角色常數：admin=0、user=1、whitelist=2。",
    "constants.limits": "執行期收集限制，例如 reply/log 最大次數。",
    "reply(text)": "追加一段回覆文字。最多收集 4 段，最終用換行合併傳送。",
    "log(text)": "追加一條本次執行日誌。最多收集 8 條，敏感內容會脫敏截斷。",
    "auth(role)": "檢查目前綁定使用者角色。支援 admin、whitelist、user 或數字角色字串。",
    "getUser(uid)": "按精確 UID 讀取脫敏使用者快照。讀取他人需要目前 Telegram 綁定使用者是管理員；普通使用者只能讀取自己。",
    "config(key)": "讀取白名單內的非敏感配置。未允許或敏感鍵返回空字串。",
    "env(key)": "讀取白名單內的非敏感 TWILIGHT_* 環境變數。未允許或敏感鍵返回空字串。",
    "users.current()": "返回目前 Telegram 綁定使用者的脫敏快照。",
    "users.describe()": "users.current() 的可讀別名。",
    "users.get(uid)": "getUser(uid) 的命名空間形式，按精確 UID 返回脫敏使用者快照或 null。",
    "users.byUID(uid)": "users.get(uid) 的別名。",
    "users.hasRole(role)": "按 auth(role) 相同規則檢查目前使用者角色。",
    "users.requireActive()": "僅當目前 Telegram 已綁定本地使用者且帳號啟用時返回 true。",
    "users.setLoginNotify(options)": "修改目前綁定使用者的登入通知偏好，只接受 telegram/email 布林欄位。",
    "text.truncate(value, max)": "按最大字元數截斷文字。",
    "text.joinLines(values)": "把陣列連接為多行文字。",
    "text.escape(value)": "轉義基礎 HTML 敏感字元，適合純文字輸出。",
    "text.numberLines(values)": "把陣列轉換為 1. / 2. 格式的編號文字。",
    "arrays.first(values)": "返回陣列第一項，沒有則返回 undefined。",
    "arrays.compact(values)": "移除 null 和空字串。",
    "arrays.unique(values)": "按首次出現順序去重字串陣列。",
    "arrays.take(values, count)": "截取陣列前 count 項。",
    "time.now()": "返回目前 Unix 秒級時間戳。",
    "time.formatUnix(ts)": "把 Unix 秒級時間戳格式化為 UTC RFC3339 文字。",
    "interactions.inline(text, actions)": "傳送靜態 inline keyboard。按鈕只執行預定義 answer/edit/reply，不會再次執行 JS。",
    "interactions.waitText(options)": "等待同一使用者在 1-60 秒內傳送下一條非命令文字，並按限制截斷、編號或回覆。",
    Object: "Goja 提供的原生 JavaScript Object。",
    Array: "Goja 提供的原生 JavaScript Array。常見輸出處理優先使用 arrays.*。",
    JSON: "原生 JSON parse/stringify 支援。",
    Math: "原生 Math 工具。",
    Date: "原生 Date 支援。命令輸出建議優先使用 time.now/time.formatUnix。",
    "String / Number / Boolean": "原生基礎型別包裝與原型方法。",
  },
};

const engineText: Record<Exclude<Locale, "en-US">, { description: string; language: string; sandbox: string[] }> = {
  "zh-Hans": {
    description: "Telegram js: 自定义指令使用的进程内 Go JavaScript 引擎。",
    language: "偏 ECMAScript 5.1 的同步 JavaScript，支持 Goja 提供的扩展；建议编写普通同步脚本。",
    sandbox: [
      "不会注入网络、文件系统、进程、计时器、模块加载器、浏览器全局对象或任意环境访问能力。",
      "配置和环境变量访问均为显式只读白名单；敏感键返回空字符串。",
      "沙箱预览中，写入类用户接口和 Telegram 交互接口均为 dry-run。",
    ],
  },
  "zh-Hant": {
    description: "Telegram js: 自訂指令使用的進程內 Go JavaScript 引擎。",
    language: "偏 ECMAScript 5.1 的同步 JavaScript，支援 Goja 提供的擴充；建議編寫普通同步腳本。",
    sandbox: [
      "不會注入網路、檔案系統、進程、計時器、模組載入器、瀏覽器全域物件或任意環境存取能力。",
      "配置和環境變數存取均為明確唯讀白名單；敏感鍵返回空字串。",
      "沙箱預覽中，寫入類使用者介面和 Telegram 互動介面均為 dry-run。",
    ],
  },
};

const exampleText: Record<Exclude<Locale, "en-US">, Record<string, { title: string; description: string }>> = {
  "zh-Hans": {
    "command-context": { title: "指令输入上下文", description: "展示 Telegram 用户触发指令时可读取的全部非敏感值。" },
    "current-user": { title: "当前用户摘要", description: "返回当前 Telegram 绑定 Twilight 用户的脱敏摘要。" },
    "admin-get-user": { title: "管理员按 UID 查用户", description: "按精确 UID 读取脱敏用户快照；跨用户查询要求管理员角色。" },
    "login-notify": { title: "切换登录通知", description: "为当前绑定用户启用 Telegram 登录通知。沙箱预览只返回 dry_run，不写入状态。" },
    "array-tools": { title: "数组与文本工具", description: "在回复前清理和规范化指令参数。" },
    "inline-actions": { title: "Inline 操作消息", description: "发送短 inline keyboard，callback 使用预定义 answer/edit/reply 文本。" },
    "wait-text": { title: "等待下一条文本", description: "等待同一 Telegram 用户在限定时间内发送一条普通文本。" },
  },
  "zh-Hant": {
    "command-context": { title: "指令輸入上下文", description: "展示 Telegram 使用者觸發指令時可讀取的全部非敏感值。" },
    "current-user": { title: "目前使用者摘要", description: "返回目前 Telegram 綁定 Twilight 使用者的脫敏摘要。" },
    "admin-get-user": { title: "管理員按 UID 查使用者", description: "按精確 UID 讀取脫敏使用者快照；跨使用者查詢要求管理員角色。" },
    "login-notify": { title: "切換登入通知", description: "為目前綁定使用者啟用 Telegram 登入通知。沙箱預覽只返回 dry_run，不寫入狀態。" },
    "array-tools": { title: "陣列與文字工具", description: "在回覆前清理和規範化指令參數。" },
    "inline-actions": { title: "Inline 操作訊息", description: "傳送短 inline keyboard，callback 使用預定義 answer/edit/reply 文字。" },
    "wait-text": { title: "等待下一條文字", description: "等待同一 Telegram 使用者在限定時間內傳送一條普通文字。" },
  },
};

const extraEntryText: Record<Exclude<Locale, "en-US">, Record<string, string>> = {
  "zh-Hans": {
    "ctx.command": "标准化后的指令名称，例如 /hello。",
    command: "指令触发时自动初始化的对象，包含指令名、参数、参数文本、是否私聊、是否预览等非敏感信息。",
    "authAdmin()": "管理员快捷鉴权函数，当前 Telegram 绑定用户是管理员时返回 true。",
    "fetch(url, options)": "高风险同步兼容函数。仅允许受限 HTTP(S) 请求，阻断本机、内网、链路本地目标，不发送凭据并限制响应长度。",
    "setTimeout(fn, ms)": "兼容包装器，会在当前执行窗口内同步执行回调，不创建异步任务。",
    "setInterval(fn, ms)": "兼容包装器，会同步执行一次回调，不创建重复异步任务。",
    "db.schema()": "返回受控数据库集合结构和允许字段，不暴露原始 state。",
    "db.collections()": "返回 JS 沙箱允许查看的受控集合名称。",
    "db.count(name)": "返回允许的集合计数；管理员专属集合对非管理员返回 -1。",
    "db.currentUser()": "返回与 users.current() 相同的当前用户脱敏快照。",
    "db.getUser(uid)": "按精确 UID 查询脱敏用户快照，权限规则与 getUser(uid) 相同。",
    "db.updateCurrentUser(patch)": "仅允许修改当前绑定用户的登录通知偏好；预览模式返回 dry_run。",
    "Function / eval": "Goja 兼容能力，风险较高，仅建议在管理员审核后的预设中使用。",
    globalThis: "绑定到 Goja 全局对象；不会提供浏览器或 Node.js 全局对象。",
  },
  "zh-Hant": {
    "ctx.command": "標準化後的指令名稱，例如 /hello。",
    command: "指令觸發時自動初始化的物件，包含指令名、參數、參數文字、是否私聊、是否預覽等非敏感資訊。",
    "authAdmin()": "管理員快捷鑑權函式，當前 Telegram 綁定使用者是管理員時返回 true。",
    "fetch(url, options)": "高風險同步相容函式。僅允許受限 HTTP(S) 請求，阻斷本機、內網、鏈路本地目標，不傳送憑據並限制回應長度。",
    "setTimeout(fn, ms)": "相容包裝器，會在當前執行視窗內同步執行回呼，不建立非同步任務。",
    "setInterval(fn, ms)": "相容包裝器，會同步執行一次回呼，不建立重複非同步任務。",
    "db.schema()": "返回受控資料庫集合結構和允許欄位，不暴露原始 state。",
    "db.collections()": "返回 JS 沙箱允許查看的受控集合名稱。",
    "db.count(name)": "返回允許的集合計數；管理員專屬集合對非管理員返回 -1。",
    "db.currentUser()": "返回與 users.current() 相同的當前使用者脫敏快照。",
    "db.getUser(uid)": "按精確 UID 查詢脫敏使用者快照，權限規則與 getUser(uid) 相同。",
    "db.updateCurrentUser(patch)": "僅允許修改當前綁定使用者的登入通知偏好；預覽模式返回 dry_run。",
    "Function / eval": "Goja 相容能力，風險較高，僅建議在管理員審核後的預設中使用。",
    globalThis: "綁定到 Goja 全域物件；不會提供瀏覽器或 Node.js 全域物件。",
  },
};

const extraExampleText: Record<Exclude<Locale, "en-US">, Record<string, { title: string; description: string }>> = {
  "zh-Hans": {
    "db-summary": { title: "受控数据库摘要", description: "使用受控数据库函数查看安全结构元数据和允许的计数。" },
    "db-update-current-user": { title: "受控当前用户写入", description: "仅更新当前绑定用户允许的通知字段；预览模式返回 dry_run。" },
    "risk-fetch": { title: "高风险兼容 fetch", description: "fetch 为同步受限能力，会阻断内网目标，仅建议用于审核后的管理员预设。" },
  },
  "zh-Hant": {
    "db-summary": { title: "受控資料庫摘要", description: "使用受控資料庫函式查看安全結構中繼資料和允許的計數。" },
    "db-update-current-user": { title: "受控當前使用者寫入", description: "僅更新當前綁定使用者允許的通知欄位；預覽模式返回 dry_run。" },
    "risk-fetch": { title: "高風險相容 fetch", description: "fetch 為同步受限能力，會阻斷內網目標，僅建議用於審核後的管理員預設。" },
  },
};

function localizeEntry(locale: Locale, row: DeveloperJSDocEntry): LocalizedDocEntry {
  if (locale === "en-US") return row;
  return { ...row, description: entryText[locale][row.name] || extraEntryText[locale][row.name] || row.description };
}

function rowsFromKeys(keys: string[], category: string): LocalizedDocEntry[] {
  return keys.map((name) => ({ name, category, type: "string", description: "" }));
}

function DocRows({ rows }: { rows: LocalizedDocEntry[] }) {
  const { t } = useI18n();
  return (
    <div className="grid gap-2">
      {rows.map((row) => (
        <div key={`${row.category}-${row.name}`} className="rounded-md border bg-muted/20 p-3">
          <div className="flex flex-wrap items-center gap-2">
            <code className="break-all font-mono text-xs">{row.name}</code>
            {row.type ? <Badge variant="secondary" className="text-[10px]">{row.type}</Badge> : null}
            {row.mutates ? <Badge variant="warning" className="text-[10px]">{t("adminDeveloper.mutatesBadge")}</Badge> : null}
            {row.scope ? <Badge variant="outline" className="text-[10px]">{row.scope}</Badge> : null}
          </div>
          {row.description ? <p className="mt-2 text-xs leading-relaxed text-muted-foreground">{row.description}</p> : null}
          {row.fields && row.fields.length > 0 ? (
            <p className="mt-2 break-words text-[11px] text-muted-foreground">
              {t("adminDeveloper.fieldsLabel")}: {row.fields.join(", ")}
            </p>
          ) : null}
          {row.example ? <pre className="mt-2 overflow-auto rounded-md bg-background p-2 text-[11px]">{row.example}</pre> : null}
        </div>
      ))}
    </div>
  );
}

export function DeveloperJSDocsPanel({ className, onInsertSnippet }: DeveloperJSDocsPanelProps) {
  const { t, locale } = useI18n();
  const { toast } = useToast();
  const [docs, setDocs] = useState<DeveloperJSDocs | null>(null);
  const [loading, setLoading] = useState(true);

  const loadDocs = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.getDeveloperJSDocs();
      if (res.success && res.data) {
        setDocs(res.data);
      }
    } catch (err) {
      toast({ title: t("adminDeveloper.docsLoadFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setLoading(false);
    }
  }, [t, toast]);

  useEffect(() => {
    void loadDocs();
  }, [loadDocs]);

  const view = useMemo(() => {
    if (!docs) return null;
    const localizedEngine = locale === "en-US" ? docs.engine : { ...docs.engine, ...engineText[locale] };
    const localizeRows = (rows: DeveloperJSDocEntry[]) => rows.map((row) => localizeEntry(locale, row));
    return {
      engine: localizedEngine,
      bindings: localizeRows(docs.bindings),
      functions: localizeRows(docs.functions),
      namespaces: localizeRows(docs.namespaces),
      nativeObjects: localizeRows(docs.native_objects),
      configKeys: rowsFromKeys(docs.config_keys, "config"),
      envKeys: rowsFromKeys(docs.env_keys, "env"),
      examples: docs.examples.map((example) => {
        if (locale === "en-US") return example;
        return { ...example, ...(exampleText[locale][example.id] || extraExampleText[locale][example.id] || {}) };
      }),
      blockedTokens: docs.blocked_tokens,
      riskTokens: docs.risk_tokens || [],
    };
  }, [docs, locale]);

  return (
    <Card className={cn(className)}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BookOpen className="h-5 w-5" />
          {t("adminDeveloper.docsTitle")}
        </CardTitle>
        <CardDescription>{t("adminDeveloper.docsDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        {loading && !view ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t("adminDeveloper.loadingDocs")}
          </div>
        ) : view ? (
          <Tabs defaultValue="engine" className="space-y-4">
            <TabsList className="i18n-stable-tabs grid h-auto w-full grid-cols-2 lg:grid-cols-4">
              <TabsTrigger value="engine">{t("adminDeveloper.engineTitle")}</TabsTrigger>
              <TabsTrigger value="bindings">{t("adminDeveloper.bindingsTitle")}</TabsTrigger>
              <TabsTrigger value="functions">{t("adminDeveloper.functionsTitle")}</TabsTrigger>
              <TabsTrigger value="config">{t("adminDeveloper.configEnvTitle")}</TabsTrigger>
            </TabsList>
            <TabsContent value="engine" className="space-y-3">
              <div className="rounded-md border bg-muted/20 p-4">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="secondary">{view.engine.name}</Badge>
                  <Badge variant="outline">{view.engine.timeout_ms}ms</Badge>
                </div>
                <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{view.engine.description}</p>
                <code className="mt-2 block break-words text-[11px] text-muted-foreground">
                  {view.engine.module}@{view.engine.version}
                </code>
                <p className="mt-2 text-xs text-muted-foreground">{view.engine.language}</p>
                <ul className="mt-3 list-inside list-disc space-y-1 text-xs text-muted-foreground">
                  {view.engine.sandbox.map((item) => <li key={item}>{item}</li>)}
                </ul>
              </div>
              <DocRows rows={view.nativeObjects} />
              <div className="rounded-md border bg-muted/20 p-3">
                <p className="mb-2 text-xs font-medium">{t("adminDeveloper.blockedTokensTitle")}</p>
                <div className="flex flex-wrap gap-1">
                  {view.blockedTokens.map((token) => <Badge key={token} variant="outline" className="text-[10px]">{token}</Badge>)}
                </div>
              </div>
              {view.riskTokens.length > 0 ? (
                <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3">
                  <p className="mb-2 text-xs font-medium">{t("adminDeveloper.riskTokensTitle")}</p>
                  <div className="flex flex-wrap gap-1">
                    {view.riskTokens.map((token) => <Badge key={token} variant="warning" className="text-[10px]">{token}</Badge>)}
                  </div>
                </div>
              ) : null}
              <div className="rounded-md border bg-muted/20 p-3">
                <p className="mb-2 text-xs font-medium">{t("adminDeveloper.examplesApiTitle")}</p>
                <div className="grid gap-3 lg:grid-cols-2">
                  {view.examples.map((example) => (
                    <div key={example.id} className="rounded-md border bg-background p-3">
                      <p className="text-sm font-medium">{example.title}</p>
                      <p className="mt-1 text-xs text-muted-foreground">{example.description}</p>
                      <pre className="mt-2 max-h-44 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-2 text-[11px]">{example.code}</pre>
                      {onInsertSnippet ? (
                        <Button type="button" variant="outline" size="sm" className="mt-2" onClick={() => onInsertSnippet(`\n${example.code}\n`)}>
                          {t("adminDeveloper.exampleApply")}
                        </Button>
                      ) : null}
                    </div>
                  ))}
                </div>
              </div>
            </TabsContent>
            <TabsContent value="bindings" className="space-y-3">
              <DocRows rows={view.bindings} />
            </TabsContent>
            <TabsContent value="functions" className="space-y-3">
              <DocRows rows={view.functions} />
              <p className="text-xs font-medium text-muted-foreground">{t("adminDeveloper.namespacesTitle")}</p>
              <DocRows rows={view.namespaces} />
            </TabsContent>
            <TabsContent value="config" className="space-y-3">
              <p className="text-xs text-muted-foreground">{t("adminDeveloper.configEnvNotice")}</p>
              <p className="text-xs font-medium text-muted-foreground">{t("adminDeveloper.configKeysTitle")}</p>
              <DocRows rows={view.configKeys} />
              <p className="text-xs font-medium text-muted-foreground">{t("adminDeveloper.envKeysTitle")}</p>
              <DocRows rows={view.envKeys} />
            </TabsContent>
          </Tabs>
        ) : (
          <p className="text-sm text-muted-foreground">{t("adminDeveloper.docsDescription")}</p>
        )}
      </CardContent>
    </Card>
  );
}
