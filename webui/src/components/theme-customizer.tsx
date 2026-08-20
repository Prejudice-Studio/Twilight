"use client";

import { useState, useEffect, useCallback } from "react";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { useToast } from "@/hooks/use-toast";
import { useI18n } from "@/lib/i18n";
import {
  getThemeCustom,
  setThemeCustom,
  applyThemeCustom,
  resetThemeCustom,
  type ThemeCustom,
} from "@/lib/theme-custom";
import { Loader2, Save } from "lucide-react";

const primaryPresets = ["#334155", "#2563eb", "#0891b2", "#059669", "#dc2626", "#ea580c", "#0f766e", "#111827"];
const accentPresets = ["#e2e8f0", "#dbeafe", "#cffafe", "#dcfce7", "#fee2e2", "#ffedd5", "#ccfbf1", "#e5e7eb"];

function normalizeHexColor(value: string): string {
  const trimmed = value.trim();
  const short = /^#([0-9a-fA-F]{3})$/.exec(trimmed);
  if (short) {
    const [r, g, b] = short[1].split("");
    return `#${r}${r}${g}${g}${b}${b}`.toLowerCase();
  }
  if (/^#[0-9a-fA-F]{6}$/.test(trimmed)) return trimmed.toLowerCase();
  return "#000000";
}

function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const normalized = normalizeHexColor(hex);
  return {
    r: Number.parseInt(normalized.slice(1, 3), 16),
    g: Number.parseInt(normalized.slice(3, 5), 16),
    b: Number.parseInt(normalized.slice(5, 7), 16),
  };
}

function rgbToHex(r: number, g: number, b: number): string {
  const toHex = (value: number) => Math.min(255, Math.max(0, value)).toString(16).padStart(2, "0");
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`;
}

function parseRgbInput(value: string): number {
  const parsed = Number.parseInt(value, 10);
  if (Number.isNaN(parsed)) return 0;
  return Math.min(255, Math.max(0, parsed));
}

function ThemeColorControl({
  label,
  description,
  value,
  presets,
  onChange,
}: {
  label: string;
  description: string;
  value: string;
  presets: string[];
  onChange: (value: string) => void;
}) {
  const color = normalizeHexColor(value);
  const rgb = hexToRgb(color);

  const updateChannel = (channel: "r" | "g" | "b", nextValue: string) => {
    const next = { ...rgb, [channel]: parseRgbInput(nextValue) };
    onChange(rgbToHex(next.r, next.g, next.b));
  };

  return (
    <div className="grid gap-4 rounded-xl border bg-muted/20 p-3 sm:grid-cols-[auto_minmax(0,1fr)]">
      <div className="flex items-start gap-3">
        <Input
          type="color"
          value={color}
          onChange={(event) => onChange(normalizeHexColor(event.target.value))}
          aria-label={label}
          className="theme-color-input h-16 w-16 shrink-0 cursor-pointer rounded-none border border-border bg-transparent p-0"
        />
        <div className="min-w-0 sm:hidden">
          <Label className="text-sm font-medium">{label}</Label>
          <p className="text-xs text-muted-foreground">{description}</p>
        </div>
      </div>

      <div className="min-w-0 space-y-3">
        <div className="hidden sm:block">
          <Label className="text-sm font-medium">{label}</Label>
          <p className="text-xs text-muted-foreground">{description}</p>
        </div>

        <div className="grid grid-cols-3 gap-2">
          {(["r", "g", "b"] as const).map((channel) => (
            <div key={channel} className="space-y-1">
              <Label className="text-[11px] font-semibold uppercase text-muted-foreground">{channel}</Label>
              <Input
                type="number"
                min={0}
                max={255}
                value={rgb[channel]}
                onChange={(event) => updateChannel(channel, event.target.value)}
                inputSize="sm"
                className="tabular-nums"
              />
            </div>
          ))}
        </div>

        <div className="flex flex-wrap gap-2">
          {presets.map((preset) => {
            const selected = preset.toLowerCase() === color;
            return (
              <button
                key={preset}
                type="button"
                className={`h-8 w-8 border transition-transform hover:scale-105 ${
                  selected ? "border-foreground ring-2 ring-ring ring-offset-2 ring-offset-background" : "border-border"
                }`}
                style={{ backgroundColor: preset }}
                onClick={() => onChange(preset)}
                aria-label={preset}
              />
            );
          })}
        </div>
      </div>
    </div>
  );
}

// 表单实时预览但不同步 localStorage；点击「保存」才持久化。
export default function ThemeCustomizer() {
  const { t } = useI18n();
  const { toast } = useToast();

  const [draft, setDraft] = useState<ThemeCustom>(() => getThemeCustom());
  const [saved, setSaved] = useState<ThemeCustom>(() => getThemeCustom());
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const current = getThemeCustom();
    setDraft(current);
    setSaved(current);
  }, []);

  // 滑条/开关变更 → 更新草稿 + 实时预览（不写 localStorage）
  const change = useCallback((patch: Partial<ThemeCustom>) => {
    setDraft((prev) => {
      const next = { ...prev, ...patch };
      applyThemeCustom(next);
      return next;
    });
  }, []);

  // 保存 → 写 localStorage
  const save = useCallback(() => {
    setSaving(true);
    try {
      const persisted = setThemeCustom(draft);
      setSaved(persisted);
      toast({ title: t("appearance.theme.saved"), variant: "success" });
    } catch {
      toast({ title: t("appearance.saveFailedRetry"), variant: "destructive" });
    } finally {
      setSaving(false);
    }
  }, [draft, t, toast]);

  // 撤销草稿 → 回到上次保存的状态
  const revert = useCallback(() => {
    const current = saved;
    setDraft(current);
    applyThemeCustom(current);
  }, [saved]);

  // 恢复默认（预置值）并自动保存
  const resetToDefaults = useCallback(() => {
    const defs = resetThemeCustom();
    setDraft(defs);
    setSaved(defs);
    toast({ title: t("appearance.theme.resetDone"), variant: "success" });
  }, [t, toast]);

  const isDirty =
    draft.primaryColor !== saved.primaryColor ||
    draft.accentColor !== saved.accentColor ||
    draft.radius !== saved.radius ||
    draft.glassBlur !== saved.glassBlur ||
    draft.compact !== saved.compact ||
    draft.reduceMotion !== saved.reduceMotion;

  return (
    <div className="space-y-8">
      {/* 强调色 / 主色 */}
      <div className="grid gap-3 xl:grid-cols-2">
        <ThemeColorControl
          label={t("appearance.theme.primaryLabel")}
          description={t("appearance.theme.primaryDesc")}
          value={draft.primaryColor}
          presets={primaryPresets}
          onChange={(value) => change({ primaryColor: value })}
        />
        <ThemeColorControl
          label={t("appearance.theme.accentLabel")}
          description={t("appearance.theme.accentDesc")}
          value={draft.accentColor}
          presets={accentPresets}
          onChange={(value) => change({ accentColor: value })}
        />
      </div>

      {/* 圆角 */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <Label className="text-sm font-medium">{t("appearance.theme.radiusLabel")}</Label>
          <span className="text-xs tabular-nums text-muted-foreground">
            {draft.radius.toFixed(2)}rem
          </span>
        </div>
        <input
          type="range"
          min={0.25}
          max={2.0}
          step={0.05}
          value={draft.radius}
          onChange={(e) => change({ radius: Number(e.target.value) })}
          className="w-full"
          aria-label={t("appearance.theme.radiusLabel")}
        />
      </div>

      {/* 玻璃模糊 */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <Label className="text-sm font-medium">{t("appearance.theme.glassBlurLabel")}</Label>
          <span className="text-xs tabular-nums text-muted-foreground">
            {draft.glassBlur}px
          </span>
        </div>
        <input
          type="range"
          min={0}
          max={32}
          step={1}
          value={draft.glassBlur}
          onChange={(e) => change({ glassBlur: Number(e.target.value) })}
          className="w-full"
          aria-label={t("appearance.theme.glassBlurLabel")}
        />
      </div>

      {/* 紧凑模式 */}
      <div className="flex min-h-14 items-center justify-between gap-4 rounded-lg border bg-muted/20 p-3">
        <div className="min-w-0 space-y-0.5">
          <Label className="text-sm font-medium">{t("appearance.theme.compactLabel")}</Label>
          <p className="text-xs text-muted-foreground">{t("appearance.theme.compactDesc")}</p>
        </div>
        <Switch
          checked={draft.compact}
          onCheckedChange={(v) => change({ compact: v })}
        />
      </div>

      <div className="flex min-h-14 items-center justify-between gap-4 rounded-lg border bg-muted/20 p-3">
        <div className="min-w-0 space-y-0.5">
          <Label className="text-sm font-medium">{t("appearance.theme.reduceMotionLabel")}</Label>
          <p className="text-xs text-muted-foreground">{t("appearance.theme.reduceMotionDesc")}</p>
        </div>
        <Switch
          checked={draft.reduceMotion}
          onCheckedChange={(v) => change({ reduceMotion: v })}
        />
      </div>

      {/* 操作按钮 */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap gap-2">
          <Button
            onClick={save}
            disabled={!isDirty || saving}
            size="sm"
            className="min-h-9 whitespace-normal text-left leading-tight"
          >
            {saving ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Save className="mr-2 h-4 w-4" />
            )}
            {t("appearance.theme.save")}
          </Button>
          {isDirty && (
            <Button
              variant="outline"
              size="sm"
              onClick={revert}
              disabled={saving}
              className="min-h-9 whitespace-normal leading-tight"
            >
              {t("common.cancel")}
            </Button>
          )}
        </div>
        <button
          type="button"
          onClick={resetToDefaults}
          className="text-xs text-muted-foreground underline underline-offset-4 hover:text-foreground"
        >
          {t("appearance.theme.reset")}
        </button>
      </div>
    </div>
  );
}
