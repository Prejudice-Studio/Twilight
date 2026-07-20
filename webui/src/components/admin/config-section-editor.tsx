"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Eye, EyeOff, Loader2, Plus, RotateCcw, Save, X } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import type { ConfigField, ConfigSchema, ConfigSection } from "@/lib/api-types";
import { deepClone } from "@/lib/deep-clone";
import { useI18n } from "@/lib/i18n";

function toEditorList(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item ?? ""));
  if (value === null || value === undefined || value === "") return [];
  return [String(value)];
}

function serializeField(field: ConfigField, value: unknown, original: unknown): unknown {
  if (field.type === "list") {
    return toEditorList(value).map((item) => item.trim()).filter(Boolean);
  }
  if (field.type === "int") {
    const parsed = Number.parseInt(String(value), 10);
    return Number.isFinite(parsed) ? parsed : original;
  }
  if (field.type === "float") {
    const parsed = Number.parseFloat(String(value));
    return Number.isFinite(parsed) ? parsed : original;
  }
  if (field.type === "bool") return Boolean(value);
  return value;
}

function isChanged(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) !== JSON.stringify(b);
}

function SecretInput({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const { t } = useI18n();
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative">
      <Input
        type={visible ? "text" : "password"}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="pr-10"
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute right-0 top-0 h-full"
        onClick={() => setVisible((next) => !next)}
        aria-label={visible ? t("adminConfig.sectionEditor.hideSecret") : t("adminConfig.sectionEditor.showSecret")}
      >
        {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </Button>
    </div>
  );
}

function ListInput({ value, onChange }: { value: unknown; onChange: (value: string[]) => void }) {
  const { t } = useI18n();
  const items = toEditorList(value);
  return (
    <div className="space-y-2">
      {items.map((item, index) => (
        <div key={index} className="flex gap-2">
          <Input
            value={item}
            onChange={(event) => {
              const next = [...items];
              next[index] = event.target.value;
              onChange(next);
            }}
          />
          <Button type="button" variant="ghost" size="icon" onClick={() => onChange(items.filter((_, i) => i !== index))}>
            <X className="h-4 w-4" />
          </Button>
        </div>
      ))}
      <Button type="button" variant="outline" size="sm" className="w-full" onClick={() => onChange([...items, ""])}>
        <Plus className="mr-2 h-4 w-4" />
        {t("adminConfig.addItem")}
      </Button>
    </div>
  );
}

function toCommandRows(value: unknown): Array<{ command: string; reply: string }> {
  if (Array.isArray(value)) {
    return value.map((item) => {
      const row = item as Record<string, unknown>;
      return { command: String(row.command ?? ""), reply: String(row.reply ?? "") };
    });
  }
  return [];
}

function CommandMapInput({
  value,
  onChange,
}: {
  value: unknown;
  onChange: (value: Array<{ command: string; reply: string }>) => void;
}) {
  const { t } = useI18n();
  const rows = toCommandRows(value);
  return (
    <div className="space-y-2">
      {rows.map((row, index) => (
        <div key={index} className="grid gap-2 rounded-md border p-3 sm:grid-cols-[minmax(120px,180px)_1fr_auto]">
          <Input
            value={row.command}
            onChange={(event) => {
              const next = [...rows];
              next[index] = { ...row, command: event.target.value };
              onChange(next);
            }}
            placeholder="/command"
          />
          <Textarea
            value={row.reply}
            onChange={(event) => {
              const next = [...rows];
              next[index] = { ...row, reply: event.target.value };
              onChange(next);
            }}
            className="min-h-20"
            placeholder={t("adminConfig.commandReplyPlaceholder")}
          />
          <Button type="button" variant="ghost" size="icon" onClick={() => onChange(rows.filter((_, i) => i !== index))}>
            <X className="h-4 w-4" />
          </Button>
        </div>
      ))}
      <Button type="button" variant="outline" size="sm" className="w-full" onClick={() => onChange([...rows, { command: "/", reply: "" }])}>
        <Plus className="mr-2 h-4 w-4" />
        {t("adminConfig.addCustomCommand")}
      </Button>
    </div>
  );
}

function FieldEditor({
  field,
  value,
  onChange,
}: {
  field: ConfigField;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const hints = field.placeholder_hints ?? [];
  const insertHint = (hint: string) => {
    const active = document.activeElement;
    if (!(active instanceof HTMLTextAreaElement) && !(active instanceof HTMLInputElement)) return;
    const start = active.selectionStart ?? 0;
    const end = active.selectionEnd ?? 0;
    const val = active.value;
    active.value = val.slice(0, start) + hint + val.slice(end);
    active.selectionStart = active.selectionEnd = start + hint.length;
    active.dispatchEvent(new Event("input", { bubbles: true }));
    active.focus();
  };

  const PlaceholderRow = hints.length > 0 ? (
    <div className="flex flex-wrap gap-1">
      {hints.map((hint) => (
        <button
          key={hint}
          type="button"
          className="inline-flex items-center rounded-md border bg-muted/30 px-1.5 py-0.5 font-mono text-[11px] leading-tight hover:bg-muted cursor-pointer transition-colors"
          onClick={() => insertHint(hint)}
          title={`插入 ${hint}`}
        >
          {hint}
        </button>
      ))}
    </div>
  ) : null;

  if (field.type === "bool") {
    return <Switch checked={Boolean(value)} onCheckedChange={onChange} />;
  }
  if (field.type === "textarea") {
    return (
      <div className="space-y-2">
        {PlaceholderRow}
        <Textarea value={String(value ?? "")} onChange={(event) => onChange(event.target.value)} className="min-h-28 font-mono text-sm" />
      </div>
    );
  }
  if (field.type === "secret") {
    return <SecretInput value={String(value ?? "")} onChange={onChange} />;
  }
  if (field.type === "list") {
    return <ListInput value={value} onChange={onChange} />;
  }
  if (field.type === "command_map") {
    return <CommandMapInput value={value} onChange={onChange} />;
  }
  if (field.type === "select") {
    return (
      <div className="grid gap-2 sm:grid-cols-2">
        {(field.options ?? []).map((option) => (
          <Button
            key={String(option.value)}
            type="button"
            variant={String(value) === String(option.value) ? "default" : "outline"}
            className="h-auto min-h-10 justify-start whitespace-normal text-left"
            onClick={() => onChange(option.value)}
          >
            {option.label}
          </Button>
        ))}
      </div>
    );
  }
  return (
    <div className="space-y-2">
      {PlaceholderRow}
      <Input
        type={field.type === "int" || field.type === "float" ? "number" : "text"}
        step={field.type === "float" ? "0.01" : undefined}
        value={String(value ?? "")}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  );
}

export function AdminConfigSections({
  sectionKeys,
  sectionFieldKeys,
  title,
  description,
  notice,
}: {
  sectionKeys: string[];
  sectionFieldKeys?: Record<string, string[]>;
  title: string;
  description: string;
  notice?: string;
}) {
  const { toast } = useToast();
  const { t } = useI18n();
  const [schema, setSchema] = useState<ConfigSchema | null>(null);
  const [values, setValues] = useState<Record<string, Record<string, unknown>>>({});
  const [original, setOriginal] = useState<Record<string, Record<string, unknown>>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const sectionKeysKey = useMemo(() => JSON.stringify(sectionKeys), [sectionKeys]);
  const sectionFieldKeysKey = useMemo(() => JSON.stringify(sectionFieldKeys ?? {}), [sectionFieldKeys]);
  const stableSectionKeys = useMemo(() => JSON.parse(sectionKeysKey) as string[], [sectionKeysKey]);
  const stableSectionFieldKeys = useMemo(
    () => JSON.parse(sectionFieldKeysKey) as Record<string, string[]>,
    [sectionFieldKeysKey],
  );

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.getConfigSchema();
      if (!res.success || !res.data) throw new Error(res.message || t("adminConfig.sectionEditor.loadFailed"));
      const nextValues: Record<string, Record<string, unknown>> = {};
      for (const section of res.data.sections) {
        if (!stableSectionKeys.includes(section.key)) continue;
        nextValues[section.key] = {};
        for (const field of section.fields) {
          const allowedFields = stableSectionFieldKeys[section.key];
          if (allowedFields && !allowedFields.includes(field.key)) continue;
          nextValues[section.key][field.key] = field.type === "list" ? toEditorList(field.value) : field.value;
        }
      }
      setSchema(res.data);
      setValues(deepClone(nextValues));
      setOriginal(deepClone(nextValues));
    } catch (err) {
      setError(err instanceof Error ? err.message : t("adminConfig.sectionEditor.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [stableSectionFieldKeys, stableSectionKeys, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const sections = useMemo<ConfigSection[]>(() => {
    if (!schema) return [];
    return schema.sections
      .filter((section) => stableSectionKeys.includes(section.key))
      .map((section) => {
        const allowedFields = stableSectionFieldKeys[section.key];
        if (!allowedFields) return section;
        return { ...section, fields: section.fields.filter((field) => allowedFields.includes(field.key)) };
      })
      .filter((section) => section.fields.length > 0);
  }, [schema, stableSectionFieldKeys, stableSectionKeys]);

  const changedCount = useMemo(() => {
    let count = 0;
    for (const section of sections) {
      for (const field of section.fields) {
        if (isChanged(values[section.key]?.[field.key], original[section.key]?.[field.key])) count++;
      }
    }
    return count;
  }, [sections, values, original]);

  const save = async () => {
    if (!schema || changedCount === 0) return;
    setSaving(true);
    try {
      const payload: Record<string, Record<string, unknown>> = {};
      for (const section of sections) {
        payload[section.key] = {};
        for (const field of section.fields) {
          payload[section.key][field.key] = serializeField(
            field,
            values[section.key]?.[field.key],
            original[section.key]?.[field.key],
          );
        }
      }
      const res = await api.updateConfigBySchema(payload);
      if (!res.success) throw new Error(res.message || t("adminConfig.sectionEditor.saveFailed"));
      toast({ title: t("adminConfig.sectionEditor.saved"), variant: "success" });
      await load();
    } catch (err) {
      toast({ title: t("adminConfig.sectionEditor.saveFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <div className="flex h-64 items-center justify-center"><Loader2 className="h-8 w-8 animate-spin text-muted-foreground" /></div>;
  }

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>{t("adminConfig.sectionEditor.loadFailed")}</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{title}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => setValues(deepClone(original))} disabled={saving || changedCount === 0}>
            <RotateCcw className="mr-2 h-4 w-4" />
            {t("common.reset")}
          </Button>
          <Button onClick={() => void save()} disabled={saving || changedCount === 0}>
            {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
            {t("common.save")}
            {changedCount > 0 && <Badge className="ml-2" variant="secondary">{changedCount}</Badge>}
          </Button>
        </div>
      </div>

      {notice && (
        <Alert>
          <AlertTitle>{t("adminConfig.sectionEditor.migrationNotice")}</AlertTitle>
          <AlertDescription>{notice}</AlertDescription>
        </Alert>
      )}

      {sections.map((section) => (
        <Card key={section.key}>
          <CardHeader>
            <CardTitle>{section.title}</CardTitle>
            <CardDescription>{section.description}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {section.fields.map((field) => {
              const value = values[section.key]?.[field.key];
              const changed = isChanged(value, original[section.key]?.[field.key]);
              return (
                <div key={field.key} className="grid gap-2 rounded-lg border p-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="min-w-0">
                      <Label className="font-medium">{field.label}</Label>
                      <p className="mt-1 text-xs text-muted-foreground">{field.description}</p>
                    </div>
                    {changed && <Badge variant="warning">{t("adminConfig.changed")}</Badge>}
                  </div>
                  <FieldEditor
                    field={field}
                    value={value}
                    onChange={(next) => {
                      setValues((prev) => ({
                        ...prev,
                        [section.key]: { ...prev[section.key], [field.key]: next },
                      }));
                    }}
                  />
                </div>
              );
            })}
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
