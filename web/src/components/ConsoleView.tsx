import React, { useState } from "react";
import {
  Activity,
  BadgeDollarSign,
  Building2,
  BookOpen,
  CheckCircle2,
  Code2,
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  Copy,
  ExternalLink,
  FolderKanban,
  Gauge,
  KeyRound,
  LayoutDashboard,
  Image,
  LogOut,
  MessageSquare,
  Network,
  Pencil,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  Ban,
  Trash2,
  UserRound,
  Video,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { BillingAccount, ConsoleProfile, ConsoleSection, ConsoleUsageStatus, EmailFormState, EnterpriseVerification, Language, LoginMessage, MFAEnrollment, MFAStatus, ModelStatusReport, PasswordFormState, PaymentOrder, PaymentOrderList, Principal, ProfileFormState, ProjectFormState, ProjectMember, ProjectSummary, PublicAPIEndpoint, PublicModelSummary, PublicPaymentProvider, TenantMember, TokenGroupOption, TokenSummary, TranslationKey, UsageRecord, UsageReport } from "@/types";
import { translations } from "@/locales/translations";
import { cn, formatDecimalWithoutTrailingZeros } from "@/lib/utils";
import { ProfilePanel } from "@/components/ProfilePanel";
import { UsageDocsPanel } from "@/components/UsageDocsPanel";
import { TenantWorkspacePanel } from "@/components/TenantWorkspacePanel";
import { ModelStatusPanel } from "@/components/ModelStatusPanel";
import { EnterpriseVerificationPanel } from "@/components/EnterpriseVerificationPanel";
import { PaymentRechargePanel } from "@/components/PaymentRechargePanel";
import { resolveAPIEndpointURLs } from "@/lib/api-endpoint";
import { ImageLabView, VideoLabView } from "@/components/MediaLabsView";
import { TextDebugPanel } from "@/components/TextDebugPanel";

type ConsoleNavItem = { id: ConsoleSection; icon: typeof LayoutDashboard; label: TranslationKey; title: TranslationKey; description: TranslationKey; permission?: string };

interface ConsoleViewProps {
  language: Language;
  principal: Principal | null;
  section: ConsoleSection;
  setSection: (section: ConsoleSection) => void;
  routeTo: (target: string) => void;
  handleSignOut: () => void;
  tokens: TokenSummary[];
  tokensBusy: boolean;
  tokensMessage: LoginMessage;
  refreshTokens: (showPending?: boolean) => Promise<void>;
  editToken: (token: TokenSummary) => void;
  pauseToken: (token: TokenSummary) => Promise<void>;
  resumeToken: (token: TokenSummary) => Promise<void>;
  terminateToken: (token: TokenSummary) => Promise<void>;
  deleteToken: (token: TokenSummary) => Promise<void>;
  copyToken: (token: TokenSummary) => Promise<void>;
  tokenSecretAvailable: (token: TokenSummary) => boolean;
  tokenActionBusy: string;
  openCreateToken: () => void;
  billingAccount: BillingAccount | null;
  billingBusy: boolean;
  billingMessage: LoginMessage;
  refreshBilling: () => Promise<void>;
  enterpriseVerification: EnterpriseVerification | null;
  enterpriseBusy: boolean;
  enterpriseMessage: LoginMessage;
  refreshEnterprise: () => Promise<void>;
  submitEnterprise: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  paymentProviders: PublicPaymentProvider[];
  paymentBusy: boolean;
  paymentMessage: LoginMessage;
  paymentOrder: PaymentOrder | null;
  createPaymentOrder: (provider: PaymentOrder["provider"], amount: string, currency: string, packageID?: string) => Promise<void>;
  refreshPaymentOrder: () => Promise<void>;
  capturePayPal: () => Promise<void>;
  paymentOrders: PaymentOrderList | null;
  paymentOrdersBusy: boolean;
  refreshPaymentOrders: (offset?: number) => Promise<void>;
  usageStatus: ConsoleUsageStatus | null;
  usageBusy: boolean;
  usageMessage: LoginMessage;
  refreshUsage: (showPending?: boolean, offset?: number) => Promise<void>;
  consoleUsageReport: UsageReport | null;
  consoleUsageTokens: TokenSummary[];
  consoleUsageGroups: TokenGroupOption[];
  consoleUsageTokenName: string;
  setConsoleUsageTokenName: (value: string) => void;
  consoleUsageModel: string;
  setConsoleUsageModel: (value: string) => void;
  consoleUsageGroup: string;
  setConsoleUsageGroup: (value: string) => void;
  consoleUsageFrom: string;
  setConsoleUsageFrom: (value: string) => void;
  consoleUsageTo: string;
  setConsoleUsageTo: (value: string) => void;
  apiEndpoints: PublicAPIEndpoint[];
  models: PublicModelSummary[];
  modelStatusEnabled: boolean;
  modelStatusReport: ModelStatusReport | null;
  modelStatusBusy: boolean;
  modelStatusMessage: LoginMessage;
  refreshModelStatus: (showPending?: boolean) => Promise<void>;
  consoleProfile: ConsoleProfile | null;
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
  projects: ProjectSummary[];
  projectsBusy: boolean;
  projectsMessage: LoginMessage;
  refreshProjects: (showPending?: boolean) => Promise<void>;
  saveProject: (form: ProjectFormState) => Promise<boolean>;
  deleteProject: (project: ProjectSummary) => Promise<void>;
  projectActionBusy: string;
  projectDeleteConfirm: string;
  members: TenantMember[];
  membersBusy: boolean;
  membersMessage: LoginMessage;
  refreshMembers: (showPending?: boolean) => Promise<void>;
  addMember: (email: string, role: TenantMember["role"]) => Promise<void>;
  updateMember: (member: TenantMember, role: TenantMember["role"], status: TenantMember["status"]) => Promise<void>;
  removeMember: (member: TenantMember) => Promise<void>;
  memberActionBusy: string;
  projectMembers: ProjectMember[];
  projectMembersBusy: boolean;
  projectMembersMessage: LoginMessage;
  selectedProjectID: string;
  selectProject: (projectID: string) => void;
  refreshProjectMembers: (projectID: string, showPending?: boolean) => Promise<void>;
  addProjectMember: (email: string, role: ProjectMember["role"]) => Promise<void>;
  updateProjectMember: (member: ProjectMember, role: ProjectMember["role"]) => Promise<void>;
  removeProjectMember: (member: ProjectMember) => Promise<void>;
  projectMemberActionBusy: string;
}

const consoleSections: ConsoleNavItem[] = [
  { id: "dashboard", icon: LayoutDashboard, label: "consoleNavDashboard", title: "consoleDashboardTitle", description: "consoleDashboardDescription" },
  { id: "model-status", icon: Activity, label: "consoleNavModelStatus", title: "consoleModelStatusTitle", description: "consoleModelStatusDescription", permission: "model:status:read" },
  { id: "projects", icon: FolderKanban, label: "consoleNavProjects", title: "consoleProjectsTitle", description: "consoleProjectsDescription", permission: "project:read" },
  { id: "tokens", icon: KeyRound, label: "consoleNavTokens", title: "tokensConsoleTitle", description: "tokensConsoleDescription", permission: "token:read" },
  { id: "enterprise", icon: Building2, label: "consoleNavEnterprise", title: "enterpriseTitle", description: "enterpriseDescription", permission: "enterprise:read" },
  { id: "profile", icon: UserRound, label: "consoleNavProfile", title: "consoleProfileTitle", description: "consoleProfileDescription" },
  { id: "docs", icon: BookOpen, label: "consoleNavDocs", title: "consoleDocsTitle", description: "consoleDocsDescription" },
];

const financeSections: ConsoleNavItem[] = [
  { id: "billing-records", icon: Activity, label: "consoleNavUsage", title: "consoleUsageTitle", description: "consoleUsageDescription", permission: "usage:read" },
  { id: "billing", icon: BadgeDollarSign, label: "consoleNavBilling", title: "consoleBillingTitle", description: "consoleBillingDescription", permission: "billing:read" },
  { id: "billing-orders", icon: BadgeDollarSign, label: "consoleNavBillingOrders", title: "consoleBillingOrdersTitle", description: "consoleBillingOrdersDescription", permission: "billing:read" },
];

const interfaceDebugSections: Array<{ id: ConsoleSection; icon: typeof LayoutDashboard; label: TranslationKey; title: TranslationKey; description: TranslationKey }> = [
  { id: "interface-debug-text", icon: MessageSquare, label: "consoleNavDebugText", title: "consoleDebugTextTitle", description: "consoleDebugTextDescription" },
  { id: "interface-debug-model", icon: Video, label: "consoleNavDebugModel", title: "consoleDebugModelTitle", description: "consoleDebugModelDescription" },
  { id: "interface-debug-image", icon: Image, label: "consoleNavDebugImage", title: "consoleDebugImageTitle", description: "consoleDebugImageDescription" },
];

function ConsoleSectionButton({ item, active, compact = false, onSelect, t }: { item: ConsoleNavItem; active: boolean; compact?: boolean; onSelect: (section: ConsoleSection) => void; t: (key: TranslationKey) => string }) {
  const Icon = item.icon;
  return <button key={item.id} type="button" onClick={() => onSelect(item.id)} className={cn("flex w-full items-center gap-3 rounded-xl text-left text-sm font-medium transition-all", compact ? "h-10 px-3" : "h-11 px-3", active ? "bg-gradient-to-r from-indigo-600 to-indigo-500 text-white shadow-md shadow-indigo-500/20" : "text-slate-600 hover:bg-slate-100 hover:text-slate-950 dark:text-slate-400 dark:hover:bg-slate-900 dark:hover:text-white")}><Icon className="h-4 w-4 shrink-0" /><span className="truncate">{t(item.label)}</span>{active ? <span className="ml-auto h-1.5 w-1.5 rounded-full bg-white" /> : null}</button>;
}

function ConsoleCompactSectionButton({ item, active, onSelect, t }: { item: ConsoleNavItem; active: boolean; onSelect: (section: ConsoleSection) => void; t: (key: TranslationKey) => string }) {
  const Icon = item.icon;
  return <button key={item.id} type="button" onClick={() => onSelect(item.id)} className={cn("inline-flex shrink-0 items-center gap-2 rounded-lg border px-3 py-2 text-xs font-semibold", active ? "border-indigo-500 bg-indigo-600 text-white" : "border-slate-200 bg-white text-slate-600 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300")}><Icon className="h-3.5 w-3.5" />{t(item.label)}</button>;
}

export function ConsoleView({
  language,
  principal,
  section,
  setSection,
  routeTo,
  handleSignOut,
  tokens,
  tokensBusy,
  tokensMessage,
  refreshTokens,
  editToken,
  pauseToken,
  resumeToken,
  terminateToken,
  deleteToken,
  copyToken,
  tokenSecretAvailable,
  tokenActionBusy,
  openCreateToken,
  billingAccount,
  billingBusy,
  billingMessage,
  refreshBilling,
  enterpriseVerification,
  enterpriseBusy,
  enterpriseMessage,
  refreshEnterprise,
  submitEnterprise,
  paymentProviders,
  paymentBusy,
  paymentMessage,
  paymentOrder,
  createPaymentOrder,
  refreshPaymentOrder,
  capturePayPal,
  paymentOrders,
  paymentOrdersBusy,
  refreshPaymentOrders,
  usageStatus,
  usageBusy,
  usageMessage,
  refreshUsage,
  consoleUsageReport,
  consoleUsageTokens,
  consoleUsageGroups,
  consoleUsageTokenName,
  setConsoleUsageTokenName,
  consoleUsageModel,
  setConsoleUsageModel,
  consoleUsageGroup,
  setConsoleUsageGroup,
  consoleUsageFrom,
  setConsoleUsageFrom,
  consoleUsageTo,
  setConsoleUsageTo,
  apiEndpoints,
  modelStatusEnabled,
  modelStatusReport,
  modelStatusBusy,
  modelStatusMessage,
  refreshModelStatus,
  consoleProfile,
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
  projects,
  projectsBusy,
  projectsMessage,
  refreshProjects,
  saveProject,
  deleteProject,
  projectActionBusy,
  projectDeleteConfirm,
  members,
  membersBusy,
  membersMessage,
  refreshMembers,
  addMember,
  updateMember,
  removeMember,
  memberActionBusy,
  projectMembers,
  projectMembersBusy,
  projectMembersMessage,
  selectedProjectID,
  selectProject,
  refreshProjectMembers,
  addProjectMember,
  updateProjectMember,
  removeProjectMember,
  projectMemberActionBusy,
  models,
}: ConsoleViewProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const hasPermission = (permission?: string) => !permission || principal?.permissions?.includes(permission) === true;
  const visibleSections = consoleSections.filter((item) => item.id !== "model-status" || modelStatusEnabled).filter((item) => hasPermission(item.permission));
  const visibleFinanceSections = financeSections.filter((item) => hasPermission(item.permission));
  const visibleInterfaceDebugSections = interfaceDebugSections;
  const allVisibleSections = [...visibleSections, ...visibleFinanceSections, ...visibleInterfaceDebugSections];
  const displayedSection = allVisibleSections.some((item) => item.id === section) ? section : "dashboard";
  const activeSection = allVisibleSections.find((item) => item.id === displayedSection) || consoleSections[0];
  const navGroupAnchorID = visibleSections.find((item) => item.id === "enterprise")?.id || visibleSections[visibleSections.length - 1]?.id;
  const [financeExpanded, setFinanceExpanded] = useState(true);
  const [interfaceDebugExpanded, setInterfaceDebugExpanded] = useState(true);
  const financeActive = visibleFinanceSections.some((item) => item.id === displayedSection);
  const interfaceDebugActive = visibleInterfaceDebugSections.some((item) => item.id === displayedSection);
  const selectSection = (next: ConsoleSection) => {
    const normalized = next === "usage" ? "billing-records" : next;
    setSection(normalized);
    routeTo(`#console/${normalized}`);
  };
  const activeTokens = tokens.filter((token) => token.status === "active").length;
  const projectCount = projects.length;
  const canManageTenant = hasPermission("member:invite");
  const canManageProjects = hasPermission("project:update");
  const canCreateToken = hasPermission("token:create");
  const canRevokeToken = hasPermission("token:revoke");

  return (
    <div className="flex min-h-[calc(100vh-72px)] w-full bg-slate-50 dark:bg-slate-950">
      <aside className="sticky top-[72px] hidden h-[calc(100vh-72px)] w-[272px] shrink-0 flex-col border-r border-slate-200/80 bg-white/90 px-4 py-5 dark:border-slate-800/80 dark:bg-slate-950/90 lg:flex">
        <div className="mb-6 rounded-2xl border border-indigo-500/20 bg-gradient-to-br from-indigo-500/10 via-cyan-500/5 to-transparent p-4 dark:border-indigo-400/20"><div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-indigo-600 dark:text-indigo-400"><ShieldCheck className="h-4 w-4" />{t("consoleWorkspaceEyebrow")}</div><div className="mt-3 truncate text-sm font-semibold text-slate-900 dark:text-white">{principal?.tenant_id || t("consoleTenantUnknown")}</div><div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("consoleTenantWorkspace")}</div></div>
        <nav className="flex-1 space-y-1" aria-label={t("consoleNavigation")}>
          {visibleSections.map((item) => item.id === navGroupAnchorID ? <React.Fragment key={item.id}><button type="button" onClick={() => setInterfaceDebugExpanded((current) => !current)} aria-expanded={interfaceDebugExpanded} className={cn("flex h-11 w-full items-center gap-3 rounded-xl px-3 text-left text-sm font-semibold transition-all", interfaceDebugActive ? "bg-indigo-500/10 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300" : "text-slate-600 hover:bg-slate-100 hover:text-slate-950 dark:text-slate-400 dark:hover:bg-slate-900 dark:hover:text-white")}><Code2 className="h-4 w-4 shrink-0" /><span className="truncate">{t("consoleNavInterfaceDebug")}</span>{interfaceDebugExpanded ? <ChevronDown className="ml-auto h-4 w-4" /> : <ChevronRight className="ml-auto h-4 w-4" />}</button>{interfaceDebugExpanded ? <div className="ml-4 space-y-1 border-l border-slate-200 pl-3 dark:border-slate-800">{visibleInterfaceDebugSections.map((debug) => <ConsoleSectionButton key={debug.id} item={debug} active={debug.id === displayedSection} compact onSelect={selectSection} t={t} />)}</div> : null}<button type="button" onClick={() => setFinanceExpanded((current) => !current)} aria-expanded={financeExpanded} className={cn("flex h-11 w-full items-center gap-3 rounded-xl px-3 text-left text-sm font-semibold transition-all", financeActive ? "bg-indigo-500/10 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300" : "text-slate-600 hover:bg-slate-100 hover:text-slate-950 dark:text-slate-400 dark:hover:bg-slate-900 dark:hover:text-white")}><BadgeDollarSign className="h-4 w-4 shrink-0" /><span className="truncate">{t("consoleNavFinance")}</span>{financeExpanded ? <ChevronDown className="ml-auto h-4 w-4" /> : <ChevronRight className="ml-auto h-4 w-4" />}</button>{financeExpanded ? <div className="ml-4 space-y-1 border-l border-slate-200 pl-3 dark:border-slate-800">{visibleFinanceSections.map((finance) => <ConsoleSectionButton key={finance.id} item={finance} active={finance.id === displayedSection} compact onSelect={selectSection} t={t} />)}</div> : null}<ConsoleSectionButton item={item} active={item.id === displayedSection} onSelect={selectSection} t={t} /></React.Fragment> : <ConsoleSectionButton key={item.id} item={item} active={item.id === displayedSection} onSelect={selectSection} t={t} />)}
        </nav>
        <div className="space-y-3 border-t border-slate-200/80 pt-4 dark:border-slate-800/80"><Button variant="outline" size="sm" onClick={handleSignOut} className="w-full gap-2 text-xs"><LogOut className="h-3.5 w-3.5" />{t("signOut")}</Button></div>
      </aside>

      <div className="min-w-0 flex-1 p-4 sm:p-6 lg:p-8">
        <div className="mx-auto max-w-[1560px] space-y-6">
          <div className="flex flex-col gap-4 border-b border-slate-200/80 pb-5 dark:border-slate-800/80 sm:flex-row sm:items-center sm:justify-between"><div><div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-indigo-600 dark:text-indigo-400"><span>{t("consoleWorkspaceEyebrow")}</span><span>/</span><span className="text-slate-700 dark:text-slate-300">{t(activeSection.label)}</span></div><h1 className="mt-2 text-2xl font-extrabold tracking-tight text-slate-950 dark:text-white sm:text-3xl">{t(activeSection.title)}</h1><p className="mt-2 max-w-3xl text-sm text-slate-600 dark:text-slate-400">{t(activeSection.description)}</p></div><div className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white/80 px-3.5 py-2 shadow-sm dark:border-slate-800 dark:bg-slate-900/80"><div className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/10 text-xs font-bold uppercase text-indigo-600 dark:bg-indigo-500/20 dark:text-indigo-300">{(principal?.id || "U").slice(0, 2)}</div><div><div className="text-[10px] font-bold uppercase text-slate-400">{t("consoleCurrentUser")}</div><div className="max-w-[180px] truncate font-mono text-xs text-slate-800 dark:text-slate-200">{principal?.id || "-"}</div></div></div></div>

          <div className="flex gap-2 overflow-x-auto pb-1 lg:hidden">{visibleSections.map((item) => item.id === navGroupAnchorID ? <React.Fragment key={item.id}><button type="button" onClick={() => setInterfaceDebugExpanded((current) => !current)} aria-expanded={interfaceDebugExpanded} className={cn("inline-flex shrink-0 items-center gap-2 rounded-lg border px-3 py-2 text-xs font-semibold", interfaceDebugActive ? "border-indigo-500 bg-indigo-600 text-white" : "border-slate-200 bg-white text-slate-600 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300")}><Code2 className="h-3.5 w-3.5" />{t("consoleNavInterfaceDebug")} {interfaceDebugExpanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}</button>{interfaceDebugExpanded ? visibleInterfaceDebugSections.map((debug) => <ConsoleCompactSectionButton key={debug.id} item={debug} active={debug.id === displayedSection} onSelect={selectSection} t={t} />) : null}<button type="button" onClick={() => setFinanceExpanded((current) => !current)} aria-expanded={financeExpanded} className={cn("inline-flex shrink-0 items-center gap-2 rounded-lg border px-3 py-2 text-xs font-semibold", financeActive ? "border-indigo-500 bg-indigo-600 text-white" : "border-slate-200 bg-white text-slate-600 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300")}><BadgeDollarSign className="h-3.5 w-3.5" />{t("consoleNavFinance")} {financeExpanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}</button>{financeExpanded ? visibleFinanceSections.map((finance) => <ConsoleCompactSectionButton key={finance.id} item={finance} active={finance.id === displayedSection} onSelect={selectSection} t={t} />) : null}<ConsoleCompactSectionButton item={item} active={item.id === displayedSection} onSelect={selectSection} t={t} /></React.Fragment> : <ConsoleCompactSectionButton key={item.id} item={item} active={item.id === displayedSection} onSelect={selectSection} t={t} />)}</div>

          {displayedSection === "dashboard" ? <DashboardPanel t={t} tokens={tokens} activeTokens={activeTokens} projectCount={projectCount} principal={principal} usageStatus={usageStatus} usageBusy={usageBusy} canCreateToken={canCreateToken} onNavigate={selectSection} /> : null}
          {displayedSection === "model-status" ? <ModelStatusPanel language={language} report={modelStatusReport} busy={modelStatusBusy} message={modelStatusMessage} refresh={refreshModelStatus} /> : null}
          {displayedSection === "billing-records" ? <UsagePanel language={language} t={t} usageStatus={usageStatus} usageReport={consoleUsageReport} usageBusy={usageBusy} usageMessage={usageMessage} refreshUsage={refreshUsage} tokens={consoleUsageTokens} groups={consoleUsageGroups} tokenName={consoleUsageTokenName} setTokenName={setConsoleUsageTokenName} model={consoleUsageModel} setModel={setConsoleUsageModel} group={consoleUsageGroup} setGroup={setConsoleUsageGroup} from={consoleUsageFrom} setFrom={setConsoleUsageFrom} to={consoleUsageTo} setTo={setConsoleUsageTo} /> : null}
          {displayedSection === "projects" ? <TenantWorkspacePanel language={language} canManageTenant={canManageTenant} canManageProjects={canManageProjects} projects={projects} projectsBusy={projectsBusy} projectsMessage={projectsMessage} refreshProjects={refreshProjects} saveProject={saveProject} deleteProject={deleteProject} projectActionBusy={projectActionBusy} projectDeleteConfirm={projectDeleteConfirm} members={members} membersBusy={membersBusy} membersMessage={membersMessage} refreshMembers={refreshMembers} addMember={addMember} updateMember={updateMember} removeMember={removeMember} memberActionBusy={memberActionBusy} projectMembers={projectMembers} projectMembersBusy={projectMembersBusy} projectMembersMessage={projectMembersMessage} selectedProjectID={selectedProjectID} selectProject={selectProject} refreshProjectMembers={refreshProjectMembers} addProjectMember={addProjectMember} updateProjectMember={updateProjectMember} removeProjectMember={removeProjectMember} projectMemberActionBusy={projectMemberActionBusy} /> : null}
          {displayedSection === "tokens" ? <TokensPanel language={language} t={t} tokens={tokens} tokensBusy={tokensBusy} tokensMessage={tokensMessage} refreshTokens={refreshTokens} editToken={editToken} pauseToken={pauseToken} resumeToken={resumeToken} terminateToken={terminateToken} deleteToken={deleteToken} copyToken={copyToken} tokenSecretAvailable={tokenSecretAvailable} tokenActionBusy={tokenActionBusy} openCreateToken={openCreateToken} canCreateToken={canCreateToken} canRevokeToken={canRevokeToken} canUpdateToken={hasPermission("token:update")} apiEndpoints={apiEndpoints} /> : null}
          {displayedSection === "billing" || displayedSection === "billing-center" ? <BillingPanel language={language} t={t} billingAccount={billingAccount} billingBusy={billingBusy} billingMessage={billingMessage} refreshBilling={refreshBilling} paymentProviders={paymentProviders} paymentBusy={paymentBusy} paymentMessage={paymentMessage} paymentOrder={paymentOrder} createPaymentOrder={createPaymentOrder} refreshPaymentOrder={refreshPaymentOrder} capturePayPal={capturePayPal} /> : null}
          {displayedSection === "billing-orders" ? <PaymentOrdersPanel language={language} t={t} report={paymentOrders} busy={paymentOrdersBusy} refresh={refreshPaymentOrders} /> : null}
          {displayedSection === "interface-debug-text" ? <TextDebugPanel language={language} models={models} apiEndpoints={apiEndpoints} /> : null}
          {displayedSection === "interface-debug-model" ? <VideoLabView language={language} models={models} apiEndpoints={apiEndpoints} routeTo={routeTo} embedded /> : null}
          {displayedSection === "interface-debug-image" ? <ImageLabView language={language} models={models} apiEndpoints={apiEndpoints} routeTo={routeTo} embedded /> : null}
          {displayedSection === "enterprise" ? <EnterpriseVerificationPanel language={language} item={enterpriseVerification} busy={enterpriseBusy} message={enterpriseMessage} refresh={refreshEnterprise} submit={submitEnterprise} /> : null}
          {displayedSection === "profile" ? <ProfilePanel language={language} profile={consoleProfile} profileForm={profileForm} setProfileForm={setProfileForm} emailForm={emailForm} setEmailForm={setEmailForm} passwordForm={passwordForm} setPasswordForm={setPasswordForm} profileBusy={profileBusy} profileMessage={profileMessage} refreshProfile={refreshProfile} saveProfile={saveProfile} saveEmail={saveEmail} savePassword={savePassword} totpEnabled={totpEnabled} mfaStatus={mfaStatus} mfaEnrollment={mfaEnrollment} profileMfaCode={profileMfaCode} setProfileMfaCode={setProfileMfaCode} mfaBusy={mfaBusy} beginMFA={beginMFA} confirmMFA={confirmMFA} cancelMFA={cancelMFA} disableMFA={disableMFA} /> : null}
          {displayedSection === "docs" ? <UsageDocsPanel language={language} routeTo={routeTo} apiEndpoints={apiEndpoints} /> : null}
        </div>
      </div>
    </div>
  );
}

function DashboardPanel({ t, tokens, activeTokens, projectCount, principal, usageStatus, usageBusy, canCreateToken, onNavigate }: { t: (key: TranslationKey) => string; tokens: TokenSummary[]; activeTokens: number; projectCount: number; principal: Principal | null; usageStatus: ConsoleUsageStatus | null; usageBusy: boolean; canCreateToken: boolean; onNavigate: (section: ConsoleSection) => void }) {
  const cards = [{ label: t("consoleStatProjects"), value: String(projectCount), icon: FolderKanban, tone: "indigo" }, { label: t("consoleStatTokens"), value: String(tokens.length), icon: KeyRound, tone: "cyan" }, { label: t("consoleStatActiveTokens"), value: String(activeTokens), icon: CheckCircle2, tone: "emerald" }, { label: t("consoleStatUsage"), value: usageBusy ? "..." : usageStatus?.status === "ready" ? t("consoleReady") : "-", icon: Gauge, tone: "amber" }];
  return <div className="space-y-6"><div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{cards.map((card) => <Card key={card.label} className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2"><CardDescription>{card.label}</CardDescription><div className={cn("flex h-9 w-9 items-center justify-center rounded-xl", card.tone === "emerald" ? "bg-emerald-500/10 text-emerald-600" : card.tone === "cyan" ? "bg-cyan-500/10 text-cyan-600" : card.tone === "amber" ? "bg-amber-500/10 text-amber-600" : "bg-indigo-500/10 text-indigo-600")}><card.icon className="h-4 w-4" /></div></CardHeader><CardContent><div className="text-2xl font-bold text-slate-950 dark:text-white">{card.value}</div></CardContent></Card>)}</div><div className="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(340px,0.8fr)]"><Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Activity className="h-5 w-5 text-indigo-600" />{t("consoleOverviewTitle")}</CardTitle><CardDescription>{t("consoleOverviewHint")}</CardDescription></CardHeader><CardContent className="space-y-3"><div className="flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-950/50"><span className="text-slate-500">{t("consoleTenantID")}</span><code className="max-w-[65%] truncate text-xs text-slate-700 dark:text-slate-300">{principal?.tenant_id || "-"}</code></div><div className="flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-950/50"><span className="text-slate-500">{t("consoleRoles")}</span><span className="font-medium text-slate-800 dark:text-slate-200">{principal?.roles?.join(", ") || "-"}</span></div></CardContent></Card><Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Network className="h-5 w-5 text-cyan-600" />{t("consoleQuickActions")}</CardTitle></CardHeader><CardContent className="space-y-2">{canCreateToken ? <button type="button" onClick={() => onNavigate("tokens")} className="flex w-full items-center gap-3 rounded-xl bg-indigo-500/5 p-3 text-left text-sm text-slate-700 transition-colors hover:bg-indigo-500/10 dark:text-slate-300"><KeyRound className="h-4 w-4 text-indigo-600" />{t("consoleQuickToken")}</button> : null}<button type="button" onClick={() => onNavigate("usage")} className="flex w-full items-center gap-3 rounded-xl bg-cyan-500/5 p-3 text-left text-sm text-slate-700 transition-colors hover:bg-cyan-500/10 dark:text-slate-300"><Activity className="h-4 w-4 text-cyan-600" />{t("consoleQuickUsage")}</button></CardContent></Card></div></div>;
}

function UsagePanel({
  language,
  t,
  usageStatus,
  usageReport,
  usageBusy,
  usageMessage,
  refreshUsage,
  tokens,
  groups,
  tokenName,
  setTokenName,
  model,
  setModel,
  group,
  setGroup,
  from,
  setFrom,
  to,
  setTo,
}: {
  language: Language;
  t: (key: TranslationKey) => string;
  usageStatus: ConsoleUsageStatus | null;
  usageReport: UsageReport | null;
  usageBusy: boolean;
  usageMessage: LoginMessage;
  refreshUsage: (showPending?: boolean, offset?: number) => Promise<void>;
  tokens: TokenSummary[];
  groups: TokenGroupOption[];
  tokenName: string;
  setTokenName: (value: string) => void;
  model: string;
  setModel: (value: string) => void;
  group: string;
  setGroup: (value: string) => void;
  from: string;
  setFrom: (value: string) => void;
  to: string;
  setTo: (value: string) => void;
}) {
  const report = usageReport;
  const summary = report?.summary;
  const hasPrevious = Boolean(report && report.offset > 0);
  const hasNext = Boolean(report && report.offset + report.records.length < report.summary.total_records);
  return (
    <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
      <CardHeader className="flex flex-col gap-4 border-b border-slate-200/80 pb-5 dark:border-slate-800/80 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <CardTitle className="flex items-center gap-2 text-lg"><Activity className="h-5 w-5 text-indigo-600" />{t("consoleUsageTitle")}</CardTitle>
          <CardDescription>{t("consoleUsageDescription")}</CardDescription>
        </div>
        <div className="flex items-center gap-3">
          <div className="hidden text-right sm:block">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">{t("consoleUsageAPIStatus")}</div>
            <div className="mt-1 text-xs font-medium text-slate-700 dark:text-slate-300">{usageStatus?.status === "ready" ? t("consoleReady") : t("consolePending")}</div>
          </div>
          <Button variant="outline" size="sm" onClick={() => void refreshUsage(true)} disabled={usageBusy} className="gap-2"><RefreshCw className={cn("h-4 w-4", usageBusy ? "animate-spin" : "")} />{t("consoleRefresh")}</Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-5 pt-5">
        {usageMessage.text ? <div className="rounded-xl border border-rose-500/30 bg-rose-50 p-3 text-xs text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{usageMessage.text}</div> : null}
        <div className="grid gap-2 rounded-xl border border-slate-200 bg-slate-50/80 p-3 dark:border-slate-800 dark:bg-slate-950/35 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
          <div className="relative sm:col-span-2 lg:col-span-1">
            <Search className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-slate-400" />
            <Input value={model} onChange={(event) => setModel(event.target.value)} placeholder={t("consoleUsageModelFilter")} className="h-9 pl-9 text-xs" />
          </div>
          <select value={tokenName} onChange={(event) => setTokenName(event.target.value)} className="h-9 rounded-xl border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
            <option value="">{t("consoleUsageAllTokens")}</option>
            {tokens.map((token) => <option key={token.id} value={token.name}>{token.name}</option>)}
          </select>
          <select value={group} onChange={(event) => setGroup(event.target.value)} className="h-9 rounded-xl border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
            <option value="">{t("consoleUsageAllGroups")}</option>
            {groups.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
          <Input type="date" value={from} onChange={(event) => setFrom(event.target.value)} className="h-9 text-xs" aria-label={t("consoleUsageFrom")} />
          <Input type="date" value={to} onChange={(event) => setTo(event.target.value)} className="h-9 text-xs" aria-label={t("consoleUsageTo")} />
          <Button type="button" size="sm" onClick={() => void refreshUsage(true, 0)} disabled={usageBusy} className="h-9 gap-2"><Search className="h-3.5 w-3.5" />{t("consoleUsageApplyFilters")}</Button>
        </div>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <UsageMetric label={t("consoleUsageRequests")} value={summary ? formatInteger(summary.total_records) : "-"} tone="indigo" icon={Activity} />
          <UsageMetric label={t("consoleUsageTotalTokens")} value={summary ? formatInteger(summary.total_tokens) : "-"} tone="cyan" icon={Gauge} />
          <UsageMetric label={t("consoleUsageInputOutput")} value={summary ? `${formatInteger(summary.input_tokens)} / ${formatInteger(summary.output_tokens)}` : "-"} detail={summary ? `${t("consoleUsageCachedTokens")} ${formatInteger(summary.cached_input_tokens)} · ${t("consoleUsageReasoningTokens")} ${formatInteger(summary.reasoning_tokens)}` : undefined} tone="amber" icon={Network} />
          <UsageMetric label={t("consoleUsageTotalCost")} value={summary ? formatCostSummary(summary, t("consoleUsageMultipleCurrencies")) : "-"} tone="emerald" icon={BadgeDollarSign} />
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-slate-200 bg-slate-50/80 px-4 py-3 text-xs dark:border-slate-800 dark:bg-slate-950/35">
          <span className="font-semibold text-slate-700 dark:text-slate-200">{t("consoleUsageDetailStatus")}</span>
          <span className="text-slate-500 dark:text-slate-400">{report ? `${report.records.length} / ${report.limit}` : "-"}</span>
          <span className="text-slate-300 dark:text-slate-700">|</span>
          <span className="text-slate-500 dark:text-slate-400">{t("consoleUsageInputOutput")}: {summary ? `${formatInteger(summary.input_tokens)} / ${formatInteger(summary.output_tokens)}` : "-"}</span>
        </div>
        <SummaryMeterBreakdown metrics={summary?.usage_metrics} language={language} label={t("consoleUsageOtherMetrics")} />
        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <Table className="min-w-[1180px]">
            <TableHeader><TableRow><TableHead className="w-[150px]">{t("consoleUsageTime")}</TableHead><TableHead className="w-[180px]">{t("consoleUsageTokenName")}</TableHead><TableHead className="w-[190px]">{t("consoleUsageModel")}</TableHead><TableHead className="w-[180px]">{t("consoleUsageGroup")}</TableHead><TableHead className="w-[150px]">{t("consoleUsageIP")}</TableHead><TableHead className="w-[92px]">{t("consoleUsageLatency")}</TableHead><TableHead className="w-[190px]">{t("consoleUsageTokens")}</TableHead><TableHead className="w-[125px]">{t("consoleUsageCost")}</TableHead><TableHead className="w-[130px]">{t("consoleUsageStatus")}</TableHead></TableRow></TableHeader>
            <TableBody>
              {usageBusy && !report ? <TableRow><TableCell colSpan={9} className="py-12 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-2 h-4 w-4 animate-spin" />{t("consoleUsageLoading")}</TableCell></TableRow> : !report || report.records.length === 0 ? <TableRow><TableCell colSpan={9} className="py-12 text-center text-sm text-slate-500">{t("consoleUsageEmpty")}</TableCell></TableRow> : report.records.map((record) => <TableRow key={record.id}>
                <TableCell className="whitespace-nowrap"><div className="font-mono text-xs font-medium text-slate-700 dark:text-slate-200">{formatDate(record.created_at, language)}</div><div className="mt-1 max-w-[135px] truncate font-mono text-[10px] text-slate-400" title={record.endpoint}>{record.endpoint || "-"}</div></TableCell>
                <TableCell><div className="max-w-[155px] truncate font-semibold text-slate-900 dark:text-white" title={record.token_name || undefined}>{record.token_name || t("consoleUsageUnnamedToken")}</div><div className="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-400">{record.token_prefix ? `${record.token_prefix}...` : "-"}</div></TableCell>
                <TableCell><div className="max-w-[170px] truncate font-semibold text-slate-900 dark:text-white" title={record.model}>{record.model || t("consoleUsageUnknownModel")}</div><div className="mt-1 text-[10px] uppercase tracking-wide text-slate-500 dark:text-slate-400">{record.provider || "-"}</div></TableCell>
                <TableCell><Badge variant="cyan" className="max-w-[165px] truncate" title={record.group_name || record.group_code || undefined}>{record.group_name || record.group_code || t("consoleUsageNoGroup")}</Badge><div className="mt-1 max-w-[165px] truncate font-mono text-[10px] text-slate-500 dark:text-slate-400">{record.group_code || "-"}</div></TableCell>
                <TableCell className="font-mono text-xs text-slate-600 dark:text-slate-300">{record.client_ip || "-"}</TableCell>
                <TableCell className="whitespace-nowrap"><div className="font-mono text-xs text-slate-700 dark:text-slate-200">{record.latency_ms > 0 ? `${(record.latency_ms / 1000).toFixed(2)}s` : "-"}</div><div className="mt-1 text-[10px] text-slate-500 dark:text-slate-400">{record.request_type || t("consoleUsageUnknownRequest")}</div></TableCell>
                <TableCell><UsageTokenBreakdown record={record} t={t} /><MeterBreakdown metrics={record.usage_metrics} language={language} label={t("consoleUsageOtherMetrics")} /></TableCell>
                <TableCell className="whitespace-nowrap font-mono text-xs font-semibold text-emerald-700 dark:text-emerald-300"><div>{record.currency} {record.status === "settlement_pending" ? `~${formatUsageCost(record.estimated_cost)}` : formatUsageCost(record.cost)}</div>{record.status === "settlement_pending" ? <div className="text-[10px] font-normal text-amber-600 dark:text-amber-300">{t("consoleUsageReservedCost")}</div> : null}<ChargeBreakdown lines={record.charge_breakdown} language={language} t={t} /></TableCell>
                <TableCell><UsageStatusBadge status={record.status} failureReason={record.failure_reason} t={t} /></TableCell>
              </TableRow>)}
            </TableBody>
          </Table>
        </div>
        <div className="flex flex-col gap-3 border-t border-slate-200/80 pt-4 text-xs text-slate-500 dark:border-slate-800/80 dark:text-slate-400 sm:flex-row sm:items-center sm:justify-between">
          <span>{report ? `${report.offset + 1}-${report.offset + report.records.length} / ${report.summary.total_records}` : "-"}</span>
          <div className="flex gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => void refreshUsage(true, Math.max(0, (report?.offset || 0) - (report?.limit || 50)))} disabled={usageBusy || !hasPrevious} className="gap-1"><ChevronLeft className="h-3.5 w-3.5" />{t("consoleUsagePrevious")}</Button>
            <Button type="button" variant="outline" size="sm" onClick={() => void refreshUsage(true, (report?.offset || 0) + (report?.limit || 50))} disabled={usageBusy || !hasNext} className="gap-1">{t("consoleUsageNext")}<ChevronRight className="h-3.5 w-3.5" /></Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function UsageMetric({ label, value, detail, tone, icon: Icon }: { label: string; value: string; detail?: string; tone: "indigo" | "cyan" | "amber" | "emerald"; icon: typeof Activity }) { return <div className="rounded-xl border border-slate-200 bg-slate-50/80 p-4 dark:border-slate-800 dark:bg-slate-950/30"><div className="flex items-center justify-between gap-3"><div className="text-xs font-medium text-slate-500 dark:text-slate-400">{label}</div><div className={cn("flex h-8 w-8 shrink-0 items-center justify-center rounded-lg", tone === "cyan" ? "bg-cyan-500/10 text-cyan-600 dark:text-cyan-300" : tone === "amber" ? "bg-amber-500/10 text-amber-600 dark:text-amber-300" : tone === "emerald" ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-300" : "bg-indigo-500/10 text-indigo-600 dark:text-indigo-300")}><Icon className="h-4 w-4" /></div></div><div className="mt-3 font-mono text-xl font-bold text-slate-900 dark:text-white">{value}</div>{detail ? <div className="mt-1 truncate text-[10px] text-slate-500 dark:text-slate-400" title={detail}>{detail}</div> : null}</div>; }
function UsageTokenBreakdown({ record, t }: { record: UsageRecord; t: (key: TranslationKey) => string }) {
  const hasTokens = record.input_tokens > 0 || record.output_tokens > 0 || record.cached_input_tokens > 0 || record.reasoning_tokens > 0;
  if (!hasTokens) return <span className="text-[10px] text-slate-400">{t("consoleUsageNoTokenMeter")}</span>;
  return <div className="grid grid-cols-2 gap-x-4 gap-y-1 whitespace-nowrap font-mono text-[11px]"><span><span className="mr-1 text-cyan-600">↓</span>{t("consoleUsageInputShort")} {formatInteger(record.input_tokens)}</span><span><span className="mr-1 text-indigo-600">↑</span>{t("consoleUsageOutputShort")} {formatInteger(record.output_tokens)}</span><span className="text-slate-500 dark:text-slate-400">{t("consoleUsageCachedShort")} {formatInteger(record.cached_input_tokens)}</span><span className="text-slate-500 dark:text-slate-400">{t("consoleUsageReasoningShort")} {formatInteger(record.reasoning_tokens)}</span></div>;
}
function SummaryMeterBreakdown({ metrics, language, label }: { metrics?: Record<string, string>; language: Language; label: string }) {
  const entries = metricEntries(metrics);
  if (entries.length === 0) return null;
  return <div className="rounded-xl border border-slate-200 bg-slate-50/80 px-4 py-3 text-xs dark:border-slate-800 dark:bg-slate-950/35"><span className="mr-3 font-semibold text-slate-700 dark:text-slate-200">{label}</span><span className="inline-flex flex-wrap gap-x-4 gap-y-1 text-slate-500 dark:text-slate-400">{entries.map(([key, value]) => <span key={key}><span className="text-slate-700 dark:text-slate-300">{metricLabel(key, language)}</span> {formatUsageQuantity(value)}</span>)}</span></div>;
}
function MeterBreakdown({ metrics, language, label }: { metrics?: Record<string, string>; language: Language; label: string }) {
  const entries = metricEntries(metrics);
  if (entries.length === 0) return null;
  return <div className="mt-1 max-w-[190px] truncate text-[10px] text-slate-400" title={entries.map(([key, value]) => `${metricLabel(key, language)}: ${formatUsageQuantity(value)}`).join(" · ")}><span className="mr-1">{label}:</span>{entries.map(([key, value]) => `${metricLabel(key, language)} ${formatUsageQuantity(value)}`).join(" · ")}</div>;
}
function ChargeBreakdown({ lines, language, t }: { lines?: UsageRecord["charge_breakdown"]; language: Language; t: (key: TranslationKey) => string }) {
  if (!lines || lines.length === 0) return null;
  return <details className="mt-1 max-w-[170px] text-[10px] font-normal text-slate-500 dark:text-slate-400"><summary className="cursor-pointer list-none truncate">{t("consoleUsageChargeDetails")} · {lines.length} {t("consoleUsageChargeLines")}</summary><div className="mt-1 space-y-0.5 whitespace-normal font-mono">{lines.map((line) => <div key={`${line.component_code}-${line.unit}`}><span>{metricLabel(line.component_code, language)}</span> {formatUsageQuantity(line.quantity)} × {formatUsageQuantity(line.price_per_unit)} = {formatUsageQuantity(line.amount)}</div>)}</div></details>;
}
function metricEntries(metrics?: Record<string, string>) {
  return Object.entries(metrics || {}).filter(([key, value]) => !["input_tokens", "output_tokens", "cached_input_tokens", "reasoning_tokens"].includes(key) && !isZeroUsageQuantity(value));
}
function metricLabel(key: string, language: Language) {
  const labels: Record<string, [string, string]> = {
    input_images: ["输入图片", "input images"], output_images: ["输出图片", "output images"], input_audio_seconds: ["输入音频秒数", "input audio seconds"], output_audio_seconds: ["输出音频秒数", "output audio seconds"], input_video_seconds: ["输入视频秒数", "input video seconds"], output_video_seconds: ["输出视频秒数", "output video seconds"], input_characters: ["输入字符", "input characters"], output_characters: ["输出字符", "output characters"], requests: ["请求", "requests"], queries: ["查询", "queries"], sessions: ["会话", "sessions"], pages: ["页数", "pages"], storage_gb_days: ["存储 GB-天", "storage GB-days"],
  };
  return labels[key]?.[language === "zh" ? 0 : 1] || key.replace(/_/g, " ");
}
function formatUsageQuantity(value: string) {
  const normalized = String(value ?? "").trim();
  const match = normalized.match(/^([+-]?)(\d+)(?:\.(\d+))?$/);
  if (!match) return normalized || "0";
  const integerPart = match[2].replace(/^0+(?=\d)/, "");
  const fractionPart = (match[3] || "").replace(/0+$/, "");
  return `${match[1] === "-" ? "-" : ""}${integerPart}${fractionPart ? `.${fractionPart}` : ""}`;
}
function formatInteger(value: number) { return new Intl.NumberFormat("en-US", { notation: value >= 10000 ? "compact" : "standard", maximumFractionDigits: 1 }).format(value || 0); }
function formatMoney(value?: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "0";
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 3 }).format(parsed);
}
function formatUsageCost(value?: string) {
  const normalized = String(value ?? "").trim();
  const match = normalized.match(/^([+-]?)(\d+)(?:\.(\d+))?$/);
  if (!match) return "0";

  const integerPart = match[2].replace(/^0+(?=\d)/, "");
  const fractionPart = (match[3] || "").replace(/0+$/, "");
  if (/^0+$/.test(integerPart) && fractionPart.length === 0) return "0";
  return `${match[1] === "-" ? "-" : ""}${integerPart}${fractionPart ? `.${fractionPart}` : ""}`;
}
function isZeroUsageQuantity(value: string) {
  return /^[-+]?0(?:\.0*)?$/.test(String(value ?? "").trim());
}
function formatCostSummary(summary: UsageReport["summary"], multipleLabel: string) {
  const entries = Object.entries(summary.cost_by_currency || {}).sort(([left], [right]) => left.localeCompare(right));
  if (entries.length === 0) return "0";
  if (entries.length === 1) {
    const [currency, amount] = entries[0];
    return `${currency} ${formatUsageCost(amount)}`;
  }
  return `${multipleLabel}: ${entries.map(([currency, amount]) => `${currency} ${formatUsageCost(amount)}`).join(" · ")}`;
}
function UsageStatusBadge({ status, failureReason, t }: { status: string; failureReason?: string; t: (key: TranslationKey) => string }) {
  const normalized = status.trim().toLowerCase();
  const label = normalized === "settled" ? t("consoleUsageStatusSettled") : normalized === "failed" ? t("consoleUsageStatusFailed") : normalized === "started" ? t("consoleUsageStatusStarted") : normalized === "settlement_pending" ? t("consoleUsageStatusPending") : status || "-";
  const variant = normalized === "settled" ? "success" : normalized === "failed" ? "destructive" : normalized === "settlement_pending" ? "warning" : normalized === "started" ? "cyan" : "muted";
  return <div className="space-y-1"><Badge variant={variant} title={failureReason || undefined}>{label}</Badge>{failureReason ? <div className="max-w-[180px] truncate text-[10px] text-rose-600 dark:text-rose-300" title={failureReason}>{failureReason}</div> : null}</div>;
}
function formatDate(value: string, language: Language) { const date = new Date(value); return Number.isNaN(date.getTime()) ? "-" : new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "short", timeStyle: "medium" }).format(date); }

function TokensPanel({
  language,
  t,
  tokens,
  tokensBusy,
  tokensMessage,
  refreshTokens,
  editToken,
  pauseToken,
  resumeToken,
  terminateToken,
  deleteToken,
  copyToken,
  tokenSecretAvailable,
  tokenActionBusy,
  openCreateToken,
  canCreateToken,
  canRevokeToken,
  canUpdateToken,
  apiEndpoints,
}: {
  language: Language;
  t: (key: TranslationKey) => string;
  tokens: TokenSummary[];
  tokensBusy: boolean;
  tokensMessage: LoginMessage;
  refreshTokens: (showPending?: boolean) => Promise<void>;
  editToken: (token: TokenSummary) => void;
  pauseToken: (token: TokenSummary) => Promise<void>;
  resumeToken: (token: TokenSummary) => Promise<void>;
  terminateToken: (token: TokenSummary) => Promise<void>;
  deleteToken: (token: TokenSummary) => Promise<void>;
  copyToken: (token: TokenSummary) => Promise<void>;
  tokenSecretAvailable: (token: TokenSummary) => boolean;
  tokenActionBusy: string;
  openCreateToken: () => void;
  canCreateToken: boolean;
  canRevokeToken: boolean;
  canUpdateToken: boolean;
  apiEndpoints: PublicAPIEndpoint[];
}) {
  const formatTime = (value?: string) => value ? new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "-";
  const [copiedEndpointID, setCopiedEndpointID] = useState("");
  async function copyEndpoint(endpoint: PublicAPIEndpoint, protocol: "openai" | "anthropic") {
    const urls = resolveAPIEndpointURLs(endpoint);
    const value = protocol === "openai" ? urls.openai : urls.anthropic;
    const copyID = `${endpoint.base_url}:${protocol}`;
    let copied: boolean;
    try {
      await navigator.clipboard.writeText(value);
      copied = true;
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = value;
      textarea.setAttribute("readonly", "");
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      copied = document.execCommand("copy");
      document.body.removeChild(textarea);
    }
    if (copied) {
      setCopiedEndpointID(copyID);
      window.setTimeout(() => setCopiedEndpointID((current) => current === copyID ? "" : current), 1600);
    }
  }

  return (
    <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
      <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <CardTitle className="flex items-center gap-2 text-lg"><KeyRound className="h-5 w-5 text-indigo-600" />{t("tokensTableTitle")}</CardTitle>
          <CardDescription>{t("tokensConsoleTableHint")}</CardDescription>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => void refreshTokens(true)} disabled={tokensBusy} className="gap-2"><RefreshCw className={cn("h-4 w-4", tokensBusy ? "animate-spin" : "")} />{t("tokensRefresh")}</Button>
          {canCreateToken ? <Button size="sm" onClick={openCreateToken} className="gap-2"><Plus className="h-4 w-4" />{t("tokensCreateAction")}</Button> : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <APIEndpointAddresses t={t} apiEndpoints={apiEndpoints} copiedEndpointID={copiedEndpointID} copyEndpoint={copyEndpoint} />
        {tokensMessage.text ? <div className="rounded-xl border border-amber-500/30 bg-amber-50 p-3 text-xs text-amber-800 dark:bg-amber-500/10 dark:text-amber-200">{tokensMessage.text}</div> : null}
        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <Table className="min-w-[920px]">
            <TableHeader><TableRow><TableHead>{t("tokensName")}</TableHead><TableHead>{t("tokensPrefix")}</TableHead><TableHead>{t("tokensProjectID")}</TableHead><TableHead>{t("tokensGroup")}</TableHead><TableHead>{t("tokensSpendLimit")}</TableHead><TableHead>{t("tokensStatus")}</TableHead><TableHead>{t("tokensCreated")}</TableHead><TableHead className="text-right">{t("tokensActions")}</TableHead></TableRow></TableHeader>
            <TableBody>
              {tokensBusy && tokens.length === 0 ? <TableRow><TableCell colSpan={8} className="py-12 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin text-indigo-600" />{t("tokensLoading")}</TableCell></TableRow> : tokens.length === 0 ? <TableRow><TableCell colSpan={8} className="py-12 text-center text-sm text-slate-500">{t("tokensEmpty")}</TableCell></TableRow> : tokens.map((token) => { const spendLimit = token.spend_limit || "0"; const spentAmount = token.spent_amount || "0"; return <TableRow key={token.id}><TableCell><div className="font-semibold text-slate-900 dark:text-white">{token.name}</div><div className="mt-1 font-mono text-[10px] text-slate-500">{token.id}</div></TableCell><TableCell><code className="rounded-md bg-slate-100 px-2 py-1 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300">{token.token_prefix}...</code></TableCell><TableCell className="font-mono text-xs text-slate-600 dark:text-slate-400">{token.project_id}</TableCell><TableCell><Badge variant="muted">{token.group_code || t("tokensNoGroup")}</Badge></TableCell><TableCell className="whitespace-nowrap text-xs text-slate-600 dark:text-slate-300"><div>{formatDecimalWithoutTrailingZeros(spentAmount, "0")} / {spendLimit === "0" ? t("tokensUnlimited") : formatDecimalWithoutTrailingZeros(spendLimit)}</div><div className="mt-1 text-[10px] text-slate-400">{t("tokensSpentAmount")}</div></TableCell><TableCell><Badge variant={token.status === "active" ? "success" : token.status === "disabled" ? "warning" : token.status === "expired" ? "warning" : "muted"}>{token.status === "active" ? t("tokensStatusActive") : token.status === "disabled" ? t("tokensStatusPaused") : token.status === "expired" ? t("tokensStatusExpired") : t("tokensStatusRevoked")}</Badge></TableCell><TableCell className="whitespace-nowrap text-xs text-slate-500">{formatTime(token.created_at)}</TableCell><TableCell><div className="flex justify-end gap-1"><Button type="button" variant="ghost" size="icon" onClick={() => editToken(token)} disabled={!canUpdateToken || token.status === "revoked" || token.status === "expired" || tokenActionBusy === token.id} title={t("tokensEdit")} aria-label={t("tokensEdit")}><Pencil className="h-4 w-4" /></Button>{token.status === "active" && canUpdateToken ? <Button type="button" variant="ghost" size="icon" onClick={() => void pauseToken(token)} disabled={tokenActionBusy === token.id} title={t("tokensPause")} aria-label={t("tokensPause")}><Pause className="h-4 w-4" /></Button> : token.status === "disabled" && canUpdateToken ? <Button type="button" variant="ghost" size="icon" onClick={() => void resumeToken(token)} disabled={tokenActionBusy === token.id} title={t("tokensResume")} aria-label={t("tokensResume")}><Play className="h-4 w-4" /></Button> : null}<Button type="button" variant="ghost" size="icon" onClick={() => void copyToken(token)} disabled={!tokenSecretAvailable(token) || tokenActionBusy === token.id} title={tokenSecretAvailable(token) ? t("tokensCopySecret") : t("tokensSecretUnavailable")} aria-label={t("tokensCopySecret")}><Copy className="h-4 w-4" /></Button>{canRevokeToken && token.status !== "revoked" && token.status !== "expired" ? <Button type="button" variant="ghost" size="icon" onClick={() => void terminateToken(token)} disabled={tokenActionBusy === token.id} title={t("tokensTerminate")} aria-label={t("tokensTerminate")}><Ban className="h-4 w-4 text-amber-600" /></Button> : null}{canRevokeToken ? <Button type="button" variant="ghost" size="icon" onClick={() => void deleteToken(token)} disabled={tokenActionBusy === token.id} title={t("tokensDelete")} aria-label={t("tokensDelete")}><Trash2 className="h-4 w-4 text-rose-600" /></Button> : null}</div></TableCell></TableRow>; })}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  );
}

function APIEndpointAddresses({ t, apiEndpoints, copiedEndpointID, copyEndpoint }: { t: (key: TranslationKey) => string; apiEndpoints: PublicAPIEndpoint[]; copiedEndpointID: string; copyEndpoint: (endpoint: PublicAPIEndpoint, protocol: "openai" | "anthropic") => Promise<void> }) {
  return <section className="space-y-3 rounded-xl border border-cyan-500/20 bg-cyan-50/60 p-4 dark:border-cyan-400/20 dark:bg-cyan-500/10" aria-labelledby="tokens-api-endpoints-title"><div><h3 id="tokens-api-endpoints-title" className="flex items-center gap-2 text-sm font-semibold text-slate-900 dark:text-white"><Network className="h-4 w-4 text-cyan-600" />{t("tokensAPIEndpointsTitle")}</h3><p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">{t("tokensAPIEndpointsDescription")}</p></div>{apiEndpoints.length === 0 ? <p className="rounded-lg border border-dashed border-slate-300 px-3 py-6 text-center text-xs text-slate-500 dark:border-slate-700 dark:text-slate-400">{t("tokensAPIEndpointsEmpty")}</p> : <div className="space-y-3">{apiEndpoints.map((endpoint) => { const urls = resolveAPIEndpointURLs(endpoint); const openAICopyID = `${endpoint.base_url}:openai`; const anthropicCopyID = `${endpoint.base_url}:anthropic`; return <div key={endpoint.base_url} className="rounded-lg border border-cyan-500/15 bg-white/75 p-3 dark:border-cyan-400/15 dark:bg-slate-950/35"><div className="text-xs font-semibold text-slate-900 dark:text-white">{endpoint.name}</div><div className="mt-3 grid gap-2 lg:grid-cols-2"><EndpointAddress label={t("tokensAPIEndpointOpenAI")} url={urls.openai} recommended copied={copiedEndpointID === openAICopyID} onCopy={() => void copyEndpoint(endpoint, "openai")} t={t} /><EndpointAddress label={t("tokensAPIEndpointAnthropic")} url={urls.anthropic} copied={copiedEndpointID === anthropicCopyID} onCopy={() => void copyEndpoint(endpoint, "anthropic")} t={t} /></div></div>; })}</div>}</section>;
}

function EndpointAddress({ label, url, recommended = false, copied, onCopy, t }: { label: string; url: string; recommended?: boolean; copied: boolean; onCopy: () => void; t: (key: TranslationKey) => string }) {
  return <div className={cn("flex min-w-0 items-center justify-between gap-3 rounded-lg border px-3 py-2.5", recommended ? "border-indigo-500/25 bg-indigo-500/[0.06]" : "border-slate-200 bg-white/70 dark:border-slate-800 dark:bg-slate-900/50")}><div className="min-w-0"><div className="flex flex-wrap items-center gap-2 text-[11px] font-semibold text-slate-900 dark:text-white"><span>{label}</span>{recommended ? <Badge variant="cyan">{t("tokensAPIEndpointRecommended")}</Badge> : null}</div><code className="mt-1 block break-all text-[11px] text-slate-600 dark:text-slate-300">{url || "-"}</code></div><Button type="button" variant="outline" size="icon" onClick={onCopy} disabled={!url} title={t("tokensAPIEndpointCopy")} aria-label={t("tokensAPIEndpointCopy")} className="shrink-0"><span className="sr-only">{copied ? t("tokensAPIEndpointCopied") : t("tokensAPIEndpointCopy")}</span>{copied ? <CheckCircle2 className="h-3.5 w-3.5 text-emerald-600" /> : <Copy className="h-3.5 w-3.5" />}</Button></div>;
}

function BillingPanel({ language, t, billingAccount, billingBusy, billingMessage, refreshBilling, paymentProviders, paymentBusy, paymentMessage, paymentOrder, createPaymentOrder, refreshPaymentOrder, capturePayPal }: { language: Language; t: (key: TranslationKey) => string; billingAccount: BillingAccount | null; billingBusy: boolean; billingMessage: LoginMessage; refreshBilling: () => Promise<void>; paymentProviders: PublicPaymentProvider[]; paymentBusy: boolean; paymentMessage: LoginMessage; paymentOrder: PaymentOrder | null; createPaymentOrder: (provider: PaymentOrder["provider"], amount: string, currency: string, packageID?: string) => Promise<void>; refreshPaymentOrder: () => Promise<void>; capturePayPal: () => Promise<void> }) {
  return <div className="space-y-6"><Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader className="flex flex-row items-start justify-between gap-4"><div><CardTitle className="flex items-center gap-2 text-lg"><BadgeDollarSign className="h-5 w-5 text-emerald-600" />{t("consoleBillingTitle")}</CardTitle><CardDescription>{t("consoleBillingDescription")}</CardDescription></div><Button variant="outline" size="sm" onClick={() => void refreshBilling()} disabled={billingBusy} className="gap-2"><RefreshCw className={cn("h-4 w-4", billingBusy ? "animate-spin" : "")} />{t("consoleRefresh")}</Button></CardHeader><CardContent className="space-y-4">{billingMessage.text ? <div className="rounded-xl border border-amber-500/30 bg-amber-50 p-3 text-xs text-amber-800 dark:bg-amber-500/10 dark:text-amber-200">{billingMessage.text}</div> : null}{billingAccount ? <div className="grid gap-4 md:grid-cols-3"><div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-5"><div className="text-xs text-slate-500">{t("consoleBillingBalance")}</div><div className="mt-2 text-3xl font-bold text-emerald-700 dark:text-emerald-300">{billingAccount.currency} {formatMoney(billingAccount.balance)}</div></div><div className="rounded-xl border border-slate-200 p-5 dark:border-slate-800"><div className="text-xs text-slate-500">{t("consoleBillingStatus")}</div><div className="mt-2"><Badge variant="success">{billingAccount.status}</Badge></div></div><div className="rounded-xl border border-slate-200 p-5 dark:border-slate-800"><div className="text-xs text-slate-500">{t("consoleBillingAccountID")}</div><div className="mt-2 truncate font-mono text-xs text-slate-700 dark:text-slate-300">{billingAccount.id}</div></div></div> : <div className="rounded-xl border border-dashed border-slate-300 py-16 text-center text-sm text-slate-500 dark:border-slate-700">{t("consoleBillingNoAccount")}</div>}</CardContent></Card><PaymentRechargePanel language={language} account={billingAccount} providers={paymentProviders} busy={paymentBusy} message={paymentMessage} order={paymentOrder} createOrder={createPaymentOrder} refreshOrder={refreshPaymentOrder} capturePayPal={capturePayPal} /></div>;
}

function PaymentOrdersPanel({ language, t, report, busy, refresh }: { language: Language; t: (key: TranslationKey) => string; report: PaymentOrderList | null; busy: boolean; refresh: (offset?: number) => Promise<void> }) {
  const orders = report?.orders || [];
  const hasPrevious = Boolean(report && report.offset > 0);
  const hasNext = Boolean(report && report.offset + orders.length < report.total);
  const providerLabels: Record<PaymentOrder["provider"], TranslationKey> = { wechat: "paymentWechat", alipay: "paymentAlipay", stripe: "paymentStripe", paypal: "paymentPayPal" };
  const statusLabels: Record<string, TranslationKey> = { paid: "rechargeStatusPaid", pending: "rechargeStatusPending", failed: "rechargeStatusFailed", expired: "rechargeStatusExpired", cancelled: "rechargeStatusCancelled" };
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader className="flex flex-row items-start justify-between gap-4"><div><CardTitle className="flex items-center gap-2 text-lg"><BadgeDollarSign className="h-5 w-5 text-indigo-600" />{t("consoleBillingOrdersTitle")}</CardTitle><CardDescription>{t("consoleBillingOrdersDescription")}</CardDescription></div><Button variant="outline" size="sm" onClick={() => void refresh(report?.offset || 0)} disabled={busy} className="gap-2"><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} />{t("consoleRefresh")}</Button></CardHeader><CardContent>{busy && !report ? <div className="py-12 text-center text-sm text-slate-500">{t("consoleBillingOrdersLoading")}</div> : orders.length === 0 ? <div className="rounded-xl border border-dashed border-slate-300 py-16 text-center text-sm text-slate-500 dark:border-slate-700">{t("consoleBillingOrdersEmpty")}</div> : <><div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800"><table className="w-full min-w-[860px] text-left text-sm"><thead className="bg-slate-50 text-xs text-slate-500 dark:bg-slate-950/60"><tr><th className="px-4 py-3">{t("rechargeOrderTitle")}</th><th className="px-4 py-3">{t("rechargeProvider")}</th><th className="px-4 py-3">{t("rechargeAmount")}</th><th className="px-4 py-3">{t("rechargeCreditedAmount")}</th><th className="px-4 py-3">{t("consoleBillingOrdersStatus")}</th><th className="px-4 py-3">{t("consoleBillingOrdersCreated")}</th><th className="px-4 py-3">{t("consoleBillingOrdersAction")}</th></tr></thead><tbody className="divide-y divide-slate-200 dark:divide-slate-800">{orders.map((order) => <tr key={order.id}><td className="px-4 py-3"><div className="font-mono text-xs text-slate-900 dark:text-white">{order.merchant_order_no}</div><div className="mt-1 font-mono text-[10px] text-slate-500">{order.id}</div></td><td className="px-4 py-3">{t(providerLabels[order.provider])}</td><td className="px-4 py-3 font-semibold">{order.currency} {formatMoney(order.amount)}</td><td className="px-4 py-3 font-semibold text-emerald-700 dark:text-emerald-300">{order.currency} {formatMoney(order.credited_amount || order.amount)}</td><td className="px-4 py-3"><Badge variant={order.status === "paid" ? "success" : order.status === "failed" || order.status === "expired" ? "destructive" : "warning"}>{t(statusLabels[order.status] || "rechargeStatusPending")}</Badge></td><td className="whitespace-nowrap px-4 py-3 text-xs text-slate-500">{formatDate(order.created_at, language)}</td><td className="px-4 py-3">{order.checkout_url && order.status === "pending" ? <Button type="button" variant="outline" size="icon" onClick={() => window.location.assign(order.checkout_url || "")} title={t("rechargeOpenPayment")} aria-label={t("rechargeOpenPayment")}><ExternalLink className="h-4 w-4" /></Button> : <span className="text-xs text-slate-400">-</span>}</td></tr>)}</tbody></table></div><div className="mt-4 flex items-center justify-between text-xs text-slate-500"><span>{report ? `${report.offset + 1}-${report.offset + orders.length} / ${report.total}` : "-"}</span><div className="flex gap-2"><Button type="button" variant="outline" size="sm" onClick={() => void refresh(Math.max(0, (report?.offset || 0) - (report?.limit || 20)))} disabled={busy || !hasPrevious}><ChevronLeft className="h-3.5 w-3.5" /></Button><Button type="button" variant="outline" size="sm" onClick={() => void refresh((report?.offset || 0) + (report?.limit || 20))} disabled={busy || !hasNext}><ChevronRight className="h-3.5 w-3.5" /></Button></div></div></>}</CardContent></Card>;
}
