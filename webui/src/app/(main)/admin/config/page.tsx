"use client";

import { useCallback, useEffect, useState, useMemo, useRef } from "react";
import { motion, AnimatePresence } from "framer-motion";
import {
  Settings,
  Save,
  Loader2,
  AlertTriangle,
  Eye,
  EyeOff,
  FileText,
  SlidersHorizontal,
  Plus,
  X,
  Search,
  RotateCcw,
  ChevronDown,
  ChevronRight,
  Globe,
  Tv,
  Send,
  Coins,
  Monitor,
  Server,
  Shield,
  Clock,
  Bell,
  BookOpen,
  Info,
  CircleDot,
  GitPullRequest,
  Database,
  Archive,
  Trash2,
  Upload,
  Image as ImageIcon,
} from "lucide-react";
import { GithubIcon } from "@/components/icons/github-icon";
import { GithubProjectLink } from "@/components/github-project-link";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useAsyncResource } from "@/hooks/use-async-resource";
import { PageError } from "@/components/layout/page-state";
import { api } from "@/lib/api";
import { useI18n, type MessageKey, type MessageParams } from "@/lib/i18n";
import { useSystemStore } from "@/store/system";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type {
  ConfigSchema,
  ConfigSection,
  ConfigField,
  ConfigCategory,
  ConfigBackup,
  ConfigBackupView,
  ConfigRestoreResult,
} from "@/lib/api";

// 没有声明 categories 时的回退：所有 section 归到「全部」一类，保持原来的扁平体验
const FALLBACK_CATEGORY: ConfigCategory = { key: "_all", title: "all" };

type Translate = (key: MessageKey, params?: MessageParams) => string;

const NUMERIC_LIST_FIELD_KEYS = new Set([
  "streak_bonus_days",
  "streak_bonus_points",
]);

const STRING_ID_LIST_FIELD_KEYS = new Set([
  "admin_id",
  "group_id",
  "channel_id",
]);

const CONFIG_RESTORE_CONFIRM = "RESTORE_CONFIG_BACKUP";

function toEditorList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((v) => String(v ?? ""));
  }
  if (value === null || value === undefined || value === "") {
    return [];
  }
  return [String(value)];
}

function serializeListValue(
  fieldKey: string,
  value: unknown,
  originalValue: unknown
): unknown[] {
  const items = toEditorList(value)
    .map((v) => v.trim())
    .filter((v) => v.length > 0);

  if (STRING_ID_LIST_FIELD_KEYS.has(fieldKey)) {
    return items;
  }

  const originalLooksNumeric =
    Array.isArray(originalValue) &&
    originalValue.length > 0 &&
    originalValue.every((v) => typeof v === "number");

  if (NUMERIC_LIST_FIELD_KEYS.has(fieldKey) || originalLooksNumeric) {
    return items.map((v) => (/^-?\d+$/.test(v) ? Number.parseInt(v, 10) : v));
  }

  return items;
}

function serializeFieldValue(
  field: ConfigField,
  editedValue: unknown,
  originalValue: unknown
): unknown {
  if (field.type === "list") {
    return serializeListValue(field.key, editedValue, originalValue);
  }
  if (field.type === "int") {
    if (typeof editedValue === "number") return editedValue;
    if (typeof editedValue === "string" && /^-?\d+$/.test(editedValue.trim())) {
      return Number.parseInt(editedValue.trim(), 10);
    }
    return originalValue;
  }
  if (field.type === "float") {
    if (typeof editedValue === "number") return editedValue;
    if (typeof editedValue === "string" && /^-?\d+(\.\d+)?$/.test(editedValue.trim())) {
      return Number.parseFloat(editedValue.trim());
    }
    return originalValue;
  }
  if (field.type === "bool") {
    return Boolean(editedValue);
  }
  return editedValue;
}

function formatBytes(value?: number): string {
  const size = Number(value || 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / 1024 / 1024).toFixed(2)} MB`;
}

function formatUnixTime(value?: number): string {
  if (!value) return "-";
  return new Date(value * 1000).toLocaleString("zh-CN");
}

interface ConfigChangeItem {
  sectionKey: string;
  sectionTitle: string;
  field: ConfigField;
  before: unknown;
  after: unknown;
}

function isConfigValueChanged(next: unknown, prev: unknown): boolean {
  return JSON.stringify(next) !== JSON.stringify(prev);
}

function truncateConfigPreview(value: string, emptyLabel: string): string {
  const compact = value.replace(/\s+/g, " ").trim();
  if (!compact) return emptyLabel;
  return compact.length > 80 ? `${compact.slice(0, 77)}...` : compact;
}

function formatConfigPreviewValue(field: ConfigField, value: unknown, t: Translate): string {
  if (field.type === "secret") {
    return value ? t("common.setHidden") : t("common.unset");
  }
  if (field.type === "bool") {
    return value ? t("common.enabled") : t("common.disabled");
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return t("common.emptyList");
    const sample = value
      .slice(0, 3)
      .map((item) => truncateConfigPreview(typeof item === "string" ? item : JSON.stringify(item), t("common.empty")))
      .join("；");
    return t(value.length > 3 ? "common.itemCountMore" : "common.itemCount", { count: value.length, sample });
  }
  if (value && typeof value === "object") {
    return truncateConfigPreview(JSON.stringify(value), t("common.empty"));
  }
  if (value === null || value === undefined) return t("common.empty");
  return truncateConfigPreview(String(value), t("common.empty"));
}

// ==================== 动画 ====================

const container = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: { staggerChildren: 0.05 },
  },
};

const item = {
  hidden: { opacity: 0, y: 12 },
  show: { opacity: 1, y: 0 },
};

// ==================== Section 图标映射 ====================

const SECTION_ICONS: Record<string, React.ElementType> = {
  Global: Globe,
  Emby: Tv,
  Telegram: Send,
  Signin: Coins,
  SAR: Coins,
  DeviceLimit: Monitor,
  RateLimit: Shield,
  API: Server,
  Database,
  Security: Shield,
  Scheduler: Clock,
  Notification: Bell,
  BangumiSync: BookOpen,
};

// ==================== 字段渲染组件 ====================

function SecretField({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative">
      <Input
        type={visible ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="pr-10"
      />
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="absolute right-0 top-0 h-full px-3 hover:bg-transparent"
        onClick={() => setVisible(!visible)}
      >
        {visible ? (
          <EyeOff className="h-4 w-4 text-muted-foreground" />
        ) : (
          <Eye className="h-4 w-4 text-muted-foreground" />
        )}
      </Button>
    </div>
  );
}

function ListField({
  value,
  onChange,
}: {
  value: unknown;
  onChange: (v: unknown[]) => void;
}) {
  const { t } = useI18n();
  const items = toEditorList(value);

  const addItem = () => onChange([...items, ""]);
  const removeItem = (idx: number) =>
    onChange(items.filter((_, i) => i !== idx));
  const updateItem = (idx: number, val: string) => {
    const next = [...items];
    next[idx] = val;
    onChange(next);
  };

  return (
    <div className="space-y-2">
      {items.map((it, idx) => (
        <div key={idx} className="flex gap-2">
          <Input
            value={it}
            onChange={(e) => updateItem(idx, e.target.value)}
            className="flex-1"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={() => removeItem(idx)}
            className="shrink-0 text-muted-foreground hover:text-destructive"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={addItem}
        className="w-full"
      >
        <Plus className="h-4 w-4 mr-1" />
        {t("adminConfig.addItem")}
      </Button>
    </div>
  );
}

function toCommandRows(value: unknown): Array<{ command: string; reply: string }> {
  if (!Array.isArray(value)) return [];
  return value.map((item) => {
    if (item && typeof item === "object") {
      const row = item as Record<string, unknown>;
      return { command: String(row.command ?? ""), reply: String(row.reply ?? "") };
    }
    const text = String(item ?? "");
    const [command, ...replyParts] = text.split(" = ");
    return { command: command ?? "", reply: replyParts.join(" = ") };
  });
}

function CommandMapField({
  value,
  onChange,
}: {
  value: unknown;
  onChange: (v: Array<{ command: string; reply: string }>) => void;
}) {
  const { t } = useI18n();
  const rows = toCommandRows(value);
  const updateRow = (idx: number, patch: Partial<{ command: string; reply: string }>) => {
    const next = [...rows];
    next[idx] = { ...next[idx], ...patch };
    onChange(next);
  };
  const addRow = () => onChange([...rows, { command: "/", reply: "" }]);
  const removeRow = (idx: number) => onChange(rows.filter((_, i) => i !== idx));

  return (
    <div className="space-y-3">
      {rows.map((row, idx) => (
        <div key={idx} className="grid gap-2 rounded-md border p-3 sm:grid-cols-[minmax(140px,220px)_1fr_auto]">
          <Input
            value={row.command}
            placeholder="/hello"
            onChange={(e) => updateRow(idx, { command: e.target.value })}
            className="font-mono"
          />
          <Textarea
            value={row.reply}
            placeholder={t("adminConfig.commandReplyPlaceholder")}
            onChange={(e) => updateRow(idx, { reply: e.target.value })}
            className="min-h-20 font-mono text-sm"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={() => removeRow(idx)}
            className="text-muted-foreground hover:text-destructive"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
      ))}
      <Button type="button" variant="outline" size="sm" onClick={addRow} className="w-full">
        <Plus className="mr-1 h-4 w-4" />
        {t("adminConfig.addCustomCommand")}
      </Button>
    </div>
  );
}

function ConfigFieldEditor({
  field,
  value,
  onChange,
}: {
  field: ConfigField;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  switch (field.type) {
    case "bool":
      return (
        <Switch
          checked={!!value}
          onCheckedChange={(checked) => onChange(checked)}
        />
      );

    case "int":
      return (
        <Input
          type="number"
          value={value as number}
          onChange={(e) => onChange(parseInt(e.target.value) || 0)}
        />
      );

    case "float":
      return (
        <Input
          type="number"
          step="0.01"
          value={value as number}
          onChange={(e) => onChange(parseFloat(e.target.value) || 0)}
        />
      );

    case "secret":
      return (
        <SecretField
          value={(value as string) ?? ""}
          onChange={onChange}
        />
      );

    case "textarea":
      return (
        <Textarea
          value={(value as string) ?? ""}
          onChange={(e) => onChange(e.target.value)}
          className="min-h-32 font-mono text-sm"
        />
      );

    case "list":
      return (
        <ListField
          value={Array.isArray(value) ? value : []}
          onChange={onChange}
        />
      );

    case "command_map":
      return <CommandMapField value={value} onChange={onChange} />;

    case "select":
      return (
        <div className="grid gap-2 sm:grid-cols-2">
          {(field.options ?? []).map((opt) => {
            const selected = String(value) === String(opt.value);
            return (
              <Button
                key={String(opt.value)}
                type="button"
                variant={selected ? "default" : "outline"}
                className="h-auto min-h-10 justify-start whitespace-normal px-3 py-2 text-left text-sm"
                onClick={() => onChange(opt.value)}
              >
                <CircleDot className={`mr-2 h-4 w-4 shrink-0 ${selected ? "opacity-100" : "opacity-35"}`} />
                <span>{opt.label}</span>
              </Button>
            );
          })}
        </div>
      );

    default:
      return (
        <Input
          type="text"
          value={(value as string) ?? ""}
          onChange={(e) => onChange(e.target.value)}
        />
      );
  }
}

// ==================== 单个字段行 ====================

function FieldRow({
  field,
  value,
  isChanged,
  onFieldChange,
  onReset,
  highlight,
}: {
  field: ConfigField;
  value: unknown;
  isChanged: boolean;
  onFieldChange: (value: unknown) => void;
  onReset: () => void;
  highlight?: string;
}) {
  const { t } = useI18n();
  const labelRef = useRef<HTMLDivElement>(null);
  const showFullDescription = field.key === "group_user_panel_template";

  // 高亮搜索匹配
  const highlightText = (text: string) => {
    if (!highlight) return text;
    const idx = text.toLowerCase().indexOf(highlight.toLowerCase());
    if (idx === -1) return text;
    return (
      <>
        {text.slice(0, idx)}
        <mark className="bg-yellow-200 dark:bg-yellow-800 rounded px-0.5">
          {text.slice(idx, idx + highlight.length)}
        </mark>
        {text.slice(idx + highlight.length)}
      </>
    );
  };

  return (
    <div
      ref={labelRef}
      className={`group relative rounded-lg border px-4 py-3 transition-colors ${
        isChanged
          ? "border-amber-300 bg-amber-50/50 dark:border-amber-700 dark:bg-amber-950/20"
          : "border-transparent hover:bg-muted/40"
      }`}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:gap-4">
        <div className="flex-1 min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <Label className="min-w-0 text-sm font-medium leading-none">
              {highlightText(field.label)}
            </Label>
            <code className="hidden sm:inline text-[11px] font-mono text-muted-foreground bg-muted px-1.5 py-0.5 rounded">
              {field.key}
            </code>
            {isChanged && (
              <Badge variant="warning" className="text-[10px] px-1.5 py-0 h-4">
                {t("adminConfig.changed")}
              </Badge>
            )}
            <TooltipProvider delayDuration={300}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Info className="h-3.5 w-3.5 text-muted-foreground/60 cursor-help" />
                </TooltipTrigger>
                <TooltipContent side="top" className={showFullDescription ? "max-w-2xl whitespace-pre-wrap" : "max-w-xs"}>
                  <p>{field.description}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
          <p className={showFullDescription ? "text-xs leading-relaxed text-muted-foreground whitespace-pre-wrap break-words" : "text-xs text-muted-foreground line-clamp-1"}>
            {field.description}
          </p>
        </div>

        <div className="flex items-center gap-2 self-start shrink-0">
          {isChanged && (
            <TooltipProvider delayDuration={200}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    onClick={onReset}
                    className="h-8 w-8 opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-foreground"
                  >
                    <RotateCcw className="h-3.5 w-3.5" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t("adminConfig.resetChange")}</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
          {field.type === "bool" && (
            <ConfigFieldEditor
              field={field}
              value={value}
              onChange={onFieldChange}
            />
          )}
        </div>
      </div>

      {field.type !== "bool" && (
        <div className="mt-2">
          <ConfigFieldEditor
            field={field}
            value={value}
            onChange={onFieldChange}
          />
        </div>
      )}
    </div>
  );
}

// ==================== Section 卡片（可折叠） ====================

function SectionCard({
  section,
  values,
  originalValues,
  changedCount,
  onFieldChange,
  onResetField,
  searchText,
  matchedFieldKeys,
  isExpanded,
  onToggle,
}: {
  section: ConfigSection;
  values: Record<string, unknown>;
  originalValues: Record<string, unknown>;
  changedCount: number;
  onFieldChange: (sectionKey: string, fieldKey: string, value: unknown) => void;
  onResetField: (sectionKey: string, fieldKey: string) => void;
  searchText: string;
  matchedFieldKeys: Set<string>;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const { t } = useI18n();
  const Icon = SECTION_ICONS[section.key] || CircleDot;
  const visibleFields = searchText
    ? section.fields.filter((f) => matchedFieldKeys.has(f.key))
    : section.fields;

  if (searchText && visibleFields.length === 0) return null;

  return (
    <motion.div variants={item} id={`section-${section.key}`}>
      <Card className="overflow-hidden">
        <button
          type="button"
          className="w-full text-left"
          onClick={onToggle}
        >
          <CardHeader className="cursor-pointer select-none hover:bg-muted/30 transition-colors py-4">
            <div className="flex min-w-0 items-center gap-3">
              <div className="flex items-center justify-center h-9 w-9 rounded-lg bg-primary/10 text-primary">
                <Icon className="h-5 w-5" />
              </div>
              <div className="flex-1 min-w-0">
                <CardTitle className="flex min-w-0 flex-wrap items-center gap-2 text-base">
                  <span className="truncate">{section.title}</span>
                  {changedCount > 0 && (
                    <Badge variant="warning" className="text-[10px] px-1.5 py-0">
                      {t("adminConfig.changedFieldCount", { count: changedCount })}
                    </Badge>
                  )}
                </CardTitle>
                <CardDescription className="text-xs mt-0.5">
                  {section.description}
                </CardDescription>
              </div>
              <div className="text-muted-foreground">
                {isExpanded ? (
                  <ChevronDown className="h-5 w-5" />
                ) : (
                  <ChevronRight className="h-5 w-5" />
                )}
              </div>
            </div>
          </CardHeader>
        </button>
        <AnimatePresence initial={false}>
          {isExpanded && (
            <motion.div
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: "auto", opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.2 }}
            >
              <CardContent className="pt-0 pb-4 space-y-1">
                {visibleFields.map((field) => {
                  const val =
                    values[field.key] !== undefined
                      ? values[field.key]
                      : field.value;
                  const origVal = originalValues[field.key];
                  const isChanged = isConfigValueChanged(val, origVal);
                  return (
                    <FieldRow
                      key={field.key}
                      field={field}
                      value={val}
                      isChanged={isChanged}
                      onFieldChange={(v) =>
                        onFieldChange(section.key, field.key, v)
                      }
                      onReset={() => onResetField(section.key, field.key)}
                      highlight={searchText}
                    />
                  );
                })}
              </CardContent>
            </motion.div>
          )}
        </AnimatePresence>
      </Card>
    </motion.div>
  );
}

// ==================== Section 导航侧边栏 ====================

function SectionNav({
  sections,
  categories,
  activeSection,
  changedCounts,
  onSelect,
}: {
  sections: ConfigSection[];
  categories: ConfigCategory[];
  activeSection: string;
  changedCounts: Record<string, number>;
  onSelect: (key: string) => void;
}) {
  const { t } = useI18n();
  // 把 section 按 category 分组，未声明 category 的统一归到 FALLBACK_CATEGORY。
  const grouped: Array<{ category: ConfigCategory; sections: ConfigSection[] }> = [];
  const seen = new Map<string, ConfigSection[]>();
  const orderedKeys: string[] = [];

  const ensureBucket = (key: string) => {
    if (!seen.has(key)) {
      seen.set(key, []);
      orderedKeys.push(key);
    }
    return seen.get(key)!;
  };

  for (const section of sections) {
    const catKey = section.category && categories.some((c) => c.key === section.category)
      ? section.category
      : FALLBACK_CATEGORY.key;
    ensureBucket(catKey).push(section);
  }

  for (const key of orderedKeys) {
    const category =
      categories.find((c) => c.key === key) || (key === FALLBACK_CATEGORY.key ? FALLBACK_CATEGORY : null);
    if (!category) continue;
    grouped.push({ category, sections: seen.get(key) || [] });
  }

  return (
    <nav className="hidden xl:block w-52 shrink-0">
      <div className="sticky top-20 space-y-4">
        <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider px-2">
          {t("adminConfig.configGroup")}
        </p>
        {grouped.map(({ category, sections: groupSections }) => (
          <div key={category.key} className="space-y-0.5">
            <p className="px-2 text-[11px] font-medium text-muted-foreground/70">
              {category.key === FALLBACK_CATEGORY.key ? t("adminConfig.fallbackCategoryAll") : category.title}
            </p>
            {groupSections.map((section) => {
              const Icon = SECTION_ICONS[section.key] || CircleDot;
              const count = changedCounts[section.key] || 0;
              const isActive = activeSection === section.key;
              return (
                <button
                  key={section.key}
                  onClick={() => onSelect(section.key)}
                  className={`w-full flex items-center gap-2 px-2.5 py-2 rounded-md text-sm transition-colors ${
                    isActive
                      ? "bg-primary/10 text-primary font-medium"
                      : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                  }`}
                >
                  <Icon className="h-4 w-4 shrink-0" />
                  <span className="flex-1 text-left truncate">
                    {section.title}
                  </span>
                  {count > 0 && (
                    <span className="text-[10px] font-medium bg-amber-500 text-white rounded-full h-4 min-w-4 px-1 flex items-center justify-center">
                      {count}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        ))}
      </div>
    </nav>
  );
}

// ==================== 主页面 ====================

export default function AdminConfigPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const { confirm } = useConfirm();
  const { fetchInfo: fetchSystemInfo } = useSystemStore();

  // 源文件编辑状态
  const [configContent, setConfigContent] = useState("");
  const [originalContent, setOriginalContent] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [configPath, setConfigPath] = useState("");
  const [hasChanges, setHasChanges] = useState(false);

  // 可视化编辑状态
  const [schema, setSchema] = useState<ConfigSchema | null>(null);
  const [editedValues, setEditedValues] = useState<
    Record<string, Record<string, unknown>>
  >({});
  const [originalValues, setOriginalValues] = useState<
    Record<string, Record<string, unknown>>
  >({});
  const [isSavingSchema, setIsSavingSchema] = useState(false);

  // UI 状态
  const [searchText, setSearchText] = useState("");
  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set()
  );
  const [activeSection, setActiveSection] = useState("");
  const [showSaveDialog, setShowSaveDialog] = useState(false);
  const [updateRepoUrl, setUpdateRepoUrl] = useState("https://github.com/Prejudice-Studio/Twilight.git");
  const [updateBranch, setUpdateBranch] = useState("main");
  const [updateRestartServices, setUpdateRestartServices] = useState(true);
  const [isUpdating, setIsUpdating] = useState(false);
  const [updateOutput, setUpdateOutput] = useState<string[]>([]);
  const [configBackups, setConfigBackups] = useState<ConfigBackup[]>([]);
  const [isLoadingConfigBackups, setIsLoadingConfigBackups] = useState(false);
  const [isConfigBackupBusy, setIsConfigBackupBusy] = useState(false);
  const [configBackupView, setConfigBackupView] = useState<ConfigBackupView | null>(null);
  const [configRestorePreview, setConfigRestorePreview] = useState<ConfigRestoreResult | null>(null);
  const [showConfigBackupView, setShowConfigBackupView] = useState(false);
  const [showConfigRestoreDialog, setShowConfigRestoreDialog] = useState(false);
  const scrollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const serverIconInputRef = useRef<HTMLInputElement | null>(null);
  const [isUploadingServerIcon, setIsUploadingServerIcon] = useState(false);

  // 初始化时展开所有 sections
  useEffect(() => {
    if (!schema) {
      return;
    }

    setExpandedSections(new Set(schema.sections.map((s) => s.key)));
    setActiveSection((current) => current || schema.sections[0]?.key || "");
  }, [schema]);

  const hasSchemaChanges = useMemo(() => {
    return JSON.stringify(editedValues) !== JSON.stringify(originalValues);
  }, [editedValues, originalValues]);

  // 每个 section 的修改计数
  const changedCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    if (!schema) return counts;
    for (const section of schema.sections) {
      let count = 0;
      for (const field of section.fields) {
        const edited = editedValues[section.key]?.[field.key];
        const orig = originalValues[section.key]?.[field.key];
        if (isConfigValueChanged(edited, orig)) count++;
      }
      counts[section.key] = count;
    }
    return counts;
  }, [schema, editedValues, originalValues]);

  const totalChangedCount = useMemo(
    () => Object.values(changedCounts).reduce((a, b) => a + b, 0),
    [changedCounts]
  );

  const configChanges = useMemo<ConfigChangeItem[]>(() => {
    if (!schema) return [];
    const changes: ConfigChangeItem[] = [];
    for (const section of schema.sections) {
      for (const field of section.fields) {
        const edited = editedValues[section.key]?.[field.key];
        const orig = originalValues[section.key]?.[field.key];
        if (!isConfigValueChanged(edited, orig)) continue;
        changes.push({
          sectionKey: section.key,
          sectionTitle: section.title,
          field,
          before: orig,
          after: edited,
        });
      }
    }
    return changes;
  }, [schema, editedValues, originalValues]);

  const currentServerIcon = String(editedValues.Global?.server_icon ?? "").trim();
  const serverIconPreviewUrl =
    currentServerIcon && /^https:\/\/[^\s"'<>]+$/i.test(currentServerIcon)
      ? currentServerIcon
      : `/api/v1/system/server-icon?ts=${encodeURIComponent(currentServerIcon || "default")}`;

  // 搜索匹配
  const matchedFieldsBySection = useMemo(() => {
    const result: Record<string, Set<string>> = {};
    if (!schema || !searchText.trim()) return result;
    const q = searchText.toLowerCase();
    for (const section of schema.sections) {
      const matched = new Set<string>();
      for (const field of section.fields) {
        if (
          field.label.toLowerCase().includes(q) ||
          field.key.toLowerCase().includes(q) ||
          field.description.toLowerCase().includes(q)
        ) {
          matched.add(field.key);
        }
      }
      if (matched.size > 0) {
        result[section.key] = matched;
      }
    }
    return result;
  }, [schema, searchText]);

  const searchResultCount = useMemo(
    () =>
      Object.values(matchedFieldsBySection).reduce((a, s) => a + s.size, 0),
    [matchedFieldsBySection]
  );

  // 搜索时自动展开匹配的 section
  useEffect(() => {
    if (searchText.trim() && Object.keys(matchedFieldsBySection).length > 0) {
      setExpandedSections(new Set(Object.keys(matchedFieldsBySection)));
    }
  }, [matchedFieldsBySection, searchText]);

  useEffect(() => {
    setHasChanges(configContent !== originalContent);
  }, [configContent, originalContent]);

  useEffect(() => {
    return () => {
      if (scrollTimerRef.current) {
        clearTimeout(scrollTimerRef.current);
      }
    };
  }, []);

  // 加载源文件
  const loadConfigResource = useCallback(async () => {
    const res = await api.getConfigToml();
    if (res.success && res.data) {
      setConfigContent(res.data.content);
      setOriginalContent(res.data.content);
      setConfigPath(res.data.path);
    } else {
      throw new Error(res.message || t("adminConfig.loadTomlError"));
    }
    return true;
  }, [t]);

  // 加载结构化配置
  const loadSchemaResource = useCallback(async () => {
    const res = await api.getConfigSchema();
    if (res.success && res.data) {
      setSchema(res.data);
      const initial: Record<string, Record<string, unknown>> = {};
      for (const section of res.data.sections) {
        initial[section.key] = {};
        for (const field of section.fields) {
          initial[section.key][field.key] =
            field.type === "list" ? toEditorList(field.value) : field.value;
        }
      }
      setEditedValues(JSON.parse(JSON.stringify(initial)));
      setOriginalValues(JSON.parse(JSON.stringify(initial)));
    } else {
      throw new Error(res.message || t("adminConfig.loadSchemaError"));
    }
    return true;
  }, [t]);

  const {
    isLoading: isLoadingToml,
    error: tomlError,
    execute: loadConfig,
  } = useAsyncResource(loadConfigResource, { immediate: false });

  const {
    isLoading: isLoadingSchema,
    error: schemaError,
    execute: loadSchema,
  } = useAsyncResource(loadSchemaResource, { immediate: true });

  const handleFieldChange = (
    sectionKey: string,
    fieldKey: string,
    value: unknown
  ) => {
    setEditedValues((prev) => ({
      ...prev,
      [sectionKey]: {
        ...prev[sectionKey],
        [fieldKey]: value,
      },
    }));
  };

  const handleResetField = (sectionKey: string, fieldKey: string) => {
    const origVal = originalValues[sectionKey]?.[fieldKey];
    if (origVal !== undefined) {
      setEditedValues((prev) => ({
        ...prev,
        [sectionKey]: {
          ...prev[sectionKey],
          [fieldKey]: JSON.parse(JSON.stringify(origVal)),
        },
      }));
    }
  };

  const handleResetAll = () => {
    setEditedValues(JSON.parse(JSON.stringify(originalValues)));
  };

  const expandAllSections = () => {
    setExpandedSections(new Set(schema?.sections.map((section) => section.key) ?? []));
  };

  const collapseAllSections = () => {
    setExpandedSections(new Set());
  };

  const toggleSection = (key: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const scrollToSection = (key: string) => {
    setActiveSection(key);
    if (!expandedSections.has(key)) {
      setExpandedSections((prev) => new Set(prev).add(key));
    }
    if (scrollTimerRef.current) {
      clearTimeout(scrollTimerRef.current);
    }
    scrollTimerRef.current = setTimeout(() => {
      document
        .getElementById(`section-${key}`)
        ?.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 50);
  };

  // 保存可视化配置
  const handleSaveSchema = async () => {
    setShowSaveDialog(false);
    if (!hasSchemaChanges) {
      toast({ title: t("adminConfig.noChangesTitle"), description: t("adminConfig.noSchemaChangesDescription") });
      return;
    }

    setIsSavingSchema(true);
    try {
      const sectionsPayload: Record<string, Record<string, unknown>> = {};
      for (const section of schema?.sections ?? []) {
        sectionsPayload[section.key] = {};
        for (const field of section.fields) {
          const edited = editedValues[section.key]?.[field.key];
          const original = originalValues[section.key]?.[field.key];
          sectionsPayload[section.key][field.key] = serializeFieldValue(
            field,
            edited,
            original
          );
        }
      }

      const res = await api.updateConfigBySchema(sectionsPayload);
      if (res.success) {
        setOriginalValues(JSON.parse(JSON.stringify(editedValues)));
        await loadSchema();
        if (configContent) {
          await loadConfig();
        }
        // 配置保存后强制刷新 systemInfo：本次修改可能影响 server_icon 等公开字段。
        await fetchSystemInfo(true);
        toast({
          title: t("adminConfig.saveSuccessTitle"),
          description: t("adminConfig.saveSuccessDescription"),
          variant: "success",
        });
      } else {
        toast({
          title: t("adminConfig.saveFailureTitle"),
          description: res.message || t("adminConfig.saveSchemaFailureDescription"),
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: t("adminConfig.saveFailureTitle"),
        description: error.message || t("common.checkNetwork"),
        variant: "destructive",
      });
    } finally {
      setIsSavingSchema(false);
    }
  };

  // 保存源文件
  const handleSaveToml = async () => {
    if (!hasChanges) {
      toast({ title: t("adminConfig.noChangesTitle"), description: t("adminConfig.noTomlChangesDescription") });
      return;
    }

    setIsSaving(true);
    try {
      const res = await api.updateConfigToml(configContent);
      if (res.success) {
        setOriginalContent(configContent);
        setHasChanges(false);
        await loadSchema();
        // 同 schema 路径：源文件保存可能改了 server_icon 等公开字段，systemInfo 缓存必须刷新一次。
        await fetchSystemInfo(true);
        toast({
          title: t("adminConfig.saveSuccessTitle"),
          description: t("adminConfig.saveSuccessDescription"),
          variant: "success",
        });
      } else {
        toast({
          title: t("adminConfig.saveFailureTitle"),
          description: res.message || t("adminConfig.saveTomlFailureDescription"),
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: t("adminConfig.saveFailureTitle"),
        description: error.message || t("common.checkNetwork"),
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const handleServerIconFile = async (file?: File | null) => {
    if (!file) return;
    if (hasSchemaChanges || hasChanges) {
      toast({
        title: t("adminConfig.unsavedConfigTitle"),
        description: t("adminConfig.serverIconImmediateSaveWarning"),
        variant: "destructive",
      });
      return;
    }
    setIsUploadingServerIcon(true);
    try {
      const res = await api.uploadServerIcon(file);
      if (!res.success || !res.data) {
        throw new Error(res.message || t("adminConfig.serverIconUploadFailed"));
      }
      await loadSchema();
      if (configContent) {
        await loadConfig();
      }
      await fetchSystemInfo(true);
      toast({
        title: t("adminConfig.serverIconUploadSuccessTitle"),
        description: t("adminConfig.serverIconUploadSuccessDescription"),
        variant: "success",
      });
    } catch (error: any) {
      toast({
        title: t("adminConfig.serverIconUploadFailureTitle"),
        description: error.message || t("adminConfig.serverIconUploadFailureDescription"),
        variant: "destructive",
      });
    } finally {
      setIsUploadingServerIcon(false);
      if (serverIconInputRef.current) {
        serverIconInputRef.current.value = "";
      }
    }
  };

  const loadConfigBackups = useCallback(async () => {
    setIsLoadingConfigBackups(true);
    try {
      const res = await api.listConfigBackups();
      if (res.success && res.data) {
        setConfigBackups(res.data.backups || []);
      }
    } catch (error: any) {
      toast({
        title: t("adminConfig.loadBackupsFailedTitle"),
        description: error.message || t("common.checkBackendConnection"),
        variant: "destructive",
      });
    } finally {
      setIsLoadingConfigBackups(false);
    }
  }, [t, toast]);

  const handleCreateConfigBackup = async () => {
    setIsConfigBackupBusy(true);
    try {
      const res = await api.createConfigBackup();
      if (res.success) {
        toast({ title: t("adminConfig.backupCreatedTitle"), description: res.data?.backup?.name, variant: "success" });
        await loadConfigBackups();
      } else {
        toast({ title: t("adminConfig.backupCreateFailedTitle"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminConfig.backupCreateFailedTitle"), description: error.message || t("common.checkBackendLogs"), variant: "destructive" });
    } finally {
      setIsConfigBackupBusy(false);
    }
  };

  const handleViewConfigBackup = async (backup: ConfigBackup) => {
    setIsConfigBackupBusy(true);
    try {
      const res = await api.getConfigBackup(backup.name);
      if (res.success && res.data) {
        setConfigBackupView(res.data);
        setShowConfigBackupView(true);
      } else {
        toast({ title: t("adminConfig.backupReadFailedTitle"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminConfig.backupReadFailedTitle"), description: error.message || t("common.checkBackendLogs"), variant: "destructive" });
    } finally {
      setIsConfigBackupBusy(false);
    }
  };

  const handlePreviewConfigRestore = async (backup: ConfigBackup) => {
    setIsConfigBackupBusy(true);
    try {
      const res = await api.restoreConfigBackup(backup.name, { dry_run: true });
      if (res.success && res.data) {
        setConfigRestorePreview(res.data);
        setShowConfigRestoreDialog(true);
      } else {
        toast({ title: t("adminConfig.backupRestorePreviewFailedTitle"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminConfig.backupRestorePreviewFailedTitle"), description: error.message || t("common.checkBackendLogs"), variant: "destructive" });
    } finally {
      setIsConfigBackupBusy(false);
    }
  };

  const handleConfirmConfigRestore = async () => {
    if (!configRestorePreview?.restored) return;
    setIsConfigBackupBusy(true);
    try {
      const res = await api.restoreConfigBackup(configRestorePreview.restored, {
        confirm: configRestorePreview.confirm || CONFIG_RESTORE_CONFIRM,
      });
      if (res.success && res.data) {
        setConfigRestorePreview(res.data);
        setShowConfigRestoreDialog(false);
        toast({
          title: t("adminConfig.backupRestoredTitle"),
          description: res.data.pre_operation_backup
            ? t("adminConfig.preOperationBackup", { name: res.data.pre_operation_backup.name })
            : t("adminConfig.preOperationBackupCreated"),
          variant: "success",
        });
        await loadConfigBackups();
        await loadSchema();
        if (configContent) {
          await loadConfig();
        }
      } else {
        toast({ title: t("adminConfig.backupRestoreFailedTitle"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminConfig.backupRestoreFailedTitle"), description: error.message || t("common.checkBackendLogs"), variant: "destructive" });
    } finally {
      setIsConfigBackupBusy(false);
    }
  };

  const handleDeleteConfigBackup = async (backup: ConfigBackup) => {
    const accepted = await confirm({
      title: t("adminConfig.deleteBackupTitle"),
      description: t("adminConfig.deleteBackupDescription", { name: backup.name }),
      tone: "danger",
      confirmLabel: t("adminConfig.deleteBackupConfirmLabel"),
      confirmVariant: "destructive",
    });
    if (!accepted) return;
    setIsConfigBackupBusy(true);
    try {
      const res = await api.deleteConfigBackup(backup.name);
      if (res.success) {
        toast({ title: t("adminConfig.backupDeletedTitle"), description: backup.name, variant: "success" });
        await loadConfigBackups();
      } else {
        toast({ title: t("adminConfig.deleteBackupFailedTitle"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("adminConfig.deleteBackupFailedTitle"), description: error.message || t("common.checkBackendLogs"), variant: "destructive" });
    } finally {
      setIsConfigBackupBusy(false);
    }
  };

  const handleGitUpdate = async (dryRun = false) => {
    setIsUpdating(true);
    setUpdateOutput([]);
    try {
      const res = await api.updateFromGit({
        repo_url: updateRepoUrl.trim(),
        branch: updateBranch.trim() || "main",
        restart_services: updateRestartServices,
        dry_run: dryRun,
      });
      if (res.success) {
        const before = res.data?.before;
        const after = res.data?.after;
        const summary = [
          `mode=${dryRun ? "preflight" : "update"}`,
          before ? `before=${before.branch}@${before.commit.slice(0, 12)} dirty=${before.dirty_count}` : "",
          after ? `after=${after.branch}@${after.commit.slice(0, 12)} dirty=${after.dirty_count}` : "",
          res.data?.repo_url ? `repo=${res.data.repo_url}` : "",
        ].filter(Boolean);
        const logs = (res.data?.results || []).map((item) => {
          const output = [item.stdout, item.stderr].filter(Boolean).join("\n").trim();
          return `$ ${item.command}\nexit=${item.returncode} duration=${item.duration_ms}ms${output ? `\n${output}` : ""}`;
        });
        setUpdateOutput([...summary, ...logs]);
        toast({
          title: dryRun ? t("adminConfig.preflightPassedTitle") : t("adminConfig.updateCompleteTitle"),
          description: res.message || t("adminConfig.updateCompleteDescription"),
          variant: "success",
        });
      } else {
        const dirty = res.data?.before?.dirty_files?.length
          ? [`dirty files (${res.data.before.dirty_count}):`, ...res.data.before.dirty_files]
          : [];
        setUpdateOutput([
          ...dirty,
          ...(res.data?.results || []).map((item) => `$ ${item.command}\n${item.stderr || item.stdout}`),
        ]);
        toast({
          title: t("adminConfig.updateFailedTitle"),
          description: res.message || t("adminConfig.checkCommandOutput"),
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: t("adminConfig.updateFailedTitle"),
        description: error.message || t("common.checkNetwork"),
        variant: "destructive",
      });
    } finally {
      setIsUpdating(false);
    }
  };

  if (schemaError && tomlError) {
    return (
      <PageError
        message={schemaError || tomlError}
        onRetry={() => void loadSchema()}
      />
    );
  }

  return (
    <TooltipProvider delayDuration={300}>
      <motion.div
        variants={container}
        initial="hidden"
        animate="show"
        className="space-y-6"
      >
        <div>
          <h1 className="text-3xl font-bold">{t("adminConfig.pageTitle")}</h1>
          <p className="text-muted-foreground">
            {t("adminConfig.pageDescription")}
          </p>
        </div>

        <Tabs
          className="min-w-0"
          defaultValue="visual"
          onValueChange={(v) => {
            if (v === "toml" && !configContent) {
              void loadConfig();
            }
            if (v === "config-backups" && configBackups.length === 0) {
              void loadConfigBackups();
            }
          }}
        >
          <div className="min-w-0 pb-1">
            <TabsList className="grid h-auto w-full grid-cols-2 gap-1 sm:inline-flex sm:h-10 sm:w-auto sm:grid-cols-none">
              <TabsTrigger value="visual" className="min-w-0 gap-1.5 px-2 sm:px-4">
                <SlidersHorizontal className="h-4 w-4" />
                {t("adminConfig.tabVisual")}
              </TabsTrigger>
              <TabsTrigger value="toml" className="min-w-0 gap-1.5 px-2 sm:px-4">
                <FileText className="h-4 w-4" />
                {t("adminConfig.tabToml")}
              </TabsTrigger>
              <TabsTrigger value="config-backups" className="min-w-0 gap-1.5 px-2 sm:px-4">
                <Archive className="h-4 w-4" />
                {t("adminConfig.tabBackups")}
              </TabsTrigger>
              <TabsTrigger value="update" className="min-w-0 gap-1.5 px-2 sm:px-4">
                <GitPullRequest className="h-4 w-4" />
                {t("adminConfig.tabUpdate")}
              </TabsTrigger>
            </TabsList>
          </div>

          {/* ==================== 可视化编辑 ==================== */}
          <TabsContent value="visual" className="mt-4">
            <Card className="mb-4">
              <CardHeader className="pb-3">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0">
                    <CardTitle className="flex items-center gap-2 text-base">
                      <ImageIcon className="h-5 w-5" />
                      {t("adminConfig.serverIconTitle")}
                    </CardTitle>
                    <CardDescription>
                      {t("adminConfig.serverIconDescription")}
                    </CardDescription>
                  </div>
                  <div className="flex items-center gap-3">
                    <span
                      aria-hidden="true"
                      className="h-12 w-12 rounded-md border bg-muted bg-contain bg-center bg-no-repeat"
                      style={{ backgroundImage: `url("${serverIconPreviewUrl}")` }}
                    />
                    <input
                      ref={serverIconInputRef}
                      type="file"
                      accept="image/png,image/jpeg,image/gif,image/webp,image/bmp"
                      className="hidden"
                      onChange={(event) => void handleServerIconFile(event.target.files?.[0])}
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => serverIconInputRef.current?.click()}
                      disabled={isUploadingServerIcon || hasSchemaChanges || hasChanges}
                    >
                      {isUploadingServerIcon ? (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      ) : (
                        <Upload className="mr-2 h-4 w-4" />
                      )}
                      {t("adminConfig.uploadIcon")}
                    </Button>
                  </div>
                </div>
              </CardHeader>
            </Card>
            {configChanges.length > 0 && (
              <Card className="mb-4 border-amber-300/70 bg-amber-50/60 dark:border-amber-800/70 dark:bg-amber-950/20">
                <CardHeader className="pb-3">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0">
                      <CardTitle className="flex items-center gap-2 text-base">
                        <AlertTriangle className="h-5 w-5 text-amber-600" />
                        {t("adminConfig.pendingChangesTitle")}
                        <Badge variant="warning" className="text-[10px]">
                          {t("adminConfig.pendingChangesBadge", { count: configChanges.length })}
                        </Badge>
                      </CardTitle>
                      <CardDescription>
                        {t("adminConfig.pendingChangesDescription")}
                      </CardDescription>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button variant="outline" size="sm" onClick={handleResetAll} disabled={isSavingSchema}>
                        <RotateCcw className="mr-1.5 h-3.5 w-3.5" />
                        {t("common.resetAll")}
                      </Button>
                      <Button size="sm" onClick={() => setShowSaveDialog(true)} disabled={isSavingSchema}>
                        <Save className="mr-1.5 h-3.5 w-3.5" />
                        {t("adminConfig.saveConfig")}
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="grid gap-2 lg:grid-cols-2">
                    {configChanges.slice(0, 6).map((change) => (
                      <div
                        key={`${change.sectionKey}:${change.field.key}`}
                        className="rounded-lg border bg-background/70 p-3 text-sm"
                      >
                        <div className="flex min-w-0 items-center justify-between gap-3">
                          <div className="min-w-0">
                            <p className="truncate font-medium">
                              {change.sectionTitle} / {change.field.label}
                            </p>
                            <code className="text-[11px] text-muted-foreground">{change.field.key}</code>
                          </div>
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="h-7 shrink-0 px-2 text-xs"
                            onClick={() => scrollToSection(change.sectionKey)}
                          >
                            {t("adminConfig.locate")}
                          </Button>
                        </div>
                        <div className="mt-2 grid gap-2 text-xs sm:grid-cols-[1fr_auto_1fr] sm:items-center">
                          <span className="min-w-0 rounded bg-muted/70 px-2 py-1 text-muted-foreground">
                            {formatConfigPreviewValue(change.field, change.before, t)}
                          </span>
                          <span className="hidden text-muted-foreground sm:inline">→</span>
                          <span className="min-w-0 rounded bg-primary/10 px-2 py-1 text-primary">
                            {formatConfigPreviewValue(change.field, change.after, t)}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                  {configChanges.length > 6 && (
                    <p className="text-xs text-muted-foreground">
                      {t("adminConfig.remainingChanges", { count: configChanges.length - 6 })}
                    </p>
                  )}
                </CardContent>
              </Card>
            )}
            {/* 搜索与操作栏 */}
            <div className="mb-4 flex min-w-0 flex-col items-stretch gap-3 sm:flex-row sm:items-center">
              <div className="relative w-full min-w-0 sm:max-w-md sm:flex-1">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder={t("adminConfig.searchPlaceholder")}
                  value={searchText}
                  onChange={(e) => setSearchText(e.target.value)}
                  className="pl-9 pr-8"
                />
                {searchText && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    onClick={() => setSearchText("")}
                    className="absolute right-0 top-0 h-full px-2 hover:bg-transparent"
                  >
                    <X className="h-3.5 w-3.5 text-muted-foreground" />
                  </Button>
                )}
              </div>
              {searchText && (
                <span className="text-xs text-muted-foreground whitespace-nowrap">
                  {t("adminConfig.searchResultCount", { count: searchResultCount })}
                </span>
              )}
              <div className="ml-auto flex w-full flex-wrap gap-2 sm:w-auto">
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1 sm:flex-none"
                  onClick={() => void loadSchema()}
                  disabled={isLoadingSchema || isSavingSchema}
                >
                  {isLoadingSchema ? (
                    <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <RotateCcw className="mr-1.5 h-3.5 w-3.5" />
                  )}
                  {t("common.refresh")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1 sm:flex-none"
                  onClick={expandAllSections}
                  disabled={isLoadingSchema || !schema}
                >
                  <ChevronDown className="mr-1.5 h-3.5 w-3.5" />
                  {t("adminConfig.expandAll")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1 sm:flex-none"
                  onClick={collapseAllSections}
                  disabled={isLoadingSchema || !schema}
                >
                  <ChevronRight className="mr-1.5 h-3.5 w-3.5" />
                  {t("adminConfig.collapseAll")}
                </Button>
                {hasSchemaChanges && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleResetAll}
                    className="flex-1 text-muted-foreground sm:flex-none"
                  >
                    <RotateCcw className="mr-1.5 h-3.5 w-3.5" />
                    {t("common.resetAll")}
                  </Button>
                )}
                <Button
                  size="sm"
                  className="flex-1 sm:flex-none"
                  onClick={() => setShowSaveDialog(true)}
                  disabled={
                    isLoadingSchema || isSavingSchema || !hasSchemaChanges
                  }
                >
                  {isSavingSchema ? (
                    <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Save className="mr-1.5 h-3.5 w-3.5" />
                  )}
                  {t("adminConfig.saveConfig")}
                  {totalChangedCount > 0 && (
                    <Badge
                      variant="secondary"
                      className="ml-1.5 text-[10px] px-1.5 py-0 h-4"
                    >
                      {totalChangedCount}
                    </Badge>
                  )}
                </Button>
              </div>
            </div>

            {/* 主内容区（侧边栏 + 配置列表） */}
            <div className="flex min-w-0 gap-6">
              {/* 侧边导航 */}
              {schema && !searchText && (
                <SectionNav
                  sections={schema.sections}
                  categories={schema.categories ?? [FALLBACK_CATEGORY]}
                  activeSection={activeSection}
                  changedCounts={changedCounts}
                  onSelect={scrollToSection}
                />
              )}

              {/* 配置区域 */}
              <div className="flex-1 min-w-0 space-y-4">
                {isLoadingSchema ? (
                  <div className="flex items-center justify-center h-96">
                    <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                  </div>
                ) : (
                  <>
                    {(() => {
                      if (!schema) return null;
                      const categories =
                        schema.categories && schema.categories.length > 0
                          ? schema.categories
                          : [FALLBACK_CATEGORY];
                      // 把 sections 按 category 排序后渲染；搜索态下隐藏分组标题，避免视觉干扰
                      const buckets = new Map<string, ConfigSection[]>();
                      const orderedKeys: string[] = [];
                      for (const section of schema.sections) {
                        const key =
                          section.category &&
                          categories.some((c) => c.key === section.category)
                            ? section.category
                            : FALLBACK_CATEGORY.key;
                        if (!buckets.has(key)) {
                          buckets.set(key, []);
                          orderedKeys.push(key);
                        }
                        buckets.get(key)!.push(section);
                      }
                      const renderSection = (section: ConfigSection) => (
                        <SectionCard
                          key={section.key}
                          section={section}
                          values={editedValues[section.key] ?? {}}
                          originalValues={originalValues[section.key] ?? {}}
                          changedCount={changedCounts[section.key] || 0}
                          onFieldChange={handleFieldChange}
                          onResetField={handleResetField}
                          searchText={searchText}
                          matchedFieldKeys={
                            matchedFieldsBySection[section.key] ?? new Set()
                          }
                          isExpanded={expandedSections.has(section.key)}
                          onToggle={() => toggleSection(section.key)}
                        />
                      );

                      // 搜索态：扁平渲染，不显示分组标题
                      if (searchText) {
                        return schema.sections.map(renderSection);
                      }

                      return orderedKeys.map((key) => {
                        const groupSections = buckets.get(key) || [];
                        if (groupSections.length === 0) return null;
                        const category =
                          categories.find((c) => c.key === key) ||
                          (key === FALLBACK_CATEGORY.key ? FALLBACK_CATEGORY : null);
                        return (
                          <div key={key} className="space-y-4">
                            {category && key !== FALLBACK_CATEGORY.key && (
                              <div className="flex items-center gap-2 pt-2">
                                <div className="h-px flex-1 bg-border" />
                                <span className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                                  {category.title}
                                </span>
                                <div className="h-px flex-1 bg-border" />
                              </div>
                            )}
                            {groupSections.map(renderSection)}
                          </div>
                        );
                      });
                    })()}
                    {searchText &&
                      Object.keys(matchedFieldsBySection).length === 0 && (
                        <div className="py-16 text-center text-muted-foreground">
                          <Search className="h-10 w-10 mx-auto mb-3 opacity-30" />
                          <p>{t("adminConfig.noMatchesTitle")}</p>
                          <p className="text-xs mt-1">
                            {t("adminConfig.noMatchesDescription")}
                          </p>
                        </div>
                      )}
                  </>
                )}
              </div>
            </div>

            {/* 底部浮动保存栏 */}
            <AnimatePresence>
              {hasSchemaChanges && !isSavingSchema && (
                <motion.div
                  initial={{ y: 60, opacity: 0 }}
                  animate={{ y: 0, opacity: 1 }}
                  exit={{ y: 60, opacity: 0 }}
                  transition={{ type: "spring", stiffness: 400, damping: 30 }}
                  className="fixed inset-x-3 bottom-4 z-50 sm:inset-x-auto sm:bottom-6 sm:left-1/2 sm:-translate-x-1/2"
                >
                  <div className="flex flex-wrap items-center justify-center gap-2 rounded-2xl border bg-background/95 px-3 py-2.5 shadow-lg backdrop-blur sm:flex-nowrap sm:gap-3 sm:rounded-full sm:px-5">
                    <AlertTriangle className="h-4 w-4 text-amber-500 shrink-0" />
                    <span className="min-w-0 text-sm">
                      {t("adminConfig.floatingChanged", { count: totalChangedCount })}
                    </span>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={handleResetAll}
                      className="h-7 text-xs"
                    >
                      {t("common.reset")}
                    </Button>
                    <Button
                      size="sm"
                      onClick={() => setShowSaveDialog(true)}
                      className="h-7 text-xs"
                    >
                      <Save className="mr-1 h-3 w-3" />
                      {t("common.save")}
                    </Button>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </TabsContent>

          {/* ==================== 源文件编辑 ==================== */}
          <TabsContent value="toml" className="mt-4">
            <motion.div variants={item}>
              <Card>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <div>
                      <CardTitle className="flex items-center gap-2">
                        <Settings className="h-5 w-5" />
                        {t("adminConfig.tomlTitle")}
                      </CardTitle>
                      <CardDescription>
                        {t("adminConfig.tomlDescription")}
                      </CardDescription>
                    </div>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => void loadConfig()}
                        disabled={isLoadingToml || isSaving}
                      >
                        {isLoadingToml ? (
                          <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <RotateCcw className="mr-1.5 h-3.5 w-3.5" />
                        )}
                        {t("common.reload")}
                      </Button>
                      <Button
                        size="sm"
                        onClick={handleSaveToml}
                        disabled={isLoadingToml || isSaving || !hasChanges}
                      >
                        {isSaving ? (
                          <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <Save className="mr-1.5 h-3.5 w-3.5" />
                        )}
                        {t("common.save")}
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent>
                  {isLoadingToml ? (
                    <div className="flex items-center justify-center h-96">
                      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                    </div>
                  ) : (
                    <div className="space-y-3">
                      {configPath && (
                        <div className="flex items-center gap-2 text-xs text-muted-foreground bg-muted/50 rounded-md px-3 py-2">
                          <FileText className="h-3.5 w-3.5 shrink-0" />
                          <span className="truncate">{configPath}</span>
                          <span className="ml-auto shrink-0">
                            {t("adminConfig.tomlAutoBackup")}
                          </span>
                        </div>
                      )}
                      {hasChanges && (
                        <Alert>
                          <AlertTriangle className="h-4 w-4" />
                          <AlertDescription>
                            {t("adminConfig.tomlUnsavedAlert")}
                          </AlertDescription>
                        </Alert>
                      )}
                      <Textarea
                        value={configContent}
                        onChange={(e) => setConfigContent(e.target.value)}
                        className="font-mono text-sm min-h-[600px] leading-relaxed"
                        placeholder={t("adminConfig.tomlPlaceholder")}
                      />
                      <div className="flex items-center justify-between text-xs text-muted-foreground px-1">
                        <span>
                          {t("adminConfig.tomlStats", { lines: configContent.split("\n").length, chars: configContent.length })}
                        </span>
                        <span>{t("adminConfig.tomlFormat")}</span>
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            </motion.div>
          </TabsContent>

          <TabsContent value="config-backups" className="mt-4">
            <div className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h2 className="flex items-center gap-2 text-lg font-semibold">
                    <Archive className="h-5 w-5" />
                    {t("adminConfig.backupsTitle")}
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    {t("adminConfig.backupsDescription")}
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button variant="outline" size="sm" onClick={() => void loadConfigBackups()} disabled={isLoadingConfigBackups || isConfigBackupBusy}>
                    {isLoadingConfigBackups ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RotateCcw className="mr-2 h-4 w-4" />}
                    {t("common.refresh")}
                  </Button>
                  <Button size="sm" onClick={() => void handleCreateConfigBackup()} disabled={isConfigBackupBusy}>
                    {isConfigBackupBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Archive className="mr-2 h-4 w-4" />}
                    {t("adminConfig.createBackup")}
                  </Button>
                </div>
              </div>

              <Alert>
                <Shield className="h-4 w-4" />
                <AlertTitle>{t("adminConfig.safetyMechanismTitle")}</AlertTitle>
                <AlertDescription>
                  {t("adminConfig.safetyMechanismDescription")}
                </AlertDescription>
              </Alert>

              <Card>
                <CardHeader>
                  <CardTitle>{t("adminConfig.backupListTitle")}</CardTitle>
                  <CardDescription>{t("adminConfig.backupListDescription")}</CardDescription>
                </CardHeader>
                <CardContent>
                  {configBackups.length === 0 ? (
                    <div className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
                      {isLoadingConfigBackups ? t("adminConfig.loadingBackups") : t("adminConfig.noBackups")}
                    </div>
                  ) : (
                    <div className="divide-y rounded-md border">
                      {configBackups.map((backup) => (
                        <div key={backup.name} className="flex flex-col gap-3 px-3 py-3 md:flex-row md:items-center md:justify-between">
                          <div className="min-w-0">
                            <p className="break-all text-sm font-medium">{backup.name}</p>
                            <p className="text-xs text-muted-foreground">
                              {formatBytes(backup.size)} · {formatUnixTime(backup.created_at)}
                            </p>
                          </div>
                          <div className="flex flex-wrap gap-2">
                            <Button variant="outline" size="sm" onClick={() => void handleViewConfigBackup(backup)} disabled={isConfigBackupBusy}>
                              <Eye className="mr-2 h-4 w-4" />{t("common.view")}
                            </Button>
                            <Button variant="outline" size="sm" onClick={() => void handlePreviewConfigRestore(backup)} disabled={isConfigBackupBusy}>
                              <RotateCcw className="mr-2 h-4 w-4" />{t("common.restore")}
                            </Button>
                            <Button variant="outline" size="sm" onClick={() => void handleDeleteConfigBackup(backup)} disabled={isConfigBackupBusy} className="text-destructive hover:text-destructive">
                              <Trash2 className="mr-2 h-4 w-4" />{t("common.delete")}
                            </Button>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </CardContent>
              </Card>
            </div>
          </TabsContent>

          <TabsContent value="update" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <GithubIcon className="h-5 w-5" />
                  {t("adminConfig.gitUpdateTitle")}
                </CardTitle>
                <CardDescription>
                  {t("adminConfig.gitUpdateDescription")}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-5">
                <Alert>
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle>{t("adminConfig.productionOperationTitle")}</AlertTitle>
                  <AlertDescription>
                    {t("adminConfig.productionOperationDescription")}
                  </AlertDescription>
                </Alert>

                <div className="grid gap-4 md:grid-cols-[1fr_180px]">
                  <div className="space-y-2">
                    <Label htmlFor="update-repo-url">{t("adminConfig.repoUrlLabel")}</Label>
                    <Input
                      id="update-repo-url"
                      value={updateRepoUrl}
                      onChange={(e) => setUpdateRepoUrl(e.target.value)}
                      placeholder="https://github.com/Prejudice-Studio/Twilight.git"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="update-branch">{t("adminConfig.branchLabel")}</Label>
                    <Input
                      id="update-branch"
                      value={updateBranch}
                      onChange={(e) => setUpdateBranch(e.target.value)}
                      placeholder="main"
                    />
                  </div>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <label className="flex items-center justify-between rounded-lg border p-3">
                    <span>
                      <span className="block text-sm font-medium">{t("adminConfig.autoRestartServices")}</span>
                      <span className="text-xs text-muted-foreground">{t("adminConfig.autoRestartDescription")}</span>
                    </span>
                    <Switch checked={updateRestartServices} onCheckedChange={setUpdateRestartServices} />
                  </label>
                </div>

                <div className="flex flex-wrap gap-2">
                  <Button variant="outline" onClick={() => void handleGitUpdate(true)} disabled={isUpdating || !updateRepoUrl.trim()}>
                    {isUpdating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Shield className="mr-2 h-4 w-4" />}
                    {t("adminConfig.safePreflight")}
                  </Button>
                  <Button onClick={() => void handleGitUpdate(false)} disabled={isUpdating || !updateRepoUrl.trim()}>
                    {isUpdating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <GitPullRequest className="mr-2 h-4 w-4" />}
                    {t("adminConfig.pullAndUpdate")}
                  </Button>
                  <GithubProjectLink compact className="w-full sm:w-auto" />
                </div>

                {updateOutput.length > 0 && (
                  <pre className="max-h-96 overflow-auto rounded-lg border bg-muted/50 p-4 text-xs whitespace-pre-wrap">
                    {updateOutput.join("\n\n")}
                  </pre>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>

        <Dialog open={showConfigBackupView} onOpenChange={setShowConfigBackupView}>
          <DialogContent className="max-h-[85vh] max-w-3xl overflow-hidden p-0">
            <DialogHeader className="border-b p-4">
              <DialogTitle>{t("adminConfig.viewBackupTitle")}</DialogTitle>
              <DialogDescription>
                {configBackupView?.backup.name} · {formatBytes(configBackupView?.backup.size || 0)}
              </DialogDescription>
            </DialogHeader>
            <pre className="max-h-[65vh] overflow-auto p-4 text-xs leading-relaxed whitespace-pre-wrap">
              {configBackupView?.content || ""}
            </pre>
            <DialogFooter className="border-t p-4">
              <Button variant="outline" onClick={() => setShowConfigBackupView(false)}>{t("common.close")}</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={showConfigRestoreDialog} onOpenChange={setShowConfigRestoreDialog}>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>{t("adminConfig.restoreDialogTitle")}</DialogTitle>
              <DialogDescription>
                {t("adminConfig.restoreDialogDescription")}
              </DialogDescription>
            </DialogHeader>
            {configRestorePreview && (
              <div className="space-y-3 text-sm">
                <Alert className="border-amber-500/40 bg-amber-500/10">
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle>{t("adminConfig.highRiskOperationTitle")}</AlertTitle>
                  <AlertDescription>
                    {t("adminConfig.restoreHighRiskDescription")}
                  </AlertDescription>
                </Alert>
                <div className="grid gap-2 rounded-md border p-3 text-xs">
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">{t("adminConfig.targetBackup")}</span>
                    <strong className="break-all text-right">{configRestorePreview.restored}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">{t("adminConfig.configFile")}</span>
                    <strong className="break-all text-right">{configRestorePreview.config_file}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">{t("adminConfig.contentSize")}</span>
                    <strong>{formatBytes(configRestorePreview.content_bytes)}</strong>
                  </div>
                </div>
                {configRestorePreview.warnings?.length ? (
                  <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200">
                    {configRestorePreview.warnings.join("；")}
                  </div>
                ) : null}
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setShowConfigRestoreDialog(false)} disabled={isConfigBackupBusy}>{t("common.cancel")}</Button>
              <Button onClick={() => void handleConfirmConfigRestore()} disabled={isConfigBackupBusy || !configRestorePreview}>
                {isConfigBackupBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RotateCcw className="mr-2 h-4 w-4" />}
                {t("adminConfig.confirmRestore")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={showSaveDialog} onOpenChange={setShowSaveDialog}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("adminConfig.saveDialogTitle")}</DialogTitle>
              <DialogDescription>
                {t("adminConfig.saveDialogDescription")}
              </DialogDescription>
            </DialogHeader>
            <div className="max-h-64 overflow-y-auto space-y-2 py-2">
              {schema?.sections.map((section) => {
                const sectionChanges = configChanges.filter((change) => change.sectionKey === section.key);
                if (sectionChanges.length === 0) return null;
                const Icon = SECTION_ICONS[section.key] || CircleDot;
                return (
                  <div
                    key={section.key}
                    className="rounded-md border px-3 py-2"
                  >
                    <div className="flex items-center gap-2 text-sm font-medium mb-1">
                      <Icon className="h-4 w-4 text-primary" />
                      {section.title}
                    </div>
                    <div className="space-y-2">
                      {sectionChanges.map((change) => (
                        <div key={change.field.key} className="rounded border bg-muted/30 px-2 py-2 text-xs">
                          <div className="mb-1 flex min-w-0 flex-wrap items-center gap-2">
                            <span className="font-medium">{change.field.label}</span>
                            <code className="text-[11px] text-muted-foreground">{change.field.key}</code>
                          </div>
                          <div className="grid gap-1 sm:grid-cols-[1fr_auto_1fr] sm:items-center">
                            <span className="rounded bg-background px-2 py-1 text-muted-foreground">
                              {formatConfigPreviewValue(change.field, change.before, t)}
                            </span>
                            <span className="hidden text-muted-foreground sm:inline">→</span>
                            <span className="rounded bg-primary/10 px-2 py-1 text-primary">
                              {formatConfigPreviewValue(change.field, change.after, t)}
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                );
              })}
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => setShowSaveDialog(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button onClick={handleSaveSchema} disabled={isSavingSchema}>
                {isSavingSchema ? (
                  <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Save className="mr-1.5 h-3.5 w-3.5" />
                )}
                {t("adminConfig.confirmSave")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </motion.div>
    </TooltipProvider>
  );
}
