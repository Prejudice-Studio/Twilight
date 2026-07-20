// 用户管理页面的 Dialog 组件集合。
// 拆出来的目的：page.tsx 中堆叠了十几个 <Dialog>（编辑 / 重置密码 / 强制改 Emby 密码 /
// 批量到期 / 批量启用 / 删除 / 启停 等），主组件读起来需要在 JSX 内来回滚动。
// 这里按"无副作用 + 通过 props 注入交互"的原则只挪 JSX，主组件依旧持有 state；
// 行为完全保持向后兼容，新人接手时只需读 props 类型就能掌握每个对话框的契约。
import { AlertTriangle, CalendarClock, Link2, Loader2, Send, UserCheck, UserPlus } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { api, type UserInfo } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export interface EditUserForm {
  role: number;
  emby_id: string;
  active: boolean;
}

/**
 * 编辑用户基本信息对话框：角色 / Emby ID / 启停。
 * - 角色枚举与后端 store.Role 一致（0=管理员 / 1=普通 / 2=白名单）
 * - Emby ID 留空表示"清除绑定"，由后端 update 流程兜底校验
 * - 提交按钮在 isLoading 时禁用，避免双击重复请求
 */
export function EditUserDialog({
  open,
  onOpenChange,
  user,
  form,
  onFormChange,
  onSubmit,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  user: UserInfo | null;
  form: EditUserForm;
  onFormChange: (next: EditUserForm) => void;
  onSubmit: () => void;
  isLoading: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>编辑用户信息</DialogTitle>
          <DialogDescription>
            编辑用户 {user?.username} 的详细信息
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>角色</Label>
            <Select
              value={form.role.toString()}
              onValueChange={(v) => onFormChange({ ...form, role: parseInt(v) })}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="0">管理员</SelectItem>
                <SelectItem value="1">普通用户</SelectItem>
                <SelectItem value="2">白名单</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>Emby ID（可选）</Label>
            <Input
              placeholder="输入 Emby 用户 ID"
              value={form.emby_id}
              onChange={(e) => onFormChange({ ...form, emby_id: e.target.value })}
            />
          </div>

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="active"
              checked={form.active}
              onChange={(e) => onFormChange({ ...form, active: e.target.checked })}
              className="h-4 w-4 rounded border-gray-300"
            />
            <Label htmlFor="active" className="cursor-pointer">
              启用账号
            </Label>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={onSubmit} disabled={isLoading}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            保存更改
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export interface WebUserCreateForm {
  username: string;
  password: string;
  email: string;
  role: number;
}

export function WebUserCreateDialog({
  open,
  onOpenChange,
  form,
  onFormChange,
  onSubmit,
  isSubmitting,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  form: WebUserCreateForm;
  onFormChange: (next: WebUserCreateForm) => void;
  onSubmit: () => void;
  isSubmitting: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <UserPlus className="h-5 w-5" />
            新建 Web 账号
          </DialogTitle>
          <DialogDescription>
            仅创建 Twilight 本地账号，不创建或绑定 Emby 账号。密码留空时后端会生成一次性强密码。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="web-user-username">用户名</Label>
            <Input
              id="web-user-username"
              value={form.username}
              onChange={(e) => onFormChange({ ...form, username: e.target.value })}
              placeholder="如 alice"
              disabled={isSubmitting}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="web-user-password">密码（可选）</Label>
            <Input
              id="web-user-password"
              type="password"
              value={form.password}
              onChange={(e) => onFormChange({ ...form, password: e.target.value })}
              placeholder="留空自动生成"
              disabled={isSubmitting}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="web-user-email">邮箱（可选）</Label>
            <Input
              id="web-user-email"
              value={form.email}
              onChange={(e) => onFormChange({ ...form, email: e.target.value })}
              placeholder="user@example.com"
              disabled={isSubmitting}
            />
          </div>
          <div className="space-y-2">
            <Label>角色</Label>
            <Select
              value={String(form.role)}
              onValueChange={(value) => onFormChange({ ...form, role: Number(value) })}
              disabled={isSubmitting}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="1">普通用户</SelectItem>
                <SelectItem value="2">白名单</SelectItem>
                <SelectItem value="0">管理员</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isSubmitting}>
            取消
          </Button>
          <Button onClick={onSubmit} disabled={isSubmitting}>
            {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            创建
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 新建独立 Emby 账号对话框：直接调用 Emby API 创建账号，不写入本地用户表，
 * 不参与到期 / 权限管理。仅做受控输入 + 提交按钮，加载态由父组件控制。
 */
export interface StandaloneEmbyForm {
  name: string;
  pwd: string;
  email: string;
}

export function StandaloneEmbyCreateDialog({
  open,
  onOpenChange,
  form,
  onFormChange,
  onSubmit,
  isSubmitting,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  form: StandaloneEmbyForm;
  onFormChange: (next: StandaloneEmbyForm) => void;
  onSubmit: () => void;
  isSubmitting: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <UserPlus className="h-5 w-5" />
            新建独立 Emby 账号
          </DialogTitle>
          <DialogDescription>
            直接调用 Emby API 创建一个新账号；该账号不会写入本地用户表、不参与 Twilight 系统的过期/权限管理。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="standalone-name">Emby 用户名</Label>
            <Input
              id="standalone-name"
              value={form.name}
              onChange={(e) => onFormChange({ ...form, name: e.target.value })}
              placeholder="如 guest123"
              disabled={isSubmitting}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="standalone-pwd">Emby 密码</Label>
            <Input
              id="standalone-pwd"
              type="password"
              value={form.pwd}
              onChange={(e) => onFormChange({ ...form, pwd: e.target.value })}
              placeholder="至少 8 位，含大小写和数字"
              disabled={isSubmitting}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="standalone-email">邮箱（可选）</Label>
            <Input
              id="standalone-email"
              value={form.email}
              onChange={(e) => onFormChange({ ...form, email: e.target.value })}
              placeholder="仅做备注，不会同步到 Emby"
              disabled={isSubmitting}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            取消
          </Button>
          <Button onClick={onSubmit} disabled={isSubmitting}>
            {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            创建
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 启停用户对话框：禁用 / 启用 + 邀请树级联深度。
 * - cascadeDepth=1 仅处理本人；=-1 整棵子树；其他正整数表示向下 N 层
 * - 自定义输入框被钳制在 [0, 999]
 * - 仅在禁用场景显示"原因"输入框（启用不需要理由）
 */
export function ToggleActiveDialog({
  open,
  onOpenChange,
  target,
  reason,
  onReasonChange,
  cascadeDepth,
  onCascadeDepthChange,
  onConfirm,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  target: UserInfo | null;
  reason: string;
  onReasonChange: (next: string) => void;
  cascadeDepth: number;
  onCascadeDepthChange: (next: number) => void;
  onConfirm: () => void;
  isLoading: boolean;
}) {
  const isActive = Boolean(target?.active);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isActive ? "禁用" : "启用"}用户 {target?.username}
          </DialogTitle>
          <DialogDescription>
            {isActive
              ? "禁用后用户将无法登录系统与 Emby，但邀请树结构（上下级关系、已发出邀请码）完全不变；重新启用即可恢复访问。"
              : "启用后用户可以重新登录系统与 Emby。"}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 text-sm">
          {isActive && (
            <div className="space-y-1.5">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">
                原因（可选）
              </Label>
              <Input
                value={reason}
                onChange={(e) => onReasonChange(e.target.value)}
                placeholder="将记入操作日志，留空也可"
                maxLength={200}
              />
            </div>
          )}

          <div className="space-y-2 rounded-md border bg-muted/30 p-3">
            <div className="flex items-center justify-between gap-2">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">
                邀请树级联深度
              </Label>
              <span className="text-[10px] text-muted-foreground">
                当前：{cascadeDepth === -1 ? "整棵子树" : `${cascadeDepth} 层`}
              </span>
            </div>

            <div className="flex flex-wrap gap-1.5">
              {[1, 2, 3, 5, -1].map((preset) => (
                <Button
                  key={preset}
                  type="button"
                  size="sm"
                  variant={cascadeDepth === preset ? "default" : "outline"}
                  className="h-7 px-2 text-[11px]"
                  onClick={() => onCascadeDepthChange(preset)}
                >
                  {preset === -1 ? "全部" : `${preset} 层`}
                </Button>
              ))}
            </div>

            <Input
              type="number"
              inputMode="numeric"
              min={-1}
              max={999}
              value={cascadeDepth}
              onChange={(e) => {
                const v = parseInt(e.target.value, 10);
                if (Number.isNaN(v)) {
                  onCascadeDepthChange(1);
                } else {
                  onCascadeDepthChange(Math.max(-1, Math.min(999, v)));
                }
              }}
              placeholder="自定义层级，输入 -1 表示整棵子树"
              className="h-9"
            />

            <p className="text-[11px] text-muted-foreground leading-relaxed">
              {cascadeDepth === 1 ? (
                <>仅处理该用户本人；下级账号不受影响。</>
              ) : cascadeDepth === -1 ? (
                <>
                  将
                  <span className="font-semibold text-foreground">
                    {isActive ? "禁用" : "启用"}
                  </span>
                  该用户及其全部后代（沿邀请关系递归）。已经处于目标状态的会被跳过。
                </>
              ) : (
                <>
                  将{isActive ? "禁用" : "启用"}该用户及向下 {cascadeDepth - 1} 层的所有下级；
                  其他管理员账号会被跳过。
                </>
              )}
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isLoading}>
            取消
          </Button>
          <Button
            variant={isActive ? "destructive" : "default"}
            onClick={onConfirm}
            disabled={isLoading}
          >
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {isActive ? "确认禁用" : "确认启用"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 续期 / 取消永久 / 设置到期对话框：复用同一表单，由 mode 决定标题和文案。
 * - mode="renew"：叠加式续期，在现有到期时间上 +N 天；支持"永久有效"开关。
 * - mode="cancelPermanent"：把永久号改成「从现在起 N 天后」到期（绝对式覆盖）；
 *   不显示"永久"开关。
 * - mode="setExact"：把任意用户的剩余到期时间直接设为精确值（从现在起 N 天后，
 *   覆盖而非叠加）；支持"永久有效"开关。这是「手动设置剩余到期时间」的入口。
 */
export function RenewUserDialog({
  open,
  onOpenChange,
  user,
  mode,
  days,
  onDaysChange,
  permanent,
  onPermanentChange,
  onSubmit,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  user: UserInfo | null;
  mode: "renew" | "cancelPermanent" | "setExact";
  days: string;
  onDaysChange: (next: string) => void;
  permanent: boolean;
  onPermanentChange: (next: boolean) => void;
  onSubmit: () => void;
  isLoading: boolean;
}) {
  const title =
    mode === "cancelPermanent"
      ? "取消永久有效期"
      : mode === "setExact"
        ? "设置到期时间"
        : "用户续期";
  const description =
    mode === "cancelPermanent"
      ? `将用户 ${user?.username} 改为指定天数后到期`
      : mode === "setExact"
        ? `将用户 ${user?.username} 的到期时间直接设为「从现在起 N 天后」（覆盖现有时间，而非在原有基础上叠加）`
        : `为用户 ${user?.username} 延长账号时间`;
  const daysLabel =
    mode === "renew" ? "续期天数" : "从现在起多少天后到期";
  const submitLabel =
    mode === "cancelPermanent"
      ? "确认取消永久"
      : mode === "setExact"
        ? "确认设置"
        : "确认续期";
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next) onPermanentChange(false);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{daysLabel}</Label>
            <Input
              type="number"
              min={1}
              placeholder="输入天数"
              value={days}
              onChange={(e) => onDaysChange(e.target.value)}
              disabled={permanent}
            />
          </div>
          {mode !== "cancelPermanent" && (
            <label className="flex items-center gap-2 rounded-md border bg-muted/30 p-3 text-sm">
              <input
                type="checkbox"
                checked={permanent}
                onChange={(e) => onPermanentChange(e.target.checked)}
                className="h-4 w-4"
              />
              设置为永久有效
            </label>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={onSubmit} disabled={isLoading}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 一键批量调控到期时间对话框：根据当前筛选条件批量覆盖普通用户的到期时间。
 * - 默认跳过管理员、白名单与未开通 Emby 的账号
 * - 必须输入"确认"二字才能解锁执行按钮
 * - 当前筛选概览由父组件传入 filterSummary 字符串数组
 */
export interface BulkExpireFilterSummary {
  role: string;
  active: string;
  emby: string;
}

export function BulkExpireDialog({
  open,
  onOpenChange,
  mode,
  onModeChange,
  days,
  onDaysChange,
  includeAdmin,
  onIncludeAdminChange,
  includeWhitelist,
  onIncludeWhitelistChange,
  confirmText,
  onConfirmTextChange,
  filterSummary,
  onSubmit,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  mode: "permanent" | "days";
  onModeChange: (next: "permanent" | "days") => void;
  days: string;
  onDaysChange: (next: string) => void;
  includeAdmin: boolean;
  onIncludeAdminChange: (next: boolean) => void;
  includeWhitelist: boolean;
  onIncludeWhitelistChange: (next: boolean) => void;
  confirmText: string;
  onConfirmTextChange: (next: string) => void;
  filterSummary: BulkExpireFilterSummary;
  onSubmit: () => void;
  isLoading: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <CalendarClock className="h-5 w-5" />
            一键批量调控到期时间
          </DialogTitle>
          <DialogDescription>
            将根据上方筛选条件（角色/状态/Emby 绑定）批量覆盖普通用户的到期时间。
            管理员、白名单与未开通 Emby 的账号默认会被跳过，避免误伤。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 text-sm">
          <div className="space-y-2">
            <Label className="text-xs uppercase tracking-wider text-muted-foreground">目标到期时间</Label>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                size="sm"
                variant={mode === "permanent" ? "default" : "outline"}
                onClick={() => onModeChange("permanent")}
              >
                永久
              </Button>
              <div className="flex items-center gap-1.5">
                <Button
                  type="button"
                  size="sm"
                  variant={mode === "days" ? "default" : "outline"}
                  onClick={() => onModeChange("days")}
                >
                  自定义天数
                </Button>
                <Input
                  type="number"
                  min={1}
                  value={days}
                  onChange={(e) => {
                    onDaysChange(e.target.value);
                    onModeChange("days");
                  }}
                  placeholder="天数"
                  className="h-8 w-24"
                />
                <span className="text-xs text-muted-foreground">天</span>
              </div>
            </div>
            <p className="text-[11px] text-muted-foreground">
              {mode === "permanent"
                ? "将设置为永久（EXPIRED_AT=-1）。"
                : "从当前时间起 N 天后过期。"}
            </p>
          </div>

          <div className="rounded-md border bg-muted/30 p-3 space-y-2">
            <p className="text-xs font-medium">作用范围（当前筛选条件）</p>
            <div className="text-xs text-muted-foreground space-y-0.5">
              <p>角色：{filterSummary.role}</p>
              <p>启用状态：{filterSummary.active}</p>
              <p>Emby 绑定：{filterSummary.emby}</p>
            </div>
          </div>

          <div className="space-y-2">
            <Label className="text-xs uppercase tracking-wider text-muted-foreground">额外选项</Label>
            <div className="space-y-1.5">
              <label className="flex items-start gap-2 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={includeAdmin}
                  onChange={(e) => onIncludeAdminChange(e.target.checked)}
                  className="mt-0.5 h-4 w-4 rounded"
                />
                <span>
                  包含管理员账号（默认跳过；包含后<strong>仍不会改你自己</strong>，除非把「永久」选项手动改成具体天数前请慎重）
                </span>
              </label>
              <label className="flex items-start gap-2 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={includeWhitelist}
                  onChange={(e) => onIncludeWhitelistChange(e.target.checked)}
                  className="mt-0.5 h-4 w-4 rounded"
                />
                <span>包含白名单用户（一旦取消勾选，白名单的「永久」标签将被覆盖）</span>
              </label>
              <p className="rounded-md border border-muted-foreground/20 bg-muted/40 px-3 py-2 text-[11px] text-muted-foreground">
                ⚠ 未绑定 Emby 的账号一律强制跳过，「未开通」sentinel（EXPIRED_AT=0）由系统保护，无法通过批量操作覆盖。
              </p>
            </div>
          </div>

          <div className="space-y-1.5 rounded-md border border-destructive/40 bg-destructive/5 p-3">
            <Label className="text-xs uppercase tracking-wider text-destructive">
              二次确认
            </Label>
            <p className="text-xs text-muted-foreground">
              请在下方输入 <span className="font-mono text-foreground">确认</span> 二字以继续：
            </p>
            <Input
              value={confirmText}
              onChange={(e) => onConfirmTextChange(e.target.value)}
              placeholder="确认"
              className="h-9"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isLoading}>
            取消
          </Button>
          <Button
            variant="destructive"
            onClick={onSubmit}
            disabled={isLoading || confirmText.trim() !== "确认"}
          >
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            执行批量调控
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 批量启用已禁用账号对话框：按当前筛选查找已禁用账号 + 同步启用 Emby。
 * - 默认跳过管理员 / 白名单
 * - 输入"确认"才能解锁；外部可通过 blocked 标记额外锁定（如未匹配任何账号）
 */
export interface BulkEnableFilterSummary {
  search: string;
  role: string;
  active: string;
  emby: string;
}

export function BulkEnableDialog({
  open,
  onOpenChange,
  includeAdmin,
  onIncludeAdminChange,
  includeWhitelist,
  onIncludeWhitelistChange,
  confirmText,
  onConfirmTextChange,
  filterSummary,
  blocked,
  onSubmit,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  includeAdmin: boolean;
  onIncludeAdminChange: (next: boolean) => void;
  includeWhitelist: boolean;
  onIncludeWhitelistChange: (next: boolean) => void;
  confirmText: string;
  onConfirmTextChange: (next: string) => void;
  filterSummary: BulkEnableFilterSummary;
  blocked: boolean;
  onSubmit: () => void;
  isLoading: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <UserCheck className="h-5 w-5" />
            批量启用已禁用账号
          </DialogTitle>
          <DialogDescription>
            将按当前搜索、角色、状态与 Emby 筛选查找已禁用账号，并同步启用对应 Emby 账户。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 text-sm">
          <div className="rounded-md border bg-muted/30 p-3 space-y-2">
            <p className="text-xs font-medium">作用范围（当前筛选条件）</p>
            <div className="text-xs text-muted-foreground space-y-0.5">
              <p>搜索：{filterSummary.search}</p>
              <p>角色：{filterSummary.role}</p>
              <p>启用状态：{filterSummary.active}</p>
              <p>Emby 绑定：{filterSummary.emby}</p>
            </div>
          </div>

          <div className="space-y-2">
            <Label className="text-xs uppercase tracking-wider text-muted-foreground">额外选项</Label>
            <div className="space-y-1.5">
              <label className="flex items-start gap-2 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={includeAdmin}
                  onChange={(e) => onIncludeAdminChange(e.target.checked)}
                  className="mt-0.5 h-4 w-4 rounded"
                />
                <span>包含管理员账号（默认跳过，避免误恢复管理员访问权限）</span>
              </label>
              <label className="flex items-start gap-2 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={includeWhitelist}
                  onChange={(e) => onIncludeWhitelistChange(e.target.checked)}
                  className="mt-0.5 h-4 w-4 rounded"
                />
                <span>包含白名单用户（默认跳过）</span>
              </label>
              <p className="rounded-md border border-muted-foreground/20 bg-muted/40 px-3 py-2 text-[11px] text-muted-foreground">
                未识别角色会被后端强制跳过；单次最多启用 5000 个账号。
              </p>
            </div>
          </div>

          <div className="space-y-1.5 rounded-md border border-amber-500/40 bg-amber-500/5 p-3">
            <Label className="text-xs uppercase tracking-wider text-amber-700 dark:text-amber-300">
              二次确认
            </Label>
            <p className="text-xs text-muted-foreground">
              请在下方输入 <span className="font-mono text-foreground">确认</span> 二字以继续：
            </p>
            <Input
              value={confirmText}
              onChange={(e) => onConfirmTextChange(e.target.value)}
              placeholder="确认"
              className="h-9"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isLoading}>
            取消
          </Button>
          <Button
            onClick={onSubmit}
            disabled={isLoading || confirmText.trim() !== "确认" || blocked}
          >
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            执行批量启用
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 删除用户对话框（支持邀请树级联）。
 * - scope: with_emby（含 Emby）/ local_only（仅本地）/ emby_only（仅 Emby）
 *   只有目标已绑定 Emby 时才显示 with_emby / emby_only 选项
 * - cascadeDepth=1 仅本人；=-1 全子树；其他正整数表示向下 N 层
 * - emby_only + cascadeDepth!==1 时给出二次提醒（仅删 Emby，本地保留）
 */
export type DeleteScope = "with_emby" | "local_only" | "emby_only";

export function DeleteUserDialog({
  open,
  onOpenChange,
  target,
  scope,
  onScopeChange,
  cascadeDepth,
  onCascadeDepthChange,
  onConfirm,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  target: UserInfo | null;
  scope: DeleteScope;
  onScopeChange: (next: DeleteScope) => void;
  cascadeDepth: number;
  onCascadeDepthChange: (next: number) => void;
  onConfirm: () => void;
  isLoading: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>删除用户 {target?.username}</DialogTitle>
          <DialogDescription>
            本地账户与（可选）Emby 账户的删除不可恢复。请同时选择是否级联删除该用户邀请的下级。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 text-sm">
          <div className="space-y-2">
            <Label className="text-xs uppercase tracking-wider text-muted-foreground">删除范围</Label>
            <Select
              value={scope}
              onValueChange={(v) => onScopeChange(v as DeleteScope)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {target?.emby_id && (
                  <SelectItem value="with_emby">同时删除本地账户 + Emby 账户</SelectItem>
                )}
                <SelectItem value="local_only">仅删除本地账户</SelectItem>
                {target?.emby_id && (
                  <SelectItem value="emby_only">仅删除 Emby 账户（不动本地）</SelectItem>
                )}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2 rounded-md border bg-muted/30 p-3">
            <div className="flex items-center justify-between gap-2">
              <Label className="text-xs uppercase tracking-wider text-muted-foreground">
                邀请树级联深度
              </Label>
              <span className="text-[10px] text-muted-foreground">
                当前：{cascadeDepth === -1 ? "整棵子树" : `${cascadeDepth} 层`}
              </span>
            </div>

            <div className="flex flex-wrap gap-1.5">
              {[1, 2, 3, 5, -1].map((preset) => (
                <Button
                  key={preset}
                  type="button"
                  size="sm"
                  variant={cascadeDepth === preset ? "default" : "outline"}
                  className="h-7 px-2 text-[11px]"
                  onClick={() => onCascadeDepthChange(preset)}
                >
                  {preset === -1 ? "全部" : `${preset} 层`}
                </Button>
              ))}
            </div>

            <div className="flex items-center gap-2">
              <Input
                type="number"
                inputMode="numeric"
                min={-1}
                max={999}
                value={cascadeDepth}
                onChange={(e) => {
                  const v = parseInt(e.target.value, 10);
                  if (Number.isNaN(v)) {
                    onCascadeDepthChange(1);
                  } else {
                    onCascadeDepthChange(Math.max(-1, Math.min(999, v)));
                  }
                }}
                placeholder="自定义层级，输入 -1 表示整棵子树"
                className="h-9"
              />
            </div>

            <p className="text-[11px] text-muted-foreground leading-relaxed">
              {cascadeDepth === 1 ? (
                <>仅处理该用户本人；被他邀请的下级会自动成为新树根，互相之间不再有上下级关系。</>
              ) : cascadeDepth === -1 ? (
                <>
                  将沿邀请关系，一路处理该用户及其
                  <span className="font-semibold text-foreground"> 全部后代</span>
                  （
                  {scope === "with_emby"
                    ? "本地账号 + Emby 账号"
                    : scope === "local_only"
                      ? "仅本地账号，Emby 保留"
                      : "仅 Emby 账号，本地账号与邀请关系保留"}
                  ）。请二次确认！
                </>
              ) : (
                <>
                  将一并处理该用户向下 {cascadeDepth - 1} 层的所有下级（
                  {scope === "with_emby"
                    ? "本地 + Emby"
                    : scope === "local_only"
                      ? "仅本地"
                      : "仅 Emby"}
                  ）。
                </>
              )}
            </p>
            {scope === "emby_only" && cascadeDepth !== 1 && (
              <p className="text-[11px] text-amber-600 dark:text-amber-400">
                注意：「仅删除 Emby」级联只会删除下级用户的 Emby 账号，本地账号与邀请关系完全保留。
              </p>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isLoading}>
            取消
          </Button>
          <Button variant="destructive" onClick={onConfirm} disabled={isLoading}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            确认删除
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 强制绑定 Emby 对话框：管理员手动把已存在的 Emby 账号绑到本地账号。
 * - 出现绑定冲突（该 Emby 已属其它账号）时父组件传入 conflict，按钮变为"夺取"
 * - 父组件可以通过 onUsernameChange 回调清掉 conflict 缓存
 */
export interface BindEmbyConflict {
  emby_username: string;
  conflict_username: string;
  conflict_uid: number | string;
}

export function BindEmbyDialog({
  open,
  onOpenChange,
  target,
  embyName,
  onEmbyNameChange,
  conflict,
  onSubmit,
  isSubmitting,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  target: UserInfo | null;
  embyName: string;
  onEmbyNameChange: (next: string) => void;
  conflict: BindEmbyConflict | null;
  onSubmit: (force: boolean) => void;
  isSubmitting: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Link2 className="h-5 w-5" />
            绑定 Emby 账号
          </DialogTitle>
          <DialogDescription>
            将一个已存在的 Emby 账号强制绑定到系统账号 {target?.username}（UID {target?.uid}）。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 py-2">
          <div className="space-y-2">
            <Label htmlFor="bind-emby-name">Emby 用户名</Label>
            <Input
              id="bind-emby-name"
              value={embyName}
              onChange={(e) => onEmbyNameChange(e.target.value)}
              placeholder="输入要绑定的 Emby 用户名"
              disabled={isSubmitting}
            />
            <p className="text-xs text-muted-foreground">
              若该 Emby 已被其他系统账号占用，将出现确认按钮以「夺取」绑定。
            </p>
          </div>

          {conflict && (
            <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
              <div className="mb-1 flex items-center gap-2 text-amber-600 dark:text-amber-400">
                <AlertTriangle className="h-4 w-4" />
                <span className="font-medium">绑定冲突</span>
              </div>
              <p>
                Emby 用户 <span className="font-mono">{conflict.emby_username}</span> 当前已绑定到
                系统账号 <span className="font-mono">{conflict.conflict_username}</span>
                （UID {conflict.conflict_uid}）。
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                确认「夺取」将清空旧账号的 EMBYID，旧账号会被标记为「待重新绑定 Emby」状态。
              </p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isSubmitting}>
            取消
          </Button>
          {conflict ? (
            <Button
              variant="destructive"
              onClick={() => onSubmit(true)}
              disabled={isSubmitting}
            >
              {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认夺取并绑定
            </Button>
          ) : (
            <Button onClick={() => onSubmit(false)} disabled={isSubmitting}>
              {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              绑定
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 一键踢出 TG 群里未绑账号成员的对话框。
 * 视图状态：
 *   - 加载中：尚无 preview，骨架占位 + 提示
 *   - 已得 preview：展示候选人统计 + 二次确认输入框 + "踢出 N 人"按钮
 *   - 已收到 result：替换为结果摘要，按钮区只剩"关闭"
 * 类型来源于 api 调用结果，保持与后端响应严格同步。
 */
export type KickPreviewData = NonNullable<
  Awaited<ReturnType<typeof api.kickUnboundGroupMembers>>["data"]
>;
export type KickRosterStats = NonNullable<
  Awaited<ReturnType<typeof api.getTelegramRosterStats>>["data"]
>;

export function KickUnboundDialog({
  open,
  onOpenChange,
  loading,
  roster,
  preview,
  result,
  confirmText,
  onConfirmTextChange,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  loading: boolean;
  roster: KickRosterStats | null;
  preview: KickPreviewData | null;
  result: KickPreviewData | null;
  confirmText: string;
  onConfirmTextChange: (next: string) => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!loading) onOpenChange(v);
      }}
    >
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Send className="h-5 w-5" />
            一键踢出未绑 Web 账号的 TG 成员
          </DialogTitle>
          <DialogDescription>
            Bot API 没法主动枚举群成员，本功能依赖 Bot 长期被动累积的「花名册」
            （chat_member 事件 + 群消息观察）。Bot 必须在群里是有「封禁成员」权限的管理员。
            群管理员、Bot、配置中的 ADMIN_ID 与所有持有 Web 账号的人都会被自动排除；
            踢出策略是 ban + 立即 unban（临时移除，仍可重新加入）。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 text-sm">
          {roster && (
            <div className="rounded-md border bg-muted/30 p-3 space-y-1 text-xs">
              <p className="font-medium">花名册概况（chat_id: {roster.chat_id || "—"}）</p>
              <p>活跃: {roster.active ?? 0}　已离群: {roster.inactive ?? 0}　Bot: {roster.bots ?? 0}</p>
              {roster.first_seen_at && (
                <p className="text-muted-foreground">
                  最早观察：{formatDate(roster.first_seen_at)}
                  　最新观察：{formatDate(roster.last_seen_at || roster.first_seen_at)}
                </p>
              )}
            </div>
          )}

          {loading && !preview ? (
            <div className="flex items-center justify-center py-6">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              <span className="ml-2 text-xs text-muted-foreground">正在统计候选人...</span>
            </div>
          ) : preview ? (
            <div className="rounded-md border p-3 space-y-1 text-xs">
              <p>花名册成员: <strong>{preview.roster_size}</strong>（含 bot {preview.bots_in_roster}）</p>
              <p>系统内活跃且已绑 Emby: <strong>{preview.preserved_bound}</strong></p>
              <p>群管理员排除: <strong>{preview.admins_excluded}</strong></p>
              <p>排除总数: <strong>{preview.excluded_total}</strong></p>
              <p className="text-destructive">
                实际待踢: <strong>{preview.targets}</strong>
                <span className="ml-2 text-muted-foreground font-normal">
                  （无账号 {preview.reason_no_account} · 无 Emby {preview.reason_no_emby} · 已禁用 {preview.reason_disabled}）
                </span>
              </p>
              {preview.preview_targets && preview.preview_targets.length > 0 && (
                <div className="pt-1">
                  <p className="text-muted-foreground">前 {preview.preview_targets.length} 个目标：</p>
                  <p className="break-all text-[10px] text-muted-foreground">
                    {preview.preview_targets
                      .map((t) => `${t.tg_id}(${t.reason})`)
                      .join(", ")}
                  </p>
                </div>
              )}
            </div>
          ) : null}

          {result ? (
            <div className="rounded-md border border-emerald-500/40 bg-emerald-500/5 p-3 space-y-1 text-xs">
              <p className="font-medium text-emerald-600 dark:text-emerald-400">执行结果</p>
              <p>已踢出: {result.kicked}　跳过: {result.skipped}</p>
              <p>已不在群: {result.not_in_group}　失败: {result.failed}</p>
            </div>
          ) : (
            <div className="space-y-1.5 rounded-md border border-destructive/40 bg-destructive/5 p-3">
              <Label className="text-xs uppercase tracking-wider text-destructive">二次确认</Label>
              <p className="text-xs text-muted-foreground">
                请在下方输入 <span className="font-mono text-foreground">确认</span> 二字以继续执行：
              </p>
              <Input
                value={confirmText}
                onChange={(e) => onConfirmTextChange(e.target.value)}
                placeholder="确认"
                className="h-9"
                disabled={loading}
              />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
            关闭
          </Button>
          {!result && (
            <Button
              variant="destructive"
              onClick={onConfirm}
              disabled={
                loading ||
                confirmText.trim() !== "确认" ||
                !preview ||
                (preview && preview.targets === 0)
              }
            >
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              确认踢出 {preview ? preview.targets : 0} 人
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 清理无效用户对话框：删除未绑定 Telegram 且未绑定 Emby 的非管理员 / 白名单用户。
 * 父组件需要提供 preview 数据（轻量字段：uid / username / register_time），
 * 由两个回调驱动：onPreview 拉取候选、onConfirm 真正执行清理。
 */
export interface CleanupCandidate {
  uid: number | string;
  username: string;
  register_time?: number;
}

export function CleanupInvalidUsersDialog({
  open,
  onOpenChange,
  minDays,
  onMinDaysChange,
  preview,
  confirmText,
  onConfirmTextChange,
  onPreview,
  onConfirm,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  minDays: string;
  onMinDaysChange: (next: string) => void;
  preview: CleanupCandidate[] | null;
  confirmText: string;
  onConfirmTextChange: (next: string) => void;
  onPreview: () => void;
  onConfirm: () => void;
  isLoading: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>清理无效用户</DialogTitle>
          <DialogDescription>
            删除未绑定 Telegram 且未绑定 Emby 的非管理员 / 白名单用户
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>最少注册天数</Label>
            <Input
              type="number"
              min={1}
              placeholder="注册超过多少天的无效用户"
              value={minDays}
              onChange={(e) => onMinDaysChange(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              仅清理注册超过 {minDays || 7} 天的无效用户
            </p>
          </div>

          {preview !== null && (
            <div className="space-y-2">
              <Label>匹配到 {preview.length} 个无效用户</Label>
              {preview.length > 0 ? (
                <div className="max-h-48 overflow-y-auto rounded-md border">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-3 py-2 text-left">用户名</th>
                        <th className="px-3 py-2 text-left">注册时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {preview.map((u) => (
                        <tr key={u.uid} className="border-b">
                          <td className="px-3 py-1.5">{u.username}</td>
                          <td className="px-3 py-1.5 text-muted-foreground">
                            {u.register_time
                              ? new Date(u.register_time * 1000).toLocaleDateString()
                              : "-"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">没有符合条件的无效用户</p>
              )}
            </div>
          )}

          {preview !== null && preview.length > 0 && (
            <div className="space-y-2">
              <Label>输入「确认」以执行</Label>
              <Input
                value={confirmText}
                onChange={(e) => onConfirmTextChange(e.target.value)}
                placeholder="确认"
              />
              <p className="text-xs text-muted-foreground">
                后端仍会要求确认短语，避免误触或脚本错误直接删除账号。
              </p>
            </div>
          )}
        </div>
        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button variant="outline" onClick={onPreview} disabled={isLoading}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            预览
          </Button>
          <Button
            variant="destructive"
            onClick={onConfirm}
            disabled={isLoading || !preview || preview.length === 0 || confirmText.trim() !== "确认"}
          >
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            确认清理
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 一键踢出未绑 Emby 的系统账号对话框。
 * - 必须先「预览」确认范围，再二次确认输入「确认」才能执行
 * - preview / candidates 类型来自 api.kickNoEmbyUsers，最多展示前 200 行
 */
export type NoEmbyPreviewData = {
  candidate_count: number;
  skipped_admins: number;
  skipped_whitelist: number;
  skipped_unrecognized: number;
  skipped_in_queue: number;
  skipped_pending_register: number;
  skipped_too_recent: number;
  candidates: Array<{
    uid: number | string;
    username: string;
    register_time?: number | null;
    pending_emby?: boolean;
  }>;
};

export function NoEmbyKickDialog({
  open,
  onOpenChange,
  minDays,
  onMinDaysChange,
  minDaysParsed,
  preserveDirect,
  onPreserveDirectChange,
  preview,
  confirmText,
  onConfirmTextChange,
  onPreview,
  onConfirm,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  minDays: string;
  onMinDaysChange: (next: string) => void;
  minDaysParsed: number;
  preserveDirect: boolean;
  onPreserveDirectChange: (next: boolean) => void;
  preview: NoEmbyPreviewData | null;
  confirmText: string;
  onConfirmTextChange: (next: string) => void;
  onPreview: () => void;
  onConfirm: () => void;
  isLoading: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>踢出未绑 Emby 账号</DialogTitle>
          <DialogDescription>
            删除未绑定 Emby 的系统账号。管理员 / 白名单 / 未识别角色 / 当前正在 Emby
            注册队列里的 UID 一律跳过。可选限制最少注册天数。
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 text-sm">
          <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
            <div className="space-y-2">
              <Label>最少注册天数</Label>
              <Input
                type="number"
                min={0}
                value={minDays}
                onChange={(e) => onMinDaysChange(e.target.value)}
                placeholder="0 表示不卡注册时间"
              />
              <p className="text-[11px] text-muted-foreground">
                {minDaysParsed > 0
                  ? `仅清理注册超过 ${minDaysParsed} 天的账号`
                  : "无视注册时间，匹配所有未绑 Emby 的账号"}
              </p>
            </div>
            <label className="flex items-center gap-2 rounded-md border border-border/60 bg-muted/30 px-3 py-2">
              <input
                type="checkbox"
                checked={preserveDirect}
                onChange={(e) => onPreserveDirectChange(e.target.checked)}
                className="h-4 w-4 rounded border-gray-300"
              />
              <span className="text-xs">
                保留可直接补建 Emby 的用户
                <span className="block text-[10px] text-muted-foreground">
                  (已绑 Telegram 的待激活账号)
                </span>
              </span>
            </label>
          </div>

          <p className="rounded-md border border-amber-500/40 bg-amber-500/5 px-3 py-2 text-amber-700 dark:text-amber-300">
            ⚠️ 请先「预览」确认范围后再确认执行。
          </p>

          {preview && (
            <div className="space-y-2">
              <Label>匹配到 {preview.candidate_count} 个待清理账号</Label>
              <div className="flex flex-wrap gap-1.5 text-[10px]">
                <Badge variant="outline">管理员 {preview.skipped_admins}</Badge>
                <Badge variant="outline">白名单 {preview.skipped_whitelist}</Badge>
                <Badge variant="outline">未识别 {preview.skipped_unrecognized}</Badge>
                <Badge variant="outline">注册队列 {preview.skipped_in_queue}</Badge>
                {preview.skipped_pending_register > 0 && (
                  <Badge variant="outline">可补建 Emby {preview.skipped_pending_register}</Badge>
                )}
                {preview.skipped_too_recent > 0 && (
                  <Badge variant="outline">注册时间不够 {preview.skipped_too_recent}</Badge>
                )}
              </div>
              {preview.candidate_count > 0 ? (
                <div className="max-h-56 overflow-y-auto rounded-md border">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-3 py-2 text-left">UID</th>
                        <th className="px-3 py-2 text-left">用户名</th>
                        <th className="px-3 py-2 text-left">注册时间</th>
                        <th className="px-3 py-2 text-left">状态</th>
                      </tr>
                    </thead>
                    <tbody>
                      {preview.candidates.slice(0, 200).map((u) => (
                        <tr key={u.uid} className="border-b">
                          <td className="px-3 py-1.5 font-mono text-xs">{u.uid}</td>
                          <td className="px-3 py-1.5">{u.username}</td>
                          <td className="px-3 py-1.5 text-muted-foreground">
                            {u.register_time
                              ? new Date(u.register_time * 1000).toLocaleDateString()
                              : "-"}
                          </td>
                          <td className="px-3 py-1.5 text-xs">
                            {u.pending_emby ? "待激活" : "未绑定"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {preview.candidates.length > 200 && (
                    <p className="px-3 py-1.5 text-xs text-muted-foreground">
                      仅展示前 200 个，实际仍会按候选总数执行
                    </p>
                  )}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">没有需要清理的账号</p>
              )}
            </div>
          )}

          {preview && preview.candidate_count > 0 && (
            <div className="space-y-2">
              <Label>输入「确认」以执行</Label>
              <Input
                value={confirmText}
                onChange={(e) => onConfirmTextChange(e.target.value)}
                placeholder="确认"
              />
            </div>
          )}
        </div>
        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button variant="outline" onClick={onPreview} disabled={isLoading}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            预览
          </Button>
          <Button
            variant="destructive"
            onClick={onConfirm}
            disabled={
              isLoading ||
              !preview ||
              preview.candidate_count === 0 ||
              confirmText.trim() !== "确认"
            }
          >
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            确认踢出
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 重置用户密码对话框：支持系统 / Emby / 同时三档。
 * - 已成功执行时切换到结果视图（明文密码 + 复制按钮 + 完成）
 * - emby_bound=false 时仅允许 scope=system
 */
export interface ResetPasswordResult {
  scope: "system" | "emby" | "both";
  new_password: string;
  auto_generated: boolean;
}

export function ResetPasswordDialog({
  open,
  onOpenChange,
  user,
  scope,
  onScopeChange,
  auto,
  onAutoChange,
  custom,
  onCustomChange,
  result,
  onSubmit,
  onCopyPassword,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  user: UserInfo | null;
  scope: "system" | "emby" | "both";
  onScopeChange: (next: "system" | "emby" | "both") => void;
  auto: boolean;
  onAutoChange: (next: boolean) => void;
  custom: string;
  onCustomChange: (next: string) => void;
  result: ResetPasswordResult | null;
  onSubmit: () => void;
  onCopyPassword: (password: string) => void;
  isLoading: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>重置密码 · {user?.username}</DialogTitle>
          <DialogDescription>
            可单独重置系统登录密码或 Emby 播放密码；同时支持自定义新密码或随机生成。
          </DialogDescription>
        </DialogHeader>

        {result ? (
          <div className="space-y-3">
            <div className="rounded-lg border bg-muted/30 p-3 space-y-1.5">
              <p className="text-sm">
                <span className="text-muted-foreground">重置范围：</span>
                <span className="font-medium">
                  {result.scope === "system"
                    ? "仅系统密码"
                    : result.scope === "emby"
                      ? "仅 Emby 密码"
                      : "系统 + Emby"}
                </span>
              </p>
              <p className="text-sm text-muted-foreground">
                {result.auto_generated ? "随机生成" : "管理员指定"}的新密码：
              </p>
              <code className="block break-all rounded bg-background px-2 py-1.5 text-base font-mono">
                {result.new_password}
              </code>
              <p className="text-xs text-muted-foreground">
                请尽快告知用户。本对话框关闭后无法再次查看明文。
              </p>
            </div>
            <DialogFooter className="gap-2">
              <Button variant="outline" onClick={() => onCopyPassword(result.new_password)}>
                复制密码
              </Button>
              <Button onClick={() => onOpenChange(false)}>完成</Button>
            </DialogFooter>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>重置范围</Label>
              <Select value={scope} onValueChange={(v) => onScopeChange(v as "system" | "emby" | "both")}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="both" disabled={!user?.emby_bound}>
                    系统 + Emby（同一个密码）
                  </SelectItem>
                  <SelectItem value="system">仅系统登录密码</SelectItem>
                  <SelectItem value="emby" disabled={!user?.emby_bound}>
                    仅 Emby 密码
                  </SelectItem>
                </SelectContent>
              </Select>
              {!user?.emby_bound && (
                <p className="text-xs text-amber-600 dark:text-amber-400">
                  该用户未绑定 Emby，只能重置系统密码。
                </p>
              )}
            </div>

            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={auto}
                onChange={(e) => onAutoChange(e.target.checked)}
                className="h-4 w-4 rounded border-gray-300"
              />
              <span>随机生成 12 位强密码</span>
            </label>

            {!auto && (
              <div className="space-y-2">
                <Label>自定义新密码</Label>
                <Input
                  type="text"
                  value={custom}
                  onChange={(e) => onCustomChange(e.target.value)}
                  placeholder="≥ 8 位，含大小写字母 + 数字"
                  autoComplete="new-password"
                />
                <p className="text-[11px] text-muted-foreground">
                  至少 8 位，且包含大小写字母与数字。
                </p>
              </div>
            )}
          </div>
        )}

        {!result && (
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button onClick={onSubmit} disabled={isLoading}>
              {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              重置密码
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}

/**
 * 强制重置 Emby 密码对话框（不需要本地账号绑定）。
 * 与 ResetPasswordDialog 同样支持随机 / 自定义 + 结果视图复制按钮。
 */
export interface ForceEmbyPasswordResult {
  emby_username: string;
  linked_local_user: boolean;
  new_password: string;
}

export function ForceEmbyPasswordDialog({
  open,
  onOpenChange,
  embyName,
  onEmbyNameChange,
  newPwd,
  onNewPwdChange,
  auto,
  onAutoChange,
  result,
  onSubmit,
  onCopyPassword,
  isLoading,
}: {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  embyName: string;
  onEmbyNameChange: (next: string) => void;
  newPwd: string;
  onNewPwdChange: (next: string) => void;
  auto: boolean;
  onAutoChange: (next: boolean) => void;
  result: ForceEmbyPasswordResult | null;
  onSubmit: () => void;
  onCopyPassword: (password: string) => void;
  isLoading: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>强制重置 Emby 密码</DialogTitle>
          <DialogDescription>
            通过 Emby 用户名直接重置密码。即使该 Emby 账号没有绑定本地系统账号也可执行。
          </DialogDescription>
        </DialogHeader>
        {result ? (
          <div className="space-y-3">
            <div className="rounded-lg border bg-muted/30 p-3 space-y-1.5">
              <p className="text-sm">
                <span className="text-muted-foreground">Emby 用户：</span>
                <span className="font-medium break-all">{result.emby_username}</span>
              </p>
              <p className="text-sm">
                <span className="text-muted-foreground">绑定本地账号：</span>
                <span className="font-medium">{result.linked_local_user ? "是" : "否"}</span>
              </p>
              <p className="text-sm">
                <span className="text-muted-foreground">新密码：</span>
              </p>
              <code className="block break-all rounded bg-background px-2 py-1.5 text-base font-mono">
                {result.new_password}
              </code>
              <p className="text-xs text-muted-foreground">请尽快将新密码告知用户。该密码仅本次显示。</p>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => onCopyPassword(result.new_password)}>
                复制密码
              </Button>
              <Button onClick={() => onOpenChange(false)}>完成</Button>
            </DialogFooter>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Emby 用户名</Label>
              <Input
                placeholder="输入要重置密码的 Emby 用户名"
                value={embyName}
                onChange={(e) => onEmbyNameChange(e.target.value)}
              />
            </div>
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="forcePwdAuto"
                checked={auto}
                onChange={(e) => onAutoChange(e.target.checked)}
                className="h-4 w-4 rounded"
              />
              <Label htmlFor="forcePwdAuto" className="cursor-pointer">
                自动生成 12 位强密码
              </Label>
            </div>
            {!auto && (
              <div className="space-y-2">
                <Label>自定义新密码</Label>
                <Input
                  type="text"
                  value={newPwd}
                  onChange={(e) => onNewPwdChange(e.target.value)}
                  placeholder="≥ 8 位，含大小写字母 + 数字"
                  autoComplete="new-password"
                />
              </div>
            )}
            <DialogFooter className="gap-2">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                取消
              </Button>
              <Button onClick={onSubmit} disabled={isLoading}>
                {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                强制重置
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
