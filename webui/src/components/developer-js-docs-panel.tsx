"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { BookOpen, Code2, Database, Loader2, Search, ShieldCheck, TerminalSquare } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { api } from "@/lib/api";
import type { DeveloperJSDocEntry, DeveloperJSDocs } from "@/lib/api-types";
import { useI18n, type Locale } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type TextLocale = "zh-Hans" | "zh-Hant" | "en-US";
type DocParam = NonNullable<DeveloperJSDocEntry["params"]>[number];

interface DeveloperJSDocsPanelProps {
  className?: string;
  onInsertSnippet?: (code: string) => void;
}

type Labels = {
  title: string;
  description: string;
  loading: string;
  loadFailed: string;
  searchPlaceholder: string;
  overviewTab: string;
  contextTab: string;
  functionsTab: string;
  typesTab: string;
  configTab: string;
  examplesTab: string;
  engineTitle: string;
  securityTitle: string;
  securityIntro: string;
  runtimeTitle: string;
  runtimeItems: string[];
  lifecycleTitle: string;
  lifecycleItems: string[];
  functionIntro: string;
  typeIntro: string;
  configIntro: string;
  examplesIntro: string;
  fields: string;
  params: string;
  returns: string;
  required: string;
  optional: string;
  defaultValue: string;
  example: string;
  insert: string;
  mutates: string;
  scope: string;
  blockedTokens: string;
  riskTokens: string;
  noMatches: string;
  configKeys: string;
  envKeys: string;
  bindings: string;
  nativeObjects: string;
  namespaces: string;
  allCategories: string;
};

const labels: Record<TextLocale, Labels> = {
  "zh-Hans": {
    title: "Telegram Bot JS 接口文档",
    description: "面向管理员的完整沙箱参考。此页按当前语言展示运行模型、可用对象、函数参数、返回结构、类型定义、安全边界和示例。",
    loading: "正在加载 JS 接口文档...",
    loadFailed: "加载 JS 接口文档失败",
    searchPlaceholder: "搜索函数、对象、字段、类型或说明...",
    overviewTab: "概览",
    contextTab: "上下文",
    functionsTab: "函数",
    typesTab: "类型",
    configTab: "配置 / 环境",
    examplesTab: "示例",
    engineTitle: "运行引擎",
    securityTitle: "安全边界",
    securityIntro: "开发者模式只放开受控能力。脚本不能访问文件系统、进程、模块加载器、原始数据库、敏感配置、Token、密码或 API Key。",
    runtimeTitle: "执行模型",
    runtimeItems: [
      "脚本由 Goja 执行，并包裹在函数作用域内，因此顶层 return 可以正常结束当前命令。",
      "单次执行是同步模型，默认 8 秒超时；setTimeout/setInterval 只是兼容包装器，会同步运行回调，不创建后台任务。",
      "reply(text) 最多收集 4 段回复；log(text) 最多收集 8 条审计安全日志。",
      "exit(message?) 和 assert(condition, message?) 是正常控制流，不会被记为沙箱错误。",
      "预览模式下 ctx.preview=true，写入类接口只返回 dry_run，不会修改用户数据或发送真实交互。",
    ],
    lifecycleTitle: "推荐调用流程",
    lifecycleItems: [
      "先用 input.* 解析参数，再用 assert()/exit() 做参数和权限守卫。",
      "读取用户数据优先使用 users.current()；跨用户读取或写入必须使用管理员权限。",
      "所有管理员写操作都会走受控字段、last-admin 保护和审计日志。",
      "生产 Bot 指令建议保存为 js:preset:<id>，更新预设后可自动使用最新代码。",
    ],
    functionIntro: "函数按命名空间分组展示。写入类函数会带有“写入”标记；管理员专用函数会显示作用域。",
    typeIntro: "这些类型不是 TypeScript 声明文件，而是沙箱运行时对象结构。字段均为已脱敏快照或受控返回值。",
    configIntro: "config(key) 和 env(key) 都是只读白名单。敏感配置、数据库连接、线路、Token、Secret、密码和 API Key 不会返回。",
    examplesIntro: "示例来自后端结构化文档。中文界面中的说明已本地化；代码可直接复制到开发者模式编辑器或 Bot 指令预设中调整。",
    fields: "字段",
    params: "参数",
    returns: "返回",
    required: "必填",
    optional: "可选",
    defaultValue: "默认",
    example: "示例",
    insert: "载入到编辑器",
    mutates: "写入",
    scope: "作用域",
    blockedTokens: "静态阻止标记",
    riskTokens: "允许但高风险标记",
    noMatches: "没有匹配的条目",
    configKeys: "配置键",
    envKeys: "环境变量键",
    bindings: "自动注入对象",
    nativeObjects: "原生 JS 对象",
    namespaces: "命名空间",
    allCategories: "全部分类",
  },
  "zh-Hant": {
    title: "Telegram Bot JS 介面文件",
    description: "面向管理員的完整沙箱參考。此頁會依目前語言展示執行模型、可用物件、函式參數、返回結構、型別定義、安全邊界和範例。",
    loading: "正在載入 JS 介面文件...",
    loadFailed: "載入 JS 介面文件失敗",
    searchPlaceholder: "搜尋函式、物件、欄位、型別或說明...",
    overviewTab: "概覽",
    contextTab: "上下文",
    functionsTab: "函式",
    typesTab: "型別",
    configTab: "設定 / 環境",
    examplesTab: "範例",
    engineTitle: "執行引擎",
    securityTitle: "安全邊界",
    securityIntro: "開發者模式只開放受控能力。腳本不能存取檔案系統、程序、模組載入器、原始資料庫、敏感設定、Token、密碼或 API Key。",
    runtimeTitle: "執行模型",
    runtimeItems: [
      "腳本由 Goja 執行，並包裹在函式作用域內，因此頂層 return 可以正常結束目前命令。",
      "單次執行是同步模型，預設 8 秒逾時；setTimeout/setInterval 只是相容包裝器，會同步執行回呼，不建立背景任務。",
      "reply(text) 最多收集 4 段回覆；log(text) 最多收集 8 條稽核安全日誌。",
      "exit(message?) 和 assert(condition, message?) 是正常控制流程，不會被記為沙箱錯誤。",
      "預覽模式下 ctx.preview=true，寫入類介面只返回 dry_run，不會修改使用者資料或傳送真實互動。",
    ],
    lifecycleTitle: "建議呼叫流程",
    lifecycleItems: [
      "先用 input.* 解析參數，再用 assert()/exit() 做參數和權限守衛。",
      "讀取使用者資料優先使用 users.current()；跨使用者讀取或寫入必須使用管理員權限。",
      "所有管理員寫操作都會走受控欄位、last-admin 保護和稽核日誌。",
      "生產 Bot 指令建議保存為 js:preset:<id>，更新預設後可自動使用最新程式碼。",
    ],
    functionIntro: "函式按命名空間分組展示。寫入類函式會帶有「寫入」標記；管理員專用函式會顯示作用域。",
    typeIntro: "這些型別不是 TypeScript 宣告檔，而是沙箱執行時物件結構。欄位均為已脫敏快照或受控返回值。",
    configIntro: "config(key) 和 env(key) 都是唯讀白名單。敏感設定、資料庫連線、線路、Token、Secret、密碼和 API Key 不會返回。",
    examplesIntro: "範例來自後端結構化文件。繁中介面中的說明已本地化；程式碼可直接複製到開發者模式編輯器或 Bot 指令預設中調整。",
    fields: "欄位",
    params: "參數",
    returns: "返回",
    required: "必填",
    optional: "可選",
    defaultValue: "預設",
    example: "範例",
    insert: "載入到編輯器",
    mutates: "寫入",
    scope: "作用域",
    blockedTokens: "靜態阻止標記",
    riskTokens: "允許但高風險標記",
    noMatches: "沒有符合的條目",
    configKeys: "設定鍵",
    envKeys: "環境變數鍵",
    bindings: "自動注入物件",
    nativeObjects: "原生 JS 物件",
    namespaces: "命名空間",
    allCategories: "全部分類",
  },
  "en-US": {
    title: "Telegram Bot JS API Reference",
    description: "Complete sandbox reference for administrators. This page documents runtime behavior, injected objects, function parameters, return shapes, types, security boundaries, and examples.",
    loading: "Loading JS API documentation...",
    loadFailed: "Failed to load JS API documentation",
    searchPlaceholder: "Search functions, objects, fields, types, or descriptions...",
    overviewTab: "Overview",
    contextTab: "Context",
    functionsTab: "Functions",
    typesTab: "Types",
    configTab: "Config / Env",
    examplesTab: "Examples",
    engineTitle: "Runtime Engine",
    securityTitle: "Security Boundary",
    securityIntro: "Developer mode exposes only controlled capabilities. Scripts cannot access the filesystem, processes, module loaders, raw database state, sensitive config, tokens, passwords, or API keys.",
    runtimeTitle: "Execution Model",
    runtimeItems: [
      "Scripts run in Goja and are wrapped in a function scope, so top-level return can stop the command normally.",
      "Execution is synchronous with an 8s default timeout; setTimeout/setInterval are compatibility wrappers that run callbacks synchronously.",
      "reply(text) collects up to 4 reply segments; log(text) collects up to 8 audit-safe log lines.",
      "exit(message?) and assert(condition, message?) are normal control flow and are not treated as sandbox errors.",
      "In preview mode ctx.preview=true; write APIs return dry_run and do not mutate users or send real interactions.",
    ],
    lifecycleTitle: "Recommended Flow",
    lifecycleItems: [
      "Parse input with input.*, then guard arguments and permissions with assert()/exit().",
      "Read current user data with users.current(); cross-user reads or writes require administrator privileges.",
      "Admin writes use controlled fields, last-admin protection, and audit logs.",
      "Production Bot commands should use js:preset:<id> so command bindings pick up preset updates.",
    ],
    functionIntro: "Functions are grouped by namespace. Mutating functions are marked; admin-only functions show their scope.",
    typeIntro: "These are runtime object shapes, not TypeScript declarations. Fields are sanitized snapshots or controlled return values.",
    configIntro: "config(key) and env(key) are read-only allowlists. Sensitive config, database connections, server lines, tokens, secrets, passwords, and API keys are never returned.",
    examplesIntro: "Examples come from the backend structured documentation and can be copied into Developer Mode presets.",
    fields: "Fields",
    params: "Parameters",
    returns: "Returns",
    required: "Required",
    optional: "Optional",
    defaultValue: "Default",
    example: "Example",
    insert: "Load into editor",
    mutates: "Mutates",
    scope: "Scope",
    blockedTokens: "Blocked tokens",
    riskTokens: "Allowed but risky tokens",
    noMatches: "No matching entries",
    configKeys: "Config keys",
    envKeys: "Environment keys",
    bindings: "Injected objects",
    nativeObjects: "Native JS objects",
    namespaces: "Namespaces",
    allCategories: "All categories",
  },
};

const categoryText: Record<TextLocale, Record<string, string>> = {
  "zh-Hans": {
    context: "上下文",
    user: "用户对象",
    constants: "常量",
    output: "输出",
    control: "控制流",
    auth: "鉴权",
    users: "用户数据",
    network: "网络",
    runtime: "运行时兼容",
    config: "配置",
    input: "参数解析",
    db: "数据库",
    admin: "管理员",
    system: "系统",
    text: "文本工具",
    arrays: "数组工具",
    time: "时间工具",
    format: "格式化",
    interactions: "Telegram 交互",
    native: "原生对象",
  },
  "zh-Hant": {
    context: "上下文",
    user: "使用者物件",
    constants: "常數",
    output: "輸出",
    control: "控制流程",
    auth: "鑑權",
    users: "使用者資料",
    network: "網路",
    runtime: "執行相容",
    config: "設定",
    input: "參數解析",
    db: "資料庫",
    admin: "管理員",
    system: "系統",
    text: "文字工具",
    arrays: "陣列工具",
    time: "時間工具",
    format: "格式化",
    interactions: "Telegram 互動",
    native: "原生物件",
  },
  "en-US": {},
};

const scopeText: Record<TextLocale, Record<string, string>> = {
  "zh-Hans": {
    admin_only: "仅管理员",
    current_user_only: "仅当前用户",
    current_chat_owner_only: "仅当前会话发起者",
  },
  "zh-Hant": {
    admin_only: "僅管理員",
    current_user_only: "僅目前使用者",
    current_chat_owner_only: "僅目前會話發起者",
  },
  "en-US": {
    admin_only: "admin only",
    current_user_only: "current user only",
    current_chat_owner_only: "current chat owner only",
  },
};

const zhHansDescriptions: Record<string, string> = {
  "ctx.private_chat": "当前命令是否来自私聊。群聊中建议只做只读输出，涉及写操作时必须再做管理员鉴权。",
  "ctx.command_time": "命令进入沙箱时的 Unix 秒级时间戳，可用于记录或格式化执行时间。",
  "ctx.preview": "是否来自后台预览。预览模式下写入类接口只返回 dry_run，不会真实修改数据。",
  "ctx.command": "标准化后的命令名称，例如 /hello。",
  command: "自动注入的命令触发对象，包含命令名、参数数组、参数文本、私聊状态、预览状态等非敏感信息。",
  input: "更方便的输入解析对象，提供 text、first、rest、count，以及 arg、has、flag、named 等参数解析函数。",
  args: "命令参数数组，不包含命令本身。例如 /tool ping now 会得到 [\"ping\", \"now\"]。",
  user: "当前 Telegram 绑定的 Twilight 用户脱敏快照。可包含邮箱和 Telegram 用户名/ID 等联系信息，但不会包含密码哈希、Token、API Key、BGM Token 明文或 Emby 内部 ID。",
  me: "user 的快捷别名，适合在短脚本中读取当前绑定用户信息。",
  "constants.roles": "角色常量：admin=0、user=1、whitelist=2。",
  roles: "constants.roles 的快捷别名，包含 admin、user、whitelist。",
  "constants.limits": "运行期收集限制，例如 reply/log 的最大条数。",
  "reply(text)": "追加一段回复文本。最多收集 4 段，最终用换行合并发送；发送前会截断并脱敏。",
  "exit(text)": "正常提前结束当前脚本。传入文本时会先追加一段回复，不会被视为沙箱错误。",
  "assert(condition, text)": "条件为真时继续执行；条件为假时追加提示并正常退出。适合做参数校验和权限守卫。",
  "log(text)": "追加一条本次执行日志。最多 8 条，敏感内容会截断和脱敏；不要主动写入密钥。",
  "auth(role)": "检查当前绑定用户是否满足角色要求。支持 admin、whitelist、user 或数字角色。",
  "authAdmin()": "管理员快捷鉴权函数。当前 Telegram 绑定用户为管理员时返回 true。",
  "getUser(uid)": "按精确 UID 读取脱敏用户快照。普通用户只能读取自己，跨用户读取必须是管理员。",
  "fetch(url, options)": "高风险同步兼容函数。仅允许公开 HTTP(S) 的 GET/POST/HEAD，阻断 localhost、内网、链路本地目标、跳转和凭据，并限制响应体长度。",
  "setTimeout(fn, ms)": "兼容包装器，会在当前执行窗口内同步执行回调，不创建异步任务。",
  "setInterval(fn, ms)": "兼容包装器，会同步执行一次回调，不创建重复异步任务。",
  "config(key)": "读取白名单内的非敏感配置。未允许或敏感键返回空字符串并写入沙箱日志。",
  "env(key)": "读取白名单内的非敏感 TWILIGHT_* 环境变量。未允许或敏感键返回空字符串并写入沙箱日志。",
  "input.arg(index, fallback)": "按从 0 开始的下标读取一个参数；不存在时返回 fallback。",
  "input.has(index)": "判断指定位置参数是否存在且非空。",
  "input.flag(name)": "判断是否出现 --name 或 -name 标记。",
  "input.named(name, fallback)": "读取 --name=value、-name=value、--name value 或 -name value 形式的命名参数。",
  "db.schema()": "返回受控数据库集合结构和允许字段，不暴露原始 state。",
  "db.collections()": "返回 JS 沙箱允许查看的受控集合名。",
  "db.count(name)": "返回允许集合的计数；管理员专属集合对非管理员返回 -1。",
  "db.currentUser()": "返回与 users.current() 相同的当前用户脱敏快照。",
  "db.getUser(uid)": "按精确 UID 查询脱敏用户快照，权限规则与 getUser(uid) 相同。",
  "db.findUsers(query, limit)": "管理员专用：按 UID、用户名、邮箱、Telegram 用户名/ID 或 Emby 用户名搜索用户，最多返回 50 条脱敏快照。",
  "db.listUsers(options)": "列出用户。普通用户只能看到自己；管理员可使用 limit、offset、role、active 分页和筛选。",
  "db.listRegcodes(options)": "管理员专用：返回脱敏注册码快照，不含任何用户密钥。字段见 db.schema().regcodes.fields。",
  "db.listInviteCodes(options)": "返回邀请码快照。管理员可看全部；普通用户只能看到自己拥有的邀请码。",
  "db.listMediaRequests(options)": "返回求片快照。管理员可看全部；普通用户只能看到自己的求片记录。",
  "db.listAnnouncements(options)": "返回任意用户可见的公告快照，不含正文内容。",
  "db.listTickets(options)": "返回工单快照。管理员可看全部；普通用户只能看到自己的工单。工单正文不包含在内。",
  "db.listPresets(options)": "管理员专用：返回开发者 JS 预设的元数据。不含代码正文，只暴露 code_length。",
  "db.updateCurrentUser(patch)": "仅允许修改当前绑定用户的登录通知偏好；预览模式返回 dry_run。",
  "db.updateUser(uid, patch)": "管理员专用：受控更新用户状态、角色、到期时间和登录通知偏好，运行时写入审计日志。",
  "users.current()": "返回当前 Telegram 绑定用户的脱敏快照。",
  "users.describe()": "users.current() 的可读别名。",
  "users.get(uid)": "getUser(uid) 的命名空间形式，按精确 UID 返回脱敏快照或 null。",
  "users.byUID(uid)": "users.get(uid) 的别名。",
  "users.search(query, limit)": "管理员专用：按 UID、用户名、邮箱、Telegram 用户名/ID 或 Emby 用户名搜索用户。",
  "users.list(options)": "列出用户。普通用户仅返回自己；管理员可分页并按角色或启停状态筛选。",
  "users.hasRole(role)": "按 auth(role) 相同规则检查当前用户角色。",
  "users.requireActive()": "仅当前 Telegram 已绑定本地用户且 Web 账号启用时返回 true。",
  "users.setLoginNotify(options)": "修改当前绑定用户的登录通知偏好，只接受 telegram/email 布尔字段。",
  "users.setActive(uid, active)": "管理员专用：启用或停用 Web 账号，并保留最后一个启用管理员保护。",
  "users.setRole(uid, role)": "管理员专用：修改用户角色，支持 roles 或 constants.roles 中的角色常量。",
  "users.setExpiry(uid, expiredAt)": "管理员专用：修改用户到期时间，Unix 秒时间戳，-1 表示永久。",
  "users.update(uid, patch)": "管理员专用：组合式受控更新，支持 active、role、expired_at 和登录通知字段。",
  "admin.ok()": "当前 Telegram 绑定用户是管理员时返回 true。",
  "admin.ensure()": "管理员守卫函数；非管理员时写入沙箱日志并返回 false。",
  "admin.searchUsers(query, limit)": "users.search 的管理员快捷形式。",
  "admin.listUsers(options)": "users.list 的管理员快捷形式。",
  "admin.updateUser(uid, patch)": "users.update 的管理员快捷形式，写操作会写入审计日志。",
  "admin.setActive(uid, active)": "users.setActive 的管理员快捷形式。",
  "admin.setRole(uid, role)": "users.setRole 的管理员快捷形式。",
  "admin.setExpiry(uid, expiredAt)": "users.setExpiry 的管理员快捷形式。",
  "admin.stats()": "返回 system.stats() 摘要；管理员会看到额外管理计数。",
  "system.info()": "读取安全的系统元信息、功能开关和限制值，不返回原始配置密钥或敏感配置。",
  "system.feature(key)": "读取一个安全的布尔功能开关，例如 email_enabled 或 invite_enabled。",
  "system.stats()": "读取安全聚合统计；管理员会额外获得管理集合计数。",
  "text.truncate(value, max)": "按最大字符数截断文本。",
  "text.joinLines(values)": "把数组连接为多行文本。",
  "text.escape(value)": "转义基础 HTML 敏感字符，适合纯文本输出。",
  "text.numberLines(values)": "把数组转换为 1. / 2. 格式的编号文本。",
  "text.trim(value)": "移除字符串首尾空白。",
  "text.lower(value)": "转换为小写。",
  "text.upper(value)": "转换为大写。",
  "text.contains(value, needle)": "大小写不敏感的包含判断。",
  "text.split(value, separator)": "按分隔符拆分字符串，默认分隔符为空格。",
  "text.maskEmail(email)": "使用后端同款规则脱敏邮箱。",
  "text.template(template, data)": "使用简单对象替换 {key} 占位符，并自动截断和脱敏结果。",
  "arrays.first(values)": "返回数组第一项，没有则返回 undefined。",
  "arrays.last(values)": "返回数组最后一项，没有则返回 undefined。",
  "arrays.compact(values)": "移除 null 和空字符串。",
  "arrays.unique(values)": "按首次出现顺序去重字符串数组。",
  "arrays.take(values, count)": "截取数组前 count 项，负数会返回空数组。",
  "arrays.join(values, separator)": "把数组按字符串连接。",
  "arrays.includes(values, value)": "判断数组是否包含指定字符串。",
  "arrays.sortStrings(values)": "返回按字符串排序后的数组副本。",
  "time.now()": "返回当前 Unix 秒级时间戳。",
  "time.formatUnix(ts)": "把 Unix 秒级时间戳格式化为 UTC RFC3339 文本。",
  "time.fromNow(seconds)": "返回从当前时间偏移指定秒数后的 Unix 时间戳。",
  "time.addDays(ts, days)": "给 Unix 时间戳增加指定天数；ts<=0 时使用当前时间。",
  "time.duration(seconds)": "将秒数格式化为后端统一的时长文本。",
  "format.bool(value, yes, no)": "使用自定义文字格式化布尔值。",
  "format.role(role)": "把数字角色格式化为角色名称。",
  "format.date(ts)": "把 Unix 时间戳格式化为 UTC RFC3339；永久到期值显示到期标签。",
  "format.expiry(expiredAt)": "使用后端同款规则格式化用户到期状态。",
  "format.duration(seconds)": "格式化秒数。",
  "format.user(user)": "把受控用户快照格式化为紧凑的一行摘要。",
  "format.json(value)": "返回有长度限制的字符串表示；结构化 JSON 建议使用原生 JSON.stringify。",
  "interactions.inline(text, actions)": "发送 Telegram inline keyboard。按钮只保存静态 answer/edit/reply 动作，不会点击后再次运行 JS。",
  "interactions.waitText(options)": "等待同一 Telegram 用户在 1-60 秒内发送下一条非命令文本，并按限制截断、编号或回复。",
  Object: "Goja 提供的原生 JavaScript Object。",
  Array: "Goja 提供的原生 JavaScript Array。常见输出处理优先使用 arrays.*。",
  JSON: "原生 JSON parse/stringify 支持。",
  Math: "原生 Math 工具。",
  Date: "原生 Date 支持。命令输出建议优先使用 time.now/time.formatUnix。",
  "Function / eval": "Goja 兼容能力，风险较高，仅建议在管理员审查后的预设中使用。",
  globalThis: "绑定到 Goja 全局对象；不会提供浏览器或 Node.js 全局对象。",
  "String / Number / Boolean": "原生基础类型包装与原型方法。",
};

const zhHantDescriptions: Record<string, string> = {
  "ctx.private_chat": "目前命令是否來自私聊。群聊中建議只做唯讀輸出，涉及寫操作時必須再做管理員鑑權。",
  "ctx.command_time": "命令進入沙箱時的 Unix 秒級時間戳，可用於記錄或格式化執行時間。",
  "ctx.preview": "是否來自後台預覽。預覽模式下寫入類介面只返回 dry_run，不會真實修改資料。",
  "ctx.command": "標準化後的命令名稱，例如 /hello。",
  command: "自動注入的命令觸發物件，包含命令名、參數陣列、參數文字、私聊狀態、預覽狀態等非敏感資訊。",
  input: "更方便的輸入解析物件，提供 text、first、rest、count，以及 arg、has、flag、named 等參數解析函式。",
  args: "命令參數陣列，不包含命令本身。例如 /tool ping now 會得到 [\"ping\", \"now\"]。",
  user: "目前 Telegram 綁定的 Twilight 使用者脫敏快照。可包含信箱和 Telegram 使用者名稱/ID 等聯絡資訊，但不會包含密碼雜湊、Token、API Key、BGM Token 明文或 Emby 內部 ID。",
  me: "user 的快捷別名，適合在短腳本中讀取目前綁定使用者資訊。",
  "constants.roles": "角色常數：admin=0、user=1、whitelist=2。",
  roles: "constants.roles 的快捷別名，包含 admin、user、whitelist。",
  "constants.limits": "執行期收集限制，例如 reply/log 的最大條數。",
  "reply(text)": "追加一段回覆文字。最多收集 4 段，最終用換行合併傳送；傳送前會截斷並脫敏。",
  "exit(text)": "正常提前結束目前腳本。傳入文字時會先追加一段回覆，不會被視為沙箱錯誤。",
  "assert(condition, text)": "條件為真時繼續執行；條件為假時追加提示並正常退出。適合做參數校驗和權限守衛。",
  "log(text)": "追加一條本次執行日誌。最多 8 條，敏感內容會截斷和脫敏；不要主動寫入密鑰。",
  "auth(role)": "檢查目前綁定使用者是否滿足角色要求。支援 admin、whitelist、user 或數字角色。",
  "authAdmin()": "管理員快捷鑑權函式。目前 Telegram 綁定使用者為管理員時返回 true。",
  "getUser(uid)": "按精確 UID 讀取脫敏使用者快照。普通使用者只能讀取自己，跨使用者讀取必須是管理員。",
  "fetch(url, options)": "高風險同步相容函式。僅允許公開 HTTP(S) 的 GET/POST/HEAD，阻斷 localhost、內網、鏈路本地目標、跳轉和憑據，並限制回應體長度。",
  "setTimeout(fn, ms)": "相容包裝器，會在目前執行視窗內同步執行回呼，不建立非同步任務。",
  "setInterval(fn, ms)": "相容包裝器，會同步執行一次回呼，不建立重複非同步任務。",
  "config(key)": "讀取白名單內的非敏感設定。未允許或敏感鍵返回空字串並寫入沙箱日誌。",
  "env(key)": "讀取白名單內的非敏感 TWILIGHT_* 環境變數。未允許或敏感鍵返回空字串並寫入沙箱日誌。",
  "input.arg(index, fallback)": "按從 0 開始的索引讀取一個參數；不存在時返回 fallback。",
  "input.has(index)": "判斷指定位置參數是否存在且非空。",
  "input.flag(name)": "判斷是否出現 --name 或 -name 標記。",
  "input.named(name, fallback)": "讀取 --name=value、-name=value、--name value 或 -name value 形式的命名參數。",
  "db.schema()": "返回受控資料庫集合結構和允許欄位，不暴露原始 state。",
  "db.collections()": "返回 JS 沙箱允許查看的受控集合名。",
  "db.count(name)": "返回允許集合的計數；管理員專屬集合對非管理員返回 -1。",
  "db.currentUser()": "返回與 users.current() 相同的目前使用者脫敏快照。",
  "db.getUser(uid)": "按精確 UID 查詢脫敏使用者快照，權限規則與 getUser(uid) 相同。",
  "db.findUsers(query, limit)": "管理員專用：按 UID、使用者名稱、信箱、Telegram 使用者名稱/ID 或 Emby 使用者名稱搜尋使用者，最多返回 50 條脫敏快照。",
  "db.listUsers(options)": "列出使用者。普通使用者只能看到自己；管理員可使用 limit、offset、role、active 分頁和篩選。",
  "db.listRegcodes(options)": "管理員專用：返回脫敏註冊碼快照，不含任何使用者密鑰。欄位見 db.schema().regcodes.fields。",
  "db.listInviteCodes(options)": "返回邀請碼快照。管理員可看全部；普通使用者只能看到自己擁有的邀請碼。",
  "db.listMediaRequests(options)": "返回求片快照。管理員可看全部；普通使用者只能看到自己的求片紀錄。",
  "db.listAnnouncements(options)": "返回任意使用者可見的公告快照，不含正文內容。",
  "db.listTickets(options)": "返回工單快照。管理員可看全部；普通使用者只能看到自己的工單。工單正文不包含在內。",
  "db.listPresets(options)": "管理員專用：返回開發者 JS 預設的中繼資料。不含程式碼正文，只暴露 code_length。",
  "db.updateCurrentUser(patch)": "僅允許修改目前綁定使用者的登入通知偏好；預覽模式返回 dry_run。",
  "db.updateUser(uid, patch)": "管理員專用：受控更新使用者狀態、角色、到期時間和登入通知偏好，執行時寫入稽核日誌。",
  "users.current()": "返回目前 Telegram 綁定使用者的脫敏快照。",
  "users.describe()": "users.current() 的可讀別名。",
  "users.get(uid)": "getUser(uid) 的命名空間形式，按精確 UID 返回脫敏快照或 null。",
  "users.byUID(uid)": "users.get(uid) 的別名。",
  "users.search(query, limit)": "管理員專用：按 UID、使用者名稱、信箱、Telegram 使用者名稱/ID 或 Emby 使用者名稱搜尋使用者。",
  "users.list(options)": "列出使用者。普通使用者僅返回自己；管理員可分頁並按角色或啟停狀態篩選。",
  "users.hasRole(role)": "按 auth(role) 相同規則檢查目前使用者角色。",
  "users.requireActive()": "僅目前 Telegram 已綁定本地使用者且 Web 帳號啟用時返回 true。",
  "users.setLoginNotify(options)": "修改目前綁定使用者的登入通知偏好，只接受 telegram/email 布林欄位。",
  "users.setActive(uid, active)": "管理員專用：啟用或停用 Web 帳號，並保留最後一個啟用管理員保護。",
  "users.setRole(uid, role)": "管理員專用：修改使用者角色，支援 roles 或 constants.roles 中的角色常數。",
  "users.setExpiry(uid, expiredAt)": "管理員專用：修改使用者到期時間，Unix 秒時間戳，-1 表示永久。",
  "users.update(uid, patch)": "管理員專用：組合式受控更新，支援 active、role、expired_at 和登入通知欄位。",
  "admin.ok()": "目前 Telegram 綁定使用者是管理員時返回 true。",
  "admin.ensure()": "管理員守衛函式；非管理員時寫入沙箱日誌並返回 false。",
  "admin.searchUsers(query, limit)": "users.search 的管理員快捷形式。",
  "admin.listUsers(options)": "users.list 的管理員快捷形式。",
  "admin.updateUser(uid, patch)": "users.update 的管理員快捷形式，寫操作會寫入稽核日誌。",
  "admin.setActive(uid, active)": "users.setActive 的管理員快捷形式。",
  "admin.setRole(uid, role)": "users.setRole 的管理員快捷形式。",
  "admin.setExpiry(uid, expiredAt)": "users.setExpiry 的管理員快捷形式。",
  "admin.stats()": "返回 system.stats() 摘要；管理員會看到額外管理計數。",
  "system.info()": "讀取安全的系統中繼資訊、功能開關和限制值，不返回原始設定密鑰或敏感設定。",
  "system.feature(key)": "讀取一個安全的布林功能開關，例如 email_enabled 或 invite_enabled。",
  "system.stats()": "讀取安全聚合統計；管理員會額外獲得管理集合計數。",
  "text.truncate(value, max)": "按最大字元數截斷文字。",
  "text.joinLines(values)": "把陣列連接為多行文字。",
  "text.escape(value)": "轉義基礎 HTML 敏感字元，適合純文字輸出。",
  "text.numberLines(values)": "把陣列轉換為 1. / 2. 格式的編號文字。",
  "text.trim(value)": "移除字串首尾空白。",
  "text.lower(value)": "轉換為小寫。",
  "text.upper(value)": "轉換為大寫。",
  "text.contains(value, needle)": "大小寫不敏感的包含判斷。",
  "text.split(value, separator)": "按分隔符拆分字串，預設分隔符為空格。",
  "text.maskEmail(email)": "使用後端同款規則脫敏信箱。",
  "text.template(template, data)": "使用簡單物件替換 {key} 佔位符，並自動截斷和脫敏結果。",
  "arrays.first(values)": "返回陣列第一項，沒有則返回 undefined。",
  "arrays.last(values)": "返回陣列最後一項，沒有則返回 undefined。",
  "arrays.compact(values)": "移除 null 和空字串。",
  "arrays.unique(values)": "按首次出現順序去重字串陣列。",
  "arrays.take(values, count)": "截取陣列前 count 項，負數會返回空陣列。",
  "arrays.join(values, separator)": "把陣列按字串連接。",
  "arrays.includes(values, value)": "判斷陣列是否包含指定字串。",
  "arrays.sortStrings(values)": "返回按字串排序後的陣列副本。",
  "time.now()": "返回目前 Unix 秒級時間戳。",
  "time.formatUnix(ts)": "把 Unix 秒級時間戳格式化為 UTC RFC3339 文字。",
  "time.fromNow(seconds)": "返回從目前時間偏移指定秒數後的 Unix 時間戳。",
  "time.addDays(ts, days)": "給 Unix 時間戳增加指定天數；ts<=0 時使用目前時間。",
  "time.duration(seconds)": "將秒數格式化為後端統一的時長文字。",
  "format.bool(value, yes, no)": "使用自訂文字格式化布林值。",
  "format.role(role)": "把數字角色格式化為角色名稱。",
  "format.date(ts)": "把 Unix 時間戳格式化為 UTC RFC3339；永久到期值顯示到期標籤。",
  "format.expiry(expiredAt)": "使用後端同款規則格式化使用者到期狀態。",
  "format.duration(seconds)": "格式化秒數。",
  "format.user(user)": "把受控使用者快照格式化為緊湊的一行摘要。",
  "format.json(value)": "返回有長度限制的字串表示；結構化 JSON 建議使用原生 JSON.stringify。",
  "interactions.inline(text, actions)": "傳送 Telegram inline keyboard。按鈕只保存靜態 answer/edit/reply 動作，不會點擊後再次執行 JS。",
  "interactions.waitText(options)": "等待同一 Telegram 使用者在 1-60 秒內傳送下一條非命令文字，並按限制截斷、編號或回覆。",
  Object: "Goja 提供的原生 JavaScript Object。",
  Array: "Goja 提供的原生 JavaScript Array。常見輸出處理優先使用 arrays.*。",
  JSON: "原生 JSON parse/stringify 支援。",
  Math: "原生 Math 工具。",
  Date: "原生 Date 支援。命令輸出建議優先使用 time.now/time.formatUnix。",
  "Function / eval": "Goja 相容能力，風險較高，僅建議在管理員審查後的預設中使用。",
  globalThis: "綁定到 Goja 全域物件；不會提供瀏覽器或 Node.js 全域物件。",
  "String / Number / Boolean": "原生基礎型別包裝與原型方法。",
};

const descriptions: Record<TextLocale, Record<string, string>> = {
  "zh-Hans": zhHansDescriptions,
  "zh-Hant": zhHantDescriptions,
  "en-US": {},
};

const paramDescriptions: Record<TextLocale, Record<string, string>> = {
  "zh-Hans": {
    text: "要发送或展示的文本。发送前会按后端限制截断并脱敏。",
    condition: "用于判断是否继续执行的值；真值继续，假值触发提前退出。",
    role: "角色名或角色 ID，例如 admin/0、user/1、whitelist/2。",
    uid: "精确的 Twilight 用户 UID。",
    url: "公开 http/https 地址；localhost、内网、链路本地地址和非 HTTP 协议会被阻断。",
    key: "白名单允许读取的配置键、环境变量键或功能开关键。",
    index: "从 0 开始的参数下标。",
    fallback: "目标参数不存在时返回的备用值。",
    name: "参数名、选项名、集合名或标记名，可带或不带前导短横线。",
    query: "搜索文本。管理员可匹配 UID、用户名、邮箱、Telegram 用户名/ID 或 Emby 用户名。",
    limit: "最多返回条数；后端会按安全上限截断。",
    options: "选项对象，只读取文档列出的允许字段。",
    patch: "受控更新对象，只接受文档列出的允许字段。",
    active: "新的 Web 账号启用状态。",
    expiredAt: "Unix 秒级到期时间；-1 表示永久。",
    value: "任意可转为字符串或布尔值的值。",
    values: "数组或可转换为数组的输入。",
    max: "最大字符数。",
    needle: "要查找的文本。",
    separator: "分隔符。",
    email: "邮箱地址；展示前会按后端规则脱敏。",
    template: "包含 {key} 占位符的模板字符串。",
    data: "用于替换占位符的简单对象。",
    count: "要截取的条数。",
    ts: "Unix 秒级时间戳。",
    days: "要增加的天数；负数表示向前调整。",
    seconds: "秒数，可用于等待窗口、偏移时间或时长格式化。",
    user: "来自 users.current/list/search/get 的脱敏用户对象。",
    actions: "按钮定义数组。callback 只执行预设 answer/edit/reply，不会再次运行 JS。",
    fn: "回调函数；会在同一次沙箱执行中立即同步运行。",
    ms: "请求的延迟或间隔毫秒数；仅记录日志，不创建异步任务。",
    yes: "值为真时使用的文本。",
    no: "值为假时使用的文本。",
  },
  "zh-Hant": {
    text: "要傳送或顯示的文字。傳送前會依後端限制截斷並脫敏。",
    condition: "用於判斷是否繼續執行的值；真值繼續，假值觸發提前退出。",
    role: "角色名稱或角色 ID，例如 admin/0、user/1、whitelist/2。",
    uid: "精確的 Twilight 使用者 UID。",
    url: "公開 http/https 位址；localhost、內網、鏈路本地位址和非 HTTP 協定會被阻擋。",
    key: "白名單允許讀取的設定鍵、環境變數鍵或功能開關鍵。",
    index: "從 0 開始的參數索引。",
    fallback: "目標參數不存在時返回的備用值。",
    name: "參數名、選項名、集合名或標記名，可帶或不帶前導短橫線。",
    query: "搜尋文字。管理員可匹配 UID、使用者名稱、信箱、Telegram 使用者名稱/ID 或 Emby 使用者名稱。",
    limit: "最多返回筆數；後端會依安全上限截斷。",
    options: "選項物件，只讀取文件列出的允許欄位。",
    patch: "受控更新物件，只接受文件列出的允許欄位。",
    active: "新的 Web 帳號啟用狀態。",
    expiredAt: "Unix 秒級到期時間；-1 表示永久。",
    value: "任意可轉為字串或布林值的值。",
    values: "陣列或可轉換為陣列的輸入。",
    max: "最大字元數。",
    needle: "要查找的文字。",
    separator: "分隔符。",
    email: "信箱地址；顯示前會依後端規則脫敏。",
    template: "包含 {key} 佔位符的模板字串。",
    data: "用於替換佔位符的簡單物件。",
    count: "要截取的筆數。",
    ts: "Unix 秒級時間戳。",
    days: "要增加的天數；負數表示向前調整。",
    seconds: "秒數，可用於等待視窗、偏移時間或時長格式化。",
    user: "來自 users.current/list/search/get 的脫敏使用者物件。",
    actions: "按鈕定義陣列。callback 只執行預設 answer/edit/reply，不會再次執行 JS。",
    fn: "回呼函式；會在同一次沙箱執行中立即同步執行。",
    ms: "請求的延遲或間隔毫秒數；僅記錄日誌，不建立非同步任務。",
    yes: "值為真時使用的文字。",
    no: "值為假時使用的文字。",
  },
  "en-US": {},
};

const exampleText: Record<TextLocale, Record<string, { title: string; description: string }>> = {
  "zh-Hans": {
    "command-context": { title: "命令输入上下文", description: "展示用户触发命令时能读取到的所有非敏感字段。" },
    "current-user": { title: "当前用户摘要", description: "返回当前 Telegram 绑定用户的脱敏摘要。" },
    "exit-and-assert": { title: "提前退出与断言守卫", description: "使用 exit/assert 做参数校验，正常结束脚本而不产生运行错误。" },
    "db-summary": { title: "受控数据库摘要", description: "查看允许的集合结构和计数，不接触原始 state。" },
    "admin-get-user": { title: "管理员精确 UID 查询", description: "管理员按 UID 读取用户脱敏快照；普通用户不能跨用户读取。" },
    "login-notify": { title: "开启登录通知", description: "修改当前绑定用户的 Telegram 登录通知；预览模式只返回 dry_run。" },
    "db-update-current-user": { title: "当前用户受控写入", description: "只更新当前用户允许的通知字段。" },
    "admin-search-users": { title: "管理员搜索用户", description: "按 UID、用户名、邮箱、Telegram 或 Emby 用户名搜索用户。" },
    "admin-update-user": { title: "管理员受控更新用户", description: "更新允许字段；运行时写入审计日志。" },
    "system-info": { title: "系统功能开关", description: "读取安全的系统元信息和功能开关。" },
    "convenience-admin-lookup": { title: "便捷管理员查询", description: "组合 input、admin 和 format 快捷函数。" },
    "echo-arguments": { title: "回显参数和标记", description: "解析位置参数、布尔标记和命名选项。" },
    "template-self-summary": { title: "模板化当前用户摘要", description: "使用 text.template 生成可读回复。" },
    "self-email-status": { title: "当前用户邮箱状态", description: "展示邮箱、验证状态和登录通知开关。" },
    "system-stats-summary": { title: "系统统计摘要", description: "输出安全聚合计数；管理员可看到额外计数。" },
    "toggle-login-notify-by-flag": { title: "按参数开关登录通知", description: "使用 on/off 或 flag 更新当前用户通知偏好。" },
    "admin-list-filtered-users": { title: "管理员筛选用户列表", description: "分页列出符合条件的用户并格式化输出。" },
    "admin-set-expiry-days": { title: "管理员按天数设置到期", description: "使用命名参数设置用户到期时间。" },
    "admin-disable-user-with-confirm-flag": { title: "带确认参数禁用用户", description: "要求 --confirm 后才执行状态变更。" },
    "wait-text-note": { title: "等待下一条文本", description: "提示同一用户在限定时间内发送一条普通文本。" },
    "inline-user-menu": { title: "Inline 用户菜单", description: "发送静态按钮菜单，callback 不会再次执行 JS。" },
    "fetch-json-status": { title: "读取公开 JSON 状态", description: "使用受限 fetch 获取公开 HTTP(S) JSON 并处理失败。" },
    "complex-admin-audit-summary": { title: "复杂管理员摘要", description: "组合搜索、计数、格式化和有界输出。" },
    "risk-fetch": { title: "高风险兼容 fetch", description: "展示受限同步 fetch 的用法和错误处理。" },
  },
  "zh-Hant": {
    "command-context": { title: "命令輸入上下文", description: "展示使用者觸發命令時能讀取到的所有非敏感欄位。" },
    "current-user": { title: "目前使用者摘要", description: "返回目前 Telegram 綁定使用者的脫敏摘要。" },
    "exit-and-assert": { title: "提前退出與斷言守衛", description: "使用 exit/assert 做參數校驗，正常結束腳本而不產生執行錯誤。" },
    "db-summary": { title: "受控資料庫摘要", description: "查看允許的集合結構和計數，不接觸原始 state。" },
    "admin-get-user": { title: "管理員精確 UID 查詢", description: "管理員按 UID 讀取使用者脫敏快照；普通使用者不能跨使用者讀取。" },
    "login-notify": { title: "開啟登入通知", description: "修改目前綁定使用者的 Telegram 登入通知；預覽模式只返回 dry_run。" },
    "db-update-current-user": { title: "目前使用者受控寫入", description: "只更新目前使用者允許的通知欄位。" },
    "admin-search-users": { title: "管理員搜尋使用者", description: "按 UID、使用者名稱、信箱、Telegram 或 Emby 使用者名稱搜尋使用者。" },
    "admin-update-user": { title: "管理員受控更新使用者", description: "更新允許欄位；執行時寫入稽核日誌。" },
    "system-info": { title: "系統功能開關", description: "讀取安全的系統中繼資訊和功能開關。" },
    "convenience-admin-lookup": { title: "便捷管理員查詢", description: "組合 input、admin 和 format 快捷函式。" },
    "echo-arguments": { title: "回顯參數和標記", description: "解析位置參數、布林標記和命名選項。" },
    "template-self-summary": { title: "模板化目前使用者摘要", description: "使用 text.template 產生可讀回覆。" },
    "self-email-status": { title: "目前使用者信箱狀態", description: "展示信箱、驗證狀態和登入通知開關。" },
    "system-stats-summary": { title: "系統統計摘要", description: "輸出安全聚合計數；管理員可看到額外計數。" },
    "toggle-login-notify-by-flag": { title: "按參數開關登入通知", description: "使用 on/off 或 flag 更新目前使用者通知偏好。" },
    "admin-list-filtered-users": { title: "管理員篩選使用者列表", description: "分頁列出符合條件的使用者並格式化輸出。" },
    "admin-set-expiry-days": { title: "管理員按天數設定到期", description: "使用命名參數設定使用者到期時間。" },
    "admin-disable-user-with-confirm-flag": { title: "帶確認參數停用使用者", description: "要求 --confirm 後才執行狀態變更。" },
    "wait-text-note": { title: "等待下一條文字", description: "提示同一使用者在限定時間內傳送一條普通文字。" },
    "inline-user-menu": { title: "Inline 使用者選單", description: "傳送靜態按鈕選單，callback 不會再次執行 JS。" },
    "fetch-json-status": { title: "讀取公開 JSON 狀態", description: "使用受限 fetch 取得公開 HTTP(S) JSON 並處理失敗。" },
    "complex-admin-audit-summary": { title: "複雜管理員摘要", description: "組合搜尋、計數、格式化和有界輸出。" },
    "risk-fetch": { title: "高風險相容 fetch", description: "展示受限同步 fetch 的用法和錯誤處理。" },
  },
  "en-US": {},
};

const configKeyText: Record<TextLocale, Record<string, string>> = {
  "zh-Hans": {
    "app.name": "站点显示名称。",
    "site.name": "站点显示名称别名。",
    "global.server_name": "全局服务器显示名称。",
    "app.version": "当前后端版本。",
    "telegram.enabled": "Telegram Bot 模式是否启用。",
    "global.telegram_mode": "Telegram Bot 模式是否启用。",
    "telegram.force_bind": "是否要求用户绑定 Telegram。",
    "global.force_bind_telegram": "是否要求用户绑定 Telegram。",
    "telegram.require_membership": "是否要求 Telegram 群组成员身份。",
    "telegram.panel_enabled": "Telegram 面板能力是否启用。",
    "telegram.ban_on_leave": "退群封禁模式是否启用。",
    "invite.enabled": "邀请系统是否允许生成和使用新邀请码。",
    "invite.max_depth": "邀请关系最大深度。",
    "invite.limit": "普通邀请数量限制。",
    "invite.root_user_limit": "根用户邀请数量限制。",
    "email.enabled": "邮箱验证功能是否启用。",
    "email.force_bind": "是否强制绑定邮箱。",
    "media_request.enabled": "求片功能是否启用。",
    "signin.enabled": "签到功能是否启用。",
    "ticket.enabled": "工单系统是否启用。",
    "limits.user": "Web 用户容量限制。",
    "limits.emby_user": "Emby 用户容量限制。",
  },
  "zh-Hant": {
    "app.name": "站點顯示名稱。",
    "site.name": "站點顯示名稱別名。",
    "global.server_name": "全域伺服器顯示名稱。",
    "app.version": "目前後端版本。",
    "telegram.enabled": "Telegram Bot 模式是否啟用。",
    "global.telegram_mode": "Telegram Bot 模式是否啟用。",
    "telegram.force_bind": "是否要求使用者綁定 Telegram。",
    "global.force_bind_telegram": "是否要求使用者綁定 Telegram。",
    "telegram.require_membership": "是否要求 Telegram 群組成員身分。",
    "telegram.panel_enabled": "Telegram 面板能力是否啟用。",
    "telegram.ban_on_leave": "退群封禁模式是否啟用。",
    "invite.enabled": "邀請系統是否允許產生和使用新邀請碼。",
    "invite.max_depth": "邀請關係最大深度。",
    "invite.limit": "普通邀請數量限制。",
    "invite.root_user_limit": "根使用者邀請數量限制。",
    "email.enabled": "信箱驗證功能是否啟用。",
    "email.force_bind": "是否強制綁定信箱。",
    "media_request.enabled": "求片功能是否啟用。",
    "signin.enabled": "簽到功能是否啟用。",
    "ticket.enabled": "工單系統是否啟用。",
    "limits.user": "Web 使用者容量限制。",
    "limits.emby_user": "Emby 使用者容量限制。",
  },
  "en-US": {},
};

const envKeyText: Record<TextLocale, Record<string, string>> = {
  "zh-Hans": {
    TWILIGHT_APP_NAME: "应用名称环境变量，仅当进程环境中设置时有值。",
    TWILIGHT_SERVER_NAME: "站点名称环境变量，仅当进程环境中设置时有值。",
    TWILIGHT_HOST: "后端监听地址。",
    TWILIGHT_PORT: "后端监听端口。",
    TWILIGHT_BASE_URL: "后端公开基址，不包含任何密钥。",
    TWILIGHT_DATABASE_DRIVER: "存储驱动类型，不包含连接串或密码。",
    TWILIGHT_EMAIL_ENABLED: "邮箱功能环境开关。",
    TWILIGHT_TELEGRAM_REQUIRE_GROUP_MEMBERSHIP: "Telegram 群组成员校验开关。",
    TWILIGHT_TELEGRAM_BAN_ON_LEAVE: "退群封禁环境开关。",
    TWILIGHT_INVITE_ENABLED: "邀请功能环境开关。",
    TWILIGHT_MEDIA_REQUEST_ENABLED: "求片功能环境开关。",
  },
  "zh-Hant": {
    TWILIGHT_APP_NAME: "應用名稱環境變數，僅當程序環境中設定時有值。",
    TWILIGHT_SERVER_NAME: "站點名稱環境變數，僅當程序環境中設定時有值。",
    TWILIGHT_HOST: "後端監聽位址。",
    TWILIGHT_PORT: "後端監聽連接埠。",
    TWILIGHT_BASE_URL: "後端公開基址，不包含任何密鑰。",
    TWILIGHT_DATABASE_DRIVER: "儲存驅動類型，不包含連線字串或密碼。",
    TWILIGHT_EMAIL_ENABLED: "信箱功能環境開關。",
    TWILIGHT_TELEGRAM_REQUIRE_GROUP_MEMBERSHIP: "Telegram 群組成員校驗開關。",
    TWILIGHT_TELEGRAM_BAN_ON_LEAVE: "退群封禁環境開關。",
    TWILIGHT_INVITE_ENABLED: "邀請功能環境開關。",
    TWILIGHT_MEDIA_REQUEST_ENABLED: "求片功能環境開關。",
  },
  "en-US": {},
};

const typeSections: Record<TextLocale, Array<{ name: string; description: string; fields: string[]; example?: string }>> = {
  "zh-Hans": [
    {
      name: "UserSnapshot",
      description: "用户快照是所有 users.*、db.* 和 getUser(uid) 返回的核心类型。它允许显示账号状态和联系方式，但不会包含密码、Token、API Key、BGM Token 明文或 Emby 内部 ID。",
      fields: ["uid:number", "username:string", "email:string", "email_masked:string", "has_email:boolean", "role:number", "role_name:string", "active:boolean", "expired_at:number|null", "expire_status:string", "has_emby:boolean", "emby_username:string", "email_verified:boolean", "telegram_bound:boolean", "telegram_id:number|null", "telegram_username:string", "notify_on_login_telegram:boolean", "notify_on_login_email:boolean"],
      example: "const me = users.current();\nreply(format.user(me));",
    },
    {
      name: "CommandContext",
      description: "命令上下文描述触发来源和参数。它不暴露 chat ID、message ID 或群组 ID，避免脚本跨会话操作。",
      fields: ["ctx.private_chat:boolean", "ctx.command_time:number", "ctx.preview:boolean", "ctx.command:string", "command.name:string", "command.args:string[]", "command.text:string", "args:string[]"],
    },
    {
      name: "InputHelper",
      description: "input 是对 args 的便捷封装，适合解析位置参数、开关标记和命名选项。",
      fields: ["input.text:string", "input.first:string", "input.rest:string[]", "input.count:number", "input.arg(index,fallback):string", "input.has(index):boolean", "input.flag(name):boolean", "input.named(name,fallback):string"],
      example: "const uid = Number(input.named('uid', 0));\nconst force = input.flag('force');",
    },
    {
      name: "MutationResult",
      description: "写入类接口统一返回结构。预览模式会带 dry_run=true；运行时成功写入会返回更新后的脱敏用户快照并记录审计日志。",
      fields: ["ok:boolean", "dry_run?:boolean", "user?:UserSnapshot", "error?:string"],
    },
    {
      name: "InlineAction",
      description: "inline 按钮动作是静态文本，不会点击后重新运行 JS。callback 绑定同一 chat、message 和 Telegram 用户，并有过期时间。",
      fields: ["text:string", "answer?:string", "edit?:string", "reply?:string"],
    },
    {
      name: "FetchResult",
      description: "fetch 返回受控 HTTP 结果。被阻断的请求会返回 blocked=true；响应体有长度限制并会脱敏。",
      fields: ["ok:boolean", "status?:number", "statusText?:string", "text?:string", "truncated?:boolean", "error?:string", "blocked?:boolean"],
    },
  ],
  "zh-Hant": [
    {
      name: "UserSnapshot",
      description: "使用者快照是所有 users.*、db.* 和 getUser(uid) 返回的核心型別。它允許顯示帳號狀態和聯絡方式，但不會包含密碼、Token、API Key、BGM Token 明文或 Emby 內部 ID。",
      fields: ["uid:number", "username:string", "email:string", "email_masked:string", "has_email:boolean", "role:number", "role_name:string", "active:boolean", "expired_at:number|null", "expire_status:string", "has_emby:boolean", "emby_username:string", "email_verified:boolean", "telegram_bound:boolean", "telegram_id:number|null", "telegram_username:string", "notify_on_login_telegram:boolean", "notify_on_login_email:boolean"],
      example: "const me = users.current();\nreply(format.user(me));",
    },
    {
      name: "CommandContext",
      description: "命令上下文描述觸發來源和參數。它不暴露 chat ID、message ID 或群組 ID，避免腳本跨會話操作。",
      fields: ["ctx.private_chat:boolean", "ctx.command_time:number", "ctx.preview:boolean", "ctx.command:string", "command.name:string", "command.args:string[]", "command.text:string", "args:string[]"],
    },
    {
      name: "InputHelper",
      description: "input 是對 args 的便捷封裝，適合解析位置參數、開關標記和命名選項。",
      fields: ["input.text:string", "input.first:string", "input.rest:string[]", "input.count:number", "input.arg(index,fallback):string", "input.has(index):boolean", "input.flag(name):boolean", "input.named(name,fallback):string"],
      example: "const uid = Number(input.named('uid', 0));\nconst force = input.flag('force');",
    },
    {
      name: "MutationResult",
      description: "寫入類介面統一返回結構。預覽模式會帶 dry_run=true；執行時成功寫入會返回更新後的脫敏使用者快照並記錄稽核日誌。",
      fields: ["ok:boolean", "dry_run?:boolean", "user?:UserSnapshot", "error?:string"],
    },
    {
      name: "InlineAction",
      description: "inline 按鈕動作是靜態文字，不會點擊後重新執行 JS。callback 綁定同一 chat、message 和 Telegram 使用者，並有過期時間。",
      fields: ["text:string", "answer?:string", "edit?:string", "reply?:string"],
    },
    {
      name: "FetchResult",
      description: "fetch 返回受控 HTTP 結果。被阻擋的請求會返回 blocked=true；回應體有長度限制並會脫敏。",
      fields: ["ok:boolean", "status?:number", "statusText?:string", "text?:string", "truncated?:boolean", "error?:string", "blocked?:boolean"],
    },
  ],
  "en-US": [
    {
      name: "UserSnapshot",
      description: "The central sanitized user shape returned by users.*, db.*, and getUser(uid). It includes account status and contact metadata, but never passwords, tokens, API keys, raw BGM tokens, or internal Emby IDs.",
      fields: ["uid:number", "username:string", "email:string", "email_masked:string", "has_email:boolean", "role:number", "role_name:string", "active:boolean", "expired_at:number|null", "expire_status:string", "has_emby:boolean", "emby_username:string", "email_verified:boolean", "telegram_bound:boolean", "telegram_id:number|null", "telegram_username:string", "notify_on_login_telegram:boolean", "notify_on_login_email:boolean"],
      example: "const me = users.current();\nreply(format.user(me));",
    },
    {
      name: "CommandContext",
      description: "Command context describes the trigger source and arguments. It does not expose chat ID, message ID, or group ID.",
      fields: ["ctx.private_chat:boolean", "ctx.command_time:number", "ctx.preview:boolean", "ctx.command:string", "command.name:string", "command.args:string[]", "command.text:string", "args:string[]"],
    },
    {
      name: "InputHelper",
      description: "input wraps args for positional arguments, flags, and named options.",
      fields: ["input.text:string", "input.first:string", "input.rest:string[]", "input.count:number", "input.arg(index,fallback):string", "input.has(index):boolean", "input.flag(name):boolean", "input.named(name,fallback):string"],
      example: "const uid = Number(input.named('uid', 0));\nconst force = input.flag('force');",
    },
    {
      name: "MutationResult",
      description: "Common return shape for controlled writes. Preview mode sets dry_run=true; runtime writes return an updated sanitized snapshot and audit log.",
      fields: ["ok:boolean", "dry_run?:boolean", "user?:UserSnapshot", "error?:string"],
    },
    {
      name: "InlineAction",
      description: "Inline button actions are static text actions. Callbacks do not rerun JavaScript and are bound to the same chat, message, and Telegram user.",
      fields: ["text:string", "answer?:string", "edit?:string", "reply?:string"],
    },
    {
      name: "FetchResult",
      description: "Controlled HTTP result from fetch. Blocked requests set blocked=true; response bodies are bounded and redacted.",
      fields: ["ok:boolean", "status?:number", "statusText?:string", "text?:string", "truncated?:boolean", "error?:string", "blocked?:boolean"],
    },
  ],
};

function asTextLocale(locale: Locale): TextLocale {
  if (locale === "zh-Hant") return "zh-Hant";
  if (locale === "en-US") return "en-US";
  return "zh-Hans";
}

function categoryLabel(locale: TextLocale, category: string) {
  return categoryText[locale][category] || category;
}

function scopeLabel(locale: TextLocale, scope?: string) {
  if (!scope) return "";
  return scopeText[locale][scope] || scope;
}

function localizedDescription(locale: TextLocale, row: DeveloperJSDocEntry) {
  if (locale === "en-US") return row.description;
  return descriptions[locale][row.name] || genericDescription(locale, row);
}

function genericDescription(locale: TextLocale, row: DeveloperJSDocEntry) {
  const category = categoryLabel(locale, row.category);
  if (locale === "zh-Hant") {
    if (row.type === "function") return `${category}函式。請依參數表傳入受控資料；未列出的全域物件不會被注入。`;
    if (row.type === "object") return `${category}物件。只包含文件列出的受控欄位。`;
    return `${category}條目。此能力由沙箱明確注入，未列出的能力不可用。`;
  }
  if (row.type === "function") return `${category}函数。请按参数表传入受控数据；未列出的全局对象不会被注入。`;
  if (row.type === "object") return `${category}对象。只包含文档列出的受控字段。`;
  return `${category}条目。此能力由沙箱明确注入，未列出的能力不可用。`;
}

function localizedParam(locale: TextLocale, param: DocParam) {
  if (locale === "en-US") return param;
  const direct = paramDescriptions[locale][param.name];
  const root = paramDescriptions[locale][param.name.replace(/^options\./, "")];
  return {
    ...param,
    description: direct || root || (locale === "zh-Hant" ? "受控參數。請依型別傳入，不要包含敏感資訊。" : "受控参数。请按类型传入，不要包含敏感信息。"),
    default: localizeDefault(locale, param.default),
  };
}

function localizeDefault(locale: TextLocale, value?: string) {
  if (!value || locale === "en-US") return value;
  if (value === "backend default") return locale === "zh-Hant" ? "後端預設值" : "后端默认值";
  if (value === "space") return locale === "zh-Hant" ? "空格" : "空格";
  return value;
}

function localizedEntry(locale: TextLocale, row: DeveloperJSDocEntry): DeveloperJSDocEntry {
  return {
    ...row,
    description: localizedDescription(locale, row),
    params: row.params?.map((param) => localizedParam(locale, param)),
  };
}

function localizedExample(locale: TextLocale, example: DeveloperJSDocs["examples"][number]) {
  if (locale === "en-US") return example;
  return { ...example, ...(exampleText[locale][example.id] || { title: example.id, description: locale === "zh-Hant" ? "可直接調整後用於 JS 預設。" : "可直接调整后用于 JS 预设。" }) };
}

function keyRows(locale: TextLocale, keys: string[], kind: "config" | "env"): DeveloperJSDocEntry[] {
  const table = kind === "config" ? configKeyText[locale] : envKeyText[locale];
  return keys.map((name) => ({
    name,
    category: kind,
    type: "key",
    description: locale === "en-US"
      ? (kind === "config" ? "Allowlisted non-sensitive config key." : "Allowlisted non-sensitive environment key.")
      : table[name] || (kind === "config"
        ? (locale === "zh-Hant" ? "白名單允許讀取的非敏感設定鍵。" : "白名单允许读取的非敏感配置键。")
        : (locale === "zh-Hant" ? "白名單允許讀取的非敏感環境變數鍵。" : "白名单允许读取的非敏感环境变量键。")),
  }));
}

function matchRow(row: DeveloperJSDocEntry, query: string) {
  if (!query) return true;
  const haystack = [
    row.name,
    row.category,
    row.type,
    row.description,
    row.scope,
    row.returns,
    row.fields?.join(" "),
    row.params?.map((param) => `${param.name} ${param.type} ${param.description}`).join(" "),
  ].filter(Boolean).join(" ").toLowerCase();
  return haystack.includes(query.toLowerCase());
}

function filterRows(rows: DeveloperJSDocEntry[], query: string, category?: string) {
  return rows.filter((row) => (!category || row.category === category) && matchRow(row, query));
}

// returnNote derives a short plain-language explanation for a raw return-type
// string so admins understand the shape without reading TypeScript. It is locale
// aware for all three locales (English is generated here, not supplied by backend).
function returnNote(locale: TextLocale, returns?: string): string {
  if (!returns) return "";
  const r = returns.trim();
  const en = locale === "en-US";
  const zhHant = locale === "zh-Hant";
  if (/\bok\s*:\s*boolean/.test(r) && /dry_run/.test(r)) {
    if (en) return "Controlled write result. ok marks success; dry_run is a preview without real changes; error carries the failure reason.";
    if (zhHant) return "受控寫入結果。ok 表示是否成功，dry_run 代表僅預覽未實際變更，error 為失敗原因。";
    return "受控写入结果。ok 表示是否成功，dry_run 代表仅预览未实际变更，error 为失败原因。";
  }
  if (/^void$/.test(r)) {
    if (en) return "No return value.";
    return zhHant ? "無回傳值。" : "无返回值。";
  }
  if (/^never$/.test(r)) {
    if (en) return "Stops the current script run; nothing is returned to the caller.";
    return zhHant ? "結束當前腳本執行，不會回傳。" : "结束当前脚本执行，不会返回。";
  }
  if (/UserSnapshot/.test(r)) {
    const arr = /\[\]/.test(r);
    const nullable = /null/.test(r);
    if (en) return arr ? "Array of sanitized user snapshots." : nullable ? "Sanitized user snapshot, or null when not found or not permitted." : "Sanitized user snapshot.";
    if (zhHant) return arr ? "脫敏使用者快照陣列。" : nullable ? "脫敏使用者快照；找不到或無權限時為 null。" : "脫敏使用者快照。";
    return arr ? "脱敏用户快照数组。" : nullable ? "脱敏用户快照；找不到或无权限时为 null。" : "脱敏用户快照。";
  }
  if (/Snapshot/.test(r)) {
    const arr = /\[\]/.test(r);
    if (en) return arr ? "Array of masked snapshots; secrets are never included." : "Masked snapshot; secrets are never included.";
    if (zhHant) return arr ? "脫敏快照陣列，永不包含機密。" : "脫敏快照，永不包含機密。";
    return arr ? "脱敏快照数组，永不包含机密。" : "脱敏快照，永不包含机密。";
  }
  if (/^boolean$/.test(r)) {
    if (en) return "Boolean true/false result.";
    return zhHant ? "布林 true/false 結果。" : "布尔 true/false 结果。";
  }
  return "";
}

// DocRowsGrouped renders entries grouped by category with a heading and count,
// giving the Functions tab a clearer structure than a flat list when no single
// category is selected.
function DocRowsGrouped({ rows, labels: l, locale, emptyIcon }: { rows: DeveloperJSDocEntry[]; labels: Labels; locale: TextLocale; emptyIcon?: React.ReactNode }) {
  if (rows.length === 0) {
    return (
      <div className="flex min-h-24 items-center justify-center rounded-md border border-dashed bg-muted/20 p-4 text-sm text-muted-foreground">
        {emptyIcon}
        <span className="ml-2">{l.noMatches}</span>
      </div>
    );
  }
  const groups: Array<{ category: string; rows: DeveloperJSDocEntry[] }> = [];
  const index = new Map<string, number>();
  for (const row of rows) {
    let pos = index.get(row.category);
    if (pos === undefined) {
      pos = groups.length;
      index.set(row.category, pos);
      groups.push({ category: row.category, rows: [] });
    }
    groups[pos].rows.push(row);
  }
  groups.sort((a, b) => a.category.localeCompare(b.category));
  return (
    <div className="space-y-4">
      {groups.map((group) => (
        <div key={group.category} className="space-y-2">
          <div className="flex items-center gap-2 border-b pb-1">
            <p className="text-sm font-semibold">{categoryLabel(locale, group.category)}</p>
            <Badge variant="secondary" className="text-[10px]">{group.rows.length}</Badge>
          </div>
          <DocRows rows={group.rows} labels={l} locale={locale} />
        </div>
      ))}
    </div>
  );
}

function DocRows({ rows, labels: l, locale, emptyIcon }: { rows: DeveloperJSDocEntry[]; labels: Labels; locale: TextLocale; emptyIcon?: React.ReactNode }) {
  if (rows.length === 0) {
    return (
      <div className="flex min-h-24 items-center justify-center rounded-md border border-dashed bg-muted/20 p-4 text-sm text-muted-foreground">
        {emptyIcon}
        <span className="ml-2">{l.noMatches}</span>
      </div>
    );
  }
  return (
    <div className="grid gap-3">
      {rows.map((row) => (
        <div key={`${row.category}-${row.name}`} className="rounded-md border bg-muted/20 p-3">
          <div className="flex flex-wrap items-center gap-2">
            <code className="break-all font-mono text-xs">{row.name}</code>
            {row.type ? <Badge variant="secondary" className="text-[10px]">{row.type}</Badge> : null}
            <Badge variant="outline" className="text-[10px]">{categoryLabel(locale, row.category)}</Badge>
            {row.mutates ? <Badge variant="warning" className="text-[10px]">{l.mutates}</Badge> : null}
            {row.scope ? <Badge variant="outline" className="text-[10px]">{scopeLabel(locale, row.scope)}</Badge> : null}
          </div>
          <p className="mt-2 break-words text-xs leading-relaxed text-muted-foreground">{row.description}</p>
          {row.fields?.length ? (
            <div className="mt-2">
              <p className="mb-1 text-[11px] font-medium text-muted-foreground">{l.fields}</p>
              <div className="flex flex-wrap gap-1">
                {row.fields.map((field) => <Badge key={`${row.name}-${field}`} variant="secondary" className="max-w-full break-all text-[10px]">{field}</Badge>)}
              </div>
            </div>
          ) : null}
          {row.params?.length ? (
            <div className="mt-3 space-y-1.5">
              <p className="text-[11px] font-medium text-muted-foreground">{l.params}</p>
              {row.params.map((param) => (
                <div key={`${row.name}-${param.name}`} className="rounded border bg-background/70 p-2 text-[11px]">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <code className="break-all font-mono font-semibold">{param.name}</code>
                    {param.type ? <Badge variant="secondary" className="text-[10px]">{param.type}</Badge> : null}
                    <Badge variant={param.required ? "default" : "outline"} className="text-[10px]">{param.required ? l.required : l.optional}</Badge>
                    {param.default ? <span className="break-all text-muted-foreground">{l.defaultValue}: <code>{param.default}</code></span> : null}
                  </div>
                  <p className="mt-1 break-words text-muted-foreground">{param.description}</p>
                </div>
              ))}
            </div>
          ) : null}
          {row.returns ? (
            <div className="mt-2 rounded border border-dashed bg-background/50 p-2 text-[11px]">
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="font-medium text-muted-foreground">{l.returns}:</span>
                <code className="break-all font-mono">{row.returns}</code>
              </div>
              {returnNote(locale, row.returns) ? (
                <p className="mt-1 break-words text-muted-foreground">{returnNote(locale, row.returns)}</p>
              ) : null}
            </div>
          ) : null}
          {row.example ? (
            <div className="mt-3">
              <p className="mb-1 text-[11px] font-medium text-muted-foreground">{l.example}</p>
              <pre className="max-h-56 overflow-auto whitespace-pre-wrap rounded-md bg-background p-2 text-[11px]">{row.example}</pre>
            </div>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function TypeCards({ locale, labels: l, query }: { locale: TextLocale; labels: Labels; query: string }) {
  const sections = typeSections[locale].filter((section) => {
    if (!query) return true;
    return [section.name, section.description, section.fields.join(" "), section.example].join(" ").toLowerCase().includes(query.toLowerCase());
  });
  return (
    <div className="grid gap-3">
      {sections.map((section) => (
        <div key={section.name} className="rounded-md border bg-muted/20 p-3">
          <div className="flex flex-wrap items-center gap-2">
            <code className="font-mono text-xs">{section.name}</code>
            <Badge variant="secondary" className="text-[10px]">type</Badge>
          </div>
          <p className="mt-2 text-xs leading-relaxed text-muted-foreground">{section.description}</p>
          <div className="mt-3 grid gap-1.5">
            {section.fields.map((field) => {
              const sep = field.indexOf(":");
              const fieldName = sep > 0 ? field.slice(0, sep).trim() : field;
              const fieldType = sep > 0 ? field.slice(sep + 1).trim() : "";
              return (
                <div key={`${section.name}-${field}`} className="flex flex-wrap items-center gap-1.5 rounded border bg-background/70 px-2 py-1 text-[11px]">
                  <code className="break-all font-mono font-semibold">{fieldName}</code>
                  {fieldType ? <Badge variant="secondary" className="max-w-full break-all text-[10px]">{fieldType}</Badge> : null}
                </div>
              );
            })}
          </div>
          {section.example ? <pre className="mt-3 overflow-auto whitespace-pre-wrap rounded-md bg-background p-2 text-[11px]">{section.example}</pre> : null}
        </div>
      ))}
      {sections.length === 0 ? (
        <div className="rounded-md border border-dashed bg-muted/20 p-4 text-center text-sm text-muted-foreground">{l.noMatches}</div>
      ) : null}
    </div>
  );
}

export function DeveloperJSDocsPanel({ className, onInsertSnippet }: DeveloperJSDocsPanelProps) {
  const { locale } = useI18n();
  const textLocale = asTextLocale(locale);
  const l = labels[textLocale];
  const { toast } = useToast();
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState<string>("");

  const loadDocs = useCallback(async (signal?: AbortSignal): Promise<DeveloperJSDocs> => {
    const res = await api.getDeveloperJSDocs(signal);
    if (res.success && res.data) return res.data;
    throw new Error(res.message || l.loadFailed);
  }, [l.loadFailed]);

  const { data: docs, isLoading: loading, error: loadError } = useAsyncResource(loadDocs, { immediate: true });

  useEffect(() => {
    if (loadError) {
      toast({ title: l.loadFailed, description: loadError, variant: "destructive" });
    }
  }, [l.loadFailed, loadError, toast]);

  const view = useMemo(() => {
    if (!docs) return null;
    const bindings = docs.bindings.map((row) => localizedEntry(textLocale, row));
    const functions = docs.functions.map((row) => localizedEntry(textLocale, row));
    const namespaces = docs.namespaces.map((row) => localizedEntry(textLocale, row));
    const nativeObjects = docs.native_objects.map((row) => localizedEntry(textLocale, row));
    const configKeys = keyRows(textLocale, docs.config_keys, "config");
    const envKeys = keyRows(textLocale, docs.env_keys, "env");
    const categories = Array.from(new Set(functions.map((row) => row.category))).sort();
    return {
      engine: docs.engine,
      bindings,
      functions,
      namespaces,
      nativeObjects,
      configKeys,
      envKeys,
      examples: docs.examples.map((example) => localizedExample(textLocale, example)),
      blockedTokens: docs.blocked_tokens,
      riskTokens: docs.risk_tokens || [],
      categories,
    };
  }, [docs, textLocale]);

  const searchedBindings = view ? filterRows(view.bindings, query) : [];
  const searchedFunctions = view ? filterRows(view.functions, query, category || undefined) : [];
  const searchedNamespaces = view ? filterRows(view.namespaces, query) : [];
  const searchedNative = view ? filterRows(view.nativeObjects, query) : [];
  const searchedConfig = view ? filterRows(view.configKeys, query) : [];
  const searchedEnv = view ? filterRows(view.envKeys, query) : [];
  const searchedExamples = view ? view.examples.filter((example) => {
    if (!query) return true;
    return [example.id, example.title, example.description, example.code].join(" ").toLowerCase().includes(query.toLowerCase());
  }) : [];

  return (
    <Card className={cn(className)}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BookOpen className="h-5 w-5" />
          {l.title}
        </CardTitle>
        <CardDescription>{l.description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={l.searchPlaceholder}
            className="pl-9"
          />
        </div>

        {loading && !view ? (
          <div className="flex items-center gap-2 rounded-md border bg-muted/20 p-4 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {l.loading}
          </div>
        ) : view ? (
          <Tabs defaultValue="overview" className="space-y-4">
            <div className="custom-scrollbar overflow-x-auto overscroll-x-contain pb-1">
            <TabsList className="i18n-stable-tabs grid h-auto min-w-[34rem] grid-cols-2 md:grid-cols-3 xl:grid-cols-6 xl:min-w-0">
              <TabsTrigger value="overview">{l.overviewTab}</TabsTrigger>
              <TabsTrigger value="context">{l.contextTab}</TabsTrigger>
              <TabsTrigger value="functions">{l.functionsTab}</TabsTrigger>
              <TabsTrigger value="types">{l.typesTab}</TabsTrigger>
              <TabsTrigger value="config">{l.configTab}</TabsTrigger>
              <TabsTrigger value="examples">{l.examplesTab}</TabsTrigger>
            </TabsList>
            </div>

            <TabsContent value="overview" className="space-y-4">
              <div className="grid gap-3 lg:grid-cols-2">
                <div className="rounded-md border bg-muted/20 p-4">
                  <div className="flex flex-wrap items-center gap-2">
                    <TerminalSquare className="h-4 w-4" />
                    <p className="font-medium">{l.engineTitle}</p>
                    <Badge variant="secondary">{view.engine.name}</Badge>
                    <Badge variant="outline">{view.engine.timeout_ms}ms</Badge>
                  </div>
                  <p className="mt-2 text-sm text-muted-foreground">
                    {textLocale === "en-US"
                      ? view.engine.description
                      : textLocale === "zh-Hant"
                        ? "Telegram 自訂 JS 指令使用的進程內 Go JavaScript 引擎。"
                        : "Telegram 自定义 JS 指令使用的进程内 Go JavaScript 引擎。"}
                  </p>
                  <code className="mt-2 block break-words text-[11px] text-muted-foreground">
                    {view.engine.module}@{view.engine.version}
                  </code>
                </div>
                <div className="rounded-md border bg-muted/20 p-4">
                  <div className="flex items-center gap-2">
                    <ShieldCheck className="h-4 w-4" />
                    <p className="font-medium">{l.securityTitle}</p>
                  </div>
                  <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{l.securityIntro}</p>
                </div>
              </div>

              <div className="grid gap-3 lg:grid-cols-2">
                <div className="rounded-md border bg-muted/20 p-4">
                  <p className="font-medium">{l.runtimeTitle}</p>
                  <ul className="mt-2 list-inside list-disc space-y-1 text-sm text-muted-foreground">
                    {l.runtimeItems.map((item) => <li key={item}>{item}</li>)}
                  </ul>
                </div>
                <div className="rounded-md border bg-muted/20 p-4">
                  <p className="font-medium">{l.lifecycleTitle}</p>
                  <ul className="mt-2 list-inside list-disc space-y-1 text-sm text-muted-foreground">
                    {l.lifecycleItems.map((item) => <li key={item}>{item}</li>)}
                  </ul>
                </div>
              </div>

              <div className="grid gap-3 lg:grid-cols-2">
                <div className="rounded-md border bg-muted/20 p-3">
                  <p className="mb-2 text-xs font-medium">{l.blockedTokens}</p>
                  <div className="flex flex-wrap gap-1">
                    {view.blockedTokens.map((token) => <Badge key={token} variant="outline" className="text-[10px]">{token}</Badge>)}
                  </div>
                </div>
                <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3">
                  <p className="mb-2 text-xs font-medium">{l.riskTokens}</p>
                  <div className="flex flex-wrap gap-1">
                    {view.riskTokens.map((token) => <Badge key={token} variant="warning" className="text-[10px]">{token}</Badge>)}
                  </div>
                </div>
              </div>
            </TabsContent>

            <TabsContent value="context" className="space-y-4">
              <div className="rounded-md border bg-muted/20 p-3 text-sm text-muted-foreground">{l.bindings}</div>
              <DocRows rows={searchedBindings} labels={l} locale={textLocale} />
              <div className="rounded-md border bg-muted/20 p-3 text-sm text-muted-foreground">{l.nativeObjects}</div>
              <DocRows rows={searchedNative} labels={l} locale={textLocale} />
            </TabsContent>

            <TabsContent value="functions" className="space-y-4">
              <p className="text-sm text-muted-foreground">{l.functionIntro}</p>
              <div className="custom-scrollbar flex gap-2 overflow-x-auto overscroll-x-contain pb-2">
                <Button type="button" size="sm" variant={category === "" ? "default" : "outline"} onClick={() => setCategory("")}>
                  {l.allCategories}
                </Button>
                {view.categories.map((item) => (
                  <Button key={item} type="button" size="sm" variant={category === item ? "default" : "outline"} onClick={() => setCategory(item)}>
                    {categoryLabel(textLocale, item)}
                  </Button>
                ))}
              </div>
              {category === "" ? (
                <DocRowsGrouped rows={searchedFunctions} labels={l} locale={textLocale} emptyIcon={<Code2 className="h-4 w-4" />} />
              ) : (
                <DocRows rows={searchedFunctions} labels={l} locale={textLocale} emptyIcon={<Code2 className="h-4 w-4" />} />
              )}
              <div className="rounded-md border bg-muted/20 p-3 text-sm text-muted-foreground">{l.namespaces}</div>
              <DocRows rows={searchedNamespaces} labels={l} locale={textLocale} />
            </TabsContent>

            <TabsContent value="types" className="space-y-4">
              <p className="text-sm text-muted-foreground">{l.typeIntro}</p>
              <TypeCards locale={textLocale} labels={l} query={query} />
            </TabsContent>

            <TabsContent value="config" className="space-y-4">
              <p className="text-sm text-muted-foreground">{l.configIntro}</p>
              <div className="rounded-md border bg-muted/20 p-3 text-sm font-medium">{l.configKeys}</div>
              <DocRows rows={searchedConfig} labels={l} locale={textLocale} emptyIcon={<Database className="h-4 w-4" />} />
              <div className="rounded-md border bg-muted/20 p-3 text-sm font-medium">{l.envKeys}</div>
              <DocRows rows={searchedEnv} labels={l} locale={textLocale} emptyIcon={<Database className="h-4 w-4" />} />
            </TabsContent>

            <TabsContent value="examples" className="space-y-4">
              <p className="text-sm text-muted-foreground">{l.examplesIntro}</p>
              <div className="grid gap-3 lg:grid-cols-2">
                {searchedExamples.map((example) => (
                  <div key={example.id} className="rounded-md border bg-muted/20 p-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-sm font-medium">{example.title}</p>
                      <Badge variant="outline" className="text-[10px]">{example.id}</Badge>
                    </div>
                    <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{example.description}</p>
                    <pre className="custom-scrollbar mt-2 max-h-[min(32dvh,18rem)] overflow-auto overscroll-contain whitespace-pre-wrap rounded-md bg-background p-2 text-[11px]">{example.code}</pre>
                    {onInsertSnippet ? (
                      <Button type="button" variant="outline" size="sm" className="mt-2" onClick={() => onInsertSnippet(`\n${example.code}\n`)}>
                        {l.insert}
                      </Button>
                    ) : null}
                  </div>
                ))}
              </div>
              {searchedExamples.length === 0 ? <div className="rounded-md border border-dashed bg-muted/20 p-4 text-center text-sm text-muted-foreground">{l.noMatches}</div> : null}
            </TabsContent>
          </Tabs>
        ) : null}
      </CardContent>
    </Card>
  );
}
