import React, { useEffect, useState } from "react";
import {
  Copy,
  CreditCard,
  Globe2,
  KeyRound,
  Mail,
  Pencil,
  PlugZap,
  Plus,
  Save,
  Send,
  Settings2,
  ShieldCheck,
  ShieldOff,
  Trash2,
  ToggleRight,
  UserRound,
  X,
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { resolveAPIEndpointURLs } from "@/lib/api-endpoint";
import {
  APIEndpoint,
  APIEndpointFormState,
  ConsoleProfile,
  EmailFormState,
  EmailSettings,
  EmailTemplate,
  EmailTemplateFormState,
  FeatureSettings,
  Language,
  LoginMessage,
  MFAEnrollment,
  MFAStatus,
  PasswordFormState,
  ProfileFormState,
  SecuritySettings,
  SiteSettings,
  SMTPSettingsForm,
  PaymentProviderConfig,
  PaymentRechargePackage,
  LoginSettings,
  TranslationKey,
} from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";
import { PaymentSettingsPanel } from "@/components/admin/PaymentSettingsPanel";
import { PaymentPackagesPanel } from "@/components/admin/PaymentPackagesPanel";
import { LoginSettingsPanel } from "@/components/admin/LoginSettingsPanel";

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
  canUpdateSystemSettings: boolean;
  siteForm: SiteSettings;
  setSiteForm: React.Dispatch<React.SetStateAction<SiteSettings>>;
  siteBusy: boolean;
  siteMessage: LoginMessage;
  saveSiteSettings: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  apiEndpoints: APIEndpoint[];
  apiEndpointFormOpen: boolean;
  apiEndpointForm: APIEndpointFormState;
  setAPIEndpointForm: React.Dispatch<React.SetStateAction<APIEndpointFormState>>;
  apiEndpointBusy: boolean;
  apiEndpointActionBusy: string;
  apiEndpointMessage: LoginMessage;
  openCreateAPIEndpoint: () => void;
  openEditAPIEndpoint: (endpoint: APIEndpoint) => void;
  closeAPIEndpointForm: () => void;
  saveAPIEndpoint: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  toggleAPIEndpoint: (endpoint: APIEndpoint) => Promise<void>;
  deleteAPIEndpoint: (endpoint: APIEndpoint) => Promise<void>;
  smtpForm: SMTPSettingsForm;
  setSmtpForm: React.Dispatch<React.SetStateAction<SMTPSettingsForm>>;
  emailSettings: EmailSettings | null;
  emailBusy: boolean;
  emailMessage: LoginMessage;
  emailTestRecipient: string;
  setEmailTestRecipient: (value: string) => void;
  smtpConnectionBusy: boolean;
  smtpMessageBusy: boolean;
  saveEmailSettings: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  testSMTPConnection: () => Promise<void>;
  sendTestEmail: () => Promise<void>;
  featureSettings: FeatureSettings;
  featureBusy: boolean;
  featureMessage: LoginMessage;
  saveFeatureSettings: (settings: FeatureSettings) => Promise<void>;
  emailTemplates: EmailTemplate[];
  emailTemplatesBusy: boolean;
  emailTemplatesMessage: LoginMessage;
  saveEmailTemplate: (form: EmailTemplateFormState) => Promise<boolean>;
  deleteEmailTemplate: (template: EmailTemplate) => Promise<void>;
  paymentConfigs: PaymentProviderConfig[];
  paymentSettingsBusy: boolean;
  paymentSettingsMessage: LoginMessage;
  savePaymentConfig: (provider: PaymentProviderConfig["provider"], enabled: boolean, values: Record<string, string>, clear: string[]) => Promise<void>;
  paymentRechargePackages: PaymentRechargePackage[];
  savePaymentRechargePackages: (packages: PaymentRechargePackage[]) => Promise<void>;
  canUpdatePaymentSettings: boolean;
  loginSettings: LoginSettings;
  loginSettingsBusy: boolean;
  loginSettingsMessage: LoginMessage;
  saveLoginSettings: (settings: LoginSettings) => Promise<void>;
}

export function AdminSettingsPanel(props: AdminSettingsPanelProps) {
  const { language } = props;
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [tab, setTab] = useState<"admin" | "base" | "email" | "features" | "payments" | "packages" | "login">("admin");

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2 border-b border-slate-200/80 pb-3 dark:border-slate-800/80">
        <TabButton active={tab === "admin"} icon={UserRound} label={t("systemSettingsAdminTab")} onClick={() => setTab("admin")} />
        <TabButton active={tab === "base"} icon={Settings2} label={t("systemSettingsBaseTab")} onClick={() => setTab("base")} />
        <TabButton active={tab === "email"} icon={Mail} label={t("systemSettingsEmailTab")} onClick={() => setTab("email")} />
        <TabButton active={tab === "features"} icon={ToggleRight} label={t("systemSettingsFeaturesTab")} onClick={() => setTab("features")} />
        <TabButton active={tab === "payments"} icon={CreditCard} label={t("systemSettingsPaymentsTab")} onClick={() => setTab("payments")} />
        <TabButton active={tab === "packages"} icon={CreditCard} label={t("paymentRechargePackagesTitle")} onClick={() => setTab("packages")} />
        <TabButton active={tab === "login"} icon={KeyRound} label={t("loginSettingsNav")} onClick={() => setTab("login")} />
      </div>
      {tab === "admin" ? <AdminTab {...props} t={t} canUpdate={props.canUpdateSystemSettings} /> : null}
      {tab === "base" ? <BaseTab {...props} t={t} canUpdate={props.canUpdateSystemSettings} /> : null}
      {tab === "email" ? <EmailTab {...props} t={t} canUpdate={props.canUpdateSystemSettings} /> : null}
      {tab === "features" ? <FeatureTab {...props} t={t} canUpdate={props.canUpdateSystemSettings} /> : null}
      {tab === "payments" ? <PaymentSettingsPanel language={language} configs={props.paymentConfigs} busy={props.paymentSettingsBusy} message={props.paymentSettingsMessage} save={props.savePaymentConfig} canUpdate={props.canUpdatePaymentSettings} /> : null}
      {tab === "packages" ? <PaymentPackagesPanel language={language} packages={props.paymentRechargePackages} busy={props.paymentSettingsBusy} message={props.paymentSettingsMessage} save={props.savePaymentRechargePackages} canUpdate={props.canUpdatePaymentSettings} /> : null}
      {tab === "login" ? <LoginSettingsPanel language={language} settings={props.loginSettings} busy={props.loginSettingsBusy} message={props.loginSettingsMessage} save={props.saveLoginSettings} canUpdate={props.canUpdateSystemSettings} /> : null}
      {props.apiEndpointFormOpen ? <APIEndpointModal {...props} t={t} canUpdate={props.canUpdateSystemSettings} /> : null}
    </div>
  );
}

type Translator = (key: TranslationKey) => string;

function TabButton({ active, icon: Icon, label, onClick }: { active: boolean; icon: typeof UserRound; label: string; onClick: () => void }) {
  return <button type="button" onClick={onClick} className={cn("inline-flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold transition-colors", active ? "bg-indigo-600 text-white shadow-md shadow-indigo-500/20" : "text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-900")}><Icon className="h-4 w-4" />{label}</button>;
}

function AdminTab({ t, profile, profileForm, setProfileForm, emailForm, setEmailForm, passwordForm, setPasswordForm, profileBusy, profileMessage, saveProfile, saveEmail, savePassword, mfaStatus, mfaEnrollment, mfaCode, setMfaCode, mfaBusy, beginMFA, confirmMFA, cancelMFA, disableMFA, securitySettings, securityMessage, securityBusy, persistSecurity, featureSettings, canUpdate }: AdminSettingsPanelProps & { t: Translator; canUpdate: boolean }) {
  const busy = profileBusy || mfaBusy;
  const entryPath = profile?.admin_entry_path?.trim() || "";
  const entryURL = /^\/admin-[A-Za-z0-9_-]{16,160}$/.test(entryPath) ? new URL(entryPath, window.location.origin).toString() : "";
  return <div className="space-y-6">
    <Card className="border-indigo-500/20 bg-gradient-to-br from-indigo-500/10 via-cyan-500/5 to-white dark:from-indigo-500/15 dark:via-cyan-500/5 dark:to-slate-900/70"><CardContent className="flex flex-col gap-5 p-5 sm:flex-row sm:items-center sm:justify-between sm:p-6"><div className="flex min-w-0 items-center gap-4"><div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-600 to-cyan-500 text-lg font-bold text-white">{(profile?.display_name || profile?.email || "A").slice(0, 2).toUpperCase()}</div><div className="min-w-0"><div className="text-xs font-bold uppercase tracking-[0.16em] text-indigo-600 dark:text-indigo-300">{t("adminCenterTitle")}</div><div className="mt-1 truncate text-xl font-extrabold text-slate-950 dark:text-white">{profile?.display_name || "-"}</div><div className="mt-1 flex items-center gap-2 text-sm text-slate-600 dark:text-slate-300"><Mail className="h-3.5 w-3.5" />{profile?.email || "-"}</div></div></div><Badge variant="success">{t("adminCenterRole")}: {profile?.roles?.join(", ") || "-"}</Badge></CardContent></Card>
    {profileMessage.text ? <Notice message={profileMessage} /> : null}
    <div className="grid gap-6 xl:grid-cols-2">
      <Card className="border-violet-500/20"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><ShieldCheck className="h-5 w-5 text-violet-600" />{t("adminEntryURLTitle")}</CardTitle><CardDescription>{t("adminEntryURLDescription")}</CardDescription></CardHeader><CardContent className="space-y-4"><InfoField label={t("adminEntryURLLabel")} value={entryURL || t("adminEntryURLUnavailable")} mono /><Button type="button" variant="secondary" disabled={!entryURL} onClick={() => void navigator.clipboard?.writeText(entryURL)} className="gap-2"><Copy className="h-4 w-4" />{t("adminEntryURLCopy")}</Button></CardContent></Card>
      <Card><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><UserRound className="h-5 w-5 text-indigo-600" />{t("adminProfileTitle")}</CardTitle><CardDescription>{t("adminProfileDescription")}</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={saveProfile}><InputField id="admin-profile-display-name" label={t("profileDisplayName")} value={profileForm.display_name} onChange={(value) => setProfileForm({ display_name: value })} disabled={busy} /><InfoField label={t("adminCenterAccountID")} value={profile?.id || "-"} mono /><Button type="submit" disabled={busy} className="gap-2"><Save className="h-4 w-4" />{t("profileSave")}</Button></form></CardContent></Card>
      <Card><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Mail className="h-5 w-5 text-cyan-600" />{t("profileEmailTitle")}</CardTitle><CardDescription>{t("adminEmailDescription")}</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={saveEmail}><InputField id="admin-profile-email" label={t("profileNewEmail")} type="email" value={emailForm.email} onChange={(value) => setEmailForm((current) => ({ ...current, email: value }))} disabled={busy} /><InputField id="admin-profile-email-password" label={t("profileCurrentPassword")} type="password" value={emailForm.current_password} onChange={(value) => setEmailForm((current) => ({ ...current, current_password: value }))} disabled={busy} /><Button type="submit" variant="secondary" disabled={busy} className="gap-2"><Mail className="h-4 w-4" />{t("profileEmailSave")}</Button></form></CardContent></Card>
      <Card><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><KeyRound className="h-5 w-5 text-amber-600" />{t("profilePasswordTitle")}</CardTitle><CardDescription>{t("adminPasswordDescription")}</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={savePassword}><InputField id="admin-current-password" label={t("profileCurrentPassword")} type="password" value={passwordForm.current_password} onChange={(value) => setPasswordForm((current) => ({ ...current, current_password: value }))} disabled={busy} /><InputField id="admin-new-password" label={t("profileNewPassword")} type="password" value={passwordForm.new_password} onChange={(value) => setPasswordForm((current) => ({ ...current, new_password: value }))} disabled={busy} /><InputField id="admin-confirm-password" label={t("profileConfirmPassword")} type="password" value={passwordForm.confirm_password} onChange={(value) => setPasswordForm((current) => ({ ...current, confirm_password: value }))} disabled={busy} /><Button type="submit" variant="secondary" disabled={busy} className="gap-2"><KeyRound className="h-4 w-4" />{t("profilePasswordSave")}</Button></form></CardContent></Card>
      {featureSettings.totp_enabled ? <MFAAdminCard t={t} mfaStatus={mfaStatus} mfaEnrollment={mfaEnrollment} mfaCode={mfaCode} setMfaCode={setMfaCode} mfaBusy={mfaBusy} beginMFA={beginMFA} confirmMFA={confirmMFA} cancelMFA={cancelMFA} disableMFA={disableMFA} enforced={securitySettings.admin_mfa_enabled} copySecret={() => navigator.clipboard?.writeText(mfaEnrollment?.secret || "")} /> : null}
    </div>
    {featureSettings.totp_enabled ? <Card className="border-indigo-500/20"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><ShieldCheck className="h-5 w-5 text-indigo-600" />{t("systemSettingsPolicyTitle")}</CardTitle><CardDescription>{t("systemSettingsPolicyDescription")}</CardDescription></CardHeader><CardContent><div className="flex flex-col gap-4 rounded-xl border border-indigo-500/20 bg-indigo-50/70 p-4 dark:bg-indigo-500/10 sm:flex-row sm:items-center sm:justify-between"><div><div className="text-sm font-bold text-slate-900 dark:text-white">{t("systemSettingsAdminMFA")}</div><p className="mt-1 text-xs text-slate-600 dark:text-slate-300">{t("systemSettingsAdminMFAHint")}</p></div><div className="flex items-center gap-3"><Switch checked={securitySettings.admin_mfa_enabled} disabled={securityBusy || !canUpdate} onCheckedChange={(checked) => void persistSecurity(checked)} /><span className="text-xs font-semibold">{securitySettings.admin_mfa_enabled ? t("securityEnabledText") : t("securityDisabledText")}</span></div></div>{securityMessage.text ? <div className="mt-4"><Notice message={securityMessage} /></div> : null}</CardContent></Card> : null}
  </div>;
}

function MFAAdminCard({ t, mfaStatus, mfaEnrollment, mfaCode, setMfaCode, mfaBusy, beginMFA, confirmMFA, cancelMFA, disableMFA, enforced, copySecret }: { t: Translator; mfaStatus: MFAStatus; mfaEnrollment: MFAEnrollment | null; mfaCode: string; setMfaCode: (value: string) => void; mfaBusy: boolean; beginMFA: () => Promise<void>; confirmMFA: (event: React.FormEvent<HTMLFormElement>) => Promise<void>; cancelMFA: () => void; disableMFA: (event: React.FormEvent<HTMLFormElement>) => Promise<void>; enforced: boolean; copySecret: () => Promise<void> }) {
  return <Card className="border-emerald-500/20"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><ShieldCheck className="h-5 w-5 text-emerald-600" />{t("adminMFATitle")}</CardTitle><CardDescription>{t("adminMFADescription")}</CardDescription></CardHeader><CardContent className="space-y-4"><div className="flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/40"><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("adminMFAStatus")}</div><div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{mfaStatus.enabled ? t("adminMFAEnabledBody") : t("adminMFADisabledBody")}</div></div><Badge variant={mfaStatus.enabled ? "success" : "muted"}>{mfaStatus.enabled ? t("adminMFAEnabled") : t("adminMFADisabled")}</Badge></div>{mfaStatus.enabled ? <form className="space-y-3" onSubmit={disableMFA}><InputField id="admin-mfa-disable-code" label={t("profileMFACode")} value={mfaCode} onChange={(value) => setMfaCode(value.replace(/\D/g, "").slice(0, 6))} disabled={mfaBusy} /><Button type="submit" variant="destructive" disabled={mfaBusy || enforced} className="gap-2"><ShieldOff className="h-4 w-4" />{t("adminMFADisable")}</Button></form> : mfaEnrollment ? <div className="space-y-3"><div className="flex justify-center rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-700"><QRCodeSVG value={mfaEnrollment.otpauth_url} size={145} includeMargin /></div><div className="flex items-center gap-2"><code className="min-w-0 flex-1 break-all font-mono text-xs">{mfaEnrollment.secret}</code><Button type="button" variant="outline" size="icon" onClick={() => void copySecret()} aria-label={t("profileMFACopySecret")}><Copy className="h-4 w-4" /></Button></div><form className="space-y-3" onSubmit={confirmMFA}><InputField id="admin-mfa-confirm-code" label={t("profileMFACode")} value={mfaCode} onChange={(value) => setMfaCode(value.replace(/\D/g, "").slice(0, 6))} disabled={mfaBusy} /><div className="flex gap-2"><Button type="submit" disabled={mfaBusy}>{t("profileMFAConfirm")}</Button><Button type="button" variant="outline" onClick={cancelMFA} disabled={mfaBusy}>{t("profileMFACancel")}</Button></div></form></div> : <Button type="button" variant="emerald" onClick={() => void beginMFA()} disabled={mfaBusy} className="gap-2"><ShieldCheck className="h-4 w-4" />{t("adminMFAEnable")}</Button>}</CardContent></Card>;
}

function BaseTab({
  t,
  siteForm,
  setSiteForm,
  siteBusy,
  siteMessage,
  saveSiteSettings,
  apiEndpoints,
  apiEndpointActionBusy,
  apiEndpointMessage,
  openCreateAPIEndpoint,
  openEditAPIEndpoint,
  toggleAPIEndpoint,
  deleteAPIEndpoint,
  canUpdate,
}: AdminSettingsPanelProps & { t: Translator; canUpdate: boolean }) {
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <Settings2 className="h-5 w-5 text-indigo-600" />
            {t("systemSettingsBaseTitle")}
          </CardTitle>
          <CardDescription>{t("systemSettingsBaseDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-5" onSubmit={saveSiteSettings}>
            <InputField id="system-site-name" label={t("systemSiteName")} value={siteForm.site_name} onChange={(value) => setSiteForm((current) => ({ ...current, site_name: value }))} disabled={siteBusy || !canUpdate} />
            <InputField id="system-site-logo" label={t("systemSiteLogo")} value={siteForm.site_logo_url} onChange={(value) => setSiteForm((current) => ({ ...current, site_logo_url: value }))} disabled={siteBusy || !canUpdate} />
            <p className="-mt-3 text-xs text-slate-500 dark:text-slate-400">{t("systemAssetURLHint")}</p>
            <InputField id="system-site-favicon" label={t("systemSiteFavicon")} value={siteForm.site_favicon_url} onChange={(value) => setSiteForm((current) => ({ ...current, site_favicon_url: value }))} disabled={siteBusy || !canUpdate} />
            <div className="grid gap-4 md:grid-cols-2">
              <AssetPreview label={t("systemSitePreview")} name={siteForm.site_name} source={siteForm.site_logo_url} />
              <AssetPreview label={t("systemSiteFaviconPreview")} name={siteForm.site_name} source={siteForm.site_favicon_url} compact />
            </div>
            {siteMessage.text ? <Notice message={siteMessage} /> : null}
            <Button type="submit" disabled={siteBusy || !canUpdate} className="gap-2">
              <Save className="h-4 w-4" />
              {siteBusy ? t("systemSettingsSaving") : t("systemSettingsSave")}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card className="border-cyan-500/20">
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2 text-lg">
              <Globe2 className="h-5 w-5 text-cyan-600" />
              {t("systemAPIEndpointsTitle")}
            </CardTitle>
            <CardDescription>{t("systemAPIEndpointsDescription")}</CardDescription>
          </div>
          <Button type="button" onClick={openCreateAPIEndpoint} disabled={!canUpdate} className="gap-2">
            <Plus className="h-4 w-4" />
            {t("systemAPIEndpointAdd")}
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {apiEndpointMessage.text ? <Notice message={apiEndpointMessage} /> : null}
          {apiEndpoints.length === 0 ? (
            <div className="rounded-xl border border-dashed border-slate-300 px-4 py-12 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">
              {t("systemAPIEndpointsEmpty")}
            </div>
          ) : (
            <div className="divide-y divide-slate-100 overflow-hidden rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
              {apiEndpoints.map((endpoint) => {
                const busy = apiEndpointActionBusy === endpoint.id;
                const endpointURLs = resolveAPIEndpointURLs(endpoint);
                return (
                  <div key={endpoint.id} className="flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:justify-between">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-semibold text-slate-900 dark:text-white">{endpoint.name}</span>
                        <Badge variant={endpoint.enabled ? "success" : "muted"}>{endpoint.enabled ? t("systemAPIEndpointEnabled") : t("systemAPIEndpointDisabled")}</Badge>
                      </div>
                      <div className="mt-2 space-y-1 text-[11px]">
                        <div><span className="mr-2 font-semibold text-slate-500 dark:text-slate-400">{t("systemAPIEndpointRootURL")}</span><code className="break-all text-slate-600 dark:text-slate-300">{endpointURLs.root || "-"}</code></div>
                        <div><span className="mr-2 font-semibold text-indigo-600 dark:text-indigo-400">OpenAI</span><code className="break-all text-slate-600 dark:text-slate-300">{endpointURLs.openai || "-"}</code></div>
                        <div><span className="mr-2 font-semibold text-cyan-600 dark:text-cyan-400">Anthropic</span><code className="break-all text-slate-600 dark:text-slate-300">{endpointURLs.anthropic || "-"}</code></div>
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <Switch checked={endpoint.enabled} onCheckedChange={() => void toggleAPIEndpoint(endpoint)} disabled={busy || !canUpdate} aria-label={endpoint.enabled ? t("systemAPIEndpointDisable") : t("systemAPIEndpointEnable")} />
                      <Button type="button" variant="outline" size="icon" onClick={() => openEditAPIEndpoint(endpoint)} disabled={busy || !canUpdate} title={t("systemAPIEndpointEdit")} aria-label={t("systemAPIEndpointEdit")}>
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button type="button" variant="ghost" size="icon" onClick={() => void deleteAPIEndpoint(endpoint)} disabled={busy || !canUpdate} title={t("systemAPIEndpointDelete")} aria-label={t("systemAPIEndpointDelete")}>
                        <Trash2 className="h-4 w-4 text-rose-600" />
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function APIEndpointModal({
  t,
  apiEndpointForm,
  setAPIEndpointForm,
  apiEndpointBusy,
  apiEndpointMessage,
  closeAPIEndpointForm,
  saveAPIEndpoint,
  canUpdate,
}: AdminSettingsPanelProps & { t: Translator; canUpdate: boolean }) {
  return (
    <div className="fixed inset-0 z-[80] flex items-center justify-center bg-slate-950/55 p-4 backdrop-blur-sm">
      <Card className="w-full max-w-xl border-slate-200/80 bg-white shadow-2xl dark:border-slate-700 dark:bg-slate-900">
        <CardHeader className="flex flex-row items-start justify-between gap-4 border-b border-slate-200/80 pb-4 dark:border-slate-800">
          <div>
            <CardTitle className="flex items-center gap-2 text-xl text-slate-900 dark:text-white">
              <Globe2 className="h-5 w-5 text-cyan-600" />
              {apiEndpointForm.id ? t("systemAPIEndpointEdit") : t("systemAPIEndpointAdd")}
            </CardTitle>
            <CardDescription className="mt-1">{t("systemAPIEndpointModalDescription")}</CardDescription>
          </div>
          <Button type="button" variant="ghost" size="icon" onClick={closeAPIEndpointForm} disabled={apiEndpointBusy} aria-label={t("close")}>
            <X className="h-4 w-4" />
          </Button>
        </CardHeader>
        <CardContent>
          <form className="space-y-5" onSubmit={(event) => void saveAPIEndpoint(event)}>
            <InputField id="api-endpoint-name" label={t("systemAPIEndpointName")} value={apiEndpointForm.name} onChange={(value) => setAPIEndpointForm((current) => ({ ...current, name: value }))} disabled={apiEndpointBusy || !canUpdate} required />
            <div className="space-y-2">
              <Label htmlFor="api-endpoint-base-url">{t("systemAPIEndpointBaseURL")}</Label>
              <Input id="api-endpoint-base-url" type="url" value={apiEndpointForm.base_url} onChange={(event) => setAPIEndpointForm((current) => ({ ...current, base_url: event.target.value }))} placeholder="https://gateway.example.com" disabled={apiEndpointBusy || !canUpdate} required />
              <p className="text-xs leading-5 text-slate-500 dark:text-slate-400">{t("systemAPIEndpointBaseURLHint")}</p>
            </div>
            <div className="flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/40">
              <div>
                <div className="text-sm font-semibold text-slate-900 dark:text-white">{t("systemAPIEndpointStatus")}</div>
                <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("systemAPIEndpointStatusHint")}</p>
              </div>
              <Switch checked={apiEndpointForm.enabled} onCheckedChange={(checked) => setAPIEndpointForm((current) => ({ ...current, enabled: checked }))} disabled={apiEndpointBusy || !canUpdate} />
            </div>
            {apiEndpointMessage.text ? <Notice message={apiEndpointMessage} /> : null}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="outline" onClick={closeAPIEndpointForm} disabled={apiEndpointBusy}>{t("cancel")}</Button>
              <Button type="submit" disabled={apiEndpointBusy || !canUpdate} className="gap-2">
                <Save className="h-4 w-4" />
                {apiEndpointBusy ? t("systemAPIEndpointSaving") : t("systemAPIEndpointSave")}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}

function EmailTab({ t, emailSettings, smtpForm, setSmtpForm, emailBusy, emailMessage, emailTestRecipient, setEmailTestRecipient, smtpConnectionBusy, smtpMessageBusy, saveEmailSettings, testSMTPConnection, sendTestEmail, emailTemplates, emailTemplatesBusy, emailTemplatesMessage, templateForm, setTemplateForm, openTemplate, submitTemplate, deleteEmailTemplate, canUpdate }: AdminSettingsPanelProps & { t: Translator; canUpdate: boolean; templateForm?: EmailTemplateFormState | null; setTemplateForm?: React.Dispatch<React.SetStateAction<EmailTemplateFormState | null>>; openTemplate?: (template?: EmailTemplate) => void; submitTemplate?: (event: React.FormEvent<HTMLFormElement>) => Promise<void> }) {
  return <div className="space-y-6"><Card className="border-cyan-500/20"><CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"><div><CardTitle className="flex items-center gap-2 text-lg"><Mail className="h-5 w-5 text-cyan-600" />{t("emailSettingsTitle")}</CardTitle><CardDescription>{t("emailSettingsDescription")}</CardDescription></div><Badge variant={emailSettings?.email_enabled ? "success" : "muted"}>{emailSettings?.email_enabled ? t("emailSystemEnabled") : t("emailSystemDisabled")}</Badge></CardHeader><CardContent><form className="space-y-5" onSubmit={saveEmailSettings}><div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_150px]"><InputField id="smtp-host" label={t("emailSMTPHost")} value={smtpForm.smtp_host} onChange={(value) => setSmtpForm((current) => ({ ...current, smtp_host: value }))} disabled={emailBusy || !canUpdate} /><InputField id="smtp-port" label={t("emailSMTPPort")} type="number" value={String(smtpForm.smtp_port || "")} onChange={(value) => setSmtpForm((current) => ({ ...current, smtp_port: Number(value) || 0 }))} disabled={emailBusy || !canUpdate} /></div><div className="grid gap-4 sm:grid-cols-2"><InputField id="smtp-username" label={t("emailSMTPUsername")} value={smtpForm.smtp_username} onChange={(value) => setSmtpForm((current) => ({ ...current, smtp_username: value }))} disabled={emailBusy || !canUpdate} /><div className="space-y-2"><Label htmlFor="smtp-password">{t("emailSMTPPassword")}</Label><Input id="smtp-password" type="password" value={smtpForm.smtp_password} onChange={(event) => setSmtpForm((current) => ({ ...current, smtp_password: event.target.value, smtp_password_clear: false }))} placeholder={t("emailSMTPPasswordPlaceholder")} disabled={emailBusy || !canUpdate} /><p className="text-xs text-slate-500 dark:text-slate-400">{emailSettings?.smtp_password_configured ? t("emailSMTPPasswordConfigured") : t("emailSMTPPasswordNotConfigured")}</p></div><InputField id="smtp-from-email" label={t("emailSMTPFromEmail")} type="email" value={smtpForm.smtp_from_email} onChange={(value) => setSmtpForm((current) => ({ ...current, smtp_from_email: value }))} disabled={emailBusy || !canUpdate} /><InputField id="smtp-from-name" label={t("emailSMTPFromName")} value={smtpForm.smtp_from_name} onChange={(value) => setSmtpForm((current) => ({ ...current, smtp_from_name: value }))} disabled={emailBusy || !canUpdate} /></div><div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="smtp-public-url">{t("emailPublicBaseURL")}</Label><Input id="smtp-public-url" type="url" value={smtpForm.public_base_url} onChange={(event) => setSmtpForm((current) => ({ ...current, public_base_url: event.target.value }))} disabled={emailBusy || !canUpdate} /><p className="text-xs text-slate-500 dark:text-slate-400">{t("emailPublicBaseURLHint")}</p></div><div className="flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/40"><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("emailSMTPTLS")}</div><div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("emailSMTPTLSHint")}</div></div><Switch checked={smtpForm.smtp_tls} onCheckedChange={(checked) => setSmtpForm((current) => ({ ...current, smtp_tls: checked }))} disabled={emailBusy || !canUpdate} /></div></div><label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300"><input type="checkbox" checked={smtpForm.smtp_password_clear} onChange={(event) => setSmtpForm((current) => ({ ...current, smtp_password_clear: event.target.checked, smtp_password: "" }))} disabled={emailBusy || !canUpdate} />{t("emailSMTPPasswordClear")}</label>{emailMessage.text ? <Notice message={emailMessage} /> : null}<div className="flex flex-wrap gap-2"><Button type="submit" disabled={emailBusy || !canUpdate} className="gap-2"><Save className="h-4 w-4" />{emailBusy ? t("emailSettingsSaving") : t("emailSettingsSave")}</Button><Button type="button" variant="outline" onClick={() => void testSMTPConnection()} disabled={smtpConnectionBusy || emailBusy || !canUpdate} className="gap-2"><PlugZap className="h-4 w-4" />{smtpConnectionBusy ? t("emailSMTPTesting") : t("emailSMTPTestConnection")}</Button></div></form><div className="mt-6 border-t border-slate-200 pt-5 dark:border-slate-800"><div className="mb-3"><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("emailTestMessageTitle")}</div><p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("emailTestMessageHint")}</p></div><div className="flex flex-col gap-2 sm:flex-row"><Input type="email" value={emailTestRecipient} onChange={(event) => setEmailTestRecipient(event.target.value)} placeholder={t("emailTestRecipientPlaceholder")} disabled={smtpMessageBusy || !canUpdate} /><Button type="button" variant="secondary" onClick={() => void sendTestEmail()} disabled={smtpMessageBusy || !canUpdate} className="gap-2"><Send className="h-4 w-4" />{smtpMessageBusy ? t("emailTestSending") : t("emailSendTestMessage")}</Button></div></div></CardContent></Card>{openTemplate && setTemplateForm && submitTemplate ? <EmailTemplates t={t} items={emailTemplates} busy={emailTemplatesBusy} message={emailTemplatesMessage} form={templateForm || null} setForm={setTemplateForm} openTemplate={openTemplate} submitTemplate={submitTemplate} deleteTemplate={deleteEmailTemplate} canUpdate={canUpdate} /> : null}</div>;
}

function EmailTemplates({ t, items, busy, message, form, setForm, openTemplate, submitTemplate, deleteTemplate, canUpdate }: { t: Translator; items: EmailTemplate[]; busy: boolean; message: LoginMessage; form: EmailTemplateFormState | null; setForm: React.Dispatch<React.SetStateAction<EmailTemplateFormState | null>>; openTemplate: (template?: EmailTemplate) => void; submitTemplate: (event: React.FormEvent<HTMLFormElement>) => Promise<void>; deleteTemplate: (template: EmailTemplate) => Promise<void>; canUpdate: boolean }) {
  const languageLabel = (value: string) => value === "zh" ? t("languageChinese") : t("languageEnglish");
  return <Card><CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"><div><CardTitle className="flex items-center gap-2 text-lg"><Mail className="h-5 w-5 text-indigo-600" />{t("emailTemplatesTitle")}</CardTitle><CardDescription>{t("emailTemplatesDescription")}</CardDescription></div><Button type="button" onClick={() => openTemplate()} disabled={!canUpdate} className="gap-2"><Plus className="h-4 w-4" />{t("emailTemplateNew")}</Button></CardHeader><CardContent className="space-y-4">{message.text ? <Notice message={message} /> : null}{form ? <form className="space-y-4 rounded-xl border border-indigo-500/20 bg-indigo-50/50 p-4 dark:bg-indigo-500/10" onSubmit={(event) => void submitTemplate(event)}><div className="grid gap-4 sm:grid-cols-2"><InputField id="template-event" label={t("emailTemplateEvent")} value={form.event_code} onChange={(value) => setForm((current) => current ? { ...current, event_code: value } : current)} disabled={!canUpdate || busy} /><div className="space-y-2"><Label htmlFor="template-language">{t("emailTemplateLanguage")}</Label><select id="template-language" value={form.language} onChange={(event) => setForm((current) => current ? { ...current, language: event.target.value as "zh" | "en" } : current)} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 text-sm dark:border-slate-700 dark:bg-slate-950" disabled={!canUpdate || busy}><option value="zh">{languageLabel("zh")}</option><option value="en">{languageLabel("en")}</option></select></div></div><InputField id="template-subject" label={t("emailTemplateSubject")} value={form.subject} onChange={(value) => setForm((current) => current ? { ...current, subject: value } : current)} disabled={!canUpdate || busy} /><div className="space-y-2"><Label htmlFor="template-html">{t("emailTemplateHTML")}</Label><textarea id="template-html" rows={8} value={form.html_body} onChange={(event) => setForm((current) => current ? { ...current, html_body: event.target.value } : current)} className="min-h-40 w-full rounded-xl border border-slate-200 bg-white p-3 font-mono text-xs text-slate-900 outline-none dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100" disabled={!canUpdate || busy} required /><p className="text-xs text-slate-500 dark:text-slate-400">{t("emailTemplateVariables")}</p></div><label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300"><input type="checkbox" checked={form.enabled} onChange={(event) => setForm((current) => current ? { ...current, enabled: event.target.checked } : current)} disabled={!canUpdate || busy} />{t("emailTemplateEnabled")}</label><div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={() => setForm(null)} disabled={busy}>{t("cancel")}</Button><Button type="submit" disabled={!canUpdate || busy} className="gap-2"><Save className="h-4 w-4" />{t("emailTemplateSave")}</Button></div></form> : null}<div className="divide-y divide-slate-100 overflow-hidden rounded-xl border border-slate-200 dark:divide-slate-800 dark:border-slate-800">{items.length === 0 ? <div className="p-10 text-center text-sm text-slate-500 dark:text-slate-400">{t("emailTemplatesEmpty")}</div> : items.map((item) => <div key={item.id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><code className="rounded bg-slate-100 px-2 py-1 text-xs dark:bg-slate-800">{item.event_code}</code><Badge variant="muted">{languageLabel(item.language)}</Badge><Badge variant={item.enabled ? "success" : "muted"}>{item.enabled ? t("emailTemplateEnabled") : t("emailTemplateDisabled")}</Badge></div><div className="mt-2 truncate text-sm font-semibold text-slate-900 dark:text-white">{item.subject}</div></div><div className="flex shrink-0 gap-2"><Button type="button" variant="outline" size="sm" onClick={() => openTemplate(item)} disabled={!canUpdate || busy}>{t("emailTemplateEdit")}</Button><Button type="button" variant="ghost" size="icon" onClick={() => void deleteTemplate(item)} disabled={!canUpdate || busy} title={t("emailTemplateDelete")} aria-label={t("emailTemplateDelete")}><Trash2 className="h-4 w-4 text-rose-600" /></Button></div></div>)}</div></CardContent></Card>;
}

function FeatureTab({ t, featureSettings: settings, featureBusy: busy, featureMessage: message, saveFeatureSettings: save, canUpdate }: AdminSettingsPanelProps & { t: Translator; canUpdate: boolean }) {
  const [draft, setDraft] = useState(settings);
  useEffect(() => setDraft(settings), [settings]);
  const items: Array<{ key: keyof FeatureSettings; label: TranslationKey; description: TranslationKey }> = [
    { key: "registration_enabled", label: "featureRegistration", description: "featureRegistrationHint" },
    { key: "email_verification_enabled", label: "featureEmailVerification", description: "featureEmailVerificationHint" },
    { key: "email_password_reset_enabled", label: "featureEmailPasswordReset", description: "featureEmailPasswordResetHint" },
    { key: "email_subscription_enabled", label: "featureEmailSubscription", description: "featureEmailSubscriptionHint" },
    { key: "email_low_balance_alert_enabled", label: "featureEmailLowBalance", description: "featureEmailLowBalanceHint" },
    { key: "email_recharge_success_enabled", label: "featureEmailRecharge", description: "featureEmailRechargeHint" },
    { key: "email_usage_limit_alert_enabled", label: "featureEmailUsageLimit", description: "featureEmailUsageLimitHint" },
    { key: "email_content_audit_enabled", label: "featureEmailContentAudit", description: "featureEmailContentAuditHint" },
    { key: "email_account_disabled_enabled", label: "featureEmailAccountDisabled", description: "featureEmailAccountDisabledHint" },
    { key: "email_cyber_policy_enabled", label: "featureEmailCyberPolicy", description: "featureEmailCyberPolicyHint" },
    { key: "email_operations_enabled", label: "featureEmailOperations", description: "featureEmailOperationsHint" },
  ];
  const stepUpPolicies: Array<{ key: keyof FeatureSettings; label: TranslationKey; description: TranslationKey }> = [
    { key: "step_up_channel_model_enabled", label: "featureStepUpChannelModel", description: "featureStepUpChannelModelHint" },
    { key: "step_up_group_enabled", label: "featureStepUpGroup", description: "featureStepUpGroupHint" },
    { key: "step_up_token_enabled", label: "featureStepUpToken", description: "featureStepUpTokenHint" },
    { key: "step_up_user_enabled", label: "featureStepUpUser", description: "featureStepUpUserHint" },
    { key: "step_up_role_enabled", label: "featureStepUpRole", description: "featureStepUpRoleHint" },
    { key: "step_up_billing_enabled", label: "featureStepUpBilling", description: "featureStepUpBillingHint" },
    { key: "step_up_system_enabled", label: "featureStepUpSystem", description: "featureStepUpSystemHint" },
  ];

  return (
    <Card className="border-amber-500/20">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg"><ToggleRight className="h-5 w-5 text-amber-600" />{t("featureSettingsTitle")}</CardTitle>
        <CardDescription>{t("featureSettingsDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="space-y-5" onSubmit={(event) => { event.preventDefault(); void save(draft); }}>
          <div className="flex flex-col gap-4 rounded-xl border border-amber-500/30 bg-amber-50/70 p-4 dark:bg-amber-500/10 sm:flex-row sm:items-center sm:justify-between">
            <div><div className="text-sm font-bold text-slate-900 dark:text-white">{t("featureEmailMaster")}</div><p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">{t("featureEmailMasterHint")}</p></div>
            <Switch checked={draft.email_enabled} onCheckedChange={(checked) => setDraft((current) => ({ ...current, email_enabled: checked }))} disabled={busy || !canUpdate} />
          </div>
          <div className="flex flex-col gap-4 rounded-xl border border-emerald-500/30 bg-emerald-50/70 p-4 dark:bg-emerald-500/10 sm:flex-row sm:items-center sm:justify-between">
            <div><div className="text-sm font-bold text-slate-900 dark:text-white">{t("featureTOTP")}</div><p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">{t("featureTOTPHint")}</p></div>
            <Switch checked={draft.totp_enabled} onCheckedChange={(checked) => setDraft((current) => ({ ...current, totp_enabled: checked }))} disabled={busy || !canUpdate} />
          </div>
          {draft.totp_enabled ? (
            <section className="space-y-4 rounded-xl border border-emerald-500/25 bg-emerald-50/40 p-4 dark:bg-emerald-500/5">
              <div>
                <div className="text-sm font-bold text-slate-900 dark:text-white">{t("featureStepUpPoliciesTitle")}</div>
                <p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">{t("featureStepUpPoliciesDescription")}</p>
              </div>
              <div className="grid gap-3 lg:grid-cols-2">
                {stepUpPolicies.map((item) => (
                  <div key={item.key} className="flex items-center justify-between gap-4 rounded-xl border border-emerald-500/20 bg-white/80 p-4 dark:border-emerald-500/15 dark:bg-slate-950/30">
                    <div className="min-w-0">
                      <div className="text-sm font-semibold text-slate-900 dark:text-white">{t(item.label)}</div>
                      <p className="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">{t(item.description)}</p>
                    </div>
                    <Switch checked={Boolean(draft[item.key])} onCheckedChange={(checked) => setDraft((current) => ({ ...current, [item.key]: checked }))} disabled={busy || !canUpdate} />
                  </div>
                ))}
              </div>
            </section>
          ) : null}
          <div className="flex flex-col gap-4 rounded-xl border border-indigo-500/30 bg-indigo-50/70 p-4 dark:bg-indigo-500/10 sm:flex-row sm:items-center sm:justify-between">
            <div><div className="text-sm font-bold text-slate-900 dark:text-white">{t("featureModelStatus")}</div><p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">{t("featureModelStatusHint")}</p></div>
            <Switch checked={draft.model_status_enabled} onCheckedChange={(checked) => setDraft((current) => ({ ...current, model_status_enabled: checked }))} disabled={busy || !canUpdate} />
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            {items.map((item) => (
              <div key={item.key} className="flex items-center justify-between gap-4 rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-950/30">
                <div className="min-w-0"><div className="text-sm font-semibold text-slate-900 dark:text-white">{t(item.label)}</div><p className="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">{t(item.description)}</p></div>
                <Switch checked={Boolean(draft[item.key])} onCheckedChange={(checked) => setDraft((current) => ({ ...current, [item.key]: checked }))} disabled={busy || !canUpdate} />
              </div>
            ))}
          </div>
          <div className="grid gap-4 border-t border-slate-200 pt-5 dark:border-slate-800 sm:grid-cols-2">
            <div className="space-y-2"><Label htmlFor="balance-threshold">{t("featureBalanceThreshold")}</Label><Input id="balance-threshold" inputMode="decimal" value={draft.balance_threshold} onChange={(event) => setDraft((current) => ({ ...current, balance_threshold: event.target.value }))} disabled={busy || !canUpdate} /><p className="text-xs text-slate-500 dark:text-slate-400">{t("featureBalanceThresholdHint")}</p></div>
            <div className="space-y-2"><Label htmlFor="recharge-url">{t("featureRechargeURL")}</Label><Input id="recharge-url" type="url" value={draft.recharge_url} onChange={(event) => setDraft((current) => ({ ...current, recharge_url: event.target.value }))} placeholder="https://aokede.com" disabled={busy || !canUpdate} /><p className="text-xs text-slate-500 dark:text-slate-400">{t("featureRechargeURLHint")}</p></div>
          </div>
          {message.text ? <Notice message={message} /> : null}
          <div className="flex justify-end"><Button type="submit" disabled={busy || !canUpdate} className="gap-2"><Save className="h-4 w-4" />{busy ? t("featureSettingsSaving") : t("featureSettingsSave")}</Button></div>
        </form>
      </CardContent>
    </Card>
  );
}

function InputField({ id, label, value, onChange, type = "text", disabled = false, required = false }: { id: string; label: string; value: string; onChange: (value: string) => void; type?: string; disabled?: boolean; required?: boolean }) {
  return <div className="space-y-2"><Label htmlFor={id}>{label}</Label><Input id={id} type={type} value={value} onChange={(event) => onChange(event.target.value)} disabled={disabled} required={required} /></div>;
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
