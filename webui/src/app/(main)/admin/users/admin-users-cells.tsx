// 用户列表中纯展示型的 cell 渲染：角色徽章 / 到期时间 / 单行操作菜单。
// 拆出来的目的：把没有状态依赖的展示逻辑从 page.tsx 主组件中剥离，主组件
// 仅保留交互编排；新人接手时可以单独阅读 cell 渲染规则、单独写单测，不必
// 啃完 3500+ 行的 page.tsx。
import {
  Ban,
  CalendarClock,
  Edit,
  Key,
  Link2,
  Mail,
  MonitorCheck,
  MonitorX,
  MoreHorizontal,
  RefreshCcw,
  RefreshCw,
  Trash2,
  Unlink,
  UserCheck,
  UserPlus,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { UserInfo } from "@/lib/api";
import { formatDate, isPermanentDateValue } from "@/lib/utils";
import type { MessageKey, MessageParams } from "@/lib/i18n";

// 翻译函数类型：与 LocaleContextValue.t 同构。cells / helpers 是无状态渲染，
// 不能直接用 useI18n（会破坏 renderXxx 的纯函数契约），由 page.tsx 注入 t。
type TFunc = (key: MessageKey, params?: MessageParams) => string;

/**
 * 角色徽章。
 * - 0 管理员 → 渐变高亮
 * - 2 白名单 → 成功色
 * - 其余（含 -1 未识别 / 1 普通）→ 次级标签
 */
export function renderRoleBadge(role: number, t: TFunc) {
  switch (role) {
    case 0:
      return <Badge variant="gradient">{t("adminUsers.roleAdmin")}</Badge>;
    case 2:
      return <Badge variant="success">{t("adminUsers.roleWhitelist")}</Badge>;
    default:
      return <Badge variant="secondary">{t("adminUsers.roleUser")}</Badge>;
  }
}

/**
 * 根据 emby_bound / expired_at / pending_emby 渲染到期时间单元格。
 * - 未绑定 Emby（emby_bound===false / pending_emby / expired_at===0）→"未绑定"
 * - -1 / "-1" → "永久"
 * - 真实时间戳 → 用 formatDate；已过期红字
 */
export function renderExpireCell(user: UserInfo, t: TFunc) {
  const exp = user.expired_at;
  const isUnbound =
    user.emby_bound === false ||
    Boolean(user.pending_emby) ||
    exp === 0 ||
    exp === "0";
  if (isUnbound) {
    return <span className="text-muted-foreground italic">{t("adminUsers.cellUnbound")}</span>;
  }
  if (isPermanentDateValue(exp)) {
    return <span className="text-emerald-500">{t("adminUsers.cellPermanent")}</span>;
  }
  const expMs = typeof exp === "number" && exp < 10000000000 ? exp * 1000 : Number(exp);
  const expired = !Number.isNaN(expMs) && expMs < Date.now();
  return (
    <span className={expired ? "text-destructive" : undefined}>
      {formatDate(exp)}
    </span>
  );
}

/**
 * Web 账号状态徽章：系统账号本身能否登录，仅取决于 active。
 * 与 Emby 账号状态分开展示——一个用户的网页账号正常、Emby 账号却可能因到期被禁用。
 */
export function renderWebStatusBadge(user: UserInfo) {
  return (
    <Badge variant={user.active ? "success" : "destructive"}>
      {user.active ? "正常" : "禁用"}
    </Badge>
  );
}

/**
 * Emby 账号状态单元格：独立于 Web 账号状态，按绑定 / 待开通 / 启用 / 禁用区分。
 * - pending_emby → 待开通（系统账号已建，等首次登录补建 Emby）
 * - 无 emby_id → 未绑定
 * - emby_disabled_by_expiry → 已禁用（到期）
 * - emby_disabled → 已禁用（管理员单独禁用，Web 仍正常）
 * - 其余已绑定 → 已启用，并展示绑定的 Emby 用户名
 */
export function renderEmbyStatusCell(user: UserInfo) {
  if (user.pending_emby) {
    return (
      <Badge variant="outline" className="w-fit border-amber-500/40 text-[10px] text-amber-600">
        待开通
      </Badge>
    );
  }
  if (!user.emby_id) {
    return (
      <Badge variant="outline" className="text-[10px] text-muted-foreground">
        未绑定
      </Badge>
    );
  }
  const disabledByExpiry = Boolean(user.emby_disabled_by_expiry);
  const manuallyDisabled = Boolean(user.emby_disabled);
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      {disabledByExpiry ? (
        <Badge variant="destructive" className="w-fit text-[10px]">
          已禁用（到期）
        </Badge>
      ) : manuallyDisabled ? (
        <Badge variant="destructive" className="w-fit text-[10px]">
          已禁用
        </Badge>
      ) : (
        <Badge variant="success" className="w-fit text-[10px]">
          已启用
        </Badge>
      )}
      <span
        className="max-w-[160px] truncate text-xs text-muted-foreground"
        title={user.emby_username || user.username}
      >
        {user.emby_username || user.username}
      </span>
    </div>
  );
}

/**
 * 单行操作下拉菜单。所有交互通过 handlers 注入，组件本身无状态，便于
 * page.tsx 主组件甩开 90+ 行 JSX。子项的可见性 / 禁用规则与原行为一致：
 *   - "取消永久到期" 仅在永久到期 + 非管理员 + 已绑定 Emby 时出现
 *   - "授权 / 授权并移出队列" 在 emby_id 已存在或账号被禁用时禁用
 */
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

export function UserActionsMenu({
  user,
  handlers,
  t,
}: {
  user: UserInfo;
  handlers: UserActionsMenuHandlers;
  t: TFunc;
}) {
  const showCancelPermanent =
    isPermanentDateValue(user.expired_at) &&
    user.role !== 0 &&
    Boolean(user.emby_id);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon">
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={() => handlers.onEdit(user)}>
          <Edit className="mr-2 h-4 w-4" />
          {t("adminUsers.menuEdit")}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onRenew(user)}>
          <RefreshCw className="mr-2 h-4 w-4" />
          {t("adminUsers.menuRenew")}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onSetExpiry(user)}>
          <CalendarClock className="mr-2 h-4 w-4" />
          {t("adminUsers.menuSetExpiry")}
        </DropdownMenuItem>
        {showCancelPermanent && (
          <DropdownMenuItem onClick={() => handlers.onCancelPermanent(user)}>
            <CalendarClock className="mr-2 h-4 w-4" />
            {t("adminUsers.menuCancelPermanent")}
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={() => handlers.onResetPassword(user)}>
          <Key className="mr-2 h-4 w-4" />
          {t("adminUsers.menuResetPassword")}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onBindEmby(user)}>
          <Link2 className="mr-2 h-4 w-4" />
          {t("adminUsers.menuBindEmby")}
        </DropdownMenuItem>
        {user.emby_id && (
          <>
            <DropdownMenuItem onClick={() => handlers.onEmbyDisable(user)}>
              <MonitorX className="mr-2 h-4 w-4" />
              {t("adminUsers.menuEmbyDisable")}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => handlers.onEmbyEnable(user)} disabled={!user.active}>
              <MonitorCheck className="mr-2 h-4 w-4" />
              {t("adminUsers.menuEmbyEnable")}
            </DropdownMenuItem>
          </>
        )}
        <DropdownMenuItem onClick={() => handlers.onBindEmail(user)}>
          <Mail className="mr-2 h-4 w-4" />
          {t("email.admin.bindTitle")}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onBindTelegram(user)}>
          <Link2 className="mr-2 h-4 w-4" />
          {t("adminUsers.menuBindTelegram")}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onSyncBindings(user)}>
          <RefreshCw className="mr-2 h-4 w-4" />
          {t("adminUsers.menuSyncBindings")}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onRefreshStatus(user, "telegram")}>
          <RefreshCcw className="mr-2 h-4 w-4" />
          {t("adminUsers.menuRefreshTelegram")}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onRefreshStatus(user, "emby")}>
          <RefreshCcw className="mr-2 h-4 w-4" />
          {t("adminUsers.menuRefreshEmby")}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handlers.onForceUnbind(user)}>
          <Unlink className="mr-2 h-4 w-4" />
          {t("adminUsers.menuForceUnbind")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => handlers.onClearRegistrationQueue(user)}>
          <CalendarClock className="mr-2 h-4 w-4" />
          {t("adminUsers.menuClearQueue")}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => handlers.onGrantRegistrationEntitlement(user)}
          disabled={Boolean(user.emby_id) || !user.active}
        >
          <UserPlus className="mr-2 h-4 w-4" />
          {t("adminUsers.menuGrantEntitlement")}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => handlers.onGrantRegistrationEntitlementAndDequeue(user)}
          disabled={Boolean(user.emby_id) || !user.active}
        >
          <UserCheck className="mr-2 h-4 w-4" />
          {t("adminUsers.menuGrantDequeue")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => handlers.onToggleActive(user)}>
          <Ban className="mr-2 h-4 w-4" />
          {user.active ? t("adminUsers.menuDisable") : t("adminUsers.menuEnable")}
        </DropdownMenuItem>
        <DropdownMenuItem className="text-destructive" onClick={() => handlers.onDelete(user)}>
          <Trash2 className="mr-2 h-4 w-4" />
          {t("adminUsers.menuDelete")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
