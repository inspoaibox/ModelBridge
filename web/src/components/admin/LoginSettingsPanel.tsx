import React, { useEffect, useState } from "react";
import { GitBranch, Globe2, KeyRound, LogIn, Save } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Language, LoginMessage, LoginProviderSettings, LoginSettings, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";

interface Props {
  language: Language;
  settings: LoginSettings;
  busy: boolean;
  message: LoginMessage;
  save: (settings: LoginSettings) => Promise<void>;
  canUpdate: boolean;
}

const providerMeta: Record<string, { label: TranslationKey; icon: typeof GitBranch }> = {
  google: { label: "loginProviderGoogle", icon: Globe2 },
  github: { label: "loginProviderGitHub", icon: GitBranch },
  linuxdo: { label: "loginProviderLinuxDo", icon: LogIn },
};

export function LoginSettingsPanel({ language, settings, busy, message, save, canUpdate }: Props) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [draft, setDraft] = useState<LoginProviderSettings[]>(settings.providers);
  useEffect(() => setDraft(settings.providers), [settings.providers]);

  function update(provider: string, patch: Partial<LoginProviderSettings>) {
    setDraft((current) => current.map((item) => item.provider === provider ? { ...item, ...patch } : item));
  }

  return <div className="space-y-5">
    <div className="flex items-start gap-3 rounded-2xl border border-indigo-500/20 bg-indigo-500/5 p-5 dark:bg-indigo-500/10"><KeyRound className="mt-0.5 h-5 w-5 shrink-0 text-indigo-600 dark:text-indigo-300" /><div><h2 className="text-xl font-bold text-slate-950 dark:text-white">{t("loginSettingsTitle")}</h2><p className="mt-1 text-sm leading-6 text-slate-600 dark:text-slate-300">{t("loginSettingsDescription")}</p></div></div>
    {message.text ? <div className={`rounded-xl border p-3 text-sm ${message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : message.kind === "pending" ? "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300" : "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300"}`}>{message.text}</div> : null}
    <div className="grid gap-5 xl:grid-cols-3">{draft.map((item) => { const meta = providerMeta[item.provider] || providerMeta.google; const Icon = meta.icon; return <Card key={item.provider} className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader className="pb-4"><div className="flex items-center justify-between gap-3"><CardTitle className="flex items-center gap-2 text-lg"><Icon className="h-5 w-5 text-indigo-600 dark:text-indigo-300" />{t(meta.label)}</CardTitle><Switch checked={item.enabled} onCheckedChange={(checked) => update(item.provider, { enabled: checked })} disabled={busy || !canUpdate} /></div><CardDescription>{item.enabled ? t("loginProviderEnabled") : t("loginProviderDisabled")}</CardDescription></CardHeader><CardContent className="space-y-4"><Field label={t("loginClientID")}><Input value={item.client_id} onChange={(event) => update(item.provider, { client_id: event.target.value })} disabled={busy || !canUpdate} placeholder="client-id" /></Field><Field label={t("loginClientSecret")}><Input type="password" value={item.client_secret || ""} onChange={(event) => update(item.provider, { client_secret: event.target.value, clear_client_secret: false })} disabled={busy || !canUpdate} placeholder={item.client_secret_configured ? t("loginClientSecretKeep") : "client-secret"} /></Field>{item.client_secret_configured ? <label className="flex items-center gap-2 text-xs text-slate-500"><input type="checkbox" checked={Boolean(item.clear_client_secret)} onChange={(event) => update(item.provider, { clear_client_secret: event.target.checked, client_secret: "" })} disabled={busy || !canUpdate} />{t("loginClearClientSecret")}</label> : null}<Field label={t("loginScopes")}><Input value={item.scopes} onChange={(event) => update(item.provider, { scopes: event.target.value })} disabled={busy || !canUpdate} /></Field><details className="rounded-lg border border-slate-200 p-3 text-xs dark:border-slate-800"><summary className="cursor-pointer font-semibold text-slate-700 dark:text-slate-200">{t("loginAdvancedSettings")}</summary><div className="mt-3 space-y-3"><Field label={t("loginAuthorizationURL")}><Input value={item.authorization_url} onChange={(event) => update(item.provider, { authorization_url: event.target.value })} disabled={busy || !canUpdate} /></Field><Field label={t("loginTokenURL")}><Input value={item.token_url} onChange={(event) => update(item.provider, { token_url: event.target.value })} disabled={busy || !canUpdate} /></Field><Field label={t("loginUserInfoURL")}><Input value={item.userinfo_url} onChange={(event) => update(item.provider, { userinfo_url: event.target.value })} disabled={busy || !canUpdate} /></Field></div></details><div className="flex items-center justify-between border-t border-slate-200 pt-3 text-xs dark:border-slate-800"><span className="text-slate-500">{t("loginSecretStatus")}</span><Badge variant={item.client_secret_configured ? "success" : "muted"}>{item.client_secret_configured ? t("loginConfigured") : t("loginNotConfigured")}</Badge></div></CardContent></Card>; })}</div>
    <div className="flex justify-end"><Button type="button" onClick={() => void save({ providers: draft })} disabled={busy || !canUpdate} className="gap-2"><Save className="h-4 w-4" />{busy ? t("loginSettingsSaving") : t("loginSettingsSave")}</Button></div>
  </div>;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div>; }
