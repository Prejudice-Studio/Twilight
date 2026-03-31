"use client";

import { useCallback, useEffect, useState, useMemo } from "react";
import { motion } from "framer-motion";
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ConfigSchema, ConfigSection, ConfigField } from "@/lib/api";

const container = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: {
      staggerChildren: 0.08,
    },
  },
};

const item = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0 },
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
            className="shrink-0"
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

function SectionCard({
  section,
  values,
  onFieldChange,
}: {
  section: ConfigSection;
  values: Record<string, unknown>;
  onFieldChange: (sectionKey: string, fieldKey: string, value: unknown) => void;
}) {
  return (
    <motion.div variants={item}>
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{section.title}</CardTitle>
          <CardDescription>{section.description}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {section.fields.map((field) => {
            const val =
              values[field.key] !== undefined ? values[field.key] : field.value;
            return (
              <div key={field.key} className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <div className="space-y-0.5 flex-1 mr-4">
                    <Label className="text-sm font-medium">
                      {field.label}
                      <span className="ml-2 text-xs font-mono text-muted-foreground">
                        {field.key}
                      </span>
                    </Label>
                    <p className="text-xs text-muted-foreground">
                      {field.description}
                    </p>
                  </div>
                  {field.type === "bool" && (
                    <ConfigFieldEditor
                      field={field}
                      value={val}
                      onChange={(v) =>
                        onFieldChange(section.key, field.key, v)
                      }
                    />
                  )}
                </div>
                {field.type !== "bool" && (
                  <ConfigFieldEditor
                    field={field}
                    value={val}
                    onChange={(v) =>
                      onFieldChange(section.key, field.key, v)
                    }
                  />
                )}
              </div>
            );
          })}
        </CardContent>
      </Card>
    </motion.div>
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

  const hasSchemaChanges = useMemo(() => {
    return JSON.stringify(editedValues) !== JSON.stringify(originalValues);
  }, [editedValues, originalValues]);

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
      // 初始化编辑值
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

  // 保存可视化配置
  const handleSaveSchema = async () => {
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

      {configPath && (
        <Alert>
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>配置文件路径</AlertTitle>
          <AlertDescription>
            {configPath}
            <br />
            <span className="text-xs text-muted-foreground">
              修改前会自动备份原文件为 config.toml.backup
            </span>
          </AlertDescription>
        </Alert>
      )}

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
        <TabsContent value="visual" className="space-y-4 mt-4">
          <div className="flex justify-end">
            <div className="flex gap-2">
              <Button
                variant="outline"
                onClick={() => void loadSchema()}
                disabled={isLoadingSchema || isSavingSchema}
              >
                {isLoadingSchema ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    加载中...
                  </>
                ) : (
                  "重新加载"
                )}
              </Button>
              <Button
                onClick={handleSaveSchema}
                disabled={isLoadingSchema || isSavingSchema || !hasSchemaChanges}
              >
                {isSavingSchema ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    保存中...
                  </>
                ) : (
                  <>
                    <Save className="mr-2 h-4 w-4" />
                    保存配置
                  </>
                )}
              </Button>
            </div>
          </div>

          {hasSchemaChanges && (
            <Alert>
              <AlertTriangle className="h-4 w-4" />
              <AlertDescription>
                检测到未保存的更改，请点击保存按钮应用更改
              </AlertDescription>
            </Alert>
          )}

          {isLoadingSchema ? (
            <div className="flex items-center justify-center h-96">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : (
            schema?.sections.map((section) => (
              <SectionCard
                key={section.key}
                section={section}
                values={editedValues[section.key] ?? {}}
                onFieldChange={handleFieldChange}
              />
            ))
          )}
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
                      onClick={() => void loadConfig()}
                      disabled={isLoadingToml || isSaving}
                    >
                      {isLoadingToml ? (
                        <>
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                          加载中...
                        </>
                      ) : (
                        "重新加载"
                      )}
                    </Button>
                    <Button
                      onClick={handleSaveToml}
                      disabled={isLoadingToml || isSaving || !hasChanges}
                    >
                      {isSaving ? (
                        <>
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                          保存中...
                        </>
                      ) : (
                        <>
                          <Save className="mr-2 h-4 w-4" />
                          保存
                        </>
                      )}
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
                  <div className="space-y-4">
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
                      className="font-mono text-sm min-h-[600px]"
                      placeholder="配置文件内容..."
                    />
                    <div className="flex items-center justify-between text-sm text-muted-foreground">
                      <span>行数: {configContent.split("\n").length}</span>
                      <span>字符数: {configContent.length}</span>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          </motion.div>
        </TabsContent>
      </Tabs>
    </motion.div>
  );
}

