"use client";

import { motion } from "framer-motion";
import { BookOpen, Shield, Users, Ticket, Tv, Key, Bell, RefreshCw, Calendar, MessageCircle, Settings, UserPlus, Eye, EyeOff, Ban, Trash2, Link2, LockKeyhole, Monitor, Eraser, Activity } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

const Section = ({ icon: Icon, title, children }: { icon: any; title: string; children: React.ReactNode }) => (
  <Card>
    <CardHeader className="pb-2">
      <CardTitle className="text-base flex items-center gap-2"><Icon className="h-5 w-5 text-primary" />{title}</CardTitle>
    </CardHeader>
    <CardContent className="text-sm text-muted-foreground space-y-2">{children}</CardContent>
  </Card>
);

export default function WikiPage() {
  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="space-y-6 max-w-4xl mx-auto py-8 px-4">
      <div>
        <h1 className="text-3xl font-bold flex items-center gap-3">
          <BookOpen className="h-8 w-8 text-primary" />
          Twilight 系统 Wiki
        </h1>
        <p className="text-muted-foreground mt-2">本页面提供系统所有功能的完整说明，适用于普通用户和管理员。不包含任何后端敏感配置信息。</p>
      </div>

      <Section icon={BookOpen} title="1. 系统概述">
        <p>Twilight 是一个 Emby / Jellyfin 用户管理面板，提供账号注册、邀请机制、签到积分、求片、Bangumi 同步、工单系统等功能。</p>
        <p>普通用户可通过注册码或邀请码注册，绑定 Emby 账号后享受媒体服务。管理员可通过后台管理面板管理用户、生成卡码、配置系统参数。</p>
      </Section>

      <Section icon={Users} title="2. 用户注册与登录">
        <p><Badge variant="outline" className="mr-1">注册</Badge>支持以下方式：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>注册码注册</strong>：管理员预先生成注册码（支持限次数、限有效期），用户持码注册。</li>
          <li><strong>邀请码注册</strong>：现有用户生成邀请链接/邀请码，新用户通过邀请注册后自动加入邀请树。</li>
          <li><strong>白名单码</strong>：管理员生成白名单码，持有者注册后直接获得白名单角色。</li>
        </ul>
        <p className="mt-2"><Badge variant="outline" className="mr-1">登录</Badge>支持用户名密码登录、邮箱登录（自动检测含 @ 的用户名）、API Key 登录。</p>
        <p>登录后会记录设备信息和 IP 地址，可在「设置 → 登录历史」中查看。</p>
      </Section>

      <Section icon={Tv} title="3. Emby 绑定与管理">
        <p>用户注册后需绑定 Emby 账号才能访问媒体内容：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>自助绑定</strong>：在仪表盘点击「绑定 Emby」，输入 Emby 服务器地址、用户名和密码。</li>
          <li><strong>自助注册</strong>：如果服务器配置允许，可自助创建新的 Emby 账号。</li>
          <li><strong>管理员绑定</strong>：管理员可在后台为用户强制绑定指定 Emby 账号。</li>
        </ul>
        <p className="mt-2">Emby 账号状态与 Web 账号状态是独立的：可以单独禁用 Emby 访问而不影响 Web 面板登录，反之亦然。</p>
        <p>管理员可通过「线路测速」功能检测各 Emby 服务器线路的连通性和速度。</p>
      </Section>

      <Section icon={Calendar} title="4. 账号到期与续期">
        <p>每个账号都有「到期时间」字段：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>有到期日</strong>：到期后账号自动禁用，Emby 同步关闭。</li>
          <li><strong>永久账号</strong>：设置后永不过期（管理员可取消永久）。</li>
          <li><strong>续期</strong>：使用续期码（注册码 type=1）、邀请续期码（RegCode source=invite）、签到积分兑换天数。</li>
        </ul>
        <p>管理员可在「用户管理 → 编辑/续期」中手动调整到期时间，或通过「批量到期调控」按筛选条件批量设置。</p>
      </Section>

      <Section icon={UserPlus} title="5. 用户自助功能">
        <p>登录后在「设置」页面可进行以下操作：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>修改资料</strong>：修改用户名（仅一次）、邮箱绑定与验证。</li>
          <li><strong>修改密码</strong>：修改 Web 面板密码、Emby 密码。</li>
          <li><strong>头像与背景</strong>：上传自定义头像和个人页背景图。</li>
          <li><strong>绑定 Telegram</strong>：绑定 TG 账号以接收登录通知和工单提醒。</li>
          <li><strong>API Key 管理</strong>：生成、启用/禁用、删除个人 API Key（用于脚本/第三方工具调用）。</li>
          <li><strong>登录通知</strong>：开启后在每次登录时收到 Telegram / 邮件通知。</li>
        </ul>
      </Section>

      <Section icon={RefreshCw} title="6. 签到与积分">
        <p>签到系统提供每日积分获取和续期兑换：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>每日签到</strong>：每天可获得随机积分（管理员可配置签到参数）。</li>
          <li><strong>积分续期</strong>：消耗积分兑换账号续期天数。</li>
          <li><strong>签到历史</strong>：最多保留 730 条记录（约 2 年），支持查看最近签到情况。</li>
        </ul>
        <p>管理员可在后台配置签到开关、每日积分范围和续期所需积分。</p>
      </Section>

      <Section icon={Users} title="7. 邀请系统">
        <p>邀请系统构建了用户之间的邀请关系树：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>生成邀请码</strong>：在「邀请」页面生成，可设置有效天数和使用次数。</li>
          <li><strong>续期码</strong>：邀请人可为已被邀请的下级生成续期码（即使邀请功能全局关闭仍可使用）。</li>
          <li><strong>邀请树</strong>：管理员可在后台查看完整的父子邀请关系树。</li>
          <li><strong>级联操作</strong>：管理员对用户执行某些操作时可选级联深度（如禁用用户的同时禁用其邀请的下级）。</li>
        </ul>
      </Section>

      <Section icon={Tv} title="8. 求片系统">
        <p>用户可在「求片」页面提交媒体请求：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li>支持 TMDB 和 Bangumi 两种数据源搜索。</li>
          <li>提交前会自动检查 Emby 库存，避免重复请求已有内容。</li>
          <li>管理员在后台可查看、更新状态（批准/拒绝/已完成）。</li>
        </ul>
      </Section>

      <Section icon={Ticket} title="9. 工单系统">
        <p>用户在「工单」页面提交问题或请求：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li>支持上传图片附件（每个工单最多上传数量由管理员配置，默认 5 张，单张最大 5MB）。</li>
          <li>支持文字回复：用户和管理员均可向工单追加文字回复，每次回复独立记录（显示回复人、时间、内容）。</li>
          <li>支持选择工单类型（管理员可在后台自定义类型）。</li>
          <li>工单关闭后普通用户不可再编辑或删除图片（冻结历史证据），管理员保留编辑权限。</li>
          <li>可开启 Telegram 通知，关注工单状态变更。</li>
          <li>管理员工单列表默认仅显示未解决工单，传 ?all=1 可查看全部。</li>
        </ul>
      </Section>

      <Section icon={Bell} title="10. 公告系统">
        <p>管理员发布系统公告，支持三种渲染模式：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>纯文本 (plain)</strong>：直接显示文字。</li>
          <li><strong>Markdown</strong>：安全子集（标题、列表、代码块、引用、分割线、链接、图片）。</li>
          <li><strong>BBCode</strong>：支持常见论坛标记。</li>
        </ul>
        <p>公告支持置顶（单独显示）、折叠和长内容截断。前台首页和仪表盘均可见。</p>
      </Section>

      <Section icon={BookOpen} title="11. Bangumi 同步与管理">
        <p>绑定 Bangumi (bgm.tv) Token 后可使用以下功能：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>自动同步</strong>：Emby / Jellyfin 播放记录通过 Webhook 自动同步到 Bangumi，标记对应剧集为已看。</li>
          <li><strong>收藏管理</strong>：在面板内查看/修改收藏状态（想看/看过/在看/搁置/抛弃）、观看进度、评分。</li>
          <li><strong>封面缓存</strong>：Bangumi 封面图片自动下载到服务器本地，优化加载速度。</li>
          <li><strong>最近动态</strong>：Dashboard 显示最近更新的收藏条目时间线。</li>
        </ul>
        <p>Bangumi 同步和收藏管理是两个独立开关：同步=自动记录播放、管理=手动画板管理收藏。</p>
      </Section>

      <Section icon={MessageCircle} title="12. Telegram Bot">
        <p>系统可配置 Telegram Bot 提供以下能力：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>用户指令</strong>：/bind（绑定）、/me（查看信息）、/emby（Emby 状态）、/resetpwd（重置密码）、/delaccount（注销）等。</li>
          <li><strong>管理指令</strong>：/stats（统计）、/userinfo（查用户）、/banweb（禁用 Web）、/banemby（禁用 Emby）、/broadcast（广播）等。</li>
          <li><strong>内联交互</strong>：Bot 中通过 inline button 可执行确认、编辑、回复操作。</li>
          <li><strong>自定义脚本</strong>：管理员可编写 JS 脚本（Goja 沙箱）扩展 Bot 功能。</li>
          <li><strong>登录通知</strong>：用户绑定 TG 后可接收每次登录的提醒。</li>
        </ul>
        <p>管理员可在后台管理页面配置 Bot Token、群组绑定、自定义指令和菜单。</p>
      </Section>

      <Section icon={Shield} title="13. 管理员后台功能速查">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground">用户管理</p>
            <p>筛选/搜索用户、编辑信息、续期/到期设定、重置密码、Emby 绑定/解绑、TG 绑定/解绑、角色设置、启停账号、删除用户</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground">批量操作</p>
            <p>选中多用户后：批量启用/禁用(Web)、批量 Emby 启用/禁用、批量续期/到期调控、批量删除、批量锁解绑、批量清授权记录</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground">注册码 / 邀请码</p>
            <p>批量生成注册码/续期码/白名单码、查看使用记录、编辑/删除特定码、查看码使用者列表</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground">系统配置</p>
            <p>TOML 原始编辑、Schema 可视化编辑、配置备份/恢复、服务器图标上传、认证页背景上传</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground">服务管理</p>
            <p>数据库备份/恢复/迁移、系统更新（Git pull）、实时日志查看、运行时状态监控、调度器任务管理</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground">安全审计</p>
            <p>操作审计日志查看/清除/裁剪、违规日志查看、IP 黑名单管理、设备审查（Emby 设备/IP 聚合）、登录历史</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground">其他管理功能</p>
            <p>公告管理、工单管理（含类型编辑）、邮件验证记录审查、Bangumi 用户同步管理、Telegram 花名册与换绑审批</p>
          </div>
        </div>
      </Section>

      <Section icon={Eye} title="14. 操作审计日志">
        <p>所有管理员的状态变更操作（创建/修改/删除/启停）均自动记录审计日志：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>记录内容</strong>：操作者 UID/用户名、操作类型、目标用户、详细信息、IP 地址、时间戳。</li>
          <li><strong>保留上限</strong>：默认 10000 条，超出自动裁剪最旧记录。</li>
          <li><strong>查询方式</strong>：管理员在「安全审计 → 操作日志」中查看、搜索、删除、清除。</li>
          <li><strong>覆盖范围</strong>：用户管理（CRUD/启停/续期/改密）、注册码管理、邀请管理、公告管理、工单管理、配置变更、Telegram Bot 操作等。</li>
        </ul>
      </Section>

      <Section icon={EyeOff} title="15. 禁用/解绑/清授权 语义说明">
        <div className="space-y-3">
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground inline-flex items-center gap-1"><Ban className="h-3.5 w-3.5 text-destructive" /> 禁用 Web 账号</p>
            <p>阻止用户登录 Web 面板。同时会级联禁用其 Emby 账号（可配置级联深度）。不会删除数据，启用后恢复。</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground inline-flex items-center gap-1"><Monitor className="h-3.5 w-3.5 text-destructive" /> 禁用 Emby</p>
            <p>单独停止该用户的 Emby 访问权限，不影响其 Web 面板登录。对应 Emby 服务器的「禁用该用户」操作。</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground inline-flex items-center gap-1"><Link2 className="h-3.5 w-3.5" /> 强制解绑</p>
            <p>仅解除本地绑定关系，不删除 Telegram/Emby 远端账号。用户仍可通过重新绑定找回关联。</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground inline-flex items-center gap-1"><LockKeyhole className="h-3.5 w-3.5" /> 禁止自助解绑 Emby</p>
            <p>锁定后用户无法在仪表盘自行解绑 Emby 账号。这是预防措施，防止用户绕过管理脱离管控。</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1 border-amber-500/30 bg-amber-500/5">
            <p className="font-bold text-foreground inline-flex items-center gap-1"><Eraser className="h-3.5 w-3.5 text-amber-500" /> 清理注册资格记录 (清授权)</p>
            <p>清理对象：<strong>使用了注册码/邀请码但没有正常注册 Emby 账号</strong>的用户的码使用记录。</p>
            <p className="text-amber-600 dark:text-amber-400">这些用户占用了码的使用次数配额但没有完成 Emby 注册，清理后他们可以再次使用注册码。已通过该码正常注册 Emby 的用户不受影响，他们不能再使用注册码（防止重复使用）。</p>
            <p>简单说：清授权 = 释放被 &ldquo;消耗了码但未注册 Emby&rdquo; 占用的资格，让码可以真正被需要的人使用。</p>
          </div>
          <div className="border rounded p-3 text-xs space-y-1">
            <p className="font-bold text-foreground inline-flex items-center gap-1"><Trash2 className="h-3.5 w-3.5 text-destructive" /> 删除用户</p>
            <p>删除本地用户数据。可选择同时删除 Emby 账号、仅保留 Emby（仅删本地）、仅删 Emby（保留本地和邀请关系）。管理员账号无法被删除（last-admin 保护）。</p>
          </div>
        </div>
      </Section>

      <Section icon={Monitor} title="17. 主页 Emby 库统计">
        <p>仪表盘 Emby 服务器卡片可显示服务器内的媒体库统计信息：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>显示内容</strong>：电影数量、电视剧数量、剧集总数。</li>
          <li><strong>默认关闭</strong>：管理员需在配置中启用 <Badge variant="outline">Emby库统计</Badge>（<code>[Emby] emby_stats_enabled = true</code>）。</li>
          <li>前端自动检测功能开关，开启后在 Emby 服务器卡片底部显示三列统计数据。</li>
        </ul>
      </Section>

      <Section icon={Activity} title="18. 播放统计">
        <p>独立于 Bangumi Webhook 的播放次数/时长统计系统：</p>
        <ul className="list-disc pl-5 space-y-1">
          <li><strong>默认关闭</strong>：管理员需在配置中启用 <Badge variant="outline">播放统计</Badge>（<code>[Emby] emby_playback_stats_enabled = true</code>）。</li>
          <li>后台自动从 Emby 活动日志中采集播放记录（VideoPlayback / VideoPlaybackComplete 等事件）。</li>
          <li>管理员可在「Emby 管理 → 活动日志」页签查看原始活动记录，手动刷新拉取最新数据。</li>
          <li>播放次数/时长等统计数据通过 <code>GET /admin/emby/playback-stats</code> 接口查询。</li>
        </ul>
      </Section>

      <Section icon={Settings} title="16. 常见问题">
        <div className="space-y-2">
          <p><strong>Q: 为什么我无法登录？</strong></p>
          <p>A: 可能原因：账号被禁用、已到期、密码错误。请联系管理员或使用「找回密码」功能。</p>
          <p className="mt-2"><strong>Q: Emby 提示密码错误怎么办？</strong></p>
          <p>A: 在「设置」页面可修改 Emby 密码（同 Web 密码或单独设置）。如忘记密码，可使用面板密码修改能力。</p>
          <p className="mt-2"><strong>Q: 如何获取注册码？</strong></p>
          <p>A: 注册码由管理员生成和分发。如你已是用户，可在「邀请」页面生成邀请码给朋友使用。</p>
          <p className="mt-2"><strong>Q: Telegram 绑定失败？</strong></p>
          <p>A: 请确认 Bot 已启动、你已加入配置的群组、TG ID 未被他人绑定。遇到问题请提工单。</p>
        </div>
      </Section>

      <div className="text-center text-xs text-muted-foreground py-8">
        <p>Twilight — Emby / Jellyfin 用户管理面板</p>
        <p>本页面不含后端敏感信息（密钥、Token、数据库连接等）。如有功能疑问请提交工单。</p>
      </div>
    </motion.div>
  );
}
