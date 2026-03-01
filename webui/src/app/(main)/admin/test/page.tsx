"use client";

import { useState, useEffect, useCallback } from "react";
import { motion } from "framer-motion";
import { TestTube, Loader2, CheckCircle2, XCircle, Plus, X, List, RefreshCw } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";

const container = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: {
      staggerChildren: 0.1,
    },
  },
};

const item = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0 },
};

interface ApiInfo {
  method: string;
  path: string;
  endpoint: string;
  full_path: string;
}

interface TestEndpoint {
  id: string;
  name: string;
  method: string;
  endpoint: string;
  description: string;
  body?: string;
}

export default function AdminTestPage() {
  const { toast } = useToast();
  const [testResults, setTestResults] = useState<Record<string, any>>({});
  const [isLoading, setIsLoading] = useState<Record<string, boolean>>({});
  
  // Custom API test state
  const [customMethod, setCustomMethod] = useState("GET");
  const [customEndpoint, setCustomEndpoint] = useState("");
  const [customParams, setCustomParams] = useState<{ key: string; value: string }[]>([]);
  const [customBody, setCustomBody] = useState("");
  
  // API list state
  const [apiList, setApiList] = useState<ApiInfo[]>([]);
  const [isLoadingApis, setIsLoadingApis] = useState(false);
  const [selectedApi, setSelectedApi] = useState<ApiInfo | null>(null);

  const testEndpoints: TestEndpoint[] = [
    {
      id: "health",
      name: "健康检查",
      method: "GET",
      endpoint: "/system/health",
      description: "检查系统健康状态",
    },
    {
      id: "system_info",
      name: "系统信息",
      method: "GET",
      endpoint: "/system/info",
      description: "获取系统基本信息",
    },
    {
      id: "stats",
      name: "统计数据",
      method: "GET",
      endpoint: "/admin/stats",
      description: "获取系统统计数据",
    },
    {
      id: "media_tmdb",
      name: "TMDB 媒体详情",
      method: "GET",
      endpoint: "/media/tmdb/550",
      description: "获取 TMDB 电影详情（ID: 550）",
    },
    {
      id: "media_bangumi",
      name: "Bangumi 媒体详情",
      method: "GET",
      endpoint: "/media/bangumi/400602",
      description: "获取 Bangumi 动画详情（ID: 400602）",
    },
  ];

  const handleTest = async (test: TestEndpoint) => {
    setIsLoading((prev) => ({ ...prev, [test.id]: true }));
    try {
      let response: any;
      
      // 使用 fetch 直接调用 API
      const token = localStorage.getItem("twilight_token");
      const headers: Record<string, string> = {
        "Content-Type": "application/json",
      };
      if (token) {
        headers["Authorization"] = `Bearer ${token}`;
      }
      
      const apiBase = process.env.NEXT_PUBLIC_API_URL || "http://localhost:5000";
      const url = `${apiBase}/api/v1${test.endpoint}`;
      
      const fetchOptions: RequestInit = {
        method: test.method,
        headers,
      };
      
      if (test.method === "POST" && test.body) {
        fetchOptions.body = test.body;
      }
      
      const fetchResponse = await fetch(url, fetchOptions);
      response = await fetchResponse.json();

      setTestResults((prev) => ({
        ...prev,
        [test.id]: {
          success: response.success !== false,
          data: response,
          timestamp: new Date().toLocaleString(),
        },
      }));

      toast({
        title: test.name,
        description: response.success !== false ? "测试成功" : response.message || "测试失败",
        variant: response.success !== false ? "success" : "destructive",
      });
    } catch (error: any) {
      setTestResults((prev) => ({
        ...prev,
        [test.id]: {
          success: false,
          error: error.message,
          timestamp: new Date().toLocaleString(),
        },
      }));

      toast({
        title: test.name,
        description: error.message || "测试失败",
        variant: "destructive",
      });
    } finally {
      setIsLoading((prev) => ({ ...prev, [test.id]: false }));
    }
  };

  const handleCustomTest = async () => {
    if (!customEndpoint.trim()) {
      toast({ title: "请输入接口路径", variant: "destructive" });
      return;
    }

    const testId = "custom";
    setIsLoading((prev) => ({ ...prev, [testId]: true }));
    
    try {
      const token = localStorage.getItem("twilight_token");
      const headers: Record<string, string> = {
        "Content-Type": "application/json",
      };
      if (token) {
        headers["Authorization"] = `Bearer ${token}`;
      }

      const apiBase = process.env.NEXT_PUBLIC_API_URL || "http://localhost:5000";
      
      // 构建 URL（包含查询参数）
      let url = `${apiBase}/api/v1${customEndpoint}`;
      if (customMethod === "GET" && customParams.length > 0) {
        const queryString = customParams
          .filter((p) => p.key && p.value)
          .map((p) => `${encodeURIComponent(p.key)}=${encodeURIComponent(p.value)}`)
          .join("&");
        if (queryString) {
          url += (customEndpoint.includes("?") ? "&" : "?") + queryString;
        }
      }

      const fetchOptions: RequestInit = {
        method: customMethod,
        headers,
      };

      if (["POST", "PUT", "PATCH"].includes(customMethod) && customBody.trim()) {
        fetchOptions.body = customBody;
      }

      const fetchResponse = await fetch(url, fetchOptions);
      const response = await fetchResponse.json();

      setTestResults((prev) => ({
        ...prev,
        [testId]: {
          success: response.success !== false,
          data: response,
          timestamp: new Date().toLocaleString(),
          request: {
            method: customMethod,
            url,
            body: customBody || undefined,
          },
        },
      }));

      toast({
        title: "自定义测试",
        description: response.success !== false ? "测试成功" : response.message || "测试失败",
        variant: response.success !== false ? "success" : "destructive",
      });
    } catch (error: any) {
      setTestResults((prev) => ({
        ...prev,
        [testId]: {
          success: false,
          error: error.message,
          timestamp: new Date().toLocaleString(),
        },
      }));

      toast({
        title: "自定义测试",
        description: error.message || "测试失败",
        variant: "destructive",
      });
    } finally {
      setIsLoading((prev) => ({ ...prev, [testId]: false }));
    }
  };

  const addParam = () => {
    setCustomParams([...customParams, { key: "", value: "" }]);
  };

  const removeParam = (index: number) => {
    setCustomParams(customParams.filter((_, i) => i !== index));
  };

  const updateParam = (index: number, field: "key" | "value", value: string) => {
    const newParams = [...customParams];
    newParams[index][field] = value;
    setCustomParams(newParams);
  };

  const loadApiList = useCallback(async () => {
    setIsLoadingApis(true);
    try {
      const res = await api.getAllApis();
      if (res.success && res.data) {
        setApiList(res.data.apis);
      } else {
        toast({
          title: "加载失败",
          description: res.message || "无法加载API列表",
          variant: "destructive",
        });
      }
    } catch (error: any) {
      const errorMessage = error.message || "请检查网络连接";
      // 如果是接口不存在，提供更详细的提示
      if (errorMessage.includes("接口不存在")) {
        toast({
          title: "接口不存在",
          description: "请确认后端服务已重启并包含最新代码。如果问题持续，请联系管理员。",
          variant: "destructive",
        });
      } else {
        toast({
          title: "加载失败",
          description: errorMessage,
          variant: "destructive",
        });
      }
    } finally {
      setIsLoadingApis(false);
    }
  }, [toast]);

  useEffect(() => {
    void loadApiList();
  }, [loadApiList]);

  const handleSelectApi = (apiInfo: ApiInfo) => {
    setSelectedApi(apiInfo);
    setCustomMethod(apiInfo.method);
    setCustomEndpoint(apiInfo.path);
    // 自动填入管理员token（从localStorage获取）
    const token = localStorage.getItem("twilight_token");
    if (token) {
      // 清空现有参数并添加Authorization
      setCustomParams([{ key: "Authorization", value: `Bearer ${token}` }]);
    } else {
      setCustomParams([]);
    }
  };

  return (
    <motion.div
      variants={container}
      initial="hidden"
      animate="show"
      className="space-y-6"
    >
      <div>
        <h1 className="text-3xl font-bold">API 测试工具</h1>
        <p className="text-muted-foreground">测试各种 API 接口</p>
      </div>

      <Tabs defaultValue="preset" className="space-y-6">
        <TabsList className="grid w-full max-w-2xl grid-cols-3">
          <TabsTrigger value="preset">预设接口</TabsTrigger>
          <TabsTrigger value="custom">自定义测试</TabsTrigger>
          <TabsTrigger value="list">API 列表</TabsTrigger>
        </TabsList>

        {/* Preset Tests */}
        <TabsContent value="preset">
          <motion.div variants={item}>
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <TestTube className="h-5 w-5" />
                  预设接口测试
                </CardTitle>
                <CardDescription>
                  点击测试按钮来测试各个 API 接口
                </CardDescription>
              </CardHeader>
              <CardContent>
            <div className="space-y-4">
              {testEndpoints.map((test) => (
                <div
                  key={test.id}
                  className="rounded-lg border p-4 space-y-3"
                >
                  <div className="flex items-start justify-between">
                    <div className="flex-1">
                      <div className="flex items-center gap-2 mb-1">
                        <Badge variant="outline">{test.method}</Badge>
                        <span className="font-medium">{test.name}</span>
                      </div>
                      <p className="text-sm text-muted-foreground mb-2">
                        {test.description}
                      </p>
                      <code className="text-xs bg-muted px-2 py-1 rounded">
                        {test.endpoint}
                      </code>
                    </div>
                    <Button
                      size="sm"
                      onClick={() => handleTest(test)}
                      disabled={isLoading[test.id]}
                    >
                      {isLoading[test.id] ? (
                        <>
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                          测试中...
                        </>
                      ) : (
                        "测试"
                      )}
                    </Button>
                  </div>

                  {testResults[test.id] && (
                    <div className="mt-3 pt-3 border-t">
                      <div className="flex items-center gap-2 mb-2">
                        {testResults[test.id].success ? (
                          <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                        ) : (
                          <XCircle className="h-4 w-4 text-destructive" />
                        )}
                        <span className="text-sm font-medium">
                          {testResults[test.id].success ? "成功" : "失败"}
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {testResults[test.id].timestamp}
                        </span>
                      </div>
                      <textarea
                        readOnly
                        value={JSON.stringify(
                          testResults[test.id].data || testResults[test.id].error,
                          null,
                          2
                        )}
                        className="font-mono text-xs h-32 w-full rounded-md border border-input bg-background px-3 py-2 resize-none"
                      />
                    </div>
                  )}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </motion.div>
        </TabsContent>

        {/* Custom Test */}
        <TabsContent value="custom">
          <motion.div variants={item}>
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <TestTube className="h-5 w-5" />
                  自定义 API 测试
                </CardTitle>
                <CardDescription>
                  输入自定义接口路径和参数进行测试
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label>请求方法</Label>
                    <Select value={customMethod} onValueChange={setCustomMethod}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="GET">GET</SelectItem>
                        <SelectItem value="POST">POST</SelectItem>
                        <SelectItem value="PUT">PUT</SelectItem>
                        <SelectItem value="PATCH">PATCH</SelectItem>
                        <SelectItem value="DELETE">DELETE</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-2">
                    <Label>接口路径</Label>
                    <Input
                      placeholder="/media/tmdb/123"
                      value={customEndpoint}
                      onChange={(e) => setCustomEndpoint(e.target.value)}
                    />
                  </div>
                </div>

                {/* Query Parameters (for GET) */}
                {customMethod === "GET" && (
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <Label>查询参数</Label>
                      <Button size="sm" variant="outline" onClick={addParam}>
                        <Plus className="mr-1 h-3 w-3" />
                        添加参数
                      </Button>
                    </div>
                    <div className="space-y-2">
                      {customParams.map((param, index) => (
                        <div key={index} className="flex gap-2">
                          <Input
                            placeholder="键"
                            value={param.key}
                            onChange={(e) => updateParam(index, "key", e.target.value)}
                            className="flex-1"
                          />
                          <Input
                            placeholder="值"
                            value={param.value}
                            onChange={(e) => updateParam(index, "value", e.target.value)}
                            className="flex-1"
                          />
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => removeParam(index)}
                          >
                            <X className="h-4 w-4" />
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Request Body (for POST/PUT/PATCH) */}
                {["POST", "PUT", "PATCH"].includes(customMethod) && (
                  <div className="space-y-2">
                    <Label>请求体 (JSON)</Label>
                    <textarea
                      placeholder='{"key": "value"}'
                      value={customBody}
                      onChange={(e) => setCustomBody(e.target.value)}
                      className="font-mono text-sm h-32 w-full rounded-md border border-input bg-background px-3 py-2 resize-none"
                    />
                  </div>
                )}

                <Button onClick={handleCustomTest} disabled={isLoading["custom"]} className="w-full">
                  {isLoading["custom"] ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      测试中...
                    </>
                  ) : (
                    <>
                      <TestTube className="mr-2 h-4 w-4" />
                      发送测试请求
                    </>
                  )}
                </Button>

                {/* Custom Test Results */}
                {testResults["custom"] && (
                  <div className="mt-4 pt-4 border-t">
                    <div className="flex items-center gap-2 mb-2">
                      {testResults["custom"].success ? (
                        <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                      ) : (
                        <XCircle className="h-4 w-4 text-destructive" />
                      )}
                      <span className="text-sm font-medium">
                        {testResults["custom"].success ? "成功" : "失败"}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {testResults["custom"].timestamp}
                      </span>
                    </div>
                    {testResults["custom"].request && (
                      <div className="mb-2 text-xs text-muted-foreground">
                        <code>{testResults["custom"].request.method} {testResults["custom"].request.url}</code>
                      </div>
                    )}
                    <textarea
                      readOnly
                      value={JSON.stringify(
                        testResults["custom"].data || testResults["custom"].error,
                        null,
                        2
                      )}
                      className="font-mono text-xs h-64 w-full rounded-md border border-input bg-background px-3 py-2 resize-none"
                    />
                  </div>
                )}
              </CardContent>
            </Card>
          </motion.div>
        </TabsContent>

        {/* API List */}
        <TabsContent value="list">
          <motion.div variants={item}>
            <Card>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <div>
                    <CardTitle className="flex items-center gap-2">
                      <List className="h-5 w-5" />
                      API 列表
                    </CardTitle>
                    <CardDescription>
                      点击API可自动填入到自定义测试中，并自动添加管理员权限
                    </CardDescription>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={loadApiList}
                    disabled={isLoadingApis}
                  >
                    {isLoadingApis ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        加载中...
                      </>
                    ) : (
                      <>
                        <RefreshCw className="mr-2 h-4 w-4" />
                        刷新
                      </>
                    )}
                  </Button>
                </div>
              </CardHeader>
              <CardContent>
                {isLoadingApis ? (
                  <div className="flex items-center justify-center h-64">
                    <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                  </div>
                ) : apiList.length === 0 ? (
                  <div className="text-center text-muted-foreground py-8">
                    暂无API列表
                  </div>
                ) : (
                  <div className="space-y-2 max-h-[600px] overflow-y-auto">
                    {apiList.map((apiInfo, index) => (
                      <div
                        key={index}
                        className={`flex items-center justify-between p-3 rounded-lg border cursor-pointer transition-colors ${
                          selectedApi?.full_path === apiInfo.full_path
                            ? "bg-primary/10 border-primary"
                            : "hover:bg-accent"
                        }`}
                        onClick={() => handleSelectApi(apiInfo)}
                      >
                        <div className="flex items-center gap-3 flex-1">
                          <Badge variant="outline">{apiInfo.method}</Badge>
                          <code className="text-sm flex-1">{apiInfo.path}</code>
                        </div>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleSelectApi(apiInfo);
                            // 切换到自定义测试标签页
                            const tabs = document.querySelector('[role="tablist"]');
                            const customTab = Array.from(tabs?.querySelectorAll('[role="tab"]') || []).find(
                              (tab) => tab.textContent === "自定义测试"
                            ) as HTMLElement;
                            customTab?.click();
                          }}
                        >
                          使用
                        </Button>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </motion.div>
        </TabsContent>
      </Tabs>
    </motion.div>
  );
}

