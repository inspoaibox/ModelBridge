import React, { useState } from "react";
import {
  Check,
  Copy,
  KeyRound,
  LockKeyhole,
  Mail,
  RefreshCw,
  Save,
  ShieldCheck,
  ShieldOff,
  UserRound,
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  ConsoleProfile,
  EmailFormState,
  Language,
  LoginMessage,
  MFAEnrollment,
  MFAStatus,
  PasswordFormState,
  ProfileFormState,
  TranslationKey,
} from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface ProfilePanelProps {
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
  refreshProfile: (showPending?: boolean) => Promise<void>;
  saveProfile: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  saveEmail: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  savePassword: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  totpEnabled: boolean;
  mfaStatus: MFAStatus;
  mfaEnrollment: MFAEnrollment | null;
  profileMfaCode: string;
  setProfileMfaCode: (value: string) => void;
  mfaBusy: boolean;
  beginMFA: () => Promise<void>;
  confirmMFA: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  cancelMFA: () => void;
  disableMFA: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
}

export function ProfilePanel({
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
  refreshProfile,
  saveProfile,
  saveEmail,
  savePassword,
  totpEnabled,
  mfaStatus,
  mfaEnrollment,
  profileMfaCode,
  setProfileMfaCode,
  mfaBusy,
  beginMFA,
  confirmMFA,
  cancelMFA,
  disableMFA,
}: ProfilePanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [copied, setCopied] = useState(false);
  const busy = profileBusy || mfaBusy;

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
      <Card className="overflow-hidden border-indigo-500/20 bg-gradient-to-br from-indigo-500/10 via-cyan-500/5 to-white shadow-sm dark:from-indigo-500/15 dark:via-cyan-500/5 dark:to-slate-900/70">
        <CardContent className="flex flex-col gap-5 p-5 sm:flex-row sm:items-center sm:justify-between sm:p-6">
          <div className="flex min-w-0 items-center gap-4">
            <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-600 to-cyan-500 text-lg font-bold text-white shadow-lg shadow-indigo-500/20">
              {(profile?.display_name || profile?.email || "U").slice(0, 2).toUpperCase()}
            </div>
            <div className="min-w-0">
              <div className="text-xs font-bold uppercase tracking-[0.16em] text-indigo-600 dark:text-indigo-300">{t("consoleProfileTitle")}</div>
              <div className="mt-1 truncate text-xl font-extrabold text-slate-950 dark:text-white">{profile?.display_name || "-"}</div>
              <div className="mt-1 flex items-center gap-2 text-sm text-slate-600 dark:text-slate-300"><Mail className="h-3.5 w-3.5" />{profile?.email || "-"}</div>
            </div>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={() => void refreshProfile(true)} disabled={busy} className="gap-2 self-start sm:self-center">
            <RefreshCw className={cn("h-4 w-4", profileBusy ? "animate-spin" : "")} />
            {t("consoleRefresh")}
          </Button>
        </CardContent>
      </Card>

      {profileMessage.text ? (
        <div className={cn("rounded-xl border p-3 text-sm", profileMessage.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : profileMessage.kind === "pending" ? "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300" : "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300")}>{profileMessage.text}</div>
      ) : null}

      <div className="grid gap-6 xl:grid-cols-2">
        <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg"><UserRound className="h-5 w-5 text-indigo-600" />{t("profileIdentityTitle")}</CardTitle>
            <CardDescription>{t("profileIdentityDescription")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={saveProfile}>
              <div className="space-y-2"><Label htmlFor="profile-display-name">{t("profileDisplayName")}</Label><Input id="profile-display-name" value={profileForm.display_name} onChange={(event) => setProfileForm({ display_name: event.target.value })} autoComplete="name" disabled={busy} required maxLength={100} /></div>
              <div className="grid gap-3 sm:grid-cols-2">
                <InfoField label={t("profileAccountStatus")} value={profile?.status || "-"} />
                <InfoField label={t("profileTenantID")} value={profile?.tenant_id || "-"} mono />
              </div>
              <InfoField label={t("profileCreatedAt")} value={formatDate(profile?.created_at, language)} />
              <Button type="submit" disabled={busy} className="gap-2"><Save className="h-4 w-4" />{t("profileSave")}</Button>
            </form>
          </CardContent>
        </Card>

        <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg"><Mail className="h-5 w-5 text-cyan-600" />{t("profileEmailTitle")}</CardTitle>
            <CardDescription>{t("profileEmailDescription")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={saveEmail}>
              <div className="space-y-2"><Label htmlFor="profile-email">{t("profileNewEmail")}</Label><Input id="profile-email" type="email" value={emailForm.email} onChange={(event) => setEmailForm((current) => ({ ...current, email: event.target.value }))} autoComplete="email" disabled={busy} required /></div>
              <div className="space-y-2"><Label htmlFor="profile-email-password">{t("profileCurrentPassword")}</Label><Input id="profile-email-password" type="password" value={emailForm.current_password} onChange={(event) => setEmailForm((current) => ({ ...current, current_password: event.target.value }))} autoComplete="current-password" disabled={busy} required /></div>
              <Button type="submit" variant="secondary" disabled={busy} className="gap-2"><Mail className="h-4 w-4" />{t("profileEmailSave")}</Button>
            </form>
          </CardContent>
        </Card>

        <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg"><KeyRound className="h-5 w-5 text-amber-600" />{t("profilePasswordTitle")}</CardTitle>
            <CardDescription>{t("profilePasswordDescription")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={savePassword}>
              <div className="space-y-2"><Label htmlFor="profile-password-current">{t("profileCurrentPassword")}</Label><Input id="profile-password-current" type="password" value={passwordForm.current_password} onChange={(event) => setPasswordForm((current) => ({ ...current, current_password: event.target.value }))} autoComplete="current-password" disabled={busy} required /></div>
              <div className="space-y-2"><Label htmlFor="profile-password-new">{t("profileNewPassword")}</Label><Input id="profile-password-new" type="password" value={passwordForm.new_password} onChange={(event) => setPasswordForm((current) => ({ ...current, new_password: event.target.value }))} autoComplete="new-password" minLength={12} disabled={busy} required /></div>
              <div className="space-y-2"><Label htmlFor="profile-password-confirm">{t("profileConfirmPassword")}</Label><Input id="profile-password-confirm" type="password" value={passwordForm.confirm_password} onChange={(event) => setPasswordForm((current) => ({ ...current, confirm_password: event.target.value }))} autoComplete="new-password" minLength={12} disabled={busy} required /></div>
              <Button type="submit" variant="secondary" disabled={busy} className="gap-2"><LockKeyhole className="h-4 w-4" />{t("profilePasswordSave")}</Button>
            </form>
          </CardContent>
        </Card>

        {totpEnabled ? <Card className="border-emerald-500/20 shadow-sm dark:border-emerald-400/20 dark:bg-slate-900/60 xl:col-span-2">
          <CardHeader>
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div><CardTitle className="flex items-center gap-2 text-lg"><ShieldCheck className="h-5 w-5 text-emerald-600" />{t("profileMFASectionTitle")}</CardTitle><CardDescription>{t("profileMFASectionDescription")}</CardDescription></div>
              <Badge variant={mfaStatus.enabled ? "success" : "muted"} className="self-start">{mfaStatus.enabled ? t("profileMFAEnabled") : t("profileMFADisabled")}</Badge>
            </div>
          </CardHeader>
          <CardContent>
            {mfaStatus.enabled ? (
              <div className="space-y-4">
                <div className="flex flex-col gap-3 rounded-xl border border-emerald-500/20 bg-emerald-50/70 p-4 dark:bg-emerald-500/10 sm:flex-row sm:items-center sm:justify-between"><div><div className="text-sm font-semibold text-emerald-800 dark:text-emerald-200">{t("profileMFAEnabled")}</div><div className="mt-1 text-xs text-emerald-700/80 dark:text-emerald-300/80">{t("profileMFAEnabledBody")}</div>{mfaStatus.enrolled_at ? <div className="mt-2 text-[11px] text-emerald-700/70 dark:text-emerald-300/70">{t("profileMFAEnrolledAt")}: {formatDate(mfaStatus.enrolled_at, language)}</div> : null}</div><ShieldCheck className="h-8 w-8 shrink-0 text-emerald-600" /></div>
                <form className="flex flex-col gap-3 sm:flex-row sm:items-end" onSubmit={disableMFA}><div className="w-full max-w-sm space-y-2"><Label htmlFor="profile-mfa-disable-code">{t("profileMFACode")}</Label><Input id="profile-mfa-disable-code" inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={profileMfaCode} onChange={(event) => setProfileMfaCode(event.target.value.replace(/\D/g, "").slice(0, 6))} placeholder="000000" disabled={busy} required /></div><Button type="submit" variant="destructive" disabled={busy} className="gap-2"><ShieldOff className="h-4 w-4" />{t("profileMFADisable")}</Button></form>
                <p className="text-xs text-slate-500 dark:text-slate-400">{t("profileMFADisableHint")}</p>
              </div>
            ) : mfaEnrollment ? (
              <div className="grid gap-5 lg:grid-cols-[190px_minmax(0,1fr)]">
                <div className="flex items-start justify-center rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-700"><QRCodeSVG value={mfaEnrollment.otpauth_url} size={160} includeMargin /></div>
                <div className="space-y-4">
                  <div><h3 className="text-sm font-bold text-slate-900 dark:text-white">{t("profileMFAEnrollmentTitle")}</h3><p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-400">{t("profileMFAEnrollmentHint")}</p></div>
                  <div className="rounded-xl border border-slate-200 bg-slate-50 p-3 dark:border-slate-800 dark:bg-slate-950/40"><div className="text-[11px] font-semibold text-slate-500 dark:text-slate-400">{t("profileMFASecret")}</div><div className="mt-2 flex items-center gap-2"><code className="min-w-0 flex-1 break-all rounded-lg bg-white px-2.5 py-2 font-mono text-xs text-slate-800 dark:bg-slate-900 dark:text-slate-200">{mfaEnrollment.secret}</code><Button type="button" variant="outline" size="icon" onClick={() => void copySecret()} disabled={busy} title={t("profileMFACopySecret")} aria-label={t("profileMFACopySecret")}><Check className={cn("h-4 w-4", copied ? "text-emerald-600" : "hidden")} /><Copy className={cn("h-4 w-4", copied ? "hidden" : "")} /></Button></div><div className="mt-1 text-[10px] text-slate-400">{copied ? t("profileMFACopied") : t("profileMFACopySecret")}</div></div>
                  <form className="space-y-3" onSubmit={confirmMFA}><div className="space-y-2"><Label htmlFor="profile-mfa-confirm-code">{t("profileMFACode")}</Label><Input id="profile-mfa-confirm-code" inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={profileMfaCode} onChange={(event) => setProfileMfaCode(event.target.value.replace(/\D/g, "").slice(0, 6))} placeholder="000000" disabled={busy} required /></div><div className="flex flex-wrap gap-2"><Button type="submit" disabled={busy} className="gap-2"><ShieldCheck className="h-4 w-4" />{t("profileMFAConfirm")}</Button><Button type="button" variant="outline" onClick={cancelMFA} disabled={busy}>{t("profileMFACancel")}</Button></div></form>
                </div>
              </div>
            ) : (
              <div className="flex flex-col gap-4 rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/40 sm:flex-row sm:items-center sm:justify-between"><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("profileMFADisabled")}</div><div className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-400">{t("profileMFADisabledBody")}</div></div><Button type="button" variant="emerald" onClick={() => void beginMFA()} disabled={busy} className="gap-2"><ShieldCheck className="h-4 w-4" />{t("profileMFAEnable")}</Button></div>
            )}
          </CardContent>
        </Card> : null}
      </div>
    </div>
  );
}

function InfoField({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div className="space-y-1.5"><div className="text-xs font-semibold text-slate-500 dark:text-slate-400">{label}</div><div className={cn("min-h-10 rounded-xl border border-slate-200 bg-slate-50 px-3.5 py-2.5 text-sm text-slate-800 dark:border-slate-800 dark:bg-slate-950/40 dark:text-slate-200", mono ? "break-all font-mono text-[11px]" : "")}>{value}</div></div>;
}

function formatDate(value: string | undefined, language: Language) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "medium", timeStyle: "short" }).format(date);
}
