"use client";

import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import {
  FileText,
  Plus,
  Copy,
  Trash2,
  Check,
  Loader2,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { api, type Regcode } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function AdminRegcodesPage() {
  const { toast } = useToast();
  const [regcodes, setRegcodes] = useState<Regcode[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [isLoading, setIsLoading] = useState(true);

  // Create dialog
  const [createOpen, setCreateOpen] = useState(false);
  const [activeTab, setActiveTab] = useState("1"); // 1: 注册, 2: 续期, 3: 白名单
  const [createData, setCreateData] = useState({
    days: "30",
    validityTime: "-1",
    useCountLimit: "1",
    count: "1",
  });
  const [isCreating, setIsCreating] = useState(false);
  const [createdCodes, setCreatedCodes] = useState<string[]>([]);

  useEffect(() => {
    loadRegcodes();
  }, [page]);

  const loadRegcodes = async () => {
    setIsLoading(true);
    try {
      const res = await api.getRegcodes(page);
      if (res.success && res.data) {
        const regcodesList = Array.isArray(res.data.regcodes) 
          ? res.data.regcodes 
          : Array.isArray(res.data) 
            ? res.data 
            : [];
        setRegcodes(regcodesList);
        setTotal(res.data.total || regcodesList.length);
      } else {
        setRegcodes([]);
        setTotal(0);
      }
    } catch (error) {
      console.error(error);
      setRegcodes([]);
      setTotal(0);
    } finally {
      setIsLoading(false);
    }
  };

  // 监听 Tab 切换，重置数据
  useEffect(() => {
    if (!createOpen) return;
    setCreatedCodes([]);
    if (activeTab === "1") {
      setCreateData({ days: "30", validityTime: "-1", useCountLimit: "1", count: "1" });
    } else if (activeTab === "2") {
      setCreateData({ days: "30", validityTime: "72", useCountLimit: "1", count: "1" });
    } else {
      setCreateData({ days: "-1", validityTime: "-1", useCountLimit: "-1", count: "1" });
    }
  }, [activeTab, createOpen]);

  const handleCreate = async () => {
    setIsCreating(true);
    try {
      const res = await api.createRegcode({
        type: parseInt(activeTab),
        days: parseInt(createData.days),
        validity_time: parseInt(createData.validityTime) || -1,
        use_count_limit: parseInt(createData.useCountLimit) || 1,
        count: parseInt(createData.count) || 1,
      });

      if (res.success && res.data) {
        toast({ title: "生成成功", variant: "success" });
        setCreatedCodes(res.data.codes || []);
        loadRegcodes();
      } else {
        toast({ title: "生成失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "生成失败", description: error.message, variant: "destructive" });
    } finally {
      setIsCreating(false);
    }
  };

  const handleDelete = async (code: string) => {
    if (!confirm("确定要删除这个卡码吗？")) return;

    try {
      const res = await api.deleteRegcode(code);
      if (res.success) {
        toast({ title: "删除成功", variant: "success" });
        loadRegcodes();
      } else {
        toast({ title: "删除失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "删除失败", description: error.message, variant: "destructive" });
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    toast({ title: "已复制到剪贴板" });
  };

  const getTypeBadge = (type: number) => {
    switch (type) {
      case 1:
        return <Badge variant="secondary" className="bg-blue-500/10 text-blue-500 border-blue-500/20">注册码</Badge>;
      case 2:
        return <Badge variant="default" className="bg-orange-500/10 text-orange-500 border-orange-500/20">续期码</Badge>;
      case 3:
        return <Badge variant="success" className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20">白名单</Badge>;
      default:
        return <Badge variant="secondary">未知</Badge>;
    }
  };

  const pages = Math.ceil(total / 20);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">卡码管理</h1>
          <p className="text-muted-foreground">批量生成注册、续期和白名单授权码</p>
        </div>
        <Dialog open={createOpen} onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreatedCodes([]);
          }
        }}>
          <DialogTrigger asChild>
            <Button variant="default" className="rounded-xl shadow-lg shadow-primary/20">
              <Plus className="mr-2 h-4 w-4" />
              生成授权码
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <DialogTitle>生成授权码</DialogTitle>
              <DialogDescription>请选择授权码类型并配置参数</DialogDescription>
            </DialogHeader>
            
            <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
              <TabsList className="grid w-full grid-cols-3 mb-4">
                <TabsTrigger value="1">注册</TabsTrigger>
                <TabsTrigger value="2">续期</TabsTrigger>
                <TabsTrigger value="3">白名单</TabsTrigger>
              </TabsList>

              <div className="space-y-4 py-2">
                <div className="space-y-2">
                  <Label>{activeTab === "3" ? "授权天数" : "账号天数"}</Label>
                  <Input
                    type="number"
                    value={createData.days}
                    onChange={(e) => setCreateData({ ...createData, days: e.target.value })}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    {activeTab === "3" ? "白名单用户的有效时长，-1 为永久" : "使用此码后账号增加的有效天数"}
                  </p>
                </div>
                
                <div className="space-y-2">
                  <Label>卡码本身的有效期 (小时)</Label>
                  <Input
                    type="number"
                    value={createData.validityTime}
                    onChange={(e) => setCreateData({ ...createData, validityTime: e.target.value })}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    在此时间内不使用则卡码失效，-1 为永久有效
                  </p>
                </div>

                <div className="space-y-2">
                  <Label>使用次数上限</Label>
                  <Input
                    type="number"
                    value={createData.useCountLimit}
                    onChange={(e) => setCreateData({ ...createData, useCountLimit: e.target.value })}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    该授权码可以被使用的总次数，-1 为无限制
                  </p>
                </div>

                <div className="space-y-2">
                  <Label>生成数量</Label>
                  <Input
                    type="number"
                    value={createData.count}
                    onChange={(e) => setCreateData({ ...createData, count: e.target.value })}
                    min="1"
                  />
                </div>

                {createdCodes.length > 0 && (
                  <div className="mt-4 space-y-2 p-3 bg-muted/50 rounded-xl border border-border">
                    <Label className="text-xs">已生成的代码</Label>
                    <div className="max-h-40 overflow-y-auto space-y-2 pr-1">
                      {createdCodes.map((code) => (
                        <div key={code} className="flex items-center gap-2 group">
                          <code className="flex-1 text-[12px] font-mono bg-background px-2 py-1.5 rounded-lg border border-border group-hover:border-primary/50 transition-colors">
                            {code}
                          </code>
                          <Button size="icon" variant="ghost" className="h-8 w-8 hover:bg-primary/10 hover:text-primary" onClick={() => copyToClipboard(code)}>
                            <Copy className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </Tabs>

            <DialogFooter className="mt-4">
              <Button variant="outline" onClick={() => setCreateOpen(false)}>
                取消
              </Button>
              <Button onClick={handleCreate} disabled={isCreating} className="min-w-[80px]">
                {isCreating ? <Loader2 className="h-4 w-4 animate-spin" /> : "立即生成"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex h-64 items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-primary" />
            </div>
          ) : !regcodes || regcodes.length === 0 ? (
            <div className="flex h-64 items-center justify-center text-muted-foreground">
              暂无注册码
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-4 py-3 text-left text-sm font-medium">注册码</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">类型</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">账号有效天数</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">注册码有效期</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">使用次数</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">状态</th>
                    <th className="px-4 py-3 text-left text-sm font-medium">创建时间</th>
                    <th className="px-4 py-3 text-right text-sm font-medium">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {regcodes.map((code) => (
                    <tr key={code.code} className="border-b hover:bg-muted/30">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <code className="rounded bg-muted px-2 py-1 text-sm">
                            {code.code}
                          </code>
                          <Button
                            size="icon"
                            variant="ghost"
                            className="h-6 w-6"
                            onClick={() => copyToClipboard(code.code)}
                          >
                            <Copy className="h-3 w-3" />
                          </Button>
                        </div>
                      </td>
                      <td className="px-4 py-3">{getTypeBadge(code.type)}</td>
                      <td className="px-4 py-3">{code.days} 天</td>
                      <td className="px-4 py-3 text-sm">
                        {code.validity_time === -1 || code.validity_time === undefined 
                          ? '永久有效' 
                          : `${code.validity_time} 小时`}
                      </td>
                      <td className="px-4 py-3 text-sm">
                        {code.use_count || 0} / {code.use_count_limit === -1 ? '∞' : code.use_count_limit || '∞'}
                      </td>
                      <td className="px-4 py-3">
                        <Badge variant={code.active === false ? "destructive" : "success"}>
                          {code.active === false ? "已禁用" : code.use_count && code.use_count_limit && code.use_count >= code.use_count_limit ? "已用完" : "可用"}
                        </Badge>
                        {code.used_by && (
                          <span className="ml-2 text-xs text-muted-foreground">
                            UID: {code.used_by}
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {formatDate(code.created_time || code.created_at)}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button
                          size="icon"
                          variant="ghost"
                          className="text-destructive hover:text-destructive"
                          onClick={() => handleDelete(code.code)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {pages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button
            variant="outline"
            size="icon"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page === 1}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="text-sm">
            第 {page} 页，共 {pages} 页
          </span>
          <Button
            variant="outline"
            size="icon"
            onClick={() => setPage((p) => Math.min(pages, p + 1))}
            disabled={page === pages}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      )}
    </div>
  );
}

