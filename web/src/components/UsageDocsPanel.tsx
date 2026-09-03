import { useMemo, useState } from "react";
import {
  AlertTriangle,
  ArrowRight,
  BookOpen,
  Check,
  CheckCircle2,
  CircleDollarSign,
  Code2,
  Copy,
  Image as ImageIcon,
  KeyRound,
  LockKeyhole,
  Network,
  Route,
  ShieldCheck,
  Terminal,
  Video,
  Volume2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Language, PublicAPIEndpoint } from "@/types";
import { cn } from "@/lib/utils";
import { resolveAPIEndpointURLs } from "@/lib/api-endpoint";

interface UsageDocsPanelProps {
  language: Language;
  routeTo: (target: string) => void;
  apiEndpoints: PublicAPIEndpoint[];
}

type CodeTab = "python" | "node" | "curl" | "anthropic";

const sectionIDs = [
  "getting-started",
  "token-and-model",
  "first-call",
  "routing",
  "billing",
  "media",
  "security",
  "errors",
] as const;

function CodeBlock({
  code,
  language,
  onCopy,
  copied,
}: {
  code: string;
  language: Language;
  onCopy: () => void;
  copied: boolean;
}) {
  return (
    <div className="overflow-hidden rounded-2xl border border-slate-800 bg-[#101522] shadow-xl shadow-slate-950/10 dark:border-slate-700">
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
        <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-400">
          <Terminal className="h-3.5 w-3.5 text-cyan-400" />
          {language === "zh" ? "请求示例" : "Request example"}
        </div>
        <Button type="button" variant="ghost" size="sm" onClick={onCopy} className="h-7 gap-1.5 px-2 text-[11px] text-slate-300 hover:bg-white/10 hover:text-white">
          {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
          {copied ? (language === "zh" ? "已复制" : "Copied") : language === "zh" ? "复制" : "Copy"}
        </Button>
      </div>
      <pre className="overflow-x-auto p-5 text-xs leading-6 text-slate-200 sm:text-[13px]"><code>{code}</code></pre>
    </div>
  );
}

function InfoStep({ number, icon: Icon, title, body, tone }: { number: string; icon: typeof KeyRound; title: string; body: string; tone: "indigo" | "cyan" | "emerald" | "amber" }) {
  const tones = {
    indigo: "border-indigo-500/20 bg-indigo-500/[0.06] text-indigo-600 dark:text-indigo-300",
    cyan: "border-cyan-500/20 bg-cyan-500/[0.06] text-cyan-600 dark:text-cyan-300",
    emerald: "border-emerald-500/20 bg-emerald-500/[0.06] text-emerald-600 dark:text-emerald-300",
    amber: "border-amber-500/20 bg-amber-500/[0.06] text-amber-600 dark:text-amber-300",
  };
  return (
    <div className="flex gap-3 rounded-2xl border border-slate-200/80 bg-white/70 p-4 dark:border-slate-800 dark:bg-slate-900/50">
      <div className={cn("flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border", tones[tone])}>
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-mono text-[10px] font-bold text-slate-400">{number}</span>
          <h3 className="text-sm font-bold text-slate-900 dark:text-white">{title}</h3>
        </div>
        <p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-400">{body}</p>
      </div>
    </div>
  );
}

export function UsageDocsPanel({ language, routeTo, apiEndpoints }: UsageDocsPanelProps) {
  const zh = language === "zh";
  const configuredEndpoint = apiEndpoints[0];
  const endpointURLs = configuredEndpoint ? resolveAPIEndpointURLs(configuredEndpoint) : { root: "", openai: "", anthropic: "" };
  const gatewayBaseURL = endpointURLs.openai || "YOUR_OPENAI_BASE_URL";
  const anthropicBaseURL = endpointURLs.anthropic || "YOUR_ANTHROPIC_BASE_URL";
  const [activeTab, setActiveTab] = useState<CodeTab>("python");
  const [copied, setCopied] = useState("");

  const codeSamples = useMemo<Record<CodeTab, string>>(() => ({
    python: `from openai import OpenAI

client = OpenAI(
    api_key="YOUR_GATEWAY_TOKEN",
    base_url="GATEWAY_BASE_URL",
)

response = client.chat.completions.create(
    model="YOUR_MODEL_NAME",
    messages=[{"role": "user", "content": "你好"}],
)

print(response.choices[0].message.content)`,
    node: `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: process.env.GATEWAY_API_KEY,
  baseURL: "GATEWAY_BASE_URL",
});

const response = await client.chat.completions.create({
  model: "YOUR_MODEL_NAME",
  messages: [{ role: "user", content: "Hello" }],
});

const message = response.choices[0].message.content;`,
    curl: `curl GATEWAY_BASE_URL/chat/completions \\
  -H "Authorization: Bearer YOUR_GATEWAY_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "YOUR_MODEL_NAME",
    "messages": [{"role": "user", "content": "你好"}]
  }'`,
    anthropic: `import Anthropic from "@anthropic-ai/sdk";

const client = new Anthropic({
  apiKey: process.env.GATEWAY_API_KEY,
  baseURL: "ANTHROPIC_BASE_URL",
});

const message = await client.messages.create({
  model: "YOUR_MODEL_NAME",
  max_tokens: 1024,
  messages: [{ role: "user", content: "Hello AI Gateway!" }],
});

console.log(message.content);`,
  }), []);
  const activeCode = codeSamples[activeTab]
    .replaceAll("GATEWAY_BASE_URL", gatewayBaseURL)
    .replaceAll("ANTHROPIC_BASE_URL", anthropicBaseURL);

  const copyCode = async (id: string, value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(id);
      window.setTimeout(() => setCopied((current) => current === id ? "" : current), 1800);
    } catch {
      setCopied("");
    }
  };

  const sectionLabels = zh
    ? ["接入前准备", "令牌与模型", "第一次调用", "路由与故障兜底", "额度与计费", "图片、音频、视频", "安全使用", "错误排查"]
    : ["Before you start", "Tokens and models", "First request", "Routing and failover", "Balance and billing", "Images, audio, video", "Security", "Troubleshooting"];

  const scrollToSection = (id: string) => {
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  return (
    <div className="space-y-6 pb-10">
      <section className="relative overflow-hidden rounded-3xl border border-indigo-500/20 bg-gradient-to-br from-indigo-600 via-indigo-600 to-cyan-600 px-6 py-8 text-white shadow-xl shadow-indigo-900/15 sm:px-8 lg:px-10">
        <div className="relative max-w-3xl">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-white/20 bg-white/10 px-3 py-1.5 text-[11px] font-bold uppercase tracking-[0.16em] text-indigo-50">
            <BookOpen className="h-3.5 w-3.5" />
            {zh ? "客户接入中心" : "Developer onboarding"}
          </div>
          <h2 className="text-2xl font-extrabold tracking-tight sm:text-3xl">{zh ? "从控制台配置到第一次 API 调用" : "From console setup to your first API call"}</h2>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-indigo-50/90">{zh ? "完成令牌、分组和模型确认后，把标准 SDK 的 Base URL 指向本平台。系统会在请求链路中完成权限校验、渠道调度、故障切换和用量结算。" : "After confirming your token, group, and model, point a standard SDK Base URL at this gateway. The request path handles authorization, channel routing, failover, and usage settlement."}</p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Button type="button" onClick={() => routeTo("#console/tokens")} className="gap-2 border-white/20 bg-white text-indigo-700 shadow-lg hover:bg-indigo-50">
              <KeyRound className="h-4 w-4" />
              {zh ? "创建 API 令牌" : "Create API token"}
              <ArrowRight className="h-4 w-4" />
            </Button>
            <div className="inline-flex items-center gap-2 rounded-xl border border-white/20 bg-black/10 px-3.5 py-2 text-xs text-indigo-50">
              <CheckCircle2 className="h-4 w-4 text-emerald-200" />
              {zh ? "OpenAI 兼容接口" : "OpenAI-compatible APIs"}
            </div>
          </div>
        </div>
      </section>

      <div className="flex gap-2 overflow-x-auto pb-1 lg:hidden">
        {sectionIDs.map((id, index) => (
          <button key={id} type="button" onClick={() => scrollToSection(id)} className="inline-flex shrink-0 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs font-semibold text-slate-600 transition-colors hover:border-indigo-300 hover:text-indigo-700 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300 dark:hover:border-indigo-500/50 dark:hover:text-indigo-300">
            <span className="font-mono text-[10px] text-slate-400">{String(index + 1).padStart(2, "0")}</span>
            {sectionLabels[index]}
          </button>
        ))}
      </div>

      <div className="grid gap-6 lg:grid-cols-[220px_minmax(0,1fr)] xl:grid-cols-[240px_minmax(0,1fr)]">
        <aside className="hidden lg:block">
          <div className="sticky top-24 space-y-2 rounded-2xl border border-slate-200/80 bg-white/70 p-3 dark:border-slate-800 dark:bg-slate-900/50">
            <div className="px-3 pb-2 text-[10px] font-bold uppercase tracking-[0.16em] text-slate-400">{zh ? "文档目录" : "On this page"}</div>
            {sectionIDs.map((id, index) => (
              <button key={id} type="button" onClick={() => scrollToSection(id)} className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-xs font-medium text-slate-600 transition-colors hover:bg-indigo-50 hover:text-indigo-700 dark:text-slate-400 dark:hover:bg-indigo-500/10 dark:hover:text-indigo-300">
                <span className="font-mono text-[10px] text-slate-400">{String(index + 1).padStart(2, "0")}</span>
                <span className="truncate">{sectionLabels[index]}</span>
              </button>
            ))}
          </div>
        </aside>

        <main className="min-w-0 space-y-6">
          <section id="getting-started" className="scroll-mt-24">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg"><CheckCircle2 className="h-5 w-5 text-emerald-500" />{zh ? "接入前准备" : "Before you start"}</CardTitle>
                <CardDescription>{zh ? "客户只需要完成以下几项配置，渠道密钥和上游地址由平台管理员维护。" : "Complete these items first. Upstream credentials and channel URLs are maintained by platform administrators."}</CardDescription>
              </CardHeader>
              <CardContent className="grid gap-3 sm:grid-cols-2">
                <InfoStep number="01" icon={KeyRound} tone="indigo" title={zh ? "确认组织与项目" : "Confirm organization and project"} body={zh ? "当前账号必须属于一个有效租户，并且至少拥有一个可访问项目。" : "Your account must belong to an active tenant and have access to at least one project."} />
                <InfoStep number="02" icon={Route} tone="cyan" title={zh ? "选择路由分组" : "Choose a routing group"} body={zh ? "分组由管理员配置渠道、模型、倍率、计费类型和 RPM。令牌创建时必须绑定分组。" : "Admins configure channels, models, multipliers, billing type, and RPM in a group. Every token must bind to a group."} />
                <InfoStep number="03" icon={Code2} tone="emerald" title={zh ? "确认模型可见" : "Confirm model visibility"} body={zh ? "模型必须同时存在于分组关联渠道和令牌权限范围内，使用 GET /v1/models 可查看。" : "The model must exist on a channel linked to the group and be allowed for the token. Use GET /v1/models to verify."} />
                <InfoStep number="04" icon={CircleDollarSign} tone="amber" title={zh ? "确认余额与价格" : "Confirm balance and pricing"} body={zh ? "按量分组需要有效预付余额和已发布价格；免费分组不扣减余额，但仍记录使用情况。" : "Prepaid groups need an active balance and published pricing. Free groups do not debit balance but still record usage."} />
              </CardContent>
            </Card>
          </section>

          <section id="token-and-model" className="scroll-mt-24">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg"><KeyRound className="h-5 w-5 text-indigo-500" />{zh ? "令牌、分组和模型的关系" : "How tokens, groups, and models work together"}</CardTitle>
                <CardDescription>{zh ? "令牌不是上游密钥，而是客户调用本平台时的身份和授权凭证。" : "A token is not an upstream key. It is the identity and authorization credential used to call this gateway."}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-3 md:grid-cols-[1fr_auto_1fr_auto_1fr] md:items-center">
                  {[
                    { icon: KeyRound, title: zh ? "API 令牌" : "API token", body: zh ? "账号、项目、白名单" : "Account, project, allowlist", tone: "bg-indigo-500/10 text-indigo-600" },
                    { icon: Route, title: zh ? "路由分组" : "Routing group", body: zh ? "渠道、倍率、RPM" : "Channels, multiplier, RPM", tone: "bg-cyan-500/10 text-cyan-600" },
                    { icon: Network, title: zh ? "可调用模型" : "Available model", body: zh ? "分组内的模型映射" : "Mappings in the group", tone: "bg-emerald-500/10 text-emerald-600" },
                  ].map((item, index) => { const Icon = item.icon; return <div key={item.title} className="flex items-center gap-3 rounded-xl border border-slate-200/80 bg-slate-50/80 p-3 dark:border-slate-800 dark:bg-slate-950/30"><div className={cn("flex h-9 w-9 shrink-0 items-center justify-center rounded-lg", item.tone)}><Icon className="h-4 w-4" /></div><div className="min-w-0"><div className="text-xs font-bold text-slate-900 dark:text-white">{item.title}</div><div className="mt-1 text-[11px] text-slate-500 dark:text-slate-400">{item.body}</div></div>{index < 2 ? <ArrowRight className="ml-auto hidden h-4 w-4 shrink-0 text-slate-300 md:block" /> : null}</div>; })}
                </div>
                <div className="rounded-xl border border-indigo-500/20 bg-indigo-500/[0.06] p-4 text-xs leading-6 text-slate-700 dark:text-slate-300">
                  <span className="font-bold text-indigo-700 dark:text-indigo-300">{zh ? "关键规则：" : "Key rule: "}</span>
                  {zh ? "客户不直接选择某一个上游渠道。请求只提交平台模型名，平台按照令牌绑定的分组选择可用渠道；如果高优先级渠道失败，会按优先级和权重尝试备用渠道。" : "Clients do not select an upstream channel directly. Submit the platform model name; the group selects eligible channels. Higher-priority failures can fall back by priority and weight."}
                </div>
                <Button type="button" variant="outline" size="sm" onClick={() => routeTo("#console/tokens")} className="gap-2"><KeyRound className="h-3.5 w-3.5" />{zh ? "前往我的 API 令牌" : "Open my API tokens"}<ArrowRight className="h-3.5 w-3.5" /></Button>
              </CardContent>
            </Card>
          </section>

          <section id="first-call" className="scroll-mt-24">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg"><Terminal className="h-5 w-5 text-cyan-500" />{zh ? "第一次调用" : "Your first request"}</CardTitle>
                <CardDescription>{zh ? "所有 OpenAI 兼容 SDK 只需要替换 Base URL 和 API Key。" : "For OpenAI-compatible SDKs, replace only the Base URL and API key."}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="rounded-xl border border-indigo-500/20 bg-indigo-500/[0.06] p-4 dark:border-indigo-400/20">
                    <div className="flex flex-wrap items-center gap-2 text-[11px] font-bold uppercase tracking-[0.14em] text-indigo-700 dark:text-indigo-300">OpenAI-compatible <span className="rounded-full border border-indigo-500/25 px-2 py-0.5 text-[10px] tracking-normal">{zh ? "推荐" : "Recommended"}</span></div>
                    <code className="mt-2 block break-all text-xs text-indigo-700 dark:text-indigo-300">{gatewayBaseURL}</code>
                    <p className="mt-2 text-[11px] leading-5 text-slate-500 dark:text-slate-400">{configuredEndpoint ? (zh ? "用于 OpenAI SDK、兼容客户端和大多数第三方工具。" : "Use this with the OpenAI SDK, compatible clients, and most third-party tools.") : (zh ? "管理员尚未配置公开终端，请先在 API 令牌页面查看可用地址。" : "The administrator has not configured a public endpoint yet. Check the API token page first.")}</p>
                  </div>
                  <div className="rounded-xl border border-cyan-500/20 bg-cyan-500/[0.06] p-4 dark:border-cyan-400/20">
                    <div className="text-[11px] font-bold uppercase tracking-[0.14em] text-cyan-700 dark:text-cyan-300">Anthropic / Claude</div>
                    <code className="mt-2 block break-all text-xs text-cyan-700 dark:text-cyan-300">{anthropicBaseURL}</code>
                    <p className="mt-2 text-[11px] leading-5 text-slate-500 dark:text-slate-400">{zh ? "使用 Anthropic SDK 时填写根地址；服务端同时兼容 /v1/messages 和 /messages。" : "Use the gateway root with the Anthropic SDK. The server accepts both /v1/messages and /messages."}</p>
                  </div>
                  <div className="rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/30"><div className="text-[11px] font-bold uppercase tracking-[0.14em] text-slate-400">Authorization</div><code className="mt-2 block break-all text-xs text-emerald-700 dark:text-emerald-300">Bearer YOUR_GATEWAY_TOKEN</code><p className="mt-2 text-[11px] leading-5 text-slate-500">{zh ? "完整令牌只在创建成功时显示一次。" : "The full token is shown only once after creation."}</p></div>
                </div>
                <div className="flex flex-wrap gap-2 border-b border-slate-200 pb-3 dark:border-slate-800">
                  {(["python", "node", "curl", "anthropic"] as CodeTab[]).map((tab) => <button key={tab} type="button" onClick={() => setActiveTab(tab)} className={cn("rounded-lg px-3 py-1.5 text-xs font-semibold transition-colors", activeTab === tab ? "bg-indigo-600 text-white" : "text-slate-500 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800")}>{tab === "python" ? "Python" : tab === "node" ? "Node.js" : tab === "curl" ? "cURL" : "Anthropic"}</button>)}
                </div>
                <CodeBlock language={language} code={activeCode} copied={copied === `code-${activeTab}`} onCopy={() => void copyCode(`code-${activeTab}`, activeCode)} />
                <p className="text-xs leading-5 text-slate-500 dark:text-slate-400">{zh ? "Base URL 建议直接复制 API 令牌页面的对应地址。平台会兼容根地址、/v1 以及客户端误拼出的 /v1/v1，不需要客户手动修改请求路径。" : "Copy the matching address from the API token page. The gateway accepts the root, /v1, and an accidental /v1/v1 prefix, so clients do not need manual path workarounds."}</p>
              </CardContent>
            </Card>
          </section>

          <section id="routing" className="scroll-mt-24">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
              <CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Route className="h-5 w-5 text-cyan-500" />{zh ? "系统如何路由和兜底" : "Routing and failover"}</CardTitle><CardDescription>{zh ? "客户看到的是一个稳定的模型入口，渠道切换由平台完成。" : "Clients see one stable model endpoint while the platform handles channel selection."}</CardDescription></CardHeader>
              <CardContent className="space-y-3">
                {[
                  ["01", zh ? "身份与权限" : "Identity and authorization", zh ? "校验令牌状态、项目范围、模型白名单、IP/域名白名单和令牌限流。" : "The gateway checks token status, project scope, model allowlist, IP/domain allowlist, and token limits.", "bg-indigo-500/10 text-indigo-600"],
                  ["02", zh ? "分组策略" : "Group policy", zh ? "读取令牌绑定分组的状态、倍率、RPM、计费类型和可用渠道集合。" : "The bound group supplies status, multiplier, RPM, billing type, and eligible channels.", "bg-cyan-500/10 text-cyan-600"],
                  ["03", zh ? "渠道选择" : "Channel selection", zh ? "先按 priority 从高到低分层，同一优先级内按 weight 选择；失败时切换备用渠道。" : "Channels are tiered by priority, then selected by weight within a tier. Retryable failures can fall back.", "bg-amber-500/10 text-amber-600"],
                  ["04", zh ? "实际用量结算" : "Usage settlement", zh ? "成功响应后使用上游 Usage 和价格快照结算，并释放预占差额。" : "After success, upstream usage and the price snapshot settle the request and release the reservation difference.", "bg-emerald-500/10 text-emerald-600"],
                ].map(([number, title, body, tone]) => <div key={number} className="flex gap-3 rounded-xl border border-slate-200/80 p-4 dark:border-slate-800"><div className={cn("flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-[10px] font-bold", tone)}>{number}</div><div><h3 className="text-sm font-bold text-slate-900 dark:text-white">{title}</h3><p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-400">{body}</p></div></div>)}
                <div className="flex items-start gap-2 rounded-xl border border-amber-500/25 bg-amber-50 p-3 text-xs leading-5 text-amber-800 dark:bg-amber-500/10 dark:text-amber-200"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />{zh ? "流式响应一旦已经向客户端输出内容，平台不会再切换渠道，以避免把两个上游响应拼接在一起。" : "Once a streaming response has reached the client, the gateway will not switch channels, preventing two upstream responses from being joined."}</div>
              </CardContent>
            </Card>
          </section>

          <section id="billing" className="scroll-mt-24">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
              <CardHeader><CardTitle className="flex items-center gap-2 text-lg"><CircleDollarSign className="h-5 w-5 text-emerald-500" />{zh ? "额度与计费流程" : "Balance and billing"}</CardTitle><CardDescription>{zh ? "计费按分组倍率执行，记录中保留实际用量、价格版本和逐组件费用。" : "Billing applies the group multiplier and records actual usage, price versions, and itemized charge lines."}</CardDescription></CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-3">
                  {[{ icon: LockKeyhole, title: zh ? "预占" : "Reserve", body: zh ? "请求开始时按估算用量暂存额度。" : "A conservative estimate is reserved before the upstream call." }, { icon: Terminal, title: zh ? "调用" : "Call", body: zh ? "实际请求经过渠道优先级、权重和兜底。" : "The request follows channel priority, weight, and failover rules." }, { icon: CircleDollarSign, title: zh ? "结算" : "Settle", body: zh ? "按上游 Usage 结算，释放未使用预占。" : "Actual upstream usage settles the request and releases the remainder." }].map((item) => { const Icon = item.icon; return <div key={item.title} className="rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/30"><Icon className="h-4 w-4 text-emerald-500" /><div className="mt-3 text-sm font-bold text-slate-900 dark:text-white">{item.title}</div><p className="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">{item.body}</p></div>; })}
                </div>
                <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800"><table className="w-full min-w-[620px] text-left text-xs"><thead className="bg-slate-50 text-slate-500 dark:bg-slate-950/50 dark:text-slate-400"><tr><th className="px-4 py-3 font-semibold">{zh ? "计量类型" : "Meter"}</th><th className="px-4 py-3 font-semibold">{zh ? "示例" : "Examples"}</th><th className="px-4 py-3 font-semibold">{zh ? "说明" : "Notes"}</th></tr></thead><tbody className="divide-y divide-slate-200 dark:divide-slate-800">{[[zh ? "Token" : "Tokens", "input_tokens / output_tokens / cached_input_tokens", zh ? "按每 1M Token 价格换算。" : "Priced per 1M tokens."], [zh ? "媒体" : "Media", "input_images / output_images / audio_seconds / video_seconds", zh ? "按图片数量、秒数或媒体 Token 计费。" : "Priced by image count, seconds, or media tokens."], [zh ? "请求与字符" : "Requests and characters", "requests / input_characters / output_characters", zh ? "适用于请求费或字符计费模型。" : "Used by request-priced or character-priced models."]].map((row) => <tr key={row[0]} className="text-slate-600 dark:text-slate-300"><td className="px-4 py-3 font-semibold text-slate-900 dark:text-white">{row[0]}</td><td className="px-4 py-3 font-mono text-[11px]">{row[1]}</td><td className="px-4 py-3">{row[2]}</td></tr>)}</tbody></table></div>
                <p className="text-xs leading-5 text-slate-500 dark:text-slate-400">{zh ? "上游没有返回且无法验证的用量不会按 0 元放行。图片数量、请求字符数、明确的视频秒数等可验证计量会与上游 Usage 合并记录。" : "Unknown usage is never silently treated as zero. Verifiable quantities such as image count, request characters, and explicit video seconds are recorded alongside upstream Usage."}</p>
              </CardContent>
            </Card>
          </section>

          <section id="media" className="scroll-mt-24">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
              <CardHeader><CardTitle className="flex items-center gap-2 text-lg"><ImageIcon className="h-5 w-5 text-fuchsia-500" />{zh ? "图片、音频和视频接口" : "Images, audio, and video"}</CardTitle><CardDescription>{zh ? "媒体模型也必须先配置到渠道和分组中，接口不会绕过现有权限规则。" : "Media models must still be configured on a channel and in a group. Media endpoints do not bypass existing authorization rules."}</CardDescription></CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-2">
                  {[
                    { icon: ImageIcon, color: "text-fuchsia-500", title: zh ? "图片生成与编辑" : "Image generation and edits", endpoint: "POST /v1/images/generations · POST /v1/images/edits", body: zh ? "生成接口使用 JSON；编辑接口使用 multipart/form-data，字段为 image，可选 mask。" : "Generation uses JSON. Edits use multipart/form-data with image and an optional mask." },
                    { icon: Volume2, color: "text-cyan-500", title: zh ? "音频转写与语音" : "Transcription and speech", endpoint: "POST /v1/audio/transcriptions · /translations · /speech", body: zh ? "转写/翻译上传 file；speech 返回音频二进制。请根据响应 Content-Type 保存文件。" : "Transcription/translation upload file; speech returns audio bytes. Save the response using its Content-Type." },
                    { icon: Video, color: "text-amber-500", title: zh ? "视频异步任务" : "Asynchronous video jobs", endpoint: "POST /v1/videos · GET /v1/videos/{id} · GET /content", body: zh ? "创建后获得平台任务 ID，查询到 completed 后再下载内容。视频秒数应在请求中明确填写。" : "Create a job, poll the platform job ID until completed, then download content. Provide video seconds explicitly." },
                    { icon: Network, color: "text-emerald-500", title: zh ? "Gemini 原生能力" : "Native Gemini capabilities", endpoint: "Imagen · GenerateContent · Veo", body: zh ? "Gemini 使用原生协议；具体音色、图片和视频参数取决于渠道配置的模型能力。" : "Gemini uses native protocols. Voice, image, and video parameters depend on the configured model capability." },
                  ].map((item) => { const Icon = item.icon; return <div key={item.title} className="rounded-xl border border-slate-200 p-4 dark:border-slate-800"><div className="flex items-center gap-2"><Icon className={cn("h-4 w-4", item.color)} /><h3 className="text-sm font-bold text-slate-900 dark:text-white">{item.title}</h3></div><code className="mt-3 block break-words text-[10px] leading-5 text-indigo-700 dark:text-indigo-300">{item.endpoint}</code><p className="mt-2 text-xs leading-5 text-slate-500 dark:text-slate-400">{item.body}</p></div>; })}
                </div>
                <div className="rounded-xl border border-cyan-500/20 bg-cyan-50 p-4 text-xs leading-5 text-cyan-900 dark:bg-cyan-500/10 dark:text-cyan-100"><span className="font-bold">{zh ? "注意：" : "Note: "}</span>{zh ? "OpenAI、Grok、Gemini 的可用媒体模型不代表所有账号都自动开通。若模型没有被渠道映射，GET /v1/models 不会显示，也不能通过接口调用。" : "The existence of an OpenAI, Grok, or Gemini media model does not mean every account has access. An unmapped model will not appear in GET /v1/models and cannot be called."}</div>
              </CardContent>
            </Card>
          </section>

          <section id="security" className="scroll-mt-24">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
              <CardHeader><CardTitle className="flex items-center gap-2 text-lg"><ShieldCheck className="h-5 w-5 text-emerald-500" />{zh ? "安全使用建议" : "Security guidance"}</CardTitle><CardDescription>{zh ? "把令牌当作生产凭据管理，不要把它当作普通配置项。" : "Treat gateway tokens as production credentials, not ordinary configuration values."}</CardDescription></CardHeader>
              <CardContent className="grid gap-3 sm:grid-cols-2">
                {[zh ? "完整令牌只在创建成功时显示一次，建议立即写入服务端密钥管理系统。" : "The full token is shown once. Store it immediately in a server-side secret manager.", zh ? "为不同应用创建不同令牌，出现泄露时只撤销受影响的令牌。" : "Use a separate token per application so a leak can be isolated and revoked.", zh ? "启用 IP 或域名白名单后，请确认请求来源经过网关能正确识别。" : "After enabling an IP or domain allowlist, verify the source seen by the gateway is correct.", zh ? "不要在浏览器前端、移动端安装包或公开代码中嵌入长期 API 令牌。" : "Do not embed long-lived API tokens in browser code, mobile packages, or public repositories."].map((item) => <div key={item} className="flex gap-2 rounded-xl border border-slate-200 p-4 text-xs leading-5 text-slate-600 dark:border-slate-800 dark:text-slate-400"><LockKeyhole className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />{item}</div>)}
              </CardContent>
            </Card>
          </section>

          <section id="errors" className="scroll-mt-24">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
              <CardHeader><CardTitle className="flex items-center gap-2 text-lg"><AlertTriangle className="h-5 w-5 text-amber-500" />{zh ? "常见错误排查" : "Troubleshooting"}</CardTitle><CardDescription>{zh ? "先看 HTTP 状态码，再按错误码定位配置层级。" : "Start with the HTTP status, then use the error code to locate the configuration layer."}</CardDescription></CardHeader>
              <CardContent className="overflow-x-auto"><table className="w-full min-w-[640px] text-left text-xs"><thead className="bg-slate-50 text-slate-500 dark:bg-slate-950/50 dark:text-slate-400"><tr><th className="px-4 py-3 font-semibold">HTTP</th><th className="px-4 py-3 font-semibold">{zh ? "错误" : "Error"}</th><th className="px-4 py-3 font-semibold">{zh ? "处理方式" : "Action"}</th></tr></thead><tbody className="divide-y divide-slate-200 dark:divide-slate-800">{[["401", "AUTH_REQUIRED / AUTH_INVALID", zh ? "检查 Authorization Bearer 令牌、是否已撤销或是否已过期。" : "Check the Bearer token and whether it was revoked or expired."], ["402", "INSUFFICIENT_BALANCE / TOKEN_SPEND_LIMIT_REACHED / PRICE_NOT_CONFIGURED", zh ? "检查租户预付余额、令牌消耗额度和对应模型价格组件。" : "Check the tenant balance, token spend limit, and model price components."], ["403", "MODEL_NOT_ALLOWED / TOKEN_NETWORK_NOT_ALLOWED", zh ? "检查令牌模型权限、所属分组和 IP/域名白名单。" : "Check token model access, group binding, and IP/domain allowlist."], ["404", "MODEL_NOT_AVAILABLE", zh ? "在模型广场或 GET /v1/models 确认模型是否有启用渠道映射。" : "Verify the model has an enabled channel mapping in the Model Plaza or GET /v1/models."], ["429", "GROUP_RATE_LIMITED", zh ? "降低请求频率，或联系管理员调整分组 RPM。" : "Reduce request rate or ask the administrator to adjust group RPM."], ["503", "UPSTREAM_USAGE_UNAVAILABLE / UPSTREAM_REQUEST_FAILED", zh ? "检查渠道状态、上游账户权限、模型能力和服务商响应。" : "Check channel status, upstream access, model capability, and provider response."]].map((row) => <tr key={row[0]} className="text-slate-600 dark:text-slate-300"><td className="px-4 py-3 font-mono font-bold text-slate-900 dark:text-white">{row[0]}</td><td className="px-4 py-3 font-mono text-[10px] text-rose-700 dark:text-rose-300">{row[1]}</td><td className="px-4 py-3 leading-5">{row[2]}</td></tr>)}</tbody></table></CardContent>
            </Card>
          </section>

          <div className="flex items-center gap-2 rounded-2xl border border-indigo-500/20 bg-indigo-500/[0.06] p-4 text-xs text-slate-700 dark:text-slate-300"><ShieldCheck className="h-4 w-4 shrink-0 text-indigo-500" />{zh ? "平台会记录请求端点、模型、分组、用量、费用、延迟和状态，管理员可以在使用记录与财务页面核对每次调用。" : "The platform records endpoint, model, group, usage, cost, latency, and status. Administrators can reconcile every call in Usage and Finance."}</div>
        </main>
      </div>
    </div>
  );
}
