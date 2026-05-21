"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { motion } from "framer-motion";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";
import {
  adminDemoNav,
  demoAnnouncements,
  demoAuditEvents,
  demoMedia,
  demoNotifications,
  demoRequests,
  demoSchedulerRuns,
  demoUsers,
  type DemoNavItem,
  type DemoRole,
  userDemoNav,
} from "./mock-data";
import { CheckCircle2, DatabaseZap, Eye, Lock, PlayCircle, RefreshCw, ShieldCheck, Sparkles } from "lucide-react";

function statusVariant(status: string): "default" | "secondary" | "outline" | "destructive" | "success" {
  if (["success", "completed", "已绑定", "已入库"].includes(status)) return "success";
  if (["failed", "rejected"].includes(status)) return "destructive";
  if (["pending", "downloading", "待补建", "处理中"].includes(status)) return "secondary";
  return "outline";
}

function DemoSidebar({ role, nav, active, setActive }: { role: DemoRole; nav: DemoNavItem[]; active: string; setActive: (key: string) => void }) {
  return (
    <aside className="hidden w-72 shrink-0 p-4 lg:block">
      <div className="sticky top-4 flex h-[calc(100vh-2rem)] flex-col rounded-3xl border bg-card/80 p-4 shadow-xl backdrop-blur">
        <div className="mb-6 flex items-center gap-3 rounded-2xl bg-primary/10 p-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
            <Sparkles className="h-5 w-5" />
          </div>
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">TestWeb</p>
            <h2 className="font-semibold">{role === "admin" ? "管理端演示" : "用户端演示"}</h2>
          </div>
        </div>
        <nav className="space-y-1">
          {nav.map((item) => {
            const selected = active === item.key;
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => setActive(item.key)}
                className={cn(
                  "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition",
                  selected ? "bg-primary text-primary-foreground shadow" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </button>
            );
          })}
        </nav>
        <div className="mt-auto rounded-2xl border bg-background/70 p-3 text-xs text-muted-foreground">
          <div className="mb-2 flex items-center gap-2 font-medium text-foreground">
            <Lock className="h-4 w-4 text-emerald-500" />
            安全演示模式
          </div>
          本页面不读取 token、不调用 API、不连接真实后端数据。
        </div>
      </div>
    </aside>
  );
}

function DemoHeader({ role, activeLabel }: { role: DemoRole; activeLabel: string }) {
  return (
    <header className="sticky top-0 z-20 border-b bg-background/85 px-4 py-3 backdrop-blur lg:rounded-b-3xl lg:border lg:shadow-sm">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.16em] text-muted-foreground">{role === "admin" ? "Admin Mock Console" : "User Mock Portal"}</p>
          <h1 className="text-xl font-bold">{activeLabel}</h1>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline" className="gap-1.5"><ShieldCheck className="h-3.5 w-3.5" />无鉴权演示</Badge>
          <Badge variant="secondary" className="gap-1.5"><DatabaseZap className="h-3.5 w-3.5" />Mock Data Only</Badge>
          <Button asChild variant="outline" size="sm"><Link href="/testweb">返回入口</Link></Button>
        </div>
      </div>
    </header>
  );
}

function MetricGrid({ role }: { role: DemoRole }) {
  const metrics = role === "admin"
    ? [
        ["总用户", "186", "+12 本月"],
        ["Emby 绑定", "143", "77%"],
        ["待处理求片", "8", "3 个下载中"],
        ["定时任务", "11", "9 个启用"],
      ]
    : [
        ["账号状态", "正常", "Emby 已绑定"],
        ["剩余天数", "42", "到期提醒开启"],
        ["积分", "1,280", "今日已签到"],
        ["求片", "3", "1 个已完成"],
      ];
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {metrics.map(([label, value, desc]) => (
        <Card key={label}>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">{label}</p>
            <p className="mt-2 text-2xl font-black">{value}</p>
            <p className="mt-1 text-xs text-muted-foreground">{desc}</p>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function UserPanel({ active }: { active: string }) {
  const [query, setQuery] = useState("Dune");
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [apiEnabled, setApiEnabled] = useState(false);

  if (active === "media") {
    return (
      <div className="space-y-4">
        <Card>
          <CardHeader><CardTitle>媒体搜索与求片</CardTitle><CardDescription>输入只过滤本地 mock 列表，不发送请求。</CardDescription></CardHeader>
          <CardContent className="space-y-4">
            <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索媒体" />
            <div className="grid gap-3 md:grid-cols-3">
              {demoMedia.filter((m) => m.title.toLowerCase().includes(query.toLowerCase()) || !query).map((m) => (
                <Card key={m.title} className="bg-muted/30">
                  <CardContent className="space-y-2 p-4">
                    <div className="flex items-start justify-between gap-2"><h3 className="font-bold">{m.title}</h3><Badge variant={statusVariant(m.status)}>{m.status}</Badge></div>
                    <p className="text-sm text-muted-foreground">{m.type} · {m.year} · 评分 {m.rating}</p>
                    <Button size="sm" className="w-full" disabled={m.status === "已入库"}>{m.status === "已入库" ? "已入库" : "模拟求片"}</Button>
                  </CardContent>
                </Card>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (active === "settings" || active === "apikey") {
    return (
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader><CardTitle>个人资料</CardTitle><CardDescription>本地状态模拟保存。</CardDescription></CardHeader>
          <CardContent className="space-y-3">
            <Label>用户名</Label><Input defaultValue="demo_user" />
            <Label>邮箱</Label><Input defaultValue="demo@example.com" />
            <div className="flex items-center justify-between rounded-xl border p-3"><span>显示背景动效</span><Switch defaultChecked /></div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle>API Key</CardTitle><CardDescription>演示密钥不会生成真实凭证。</CardDescription></CardHeader>
          <CardContent className="space-y-3">
            <div className="flex items-center justify-between rounded-xl border p-3"><span>启用 API Key</span><Switch checked={apiEnabled} onCheckedChange={setApiEnabled} /></div>
            <div className="relative"><Input readOnly type={passwordVisible ? "text" : "password"} value="twilight_demo_mock_key_123456" /><Button type="button" variant="ghost" size="icon" className="absolute right-1 top-1" onClick={() => setPasswordVisible((v) => !v)}><Eye className="h-4 w-4" /></Button></div>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <MetricGrid role="user" />
      <div className="grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
        <Card><CardHeader><CardTitle>我的求片记录</CardTitle></CardHeader><CardContent className="space-y-2">{demoRequests.map((r) => <div key={r.title} className="flex items-center justify-between rounded-xl border p-3 text-sm"><span>{r.title}</span><Badge variant={statusVariant(r.status)}>{r.status}</Badge></div>)}</CardContent></Card>
        <Card><CardHeader><CardTitle>公告与通知</CardTitle></CardHeader><CardContent className="space-y-3">{demoNotifications.map((n) => <div key={n.text} className="flex items-center gap-3 rounded-xl bg-muted/40 p-3 text-sm"><n.icon className="h-4 w-4 text-primary" />{n.text}</div>)}</CardContent></Card>
      </div>
    </div>
  );
}

function AdminPanel({ active }: { active: string }) {
  if (active === "users") {
    return (
      <Card><CardHeader><CardTitle>用户管理</CardTitle><CardDescription>包含注册队列清理与授权演示。</CardDescription></CardHeader><CardContent className="space-y-3">{demoUsers.map((u) => <div key={u.uid} className="grid gap-2 rounded-xl border p-3 text-sm md:grid-cols-[60px_1fr_100px_100px_100px_auto]"><span>#{u.uid}</span><b>{u.username}</b><Badge variant="outline">{u.role}</Badge><Badge variant={u.active ? "success" : "destructive"}>{u.active ? "启用" : "禁用"}</Badge><Badge variant={statusVariant(u.emby)}>{u.emby}</Badge><Button size="sm" variant="outline">模拟操作</Button></div>)}<div className="flex flex-wrap gap-2 pt-2"><Button>清理注册队列</Button><Button variant="secondary">队列用户授权并清理</Button></div></CardContent></Card>
    );
  }
  if (active === "scheduler") {
    return (
      <Card><CardHeader><CardTitle>定时任务</CardTitle><CardDescription>历史运行可展开查看每次日志。</CardDescription></CardHeader><CardContent className="space-y-3">{demoSchedulerRuns.map((run) => <details key={run.name} className="rounded-xl border p-3"><summary className="flex cursor-pointer items-center justify-between"><span className="font-semibold">{run.name}</span><span className="flex items-center gap-2"><Badge variant={statusVariant(run.status)}>{run.status}</Badge><span className="text-xs text-muted-foreground">{run.time}</span></span></summary><pre className="mt-3 rounded-lg bg-muted p-3 text-xs">{run.logs.join("\n")}</pre></details>)}</CardContent></Card>
    );
  }
  if (active === "requests") {
    return <Card><CardHeader><CardTitle>求片审核</CardTitle></CardHeader><CardContent className="space-y-3">{demoRequests.map((r) => <div key={r.title} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border p-3 text-sm"><div><b>{r.title}</b><p className="text-xs text-muted-foreground">{r.user} · {r.source} · {r.note}</p></div><div className="flex gap-2"><Badge variant={statusVariant(r.status)}>{r.status}</Badge><Button size="sm" variant="outline">审核</Button></div></div>)}</CardContent></Card>;
  }
  if (active === "security") {
    return <Card><CardHeader><CardTitle>安全审计</CardTitle></CardHeader><CardContent className="space-y-3">{demoAuditEvents.map((e) => <div key={`${e.actor}-${e.action}`} className="flex items-center justify-between rounded-xl border p-3 text-sm"><span><b>{e.actor}</b> {e.action} <span className="text-muted-foreground">{e.target}</span></span><Badge variant={statusVariant(e.level)}>{e.level}</Badge></div>)}</CardContent></Card>;
  }
  return (
    <div className="space-y-4">
      <MetricGrid role="admin" />
      <div className="grid gap-4 xl:grid-cols-3">
        {demoAnnouncements.map((a) => <Card key={a.title}><CardHeader><CardTitle className="flex items-center justify-between text-base">{a.title}<Badge variant="outline">{a.tag}</Badge></CardTitle></CardHeader><CardContent className="text-sm text-muted-foreground">{a.text}</CardContent></Card>)}
      </div>
      <Card><CardHeader><CardTitle>管理快捷操作</CardTitle></CardHeader><CardContent className="flex flex-wrap gap-2"><Button><PlayCircle className="mr-2 h-4 w-4" />运行任务</Button><Button variant="outline"><RefreshCw className="mr-2 h-4 w-4" />同步绑定</Button><Button variant="secondary"><CheckCircle2 className="mr-2 h-4 w-4" />批量启用</Button></CardContent></Card>
    </div>
  );
}

export function TestWebDemo({ role }: { role: DemoRole }) {
  const nav = role === "admin" ? adminDemoNav : userDemoNav;
  const [active, setActive] = useState(nav[0].key);
  const activeLabel = useMemo(() => nav.find((item) => item.key === active)?.label || nav[0].label, [active, nav]);

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top_left,hsl(var(--primary)/0.16),transparent_28rem),linear-gradient(135deg,hsl(var(--background)),hsl(var(--muted)/0.55))]">
      <div className="flex min-h-screen">
        <DemoSidebar role={role} nav={nav} active={active} setActive={setActive} />
        <div className="min-w-0 flex-1">
          <DemoHeader role={role} activeLabel={activeLabel} />
          <main className="mx-auto max-w-7xl space-y-4 p-4 md:p-6">
            <div className="flex gap-2 overflow-x-auto pb-1 lg:hidden">
              {nav.map((item) => <Button key={item.key} size="sm" variant={active === item.key ? "default" : "outline"} onClick={() => setActive(item.key)}><item.icon className="mr-1.5 h-4 w-4" />{item.label}</Button>)}
            </div>
            <motion.div key={active} initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.18 }}>
              {role === "admin" ? <AdminPanel active={active} /> : <UserPanel active={active} />}
            </motion.div>
          </main>
        </div>
      </div>
    </div>
  );
}
