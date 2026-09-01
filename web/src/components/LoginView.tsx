import React from "react";
import {
  ArrowRight,
  Building,
  CheckCircle2,
  KeyRound,
  Lock,
  LogIn,
  LogOut,
  Mail,
  ShieldCheck,
  Sparkles,
  Zap,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Audience, Language, LoginMessage, Principal, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface LoginViewProps {
  language: Language;
  loginAudience: Audience;
  email: string;
  setEmail: (v: string) => void;
  password: string;
  setPassword: (v: string) => void;
  tenantId: string;
  setTenantId: (v: string) => void;
  mfaCode: string;
  setMfaCode: (v: string) => void;
  handleLogin: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  loginMessage: LoginMessage;
  formBusy: boolean;
  signedIn: boolean;
  principal: Principal | null;
  handleSignOut: () => void;
  routeTo: (target: string) => void;
}

export function LoginView({
  language,
  loginAudience,
  email,
  setEmail,
  password,
  setPassword,
  tenantId,
  setTenantId,
  mfaCode,
  setMfaCode,
  handleLogin,
  loginMessage,
  formBusy,
  signedIn,
  principal,
  handleSignOut,
  routeTo,
}: LoginViewProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;

  return (
    <div className="py-6 sm:py-12 max-w-6xl mx-auto">
      <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)] lg:items-center">
        {/* Left Side: Brand Visual Card */}
        <div className="space-y-6 lg:pr-4">
          <div className="inline-flex items-center gap-2 rounded-full border border-indigo-500/30 bg-indigo-50 dark:bg-indigo-500/10 px-3.5 py-1 text-xs font-semibold text-indigo-700 dark:text-indigo-300">
            <ShieldCheck className="h-4 w-4 text-indigo-500 dark:text-indigo-400" />
            <span>{t("authEyebrow")}</span>
          </div>

          <div className="space-y-3">
            <h1 className="text-3xl sm:text-4xl font-extrabold text-slate-900 dark:text-white tracking-tight leading-tight">
              {language === "zh" ? (
                <>
                  登录企业级 <br />
                  <span className="text-gradient-primary">AI Token 统一网关</span>
                </>
              ) : (
                <>
                  Sign In to <br />
                  <span className="text-gradient-primary">AI Token Gateway</span>
                </>
              )}
            </h1>
            <p className="text-sm text-slate-600 dark:text-slate-400 leading-relaxed max-w-md">
              {t("authDescription")}
            </p>
          </div>

          {/* Value Props Pills */}
          <div className="space-y-3 pt-2">
            {[
              {
                title: language === "zh" ? "零信任身份凭据隔离" : "Zero-Trust Identity",
                desc: language === "zh" ? "支持 MFA 双因子认证与敏感凭据密钥库隔离存储" : "MFA protection & encrypted vault storage",
                icon: Lock,
              },
              {
                title: language === "zh" ? "多租户统一计费账单" : "Multi-Tenant Ledger",
                desc: language === "zh" ? "支持项目级配额划分与实时双重记账审计流水" : "Project quota allocation & audit trail",
                icon: Building,
              },
              {
                title: language === "zh" ? "高性能毫秒级模型分发" : "High-Performance Relay",
                desc: language === "zh" ? "统一 OpenAI 协议格式，支持跨模型智能容灾路由" : "Standard API with intelligent failover",
                icon: Zap,
              },
            ].map((prop, idx) => {
              const Icon = prop.icon;
              return (
                <div
                  key={idx}
                  className="flex items-start gap-3.5 rounded-2xl border border-slate-200 dark:border-slate-800/80 bg-white/70 dark:bg-slate-900/60 p-4 transition-all duration-200"
                >
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-indigo-500/10 dark:bg-indigo-500/20 text-indigo-600 dark:text-indigo-400 border border-indigo-500/20">
                    <Icon className="h-4 w-4" />
                  </div>
                  <div className="space-y-0.5">
                    <div className="text-sm font-bold text-slate-900 dark:text-white">{prop.title}</div>
                    <div className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed">{prop.desc}</div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {/* Right Side: Auth Card or Active Session */}
        <div>
          {signedIn ? (
            <Card className="glass-panel text-center p-8 space-y-6">
              <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30">
                <CheckCircle2 className="h-8 w-8" />
              </div>
              <div className="space-y-2">
                <h3 className="text-2xl font-bold text-slate-900 dark:text-white">{t("authSignedInTitle")}</h3>
                <p className="text-xs font-mono text-slate-500 dark:text-slate-400">
                  {principal?.id ? `ID: ${principal.id}` : "Session Active"}
                </p>
                <div className="pt-2">
                  <Badge variant="success">AUTHORIZED SESSION</Badge>
                </div>
              </div>
              <div className="flex flex-col sm:flex-row items-center justify-center gap-3 pt-2">
                <Button size="lg" onClick={() => routeTo(principal?.audience === "console" ? "#console/tokens" : "#admin/dashboard")} className="gap-2 w-full sm:w-auto">
                  <span>{t("enterWorkspace")}</span>
                  <ArrowRight className="h-4 w-4" />
                </Button>
                <Button size="lg" variant="outline" onClick={handleSignOut} className="gap-2 w-full sm:w-auto">
                  <LogOut className="h-4 w-4" />
                  <span>{t("signOut")}</span>
                </Button>
              </div>
            </Card>
          ) : (
            <Card className="glass-panel">
              <CardHeader className="space-y-3 pb-6">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-2xl font-extrabold text-slate-900 dark:text-white">{t("authSignInTitle")}</CardTitle>
                  <Badge variant="secondary" className="text-xs font-mono">SECURE AUTH</Badge>
                </div>
                <CardDescription className="text-slate-500 dark:text-slate-400">{t("authSignInSubtitle")}</CardDescription>

              </CardHeader>

              <CardContent className="space-y-5">
                <form onSubmit={handleLogin} className="space-y-4">
                  {/* Tenant ID for Tenant Mode */}
                  {loginAudience === "console" && (
                    <div className="space-y-2 animate-in fade-in duration-200">
                      <Label htmlFor="tenantId" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                        {t("fieldTenantId")}
                      </Label>
                      <div className="relative">
                        <Building className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" />
                        <Input
                          id="tenantId"
                          placeholder={t("placeholderTenantId")}
                          value={tenantId}
                          onChange={(e) => setTenantId(e.target.value)}
                          className="pl-9"
                          required
                          disabled={formBusy}
                        />
                      </div>
                    </div>
                  )}

                  {/* Email */}
                  <div className="space-y-2">
                    <Label htmlFor="email" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                      {t("fieldEmail")}
                    </Label>
                    <div className="relative">
                      <Mail className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" />
                      <Input
                        id="email"
                        type="email"
                        value={email}
                        onChange={(e) => setEmail(e.target.value)}
                        className="pl-9"
                        required
                        disabled={formBusy}
                      />
                    </div>
                  </div>

                  {/* Password */}
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <Label htmlFor="password" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                        {t("fieldPassword")}
                      </Label>
                    </div>
                    <div className="relative">
                      <Lock className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" />
                      <Input
                        id="password"
                        type="password"
                        placeholder="••••••••••••"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        className="pl-9"
                        required
                        disabled={formBusy}
                      />
                    </div>
                  </div>

                  {/* MFA Code */}
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <Label htmlFor="mfaCode" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                        {t("fieldMfaCode")}
                      </Label>
                      <span className="text-[11px] text-slate-500 dark:text-slate-400">
                        {loginAudience === "admin" ? t("mfaHintAdmin") : t("mfaHintTenant")}
                      </span>
                    </div>
                    <div className="relative">
                      <KeyRound className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" />
                      <Input
                        id="mfaCode"
                        type="text"
                        placeholder={t("placeholderMfaCode")}
                        value={mfaCode}
                        onChange={(e) => setMfaCode(e.target.value)}
                        className="pl-9 font-mono tracking-widest"
                        maxLength={6}
                        disabled={formBusy}
                      />
                    </div>
                  </div>

                  {/* Status Banner */}
                  {loginMessage.text && (
                    <div
                      className={cn("rounded-xl border p-3 text-xs flex items-center gap-2", {
                        "border-emerald-500/30 bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300": loginMessage.kind === "success",
                        "border-amber-500/30 bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300": loginMessage.kind === "pending",
                        "border-rose-500/30 bg-rose-50 dark:bg-rose-500/10 text-rose-700 dark:text-rose-300": loginMessage.kind === "error",
                      })}
                    >
                      <Sparkles className="h-4 w-4 shrink-0" />
                      <span>{loginMessage.text}</span>
                    </div>
                  )}

                  {/* Submit Button */}
                  <Button type="submit" size="lg" className="w-full gap-2 shadow-lg shadow-indigo-500/30" disabled={formBusy}>
                    {formBusy ? (
                      <>
                        <span className="h-4 w-4 rounded-full border-2 border-white/30 border-t-white animate-spin" />
                        <span>{t("buttonSigningIn")}</span>
                      </>
                    ) : (
                      <>
                        <LogIn className="h-4 w-4" />
                        <span>{t("buttonSignIn")}</span>
                      </>
                    )}
                  </Button>
                  {loginAudience === "console" ? (
                    <Button type="button" variant="ghost" size="sm" className="w-full" onClick={() => routeTo("#reset")}>
                      {language === "zh" ? "忘记密码？" : "Forgot password?"}
                    </Button>
                  ) : null}
                </form>

                {/* Footer Switch */}
                {loginAudience === "console" ? (
                  <div className="text-center pt-2">
                    <span className="text-xs text-slate-500 dark:text-slate-400">
                      {t("noAccountText")}{" "}
                      <button
                        type="button"
                        onClick={() => routeTo("#register")}
                        className="font-bold text-indigo-600 dark:text-indigo-400 hover:underline cursor-pointer"
                      >
                        {t("registerLink")}
                      </button>
                    </span>
                  </div>
                ) : null}
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
