import { useState } from "react";
import { ArrowRight, Building2, CheckCircle2, KeyRound, Lock, LogIn, Mail, UserRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Language, LoginMessage, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface RegisterViewProps {
  language: Language;
  routeTo: (target: string) => void;
  onRegistered: (email: string, tenantID: string) => void;
}

export function RegisterView({ language, routeTo, onRegistered }: RegisterViewProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [tenantName, setTenantName] = useState("");
  const [tenantSlug, setTenantSlug] = useState("");
  const [projectName, setProjectName] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<LoginMessage>({ kind: "", text: "" });

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (password.length < 12 || password !== confirmPassword || !tenantSlug.trim()) {
      setMessage({ kind: "error", text: t("registerValidation") });
      return;
    }
    setBusy(true);
    setMessage({ kind: "pending", text: t("registerSubmitting") });
    try {
      const response = await fetch("/console/v1/auth/register", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({
          display_name: displayName.trim(),
          email: email.trim(),
          password,
          tenant_name: tenantName.trim(),
          tenant_slug: tenantSlug.trim().toLowerCase(),
          project_name: projectName.trim(),
        }),
      });
      const result = (await response.json().catch(() => ({}))) as { tenant_id?: string; error?: string };
      if (!response.ok || !result.tenant_id) {
        if (response.status === 409 && result.error === "EMAIL_ALREADY_REGISTERED") throw new Error(t("registerEmailExists"));
        if (response.status === 409 && result.error === "TENANT_SLUG_ALREADY_REGISTERED") throw new Error(t("registerSlugExists"));
        throw new Error(t("registerUnavailable"));
      }
      onRegistered(email.trim(), result.tenant_id);
    } catch (error) {
      setMessage({ kind: "error", text: error instanceof Error ? error.message : t("registerUnavailable") });
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-6xl py-8 sm:py-12">
      <div className="grid gap-8 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)] lg:items-start">
        <div className="space-y-6 lg:pt-8">
          <div className="inline-flex items-center gap-2 rounded-full border border-indigo-500/30 bg-indigo-50 px-3.5 py-1 text-xs font-semibold text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300"><Building2 className="h-4 w-4" /> {t("registerEyebrow")}</div>
          <div className="space-y-3"><h1 className="text-3xl font-extrabold tracking-tight text-slate-950 dark:text-white sm:text-4xl">{t("registerTitle")}</h1><p className="max-w-md text-sm leading-6 text-slate-600 dark:text-slate-400">{t("registerDescription")}</p></div>
          <div className="space-y-3">{[t("registerBenefitIdentity"), t("registerBenefitProject"), t("registerBenefitToken")].map((item) => <div key={item} className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white/70 px-4 py-3 text-sm text-slate-700 shadow-sm dark:border-slate-800 dark:bg-slate-900/60 dark:text-slate-300"><CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" /> {item}</div>)}</div>
          <Button variant="ghost" onClick={() => routeTo("#login")} className="gap-2 px-0 text-indigo-600 dark:text-indigo-400"><LogIn className="h-4 w-4" />{t("registerAction")}</Button>
        </div>

        <Card className="border-slate-200/80 shadow-xl dark:border-slate-800 dark:bg-slate-900/70">
          <CardHeader className="border-b border-slate-200/80 dark:border-slate-800"><CardTitle className="text-xl text-slate-950 dark:text-white">{t("registerFormTitle")}</CardTitle><CardDescription>{t("registerFormHint")}</CardDescription></CardHeader>
          <CardContent className="pt-6"><form onSubmit={(event) => void submit(event)} className="space-y-5">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="register-display-name">{t("registerDisplayName")}</Label><div className="relative"><UserRound className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" /><Input id="register-display-name" value={displayName} onChange={(event) => setDisplayName(event.target.value)} className="pl-9" required maxLength={100} /></div></div>
              <div className="space-y-2"><Label htmlFor="register-email">{t("fieldEmail")}</Label><div className="relative"><Mail className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" /><Input id="register-email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} className="pl-9" required /></div></div>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="register-password">{t("registerPassword")}</Label><div className="relative"><Lock className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" /><Input id="register-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} className="pl-9" required minLength={12} /></div></div>
              <div className="space-y-2"><Label htmlFor="register-confirm-password">{t("registerPasswordConfirm")}</Label><div className="relative"><Lock className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" /><Input id="register-confirm-password" type="password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} className="pl-9" required minLength={12} /></div></div>
            </div>
            <div className="border-t border-slate-200 pt-5 dark:border-slate-800"><div className="mb-4 flex items-center gap-2 text-sm font-semibold text-slate-800 dark:text-slate-200"><Building2 className="h-4 w-4 text-indigo-600 dark:text-indigo-400" />{t("registerOrganizationTitle")}</div>
              <div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="register-tenant-name">{t("registerTenantName")}</Label><Input id="register-tenant-name" value={tenantName} onChange={(event) => setTenantName(event.target.value)} required maxLength={120} /></div><div className="space-y-2"><Label htmlFor="register-tenant-slug">{t("registerTenantSlug")}</Label><Input id="register-tenant-slug" value={tenantSlug} onChange={(event) => setTenantSlug(event.target.value.replace(/[^a-zA-Z0-9-]/g, "").toLowerCase())} placeholder="acme-team" required maxLength={64} /></div></div>
              <div className="mt-4 space-y-2"><Label htmlFor="register-project-name">{t("registerProjectName")}</Label><Input id="register-project-name" value={projectName} onChange={(event) => setProjectName(event.target.value)} placeholder={t("registerProjectPlaceholder")} maxLength={100} /></div>
            </div>
            {message.text ? <div className={cn("flex items-center gap-2 rounded-xl border p-3 text-xs", message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : "border-indigo-500/30 bg-indigo-50 text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300")}><KeyRound className="h-4 w-4 shrink-0" />{message.text}</div> : null}
            <div className="flex flex-col-reverse gap-3 pt-2 sm:flex-row sm:justify-end"><Button type="button" variant="outline" onClick={() => routeTo("#home")} disabled={busy}>{t("backHome")}</Button><Button type="submit" disabled={busy} className="gap-2">{busy ? <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" /> : <ArrowRight className="h-4 w-4" />}{busy ? t("registerSubmitting") : t("registerSubmit")}</Button></div>
          </form></CardContent>
        </Card>
      </div>
    </div>
  );
}
