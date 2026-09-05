import React, { useMemo, useState } from "react";
import {
  Activity,
  Building2,
  CheckCircle2,
  CircleDollarSign,
  ClipboardList,
  Database,
  Gauge,
  KeyRound,
  Layers,
  LayoutDashboard,
  LogOut,
  MonitorCog,
  Network,
  Pause,
  Pencil,
  Play,
  Plus,
  Receipt,
  RefreshCw,
  Search,
  Settings2,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Trash2,
  Users,
  Waypoints,
  WalletCards,
  Zap,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  APIEndpoint,
  APIEndpointFormState,
  AdminSection,
  BillingAccount,
  ChannelSummary,
  ConsoleProfile,
  CreditFormState,
  EmailFormState,
	EmailSettings,
	FeatureSettings,
  EmailTemplate,
  EmailTemplateFormState,
  EnterpriseVerification,
  FinanceReport,
  GroupSummary,
  Language,
  LoginMessage,
  MFAEnrollment,
  MFAStatus,
  PasswordFormState,
  PriceMatrixCostEstimate,
  PriceMatrixSummary,
  PaymentProviderConfig,
  PaymentRechargePackage,
  OperationsSnapshot,
  ModelStatusReport,
  ModelMonitor,
  ModelMonitorFormState,
  AuditReport,
  Principal,
  ProfileFormState,
  SecuritySettings,
  SiteSettings,
  SMTPSettingsForm,
  TokenSummary,
  TranslationKey,
  UsageReport,
  PlatformPermission,
  PlatformRole,
  PlatformRoleFormState,
  UserSummary,
} from "@/types";
import { translations } from "@/locales/translations";
import { cn, formatDecimalWithoutTrailingZeros } from "@/lib/utils";
import { AdminSettingsPanel } from "@/components/admin/AdminSettingsPanel";
import { AdminFinancePanel } from "@/components/admin/AdminFinancePanel";
import { AdminAccountFinancePanel } from "@/components/admin/AdminAccountFinancePanel";
import { AdminUsagePanel } from "@/components/admin/AdminUsagePanel";
import { AdminAuditPanel } from "@/components/admin/AdminAuditPanel";
import { AdminRoleManagementPanel } from "@/components/admin/AdminRoleManagementPanel";
import { AdminModelStatusPanel } from "@/components/admin/AdminModelStatusPanel";
import { AdminEnterprisePanel } from "@/components/admin/AdminEnterprisePanel";

interface AdminConsoleProps {
  language: Language;
  adminSection: AdminSection;
  setAdminSection: (section: AdminSection) => void;
  routeTo: (target: string) => void;
  principal: Principal | null;
  siteName?: string;
  siteLogoURL?: string;
  handleSignOut: () => void;
  channels: ChannelSummary[];
  channelsBusy: boolean;
  channelsMessage: LoginMessage;
  refreshChannels: (showPending?: boolean) => Promise<void>;
  openCreateChannel: (provider?: "openai" | "anthropic" | "grok" | "gemini" | "volcengine") => void;
  openEditChannel: (channel: ChannelSummary) => void;
  syncChannelAccount: (channel: ChannelSummary) => Promise<void>;
  changeChannelStatus: (channel: ChannelSummary, nextStatus: "active" | "disabled") => Promise<void>;
  deleteChannel: (channel: ChannelSummary) => Promise<void>;
  channelDeleteConfirm: string;
  channelActionBusy: string;
  groups: GroupSummary[];
  groupsBusy: boolean;
  groupsMessage: LoginMessage;
  refreshGroups: (showPending?: boolean) => Promise<void>;
  openCreateGroup: () => void;
  openEditGroup: (group: GroupSummary) => void;
  deleteGroup: (group: GroupSummary) => Promise<void>;
  groupDeleteConfirm: string;
  groupActionBusy: string;
  tokens: TokenSummary[];
  tokensBusy: boolean;
  tokensMessage: LoginMessage;
  refreshTokens: (showPending?: boolean) => Promise<void>;
  pauseToken: (token: TokenSummary) => Promise<void>;
  tokenActionBusy: string;
  users: UserSummary[];
  usersBusy: boolean;
  usersMessage: LoginMessage;
  refreshUsers: (showPending?: boolean) => Promise<void>;
  openEditUser: (user: UserSummary) => void;
  updateUserStatus: (user: UserSummary, status: "active" | "locked" | "disabled") => Promise<void>;
  userActionBusy: string;
  platformRoles: PlatformRole[];
  platformPermissions: PlatformPermission[];
  platformRolesBusy: boolean;
  platformRolesMessage: LoginMessage;
  refreshPlatformRoles: (showPending?: boolean) => Promise<void>;
  savePlatformRole: (form: PlatformRoleFormState) => Promise<boolean>;
  disablePlatformRole: (role: PlatformRole) => Promise<void>;
  loadPlatformUserRoles: (userID: string) => Promise<PlatformRole[]>;
  savePlatformUserRoles: (user: UserSummary, roleIDs: string[]) => Promise<boolean>;
  prices: PriceMatrixSummary[];
  billingAccount: BillingAccount | null;
  billingMessage: LoginMessage;
  billingBusy: boolean;
  officialPriceSyncBusy: boolean;
  syncOfficialPrices: () => Promise<void>;
  openEditPrice: (price: PriceMatrixSummary) => void;
  creditForm: CreditFormState;
  setCreditForm: React.Dispatch<React.SetStateAction<CreditFormState>>;
  creditBillingAccount: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  loadBillingAccount: () => Promise<void>;
  securitySettings: SecuritySettings;
  securityMessage: LoginMessage;
  securityBusy: boolean;
  persistSecurity: (nextEnabled: boolean) => Promise<void>;
  adminProfile: ConsoleProfile | null;
  adminProfileForm: ProfileFormState;
  setAdminProfileForm: React.Dispatch<React.SetStateAction<ProfileFormState>>;
  adminEmailForm: EmailFormState;
  setAdminEmailForm: React.Dispatch<React.SetStateAction<EmailFormState>>;
  adminPasswordForm: PasswordFormState;
  setAdminPasswordForm: React.Dispatch<React.SetStateAction<PasswordFormState>>;
  adminProfileBusy: boolean;
  adminProfileMessage: LoginMessage;
  saveAdminProfile: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  saveAdminEmail: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  saveAdminPassword: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  adminMfaStatus: MFAStatus;
  adminMfaEnrollment: MFAEnrollment | null;
  adminMfaCode: string;
  setAdminMfaCode: (value: string) => void;
  adminMfaBusy: boolean;
  beginAdminMFA: () => Promise<void>;
  confirmAdminMFA: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  cancelAdminMFA: () => void;
  disableAdminMFA: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
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
  usageReport: UsageReport | null;
  usageReportBusy: boolean;
  usageReportMessage: LoginMessage;
  refreshUsageReport: (showPending?: boolean, offset?: number) => Promise<void>;
  financeReport: FinanceReport | null;
  financeReportBusy: boolean;
  financeReportMessage: LoginMessage;
  refreshFinanceReport: (showPending?: boolean, offset?: number) => Promise<void>;
  financeSearch: string;
  setFinanceSearch: (value: string) => void;
  financeCurrency: string;
  setFinanceCurrency: (value: string) => void;
  financeFrom: string;
  setFinanceFrom: (value: string) => void;
  financeTo: string;
  setFinanceTo: (value: string) => void;
  reportSearch: string;
  setReportSearch: (value: string) => void;
  reportStatus: string;
  setReportStatus: (value: string) => void;
  reportTenant: string;
  setReportTenant: (value: string) => void;
  reportModel: string;
  setReportModel: (value: string) => void;
  reportGroup: string;
  setReportGroup: (value: string) => void;
  reportFrom: string;
  setReportFrom: (value: string) => void;
  reportTo: string;
  setReportTo: (value: string) => void;
  reportOffset: number;
  operationsSnapshot: OperationsSnapshot | null;
  operationsBusy: boolean;
  refreshOperations: (showPending?: boolean) => Promise<void>;
  adminModelStatusReport: ModelStatusReport | null;
  adminModelStatusBusy: boolean;
  adminModelStatusMessage: LoginMessage;
  refreshAdminModelStatus: (showPending?: boolean) => Promise<void>;
  adminModelMonitors: ModelMonitor[];
  adminModelMonitorsBusy: boolean;
  adminModelMonitorsMessage: LoginMessage;
  adminModelMonitorFormOpen: boolean;
  adminModelMonitorForm: ModelMonitorFormState;
  setAdminModelMonitorForm: React.Dispatch<React.SetStateAction<ModelMonitorFormState>>;
  adminModelMonitorActionBusy: string;
  openCreateAdminModelMonitor: () => void;
  openEditAdminModelMonitor: (monitor: ModelMonitor) => void;
  closeAdminModelMonitorForm: () => void;
  saveAdminModelMonitor: (form: ModelMonitorFormState) => Promise<boolean>;
  deleteAdminModelMonitor: (monitor: ModelMonitor) => Promise<void>;
  probeAdminModelMonitor: (monitor: ModelMonitor) => Promise<void>;
  auditReport: AuditReport | null;
  auditBusy: boolean;
  auditMessage: LoginMessage;
  refreshAudit: (showPending?: boolean, offset?: number) => Promise<void>;
  enterpriseItems: EnterpriseVerification[];
  enterpriseBusy: boolean;
  enterpriseMessage: LoginMessage;
  enterpriseStatus: string;
  setEnterpriseStatus: (value: string) => void;
  refreshEnterprise: () => Promise<void>;
  loadEnterprise: (item: EnterpriseVerification) => Promise<EnterpriseVerification>;
  reviewEnterprise: (item: EnterpriseVerification, status: "approved" | "rejected", reason: string) => Promise<void>;
}

export function AdminConsole({
  language,
  adminSection,
  setAdminSection,
  routeTo,
  principal,
  siteName,
  siteLogoURL,
  handleSignOut,
  channels,
  channelsBusy,
  channelsMessage,
  refreshChannels,
  openCreateChannel,
  openEditChannel,
  syncChannelAccount,
  changeChannelStatus,
  deleteChannel,
  channelDeleteConfirm,
  channelActionBusy,
  groups,
  groupsBusy,
  groupsMessage,
  refreshGroups,
  openCreateGroup,
  openEditGroup,
  deleteGroup,
  groupDeleteConfirm,
  groupActionBusy,
  tokens,
  tokensBusy,
  tokensMessage,
  refreshTokens,
  pauseToken,
  tokenActionBusy,
  users,
  usersBusy,
  usersMessage,
  refreshUsers,
  openEditUser,
  updateUserStatus,
  userActionBusy,
  platformRoles,
  platformPermissions,
  platformRolesBusy,
  platformRolesMessage,
  refreshPlatformRoles,
  savePlatformRole,
  disablePlatformRole,
  loadPlatformUserRoles,
  savePlatformUserRoles,
  prices,
  billingAccount,
  billingMessage,
  billingBusy,
  officialPriceSyncBusy,
  syncOfficialPrices,
  openEditPrice,
  creditForm,
  setCreditForm,
  creditBillingAccount,
  loadBillingAccount,
  securitySettings,
  securityMessage,
  securityBusy,
  persistSecurity,
  adminProfile,
  adminProfileForm,
  setAdminProfileForm,
  adminEmailForm,
  setAdminEmailForm,
  adminPasswordForm,
  setAdminPasswordForm,
  adminProfileBusy,
  adminProfileMessage,
  saveAdminProfile,
  saveAdminEmail,
  saveAdminPassword,
  adminMfaStatus,
  adminMfaEnrollment,
  adminMfaCode,
  setAdminMfaCode,
  adminMfaBusy,
  beginAdminMFA,
  confirmAdminMFA,
  cancelAdminMFA,
  disableAdminMFA,
  siteForm,
  setSiteForm,
  siteBusy,
  siteMessage,
  saveSiteSettings,
  apiEndpoints,
  apiEndpointFormOpen,
  apiEndpointForm,
  setAPIEndpointForm,
  apiEndpointBusy,
  apiEndpointActionBusy,
  apiEndpointMessage,
  openCreateAPIEndpoint,
  openEditAPIEndpoint,
  closeAPIEndpointForm,
  saveAPIEndpoint,
  toggleAPIEndpoint,
  deleteAPIEndpoint,
  smtpForm,
  setSmtpForm,
	emailSettings,
	emailBusy,
	emailMessage,
	emailTestRecipient,
	setEmailTestRecipient,
	smtpConnectionBusy,
	smtpMessageBusy,
	saveEmailSettings,
	testSMTPConnection,
	sendTestEmail,
	featureSettings,
	featureBusy,
	featureMessage,
	saveFeatureSettings,
	emailTemplates,
	emailTemplatesBusy,
	emailTemplatesMessage,
	saveEmailTemplate,
	deleteEmailTemplate,
  paymentConfigs,
  paymentSettingsBusy,
  paymentSettingsMessage,
  savePaymentConfig,
	  paymentRechargePackages,
	  savePaymentRechargePackages,
  canUpdatePaymentSettings,
  usageReport,
  usageReportBusy,
  usageReportMessage,
  refreshUsageReport,
  financeReport,
  financeReportBusy,
  financeReportMessage,
  refreshFinanceReport,
  financeSearch,
  setFinanceSearch,
  financeCurrency,
  setFinanceCurrency,
  financeFrom,
  setFinanceFrom,
  financeTo,
  setFinanceTo,
  reportSearch,
  setReportSearch,
  reportStatus,
  setReportStatus,
  reportTenant,
  setReportTenant,
  reportModel,
  setReportModel,
  reportGroup,
  setReportGroup,
  reportFrom,
  setReportFrom,
  reportTo,
  setReportTo,
  reportOffset,
  operationsSnapshot,
  operationsBusy,
  refreshOperations,
  adminModelStatusReport,
  adminModelStatusBusy,
  adminModelStatusMessage,
  refreshAdminModelStatus,
  adminModelMonitors,
  adminModelMonitorsBusy,
  adminModelMonitorsMessage,
  adminModelMonitorFormOpen,
  adminModelMonitorForm,
  setAdminModelMonitorForm,
  adminModelMonitorActionBusy,
  openCreateAdminModelMonitor,
  openEditAdminModelMonitor,
  closeAdminModelMonitorForm,
  saveAdminModelMonitor,
  deleteAdminModelMonitor,
  probeAdminModelMonitor,
  auditReport,
  auditBusy,
  auditMessage,
  refreshAudit,
  enterpriseItems,
  enterpriseBusy,
  enterpriseMessage,
  enterpriseStatus,
  setEnterpriseStatus,
  refreshEnterprise,
  loadEnterprise,
  reviewEnterprise,
}: AdminConsoleProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [channelSearch, setChannelSearch] = useState("");
  const priceGroups = useMemo(() => groupPriceRows(prices), [prices]);

  const formatTime = (value: string) => {
    if (!value) return t("securityUpdatedUnknown");
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return t("securityUpdatedUnknown");
    return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
  };

  const channelCredentialLabel = (channel: ChannelSummary) => {
    const preview = channel.credential_preview || channel.credential_ref || t("channelsCredentialStored");
    switch (channel.credential_mode) {
      case "env":
        return `${t("channelsModeEnv")}: ${preview}`;
      case "secret":
        return `${t("channelsModeSecret")}: ${preview}`;
      default:
        return `${t("channelsModeExternal")}: ${preview}`;
    }
  };

  const channelOperationalStatusLabel = (channel: ChannelSummary) => {
    const autoDisabled = Boolean(channel.auto_disabled_until && new Date(channel.auto_disabled_until).getTime() > Date.now());
    return !autoDisabled && channel.status === "active"
      ? t("channelsStatusNormal")
      : t("channelsStatusAbnormal");
  };

  const channelOperationalStatusVariant = (channel: ChannelSummary): "success" | "destructive" => {
    const autoDisabled = Boolean(channel.auto_disabled_until && new Date(channel.auto_disabled_until).getTime() > Date.now());
    return !autoDisabled && channel.status === "active" ? "success" : "destructive";
  };

  const upstreamIntegrationLabel = (integration: string) => {
    switch (integration) {
      case "newapi":
        return t("channelsUpstreamIntegrationNewAPI");
      case "sub2api":
        return t("channelsUpstreamIntegrationSub2API");
      case "other":
        return t("channelsUpstreamIntegrationOther");
      default:
        return t("channelsUpstreamIntegrationOfficial");
    }
  };

  const accountSyncStatusLabel = (status: string) => {
    switch (status) {
      case "pending":
        return t("channelsAccountPending");
      case "success":
        return t("channelsAccountSuccess");
      case "failed":
        return t("channelsAccountFailed");
      case "not_supported":
        return t("channelsAccountNotSupported");
      default:
        return t("channelsAccountNotConfigured");
    }
  };

  const accountSyncStatusVariant = (status: string): "success" | "warning" | "muted" | "destructive" => {
    switch (status) {
      case "success":
        return "success";
      case "pending":
        return "warning";
      case "failed":
        return "destructive";
      default:
        return "muted";
    }
  };

  const formatAccountValue = (value?: string) => {
    if (!value) return "-";
    return formatDecimalWithoutTrailingZeros(value, "-");
  };

  const filteredChannels = channels.filter(
    (c) =>
      c.name.toLowerCase().includes(channelSearch.toLowerCase()) ||
      c.provider.toLowerCase().includes(channelSearch.toLowerCase()) ||
      c.base_url.toLowerCase().includes(channelSearch.toLowerCase()) ||
      c.models.some((m) => m.model.toLowerCase().includes(channelSearch.toLowerCase()))
  );

  const navMenuItems: Array<{ group: string; items: Array<{ id: AdminSection; label: string; icon: React.ComponentType<{ className?: string }>; permission: string }> }> = [
    {
      group: language === "zh" ? "核心概览" : "Overview",
      items: [
        { id: "dashboard" as const, label: t("adminNavDashboard"), icon: LayoutDashboard, permission: "operations:read" },
        { id: "ops" as const, label: t("adminNavOps"), icon: MonitorCog, permission: "operations:read" },
        { id: "model-status" as const, label: t("adminNavModelStatus"), icon: Activity, permission: "operations:read" },
      ],
    },
    {
      group: language === "zh" ? "模型与资产" : "Assets & Routing",
      items: [
        { id: "groups" as const, label: t("adminNavGroups"), icon: Layers, permission: "group:read" },
        { id: "tokens" as const, label: t("adminNavTokens"), icon: KeyRound, permission: "token:read" },
        { id: "channels" as const, label: t("adminNavChannels"), icon: Waypoints, permission: "channel:read" },
        { id: "billing" as const, label: t("adminNavBilling"), icon: CircleDollarSign, permission: "price:read" },
      ],
    },
    {
      group: language === "zh" ? "财务与审计" : "Finance & Audit",
      items: [
        { id: "finance" as const, label: t("adminNavFinance"), icon: Receipt, permission: "finance:read" },
        { id: "account-finance" as const, label: t("adminNavAccountFinance"), icon: WalletCards, permission: "finance:read" },
        { id: "usage" as const, label: t("adminNavUsage"), icon: ClipboardList, permission: "usage:read" },
        { id: "audit" as const, label: t("adminNavAudit"), icon: ShieldAlert, permission: "audit:read" },
      ],
    },
    {
      group: language === "zh" ? "组织与安全" : "Governance & Security",
      items: [
        { id: "users" as const, label: t("adminNavUsers"), icon: Users, permission: "user:read" },
        { id: "roles" as const, label: t("adminNavRoles"), icon: ShieldCheck, permission: "role:read" },
        { id: "settings" as const, label: t("adminNavSettings"), icon: Settings2, permission: "security:read" },
        { id: "enterprise" as const, label: t("adminNavEnterprise"), icon: Building2, permission: "enterprise:read" },
      ],
    },
  ];
  const knownPermissions = principal?.permissions;
  const canSeeAdminPermission = (permission: string) => knownPermissions?.includes(permission) === true;
  const currentMenuItem = navMenuItems.flatMap((group) => group.items).find((item) => item.id === adminSection);
  const currentSectionAllowed = currentMenuItem ? canSeeAdminPermission(currentMenuItem.permission) : false;

  return (
    <div className="w-full flex-1 flex flex-col xl:flex-row bg-slate-50/60 dark:bg-slate-950/80 transition-colors duration-200">
      {/* Left Sidebar - Firmly attached to the left edge with full height */}
      <aside className="w-full xl:w-64 xl:shrink-0 border-r border-slate-200/80 dark:border-slate-800/80 bg-white/90 dark:bg-slate-950/95 p-4 text-slate-800 dark:text-slate-100 shadow-sm xl:sticky xl:top-[53px] xl:h-[calc(100vh-53px)] xl:overflow-y-auto flex flex-col justify-between">
        <div className="space-y-6">
          {/* Sidebar Brand / Section Indicator */}
          <div className="rounded-xl border border-indigo-500/20 bg-gradient-to-br from-indigo-50 to-slate-100 dark:from-indigo-950/40 dark:to-slate-900/60 p-3.5 space-y-1">
            <div className="text-[10px] font-bold uppercase tracking-wider text-indigo-600 dark:text-indigo-400 flex items-center gap-1.5">
              <ShieldCheck className="h-3.5 w-3.5" />
              <span>{t("adminEyebrow")}</span>
            </div>
            <div className="flex items-center gap-2 text-sm font-bold text-slate-900 dark:text-white tracking-tight"><span className="relative flex h-5 w-5 items-center justify-center overflow-hidden rounded-md bg-gradient-to-br from-indigo-500 to-cyan-500 text-[8px] text-white">AT{siteLogoURL ? <img src={siteLogoURL} alt="" className="absolute inset-0 h-full w-full bg-white object-contain p-0.5" onError={(event) => { event.currentTarget.style.display = "none"; }} /> : null}</span><span className="truncate">{siteName?.trim() || t("brandName")}</span></div>
            <div className="text-[11px] text-slate-500 dark:text-slate-400 font-mono">{t("adminControlPlane")}</div>
          </div>

          {/* Navigation Groups */}
          <div className="space-y-5">
            {navMenuItems.map((grp) => (
              <div key={grp.group} className="space-y-1">
                <div className="px-3 text-[10px] font-bold uppercase tracking-wider text-slate-400 dark:text-slate-500">
                  {grp.group}
                </div>
                <div className="space-y-1">
                  {grp.items.filter((item) => canSeeAdminPermission(item.permission)).map((item) => {
                    const Icon = item.icon;
                    const active = adminSection === item.id;
                    return (
                      <button
                        key={item.id}
                        type="button"
                        onClick={() => {
                          setAdminSection(item.id);
                          routeTo(`#admin/${item.id}`);
                        }}
                        className={cn(
                          "flex h-10 w-full items-center gap-3 rounded-xl px-3 text-xs font-semibold transition-all duration-150 cursor-pointer select-none",
                          active
                            ? "bg-gradient-to-r from-indigo-600 to-indigo-700 text-white shadow-md shadow-indigo-600/30 font-bold"
                            : "text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-900 hover:text-slate-900 dark:hover:text-white"
                        )}
                      >
                        <Icon className={cn("h-4 w-4 shrink-0", active ? "text-white" : "text-slate-500 dark:text-slate-400")} />
                        <span className="truncate">{item.label}</span>
                        {active && <span className="ml-auto h-1.5 w-1.5 rounded-full bg-white animate-pulse" />}
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Bottom Sidebar Status */}
        <div className="pt-6 border-t border-slate-200/80 dark:border-slate-800/80 space-y-3">
          <Button
            variant="outline"
            size="sm"
            onClick={handleSignOut}
            className="w-full gap-2 text-xs border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-400 hover:text-rose-600 dark:hover:text-rose-300 hover:bg-rose-50 dark:hover:bg-rose-500/10"
          >
            <LogOut className="h-3.5 w-3.5" />
            <span>{t("signOut")}</span>
          </Button>
        </div>
      </aside>

      {/* Right Main Content Area - Full width with clean max constraint */}
      <div className="flex-1 min-w-0 p-4 sm:p-6 lg:p-8 space-y-6 w-full max-w-[1600px]">
        {/* Top Admin Sub-Header */}
        <div className="flex flex-col gap-4 border-b border-slate-200/80 dark:border-slate-800/80 pb-5 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-indigo-600 dark:text-indigo-400">
              <span>{t("adminEyebrow")}</span>
              <span>/</span>
              <span className="text-slate-800 dark:text-slate-200">
                {adminSection === "ops"
                  ? t("adminNavOps")
                  : adminSection === "model-status"
                  ? t("adminNavModelStatus")
                  : adminSection === "users"
                  ? t("adminNavUsers")
                  : adminSection === "roles"
                  ? t("adminNavRoles")
                  : adminSection === "groups"
                  ? t("adminNavGroups")
                  : adminSection === "tokens"
                  ? t("adminNavTokens")
                  : adminSection === "channels"
                  ? t("adminNavChannels")
                : adminSection === "billing"
                  ? t("adminNavBilling")
                : adminSection === "finance"
                  ? t("adminNavFinance")
                : adminSection === "account-finance"
                  ? t("adminNavAccountFinance")
                : adminSection === "usage"
                  ? t("adminNavUsage")
                : adminSection === "audit"
                  ? t("adminNavAudit")
                : adminSection === "enterprise"
                  ? t("adminNavEnterprise")
                : adminSection === "settings"
                  ? t("adminNavSettings")
                  : t("adminNavDashboard")}
              </span>
            </div>
            <h1 className="mt-1 text-2xl sm:text-3xl font-extrabold tracking-tight text-slate-900 dark:text-white">
              {adminSection === "ops"
                ? t("opsTitle")
                : adminSection === "model-status"
                ? t("adminModelStatusTitle")
                : adminSection === "users"
                ? t("usersTitle")
                : adminSection === "roles"
                ? t("rolesTitle")
                : adminSection === "groups"
                ? t("groupsTitle")
                : adminSection === "tokens"
                ? t("tokensTitle")
                : adminSection === "channels"
                ? t("channelsTitle")
                : adminSection === "billing"
                  ? t("billingTitle")
                : adminSection === "finance"
                  ? t("financeTitle")
                : adminSection === "account-finance"
                  ? t("accountFinanceTitle")
                : adminSection === "usage"
                  ? t("usageRecordsTitle")
                : adminSection === "audit"
                  ? t("auditTitle")
                : adminSection === "enterprise"
                  ? t("adminNavEnterprise")
                : adminSection === "settings"
                  ? t("systemSettingsTitle")
                : t("dashboardTitle")}
            </h1>
          </div>

          {/* Current Admin Identity Pill */}
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2.5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/80 px-3.5 py-2 shadow-sm">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-indigo-500/10 dark:bg-indigo-500/20 text-indigo-600 dark:text-indigo-300 font-bold text-xs">
                {principal?.id ? principal.id.charAt(0).toUpperCase() : "A"}
              </div>
              <div className="text-left">
                <div className="text-[10px] uppercase font-bold text-slate-400 dark:text-slate-500">{t("currentUser")}</div>
                <div className="text-xs font-semibold text-slate-800 dark:text-slate-200 font-mono truncate max-w-[140px]">
                  {principal?.id || "-"}
                </div>
              </div>
            </div>
          </div>
        </div>

        {!currentSectionAllowed ? (
          <Card className="glass-panel">
            <CardHeader>
              <CardTitle className="text-xl text-slate-900 dark:text-white">{t("adminAccessDeniedTitle")}</CardTitle>
              <CardDescription>{t("adminAccessDeniedDescription")}</CardDescription>
            </CardHeader>
          </Card>
        ) : <>
        {/* Section 1: Dashboard View */}
        {adminSection === "dashboard" && (
          <div className="space-y-6">
            {/* Platform summary cards */}
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
              {[
                {
                  title: t("statUsersLabel"),
                  hint: t("statUsersHint"),
                  value: operationsBusy ? "..." : operationsSnapshot ? String(operationsSnapshot.users) : "-",
                  delta: operationsSnapshot ? `${operationsSnapshot.active_tokens} ${t("adminMetricActive")}` : "-",
                  icon: Users,
                },
                {
                  title: t("statTenantsLabel"),
                  hint: t("statTenantsHint"),
                  value: operationsBusy ? "..." : operationsSnapshot ? String(operationsSnapshot.tenants) : "-",
                  delta: operationsSnapshot ? `${operationsSnapshot.active_groups} ${t("adminMetricGroups")}` : "-",
                  icon: Network,
                },
                {
                  title: t("statChannelsLabel"),
                  hint: t("statChannelsHint"),
                  value: operationsBusy ? "..." : operationsSnapshot ? String(operationsSnapshot.channels) : "-",
                  delta: operationsSnapshot ? `${operationsSnapshot.active_channels} ${t("adminMetricActive")}` : "-",
                  icon: Waypoints,
                },
                {
                    title: t("statTokensLabel"),
                    hint: t("statTokensHint"),
                    value: operationsBusy ? "..." : operationsSnapshot ? String(operationsSnapshot.tokens) : "-",
                    delta: operationsSnapshot ? `${operationsSnapshot.active_tokens} ${t("adminMetricActive")}` : "-",
                    icon: KeyRound,
                },
                {
                  title: t("statRequestsTodayLabel"),
                  hint: t("statRequestsTodayHint"),
                  value: operationsBusy ? "..." : operationsSnapshot ? String(operationsSnapshot.today_requests) : "-",
                  delta: operationsSnapshot ? `${operationsSnapshot.requests_24h} / ${t("adminMetric24h")}` : "-",
                  icon: Activity,
                },
                {
                  title: t("statSpendTodayLabel"),
                  hint: t("statSpendTodayHint"),
                  value: operationsBusy ? "..." : operationsSnapshot ? formatDecimalWithoutTrailingZeros(operationsSnapshot.today_spend, "0") : "-",
                  delta: operationsSnapshot ? t("adminMetricToday") : "-",
                  icon: CircleDollarSign,
                },
                {
                  title: t("statRechargeTodayLabel"),
                  hint: t("statRechargeTodayHint"),
                  value: operationsBusy ? "..." : operationsSnapshot ? formatDecimalWithoutTrailingZeros(operationsSnapshot.today_recharge_amount, "0") : "-",
                  delta: operationsSnapshot ? `${operationsSnapshot.today_recharge_orders} ${t("adminMetricOrders")}` : "-",
                  icon: WalletCards,
                },
                {
                  title: t("statTokensTodayLabel"),
                  hint: t("statTokensTodayHint"),
                  value: operationsBusy ? "..." : operationsSnapshot ? String(operationsSnapshot.today_tokens) : "-",
                  delta: operationsSnapshot ? `${operationsSnapshot.today_input_tokens} / ${operationsSnapshot.today_output_tokens}` : "-",
                  icon: Gauge,
                },
                {
                  title: t("statTokensTotalLabel"),
                  hint: t("statTokensTotalHint"),
                  value: operationsBusy ? "..." : operationsSnapshot ? String(operationsSnapshot.total_tokens) : "-",
                  delta: operationsSnapshot ? t("adminMetricAllTime") : "-",
                  icon: Network,
                },
              ].map((stat) => (
                <Card key={stat.title} className="glass-panel relative overflow-hidden">
                  <CardHeader className="space-y-2 pb-2">
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-semibold text-slate-500 dark:text-slate-400">{stat.title}</span>
                      <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border border-indigo-500/20">
                        <stat.icon className="h-4 w-4" />
                      </div>
                    </div>
                    <div className="text-3xl font-extrabold text-slate-900 dark:text-white font-mono">{stat.value}</div>
                  </CardHeader>
                  <CardContent className="pt-0">
                    <div className="flex items-center justify-between text-xs">
                      <span className="text-slate-500 dark:text-slate-400 truncate max-w-[140px]">{stat.hint}</span>
                      <Badge variant="success" className="text-[10px] font-mono">{stat.delta}</Badge>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>

            {/* 2 Big Detail Panels */}
            <div className="grid gap-6 xl:grid-cols-2">
              <Card className="glass-panel">
                <CardHeader className="pb-3">
                  <CardTitle className="text-lg font-bold text-slate-900 dark:text-white flex items-center gap-2">
                    <Activity className="h-4 w-4 text-indigo-600 dark:text-indigo-400" />
                    <span>{t("recentActivityTitle")}</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  {[
                    { text: `${t("statUsersLabel")}: ${operationsSnapshot?.users ?? "-"}`, tag: "DB", time: operationsSnapshot ? formatTime(operationsSnapshot.collected_at) : "-" },
                    { text: `${t("statChannelsLabel")}: ${operationsSnapshot?.active_channels ?? "-"} / ${operationsSnapshot?.channels ?? "-"}`, tag: t("adminMetricChannels"), time: operationsSnapshot ? formatTime(operationsSnapshot.collected_at) : "-" },
                    { text: `${t("statAlertsLabel")}: ${operationsSnapshot?.failed_requests_24h ?? "-"}`, tag: t("adminMetric24h"), time: operationsSnapshot ? formatTime(operationsSnapshot.collected_at) : "-" },
                  ].map((item, idx) => (
                    <div
                      key={idx}
                      className="flex items-center justify-between rounded-xl border border-slate-200 dark:border-slate-800/80 bg-slate-50/80 dark:bg-slate-950/40 p-3 text-xs"
                    >
                      <div className="flex items-center gap-3">
                        <span className={cn("h-2 w-2 rounded-full", operationsSnapshot ? "bg-emerald-500" : "bg-slate-400")} />
                        <span className="text-slate-800 dark:text-slate-200 font-medium">{item.text}</span>
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant="secondary" className="text-[10px] font-mono">{item.tag}</Badge>
                        <span className="text-slate-400 dark:text-slate-500">{item.time}</span>
                      </div>
                    </div>
                  ))}
                </CardContent>
              </Card>

              <Card className="glass-panel">
                <CardHeader className="pb-3">
                  <CardTitle className="text-lg font-bold text-slate-900 dark:text-white flex items-center gap-2">
                    <Zap className="h-4 w-4 text-cyan-600 dark:text-cyan-400" />
                    <span>{t("pendingWorkTitle")}</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  {[
                    { text: `${t("statTokensLabel")}: ${operationsSnapshot?.tokens ?? "-"}`, status: operationsSnapshot ? `${operationsSnapshot.active_tokens} ${t("adminMetricActive")}` : "-" },
                    { text: `${t("statGroupsLabel")}: ${operationsSnapshot?.groups ?? "-"}`, status: operationsSnapshot ? `${operationsSnapshot.active_groups} ${t("adminMetricActive")}` : "-" },
                    { text: `${t("statRequestsLabel")}: ${operationsSnapshot?.requests_24h ?? "-"}`, status: operationsSnapshot ? `${operationsSnapshot.average_latency_ms.toFixed(0)}ms ${t("adminMetricLatency")}` : "-" },
                  ].map((item, idx) => (
                    <div
                      key={idx}
                      className="flex items-center justify-between rounded-xl border border-slate-200 dark:border-slate-800/80 bg-slate-50/80 dark:bg-slate-950/40 p-3 text-xs"
                    >
                      <div className="flex items-center gap-2.5">
                        <CheckCircle2 className="h-4 w-4 text-indigo-600 dark:text-indigo-400 shrink-0" />
                        <span className="text-slate-700 dark:text-slate-300">{item.text}</span>
                      </div>
                      <Badge variant="secondary" className="text-[10px] text-slate-500 dark:text-slate-400">{item.status}</Badge>
                    </div>
                  ))}
                </CardContent>
              </Card>
	            </div>
	            <AdminDashboardInsights t={t} snapshot={operationsSnapshot} busy={operationsBusy} formatTime={formatTime} routeTo={routeTo} />
	          </div>
	        )}

        {/* Section 2: Ops Monitoring View */}
        {adminSection === "ops" && (
          <div className="space-y-6">
            <div className="flex items-center justify-between rounded-xl border border-slate-200 bg-white/70 px-4 py-3 dark:border-slate-800 dark:bg-slate-900/60">
              <div className="text-xs text-slate-500 dark:text-slate-400">{operationsSnapshot ? `${t("statRequestsLabel")} · ${formatTime(operationsSnapshot.collected_at)}` : t("dashboardTitle")}</div>
              <Button type="button" variant="outline" size="sm" onClick={() => void refreshOperations(true)} disabled={operationsBusy} className="gap-2"><RefreshCw className={cn("h-3.5 w-3.5", operationsBusy ? "animate-spin" : "")} />{t("channelsRefresh")}</Button>
            </div>
            <div className="grid gap-6 md:grid-cols-3">
              <Card className="glass-panel space-y-2">
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-base text-slate-900 dark:text-white">{t("monitorApiLabel")}</CardTitle>
                    <Badge variant={operationsSnapshot ? "success" : "muted"}>{operationsSnapshot ? t("monitorHealthy") : "-"}</Badge>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed">{t("monitorApiBody")}</p>
                    <div className="rounded-lg bg-slate-100 dark:bg-slate-950/60 p-2.5 font-mono text-[11px] text-emerald-600 dark:text-emerald-400 flex items-center justify-between">
                    <span>{operationsSnapshot ? "HTTP 200 OK" : "-"}</span>
                    <span>{operationsSnapshot ? `${operationsSnapshot.average_latency_ms.toFixed(0)}ms avg` : "-"}</span>
                  </div>
                </CardContent>
              </Card>

              <Card className="glass-panel space-y-2">
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-base text-slate-900 dark:text-white">{t("monitorLoginLabel")}</CardTitle>
                    <Badge variant="cyan">{t("monitorPending")}</Badge>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed">{t("monitorLoginBody")}</p>
                    <div className="rounded-lg bg-slate-100 dark:bg-slate-950/60 p-2.5 font-mono text-[11px] text-cyan-600 dark:text-cyan-400 flex items-center justify-between">
                    <span>{operationsSnapshot ? `${operationsSnapshot.active_tokens} ${t("adminMetricActive")} tokens` : "-"}</span>
                    <span>{operationsSnapshot ? `${operationsSnapshot.failed_requests_24h} failed / ${t("adminMetric24h")}` : "-"}</span>
                  </div>
                </CardContent>
              </Card>

              <Card className="glass-panel space-y-2">
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-base text-slate-900 dark:text-white">{t("monitorRelayLabel")}</CardTitle>
                    <Badge variant={operationsSnapshot && operationsSnapshot.active_channels > 0 ? "success" : "muted"}>{operationsSnapshot && operationsSnapshot.active_channels > 0 ? t("monitorStub") : "-"}</Badge>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed">{t("monitorRelayBody")}</p>
                    <div className="rounded-lg bg-slate-100 dark:bg-slate-950/60 p-2.5 font-mono text-[11px] text-indigo-600 dark:text-indigo-300 flex items-center justify-between">
                    <span>{operationsSnapshot ? `${operationsSnapshot.active_channels} / ${operationsSnapshot.channels} ${t("adminMetricChannels")}` : "-"}</span>
                    <span>{operationsSnapshot ? `${operationsSnapshot.requests_24h} requests / ${t("adminMetric24h")}` : "-"}</span>
                  </div>
                </CardContent>
              </Card>
            </div>
          </div>
        )}

        {adminSection === "model-status" && (
          <AdminModelStatusPanel
            language={language}
            report={adminModelStatusReport}
            busy={adminModelStatusBusy}
            message={adminModelStatusMessage}
            refresh={refreshAdminModelStatus}
            groups={groups}
            monitors={adminModelMonitors}
            monitorsBusy={adminModelMonitorsBusy}
            monitorsMessage={adminModelMonitorsMessage}
            formOpen={adminModelMonitorFormOpen}
            form={adminModelMonitorForm}
            setForm={setAdminModelMonitorForm}
            actionBusy={adminModelMonitorActionBusy}
            openCreate={openCreateAdminModelMonitor}
            openEdit={openEditAdminModelMonitor}
            closeForm={closeAdminModelMonitorForm}
            save={saveAdminModelMonitor}
            remove={deleteAdminModelMonitor}
            probe={probeAdminModelMonitor}
          />
        )}

        {adminSection === "enterprise" && (
          <AdminEnterprisePanel
            language={language}
            items={enterpriseItems}
            busy={enterpriseBusy}
            message={enterpriseMessage}
            status={enterpriseStatus}
            setStatus={setEnterpriseStatus}
            refresh={refreshEnterprise}
            loadDetails={loadEnterprise}
            review={reviewEnterprise}
          />
        )}

        {/* Section 3: Users & Tenants View */}
        {adminSection === "users" && (
          <Card className="glass-panel space-y-4">
            <CardHeader className="space-y-4 pb-2">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div><CardTitle className="text-xl font-bold text-slate-900 dark:text-white">{t("usersTableTitle")}</CardTitle><CardDescription>{t("usersTableHint")}</CardDescription></div>
                <div className="flex flex-wrap items-center gap-2.5">
                  <Button variant="outline" size="sm" onClick={() => void refreshUsers(true)} disabled={usersBusy || Boolean(userActionBusy)} className="h-9 gap-1.5 text-xs"><RefreshCw className={cn("h-3.5 w-3.5", usersBusy ? "animate-spin" : "")} />{t("usersRefresh")}</Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {usersMessage.text ? <div className={cn("rounded-xl border p-3 text-xs", usersMessage.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : "border-indigo-500/30 bg-indigo-50 text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300")}>{usersMessage.text}</div> : null}
              <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
                <Table>
                  <TableHeader><TableRow><TableHead>{t("usersColName")}</TableHead><TableHead>{t("usersColRole")}</TableHead><TableHead>{t("usersColTenant")}</TableHead><TableHead>{t("usersColStatus")}</TableHead><TableHead>{t("usersColCreated")}</TableHead><TableHead className="text-right">{t("usersColAction")}</TableHead></TableRow></TableHeader>
                  <TableBody>
                    {usersBusy && users.length === 0 ? <TableRow><TableCell colSpan={6} className="py-12 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin text-indigo-600" />{t("usersLoading")}</TableCell></TableRow> : users.length === 0 ? <TableRow><TableCell colSpan={6} className="py-12 text-center text-sm text-slate-500">{t("usersEmpty")}</TableCell></TableRow> : users.map((user) => {
                      const isActive = user.status === "active";
                      const isSelf = user.id === principal?.id;
                      return <TableRow key={user.id}>
                        <TableCell className="min-w-[220px]"><div className="flex items-center gap-3"><div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-indigo-500/10 text-xs font-bold uppercase text-indigo-600 dark:bg-indigo-500/20 dark:text-indigo-300">{(user.display_name || user.email).slice(0, 2)}</div><div><div className="font-semibold text-slate-900 dark:text-white">{user.display_name || t("usersUnnamed")}</div><div className="text-xs text-slate-500 dark:text-slate-400">{user.email}</div><div className="mt-1 font-mono text-[10px] text-slate-400">{user.id}</div></div></div></TableCell>
                        <TableCell><div className="flex max-w-[190px] flex-wrap gap-1">{user.platform_roles.length > 0 ? user.platform_roles.map((role) => <Badge key={role} variant="purple">{role}</Badge>) : <Badge variant="cyan">{t("usersTenantMember")}</Badge>}</div></TableCell>
                        <TableCell className="min-w-[230px]"><div className="text-sm text-slate-700 dark:text-slate-300">{user.tenant_count} {t("usersTenantUnit")}</div><div className="mt-1 max-w-[230px] truncate text-xs text-slate-500" title={user.tenant_names.join(", ")}>{user.tenant_names.join(", ") || "-"}</div>{user.tenant_ids.length > 0 ? <div className="mt-1.5 space-y-0.5"><div className="text-[10px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">{t("usersTenantID")}</div>{user.tenant_ids.map((tenantID) => <div key={tenantID} className="max-w-[230px] break-all font-mono text-[10px] leading-4 text-indigo-600 dark:text-indigo-300" title={`${t("usersTenantID")}: ${tenantID}`}>{tenantID}</div>)}</div> : null}</TableCell>
                        <TableCell><Badge variant={isActive ? "success" : user.status === "locked" ? "warning" : "muted"}>{isActive ? t("usersStatusActive") : user.status === "locked" ? t("usersStatusLocked") : user.status === "pending" ? t("usersStatusPending") : t("usersStatusDisabled")}</Badge></TableCell>
                        <TableCell className="whitespace-nowrap text-xs text-slate-500">{formatTime(user.created_at)}</TableCell>
                         <TableCell className="whitespace-nowrap text-right"><div className="flex items-center justify-end gap-2"><Button variant="ghost" size="icon" onClick={() => openEditUser(user)} disabled={!canSeeAdminPermission("user:update") || isSelf || Boolean(userActionBusy)} aria-label={isSelf ? t("usersSelfDisabled") : t("usersEdit")} title={isSelf ? t("usersSelfDisabled") : t("usersEdit")} className="h-9 w-9 text-slate-600 hover:text-indigo-600 dark:text-slate-300 dark:hover:text-indigo-300"><Pencil className="h-4 w-4" /></Button><select value={user.status} onChange={(event) => void updateUserStatus(user, event.target.value as "active" | "locked" | "disabled")} disabled={!canSeeAdminPermission("user:update") || isSelf || Boolean(userActionBusy)} aria-label={t("usersChangeStatus")} title={isSelf ? t("usersSelfDisabled") : undefined} className="h-9 min-w-[112px] rounded-lg border border-slate-200 bg-white px-2 text-xs text-slate-800 outline-none focus:border-indigo-500 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200"><option value="active">{t("usersStatusActive")}</option><option value="locked">{t("usersStatusLocked")}</option><option value="disabled">{t("usersStatusDisabled")}</option>{user.status === "pending" ? <option value="pending" disabled>{t("usersStatusPending")}</option> : null}</select></div></TableCell>
                      </TableRow>;
                    })}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        )}

        {adminSection === "roles" && (
          <Card className="glass-panel space-y-4">
            <CardHeader>
              <CardTitle className="text-xl font-bold text-slate-900 dark:text-white">{t("rolesTitle")}</CardTitle>
              <CardDescription>{t("rolesDescription")}</CardDescription>
            </CardHeader>
            <CardContent>
              <AdminRoleManagementPanel
                language={language}
                currentUserID={principal?.id}
                users={users}
                roles={platformRoles}
                permissions={platformPermissions}
                busy={platformRolesBusy}
                message={platformRolesMessage}
                refresh={refreshPlatformRoles}
                saveRole={savePlatformRole}
                disableRole={disablePlatformRole}
                loadUserRoles={loadPlatformUserRoles}
                saveUserRoles={savePlatformUserRoles}
                canManage={canSeeAdminPermission("role:update") && principal?.roles?.includes("platform_owner") === true}
              />
            </CardContent>
          </Card>
        )}

        {/* Section 4: Routing Groups Management */}
        {adminSection === "groups" && (
          <Card className="glass-panel space-y-4">
            <CardHeader className="space-y-4 pb-2">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <CardTitle className="flex items-center gap-2 text-xl font-bold text-slate-900 dark:text-white">
                    <Layers className="h-5 w-5 text-indigo-600 dark:text-indigo-400" />
                    <span>{t("groupsTableTitle")}</span>
                  </CardTitle>
                  <CardDescription>{t("groupsTableHint")}</CardDescription>
                </div>
                <div className="flex flex-wrap items-center gap-2.5">
                  <Button variant="outline" size="sm" onClick={() => refreshGroups(true)} disabled={groupsBusy || Boolean(groupActionBusy)} className="h-9 gap-1.5 text-xs">
                    <RefreshCw className={cn("h-3.5 w-3.5", groupsBusy ? "animate-spin" : "")} />
                    <span>{t("groupsRefresh")}</span>
                  </Button>
                  <Button size="sm" onClick={openCreateGroup} disabled={!canSeeAdminPermission("group:update") || Boolean(groupActionBusy)} className="h-9 gap-1.5 text-xs shadow-lg shadow-indigo-500/20">
                    <Plus className="h-3.5 w-3.5" />
                    <span>{t("groupsNew")}</span>
                  </Button>
                </div>
              </div>
            </CardHeader>

            <CardContent className="space-y-4">
              {groupsMessage.text ? (
                <div className={cn("flex items-center gap-2 rounded-xl border p-3 text-xs", groupsMessage.kind === "success" ? "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300" : groupsMessage.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300")}>
                  <Layers className="h-4 w-4 shrink-0" />
                  <span>{groupsMessage.text}</span>
                </div>
              ) : null}

              <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("groupsName")}</TableHead>
                      <TableHead>{t("groupsChannels")}</TableHead>
                      <TableHead>{t("groupsMultiplier")}</TableHead>
                      <TableHead>{t("groupsRPM")}</TableHead>
                      <TableHead>{t("groupsBillingType")}</TableHead>
                      <TableHead>{t("groupsMeteringMode")}</TableHead>
                      <TableHead>{t("groupsMeteringPriceShort")}</TableHead>
                      <TableHead>{t("groupsStatus")}</TableHead>
                      <TableHead className="text-right">{t("groupsActions")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {groupsBusy && groups.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={9} className="py-12 text-center text-sm text-slate-500 dark:text-slate-400">
                          <RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin text-indigo-600 dark:text-indigo-400" />
                          <span>{t("groupsLoading")}</span>
                        </TableCell>
                      </TableRow>
                    ) : groups.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={9} className="py-12 text-center text-sm text-slate-500 dark:text-slate-400">{t("groupsEmpty")}</TableCell>
                      </TableRow>
                    ) : (
                      groups.map((group) => (
                        <TableRow key={group.id}>
                          <TableCell className="min-w-[220px]">
                            <div className="flex items-start gap-3">
                              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-indigo-500/10 text-indigo-600 dark:bg-indigo-500/20 dark:text-indigo-300">
                                <Layers className="h-4 w-4" />
                              </div>
                              <div className="min-w-0">
                                <div className="font-semibold text-slate-900 dark:text-white">{group.name}</div>
                                <div className="mt-0.5 font-mono text-[11px] text-indigo-600 dark:text-indigo-300">{group.code}</div>
                                {group.description ? <div className="mt-1 max-w-[260px] truncate text-xs text-slate-500 dark:text-slate-400">{group.description}</div> : null}
                              </div>
                            </div>
                          </TableCell>
                          <TableCell className="min-w-[250px]">
                            {group.channels.length === 0 ? (
                              <span className="text-xs text-slate-400 dark:text-slate-500">{t("groupsNoChannelsSelected")}</span>
                            ) : (
                              <div className="flex max-w-[320px] flex-wrap gap-1">
                                {group.channels.slice(0, 3).map((channel) => <span key={channel.id} className="rounded-md border border-slate-200 bg-slate-100 px-2 py-0.5 text-[11px] text-slate-700 dark:border-slate-700/80 dark:bg-slate-800/90 dark:text-slate-300">{channel.name}</span>)}
                                {group.channels.length > 3 ? <span className="rounded-md border border-slate-200 bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-500 dark:border-slate-700/80 dark:bg-slate-800 dark:text-slate-400">+{group.channels.length - 3}</span> : null}
                              </div>
                            )}
                          </TableCell>
                          <TableCell><span className="font-mono text-sm font-semibold text-indigo-600 dark:text-indigo-300">{group.multiplier}x</span></TableCell>
                          <TableCell><span className="font-mono text-xs text-slate-700 dark:text-slate-300">{group.rpm_limit === 0 ? t("groupsRPMUnlimited") : group.rpm_limit}</span></TableCell>
                          <TableCell><Badge variant={group.billing_type === "free" ? "muted" : "success"}>{group.billing_type === "free" ? t("groupsBillingFree") : t("groupsBillingPrepaid")}</Badge></TableCell>
                          <TableCell><Badge variant="cyan">{t(group.metering_mode === "image_count" ? "groupsMeteringImageCount" : group.metering_mode === "video_seconds" ? "groupsMeteringVideoSeconds" : group.metering_mode === "video_request" ? "groupsMeteringVideoRequest" : "groupsMeteringToken")}</Badge></TableCell>
                          <TableCell className="whitespace-nowrap font-mono text-xs text-slate-700 dark:text-slate-300">{group.metering_mode === "token" ? "-" : `USD ${group.metering_price || "-"}`}</TableCell>
                          <TableCell><Badge variant={group.status === "active" ? "success" : "muted"}>{group.status === "active" ? t("groupsStatusActive") : t("groupsStatusDisabled")}</Badge></TableCell>
                          <TableCell className="whitespace-nowrap text-right">
                            <div className="inline-flex items-center gap-1">
                              <Button variant="ghost" size="sm" onClick={() => openEditGroup(group)} disabled={!canSeeAdminPermission("group:update") || Boolean(groupActionBusy)} className="h-8 px-2.5 text-xs text-slate-700 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white">
                                <Pencil className="mr-1 h-3.5 w-3.5" />
                                <span>{t("groupsEdit")}</span>
                              </Button>
                              <Button variant={groupDeleteConfirm === group.id ? "destructive" : "ghost"} size="sm" onClick={() => deleteGroup(group)} disabled={!canSeeAdminPermission("group:update") || Boolean(groupActionBusy)} className="h-8 px-2 text-xs text-rose-600 dark:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-500/10" title={groupDeleteConfirm === group.id ? t("groupsDeleteConfirm") : t("groupsDelete")}>
                                <Trash2 className="h-3.5 w-3.5" />
                                <span className="sr-only">{t("groupsDelete")}</span>
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Section 5: Token Group Assignment */}
        {adminSection === "tokens" && (
          <Card className="glass-panel space-y-4">
            <CardHeader className="space-y-4 pb-2">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <CardTitle className="flex items-center gap-2 text-xl font-bold text-slate-900 dark:text-white">
                    <KeyRound className="h-5 w-5 text-indigo-600 dark:text-indigo-400" />
                    <span>{t("tokensTableTitle")}</span>
                  </CardTitle>
                  <CardDescription>{t("tokensTableHint")}</CardDescription>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => refreshTokens(true)} disabled={tokensBusy || Boolean(tokenActionBusy)} className="h-9 gap-1.5 text-xs">
                    <RefreshCw className={cn("h-3.5 w-3.5", tokensBusy ? "animate-spin" : "")} />
                    <span>{t("tokensRefresh")}</span>
                  </Button>
                </div>
              </div>
            </CardHeader>

            <CardContent className="space-y-4">
              {tokensMessage.text ? (
                <div className={cn("flex items-center gap-2 rounded-xl border p-3 text-xs", tokensMessage.kind === "success" ? "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300" : tokensMessage.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300")}>
                  <KeyRound className="h-4 w-4 shrink-0" />
                  <span>{tokensMessage.text}</span>
                </div>
              ) : null}

              <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("tokensName")}</TableHead>
                      <TableHead>{t("tokensPrefix")}</TableHead>
                      <TableHead>{t("tokensTenant")}</TableHead>
                      <TableHead>{t("tokensGroup")}</TableHead>
                      <TableHead>{t("tokensStatus")}</TableHead>
                      <TableHead>{t("tokensCreated")}</TableHead>
                      <TableHead className="text-right">{t("tokensActions")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tokensBusy && tokens.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={7} className="py-12 text-center text-sm text-slate-500 dark:text-slate-400">
                          <RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin text-indigo-600 dark:text-indigo-400" />
                          <span>{t("tokensLoading")}</span>
                        </TableCell>
                      </TableRow>
                    ) : tokens.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={7} className="py-12 text-center text-sm text-slate-500 dark:text-slate-400">{t("tokensEmpty")}</TableCell>
                      </TableRow>
                    ) : (
                      tokens.map((token) => (
                        <TableRow key={token.id}>
                          <TableCell className="min-w-[170px]">
                            <div className="font-semibold text-slate-900 dark:text-white">{token.name}</div>
                            <div className="mt-1 flex flex-wrap items-center gap-2"><span className="font-mono text-[10px] text-slate-500 dark:text-slate-400">{token.id}</span>{token.network_allowlist_enabled ? <Badge variant="cyan">{t("tokensNetworkProtected")}</Badge> : null}</div>
                          </TableCell>
                          <TableCell><code className="rounded-md bg-slate-100 px-2 py-1 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300">{token.token_prefix}...</code></TableCell>
                          <TableCell className="min-w-[220px]">
                            <div className="font-mono text-xs text-slate-700 dark:text-slate-300">{token.tenant_id}</div>
                            <div className="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-400">{token.project_id}</div>
                          </TableCell>
                          <TableCell className="min-w-[170px]">
                            <div className="font-medium text-slate-800 dark:text-slate-200">{token.group_code || t("tokensNoGroup")}</div>
                            <div className="mt-1 text-[10px] text-slate-500">{t("tokensAdminReadOnly")}</div>
                          </TableCell>
                          <TableCell><Badge variant={token.status === "active" ? "success" : token.status === "disabled" || token.status === "expired" ? "warning" : "muted"}>{token.status === "active" ? t("tokensStatusActive") : token.status === "disabled" ? t("tokensStatusPaused") : token.status === "expired" ? t("tokensStatusExpired") : t("tokensStatusRevoked")}</Badge></TableCell>
                          <TableCell className="whitespace-nowrap text-xs text-slate-500 dark:text-slate-400">{formatTime(token.created_at)}</TableCell>
                          <TableCell className="text-right">
                            {token.status === "active" ? <Button variant="ghost" size="sm" onClick={() => void pauseToken(token)} disabled={!canSeeAdminPermission("token:pause") || Boolean(tokenActionBusy)} className="gap-1.5 text-xs text-amber-700 hover:text-amber-800 dark:text-amber-300"><Pause className="h-3.5 w-3.5" />{t("tokensPause")}</Button> : <span className="text-xs text-slate-400">{t("tokensAdminNoAction")}</span>}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Section 6: Upstream Channels Management */}
        {adminSection === "channels" && (
          <Card className="glass-panel space-y-4">
            <CardHeader className="space-y-4 pb-2">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <CardTitle className="text-xl font-bold text-slate-900 dark:text-white">{t("channelsTitle")}</CardTitle>
                  <CardDescription>{t("channelsTableHint")}</CardDescription>
                </div>
                <div className="flex flex-wrap items-center gap-2.5">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => refreshChannels(true)}
                    disabled={channelsBusy || Boolean(channelActionBusy)}
                    className="h-9 gap-1.5 text-xs"
                  >
                    <RefreshCw className={cn("h-3.5 w-3.5", channelsBusy ? "animate-spin" : "")} />
                    <span>{t("channelsRefresh")}</span>
                  </Button>
                  <Button
                    size="sm"
                    onClick={() => openCreateChannel()}
                    disabled={!canSeeAdminPermission("channel:update") || Boolean(channelActionBusy)}
                    className="h-9 gap-1.5 text-xs shadow-lg shadow-indigo-500/20"
                  >
                    <Plus className="h-4 w-4" />
                    <span>{t("channelsNew")}</span>
                  </Button>
                </div>
              </div>

              {/* Search Bar */}
              <div className="relative max-w-md">
                <Search className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" />
                <Input
                  placeholder={language === "zh" ? "搜索渠道名称、供应商、模型或 Base URL..." : "Filter channels..."}
                  value={channelSearch}
                  onChange={(e) => setChannelSearch(e.target.value)}
                  className="h-9 pl-9 text-xs"
                />
              </div>
            </CardHeader>

            <CardContent className="space-y-4">
              {/* Status Message */}
              {channelsMessage.text && (
                <div
                  className={cn("rounded-xl border p-3 text-xs flex items-center gap-2", {
                    "border-emerald-500/30 bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300": channelsMessage.kind === "success",
                    "border-amber-500/30 bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300": channelsMessage.kind === "pending",
                    "border-rose-500/30 bg-rose-50 dark:bg-rose-500/10 text-rose-700 dark:text-rose-300": channelsMessage.kind === "error",
                  })}
                >
                  <Sparkles className="h-4 w-4 shrink-0" />
                  <span>{channelsMessage.text}</span>
                </div>
              )}

              <div className="rounded-xl border border-slate-200 dark:border-slate-800 overflow-x-auto">
                <Table className="min-w-[1680px] whitespace-nowrap">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="whitespace-nowrap">{t("channelsColName")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("channelsColProvider")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("channelsColUpstreamIntegration")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("channelsColStatus")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("channelsColPriority")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("channelsColCostDiscount")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("channelsColAccount")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("channelsColModels")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("channelsColCredential")}</TableHead>
                      <TableHead className="whitespace-nowrap text-right">{t("channelsColAction")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {channelsBusy && channels.length === 0 ? (
                      <TableRow>
                          <TableCell colSpan={10} className="py-12 text-center text-sm text-slate-500 dark:text-slate-400">
                          <RefreshCw className="h-5 w-5 animate-spin mx-auto mb-2 text-indigo-600 dark:text-indigo-400" />
                          <span>{t("channelsLoading")}</span>
                        </TableCell>
                      </TableRow>
                    ) : filteredChannels.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={10} className="py-12 text-center text-sm text-slate-500 dark:text-slate-400">
                          {t("channelsEmpty")}
                        </TableCell>
                      </TableRow>
                    ) : (
                      filteredChannels.map((channel) => {
                        return (
                        <TableRow key={channel.id}>
                          {/* Name & URL */}
                          <TableCell className="min-w-[220px] whitespace-nowrap">
                            <div className="font-bold text-slate-900 dark:text-white tracking-tight">{channel.name}</div>
                            <div className="mt-0.5 max-w-[260px] truncate text-xs font-mono text-slate-500 dark:text-slate-400">
                              {channel.base_url}
                            </div>
                          </TableCell>

                          {/* Provider */}
                          <TableCell className="whitespace-nowrap">
                            <Badge
                              variant={channel.provider === "anthropic" ? "purple" : "cyan"}
                              className="font-mono text-[10px] uppercase font-bold"
                            >
                              {channel.provider}
                            </Badge>
                          </TableCell>

                          {/* Upstream integration */}
                          <TableCell className="min-w-[135px] whitespace-nowrap">
                            <Badge variant={channel.upstream_integration === "official" ? "cyan" : "purple"} className="text-[10px]">
                              {upstreamIntegrationLabel(channel.upstream_integration)}
                            </Badge>
                          </TableCell>

                          {/* Status */}
                          <TableCell className="whitespace-nowrap">
                            <Badge variant={channelOperationalStatusVariant(channel)}>
                              {channelOperationalStatusLabel(channel)}
                            </Badge>
                          </TableCell>

                          {/* Priority & Weight */}
                          <TableCell className="min-w-[130px] whitespace-nowrap">
                            <div className="text-xs font-semibold text-slate-700 dark:text-slate-200">
                              Priority: <span className="font-mono text-indigo-600 dark:text-indigo-300">{channel.priority}</span>
                            </div>
                            <div className="text-[11px] text-slate-500 dark:text-slate-400">
                              Weight: <span className="font-mono">{channel.weight}</span>
                            </div>
                          </TableCell>

                          {/* Upstream cost factor */}
                          <TableCell className="min-w-[125px] whitespace-nowrap">
                            <div className="font-mono text-xs font-semibold text-cyan-700 dark:text-cyan-300">
                              x{formatMultiplier(channel.upstream_cost_discount || "1.000000")}
                            </div>
                            <div className="mt-1 text-[10px] text-slate-500 dark:text-slate-400">
                              {t("channelsCostDiscountHint")}
                            </div>
                          </TableCell>

                          {/* Optional upstream account snapshot */}
                          <TableCell className="min-w-[260px] whitespace-nowrap">
                            <div className="flex items-center gap-2">
                              <Badge variant={accountSyncStatusVariant(channel.upstream_account_sync_status)} className="text-[10px]">
                                {accountSyncStatusLabel(channel.upstream_account_sync_status)}
                              </Badge>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-7 gap-1 px-2 text-[11px] text-cyan-700 hover:bg-cyan-50 dark:text-cyan-300 dark:hover:bg-cyan-500/10"
                                onClick={() => void syncChannelAccount(channel)}
                                disabled={!canSeeAdminPermission("channel:read") || Boolean(channelActionBusy)}
                                title={t("channelsAccountRefresh")}
                              >
                                <RefreshCw className={cn("h-3 w-3", channelActionBusy === `account-sync:${channel.id}` ? "animate-spin" : "")} />
                                <span>{t("channelsAccountRefresh")}</span>
                              </Button>
                              {channel.upstream_account_sync_status === "failed" && channel.upstream_account_sync_error ? (
                                <span className="max-w-[140px] truncate text-[10px] text-rose-600 dark:text-rose-300" title={channel.upstream_account_sync_error}>
                                  {channel.upstream_account_sync_error}
                                </span>
                              ) : null}
                            </div>
                            <div className="mt-1 space-y-0.5 text-[10px] text-slate-500 dark:text-slate-400">
                              <div>
                                {t("channelsAccountBalance")}: <span className="font-mono text-slate-700 dark:text-slate-200">{formatAccountValue(channel.upstream_balance)}{channel.upstream_balance ? ` ${channel.upstream_balance_unit || "USD"}` : ""}</span>
                              </div>
                              {channel.upstream_balance_total ? (
                                <div>
                                  {t("channelsAccountTotal")}: <span className="font-mono text-slate-700 dark:text-slate-200">{formatAccountValue(channel.upstream_balance_total)}{channel.upstream_balance_unit ? ` ${channel.upstream_balance_unit}` : ""}</span>
                                </div>
                              ) : null}
                              {channel.upstream_balance_used ? (
                                <div>
                                  {t("channelsAccountUsed")}: <span className="font-mono text-slate-700 dark:text-slate-200">{formatAccountValue(channel.upstream_balance_used)}{channel.upstream_balance_unit ? ` ${channel.upstream_balance_unit}` : ""}</span>
                                </div>
                              ) : null}
                              {channel.upstream_account_plan_name ? (
                                <div>
                                  {t("channelsAccountPlan")}: <span className="text-slate-700 dark:text-slate-200">{channel.upstream_account_plan_name}</span>
                                </div>
                              ) : null}
                              <div>
                                {t("channelsAccountRate")}: <span className="font-mono text-slate-700 dark:text-slate-200">{channel.upstream_rate_multiplier ? `x${formatAccountValue(channel.upstream_rate_multiplier)}` : "-"}</span>
                              </div>
                              {channel.upstream_account_synced_at ? (
                                <div>{t("channelsAccountLastSync")}: {formatTime(channel.upstream_account_synced_at)}</div>
                              ) : null}
                            </div>
                          </TableCell>

                          {/* Model Mapping Chips */}
                          <TableCell className="min-w-[240px] max-w-[320px]">
                            <div className="flex flex-wrap gap-1">
                              {channel.models.filter((m) => m.enabled).length === 0 ? (
                                <span className="text-xs text-slate-400">{t("channelsModelsEmpty")}</span>
                              ) : (
                                channel.models
                                  .filter((m) => m.enabled)
                                  .slice(0, 3)
                                  .map((m, idx) => (
                                    <span
                                      key={idx}
                                      className="rounded-md bg-slate-100 dark:bg-slate-800/90 border border-slate-200 dark:border-slate-700/80 px-2 py-0.5 text-[11px] font-mono text-slate-700 dark:text-slate-300"
                                    >
                                      {m.model}
                                    </span>
                                  ))
                              )}
                              {channel.models.filter((m) => m.enabled).length > 3 && (
                                <span className="rounded-md bg-slate-100 dark:bg-slate-800 px-1.5 py-0.5 text-[10px] text-slate-500 dark:text-slate-400 font-mono">
                                  +{channel.models.filter((m) => m.enabled).length - 3}
                                </span>
                              )}
                            </div>
                          </TableCell>

                          {/* Credential Reference */}
                          <TableCell className="min-w-[170px] whitespace-nowrap">
                            <span className="inline-block max-w-[230px] truncate rounded-lg border border-slate-200 bg-slate-100 px-2.5 py-1 font-mono text-[11px] text-slate-700 dark:border-slate-800 dark:bg-slate-950/80 dark:text-slate-300" title={channelCredentialLabel(channel)}>
                              {channelCredentialLabel(channel)}
                            </span>
                          </TableCell>

                          {/* Actions */}
                          <TableCell className="text-right whitespace-nowrap">
                            <div className="inline-flex items-center gap-1.5">
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-8 px-2.5 text-xs text-slate-700 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white"
                                onClick={() => openEditChannel(channel)}
                                 disabled={!canSeeAdminPermission("channel:update") || Boolean(channelActionBusy)}
                              >
                                <Pencil className="h-3.5 w-3.5 mr-1" />
                                <span>{t("channelsEdit")}</span>
                              </Button>

                              {channel.status === "active" ? (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="h-8 px-2 text-xs text-amber-600 dark:text-amber-300 hover:bg-amber-50 dark:hover:bg-amber-500/10"
                                  onClick={() => changeChannelStatus(channel, "disabled")}
                                   disabled={!canSeeAdminPermission("channel:update") || Boolean(channelActionBusy)}
                                  title={t("channelsPause")}
                                >
                                  <Pause className="h-3.5 w-3.5" />
                                </Button>
                              ) : (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="h-8 px-2 text-xs text-emerald-600 dark:text-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-500/10"
                                  onClick={() => changeChannelStatus(channel, "active")}
                                   disabled={!canSeeAdminPermission("channel:update") || Boolean(channelActionBusy)}
                                  title={t("channelsEnable")}
                                >
                                  <Play className="h-3.5 w-3.5" />
                                </Button>
                              )}

                              <Button
                                variant={channelDeleteConfirm === channel.id ? "destructive" : "ghost"}
                                size="sm"
                                className="h-8 px-2 text-xs text-rose-600 dark:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-500/10"
                                onClick={() => deleteChannel(channel)}
                                 disabled={!canSeeAdminPermission("channel:update") || Boolean(channelActionBusy)}
                                title={t("channelsDelete")}
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                        );
                      })
                    )}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Section 5: Billing & Pricing View */}
        {adminSection === "billing" && (
          <div className="space-y-6">
            <div>
              {/* Publish Price Form */}
              <Card className="glass-panel">
                <CardHeader className="space-y-3 pb-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div>
                      <CardTitle className="text-lg font-bold text-slate-900 dark:text-white flex items-center gap-2">
                        <Database className="h-4 w-4 text-indigo-600 dark:text-indigo-400" />
                        <span>{t("billingOfficialSourceTitle")}</span>
                      </CardTitle>
                      <CardDescription>{t("billingReferenceHint")}</CardDescription>
                    </div>
                    <Button type="button" variant="outline" size="sm" onClick={() => void syncOfficialPrices()} disabled={!canSeeAdminPermission("price:publish") || billingBusy || officialPriceSyncBusy} className="h-9 shrink-0 gap-1.5 text-xs">
                      <RefreshCw className={cn("h-3.5 w-3.5", officialPriceSyncBusy ? "animate-spin" : "")} />
                      <span>{officialPriceSyncBusy ? t("billingOfficialSyncing") : t("billingOfficialSync")}</span>
                    </Button>
                  </div>
                  <p className="text-[11px] leading-5 text-slate-500 dark:text-slate-400">{t("billingOfficialHint")}</p>
                </CardHeader>
                <CardContent>
                  <div className="mb-4 rounded-xl border border-indigo-500/20 bg-indigo-50/70 p-4 text-xs text-indigo-800 dark:border-indigo-400/20 dark:bg-indigo-500/10 dark:text-indigo-200">{t("billingReferenceExplain")}</div>
                </CardContent>
              </Card>

            </div>

            {/* Status Message */}
            {billingMessage.text && (
              <div
                className={cn("rounded-xl border p-3 text-xs flex items-center gap-2", {
                  "border-emerald-500/30 bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300": billingMessage.kind === "success",
                  "border-amber-500/30 bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300": billingMessage.kind === "pending",
                  "border-rose-500/30 bg-rose-50 dark:bg-rose-500/10 text-rose-700 dark:text-rose-300": billingMessage.kind === "error",
                })}
              >
                <Sparkles className="h-4 w-4 shrink-0" />
                <span>{billingMessage.text}</span>
              </div>
            )}

            {/* Current model reference price matrix */}
            <Card className="glass-panel">
              <CardHeader className="space-y-1 pb-3">
                <CardTitle className="text-lg font-bold text-slate-900 dark:text-white">{t("billingCurrentMatrixTitle")}</CardTitle>
                <CardDescription>
                  {billingBusy && prices.length === 0 ? t("billingLoad") : `${prices.length} ${t("billingModelsUnit")}`}
                </CardDescription>
                <p className="text-[11px] leading-5 text-slate-500 dark:text-slate-400">{t("billingProfitHint")}</p>
              </CardHeader>
              <CardContent>
                <PriceMatrixTable
                  groups={priceGroups}
                  t={t}
                  formatTime={formatTime}
                   openEditPrice={openEditPrice}
                   canPublishPrice={canSeeAdminPermission("price:publish")}
                   billingBusy={billingBusy}
                  officialPriceSyncBusy={officialPriceSyncBusy}
                />
              </CardContent>
            </Card>
          </div>
        )}

        {adminSection === "settings" && (
          <AdminSettingsPanel
            language={language}
            profile={adminProfile}
            profileForm={adminProfileForm}
            setProfileForm={setAdminProfileForm}
            emailForm={adminEmailForm}
            setEmailForm={setAdminEmailForm}
            passwordForm={adminPasswordForm}
            setPasswordForm={setAdminPasswordForm}
            profileBusy={adminProfileBusy}
            profileMessage={adminProfileMessage}
            saveProfile={saveAdminProfile}
            saveEmail={saveAdminEmail}
            savePassword={saveAdminPassword}
            mfaStatus={adminMfaStatus}
            mfaEnrollment={adminMfaEnrollment}
            mfaCode={adminMfaCode}
            setMfaCode={setAdminMfaCode}
            mfaBusy={adminMfaBusy}
            beginMFA={beginAdminMFA}
            confirmMFA={confirmAdminMFA}
            cancelMFA={cancelAdminMFA}
            disableMFA={disableAdminMFA}
            securitySettings={securitySettings}
            securityMessage={securityMessage}
            securityBusy={securityBusy}
            persistSecurity={persistSecurity}
            canUpdateSystemSettings={canSeeAdminPermission("security:update")}
            siteForm={siteForm}
            setSiteForm={setSiteForm}
            siteBusy={siteBusy}
            siteMessage={siteMessage}
            saveSiteSettings={saveSiteSettings}
            apiEndpoints={apiEndpoints}
            apiEndpointFormOpen={apiEndpointFormOpen}
            apiEndpointForm={apiEndpointForm}
            setAPIEndpointForm={setAPIEndpointForm}
            apiEndpointBusy={apiEndpointBusy}
            apiEndpointActionBusy={apiEndpointActionBusy}
            apiEndpointMessage={apiEndpointMessage}
            openCreateAPIEndpoint={openCreateAPIEndpoint}
            openEditAPIEndpoint={openEditAPIEndpoint}
            closeAPIEndpointForm={closeAPIEndpointForm}
            saveAPIEndpoint={saveAPIEndpoint}
            toggleAPIEndpoint={toggleAPIEndpoint}
            deleteAPIEndpoint={deleteAPIEndpoint}
	            smtpForm={smtpForm}
	            setSmtpForm={setSmtpForm}
	            emailSettings={emailSettings}
	            emailBusy={emailBusy}
	            emailMessage={emailMessage}
	            emailTestRecipient={emailTestRecipient}
	            setEmailTestRecipient={setEmailTestRecipient}
	            smtpConnectionBusy={smtpConnectionBusy}
	            smtpMessageBusy={smtpMessageBusy}
	            saveEmailSettings={saveEmailSettings}
	            testSMTPConnection={testSMTPConnection}
	            sendTestEmail={sendTestEmail}
	            featureSettings={featureSettings}
	            featureBusy={featureBusy}
	            featureMessage={featureMessage}
	            saveFeatureSettings={saveFeatureSettings}
	            emailTemplates={emailTemplates}
	            emailTemplatesBusy={emailTemplatesBusy}
            emailTemplatesMessage={emailTemplatesMessage}
            saveEmailTemplate={saveEmailTemplate}
            deleteEmailTemplate={deleteEmailTemplate}
            paymentConfigs={paymentConfigs}
            paymentSettingsBusy={paymentSettingsBusy}
            paymentSettingsMessage={paymentSettingsMessage}
            savePaymentConfig={savePaymentConfig}
            paymentRechargePackages={paymentRechargePackages}
            savePaymentRechargePackages={savePaymentRechargePackages}
            canUpdatePaymentSettings={canUpdatePaymentSettings}
          />
        )}
        {adminSection === "usage" && (
          <AdminUsagePanel
            language={language}
            report={usageReport}
            busy={usageReportBusy}
            message={usageReportMessage}
            refresh={refreshUsageReport}
            search={reportSearch}
            setSearch={setReportSearch}
            status={reportStatus}
            setStatus={setReportStatus}
            tenant={reportTenant}
            setTenant={setReportTenant}
            model={reportModel}
            setModel={setReportModel}
            group={reportGroup}
            setGroup={setReportGroup}
            from={reportFrom}
            setFrom={setReportFrom}
            to={reportTo}
            setTo={setReportTo}
            offset={reportOffset}
          />
        )}
        {adminSection === "finance" && (
          <AdminFinancePanel
            language={language}
            report={financeReport}
            busy={financeReportBusy}
            message={financeReportMessage}
            refresh={refreshFinanceReport}
            search={financeSearch}
            setSearch={setFinanceSearch}
            currency={financeCurrency}
            setCurrency={setFinanceCurrency}
            from={financeFrom}
            setFrom={setFinanceFrom}
            to={financeTo}
            setTo={setFinanceTo}
          />
        )}
        {adminSection === "account-finance" && (
          <AdminAccountFinancePanel
            language={language}
            report={financeReport}
            busy={financeReportBusy}
            message={financeReportMessage}
            refresh={refreshFinanceReport}
            search={financeSearch}
            setSearch={setFinanceSearch}
            currency={financeCurrency}
            setCurrency={setFinanceCurrency}
            from={financeFrom}
            setFrom={setFinanceFrom}
            to={financeTo}
            setTo={setFinanceTo}
            billingAccount={billingAccount}
            billingMessage={billingMessage}
            billingBusy={billingBusy}
            creditForm={creditForm}
            setCreditForm={setCreditForm}
            creditBillingAccount={creditBillingAccount}
            loadBillingAccount={loadBillingAccount}
            canReadBilling={canSeeAdminPermission("billing:read")}
            canUpdateBilling={canSeeAdminPermission("billing:update")}
          />
        )}
        {adminSection === "audit" && (
          <AdminAuditPanel language={language} report={auditReport} busy={auditBusy} message={auditMessage} refresh={refreshAudit} />
        )}
        </>}
      </div>
    </div>
  );
}

function AdminDashboardInsights({ t, snapshot, busy, formatTime, routeTo }: { t: (key: TranslationKey) => string; snapshot: OperationsSnapshot | null; busy: boolean; formatTime: (value: string) => string; routeTo: (target: string) => void }) {
  const models = snapshot?.model_usage || [];
  const trend = snapshot?.usage_trend || [];
  const maxTokens = Math.max(1, ...models.map((item) => item.total_tokens));
  const maxTrendTokens = Math.max(1, ...trend.map((item) => item.total_tokens));
  const chartWidth = 760;
  const chartHeight = 180;
  const points = trend.map((item, index) => `${trend.length === 1 ? chartWidth / 2 : (index / (trend.length - 1)) * chartWidth},${chartHeight - (item.total_tokens / maxTrendTokens) * (chartHeight - 18)}`).join(" ");
  const money = (value?: string) => formatDecimalWithoutTrailingZeros(value || "0", "0");
  return (
    <div className="space-y-6">
      <div className="grid gap-6 xl:grid-cols-[minmax(300px,0.85fr)_minmax(0,1.15fr)]">
        <Card className="glass-panel">
          <CardHeader className="flex flex-row items-start justify-between gap-3 pb-3">
            <div><CardTitle className="flex items-center gap-2 text-lg font-bold text-slate-900 dark:text-white"><WalletCards className="h-4 w-4 text-emerald-600" />{t("adminDashboardFinanceTitle")}</CardTitle><CardDescription>{t("adminDashboardFinanceDescription")}</CardDescription></div>
            <Button type="button" variant="outline" size="icon" onClick={() => routeTo("#admin/finance")} title={t("adminDashboardOpenFinance")} aria-label={t("adminDashboardOpenFinance")}><Receipt className="h-4 w-4" /></Button>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-2"><MetricRow label={t("adminDashboardTodayRecharge")} value={money(snapshot?.today_recharge_amount)} detail={`${snapshot?.today_recharge_orders || 0} ${t("adminMetricOrders")}`} tone="emerald" /><MetricRow label={t("adminDashboardTodayCredited")} value={money(snapshot?.today_credited_amount)} detail={t("adminDashboardCreditedHint")} tone="cyan" /><MetricRow label={t("adminDashboardTodaySpend")} value={money(snapshot?.today_spend)} detail={t("adminDashboardSpendHint")} tone="amber" /><MetricRow label={t("adminDashboardPendingRecharge")} value={String(snapshot?.pending_recharge_orders || 0)} detail={t("adminDashboardPendingHint")} tone="indigo" /></div>
            <div className="flex items-center justify-between border-t border-slate-200/80 pt-3 text-xs dark:border-slate-800/80"><span className="text-slate-500">{t("adminDashboardTotalRecharge")}</span><span className="font-mono font-semibold text-slate-900 dark:text-white">{money(snapshot?.total_recharge_amount)} · {snapshot?.total_recharge_orders || 0} {t("adminMetricOrders")}</span></div>
            <div className="flex items-center justify-between border-t border-slate-200/80 pt-3 text-xs dark:border-slate-800/80"><span className="text-slate-500">{t("adminDashboard24hCalls")}</span><span className="font-mono font-semibold text-slate-900 dark:text-white">{snapshot?.requests_24h || 0} · {snapshot?.failed_requests_24h || 0} {t("adminDashboardFailed")}</span></div>
            <div className="flex items-center justify-between text-xs"><span className="text-slate-500">{t("adminDashboardAverageLatency")}</span><span className="font-mono font-semibold text-slate-900 dark:text-white">{snapshot ? snapshot.average_latency_ms.toFixed(0) : "0"}ms</span></div>
          </CardContent>
        </Card>
        <Card className="glass-panel">
          <CardHeader className="flex flex-row items-start justify-between gap-3 pb-3"><div><CardTitle className="flex items-center gap-2 text-lg font-bold text-slate-900 dark:text-white"><Waypoints className="h-4 w-4 text-indigo-600" />{t("adminDashboardModelTitle")}</CardTitle><CardDescription>{t("adminDashboardModelDescription")}</CardDescription></div><Button type="button" variant="outline" size="icon" onClick={() => routeTo("#admin/usage")} title={t("adminDashboardOpenUsage")} aria-label={t("adminDashboardOpenUsage")}><ClipboardList className="h-4 w-4" /></Button></CardHeader>
          <CardContent>{models.length === 0 ? <div className="rounded-xl border border-dashed border-slate-300 py-12 text-center text-xs text-slate-500 dark:border-slate-700">{t("adminDashboardNoUsage")}</div> : <div className="space-y-3">{models.slice(0, 8).map((item) => <div key={`${item.provider}:${item.model}`}><div className="flex items-center justify-between gap-3 text-xs"><span className="min-w-0 truncate font-semibold text-slate-800 dark:text-slate-200" title={item.model}>{item.model}</span><span className="shrink-0 font-mono text-slate-500">{formatAdminInteger(item.total_tokens)}</span></div><div className="mt-1 h-2 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800"><div className="h-full rounded-full bg-indigo-500" style={{ width: `${Math.max(2, item.total_tokens / maxTokens * 100)}%` }} /></div><div className="mt-1 flex justify-between text-[10px] text-slate-500"><span>{item.provider} · {formatAdminInteger(item.requests)} {t("adminDashboardRequestsUnit")}</span><span>{money(item.total_spend)}</span></div></div>)}</div>}</CardContent>
        </Card>
      </div>
      <Card className="glass-panel">
        <CardHeader className="flex flex-row items-start justify-between gap-3 pb-3"><div><CardTitle className="flex items-center gap-2 text-lg font-bold text-slate-900 dark:text-white"><Activity className="h-4 w-4 text-cyan-600" />{t("adminDashboardTrendTitle")}</CardTitle><CardDescription>{t("adminDashboardTrendDescription")}</CardDescription></div><Badge variant={busy ? "warning" : "success"}>{busy ? t("adminDashboardRefreshing") : snapshot ? formatTime(snapshot.collected_at) : "-"}</Badge></CardHeader>
        <CardContent>{trend.length === 0 ? <div className="rounded-xl border border-dashed border-slate-300 py-12 text-center text-xs text-slate-500 dark:border-slate-700">{t("adminDashboardNoUsage")}</div> : <div className="overflow-hidden rounded-xl border border-slate-200 bg-slate-50/70 p-3 dark:border-slate-800 dark:bg-slate-950/40"><svg viewBox={`0 0 ${chartWidth} ${chartHeight}`} className="h-52 w-full" preserveAspectRatio="none" role="img" aria-label={t("adminDashboardTrendTitle")}><polyline points={points} fill="none" stroke="currentColor" strokeWidth="4" vectorEffect="non-scaling-stroke" className="text-cyan-500" />{trend.map((item, index) => <circle key={item.at} cx={trend.length === 1 ? chartWidth / 2 : (index / (trend.length - 1)) * chartWidth} cy={chartHeight - (item.total_tokens / maxTrendTokens) * (chartHeight - 18)} r="4" className="fill-emerald-500" />)}</svg><div className="mt-2 flex justify-between text-[10px] text-slate-500"><span>{formatTime(trend[0].at)}</span><span>{formatTime(trend[trend.length - 1].at)}</span></div></div>}</CardContent>
      </Card>
    </div>
  );
}

function MetricRow({ label, value, detail, tone }: { label: string; value: string; detail: string; tone: "indigo" | "cyan" | "amber" | "emerald" }) {
  return <div className="rounded-xl border border-slate-200 bg-slate-50/70 p-3 dark:border-slate-800 dark:bg-slate-950/35"><div className="text-[11px] text-slate-500">{label}</div><div className={cn("mt-1 text-xl font-bold", tone === "emerald" ? "text-emerald-700 dark:text-emerald-300" : tone === "cyan" ? "text-cyan-700 dark:text-cyan-300" : tone === "amber" ? "text-amber-700 dark:text-amber-300" : "text-indigo-700 dark:text-indigo-300")}>{value}</div><div className="mt-1 truncate text-[10px] text-slate-500" title={detail}>{detail}</div></div>;
}

function formatAdminInteger(value: number) {
  return new Intl.NumberFormat("en-US", { notation: value >= 10000 ? "compact" : "standard", maximumFractionDigits: 1 }).format(value || 0);
}

type PriceMatrixGroup = {
  provider: string;
  models: PriceMatrixSummary[];
};

function groupPriceRows(prices: PriceMatrixSummary[]): PriceMatrixGroup[] {
  const grouped = new Map<string, PriceMatrixSummary[]>();
  for (const price of prices) {
    const provider = price.provider.trim().toLowerCase() || "unknown";
    const models = grouped.get(provider) || [];
    models.push(price);
    grouped.set(provider, models);
  }
  return Array.from(grouped.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([provider, models]) => ({
      provider,
      models: models.sort((left, right) => left.model.localeCompare(right.model)),
    }));
}

function providerDisplayName(provider: string) {
  switch (provider.toLowerCase()) {
    case "openai":
      return "OpenAI";
    case "anthropic":
      return "Anthropic";
    case "grok":
      return "Grok";
    case "gemini":
      return "Gemini";
    case "volcengine":
      return "Volcano Ark";
    default:
      return provider || "Unknown provider";
  }
}

function componentUnitScale(unit: string) {
  const normalized = unit.toLowerCase();
  if (normalized.includes("1k") || normalized.includes("gb_day")) return 1;
  if (normalized === "token" || normalized.endsWith("_token")) return 1_000_000;
  return 1;
}

function componentUnitLabel(unit: string) {
  const normalized = unit.toLowerCase();
  if (normalized.includes("1k") && normalized.includes("call")) return "USD / 1K calls";
  if (normalized.includes("1k") && normalized.includes("token")) return "USD / 1K Token";
  if (normalized.includes("token")) return "USD / 1M Token";
  if (normalized === "second") return "USD / second";
  if (normalized === "image") return "USD / image";
  if (normalized === "pixel") return "USD / pixel";
  if (normalized === "character") return "USD / character";
  if (normalized === "request") return "USD / request";
  if (normalized === "query") return "USD / query";
  if (normalized === "session") return "USD / session";
  if (normalized === "page") return "USD / page";
  if (normalized === "gb_day") return "USD / GB-day";
  return "USD / " + (unit || "unit");
}

function componentDisplayPrice(value: string, unit: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "-";
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 8 }).format(parsed * componentUnitScale(unit));
}

function formatMultiplier(value: string) {
  return formatDecimalWithoutTrailingZeros(value, value || "-");
}

type PriceEstimateField = "customer_price_per_unit" | "estimated_cost_per_unit" | "profit_per_unit";

function estimateComponentLabel(code: string, t: (key: TranslationKey) => string) {
  switch (code) {
    case "input_tokens":
      return t("billingMetricInput");
    case "cached_input_tokens":
      return t("billingMetricCachedInput");
    case "cache_creation_tokens":
      return t("modelsCacheWritesPrice");
    case "cache_creation_1h_tokens":
      return t("modelsCacheWritesPrice") + " (1h)";
    case "output_tokens":
      return t("billingMetricOutput");
    case "reasoning_tokens":
      return t("billingMetricReasoning");
    default:
      return code;
  }
}

function estimateComponents(estimate: PriceMatrixCostEstimate) {
  const preferredOrder = ["input_tokens", "cached_input_tokens", "cache_creation_tokens", "output_tokens", "reasoning_tokens"];
  return [...(estimate.components || [])].sort((left, right) => {
    const leftIndex = preferredOrder.indexOf(left.component_code);
    const rightIndex = preferredOrder.indexOf(right.component_code);
    if (leftIndex === -1 && rightIndex === -1) return left.component_code.localeCompare(right.component_code);
    if (leftIndex === -1) return 1;
    if (rightIndex === -1) return -1;
    return leftIndex - rightIndex;
  });
}

function formatProfitRate(value?: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "-";
  return `${new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 }).format(parsed)}%`;
}

function PriceEstimateCell({
  estimates,
  field,
  tone,
  t,
}: {
  estimates?: PriceMatrixCostEstimate[];
  field: PriceEstimateField;
  tone: "platform" | "cost" | "profit";
  t: (key: TranslationKey) => string;
}) {
  if (!estimates || estimates.length === 0) {
    return <span className="text-xs text-slate-400">-</span>;
  }
  const valueClass = tone === "platform"
    ? "text-indigo-700 dark:text-indigo-300"
    : tone === "cost"
      ? "text-amber-700 dark:text-amber-300"
      : "text-emerald-700 dark:text-emerald-300";
  return (
    <div className="min-w-[190px] space-y-2">
      {estimates.map((estimate) => (
        <div key={estimate.group_id} className="space-y-1.5 rounded-lg border border-slate-200/80 bg-slate-50/70 p-2 dark:border-slate-800 dark:bg-slate-950/40">
          <div className="flex items-center justify-between gap-2 text-[10px]">
            <span className="min-w-0 truncate font-semibold text-slate-700 dark:text-slate-200" title={estimate.group_name || estimate.group_code}>
              {estimate.group_name || estimate.group_code}
            </span>
            <span className="shrink-0 font-mono text-slate-500 dark:text-slate-400">x{formatMultiplier(estimate.multiplier)}</span>
          </div>
          <div className="space-y-0.5">
            {estimateComponents(estimate).map((component) => (
              <div key={component.component_code} className="flex items-center justify-between gap-2 text-[10px]">
                <span className="truncate text-slate-500 dark:text-slate-400">{estimateComponentLabel(component.component_code, t)}</span>
                <span className={cn(
                  "shrink-0 font-mono font-semibold",
                  tone === "profit" && Number(component[field]) < 0 ? "text-rose-700 dark:text-rose-300" : valueClass
                )}>
                  {componentDisplayPrice(component[field] || "", component.unit)}
                </span>
              </div>
            ))}
            {estimateComponents(estimate).length === 0 ? <span className="text-xs text-slate-400">-</span> : null}
          </div>
          <div className="truncate text-[9px] text-slate-400 dark:text-slate-500" title={`${estimate.channel_name} · ${estimate.route_count} ${t("billingAvailableRoutes")} · ${t("billingDiscount")} ${estimate.upstream_cost_discount}`}>
            {t("billingPrimaryChannel")}: {estimate.channel_name || "-"} · {t("billingDiscount")} {formatMultiplier(estimate.upstream_cost_discount)} · {estimate.route_count} {t("billingAvailableRoutes")}
          </div>
        </div>
      ))}
    </div>
  );
}

function PriceMarginCell({ estimates, t }: { estimates?: PriceMatrixCostEstimate[]; t: (key: TranslationKey) => string }) {
  if (!estimates || estimates.length === 0) {
    return <span className="text-xs text-slate-400">-</span>;
  }
  return (
    <div className="min-w-[130px] space-y-2">
      {estimates.map((estimate) => (
        <div key={estimate.group_id} className="space-y-1.5 rounded-lg border border-slate-200/80 bg-slate-50/70 p-2 dark:border-slate-800 dark:bg-slate-950/40">
          <div className="flex items-center justify-between gap-2 text-[10px]">
            <span className="min-w-0 truncate font-semibold text-slate-700 dark:text-slate-200">{estimate.group_name || estimate.group_code}</span>
            <span className="shrink-0 font-mono text-slate-500 dark:text-slate-400">x{formatMultiplier(estimate.multiplier)}</span>
          </div>
          <div className="space-y-0.5">
            {estimateComponents(estimate).map((component) => (
              <div key={component.component_code} className="flex items-center justify-between gap-2 text-[10px]">
                <span className="truncate text-slate-500 dark:text-slate-400">{estimateComponentLabel(component.component_code, t)}</span>
                <span className={cn(
                  "shrink-0 font-mono font-semibold",
                  component.profit_margin_percent === undefined || component.profit_margin_percent === ""
                    ? "text-slate-400"
                    : Number(component.profit_margin_percent) < 0
                      ? "text-rose-700 dark:text-rose-300"
                      : "text-emerald-700 dark:text-emerald-300"
                )}>
                  {formatProfitRate(component.profit_margin_percent)}
                </span>
              </div>
            ))}
            {estimateComponents(estimate).length === 0 ? <span className="text-xs text-slate-400">-</span> : null}
          </div>
          <div className="truncate text-[9px] text-slate-400 dark:text-slate-500">
            {estimate.billing_type === "free" ? t("billingFreeGroup") : t("billingMarginBasis")}
          </div>
        </div>
      ))}
    </div>
  );
}

function PriceMatrixTable({
  groups,
  t,
  formatTime,
  openEditPrice,
  canPublishPrice,
  billingBusy,
  officialPriceSyncBusy,
}: {
  groups: PriceMatrixGroup[];
  t: (key: TranslationKey) => string;
  formatTime: (value: string) => string;
  openEditPrice: (price: PriceMatrixSummary) => void;
  canPublishPrice: boolean;
  billingBusy: boolean;
  officialPriceSyncBusy: boolean;
}) {
  if (groups.length === 0) {
    return <div className="rounded-xl border border-dashed border-slate-300 py-10 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">{t("billingNoPrices")}</div>;
  }

  return (
    <Tabs defaultValue={groups[0].provider} className="w-full">
      <TabsList className="flex h-auto w-full justify-start gap-1 overflow-x-auto rounded-xl border-slate-200 bg-slate-50/80 p-1 dark:border-slate-800 dark:bg-slate-950/50">
        {groups.map((group) => (
          <TabsTrigger key={group.provider} value={group.provider} className="h-9 shrink-0 gap-2 px-3 text-xs sm:px-4 sm:text-sm">
            <Building2 className="h-3.5 w-3.5" />
            <span>{providerDisplayName(group.provider)}</span>
            <span className="rounded-full bg-black/5 px-1.5 py-0.5 text-[10px] font-semibold text-current/70 dark:bg-white/10">
              {group.models.length}
            </span>
          </TabsTrigger>
        ))}
      </TabsList>
      {groups.map((group) => (
        <TabsContent key={group.provider} value={group.provider} className="mt-4">
          <div className="overflow-hidden rounded-xl border border-slate-200 dark:border-slate-800">
            <div className="flex flex-col gap-2 border-b border-slate-200 bg-slate-50/80 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/50 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-center gap-3">
                <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-indigo-500/10 text-indigo-600 dark:bg-indigo-500/20 dark:text-indigo-300"><Building2 className="h-4 w-4" /></div>
                <div><div className="text-sm font-bold text-slate-900 dark:text-white">{providerDisplayName(group.provider)}</div><div className="font-mono text-[10px] uppercase text-slate-500 dark:text-slate-400">{group.provider}</div></div>
              </div>
              <Badge variant="outline" className="self-start sm:self-auto">{group.models.length} {t("billingModelsUnit")}</Badge>
            </div>
            <div className="overflow-x-auto">
                <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("billingPriceModel")}</TableHead>
                    <TableHead>{t("billingPriceSource")}</TableHead>
                    <TableHead>{t("billingPriceComponents")}</TableHead>
                    <TableHead>{t("billingPlatformPrice")}</TableHead>
                    <TableHead>{t("billingEstimatedCost")}</TableHead>
                    <TableHead>{t("billingProfit")}</TableHead>
                    <TableHead>{t("billingProfitRate")}</TableHead>
                    <TableHead>{t("billingPriceUpdated")}</TableHead>
                    <TableHead className="text-right">{t("billingPriceActions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {group.models.map((price) => (
                    <TableRow key={price.model_id}>
                      <TableCell className="min-w-[230px]"><div className="font-semibold text-slate-900 dark:text-white">{price.model}</div></TableCell>
                      <TableCell><Badge variant={price.source === "manual" ? "default" : price.source === "litellm" ? "cyan" : "muted"}>{price.source === "manual" ? t("billingPriceSourceManual") : price.source === "litellm" ? t("billingPriceSourceLiteLLM") : t("billingPriceSourceMissing")}</Badge></TableCell>
                      <TableCell><div className="flex max-w-[420px] flex-wrap gap-1.5">{(price.components || []).map((component) => <span key={component.component_code} className="rounded-md bg-slate-100 px-2 py-1 text-[10px] text-slate-700 dark:bg-slate-800 dark:text-slate-300" title={component.component_code + " · " + component.unit}>{estimateComponentLabel(component.component_code, t)}: {componentDisplayPrice(component.price_per_unit, component.unit)} <span className="font-sans text-slate-400">{componentUnitLabel(component.unit)}</span></span>)}{(price.components || []).length === 0 ? <span className="text-xs text-slate-400">-</span> : null}</div></TableCell>
                      <TableCell><PriceEstimateCell estimates={price.cost_estimates} field="customer_price_per_unit" tone="platform" t={t} /></TableCell>
                      <TableCell><PriceEstimateCell estimates={price.cost_estimates} field="estimated_cost_per_unit" tone="cost" t={t} /></TableCell>
                      <TableCell><PriceEstimateCell estimates={price.cost_estimates} field="profit_per_unit" tone="profit" t={t} /></TableCell>
                      <TableCell><PriceMarginCell estimates={price.cost_estimates} t={t} /></TableCell>
                      <TableCell className="whitespace-nowrap text-xs text-slate-500 dark:text-slate-400">{price.updated_at ? formatTime(price.updated_at) : "-"}</TableCell>
                      <TableCell className="text-right"><Button type="button" variant="ghost" size="icon" onClick={() => openEditPrice(price)} disabled={!canPublishPrice || billingBusy || officialPriceSyncBusy} title={t("billingPriceEdit")} aria-label={t("billingPriceEdit")} className="h-9 w-9 text-slate-600 hover:text-indigo-600 dark:text-slate-300 dark:hover:text-indigo-300"><Pencil className="h-4 w-4" /></Button></TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        </TabsContent>
      ))}
    </Tabs>
  );
}
