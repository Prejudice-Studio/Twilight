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
  Webhook,
  Server,
  Shield,
  Clock,
  Bell,
  BookOpen,
  Info,
  CircleDot,
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { ConfigSchema, ConfigSection, ConfigField } from "@/lib/api";

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
  Webhook: Webhook,
  API: Server,
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
  value: unknown[];
  onChange: (v: unknown[]) => void;
}) {
  const items = Array.isArray(value) ? value.map(String) : [];

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

    case "list":
      return (
        <ListField
          value={Array.isArray(value) ? value : []}
          onChange={onChange}
        />
      );

    case "select":
      return (
        <Select
          value={String(value)}
          onValueChange={(v) => {
            const opt = field.options?.find((o) => String(o.value) === v);
            onChange(opt ? opt.value : v);
          }}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(field.options ?? []).map((opt) => (
              <SelectItem key={String(opt.value)} value={String(opt.value)}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
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
      <div className="flex items-start gap-4">
        <div className="flex-1 min-w-0 space-y-1">
          <div className="flex items-center gap-2">
            <Label className="text-sm font-medium leading-none">
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

        <div className="flex items-center gap-2 shrink-0">
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
            <div className="flex items-center gap-3">
              <div className="flex items-center justify-center h-9 w-9 rounded-lg bg-primary/10 text-primary">
                <Icon className="h-5 w-5" />
              </div>
              <div className="flex-1 min-w-0">
                <CardTitle className="text-base flex items-center gap-2">
                  {section.title}
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
  activeSection,
  changedCounts,
  onSelect,
}: {
  sections: ConfigSection[];
  activeSection: string;
  changedCounts: Record<string, number>;
  onSelect: (key: string) => void;
}) {
  return (
    <nav className="hidden xl:block w-52 shrink-0">
      <div className="sticky top-20 space-y-0.5">
        <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2 px-2">
          配置分组
        </p>
        {sections.map((section) => {
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
    </nav>
  );
}

// ==================== 主页面 ====================

export default function AdminConfigPage() {
  const { toast } = useToast();

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

  // 初始化时展开所有 sections
  useEffect(() => {
    if (schema) {
      setExpandedSections(new Set(schema.sections.map((s) => s.key)));
      if (schema.sections.length > 0 && !activeSection) {
        setActiveSection(schema.sections[0].key);
      }
    }
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
          initial[section.key][field.key] = field.value;
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
    setTimeout(() => {
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
      const res = await api.updateConfigBySchema(editedValues);
      if (res.success) {
        setOriginalValues(JSON.parse(JSON.stringify(editedValues)));
        toast({
          title: "保存成功",
          description: "配置已更新并重新加载",
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
          description: "配置文件已更新并重新加载",
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
          defaultValue="visual"
          onValueChange={(v) => {
            if (v === "toml" && !configContent) {
              void loadConfig();
            }
          }}
        >
          <div className="flex items-center justify-between">
            <TabsList>
              <TabsTrigger value="visual" className="gap-1.5">
                <SlidersHorizontal className="h-4 w-4" />
                可视化编辑
              </TabsTrigger>
              <TabsTrigger value="toml" className="gap-1.5">
                <FileText className="h-4 w-4" />
                源文件编辑
              </TabsTrigger>
            </TabsList>
          </div>

          {/* ==================== 可视化编辑 ==================== */}
          <TabsContent value="visual" className="mt-4">
            {/* 搜索与操作栏 */}
            <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3 mb-4">
              <div className="relative flex-1 max-w-md">
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
              <div className="flex gap-2 ml-auto">
                <Button
                  variant="outline"
                  size="sm"
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
                    className="text-muted-foreground"
                  >
                    <RotateCcw className="mr-1.5 h-3.5 w-3.5" />
                    全部还原
                  </Button>
                )}
                <Button
                  size="sm"
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
            <div className="flex gap-6">
              {/* 侧边导航 */}
              {schema && !searchText && (
                <SectionNav
                  sections={schema.sections}
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
                    {schema?.sections.map((section) => (
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
                    ))}
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
                  className="fixed bottom-6 left-1/2 -translate-x-1/2 z-50"
                >
                  <div className="flex items-center gap-3 bg-background/95 backdrop-blur border shadow-lg rounded-full px-5 py-2.5">
                    <AlertTriangle className="h-4 w-4 text-amber-500 shrink-0" />
                    <span className="text-sm">
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
        </Tabs>

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

