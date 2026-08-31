import React, { useState } from "react";
import {
  Check,
  Copy,
  KeyRound,
  Mail,
  Save,
  ShieldCheck,
  ShieldOff,
  UserRound,
  Settings2,
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  ConsoleProfile,
  EmailFormState,
  Language,
  LoginMessage,
  MFAEnrollment,
  MFAStatus,
  PasswordFormState,
  ProfileFormState,
  SecuritySettings,
  SiteSettings,
  TranslationKey,
} from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface AdminSettingsPanelProps {
  language: Language;
  profile: ConsoleProfile | null;
  profileForm: ProfileFormState;
  setProfileForm: React.Dispatch<React.SetStateAction<ProfileFormState>>;
  emailForm: EmailFormState;
  setEmailForm: React.Dispatch<React.SetStateAction<EmailFormState>>;
  passwordForm: PasswordFormState;
  setPasswordForm: React.Dispatch<React.SetStateAction<PasswordFormState>>;
  profileBusy: boolean;
  profileMessage: LoginMessage;
  saveProfile: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  saveEmail: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  savePassword: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  mfaStatus: MFAStatus;
  mfaEnrollment: MFAEnrollment | null;
  mfaCode: string;
  setMfaCode: (value: string) => void;
  mfaBusy: boolean;
  beginMFA: () => Promise<void>;
  confirmMFA: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  cancelMFA: () => void;
  disableMFA: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  securitySettings: SecuritySettings;
  securityMessage: LoginMessage;
  securityBusy: boolean;
  persistSecurity: (nextEnabled: boolean) => Promise<void>;
  siteForm: SiteSettings;
  setSiteForm: React.Dispatch<React.SetStateAction<SiteSettings>>;
  siteBusy: boolean;
  siteMessage: LoginMessage;
  saveSiteSettings: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
}

export function AdminSettingsPanel({
  language,
  profile,
  profileForm,
  setProfileForm,
  emailForm,
  setEmailForm,
  passwordForm,
  setPasswordForm,
  profileBusy,
  profileMessage,
  saveProfile,
  saveEmail,
  savePassword,
  mfaStatus,
  mfaEnrollment,
  mfaCode,
  setMfaCode,
  mfaBusy,
  beginMFA,
  confirmMFA,
  cancelMFA,
  disableMFA,
  securitySettings,
  securityMessage,
  securityBusy,
  persistSecurity,
  siteForm,
  setSiteForm,
  siteBusy,
  siteMessage,
  saveSiteSettings,
}: AdminSettingsPanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [tab, setTab] = useState<"admin" | "base">("admin");
  const [copied, setCopied] = useState(false);
  const accountBusy = profileBusy || mfaBusy;

  async function copySecret() {
    if (!mfaEnrollment?.secret) return;
    try {
      await navigator.clipboard.writeText(mfaEnrollment.secret);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2 border-b border-slate-200/80 pb-3 dark:border-slate-800/80">
        <button type="button" onClick={() => setTab("admin")} className={cn("inline-flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold transition-colors", tab === "admin" ? "bg-indigo-600 text-white shadow-md shadow-indigo-500/20" : "text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-900")}><UserRound className="h-4 w-4" />{t("systemSettingsAdminTab")}</button>
        <button type="button" onClick={() => setTab("base")} className={cn("inline-flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold transition-colors", tab === "base" ? "bg-indigo-600 text-white shadow-md shadow-indigo-500/20" : "text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-900")}><Settings2 className="h-4 w-4" />{t("systemSettingsBaseTab")}</button>
      </div>

      {tab === "admin" ? (
        <div className="space-y-6">
          <Card className="overflow-hidden border-indigo-500/20 bg-gradient-to-br from-indigo-500/10 via-cyan-500/5 to-white shadow-sm dark:from-indigo-500/15 dark:via-cyan-500/5 dark:to-slate-900/70">
            <CardContent className="flex flex-col gap-5 p-5 sm:flex-row sm:items-center sm:justify-between sm:p-6">
              <div className="flex min-w-0 items-center gap-4"><div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-600 to-cyan-500 text-lg font-bold text-white shadow-lg shadow-indigo-500/20">{(profile?.display_name || profile?.email || "A").slice(0, 2).toUpperCase()}</div><div className="min-w-0"><div className="text-xs font-bold uppercase tracking-[0.16em] text-indigo-600 dark:text-indigo-300">{t("adminCenterTitle")}</div><div className="mt-1 truncate text-xl font-extrabold text-slate-950 dark:text-white">{profile?.display_name || "-"}</div><div className="mt-1 flex items-center gap-2 text-sm text-slate-600 dark:text-slate-300"><Mail className="h-3.5 w-3.5" />{profile?.email || "-"}</div></div></div>
              <Badge variant="success" className="self-start sm:self-center">{t("adminCenterRole")}: {profile?.roles?.join(", ") || "-"}</Badge>
            </CardContent>
          </Card>

          {profileMessage.text ? <Notice message={profileMessage} /> : null}

          <div className="grid gap-6 xl:grid-cols-2">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><UserRound className="h-5 w-5 text-indigo-600" />{t("adminProfileTitle")}</CardTitle><CardDescription>{t("adminProfileDescription")}</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={saveProfile}><div className="space-y-2"><Label htmlFor="admin-profile-display-name">{t("profileDisplayName")}</Label><Input id="admin-profile-display-name" value={profileForm.display_name} onChange={(event) => setProfileForm({ display_name: event.target.value })} autoComplete="name" maxLength={100} disabled={accountBusy} required /></div><InfoField label={t("adminCenterAccountID")} value={profile?.id || "-"} mono /><Button type="submit" disabled={accountBusy} className="gap-2"><Save className="h-4 w-4" />{t("profileSave")}</Button></form></CardContent></Card>

            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Mail className="h-5 w-5 text-cyan-600" />{t("profileEmailTitle")}</CardTitle><CardDescription>{t("adminEmailDescription")}</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={saveEmail}><div className="space-y-2"><Label htmlFor="admin-profile-email">{t("profileNewEmail")}</Label><Input id="admin-profile-email" type="email" value={emailForm.email} onChange={(event) => setEmailForm((current) => ({ ...current, email: event.target.value }))} autoComplete="email" disabled={accountBusy} required /></div><div className="space-y-2"><Label htmlFor="admin-profile-email-password">{t("profileCurrentPassword")}</Label><Input id="admin-profile-email-password" type="password" value={emailForm.current_password} onChange={(event) => setEmailForm((current) => ({ ...current, current_password: event.target.value }))} autoComplete="current-password" disabled={accountBusy} required /></div><Button type="submit" variant="secondary" disabled={accountBusy} className="gap-2"><Mail className="h-4 w-4" />{t("profileEmailSave")}</Button></form></CardContent></Card>

            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><KeyRound className="h-5 w-5 text-amber-600" />{t("profilePasswordTitle")}</CardTitle><CardDescription>{t("adminPasswordDescription")}</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={savePassword}><div className="space-y-2"><Label htmlFor="admin-current-password">{t("profileCurrentPassword")}</Label><Input id="admin-current-password" type="password" value={passwordForm.current_password} onChange={(event) => setPasswordForm((current) => ({ ...current, current_password: event.target.value }))} autoComplete="current-password" disabled={accountBusy} required /></div><div className="space-y-2"><Label htmlFor="admin-new-password">{t("profileNewPassword")}</Label><Input id="admin-new-password" type="password" value={passwordForm.new_password} onChange={(event) => setPasswordForm((current) => ({ ...current, new_password: event.target.value }))} autoComplete="new-password" minLength={12} disabled={accountBusy} required /></div><div className="space-y-2"><Label htmlFor="admin-confirm-password">{t("profileConfirmPassword")}</Label><Input id="admin-confirm-password" type="password" value={passwordForm.confirm_password} onChange={(event) => setPasswordForm((current) => ({ ...current, confirm_password: event.target.value }))} autoComplete="new-password" minLength={12} disabled={accountBusy} required /></div><Button type="submit" variant="secondary" disabled={accountBusy} className="gap-2"><KeyRound className="h-4 w-4" />{t("profilePasswordSave")}</Button></form></CardContent></Card>

            <Card className="border-emerald-500/20 shadow-sm dark:border-emerald-400/20 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><ShieldCheck className="h-5 w-5 text-emerald-600" />{t("adminMFATitle")}</CardTitle><CardDescription>{t("adminMFADescription")}</CardDescription></CardHeader><CardContent className="space-y-4"><div className="flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/40"><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("adminMFAStatus")}</div><div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{mfaStatus.enabled ? t("adminMFAEnabledBody") : t("adminMFADisabledBody")}</div></div><Badge variant={mfaStatus.enabled ? "success" : "muted"}>{mfaStatus.enabled ? t("adminMFAEnabled") : t("adminMFADisabled")}</Badge></div>{mfaStatus.enabled ? <form className="space-y-3" onSubmit={disableMFA}><div className="space-y-2"><Label htmlFor="admin-mfa-disable-code">{t("profileMFACode")}</Label><Input id="admin-mfa-disable-code" inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={mfaCode} onChange={(event) => setMfaCode(event.target.value.replace(/\D/g, "").slice(0, 6))} placeholder="000000" disabled={accountBusy} required /></div><Button type="submit" variant="destructive" disabled={accountBusy || securitySettings.admin_mfa_enabled} className="gap-2"><ShieldOff className="h-4 w-4" />{t("adminMFADisable")}</Button>{securitySettings.admin_mfa_enabled ? <p className="text-xs text-amber-700 dark:text-amber-300">{t("systemSettingsMFAEnforced")}</p> : null}</form> : mfaEnrollment ? <div className="grid gap-4 sm:grid-cols-[170px_minmax(0,1fr)]"><div className="flex items-start justify-center rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-700"><QRCodeSVG value={mfaEnrollment.otpauth_url} size={145} includeMargin /></div><div className="space-y-3"><p className="text-xs leading-5 text-slate-600 dark:text-slate-400">{t("adminMFAEnrollmentHint")}</p><div className="rounded-xl border border-slate-200 bg-slate-50 p-3 dark:border-slate-800 dark:bg-slate-950/40"><div className="text-[11px] font-semibold text-slate-500 dark:text-slate-400">{t("profileMFASecret")}</div><div className="mt-2 flex items-center gap-2"><code className="min-w-0 flex-1 break-all font-mono text-xs text-slate-800 dark:text-slate-200">{mfaEnrollment.secret}</code><Button type="button" variant="outline" size="icon" onClick={() => void copySecret()} title={t("profileMFACopySecret")} aria-label={t("profileMFACopySecret")}><Check className={cn("h-4 w-4", copied ? "text-emerald-600" : "hidden")} /><Copy className={cn("h-4 w-4", copied ? "hidden" : "")} /></Button></div></div><form className="space-y-3" onSubmit={confirmMFA}><div className="space-y-2"><Label htmlFor="admin-mfa-confirm-code">{t("profileMFACode")}</Label><Input id="admin-mfa-confirm-code" inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={mfaCode} onChange={(event) => setMfaCode(event.target.value.replace(/\D/g, "").slice(0, 6))} placeholder="000000" disabled={accountBusy} required /></div><div className="flex flex-wrap gap-2"><Button type="submit" disabled={accountBusy} className="gap-2"><ShieldCheck className="h-4 w-4" />{t("profileMFAConfirm")}</Button><Button type="button" variant="outline" onClick={cancelMFA} disabled={accountBusy}>{t("profileMFACancel")}</Button></div></form></div></div> : <Button type="button" variant="emerald" onClick={() => void beginMFA()} disabled={accountBusy} className="gap-2"><ShieldCheck className="h-4 w-4" />{t("adminMFAEnable")}</Button>}</CardContent></Card>
          </div>

          <Card className="border-indigo-500/20 shadow-sm dark:border-indigo-400/20 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><ShieldCheck className="h-5 w-5 text-indigo-600" />{t("systemSettingsPolicyTitle")}</CardTitle><CardDescription>{t("systemSettingsPolicyDescription")}</CardDescription></CardHeader><CardContent className="space-y-4"><div className="flex flex-col gap-4 rounded-xl border border-indigo-500/20 bg-indigo-50/70 p-4 dark:bg-indigo-500/10 sm:flex-row sm:items-center sm:justify-between"><div><div className="text-sm font-bold text-slate-900 dark:text-white">{t("systemSettingsAdminMFA")}</div><p className="mt-1 text-xs text-slate-600 dark:text-slate-300">{t("systemSettingsAdminMFAHint")}</p></div><div className="flex items-center gap-3"><Switch checked={securitySettings.admin_mfa_enabled} disabled={securityBusy || !mfaStatus.enabled} onCheckedChange={(checked) => void persistSecurity(checked)} /><span className="text-xs font-semibold text-slate-800 dark:text-slate-200">{securitySettings.admin_mfa_enabled ? t("securityEnabledText") : t("securityDisabledText")}</span></div></div>{securityMessage.text ? <Notice message={securityMessage} /> : null}</CardContent></Card>
        </div>
      ) : (
        <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Settings2 className="h-5 w-5 text-indigo-600" />{t("systemSettingsBaseTitle")}</CardTitle><CardDescription>{t("systemSettingsBaseDescription")}</CardDescription></CardHeader><CardContent><form className="space-y-5" onSubmit={saveSiteSettings}><div className="space-y-2"><Label htmlFor="system-site-name">{t("systemSiteName")}</Label><Input id="system-site-name" value={siteForm.site_name} onChange={(event) => setSiteForm((current) => ({ ...current, site_name: event.target.value }))} maxLength={100} disabled={siteBusy} required /></div><div className="space-y-2"><Label htmlFor="system-site-logo">{t("systemSiteLogo")}</Label><Input id="system-site-logo" type="text" inputMode="url" value={siteForm.site_logo_url} onChange={(event) => setSiteForm((current) => ({ ...current, site_logo_url: event.target.value }))} placeholder="https://cdn.example.com/logo.png or /assets/logo.png" disabled={siteBusy} /><p className="text-xs text-slate-500 dark:text-slate-400">{t("systemAssetURLHint")}</p></div><div className="space-y-2"><Label htmlFor="system-site-favicon">{t("systemSiteFavicon")}</Label><Input id="system-site-favicon" type="text" inputMode="url" value={siteForm.site_favicon_url} onChange={(event) => setSiteForm((current) => ({ ...current, site_favicon_url: event.target.value }))} placeholder="https://cdn.example.com/favicon.ico or /favicon.ico" disabled={siteBusy} /></div><div className="grid gap-4 md:grid-cols-2"><AssetPreview label={t("systemSitePreview")} name={siteForm.site_name} source={siteForm.site_logo_url} /><AssetPreview label={t("systemSiteFaviconPreview")} name={siteForm.site_name} source={siteForm.site_favicon_url} compact /></div>{siteMessage.text ? <Notice message={siteMessage} /> : null}<Button type="submit" disabled={siteBusy} className="gap-2"><Save className="h-4 w-4" />{siteBusy ? t("systemSettingsSaving") : t("systemSettingsSave")}</Button></form></CardContent></Card>
      )}
    </div>
  );
}

function InfoField({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div className="space-y-1.5"><div className="text-xs font-semibold text-slate-500 dark:text-slate-400">{label}</div><div className={cn("min-h-10 rounded-xl border border-slate-200 bg-slate-50 px-3.5 py-2.5 text-sm text-slate-800 dark:border-slate-800 dark:bg-slate-950/40 dark:text-slate-200", mono ? "break-all font-mono text-[11px]" : "")}>{value}</div></div>;
}

function AssetPreview({ label, name, source, compact = false }: { label: string; name: string; source: string; compact?: boolean }) {
  return <div className="rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/40"><div className="text-xs font-semibold text-slate-500 dark:text-slate-400">{label}</div><div className="mt-3 flex items-center gap-3">{source ? <img src={source} alt="" className={cn("rounded-xl border border-slate-200 bg-white object-contain p-2 dark:border-slate-700", compact ? "h-10 w-10" : "h-16 w-16")} onError={(event) => { event.currentTarget.style.display = "none"; }} /> : <div className={cn("flex items-center justify-center rounded-xl bg-gradient-to-br from-indigo-600 to-cyan-500 text-sm font-bold text-white", compact ? "h-10 w-10" : "h-16 w-16")}>{(name || "A").slice(0, 2).toUpperCase()}</div>}<span className="min-w-0 truncate text-sm font-semibold text-slate-800 dark:text-slate-200">{name || "-"}</span></div></div>;
}

function Notice({ message }: { message: LoginMessage }) {
  return <div className={cn("rounded-xl border p-3 text-sm", message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : message.kind === "pending" ? "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300" : "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300")}>{message.text}</div>;
}
