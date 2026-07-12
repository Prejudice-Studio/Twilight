"use client";

import Link from "next/link";
import { motion } from "framer-motion";
import {
  Bell,
  BookOpen,
  Calendar,
  HelpCircle,
  KeyRound,
  Mail,
  MessageCircle,
  Monitor,
  RefreshCw,
  Shield,
  Ticket,
  Tv,
  UserPlus,
  Users,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

const quickLinks = [
  { href: "/dashboard", label: "仪表盘", icon: Monitor },
  { href: "/settings", label: "账号设置", icon: Shield },
  { href: "/invite", label: "邀请", icon: UserPlus },
  { href: "/tickets", label: "工单", icon: Ticket },
  { href: "/announcements", label: "公告", icon: Bell },
];

const userFlows = [
  {
    icon: Users,
    title: "注册与登录",
    items: ["使用注册码、邀请码或白名单码注册", "支持用户名/邮箱登录", "登录后会记录设备与 IP，便于安全审查"],
  },
  {
    icon: Tv,
    title: "Emby 绑定",
    items: ["绑定已有 Emby 账号或按配置自助注册", "Web 账号与 Emby 账号启停状态相互独立", "管理员可在后台进行强制绑定、解绑和设备审查"],
  },
  {
    icon: Calendar,
    title: "到期与续期",
    items: ["账号可设置到期时间或永久有效", "续期码、邀请续期码和积分续期都可延长账号", "管理员可按筛选条件批量调整到期时间"],
  },
  {
    icon: Mail,
    title: "邮箱与通知",
    items: ["邮箱绑定需要验证码", "可通过邮箱找回密码", "开启后可接收登录通知和工单提醒"],
  },
];

const adminFeatures = [
  ["用户管理", "筛选、编辑、续期、禁用、删除、重置密码、角色调整和批量操作。"],
  ["卡码管理", "批量生成注册码、续期码、白名单码，查看使用记录并管理有效期。"],
  ["Emby 管理", "同步用户、查看活动日志、踢出在线会话、审查设备与 IP。"],
  ["工单与公告", "处理用户工单、管理公告内容、控制置顶和可见性。"],
  ["系统运维", "配置管理、数据库备份、运行日志、调度任务和系统更新。"],
  ["安全审计", "查看操作审计、违规日志、登录历史、IP 黑名单和设备风险。"],
];

const concepts = [
  {
    icon: KeyRound,
    title: "清理授权记录",
    text: "用于释放“消耗了注册码但没有完成 Emby 注册”的占用记录。已正常注册 Emby 的用户不受影响。",
  },
  {
    icon: RefreshCw,
    title: "强制解绑",
    text: "只解除 Twilight 本地绑定关系，不删除远端 Emby 或 Telegram 账号。用户可在允许时重新绑定。",
  },
  {
    icon: Shield,
    title: "禁用 Web 与禁用 Emby",
    text: "禁用 Web 会阻止登录面板；禁用 Emby 只影响媒体访问。两者可独立控制。",
  },
];

const faqs = [
  ["无法登录怎么办？", "可能原因包括账号被禁用、已到期或密码错误。可尝试找回密码，或联系管理员查看登录历史。"],
  ["Emby 密码错误怎么办？", "可在设置页修改 Emby 密码；如果账号由管理员统一管理，请提交工单。"],
  ["如何获取注册码？", "注册码由管理员发放。已有用户也可能通过邀请页生成邀请码给新用户。"],
  ["Telegram 绑定失败怎么办？", "确认 Bot 已启动、自己已加入要求的群组，并且 Telegram ID 没有被其他账号绑定。"],
];

function Section({ title, icon: Icon, children }: { title: string; icon: typeof BookOpen; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <div className="flex items-center gap-2">
        <Icon className="h-5 w-5 text-primary" />
        <h2 className="text-xl font-semibold">{title}</h2>
      </div>
      {children}
    </section>
  );
}

export default function WikiPage() {
  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="mx-auto max-w-6xl space-y-8 px-4 py-8">
      <header className="space-y-3">
        <Badge variant="outline" className="w-fit">Twilight Wiki</Badge>
        <div className="space-y-2">
          <h1 className="flex items-center gap-3 text-3xl font-bold">
            <BookOpen className="h-8 w-8 text-primary" />
            Twilight 使用指南
          </h1>
          <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
            这里汇总普通用户和管理员最常用的操作说明。页面不展示任何后端密钥、Token、数据库连接或其他敏感配置。
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {quickLinks.map(({ href, label, icon: Icon }) => (
            <Button key={href} asChild variant="outline" size="sm">
              <Link href={href}>
                <Icon className="mr-2 h-4 w-4" />
                {label}
              </Link>
            </Button>
          ))}
        </div>
      </header>

      <Section title="普通用户流程" icon={Users}>
        <div className="grid gap-3 md:grid-cols-2">
          {userFlows.map(({ icon: Icon, title, items }) => (
            <Card key={title}>
              <CardHeader className="pb-2">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Icon className="h-4 w-4 text-primary" />
                  {title}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <ul className="space-y-2 text-sm text-muted-foreground">
                  {items.map((item) => (
                    <li key={item} className="leading-6">{item}</li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          ))}
        </div>
      </Section>

      <Section title="管理员功能速查" icon={Shield}>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {adminFeatures.map(([title, text]) => (
            <Card key={title}>
              <CardContent className="space-y-2 p-4">
                <h3 className="font-semibold">{title}</h3>
                <p className="text-sm leading-6 text-muted-foreground">{text}</p>
              </CardContent>
            </Card>
          ))}
        </div>
      </Section>

      <Section title="关键概念" icon={HelpCircle}>
        <div className="grid gap-3 md:grid-cols-2">
          {concepts.map(({ icon: Icon, title, text }) => (
            <Card key={title}>
              <CardContent className="flex gap-3 p-4">
                <Icon className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
                <div className="space-y-1">
                  <h3 className="font-semibold">{title}</h3>
                  <p className="text-sm leading-6 text-muted-foreground">{text}</p>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      </Section>

      <Section title="扩展功能" icon={MessageCircle}>
        <div className="grid gap-3 md:grid-cols-3">
          <Card>
            <CardContent className="space-y-2 p-4">
              <MessageCircle className="h-5 w-5 text-primary" />
              <h3 className="font-semibold">Telegram Bot</h3>
              <p className="text-sm leading-6 text-muted-foreground">支持绑定、登录通知、工单提醒、管理员命令和受控 JS 扩展。</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="space-y-2 p-4">
              <BookOpen className="h-5 w-5 text-primary" />
              <h3 className="font-semibold">Bangumi</h3>
              <p className="text-sm leading-6 text-muted-foreground">支持播放同步、收藏管理、本地封面缓存和最近动态。</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="space-y-2 p-4">
              <Ticket className="h-5 w-5 text-primary" />
              <h3 className="font-semibold">工单系统</h3>
              <p className="text-sm leading-6 text-muted-foreground">用户可提交问题、上传图片、接收状态通知；管理员可分类处理。</p>
            </CardContent>
          </Card>
        </div>
      </Section>

      <Section title="常见问题" icon={HelpCircle}>
        <div className="space-y-2">
          {faqs.map(([q, a]) => (
            <Card key={q}>
              <CardContent className="space-y-1 p-4">
                <h3 className="font-semibold">{q}</h3>
                <p className="text-sm leading-6 text-muted-foreground">{a}</p>
              </CardContent>
            </Card>
          ))}
        </div>
      </Section>
    </motion.div>
  );
}
