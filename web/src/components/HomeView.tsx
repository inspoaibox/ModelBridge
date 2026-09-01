import React, { useState } from "react";
import {
  ArrowRight,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleDollarSign,
  Copy,
  Cpu,
  Database,
  LayoutDashboard,
  LogIn,
  LogOut,
  Network,
  ShieldCheck,
  Sparkles,
  Terminal,
  Users,
  Waypoints,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Language, PublicModelSummary, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface HomeViewProps {
  language: Language;
  signedIn: boolean;
  workspaceRoute: string;
  routeTo: (target: string) => void;
  handleSignOut: () => void;
  models: PublicModelSummary[];
  registrationEnabled: boolean;
}

export function HomeView({ language, signedIn, workspaceRoute, routeTo, handleSignOut, models, registrationEnabled }: HomeViewProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [activeCodeLang, setActiveCodeLang] = useState<"curl" | "python" | "nodejs" | "golang">("python");
  const [copiedCode, setCopiedCode] = useState(false);
  const gatewayBaseURL = window.location.origin + "/v1";

  const codeSnippets = {
    python: `import openai

client = openai.OpenAI(
    base_url="GATEWAY_BASE_URL",  # 替换为平台 API 地址
    api_key="YOUR_GATEWAY_TOKEN"
)

response = client.chat.completions.create(
    model="YOUR_MODEL_NAME",  # 使用模型广场中已配置并开放的模型名称
    messages=[{"role": "user", "content": "你好，请分析大模型网关的核心价值"}],
    stream=False
)

print(response.choices[0].message.content)`,
    curl: `curl GATEWAY_BASE_URL/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_GATEWAY_TOKEN" \\
  -d '{
    "model": "YOUR_MODEL_NAME",
    "messages": [{"role": "user", "content": "Hello AI Gateway!"}],
    "stream": false
  }'`,
    nodejs: `import OpenAI from "openai";

const openai = new OpenAI({
  baseURL: "GATEWAY_BASE_URL",
  apiKey: process.env.GATEWAY_API_KEY,
});

const completion = await openai.chat.completions.create({
  model: "YOUR_MODEL_NAME",
  messages: [{ role: "user", content: "Hello from Node.js" }],
});

completion.choices[0].message.content;`,
    golang: `package main

import (
    "context"
    "fmt"
    "os"
    "github.com/sashabaranov/go-openai"
)

func main() {
    config := openai.DefaultConfig(os.Getenv("GATEWAY_API_KEY"))
    config.BaseURL = "GATEWAY_BASE_URL"
    client := openai.NewClientWithConfig(config)

    resp, _ := client.CreateChatCompletion(
        context.Background(),
        openai.ChatCompletionRequest{
            Model: "YOUR_MODEL_NAME",
            Messages: []openai.ChatCompletionMessage{
                {Role: "user", Content: "Hello Go SDK"},
            },
        },
    )
    fmt.Println(resp.Choices[0].Message.Content)
}`,
  };

  const activeCode = codeSnippets[activeCodeLang].replaceAll("GATEWAY_BASE_URL", gatewayBaseURL);
  const handleCopyCode = () => {
    navigator.clipboard.writeText(activeCode);
    setCopiedCode(true);
    setTimeout(() => setCopiedCode(false), 2000);
  };

  const supportedModels = models.slice(0, 6);
  const featuredModel = supportedModels[0]?.name || "YOUR_MODEL_NAME";
  const availableModelCount = models.filter((model) => model.available).length;
  const providerCount = new Set(models.map((model) => model.provider)).size;
  const multimodalModelCount = models.filter((model) => model.category !== "text").length;

  return (
    <div className="space-y-16 lg:space-y-24 py-6 sm:py-10">
      {/* 1. Hero Section */}
      <section className="relative">
        <div className="grid min-w-0 gap-12 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,0.85fr)] lg:items-center">
          {/* Left Column: Hero Copy & Actions */}
          <div className="min-w-0 space-y-7">
            <div className="inline-flex items-center gap-2 rounded-full border border-indigo-500/30 bg-indigo-50 dark:bg-indigo-500/10 px-4 py-1.5 text-xs font-semibold text-indigo-700 dark:text-indigo-300 shadow-sm shadow-indigo-500/10">
              <Sparkles className="h-4 w-4 text-indigo-500 dark:text-indigo-400 animate-pulse" />
              <span>{t("homeEyebrow")}</span>
            </div>

            <div className="space-y-4">
              <h1 className="text-4xl font-extrabold tracking-tight text-slate-900 dark:text-white sm:text-5xl lg:text-6xl leading-[1.15]">
                {language === "zh" ? (
                  <>
                    统一大模型路由 <br />
                    <span className="text-gradient-primary">统一接入 · 透明记账</span>
                  </>
                ) : (
                  <>
                    Unified AI Relay <br />
                    <span className="text-gradient-primary">Millisecond Dispatching</span>
                  </>
                )}
              </h1>
              <p className="max-w-2xl text-base leading-relaxed text-slate-600 dark:text-slate-300 sm:text-lg">
                {t("homeDescription")}
              </p>
            </div>

            {/* Action Buttons */}
            <div className="flex flex-wrap items-center gap-4 pt-2">
              {signedIn ? (
                <Button size="lg" onClick={() => routeTo(workspaceRoute)} className="gap-2 shadow-lg shadow-indigo-500/30">
                  <LayoutDashboard className="h-5 w-5" />
                  <span>{t("enterWorkspace")}</span>
                  <ArrowRight className="h-4 w-4" />
                </Button>
              ) : (
                <Button size="lg" onClick={() => routeTo("#login")} className="gap-2 shadow-lg shadow-indigo-500/30">
                  <LogIn className="h-5 w-5" />
                  <span>{t("enterWorkspace")}</span>
                  <ArrowRight className="h-4 w-4" />
                </Button>
              )}

              {signedIn ? (
                <Button size="lg" variant="outline" onClick={handleSignOut} className="gap-2">
                  <LogOut className="h-4 w-4" />
                  <span>{t("signOut")}</span>
                </Button>
              ) : registrationEnabled ? (
                <Button size="lg" variant="outline" onClick={() => routeTo("#register")} className="gap-2">
                  <span>{t("navRegister")}</span>
                  <ChevronRight className="h-4 w-4 text-slate-400" />
                </Button>
              ) : null}
            </div>

            {/* Live Trust Indicators */}
            <div className="flex flex-wrap items-center gap-6 pt-3 text-xs text-slate-600 dark:text-slate-400">
              <div className="flex items-center gap-2">
                <CheckCircle2 className="h-4 w-4 text-emerald-600 dark:text-emerald-400" />
                <span>{language === "zh" ? "优先级与权重故障兜底" : "Priority and weighted failover"}</span>
              </div>
              <div className="flex items-center gap-2">
                <CheckCircle2 className="h-4 w-4 text-cyan-600 dark:text-cyan-400" />
                <span>{language === "zh" ? "双重记账与使用记录" : "Double-entry usage records"}</span>
              </div>
              <div className="flex items-center gap-2">
                <CheckCircle2 className="h-4 w-4 text-indigo-600 dark:text-indigo-400" />
                <span>{language === "zh" ? "全模型统一 OpenAI 标准协议" : "OpenAI Compatible"}</span>
              </div>
            </div>
          </div>

          {/* Right Column: High-Tech Gateway Request Dispatch Simulator Card */}
          <div className="relative min-w-0 overflow-hidden">
            <div className="absolute -inset-1 rounded-3xl bg-gradient-to-r from-indigo-500 via-cyan-500 to-emerald-500 opacity-20 blur-xl dark:opacity-30" />
            <div className="relative rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/90 dark:bg-slate-900/90 p-5 shadow-2xl backdrop-blur-xl space-y-4">
              {/* Header simulator */}
              <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 pb-3">
                <div className="flex items-center gap-2">
                  <div className="h-3 w-3 rounded-full bg-rose-500/80" />
                  <div className="h-3 w-3 rounded-full bg-amber-500/80" />
                  <div className="h-3 w-3 rounded-full bg-emerald-500/80" />
                  <span className="ml-2 font-mono text-xs text-slate-500 dark:text-slate-400">api-gateway / request-flow</span>
                </div>
                  <Badge variant="cyan" className="font-mono text-[10px]">
                  {language === "zh" ? "请求链路" : "REQUEST FLOW"}
                </Badge>
              </div>

              {/* Real-time Dispatch Pipeline Simulation */}
              <div className="space-y-3 font-mono text-xs">
                <div className="rounded-xl border border-slate-200 dark:border-slate-800/80 bg-slate-50 dark:bg-slate-950/60 p-3.5 space-y-2">
                  <div className="flex items-center justify-between text-[11px] text-slate-500 dark:text-slate-400">
                    <span className="flex items-center gap-1.5">
                      <span className="h-2 w-2 rounded-full bg-indigo-500 animate-ping" />
                      <span>{language === "zh" ? "请求进入" : "INBOUND REQUEST"}</span>
                    </span>
                    <span className="text-slate-400 dark:text-slate-500">POST /v1/chat/completions</span>
                  </div>
                  <div className="text-slate-800 dark:text-slate-200 font-semibold truncate">
                    Model: &quot;{featuredModel}&quot; · API Token: &quot;masked&quot;
                  </div>
                </div>

                {/* Gateway Decision Nodes */}
                <div className="grid grid-cols-3 gap-2 text-center text-[10px]">
                  <div className="rounded-lg border border-emerald-500/30 bg-emerald-50 dark:bg-emerald-500/10 p-2 text-emerald-700 dark:text-emerald-300">
                    <div className="font-bold">{language === "zh" ? "1. 身份校验" : "1. Authorization"}</div>
                    <div className="text-[9px] text-emerald-600 dark:text-emerald-400/80 mt-0.5">{language === "zh" ? "令牌与权限" : "Token and scope"}</div>
                  </div>
                  <div className="rounded-lg border border-indigo-500/30 bg-indigo-50 dark:bg-indigo-500/10 p-2 text-indigo-700 dark:text-indigo-300">
                    <div className="font-bold">{language === "zh" ? "2. 智能调度" : "2. Routing"}</div>
                    <div className="text-[9px] text-indigo-600 dark:text-indigo-400/80 mt-0.5">{language === "zh" ? "分组与渠道策略" : "Group and channel policy"}</div>
                  </div>
                  <div className="rounded-lg border border-cyan-500/30 bg-cyan-50 dark:bg-cyan-500/10 p-2 text-cyan-700 dark:text-cyan-300">
                    <div className="font-bold">{language === "zh" ? "3. 用量结算" : "3. Usage settlement"}</div>
                    <div className="text-[9px] text-cyan-600 dark:text-cyan-400/80 mt-0.5">{language === "zh" ? "按实际 Usage 记录" : "Based on actual usage"}</div>
                  </div>
                </div>

                {/* Upstream Dispatch Result */}
                <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950/80 p-3 space-y-1.5">
                  <div className="flex items-center justify-between text-[11px]">
                    <span className="text-indigo-600 dark:text-indigo-400 font-bold">{language === "zh" ? "响应完成" : "RESPONSE"}</span>
                    <span className="text-slate-500 dark:text-slate-400 font-mono">{language === "zh" ? "用量与费用进入记录" : "Usage and billing are recorded"}</span>
                  </div>
                  <div className="text-slate-600 dark:text-slate-400 text-[11px] leading-relaxed line-clamp-2">
                    &quot;统一模型入口、按分组调度渠道，并按实际 Usage 结算。&quot;
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* 2. Live catalog counters */}
      <section className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {[
          { label: language === "zh" ? "目录模型" : "Catalog models", value: String(models.length), desc: language === "zh" ? "来自渠道真实模型映射" : "Live channel mappings" },
          { label: language === "zh" ? "当前可用" : "Available now", value: String(availableModelCount), desc: language === "zh" ? "至少有一个健康渠道" : "At least one healthy route" },
          { label: language === "zh" ? "上游厂商" : "Providers", value: String(providerCount), desc: language === "zh" ? "当前目录中的厂商数量" : "Providers in the current catalog" },
          { label: language === "zh" ? "多模态模型" : "Multimodal models", value: String(multimodalModelCount), desc: language === "zh" ? "图片、音频、视频或向量能力" : "Image, audio, video, or embedding capability" },
        ].map((item, idx) => (
          <Card key={idx} className="glass-panel text-center">
            <CardHeader className="space-y-1 p-5 pb-3">
              <div className="text-3xl sm:text-4xl font-extrabold text-slate-900 dark:text-white font-mono">{item.value}</div>
              <div className="text-xs sm:text-sm font-semibold text-slate-700 dark:text-slate-300">{item.label}</div>
            </CardHeader>
            <CardContent className="px-5 pb-5 pt-0">
              <p className="text-[11px] text-slate-500 dark:text-slate-400">{item.desc}</p>
            </CardContent>
          </Card>
        ))}
      </section>

      {/* 3. Core Enterprise Capabilities Grid */}
      <section className="space-y-8">
        <div className="text-center space-y-3 max-w-3xl mx-auto">
          <div className="inline-flex items-center gap-2 rounded-full border border-indigo-500/30 bg-indigo-50 dark:bg-indigo-500/10 px-3.5 py-1 text-xs font-semibold text-indigo-700 dark:text-indigo-300">
            <span>{t("capabilitiesEyebrow")}</span>
          </div>
          <h2 className="text-3xl font-extrabold text-slate-900 dark:text-white sm:text-4xl tracking-tight">
            {t("capabilitiesTitle")}
          </h2>
          <p className="text-sm text-slate-600 dark:text-slate-400">
            专为企业多模型调用、混合多云接入与团队权限治理而设计的商业级基础设施。
          </p>
        </div>

        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {[
            {
              title: t("capabilityIdentityTitle"),
              desc: t("capabilityIdentityBody"),
              icon: Network,
              tag: "OpenAI Compatible",
            },
            {
              title: t("capabilityBillingTitle"),
              desc: t("capabilityBillingBody"),
              icon: Users,
              tag: "Multi-Tenant",
            },
            {
              title: t("capabilitySecurityTitle"),
              desc: t("capabilitySecurityBody"),
              icon: CircleDollarSign,
              tag: "Double-Entry",
            },
            {
              title: t("capabilityRelayTitle"),
              desc: t("capabilityRelayBody"),
              icon: ShieldCheck,
              tag: "Zero-Trust",
            },
            {
              title: language === "zh" ? "智能权重与故障容灾" : "Dynamic Routing & Failover",
              desc: language === "zh" ? "支持配置渠道优先级与权重分配，在上游返回可重试错误时自动切换备用渠道。" : "Configure priority and weight, then retry eligible upstream failures on a fallback channel.",
              icon: Waypoints,
               tag: "Failover Routing",
            },
            {
              title: language === "zh" ? "多维度分词高精计费" : "Granular Token Pricing",
              desc: language === "zh" ? "支持普通输入、普通输出、缓存输入与深度推理输出独立单价计费与最低消费设定。" : "Precise pricing versions for cached inputs, reasoning tokens, and minimum charges.",
              icon: Database,
              tag: "Precision Billing",
            },
          ].map((cap, idx) => {
            const Icon = cap.icon;
            return (
              <Card key={idx} className="glass-panel glass-panel-hover">
                <CardHeader className="space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-indigo-500/10 dark:bg-indigo-500/20 text-indigo-600 dark:text-indigo-400 border border-indigo-500/20">
                      <Icon className="h-5 w-5" />
                    </div>
                    <Badge variant="secondary" className="font-mono text-[10px]">
                      {cap.tag}
                    </Badge>
                  </div>
                  <CardTitle className="text-lg font-bold text-slate-900 dark:text-white">{cap.title}</CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="text-xs sm:text-sm text-slate-600 dark:text-slate-400 leading-relaxed">{cap.desc}</p>
                </CardContent>
              </Card>
            );
          })}
        </div>
      </section>

      {/* 4. Interactive Multi-Language Code Playground */}
      <section className="rounded-3xl border border-indigo-200/80 dark:border-slate-800/80 bg-white/90 dark:bg-slate-900/90 text-slate-900 dark:text-slate-100 p-6 sm:p-10 space-y-8 shadow-xl shadow-indigo-100/70 dark:shadow-black/30 relative overflow-hidden">
        
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between relative z-10">
          <div className="space-y-2">
            <div className="inline-flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-indigo-600 dark:text-indigo-400">
              <Terminal className="h-4 w-4" />
              <span>{t("integrationEyebrow")}</span>
            </div>
            <h2 className="text-2xl sm:text-3xl font-extrabold tracking-tight text-slate-900 dark:text-white">
              {language === "zh" ? "低改造成本，兼容主流 SDK" : "Low-friction SDK integration"}
            </h2>
            <p className="text-xs sm:text-sm text-slate-600 dark:text-slate-400 max-w-xl">
              直接使用官方 OpenAI SDK 或任一兼容框架，只需替换 <code className="text-indigo-600 dark:text-indigo-300 font-mono">baseURL</code> 与网关 Token。
            </p>
          </div>

          {/* Language Selector Buttons */}
          <div className="flex flex-wrap items-center gap-2 bg-slate-100 dark:bg-slate-950/80 p-1.5 rounded-xl border border-slate-200 dark:border-slate-800">
            {(["python", "curl", "nodejs", "golang"] as const).map((lang) => (
              <button
                key={lang}
                type="button"
                onClick={() => setActiveCodeLang(lang)}
                className={cn(
                  "rounded-lg px-3 py-1.5 text-xs font-mono font-semibold transition-all cursor-pointer",
                  activeCodeLang === lang
                    ? "bg-indigo-600 text-white shadow-md shadow-indigo-600/30"
                    : "text-slate-600 dark:text-slate-400 hover:bg-white/80 dark:hover:bg-slate-900/70 hover:text-slate-900 dark:hover:text-white"
                )}
              >
                {lang === "python" ? "Python" : lang === "curl" ? "cURL" : lang === "nodejs" ? "Node.js" : "Go SDK"}
              </button>
            ))}
          </div>
        </div>

        {/* Code Snippet Box */}
        <div className="relative rounded-2xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950 p-4 sm:p-6 font-mono text-xs sm:text-sm text-slate-800 dark:text-slate-200 overflow-x-auto shadow-inner">
          <button
            type="button"
            onClick={handleCopyCode}
            className="absolute top-4 right-4 flex items-center gap-1.5 rounded-lg border border-slate-300 dark:border-slate-800 bg-white/90 dark:bg-slate-900/80 px-3 py-1.5 text-xs text-slate-600 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors cursor-pointer"
          >
            {copiedCode ? (
              <>
                <Check className="h-3.5 w-3.5 text-emerald-400" />
                <span className="text-emerald-400">{language === "zh" ? "已复制" : "Copied"}</span>
              </>
            ) : (
              <>
                <Copy className="h-3.5 w-3.5" />
                <span>{language === "zh" ? "复制代码" : "Copy"}</span>
              </>
            )}
          </button>
          <pre className="pr-20 leading-relaxed whitespace-pre font-mono">
            {activeCode}
          </pre>
        </div>
      </section>

      {/* 5. Supported Model Matrix Section */}
      <section className="space-y-8">
        <div className="text-center space-y-3 max-w-3xl mx-auto">
          <div className="inline-flex items-center gap-2 rounded-full border border-cyan-500/30 bg-cyan-50 dark:bg-cyan-500/10 px-3.5 py-1 text-xs font-semibold text-cyan-700 dark:text-cyan-300">
            <Cpu className="h-3.5 w-3.5 text-cyan-500 dark:text-cyan-400" />
            <span>{language === "zh" ? "已配置模型目录" : "Configured Model Directory"}</span>
          </div>
          <h2 className="text-3xl font-extrabold text-slate-900 dark:text-white sm:text-4xl tracking-tight">
            {language === "zh" ? "当前可用模型" : "Available Models"}
          </h2>
          <p className="text-sm text-slate-600 dark:text-slate-400">
            {language === "zh" ? "模型来自已启用渠道的真实映射，统一通过分组策略、密钥保护与故障兜底提供服务。" : "Models come from live mappings on enabled channels and are served through group policy, secret protection, and failover."}
          </p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {supportedModels.length > 0 ? supportedModels.map((model) => (
            <Card key={model.id} className="glass-panel">
              <CardHeader className="p-5 pb-3">
                <div className="flex items-center justify-between">
                  <Badge variant="cyan" className="font-mono text-[10px]">
                    {model.provider}
                  </Badge>
                  <span className={cn("font-mono text-[11px] flex items-center gap-1", model.available ? "text-emerald-600 dark:text-emerald-400" : "text-slate-500 dark:text-slate-400")}>
                    <span className={cn("h-1.5 w-1.5 rounded-full", model.available ? "bg-emerald-500 animate-pulse" : "bg-slate-400")} />
                    {model.available ? t("modelsStatusAvailable") : t("modelsStatusUnavailable")}
                  </span>
                </div>
                <CardTitle className="text-base font-bold text-slate-900 dark:text-white pt-1">{model.display_name}</CardTitle>
              </CardHeader>
              <CardContent className="px-5 pb-5 pt-0">
                <p className="text-xs text-slate-600 dark:text-slate-400">{model.protocol_family} · {model.active_channel_count}/{model.channel_count} {t("modelsRoutes")}</p>
              </CardContent>
            </Card>
          )) : <div className="col-span-full rounded-2xl border border-dashed border-slate-300 py-10 text-center text-sm text-slate-500 dark:border-slate-700">{t("modelsNoConfigured")}</div>}
        </div>
        <div className="flex justify-center pt-1">
          <Button variant="outline" onClick={() => routeTo("#models")} className="gap-2">
            <Cpu className="h-4 w-4" />
            <span>{t("modelsViewAll")}</span>
            <ArrowRight className="h-4 w-4" />
          </Button>
        </div>
      </section>

      {/* 6. Onboarding Steps */}
      <section className="rounded-3xl border border-slate-200 dark:border-slate-800 bg-gradient-to-b from-indigo-50/50 to-white dark:from-indigo-950/20 dark:to-slate-950/60 p-6 sm:p-12 space-y-10">
        <div className="text-center space-y-2 max-w-2xl mx-auto">
          <h2 className="text-2xl sm:text-3xl font-extrabold text-slate-900 dark:text-white">
            {language === "zh" ? "只需 3 步，立即接入网关" : "Get Started in 3 Steps"}
          </h2>
          <p className="text-xs sm:text-sm text-slate-600 dark:text-slate-400">
            注册租户后，在控制台选择项目和分组创建自己的 API Token，再按公开模型目录发起调用。
          </p>
        </div>

        <div className="grid gap-6 md:grid-cols-3">
          {[
            {
              step: "01",
              title: language === "zh" ? "注册并登录控制台" : "Register and sign in",
              desc: language === "zh" ? "使用自己的账号创建租户空间，并进入租户控制台。" : "Create your tenant workspace with your own account.",
            },
            {
              step: "02",
              title: language === "zh" ? "配置上游渠道" : "Add Channels",
              desc: language === "zh" ? "由平台管理员维护上游渠道和模型映射，并发布可用模型。" : "Platform administrators maintain upstream channels and publish model mappings.",
            },
            {
              step: "03",
              title: language === "zh" ? "创建自己的 API Token" : "Create your API Token",
              desc: language === "zh" ? "为项目选择分组和安全限制，保存一次性显示的密钥后开始调用。" : "Choose a project, group, and security limits, then use the one-time secret to call the API.",
            },
          ].map((step) => (
            <div key={step.step} className="rounded-2xl border border-slate-200 dark:border-slate-800/80 bg-white/80 dark:bg-slate-900/60 p-6 space-y-3 relative">
              <div className="text-4xl font-extrabold text-indigo-500/20 dark:text-indigo-400/20 font-mono">
                {step.step}
              </div>
              <h3 className="text-lg font-bold text-slate-900 dark:text-white">{step.title}</h3>
              <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed">{step.desc}</p>
            </div>
          ))}
        </div>

        <div className="text-center pt-2">
          <Button size="lg" onClick={() => routeTo(signedIn ? workspaceRoute : "#login")} className="gap-2 shadow-xl shadow-indigo-500/30">
            <span>{t("enterWorkspace")}</span>
            <ArrowRight className="h-4 w-4" />
          </Button>
        </div>
      </section>
    </div>
  );
}
