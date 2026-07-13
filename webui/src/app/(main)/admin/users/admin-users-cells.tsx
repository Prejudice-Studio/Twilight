"use client";

import { Fragment } from "react";
import {
  Ban,
  CalendarClock,
  Edit,
  ExternalLink,
  Key,
  Mail,
  MonitorCheck,
  MonitorX,
  MoreHorizontal,
  RefreshCcw,
  RefreshCw,
  Trash2,
  UserCheck,
  UserPlus,
  UserX,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import type { UserInfo } from "@/lib/api";
import type { MessageKey, MessageParams } from "@/lib/i18n";
import { formatDate, isPermanentDateValue } from "@/lib/utils";
import { sanitizeImageUrl } from "@/lib/safe-url";
import { API_BASE } from "@/lib/api-request";

type TFunc = (key: MessageKey, params?: MessageParams) => string;

export function renderRoleBadge(role: number, t: TFunc) {
  if (role === 0) return <Badge variant="destructive">管理员</Badge>;
  if (role === 2) return <Badge variant="secondary">白名单</Badge>;
  return <Badge variant="outline">普通用户</Badge>;
}

export function renderWebStatusBadge(user: UserInfo) {
  return user.active
    ? <Badge variant="default">已启用</Badge>
    : <Badge variant="destructive">已禁用</Badge>;
}

export function renderEmbyStatusCell(user: UserInfo) {
  if (!user.emby_id) return <span className="text-xs text-muted-foreground">未绑定</span>;
  const disabled = user.emby_disabled;
  return (
    <div className="space-y-1">
      <span className="text-xs block truncate max-w-[200px]" title={user.emby_username || user.emby_id}>
        {user.emby_username || user.emby_id}
      </span>
      {disabled
        ? <Badge variant="destructive" className="text-[10px]">Emby 已禁用</Badge>
        : <Badge variant="outline" className="text-[10px] border-emerald-500/40 text-emerald-600">Emby 正常</Badge>}
    </div>
  );
}

export function renderExpireCell(user: UserInfo, t: TFunc) {
  const raw = user.expired_at;
  if (raw === null || raw === undefined) return <span className="text-xs text-muted-foreground">未设置</span>;
  if (raw === 0) return <span className="text-xs text-muted-foreground">未设置</span>;
  if (isPermanentDateValue(raw)) return <Badge variant="default">永久</Badge>;
  const date = formatDate(raw);
  const now = Date.now();
  const expired = typeof raw === "string" ? new Date(raw).getTime() < now : raw * 1000 < now;
  return <span className={`text-xs ${expired ? "text-destructive font-medium" : "text-muted-foreground"}`}>{date}</span>;
}

function normalizeAssetUrl(url?: string): string | undefined {
  if (!url) return undefined;
  const trimmed = url.trim();
  if (!trimmed) return undefined;
  if (trimmed.startsWith("http")) return sanitizeImageUrl(trimmed);
  if (trimmed.startsWith("/")) return sanitizeImageUrl(`${API_BASE}${trimmed}`);
  return sanitizeImageUrl(trimmed);
}

export function UserAvatar({ user }: { user: UserInfo }) {
  const src = normalizeAssetUrl(user.avatar);
  return (
    <Avatar className="h-8 w-8 shrink-0 ring-1 ring-border/50">
      <AvatarImage src={src} />
      <AvatarFallback className="bg-primary/10 text-primary text-xs font-bold">
        {user.username.charAt(0).toUpperCase()}
      </AvatarFallback>
    </Avatar>
  );
}

export interface UserActionsMenuHandlers {
  onEdit: (user: UserInfo) => void;
  onRenew: (user: UserInfo) => void;
  onCancelPermanent: (user: UserInfo) => void;
  onSetExpiry: (user: UserInfo) => void;
  onResetPassword: (user: UserInfo) => void;
  onBindEmby: (user: UserInfo) => void;
  onEmbyDisable: (user: UserInfo) => void;
  onEmbyEnable: (user: UserInfo) => void;
  onBindEmail: (user: UserInfo) => void;
  onBindTelegram: (user: UserInfo) => void;
  onSyncBindings: (user: UserInfo) => void;
  onRefreshStatus: (user: UserInfo, scope: "telegram" | "emby") => void;
  onForceUnbind: (user: UserInfo) => void;
  onClearRegistrationQueue: (user: UserInfo) => void;
  onGrantRegistrationEntitlement: (user: UserInfo) => void;
  onGrantRegistrationEntitlementAndDequeue: (user: UserInfo) => void;
  onToggleActive: (user: UserInfo) => void;
  onDelete: (user: UserInfo) => void;
}

function MenuTitle({ title, desc }: { title: string; desc: string }) {
  return (
    <span className="flex min-w-0 flex-col">
      <span className="leading-4 text-sm">{title}</span>
      <span className="max-w-[210px] truncate text-[11px] leading-4 text-muted-foreground">{desc}</span>
    </span>
  );
}

function MenuText({ title, desc }: { title: string; desc?: string }) {
  return (
    <span className="flex min-w-0 flex-col">
      <span className="leading-4 text-sm">{title}</span>
      {desc && <span className="max-w-[190px] truncate text-[11px] leading-4 text-muted-foreground">{desc}</span>}
    </span>
  );
}

export function UserActionsMenu({
  user,
  handlers,
  t,
}: {
  user: UserInfo;
  handlers: UserActionsMenuHandlers;
  t: TFunc;
}) {
  const actionState = user.admin_action_state;
  const showCancelPermanent = isPermanentDateValue(user.expired_at) && user.role !== 0 && Boolean(user.emby_id);
  const entitlementDisabled = actionState ? !actionState.can_grant_registration_entitlement : Boolean(user.emby_id) || !user.active;
  const entitlementReason = actionState?.reasons?.grant_registration_entitlement || (
    entitlementDisabled
      ? Boolean(user.emby_id) ? "已绑定 Emby，无需授予资格" : "账号已禁用，先启用"
      : ""
  );
  const canClearRegistrationQueue = actionState?.can_clear_registration_queue ?? (!user.emby_id && user.active);
  const canEnableEmby = actionState?.can_enable_emby ?? (Boolean(user.emby_id) && user.active);
  const enableEmbyReason = actionState?.reasons?.enable_emby || (user.active ? "恢复 Emby 访问" : "Web 已禁用，不能启用 Emby");
  const canDelete = actionState?.can_delete ?? user.role !== 0;
  const deleteReason = actionState?.reasons?.delete || "";
  const showRegistrationActions = canClearRegistrationQueue || !user.emby_id || user.pending_emby;
  const isAdmin = user.role === 0;
  const toggleTitle = user.active ? "禁用此账号" : "启用此账号";
  const toggleDesc = user.active ? "可选择级联邀请树" : "恢复 Web 登录";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={`操作 ${user.username}`}>
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-[min(82vh,640px)] w-[min(20rem,calc(100vw-1.5rem))] overflow-y-auto">
        <DropdownMenuItem onClick={() => handlers.onEdit(user)}>
          <Edit className="mr-2 h-4 w-4" />
          <MenuTitle title="编辑信息" desc="角色、Emby ID 与状态" />
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onRenew(user)}>
          <CalendarClock className="mr-2 h-4 w-4" />
          <MenuTitle title="续期" desc="在现有到期时间上追加天数" />
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onResetPassword(user)}>
          <Key className="mr-2 h-4 w-4" />
          <MenuTitle title="重置密码" desc="系统密码、Emby 密码或同时重置" />
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <Ban className="mr-2 h-4 w-4" />
            <MenuText title="账号状态" desc="Web 登录与到期" />
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="w-[min(18rem,calc(100vw-1.5rem))]">
            <DropdownMenuItem onClick={() => handlers.onToggleActive(user)} className={user.active ? "text-amber-600 focus:text-amber-600" : ""}>
              <Ban className="mr-2 h-4 w-4" />
              <MenuTitle title={toggleTitle} desc={toggleDesc} />
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => handlers.onSetExpiry(user)}>
              <CalendarClock className="mr-2 h-4 w-4" />
              <MenuTitle title="设定到期时间" desc="直接覆盖为指定的到期日期" />
            </DropdownMenuItem>
            {showCancelPermanent && (
              <DropdownMenuItem onClick={() => handlers.onCancelPermanent(user)}>
                <CalendarClock className="mr-2 h-4 w-4" />
                <MenuTitle title="取消永久" desc="将永久账号改回固定到期日" />
              </DropdownMenuItem>
            )}
          </DropdownMenuSubContent>
        </DropdownMenuSub>

        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <MonitorCheck className="mr-2 h-4 w-4" />
            <MenuText title="Emby 账号" desc="绑定、启停、解绑" />
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="w-[min(18rem,calc(100vw-1.5rem))]">
            <DropdownMenuItem onClick={() => handlers.onBindEmby(user)}>
              <ExternalLink className="mr-2 h-4 w-4" />
              <MenuTitle title="Emby 绑定" desc="绑定或强绑远端 Emby 用户" />
            </DropdownMenuItem>
            {user.emby_id && (
              <>
                <DropdownMenuItem onClick={() => handlers.onEmbyDisable(user)}>
                  <MonitorX className="mr-2 h-4 w-4" />
                  <MenuTitle title="禁用 Emby" desc="仅禁用 Emby，不影响 Web 登录" />
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => handlers.onEmbyEnable(user)} disabled={!canEnableEmby} title={enableEmbyReason}>
                  <MonitorCheck className="mr-2 h-4 w-4" />
                  <MenuTitle title="启用 Emby" desc={enableEmbyReason} />
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => handlers.onForceUnbind(user)}>
                  <UserX className="mr-2 h-4 w-4" />
                  <MenuTitle title="解绑 Emby" desc="解除本地绑定，不删远端" />
                </DropdownMenuItem>
              </>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => handlers.onRefreshStatus(user, "emby")}>
              <RefreshCcw className="mr-2 h-4 w-4" />
              <MenuTitle title="刷新 Emby 状态" desc="重新核对远端启停" />
            </DropdownMenuItem>
          </DropdownMenuSubContent>
        </DropdownMenuSub>

        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <Mail className="mr-2 h-4 w-4" />
            <MenuText title="身份绑定" desc="邮箱、Telegram、同步" />
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="w-[min(18rem,calc(100vw-1.5rem))]">
            <DropdownMenuItem onClick={() => handlers.onBindEmail(user)}>
              <Mail className="mr-2 h-4 w-4" />
              <MenuTitle title="邮箱管理" desc="强制绑定邮箱或调整验证状态" />
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => handlers.onBindTelegram(user)}>
              <ExternalLink className="mr-2 h-4 w-4" />
              <MenuTitle title="TG 绑定" desc="指定 Telegram ID 绑定" />
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => handlers.onSyncBindings(user)}>
              <RefreshCw className="mr-2 h-4 w-4" />
              <MenuTitle title="同步绑定" desc="同步该用户远端绑定" />
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => handlers.onRefreshStatus(user, "telegram")}>
              <RefreshCcw className="mr-2 h-4 w-4" />
              <MenuTitle title="刷新 TG 状态" desc="重新读取 Telegram 用户名" />
            </DropdownMenuItem>
          </DropdownMenuSubContent>
        </DropdownMenuSub>

        {showRegistrationActions && (
          <DropdownMenuSub>
            <DropdownMenuSubTrigger>
              <UserPlus className="mr-2 h-4 w-4" />
              <MenuText title="注册资格" desc="待开通与队列" />
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent className="w-[min(18rem,calc(100vw-1.5rem))]">
              <DropdownMenuItem onClick={() => handlers.onClearRegistrationQueue(user)} disabled={!canClearRegistrationQueue}>
                <CalendarClock className="mr-2 h-4 w-4" />
                <MenuTitle title="清理注册队列" desc={canClearRegistrationQueue ? "移除待补建 Emby 状态" : "当前用户无需清理"} />
              </DropdownMenuItem>
            <DropdownMenuItem onClick={() => handlers.onGrantRegistrationEntitlement(user)} disabled={entitlementDisabled} title={entitlementReason}>
              <UserPlus className="mr-2 h-4 w-4" />
              <MenuTitle title="授予注册资格" desc={entitlementReason || "允许后续自助创建 Emby"} />
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => handlers.onGrantRegistrationEntitlementAndDequeue(user)} disabled={entitlementDisabled} title={entitlementReason}>
              <UserCheck className="mr-2 h-4 w-4" />
              <MenuTitle title="授予资格并出列" desc={entitlementReason || "授予并从等待队列移除"} />
            </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        )}

        <DropdownMenuSeparator />

        {!isAdmin && (
          <DropdownMenuItem className="text-destructive focus:text-destructive" onClick={() => handlers.onDelete(user)} disabled={!canDelete} title={deleteReason}>
            <Trash2 className="mr-2 h-4 w-4" />
            <MenuTitle title="删除用户" desc={deleteReason || "删除本地账号"} />
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
