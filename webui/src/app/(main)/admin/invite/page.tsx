"use client";

/**
 * 邀请森林可视化（管理员）
 * ========================
 * 用 SVG 自绘一个紧凑的「星图 / 辐射树」：
 * - 每个根节点是一颗"恒星"，子节点围绕排布，越靠外层颜色越淡。
 * - 鼠标 hover 节点显示提示，点击在右侧抽屉展示用户详情。
 * - 不引入额外可视化库，方便部署到 Cloudflare Workers（rendering tree small）。
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion } from "framer-motion";
import {
  GitBranch,
  Loader2,
  RefreshCw,
  Network,
  X,
  Crown,
  Ban,
  Trash2,
  AlertTriangle,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { api, type InviteForest, type InviteForestNode } from "@/lib/api";
import {
  Sheet,
  SheetClose,
} from "./sheet-mini";

interface Positioned {
  uid: number;
  x: number;
  y: number;
  depth: number;
}

interface PlacedForest {
  positions: Map<number, Positioned>;
  width: number;
  height: number;
}

function placeForest(forest: InviteForest): PlacedForest {
  // 每个根做一颗辐射树：圆心是根节点，每层一个同心圆。
  const childrenMap = new Map<number, number[]>();
  for (const e of forest.edges) {
    if (!childrenMap.has(e.parent)) childrenMap.set(e.parent, []);
    childrenMap.get(e.parent)!.push(e.child);
  }

  const positions = new Map<number, Positioned>();
  const COLUMN_W = 380;
  const ROW_BASE = 80;
  const RADIUS_STEP = 80;
  let cursorX = 0;
  let totalHeight = ROW_BASE;

  // 计算每个根需要的最大半径
  const getSubtreeDepth = (root: number): number => {
    let max = 1;
    const queue: Array<{ uid: number; d: number }> = [{ uid: root, d: 1 }];
    const seen = new Set<number>([root]);
    while (queue.length) {
      const { uid, d } = queue.shift()!;
      max = Math.max(max, d);
      for (const c of childrenMap.get(uid) || []) {
        if (seen.has(c)) continue;
        seen.add(c);
        queue.push({ uid: c, d: d + 1 });
      }
    }
    return max;
  };

  for (const rootUid of forest.roots) {
    const depth = getSubtreeDepth(rootUid);
    const radius = (depth - 1) * RADIUS_STEP;
    const colWidth = Math.max(COLUMN_W, radius * 2 + 80);
    const centerX = cursorX + colWidth / 2;
    const centerY = ROW_BASE + radius;

    positions.set(rootUid, { uid: rootUid, x: centerX, y: centerY, depth: 1 });

    // BFS, 每一层用对应半径，按相对父节点的角度均匀分布
    const queue: Array<{ uid: number; angle: number; depth: number; sliceStart: number; sliceEnd: number }> =
      [{ uid: rootUid, angle: 0, depth: 1, sliceStart: 0, sliceEnd: Math.PI * 2 }];

    while (queue.length) {
      const item = queue.shift()!;
      const children = childrenMap.get(item.uid) || [];
      if (children.length === 0) continue;
      const sliceSize = (item.sliceEnd - item.sliceStart) / children.length;
      children.forEach((child, idx) => {
        const childAngle = item.sliceStart + sliceSize * (idx + 0.5);
        const r = item.depth * RADIUS_STEP;
        const px = centerX + Math.cos(childAngle) * r;
        const py = centerY + Math.sin(childAngle) * r;
        positions.set(child, { uid: child, x: px, y: py, depth: item.depth + 1 });
        queue.push({
          uid: child,
          angle: childAngle,
          depth: item.depth + 1,
          sliceStart: item.sliceStart + sliceSize * idx,
          sliceEnd: item.sliceStart + sliceSize * (idx + 1),
        });
      });
    }

    cursorX += colWidth + 40;
    totalHeight = Math.max(totalHeight, centerY + radius + ROW_BASE);
  }

  return {
    positions,
    width: Math.max(cursorX, 800),
    height: Math.max(totalHeight, 400),
  };
}

const DEPTH_COLOR_LIGHT = ["#0ea5e9", "#22c55e", "#a855f7", "#f59e0b", "#ef4444"];

function depthColor(depth: number): string {
  return DEPTH_COLOR_LIGHT[(depth - 1) % DEPTH_COLOR_LIGHT.length];
}

function findRoot(forest: InviteForest, uid: number): number {
  const parentOf = new Map<number, number>();
  for (const e of forest.edges) parentOf.set(e.child, e.parent);
  let cur = uid;
  const visited = new Set<number>([cur]);
  while (parentOf.has(cur)) {
    cur = parentOf.get(cur)!;
    if (visited.has(cur)) break;
    visited.add(cur);
  }
  return cur;
}

export default function AdminInviteTreePage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const [forest, setForest] = useState<InviteForest | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedUid, setSelectedUid] = useState<number | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [scale, setScale] = useState(1);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.adminGetInviteTree();
      if (res.success && res.data) {
        setForest(res.data);
      } else {
        throw new Error(res.message || "加载失败");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const placed = useMemo(() => (forest ? placeForest(forest) : null), [forest]);

  const nodeByUid = useMemo(() => {
    const map = new Map<number, InviteForestNode>();
    if (forest) for (const n of forest.nodes) map.set(n.uid, n);
    return map;
  }, [forest]);

  const selected = selectedUid && nodeByUid.get(selectedUid) ? nodeByUid.get(selectedUid)! : null;

  const handleDetach = async () => {
    if (!selected) return;
    const ok = await confirm({
      title: "把该用户从上级断开？",
      description: "断开后他将成为新树根；下级关系不变。",
      tone: "warning",
      confirmLabel: "断开",
    });
    if (!ok) return;
    const res = await api.adminDetachInviteUser(selected.uid).catch((err) => ({
      success: false,
      message: err instanceof Error ? err.message : "请求异常",
    }));
    if (res.success) {
      toast({ title: "已断开上级关系" });
      await reload();
    } else {
      toast({ title: "操作失败", description: res.message, variant: "destructive" });
    }
  };

  const handleCascadeDelete = async () => {
    if (!selected) return;
    const cascadeDepth = window.prompt(
      "请输入级联删除层级：1=仅本人；2=本人 + 直接邀请的下级；3=再往下一层；以此类推",
      "1",
    );
    if (cascadeDepth === null) return;
    const parsed = parseInt(cascadeDepth, 10);
    if (!Number.isFinite(parsed) || parsed < 1) {
      toast({ title: "请输入大于等于 1 的整数", variant: "destructive" });
      return;
    }
    const ok = await confirm({
      title: `确认级联删除 ${parsed} 层？`,
      description: parsed === 1 ? "仅删除本用户，子节点晋升为新树根。" : `将一并删除该用户与其向下 ${parsed - 1} 层的所有下级及其 Emby 账号。`,
      tone: "danger",
      confirmLabel: "确认删除",
    });
    if (!ok) return;
    const res = await api.deleteUserCascade(selected.uid, {
      deleteEmby: true,
      cascadeDepth: parsed,
    }).catch((err) => ({
      success: false,
      message: err instanceof Error ? err.message : "请求异常",
      data: null,
    }));
    if (res.success) {
      toast({ title: "级联删除完成" });
      setSelectedUid(null);
      await reload();
    } else {
      toast({ title: "操作失败", description: res.message, variant: "destructive" });
    }
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      className="space-y-4"
    >
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Network className="h-5 w-5" />
            邀请森林
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            管理员视角的整棵邀请关系。点击任意节点查看用户详情、断开/级联删除。
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => setScale((s) => Math.max(0.4, s - 0.1))}>
            −
          </Button>
          <span className="text-xs tabular-nums w-12 text-center">{Math.round(scale * 100)}%</span>
          <Button variant="outline" size="sm" onClick={() => setScale((s) => Math.min(2, s + 0.1))}>
            ＋
          </Button>
          <Button variant="outline" size="sm" onClick={() => void reload()} disabled={loading}>
            <RefreshCw className={`mr-1 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            刷新
          </Button>
        </div>
      </div>

      {forest && (
        <div className="grid gap-3 sm:grid-cols-4">
          <Card><CardContent className="p-4">
            <p className="text-[11px] uppercase tracking-widest text-muted-foreground">节点</p>
            <p className="text-2xl font-bold">{forest.nodes.length}</p>
          </CardContent></Card>
          <Card><CardContent className="p-4">
            <p className="text-[11px] uppercase tracking-widest text-muted-foreground">树根</p>
            <p className="text-2xl font-bold">{forest.roots.length}</p>
          </CardContent></Card>
          <Card><CardContent className="p-4">
            <p className="text-[11px] uppercase tracking-widest text-muted-foreground">最大深度</p>
            <p className="text-2xl font-bold">{forest.max_depth}</p>
          </CardContent></Card>
          <Card><CardContent className="p-4">
            <p className="text-[11px] uppercase tracking-widest text-muted-foreground">配置上限</p>
            <p className="text-2xl font-bold">{forest.config.max_depth}</p>
          </CardContent></Card>
        </div>
      )}

      {error ? (
        <Card className="border-destructive/40">
          <CardContent className="p-4 text-sm text-destructive flex items-center gap-2">
            <AlertTriangle className="h-4 w-4" />
            {error}
          </CardContent>
        </Card>
      ) : loading && !forest ? (
        <div className="flex h-60 items-center justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : !forest || forest.nodes.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="p-10 text-center space-y-2">
            <GitBranch className="h-10 w-10 mx-auto text-muted-foreground" />
            <p className="font-medium">暂无邀请关系</p>
            <p className="text-xs text-muted-foreground">用户启用邀请系统后会自动出现在这里。</p>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardContent className="p-0">
            <div
              ref={containerRef}
              className="overflow-auto"
              style={{ maxHeight: "70vh" }}
            >
              <div
                style={{
                  width: placed!.width * scale,
                  height: placed!.height * scale,
                  position: "relative",
                }}
              >
                <svg
                  viewBox={`0 0 ${placed!.width} ${placed!.height}`}
                  width={placed!.width * scale}
                  height={placed!.height * scale}
                  className="block select-none"
                >
                  {/* 边 */}
                  {forest.edges.map((e) => {
                    const p = placed!.positions.get(e.parent);
                    const c = placed!.positions.get(e.child);
                    if (!p || !c) return null;
                    return (
                      <line
                        key={`${e.parent}-${e.child}`}
                        x1={p.x}
                        y1={p.y}
                        x2={c.x}
                        y2={c.y}
                        stroke={depthColor(p.depth)}
                        strokeOpacity={0.4}
                        strokeWidth={1.4}
                      />
                    );
                  })}
                  {/* 节点 */}
                  {forest.nodes.map((n) => {
                    const pos = placed!.positions.get(n.uid);
                    if (!pos) return null;
                    const isSelected = selectedUid === n.uid;
                    const color = depthColor(pos.depth);
                    const r = pos.depth === 1 ? 14 : 10;
                    return (
                      <g
                        key={n.uid}
                        transform={`translate(${pos.x}, ${pos.y})`}
                        style={{ cursor: "pointer" }}
                        onClick={() => setSelectedUid(n.uid)}
                      >
                        <circle
                          r={r + 6}
                          fill={color}
                          opacity={isSelected ? 0.25 : 0.08}
                        />
                        <circle
                          r={r}
                          fill={color}
                          opacity={n.active ? 0.95 : 0.4}
                          stroke={isSelected ? "#fff" : color}
                          strokeWidth={isSelected ? 2 : 1}
                        />
                        <text
                          textAnchor="middle"
                          y={r + 14}
                          fontSize={11}
                          fontFamily="ui-sans-serif, system-ui"
                          fill="currentColor"
                          opacity={0.85}
                        >
                          {n.username}
                        </text>
                        {pos.depth === 1 && (
                          <text
                            textAnchor="middle"
                            y={-r - 6}
                            fontSize={9}
                            fontWeight={700}
                            fill={color}
                          >
                            ROOT
                          </text>
                        )}
                      </g>
                    );
                  })}
                </svg>
              </div>
            </div>
            <div className="border-t px-4 py-3 flex flex-wrap items-center gap-3 text-[11px] text-muted-foreground">
              <span>颜色对应层级：</span>
              {DEPTH_COLOR_LIGHT.slice(0, Math.max(1, forest.max_depth)).map((c, idx) => (
                <span key={c} className="flex items-center gap-1">
                  <span className="inline-block h-2.5 w-2.5 rounded-full" style={{ background: c }} />
                  L{idx + 1}
                </span>
              ))}
              <span className="ml-auto">点击节点查看详情</span>
            </div>
          </CardContent>
        </Card>
      )}

      {selected && (
        <Sheet onClose={() => setSelectedUid(null)}>
          <div className="space-y-3">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-lg font-bold flex items-center gap-2">
                  <Crown className="h-4 w-4 text-primary" />
                  {selected.username}
                </h3>
                <p className="text-xs text-muted-foreground mt-0.5">UID #{selected.uid}</p>
              </div>
              <SheetClose />
            </div>
            <div className="flex flex-wrap gap-2 text-[11px]">
              <Badge variant={selected.active ? "success" : "secondary"}>
                {selected.active ? "启用" : "禁用"}
              </Badge>
              <Badge variant={selected.emby_id ? "outline" : "secondary"}>
                {selected.emby_id ? "已绑 Emby" : "未绑 Emby"}
              </Badge>
              {selected.is_root && <Badge>树根</Badge>}
              {selected.telegram_id && (
                <Badge variant="outline">TG {selected.telegram_id}</Badge>
              )}
            </div>
            <dl className="space-y-1 text-xs">
              <div className="flex justify-between gap-2">
                <dt className="text-muted-foreground">角色</dt>
                <dd>{selected.role === 0 ? "管理员" : selected.role === 2 ? "白名单" : "普通用户"}</dd>
              </div>
              <div className="flex justify-between gap-2">
                <dt className="text-muted-foreground">注册时间</dt>
                <dd>
                  {selected.register_time
                    ? new Date(selected.register_time * 1000).toLocaleString("zh-CN")
                    : "—"}
                </dd>
              </div>
              <div className="flex justify-between gap-2">
                <dt className="text-muted-foreground">到期</dt>
                <dd>
                  {!selected.expired_at || selected.expired_at <= 0 || selected.expired_at >= 253402214400
                    ? "永久"
                    : new Date(selected.expired_at * 1000).toLocaleString("zh-CN")}
                </dd>
              </div>
              <div className="flex justify-between gap-2">
                <dt className="text-muted-foreground">所属根</dt>
                <dd>{forest ? findRoot(forest, selected.uid) : "—"}</dd>
              </div>
            </dl>
            <div className="grid gap-2 pt-2">
              <Button variant="outline" size="sm" onClick={handleDetach} disabled={selected.is_root}>
                <Ban className="mr-2 h-4 w-4" />
                {selected.is_root ? "已是树根" : "断开上级"}
              </Button>
              <Button variant="destructive" size="sm" onClick={handleCascadeDelete}>
                <Trash2 className="mr-2 h-4 w-4" />
                级联删除（自定义层级）
              </Button>
            </div>
          </div>
        </Sheet>
      )}
    </motion.div>
  );
}
