import React from "react";
import { Cpu, Lock, ShieldCheck, Zap } from "lucide-react";
import { Language, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";

interface FooterProps {
  language: Language;
  routeTo: (target: string) => void;
  siteName?: string;
  siteLogoURL?: string;
}

export function Footer({ language, routeTo, siteName, siteLogoURL }: FooterProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const brandName = siteName?.trim() || t("brandName");

  return (
    <footer className="mt-auto border-t border-slate-200/80 dark:border-slate-800/80 bg-slate-50/90 dark:bg-slate-950/95 text-slate-600 dark:text-slate-400">
      {/* Top Banner / Trust Bar */}
      <div className="border-b border-slate-200/60 dark:border-slate-800/60 py-8 px-4 sm:px-6 lg:px-8 max-w-[1720px] mx-auto">
        <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border border-indigo-500/20">
              <Zap className="h-5 w-5" />
            </div>
            <div>
              <div className="text-sm font-semibold text-slate-900 dark:text-slate-200">{language === "zh" ? "可观测路由调度" : "Observable Routing"}</div>
              <div className="text-xs text-slate-500 dark:text-slate-400">{language === "zh" ? "按实际渠道状态调度请求" : "Dispatch by live channel state"}</div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
              <ShieldCheck className="h-5 w-5" />
            </div>
            <div>
              <div className="text-sm font-semibold text-slate-900 dark:text-slate-200">{language === "zh" ? "故障兜底策略" : "Failover Policy"}</div>
              <div className="text-xs text-slate-500 dark:text-slate-400">{language === "zh" ? "按优先级和权重切换渠道" : "Switch by priority and weight"}</div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 border border-cyan-500/20">
              <Lock className="h-5 w-5" />
            </div>
            <div>
              <div className="text-sm font-semibold text-slate-900 dark:text-slate-200">{language === "zh" ? "金融级双重记账" : "Double-Entry Ledger"}</div>
              <div className="text-xs text-slate-500 dark:text-slate-400">{language === "zh" ? "按请求写入不可变交易记录" : "Immutable request ledger records"}</div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-purple-500/10 text-purple-600 dark:text-purple-400 border border-purple-500/20">
              <Cpu className="h-5 w-5" />
            </div>
            <div>
              <div className="text-sm font-semibold text-slate-900 dark:text-slate-200">{language === "zh" ? "模型目录" : "Model Directory"}</div>
              <div className="text-xs text-slate-500 dark:text-slate-400">{language === "zh" ? "展示已配置的真实模型映射" : "Live configured model mappings"}</div>
            </div>
          </div>
        </div>
      </div>

      {/* Main Footer Links */}
      <div className="mx-auto max-w-[1720px] px-4 py-12 sm:px-6 lg:px-8">
        <div className="grid grid-cols-1 gap-8 md:grid-cols-5">
          {/* Brand Info */}
          <div className="md:col-span-2 space-y-4">
            <div className="flex items-center gap-3">
              <div className="relative flex h-9 w-9 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br from-indigo-500 to-cyan-500 text-white font-bold shadow-md shadow-indigo-500/20">
                AT
                {siteLogoURL ? <img src={siteLogoURL} alt="" className="absolute inset-0 h-full w-full bg-white object-contain p-1" onError={(event) => { event.currentTarget.style.display = "none"; }} /> : null}
              </div>
              <div className="text-base font-bold text-slate-900 dark:text-white tracking-tight">{brandName}</div>
            </div>
            <p className="text-sm text-slate-600 dark:text-slate-400 max-w-sm leading-relaxed">
              {language === "zh"
                ? "专为高可用生产环境设计的商业级大模型中转网关。为企业与开发者提供统一接入、智能负载、透明记账与多租户安全管控。"
                : "Enterprise-grade AI model routing gateway designed for mission-critical workloads. Unifying API access, load balancing, ledger billing, and multi-tenant security."}
            </p>
            <div className="flex items-center gap-2 pt-2 text-xs text-emerald-600 dark:text-emerald-400">
              <span className="flex h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
              <span>{language === "zh" ? "服务能力与渠道状态以管理后台实时数据为准" : "Service and channel status follow live admin data"}</span>
            </div>
          </div>

          {/* Product Columns */}
          <div>
            <div className="text-xs font-bold uppercase tracking-wider text-slate-900 dark:text-slate-200 mb-3">
              {language === "zh" ? "产品与核心特性" : "Product"}
            </div>
            <ul className="space-y-2 text-sm">
              <li>
                <a href="#home" onClick={() => routeTo("#home")} className="hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors">
                  {language === "zh" ? "统一模型路由" : "Unified Model Routing"}
                </a>
              </li>
              <li>
                <a href="#home" onClick={() => routeTo("#home")} className="hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors">
                  {language === "zh" ? "多租户组织隔离" : "Tenant Isolation"}
                </a>
              </li>
              <li>
                <a href="#home" onClick={() => routeTo("#home")} className="hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors">
                  {language === "zh" ? "价格发布与版本化" : "Price Versioning"}
                </a>
              </li>
              <li>
                <a href="#home" onClick={() => routeTo("#home")} className="hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors">
                  {language === "zh" ? "双重记账流水" : "Ledger Accounting"}
                </a>
              </li>
            </ul>
          </div>

          <div>
            <div className="text-xs font-bold uppercase tracking-wider text-slate-900 dark:text-slate-200 mb-3">
              {language === "zh" ? "开发者与生态" : "Developers"}
            </div>
            <ul className="space-y-2 text-sm">
              <li>
                <a href="#home" onClick={() => routeTo("#home")} className="hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors">
                  {language === "zh" ? "OpenAI 兼容协议" : "OpenAI Compatibility"}
                </a>
              </li>
              <li>
                <a href="#home" onClick={() => routeTo("#home")} className="hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors">
                  {language === "zh" ? "Anthropic Claude 协议" : "Anthropic Claude API"}
                </a>
              </li>
              <li>
                <a href="#home" onClick={() => routeTo("#home")} className="hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors">
                  {language === "zh" ? "Grok / Gemini 接入" : "Grok & Gemini Channels"}
                </a>
              </li>
              <li>
                <a href="#login" onClick={() => routeTo("#login")} className="hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors">
                  {language === "zh" ? "API 密钥发放与调试" : "Key Issuance"}
                </a>
              </li>
            </ul>
          </div>

          <div>
            <div className="text-xs font-bold uppercase tracking-wider text-slate-900 dark:text-slate-200 mb-3">
              {language === "zh" ? "安全与合规" : "Security"}
            </div>
            <ul className="space-y-2 text-sm">
              <li>
                <span className="text-slate-600 dark:text-slate-400">{language === "zh" ? "TOTP/MFA 身份认证" : "TOTP/MFA authentication"}</span>
              </li>
              <li>
                <span className="text-slate-600 dark:text-slate-400">{language === "zh" ? "AES-256 密钥落盘加密" : "AES-256 Key Encryption"}</span>
              </li>
              <li>
                <span className="text-slate-600 dark:text-slate-400">{language === "zh" ? "全量审计追踪日志" : "Full Audit Logging"}</span>
              </li>
              <li>
                <span className="text-slate-600 dark:text-slate-400">{language === "zh" ? "IP 鉴权与防刷限流" : "Rate Limiting & Firewalls"}</span>
              </li>
            </ul>
          </div>
        </div>

        {/* Bottom copyright */}
        <div className="mt-10 border-t border-slate-200/80 dark:border-slate-800/80 pt-6 flex flex-col sm:flex-row items-center justify-between gap-4 text-xs text-slate-500">
          <div>
            © {new Date().getFullYear()} AI Token Gateway Inc. {language === "zh" ? "保留所有权利。" : "All rights reserved."}
          </div>
          <span className="text-indigo-600 dark:text-indigo-400">Enterprise API Gateway</span>
        </div>
      </div>
    </footer>
  );
}
