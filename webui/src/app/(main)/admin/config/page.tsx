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
  Github,
  GitPullRequest,
  Database,
  Archive,
  UploadCloud,
  Trash2,
} from "lucide-react";
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
  DatabaseBackup,
  DatabaseStatus,
  DatabaseMigrationResult,
  DatabaseRestoreResult,
  ConfigBackup,
  ConfigBackupView,
  ConfigRestoreResult,
} from "@/lib/api";

// 没有声明 categories 时的回退：所有 section 归到「全部」一类，保持原来的扁平体验
const FALLBACK_CATEGORY: ConfigCategory = { key: "_all", title: "全部" };

const NUMERIC_LIST_FIELD_KEYS = new Set([
  "streak_bonus_days",
  "streak_bonus_points",
]);

const MIXED_ID_LIST_FIELD_KEYS = new Set([
  "admin_id",
  "group_id",
  "channel_id",
]);

const DATABASE_RESTORE_CONFIRM = "RESTORE_DATABASE_BACKUP";
const DATABASE_MIGRATE_CONFIRM = "MIGRATE_DATABASE";
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

  if (MIXED_ID_LIST_FIELD_KEYS.has(fieldKey)) {
    return items.map((v) => (/^-?\d+$/.test(v) ? Number.parseInt(v, 10) : v));
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
  SAR: Coins,
  DeviceLimit: Monitor,
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
        添加
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
  const labelRef = useRef<HTMLDivElement>(null);

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
                已修改
              </Badge>
            )}
            <TooltipProvider delayDuration={300}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Info className="h-3.5 w-3.5 text-muted-foreground/60 cursor-help" />
                </TooltipTrigger>
                <TooltipContent side="top" className="max-w-xs">
                  <p>{field.description}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
          <p className="text-xs text-muted-foreground line-clamp-1">
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
                <TooltipContent>还原修改</TooltipContent>
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
                      {changedCount} 项修改
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
                  const isChanged =
                    JSON.stringify(val) !== JSON.stringify(origVal);
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
          配置分组
        </p>
        {grouped.map(({ category, sections: groupSections }) => (
          <div key={category.key} className="space-y-0.5">
            <p className="px-2 text-[11px] font-medium text-muted-foreground/70">
              {category.title}
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
  const { confirm } = useConfirm();

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
  const [dbStatus, setDbStatus] = useState<DatabaseStatus | null>(null);
  const [dbBackups, setDbBackups] = useState<DatabaseBackup[]>([]);
  const [isLoadingDatabase, setIsLoadingDatabase] = useState(false);
  const [isDatabaseBusy, setIsDatabaseBusy] = useState(false);
  const [migrationSource, setMigrationSource] = useState<"current" | "sqlite">("current");
  const [migrationTarget, setMigrationTarget] = useState<"json" | "postgres">("postgres");
  const [migrationResult, setMigrationResult] = useState<DatabaseMigrationResult | null>(null);
  const [restorePreview, setRestorePreview] = useState<DatabaseRestoreResult | null>(null);
  const [showRestoreDialog, setShowRestoreDialog] = useState(false);
  const [showMigrationDialog, setShowMigrationDialog] = useState(false);
  const [configBackups, setConfigBackups] = useState<ConfigBackup[]>([]);
  const [isLoadingConfigBackups, setIsLoadingConfigBackups] = useState(false);
  const [isConfigBackupBusy, setIsConfigBackupBusy] = useState(false);
  const [configBackupView, setConfigBackupView] = useState<ConfigBackupView | null>(null);
  const [configRestorePreview, setConfigRestorePreview] = useState<ConfigRestoreResult | null>(null);
  const [showConfigBackupView, setShowConfigBackupView] = useState(false);
  const [showConfigRestoreDialog, setShowConfigRestoreDialog] = useState(false);
  const scrollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

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
        if (JSON.stringify(edited) !== JSON.stringify(orig)) count++;
      }
      counts[section.key] = count;
    }
    return counts;
  }, [schema, editedValues, originalValues]);

  const totalChangedCount = useMemo(
    () => Object.values(changedCounts).reduce((a, b) => a + b, 0),
    [changedCounts]
  );

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
    if (migrationSource === "sqlite" && dbStatus && !dbStatus.legacy_sqlite_detected) {
      setMigrationSource("current");
      setMigrationResult(null);
    }
  }, [dbStatus, migrationSource]);

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
      throw new Error(res.message || "无法加载配置文件");
    }
    return true;
  }, []);

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
      throw new Error(res.message || "无法加载配置结构");
    }
    return true;
  }, []);

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
      toast({ title: "没有更改", description: "配置未修改" });
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
        toast({
          title: "保存成功",
          description: "配置已热重载，调度器会自动刷新任务",
          variant: "success",
        });
      } else {
        toast({
          title: "保存失败",
          description: res.message || "无法保存配置",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "保存失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsSavingSchema(false);
    }
  };

  // 保存源文件
  const handleSaveToml = async () => {
    if (!hasChanges) {
      toast({ title: "没有更改", description: "配置文件未修改" });
      return;
    }

    setIsSaving(true);
    try {
      const res = await api.updateConfigToml(configContent);
      if (res.success) {
        setOriginalContent(configContent);
        setHasChanges(false);
        toast({
          title: "保存成功",
          description: "配置已热重载，调度器会自动刷新任务",
          variant: "success",
        });
      } else {
        toast({
          title: "保存失败",
          description: res.message || "无法保存配置文件",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "保存失败",
        description: error.message || "请检查网络连接",
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const loadDatabase = useCallback(async () => {
    setIsLoadingDatabase(true);
    try {
      const [statusRes, backupsRes] = await Promise.all([
        api.getDatabaseStatus(),
        api.listDatabaseBackups(),
      ]);
      if (statusRes.success && statusRes.data) {
        setDbStatus(statusRes.data);
      }
      if (backupsRes.success && backupsRes.data) {
        setDbBackups(backupsRes.data.backups || []);
      }
    } catch (error: any) {
      toast({
        title: "加载数据库状态失败",
        description: error.message || "请检查后端连接",
        variant: "destructive",
      });
    } finally {
      setIsLoadingDatabase(false);
    }
  }, [toast]);

  const loadConfigBackups = useCallback(async () => {
    setIsLoadingConfigBackups(true);
    try {
      const res = await api.listConfigBackups();
      if (res.success && res.data) {
        setConfigBackups(res.data.backups || []);
      }
    } catch (error: any) {
      toast({
        title: "加载配置备份失败",
        description: error.message || "请检查后端连接",
        variant: "destructive",
      });
    } finally {
      setIsLoadingConfigBackups(false);
    }
  }, [toast]);

  const handleCreateConfigBackup = async () => {
    setIsConfigBackupBusy(true);
    try {
      const res = await api.createConfigBackup();
      if (res.success) {
        toast({ title: "配置备份已创建", description: res.data?.backup?.name, variant: "success" });
        await loadConfigBackups();
      } else {
        toast({ title: "配置备份失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "配置备份失败", description: error.message || "请检查后端日志", variant: "destructive" });
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
        toast({ title: "读取配置备份失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "读取配置备份失败", description: error.message || "请检查后端日志", variant: "destructive" });
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
        toast({ title: "配置恢复预览失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "配置恢复预览失败", description: error.message || "请检查后端日志", variant: "destructive" });
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
          title: "配置已恢复",
          description: res.data.pre_operation_backup ? `保护性备份：${res.data.pre_operation_backup.name}` : "已创建保护性备份",
          variant: "success",
        });
        await loadConfigBackups();
        await loadSchema();
        if (configContent) {
          await loadConfig();
        }
      } else {
        toast({ title: "配置恢复失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "配置恢复失败", description: error.message || "请检查后端日志", variant: "destructive" });
    } finally {
      setIsConfigBackupBusy(false);
    }
  };

  const handleDeleteConfigBackup = async (backup: ConfigBackup) => {
    const accepted = await confirm({
      title: "删除配置备份",
      description: `确认删除配置备份 ${backup.name}？此操作不可恢复。`,
      tone: "danger",
      confirmLabel: "删除备份",
      confirmVariant: "destructive",
    });
    if (!accepted) return;
    setIsConfigBackupBusy(true);
    try {
      const res = await api.deleteConfigBackup(backup.name);
      if (res.success) {
        toast({ title: "配置备份已删除", description: backup.name, variant: "success" });
        await loadConfigBackups();
      } else {
        toast({ title: "删除配置备份失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "删除配置备份失败", description: error.message || "请检查后端日志", variant: "destructive" });
    } finally {
      setIsConfigBackupBusy(false);
    }
  };

  const handleCreateBackup = async () => {
    setIsDatabaseBusy(true);
    try {
      const res = await api.createDatabaseBackup();
      if (res.success) {
        toast({
          title: "备份已创建",
          description: res.data?.legacy_sqlite_backup
            ? `已同时备份旧 SQLite：${res.data.legacy_sqlite_backup.file_count} 个文件`
            : undefined,
          variant: "success",
        });
        await loadDatabase();
      } else {
        toast({ title: "备份失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "备份失败", description: error.message, variant: "destructive" });
    } finally {
      setIsDatabaseBusy(false);
    }
  };

  const handleRestoreBackup = async (backup: DatabaseBackup) => {
    setIsDatabaseBusy(true);
    setRestorePreview(null);
    try {
      const res = await api.previewDatabaseRestore(backup.name);
      if (res.success) {
        setRestorePreview(res.data || null);
        setShowRestoreDialog(true);
      } else {
        toast({ title: "恢复预览失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "恢复预览失败", description: error.message, variant: "destructive" });
    } finally {
      setIsDatabaseBusy(false);
    }
  };

  const handleConfirmRestoreBackup = async () => {
    if (!restorePreview?.restored) return;
    setIsDatabaseBusy(true);
    try {
      const res = await api.restoreDatabaseBackup(restorePreview.restored, {
        confirm: restorePreview.confirm || DATABASE_RESTORE_CONFIRM,
      });
      if (res.success) {
        setRestorePreview(res.data || restorePreview);
        setShowRestoreDialog(false);
        toast({
          title: "恢复完成",
          description: res.data?.pre_operation_backup
            ? `已创建保护性备份 ${res.data.pre_operation_backup.name}`
            : "恢复前已创建保护性备份",
          variant: "success",
        });
        await loadDatabase();
      } else {
        toast({ title: "恢复失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "恢复失败", description: error.message, variant: "destructive" });
    } finally {
      setIsDatabaseBusy(false);
    }
  };

  const handleDatabaseMigrate = async (dryRun: boolean) => {
    setIsDatabaseBusy(true);
    setMigrationResult(null);
    try {
      const res = await api.migrateDatabase({
        source_driver: migrationSource === "sqlite" ? "sqlite" : undefined,
        target_driver: migrationTarget,
        dry_run: true,
      });
      if (res.success && res.data) {
        setMigrationResult(res.data);
        if (dryRun) {
          const source = res.data.source_driver === "sqlite" ? "旧 SQLite" : "当前 Go 状态";
          toast({
            title: "迁移预检通过",
            description: `${source}: ${res.data.users} 用户，${res.data.regcodes} 卡码，${res.data.invite_codes} 邀请码`,
            variant: "success",
          });
        } else {
          setShowMigrationDialog(true);
        }
      } else {
        toast({ title: "迁移预检失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "迁移预检失败", description: error.message, variant: "destructive" });
    } finally {
      setIsDatabaseBusy(false);
    }
  };

  const handleConfirmDatabaseMigrate = async () => {
    setIsDatabaseBusy(true);
    try {
      const res = await api.migrateDatabase({
        source_driver: migrationSource === "sqlite" ? "sqlite" : undefined,
        target_driver: migrationTarget,
        dry_run: false,
        confirm: migrationResult?.confirm || DATABASE_MIGRATE_CONFIRM,
      });
      if (res.success && res.data) {
        setMigrationResult(res.data);
        setShowMigrationDialog(false);
        toast({
          title: "迁移完成",
          description: res.data.pre_operation_backup
            ? `已创建保护性备份 ${res.data.pre_operation_backup.name}${res.data.legacy_sqlite_backup ? "，并备份旧 SQLite" : ""}`
            : `${res.data.users} 用户，${res.data.regcodes} 卡码，${res.data.invite_codes} 邀请码`,
          variant: "success",
        });
        await loadDatabase();
      } else {
        toast({ title: "迁移失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "迁移失败", description: error.message, variant: "destructive" });
    } finally {
      setIsDatabaseBusy(false);
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
          title: dryRun ? "预检通过" : "更新完成",
          description: res.message || "代码已更新，服务将按设置重启",
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
          title: dryRun ? "预检失败" : "更新失败",
          description: res.message || "请查看命令输出",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "更新失败",
        description: error.message || "请检查网络连接",
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
          <h1 className="text-3xl font-bold">配置管理</h1>
          <p className="text-muted-foreground">
            查看和修改项目配置，支持可视化编辑和源文件编辑
          </p>
        </div>

        <Tabs
          className="min-w-0"
          defaultValue="visual"
          onValueChange={(v) => {
            if (v === "toml" && !configContent) {
              void loadConfig();
            }
            if (v === "database" && !dbStatus) {
              void loadDatabase();
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
                可视化编辑
              </TabsTrigger>
              <TabsTrigger value="toml" className="min-w-0 gap-1.5 px-2 sm:px-4">
                <FileText className="h-4 w-4" />
                源文件编辑
              </TabsTrigger>
              <TabsTrigger value="database" className="min-w-0 gap-1.5 px-2 sm:px-4">
                <Database className="h-4 w-4" />
                数据库
              </TabsTrigger>
              <TabsTrigger value="config-backups" className="min-w-0 gap-1.5 px-2 sm:px-4">
                <Archive className="h-4 w-4" />
                配置备份
              </TabsTrigger>
              <TabsTrigger value="update" className="min-w-0 gap-1.5 px-2 sm:px-4">
                <GitPullRequest className="h-4 w-4" />
                在线更新
              </TabsTrigger>
            </TabsList>
          </div>

          {/* ==================== 可视化编辑 ==================== */}
          <TabsContent value="visual" className="mt-4">
            {/* 搜索与操作栏 */}
            <div className="mb-4 flex min-w-0 flex-col items-stretch gap-3 sm:flex-row sm:items-center">
              <div className="relative w-full min-w-0 sm:max-w-md sm:flex-1">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="搜索配置项..."
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
                  找到 {searchResultCount} 个匹配项
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
                  刷新
                </Button>
                {hasSchemaChanges && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleResetAll}
                    className="flex-1 text-muted-foreground sm:flex-none"
                  >
                    <RotateCcw className="mr-1.5 h-3.5 w-3.5" />
                    全部还原
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
                  保存配置
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
                          <p>没有找到匹配的配置项</p>
                          <p className="text-xs mt-1">
                            尝试使用不同的关键词搜索
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
                      {totalChangedCount} 项配置已修改
                    </span>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={handleResetAll}
                      className="h-7 text-xs"
                    >
                      还原
                    </Button>
                    <Button
                      size="sm"
                      onClick={() => setShowSaveDialog(true)}
                      className="h-7 text-xs"
                    >
                      <Save className="mr-1 h-3 w-3" />
                      保存
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
                        config.toml
                      </CardTitle>
                      <CardDescription>
                        直接编辑 TOML 配置文件，保存后自动重新加载
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
                        重新加载
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
                        保存
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
                            保存时自动备份为 config.toml.backup
                          </span>
                        </div>
                      )}
                      {hasChanges && (
                        <Alert>
                          <AlertTriangle className="h-4 w-4" />
                          <AlertDescription>
                            检测到未保存的更改，请点击保存按钮应用更改
                          </AlertDescription>
                        </Alert>
                      )}
                      <Textarea
                        value={configContent}
                        onChange={(e) => setConfigContent(e.target.value)}
                        className="font-mono text-sm min-h-[600px] leading-relaxed"
                        placeholder="配置文件内容..."
                      />
                      <div className="flex items-center justify-between text-xs text-muted-foreground px-1">
                        <span>
                          {configContent.split("\n").length} 行 ·{" "}
                          {configContent.length} 字符
                        </span>
                        <span>TOML</span>
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
                    配置备份与恢复
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    保存配置前会自动生成保护性备份；也可以手动备份、查看内容、恢复或删除历史备份。
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button variant="outline" size="sm" onClick={() => void loadConfigBackups()} disabled={isLoadingConfigBackups || isConfigBackupBusy}>
                    {isLoadingConfigBackups ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RotateCcw className="mr-2 h-4 w-4" />}
                    刷新
                  </Button>
                  <Button size="sm" onClick={() => void handleCreateConfigBackup()} disabled={isConfigBackupBusy}>
                    {isConfigBackupBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Archive className="mr-2 h-4 w-4" />}
                    创建配置备份
                  </Button>
                </div>
              </div>

              <Alert>
                <Shield className="h-4 w-4" />
                <AlertTitle>安全机制</AlertTitle>
                <AlertDescription>
                  配置恢复会先校验 TOML 是否可加载，再创建当前配置的保护性备份；热重载失败会自动回滚当前配置文件。
                </AlertDescription>
              </Alert>

              <Card>
                <CardHeader>
                  <CardTitle>配置备份列表</CardTitle>
                  <CardDescription>备份文件保存在数据库备份目录下的 config 子目录。</CardDescription>
                </CardHeader>
                <CardContent>
                  {configBackups.length === 0 ? (
                    <div className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
                      {isLoadingConfigBackups ? "正在加载..." : "暂无配置备份"}
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
                              <Eye className="mr-2 h-4 w-4" />查看
                            </Button>
                            <Button variant="outline" size="sm" onClick={() => void handlePreviewConfigRestore(backup)} disabled={isConfigBackupBusy}>
                              <RotateCcw className="mr-2 h-4 w-4" />恢复
                            </Button>
                            <Button variant="outline" size="sm" onClick={() => void handleDeleteConfigBackup(backup)} disabled={isConfigBackupBusy} className="text-destructive hover:text-destructive">
                              <Trash2 className="mr-2 h-4 w-4" />删除
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

          <TabsContent value="database" className="mt-4">
            <div className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h2 className="text-lg font-semibold flex items-center gap-2">
                    <Database className="h-5 w-5" />
                    数据库管理
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    备份、恢复和迁移 Go 状态；检测到旧 SQLite 时可直接预览并导入到 JSON 或 PostgreSQL。
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <a
                    href="/admin/database"
                    className="inline-flex h-9 items-center justify-center rounded-md border border-input bg-background px-3 text-sm font-medium hover:bg-accent hover:text-accent-foreground"
                  >
                    打开独立迁移页
                  </a>
                  <Button variant="outline" size="sm" onClick={() => void loadDatabase()} disabled={isLoadingDatabase}>
                    {isLoadingDatabase ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RotateCcw className="mr-2 h-4 w-4" />}
                    刷新
                  </Button>
                </div>
              </div>

              <div className="grid gap-3 md:grid-cols-4">
                <Card>
                  <CardContent className="p-4">
                    <p className="text-xs text-muted-foreground">当前后端</p>
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
                    <p className="text-xs text-muted-foreground">用户数</p>
                    <p className="mt-1 text-xl font-semibold">{dbStatus?.user_count ?? "-"}</p>
                  </CardContent>
                </Card>
                <Card>
                  <CardContent className="p-4">
                    <p className="text-xs text-muted-foreground">备份数</p>
                    <p className="mt-1 text-xl font-semibold">{dbStatus?.backup_count ?? dbBackups.length}</p>
                  </CardContent>
                </Card>
              </div>

              {dbStatus && (
                <Alert>
                  <Info className="h-4 w-4" />
                  <AlertTitle>运行状态</AlertTitle>
                  <AlertDescription>
                    状态文件：{dbStatus.state_file}；备份目录：{dbStatus.backup_dir}；PostgreSQL {dbStatus.postgres_configured ? "已配置" : "未配置"}。
                    切换存储后端需要先迁移数据，然后重启后端进程使新 driver 生效。
                  </AlertDescription>
                </Alert>
              )}

              {dbStatus?.legacy_sqlite_detected && dbStatus.legacy_sqlite && (
                <Alert className="border-amber-500/40 bg-amber-500/10">
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle>检测到旧 SQLite 数据库</AlertTitle>
                  <AlertDescription className="space-y-2">
                    <p>
                      目录 {dbStatus.legacy_sqlite.database_dir} 中有 {dbStatus.legacy_sqlite.file_count} 个旧 SQLite/WAL 文件，
                      合计 {formatBytes(dbStatus.legacy_sqlite.total_size)}。
                      {dbStatus.legacy_sqlite.active_admin_count ? ` 可读取 active 管理员 ${dbStatus.legacy_sqlite.active_admin_count} 个。` : ""}
                    </p>
                    {dbStatus.legacy_sqlite.table_counts && (
                      <p className="break-words text-xs">
                        已识别表：{Object.keys(dbStatus.legacy_sqlite.table_counts).slice(0, 12).join("、")}
                        {Object.keys(dbStatus.legacy_sqlite.table_counts).length > 12 ? " 等" : ""}
                      </p>
                    )}
                    {dbStatus.legacy_sqlite.warnings?.length ? (
                      <p className="text-xs text-amber-700 dark:text-amber-300">
                        {dbStatus.legacy_sqlite.warnings.join("；")}
                      </p>
                    ) : null}
                  </AlertDescription>
                </Alert>
              )}

              <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
                <Card>
                  <CardHeader>
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <CardTitle className="flex items-center gap-2">
                          <Archive className="h-5 w-5" />
                          备份
                        </CardTitle>
                        <CardDescription>恢复必须先预览并二次确认，执行前会自动生成保护性备份。</CardDescription>
                      </div>
                      <Button onClick={handleCreateBackup} disabled={isDatabaseBusy}>
                        {isDatabaseBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Archive className="mr-2 h-4 w-4" />}
                        创建备份
                      </Button>
                    </div>
                  </CardHeader>
                  <CardContent>
                    {dbBackups.length === 0 ? (
                      <div className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
                        暂无备份
                      </div>
                    ) : (
                      <div className="divide-y rounded-md border">
                        {dbBackups.map((backup) => (
                          <div key={backup.name} className="flex items-center justify-between gap-3 px-3 py-2">
                            <div className="min-w-0">
                              <p className="truncate text-sm font-medium">{backup.name}</p>
                              <p className="text-xs text-muted-foreground">
                                {formatBytes(backup.size)} · {formatUnixTime(backup.created_at)}
                              </p>
                            </div>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => void handleRestoreBackup(backup)}
                              disabled={isDatabaseBusy}
                            >
                              预览恢复
                            </Button>
                          </div>
                        ))}
                      </div>
                    )}
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <UploadCloud className="h-5 w-5" />
                      迁移
                    </CardTitle>
                    <CardDescription>预检会先构造目标快照；执行前会自动备份当前状态和旧 SQLite 文件。</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="space-y-2">
                      <p className="text-xs font-medium text-muted-foreground">来源</p>
                      <div className="grid grid-cols-2 gap-2">
                        <Button
                          variant={migrationSource === "current" ? "default" : "outline"}
                          onClick={() => {
                            setMigrationSource("current");
                            setMigrationResult(null);
                          }}
                        >
                          当前 Go 状态
                        </Button>
                        <Button
                          variant={migrationSource === "sqlite" ? "default" : "outline"}
                          disabled={!dbStatus?.legacy_sqlite_detected}
                          onClick={() => {
                            setMigrationSource("sqlite");
                            setMigrationResult(null);
                          }}
                        >
                          旧 SQLite
                        </Button>
                      </div>
                    </div>
                    <div className="space-y-2">
                      <p className="text-xs font-medium text-muted-foreground">目标</p>
                    <div className="grid grid-cols-2 gap-2">
                      <Button
                        variant={migrationTarget === "postgres" ? "default" : "outline"}
                        onClick={() => {
                          setMigrationTarget("postgres");
                          setMigrationResult(null);
                        }}
                      >
                        PostgreSQL
                      </Button>
                      <Button
                        variant={migrationTarget === "json" ? "default" : "outline"}
                        onClick={() => {
                          setMigrationTarget("json");
                          setMigrationResult(null);
                        }}
                      >
                        JSON
                      </Button>
                    </div>
                    </div>
                    <div className="flex gap-2">
                      <Button variant="outline" className="flex-1" onClick={() => void handleDatabaseMigrate(true)} disabled={isDatabaseBusy}>
                        预检
                      </Button>
                      <Button className="flex-1" onClick={() => void handleDatabaseMigrate(false)} disabled={isDatabaseBusy}>
                        {isDatabaseBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
                        预览并执行
                      </Button>
                    </div>
                    {migrationResult && (
                      <div className="rounded-md border bg-muted/40 p-3 text-xs space-y-1">
                        <div className="flex justify-between"><span>来源</span><strong>{migrationResult.source_driver || "-"}</strong></div>
                        <div className="flex justify-between"><span>目标</span><strong>{migrationResult.target_driver}</strong></div>
                        <div className="flex justify-between"><span>快照</span><strong>{formatBytes(migrationResult.snapshot_bytes || 0)}</strong></div>
                        <div className="flex justify-between"><span>用户</span><strong>{migrationResult.users}</strong></div>
                        <div className="flex justify-between"><span>卡码</span><strong>{migrationResult.regcodes}</strong></div>
                        <div className="flex justify-between"><span>邀请</span><strong>{migrationResult.invite_codes}</strong></div>
                        <div className="flex justify-between"><span>求片</span><strong>{migrationResult.media_requests}</strong></div>
                        {migrationResult.legacy_sqlite_import && (
                          <>
                            <div className="flex justify-between"><span>映射表</span><strong>{migrationResult.legacy_sqlite_import.mapped_tables?.length ?? 0}</strong></div>
                            <div className="flex justify-between"><span>跳过表</span><strong>{migrationResult.legacy_sqlite_import.skipped_tables?.length ?? 0}</strong></div>
                            {migrationResult.legacy_sqlite_import.skipped_tables?.length ? (
                              <div className="pt-2 text-muted-foreground break-words">
                                未映射：{migrationResult.legacy_sqlite_import.skipped_tables.join("、")}
                              </div>
                            ) : null}
                          </>
                        )}
                        {migrationResult.target_ready && (
                          <div className="pt-2 text-muted-foreground">
                            目标状态：{JSON.stringify(migrationResult.target_ready)}
                          </div>
                        )}
                        {migrationResult.warnings && migrationResult.warnings.length > 0 && (
                          <div className="pt-2 text-amber-600 dark:text-amber-400">
                            {migrationResult.warnings.join("；")}
                          </div>
                        )}
                        {migrationResult.pre_operation_backup && (
                          <div className="pt-2 text-emerald-600 dark:text-emerald-400">
                            保护性备份：{migrationResult.pre_operation_backup.name}
                          </div>
                        )}
                        {migrationResult.legacy_sqlite_backup && (
                          <div className="pt-2 text-emerald-600 dark:text-emerald-400">
                            旧 SQLite 备份：{migrationResult.legacy_sqlite_backup.name}
                          </div>
                        )}
                      </div>
                    )}
                  </CardContent>
                </Card>
              </div>
            </div>
          </TabsContent>

          <TabsContent value="update" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Github className="h-5 w-5" />
                  Git 自动更新
                </CardTitle>
                <CardDescription>
                  从指定仓库拉取分支，并按需自动重启 systemd 服务。请只填写可信仓库。
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-5">
                <Alert>
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle>生产操作</AlertTitle>
                  <AlertDescription>
                    后端会执行 git pull --ff-only，并重启 twilight / twilight-bot / twilight-scheduler。若服务器有未提交代码改动，更新会失败而不会强制覆盖。
                  </AlertDescription>
                </Alert>

                <div className="grid gap-4 md:grid-cols-[1fr_180px]">
                  <div className="space-y-2">
                    <Label htmlFor="update-repo-url">Git 仓库地址</Label>
                    <Input
                      id="update-repo-url"
                      value={updateRepoUrl}
                      onChange={(e) => setUpdateRepoUrl(e.target.value)}
                      placeholder="https://github.com/Prejudice-Studio/Twilight.git"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="update-branch">分支</Label>
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
                      <span className="block text-sm font-medium">自动重启服务</span>
                      <span className="text-xs text-muted-foreground">systemctl restart 三个 Twilight 服务</span>
                    </span>
                    <Switch checked={updateRestartServices} onCheckedChange={setUpdateRestartServices} />
                  </label>
                </div>

                <div className="flex flex-wrap gap-2">
                  <Button variant="outline" onClick={() => void handleGitUpdate(true)} disabled={isUpdating || !updateRepoUrl.trim()}>
                    {isUpdating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Shield className="mr-2 h-4 w-4" />}
                    安全预检
                  </Button>
                  <Button onClick={() => void handleGitUpdate(false)} disabled={isUpdating || !updateRepoUrl.trim()}>
                    {isUpdating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <GitPullRequest className="mr-2 h-4 w-4" />}
                    拉取并更新
                  </Button>
                  <a
                    href="https://github.com/Prejudice-Studio/Twilight"
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-input bg-background px-4 py-2 text-sm font-medium hover:bg-accent hover:text-accent-foreground"
                  >
                    <Github className="h-4 w-4" />
                    打开 GitHub
                  </a>
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
              <DialogTitle>查看配置备份</DialogTitle>
              <DialogDescription>
                {configBackupView?.backup.name} · {formatBytes(configBackupView?.backup.size || 0)}
              </DialogDescription>
            </DialogHeader>
            <pre className="max-h-[65vh] overflow-auto p-4 text-xs leading-relaxed whitespace-pre-wrap">
              {configBackupView?.content || ""}
            </pre>
            <DialogFooter className="border-t p-4">
              <Button variant="outline" onClick={() => setShowConfigBackupView(false)}>关闭</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={showConfigRestoreDialog} onOpenChange={setShowConfigRestoreDialog}>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>确认恢复配置备份</DialogTitle>
              <DialogDescription>
                恢复会替换当前 config.toml，并立即尝试热重载；失败时会自动回滚。
              </DialogDescription>
            </DialogHeader>
            {configRestorePreview && (
              <div className="space-y-3 text-sm">
                <Alert className="border-amber-500/40 bg-amber-500/10">
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle>高风险操作</AlertTitle>
                  <AlertDescription>
                    确认后会先备份当前配置，再用备份内容覆盖当前配置文件。
                  </AlertDescription>
                </Alert>
                <div className="grid gap-2 rounded-md border p-3 text-xs">
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">目标备份</span>
                    <strong className="break-all text-right">{configRestorePreview.restored}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">配置文件</span>
                    <strong className="break-all text-right">{configRestorePreview.config_file}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">内容大小</span>
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
              <Button variant="outline" onClick={() => setShowConfigRestoreDialog(false)} disabled={isConfigBackupBusy}>取消</Button>
              <Button onClick={() => void handleConfirmConfigRestore()} disabled={isConfigBackupBusy || !configRestorePreview}>
                {isConfigBackupBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RotateCcw className="mr-2 h-4 w-4" />}
                确认恢复
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={showRestoreDialog} onOpenChange={setShowRestoreDialog}>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>确认恢复数据库备份</DialogTitle>
              <DialogDescription>
                后端已完成备份预览。确认后会先备份当前数据库，再用目标备份替换当前状态。
              </DialogDescription>
            </DialogHeader>
            {restorePreview && (
              <div className="space-y-3 text-sm">
                <Alert>
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle>高风险操作</AlertTitle>
                  <AlertDescription>
                    恢复会覆盖当前数据库状态。保护性备份会在恢复前自动创建，失败时不会继续写入。
                  </AlertDescription>
                </Alert>
                <div className="grid gap-2 rounded-md border p-3 text-xs">
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">目标备份</span>
                    <strong className="break-all text-right">{restorePreview.restored}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">备份大小</span>
                    <strong>{formatBytes(restorePreview.backup?.size || restorePreview.target_snapshot_bytes || 0)}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">当前用户数</span>
                    <strong>{restorePreview.current_counts?.users ?? "-"}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">恢复后用户数</span>
                    <strong>{restorePreview.counts?.users ?? restorePreview.users}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">卡码 / 邀请码</span>
                    <strong>{restorePreview.regcodes} / {restorePreview.invite_codes}</strong>
                  </div>
                </div>
                {restorePreview.warnings && restorePreview.warnings.length > 0 && (
                  <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200">
                    {restorePreview.warnings.join("；")}
                  </div>
                )}
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setShowRestoreDialog(false)} disabled={isDatabaseBusy}>
                取消
              </Button>
              <Button onClick={() => void handleConfirmRestoreBackup()} disabled={isDatabaseBusy || !restorePreview}>
                {isDatabaseBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Archive className="mr-2 h-4 w-4" />}
                确认恢复
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={showMigrationDialog} onOpenChange={setShowMigrationDialog}>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>确认执行数据库迁移</DialogTitle>
              <DialogDescription>
                后端已完成迁移预检。确认后会先备份当前数据库，再把快照写入目标后端。
              </DialogDescription>
            </DialogHeader>
            {migrationResult && (
              <div className="space-y-3 text-sm">
                <Alert>
                  <Info className="h-4 w-4" />
                  <AlertTitle>迁移预览</AlertTitle>
                  <AlertDescription>
                    迁移只写入目标后端；如需切换运行后端，请保存 Database.driver 配置并重启服务。
                  </AlertDescription>
                </Alert>
                <div className="grid gap-2 rounded-md border p-3 text-xs">
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">来源</span>
                    <strong>{migrationResult.source_driver || "-"}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">目标</span>
                    <strong>{migrationResult.target_driver}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">快照大小</span>
                    <strong>{formatBytes(migrationResult.snapshot_bytes || 0)}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">用户 / 卡码 / 邀请码</span>
                    <strong>{migrationResult.users} / {migrationResult.regcodes} / {migrationResult.invite_codes}</strong>
                  </div>
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">求片 / 公告</span>
                    <strong>{migrationResult.media_requests} / {migrationResult.announcements}</strong>
                  </div>
                  {migrationResult.target_ready && (
                    <div className="break-all pt-1 text-muted-foreground">
                      目标状态：{JSON.stringify(migrationResult.target_ready)}
                    </div>
                  )}
                </div>
                {migrationResult.warnings && migrationResult.warnings.length > 0 && (
                  <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200">
                    {migrationResult.warnings.join("；")}
                  </div>
                )}
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setShowMigrationDialog(false)} disabled={isDatabaseBusy}>
                取消
              </Button>
              <Button onClick={() => void handleConfirmDatabaseMigrate()} disabled={isDatabaseBusy || !migrationResult}>
                {isDatabaseBusy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <UploadCloud className="mr-2 h-4 w-4" />}
                确认迁移
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* 保存确认对话框 */}
        <Dialog open={showSaveDialog} onOpenChange={setShowSaveDialog}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>确认保存配置</DialogTitle>
              <DialogDescription>
                以下配置项将被更新，保存前将自动备份原配置文件。
              </DialogDescription>
            </DialogHeader>
            <div className="max-h-64 overflow-y-auto space-y-2 py-2">
              {schema?.sections.map((section) => {
                const sectionChanges: { label: string; key: string }[] = [];
                for (const field of section.fields) {
                  const edited = editedValues[section.key]?.[field.key];
                  const orig = originalValues[section.key]?.[field.key];
                  if (JSON.stringify(edited) !== JSON.stringify(orig)) {
                    sectionChanges.push({
                      label: field.label,
                      key: field.key,
                    });
                  }
                }
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
                    <div className="flex flex-wrap gap-1.5">
                      {sectionChanges.map((c) => (
                        <Badge
                          key={c.key}
                          variant="secondary"
                          className="text-xs"
                        >
                          {c.label}
                        </Badge>
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
                取消
              </Button>
              <Button onClick={handleSaveSchema} disabled={isSavingSchema}>
                {isSavingSchema ? (
                  <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Save className="mr-1.5 h-3.5 w-3.5" />
                )}
                确认保存
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </motion.div>
    </TooltipProvider>
  );
}
