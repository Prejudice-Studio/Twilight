import {
  BarChart3,
  Bell,
  CalendarClock,
  Coins,
  FileText,
  Film,
  GitBranch,
  Home,
  KeyRound,
  Megaphone,
  MessageSquare,
  Search,
  Server,
  Settings,
  Shield,
  TimerReset,
  UserCog,
  Users,
} from "lucide-react";
import type { ComponentType } from "react";

export type DemoRole = "user" | "admin";

export interface DemoNavItem {
  key: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
}

export const userDemoNav: DemoNavItem[] = [
  { key: "dashboard", label: "仪表盘", icon: Home },
  { key: "announcements", label: "公告", icon: Megaphone },
  { key: "media", label: "求片中心", icon: Film },
  { key: "score", label: "签到积分", icon: Coins },
  { key: "invite", label: "邀请中心", icon: GitBranch },
  { key: "settings", label: "个人设置", icon: Settings },
  { key: "apikey", label: "API Key", icon: KeyRound },
];

export const adminDemoNav: DemoNavItem[] = [
  { key: "overview", label: "总览", icon: BarChart3 },
  { key: "users", label: "用户管理", icon: Users },
  { key: "announcements", label: "公告管理", icon: Megaphone },
  { key: "regcodes", label: "注册码", icon: FileText },
  { key: "requests", label: "求片审核", icon: Film },
  { key: "telegram", label: "Telegram 换绑", icon: MessageSquare },
  { key: "emby", label: "Emby 管理", icon: Server },
  { key: "scheduler", label: "定时任务", icon: TimerReset },
  { key: "config", label: "配置管理", icon: UserCog },
  { key: "security", label: "安全审计", icon: Shield },
];

export const demoAnnouncements = [
  { title: "维护窗口", tag: "置顶", text: "周六 02:00-03:00 进行线路维护，期间播放可能短暂中断。" },
  { title: "新片补全", tag: "更新", text: "本周新增 42 部电影与 13 部剧集，求片队列处理完成率 93%。" },
  { title: "安全提醒", tag: "提醒", text: "请勿复用弱密码，演示站不会保存或提交任何输入内容。" },
];

export const demoRequests = [
  { title: "The Bear S03", user: "mika", status: "pending", source: "TMDB", note: "等待下载" },
  { title: "葬送的芙莉莲", user: "akira", status: "downloading", source: "BGM", note: "已加入队列" },
  { title: "Dune: Part Two", user: "nova", status: "completed", source: "TMDB", note: "已入库" },
];

export const demoSchedulerRuns = [
  { name: "过期用户检查", status: "success", time: "03:00", logs: ["扫描用户 186", "禁用过期账号 2", "同步 Emby 完成"] },
  { name: "系统自动更新", status: "success", time: "04:00", logs: ["git fetch origin main", "git pull --ff-only", "未发现旧后端依赖"] },
  { name: "清理未绑定 Emby", status: "failed", time: "手动", logs: ["dry_run=false", "跳过注册队列 UID 12", "失败：示例错误"] },
];

export const demoUsers = [
  { uid: 1, username: "admin", role: "管理员", active: true, emby: "已绑定", expire: "永久" },
  { uid: 28, username: "mika", role: "普通用户", active: true, emby: "待补建", expire: "未绑定" },
  { uid: 42, username: "nova", role: "白名单", active: true, emby: "已绑定", expire: "永久" },
  { uid: 77, username: "guest", role: "普通用户", active: false, emby: "未绑定", expire: "未绑定" },
];

export const demoMedia = [
  { title: "The Bear", type: "剧集", year: "2022", status: "可求片", rating: "8.6" },
  { title: "Dune: Part Two", type: "电影", year: "2024", status: "已入库", rating: "8.4" },
  { title: "Frieren", type: "动画", year: "2023", status: "处理中", rating: "9.1" },
];

export const demoAuditEvents = [
  { actor: "admin", action: "授予注册队列用户资格", target: "mika", level: "warning" },
  { actor: "system", action: "自动更新完成", target: "main", level: "success" },
  { actor: "bot", action: "Telegram 换绑审核", target: "nova", level: "info" },
];

export const demoNotifications = [
  { icon: Bell, text: "你有 1 个求片已完成" },
  { icon: CalendarClock, text: "账号将在 12 天后到期" },
  { icon: Search, text: "模拟搜索不会访问后端" },
];
