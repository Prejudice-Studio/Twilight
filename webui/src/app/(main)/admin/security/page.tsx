"use client";

import { useState } from "react";
import { motion } from "framer-motion";
import { Shield, AlertTriangle, Lock, Globe } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

const container = {
  hidden: { opacity: 0 },
  show: { opacity: 1, transition: { staggerChildren: 0.1 } },
};

const item = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0 },
};

export default function AdminSecurityPage() {
  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      <div>
        <h1 className="text-3xl font-bold">安全管理</h1>
        <p className="text-muted-foreground">系统安全设置和日志</p>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <motion.div variants={item}>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Lock className="h-5 w-5" />
                登录保护
              </CardTitle>
              <CardDescription>
                登录失败锁定和安全设置
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between rounded-lg bg-accent/50 p-3">
                <span className="text-sm">登录失败锁定</span>
                <Badge variant="success">已启用</Badge>
              </div>
              <div className="flex items-center justify-between rounded-lg bg-accent/50 p-3">
                <span className="text-sm">最大失败次数</span>
                <Badge variant="outline">5 次</Badge>
              </div>
              <div className="flex items-center justify-between rounded-lg bg-accent/50 p-3">
                <span className="text-sm">锁定时长</span>
                <Badge variant="outline">30 分钟</Badge>
              </div>
            </CardContent>
          </Card>
        </motion.div>

        <motion.div variants={item}>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Globe className="h-5 w-5" />
                IP 限制
              </CardTitle>
              <CardDescription>
                IP 黑名单和访问控制
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between rounded-lg bg-accent/50 p-3">
                <span className="text-sm">IP 黑名单</span>
                <Badge variant="secondary">0 个</Badge>
              </div>
              <div className="flex items-center justify-between rounded-lg bg-accent/50 p-3">
                <span className="text-sm">同 IP 最大用户数</span>
                <Badge variant="outline">不限制</Badge>
              </div>
              <Button variant="outline" className="w-full">
                管理黑名单
              </Button>
            </CardContent>
          </Card>
        </motion.div>

        <motion.div variants={item} className="md:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <AlertTriangle className="h-5 w-5" />
                安全日志
              </CardTitle>
              <CardDescription>
                最近的安全相关事件
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex h-32 items-center justify-center text-muted-foreground">
                暂无安全事件
              </div>
            </CardContent>
          </Card>
        </motion.div>
      </div>
    </motion.div>
  );
}

