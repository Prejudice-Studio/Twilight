"use client";

import Link from "next/link";
import { motion } from "framer-motion";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ShieldCheck, User, UserCog } from "lucide-react";

export default function TestWebPage() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-[radial-gradient(circle_at_top,hsl(var(--primary)/0.18),transparent_28rem)] p-4">
      <div className="w-full max-w-5xl space-y-6">
      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.25 }}
        className="rounded-3xl border bg-card/85 p-5 shadow-2xl backdrop-blur sm:p-8"
      >
        <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
          <div>
            <h1 className="break-words text-3xl font-black tracking-tight sm:text-4xl">TestWeb 安全演示台</h1>
            <p className="mt-3 max-w-2xl text-sm leading-relaxed text-muted-foreground">
              这里复刻真实用户端与管理端的主要界面，但只连接 /api/v1/demo 模拟接口；不会读取登录态，也不会执行真实写入操作。
            </p>
          </div>
          <Badge variant="outline" className="w-fit gap-1.5"><ShieldCheck className="h-3.5 w-3.5" />Mock Only</Badge>
        </div>
      </motion.div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card className="overflow-hidden">
          <CardHeader><CardTitle className="flex items-center gap-2"><User className="h-5 w-5" />用户端演示</CardTitle><CardDescription>仪表盘、公告、求片、签到、邀请、设置、API Key。</CardDescription></CardHeader>
          <CardContent><Button asChild className="w-full"><Link href="/testwebuser">进入 /testwebuser</Link></Button></CardContent>
        </Card>
        <Card className="overflow-hidden">
          <CardHeader><CardTitle className="flex items-center gap-2"><UserCog className="h-5 w-5" />管理端演示</CardTitle><CardDescription>用户、注册码、求片审核、Emby、定时任务、配置、安全审计。</CardDescription></CardHeader>
          <CardContent><Button asChild variant="secondary" className="w-full"><Link href="/testwebadmin">进入 /testwebadmin</Link></Button></CardContent>
        </Card>
      </div>
      </div>
    </main>
  );
}
