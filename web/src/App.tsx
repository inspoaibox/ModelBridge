import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { Navbar } from "@/components/layout/Navbar";
import { Footer } from "@/components/layout/Footer";
import { ChannelModal } from "@/components/ChannelModal";
import { GroupModal } from "@/components/GroupModal";
import { TokenCreateModal } from "@/components/TokenCreateModal";
import { UserModal } from "@/components/UserModal";
import { ModelPriceModal } from "@/components/ModelPriceModal";
import { NotFoundView } from "@/components/NotFoundView";
import { ResetPasswordView } from "@/components/ResetPasswordView";
import { EmailVerificationView } from "@/components/EmailVerificationView";
import { StepUpDialog } from "@/components/StepUpDialog";
import { resolveAPIEndpointURLs } from "@/lib/api-endpoint";
import { formatDecimalWithoutTrailingZeros } from "@/lib/utils";
import {
  AdminSection,
  APIEndpoint,
  APIEndpointFormState,
  AuditReport,
  Audience,
  BillingAccount,
  EnterpriseVerification,
  ChannelFormModel,
  ChannelFormState,
  ChannelSummary,
  CreditFormState,
  ConsoleSection,
  ConsoleUsageStatus,
  DiscoveredModel,
  GroupFormState,
  GroupSummary,
  IssuedTokenResponse,
  Language,
  LoginMessage,
  LoginSettings,
  ConsoleProfile,
  EmailFormState,
	EmailSettings,
	FeatureSettings,
	EmailTemplate,
	EmailTemplateFormState,
  	FinanceReport,
	OperationsSnapshot,
  MFAEnrollment,
  MFAStatus,
  PasswordFormState,
  ProfileFormState,
  PriceMatrixSummary,
  PaymentOrder,
  PaymentOrderList,
  PaymentProviderConfig,
  PaymentRechargePackages,
  PaymentRechargePackage,
  PublicPaymentProvider,
  ModelPriceFormState,
  ModelMonitor,
  ModelMonitorFormState,
  ModelStatusReport,
  PublicFeatureSettings,
  PublicAPIEndpoint,
  PlatformRole,
  PlatformPermission,
  PlatformRoleFormState,
  Principal,
  PublicModelSummary,
  ProjectFormState,
  ProjectMember,
  ProjectSummary,
  SiteSettings,
  SectionRoute,
  SecuritySettings,
  SMTPSettingsForm,
  SystemSettings,
  Theme,
  TokenSummary,
  TokenCreateFormState,
  TokenGroupOption,
  TranslationKey,
  TenantMember,
  UsageReport,
  ConsoleDashboardReport,
  UserAdminFormState,
  UserSummary,
} from "@/types";
import { translations } from "@/locales/translations";

const adminEntryPathPattern = /^\/admin-[A-Za-z0-9_-]{16,160}$/;
const HomeView = lazy(() => import("@/components/HomeView").then((module) => ({ default: module.HomeView })));
const ModelPlazaView = lazy(() => import("@/components/ModelPlazaView").then((module) => ({ default: module.ModelPlazaView })));
const LoginView = lazy(() => import("@/components/LoginView").then((module) => ({ default: module.LoginView })));
const RegisterView = lazy(() => import("@/components/RegisterView").then((module) => ({ default: module.RegisterView })));
const ConsoleView = lazy(() => import("@/components/ConsoleView").then((module) => ({ default: module.ConsoleView })));
const AdminConsole = lazy(() => import("@/components/admin/AdminConsole").then((module) => ({ default: module.AdminConsole })));

function isAdminEntryLocation() {
  return adminEntryPathPattern.test(window.location.pathname);
}

function isValidAdminEntryPath(value?: string) {
  return Boolean(value && adminEntryPathPattern.test(value));
}

function paymentReturnURL() {
  const url = new URL(window.location.origin + "/");
  url.hash = "#console/billing";
  return url.toString();
}

function tokenSecretStorageKey(principal: Principal | null) {
  return principal?.id && principal.tenant_id ? `ai-token-console-token-secrets:${principal.id}:${principal.tenant_id}` : "";
}

function readTokenSecrets(key: string): Record<string, string> {
  if (!key) return {};
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(key) || "{}");
    if (!parsed || typeof parsed !== "object") return {};
    return Object.fromEntries(Object.entries(parsed).filter(([, value]) => typeof value === "string" && value.trim() !== ""));
  } catch {
    return {};
  }
}

function parseRoute(hash: string): SectionRoute {
  const raw = hash.replace(/^#/, "");
  if (isAdminEntryLocation() && (raw === "" || raw === "admin-login")) {
    return { view: "admin-login", section: "dashboard" };
  }
  if (raw === "" || raw === "home") {
    return { view: "home", section: "dashboard" };
  }
  if (raw === "login") {
    return { view: "login", section: "dashboard" };
  }
  if (raw === "register") {
    return { view: "register", section: "dashboard" };
  }
  if (raw === "reset" || raw.startsWith("reset?")) {
    const [, query = ""] = raw.split("?", 2);
    return { view: "reset", section: "dashboard", reset_token: new URLSearchParams(query).get("token") || "" };
  }
  if (raw === "verify-email" || raw.startsWith("verify-email?")) {
    const [, query = ""] = raw.split("?", 2);
    return { view: "verify-email", section: "dashboard", verification_token: new URLSearchParams(query).get("token") || "" };
  }
  if (raw === "models") {
    return { view: "models", section: "dashboard" };
  }
  if (raw === "admin" || raw.startsWith("admin/")) {
    if (!isAdminEntryLocation()) {
      return { view: "not-found", section: "dashboard" };
    }
    const [, section = "dashboard"] = raw.split("/");
    return { view: "admin", section: normalizeSection(section) };
  }
  if (raw === "console" || raw.startsWith("console/")) {
    const [, section = "dashboard"] = raw.split("/");
    return { view: "console", section: "dashboard", console_section: normalizeConsoleSection(section) };
  }
  return { view: "not-found", section: "dashboard" };
}

function normalizeConsoleSection(value: string): ConsoleSection {
  if (value === "security") return "profile";
  if (value === "usage") return "billing-records";
  if (value === "billing-center") return "billing";
  return value === "model-status" || value === "usage" || value === "projects" || value === "tokens" || value === "billing" || value === "billing-records" || value === "billing-center" || value === "billing-orders" || value === "interface-debug-text" || value === "interface-debug-model" || value === "interface-debug-image" || value === "enterprise" || value === "profile" || value === "docs"
    ? value
    : "dashboard";
}

function normalizeSection(value: string): AdminSection {
  if (value === "security") return "settings";
  return value === "ops" ||
    value === "model-status" ||
    value === "users" ||
    value === "roles" ||
    value === "groups" ||
    value === "tokens" ||
    value === "channels" ||
    value === "billing" ||
    value === "finance" ||
    value === "account-finance" ||
    value === "usage" ||
    value === "audit" ||
    value === "enterprise" ||
    value === "settings"
    ? value
    : "dashboard";
}

function newFormRowID() {
  return Math.random().toString(36).slice(2, 10);
}

function parseTokenList(value: string) {
  return value.split(/[\n,]/).map((item) => item.trim()).filter(Boolean);
}

function defaultProviderBaseURL(provider: "openai" | "anthropic" | "grok" | "gemini" | "volcengine") {
  switch (provider) {
    case "anthropic":
      return "https://api.anthropic.com";
    case "grok":
      return "https://api.x.ai/v1";
    case "gemini":
      return "https://generativelanguage.googleapis.com";
    case "volcengine":
      return "https://ark.cn-beijing.volces.com/api/v3";
    default:
      return "https://api.openai.com/v1";
  }
}

function defaultChannelForm(provider: "openai" | "anthropic" | "grok" | "gemini" | "volcengine" = "openai"): ChannelFormState {
  return {
    id: "",
    name:
      provider === "openai"
        ? "OpenAI Official"
        : provider === "anthropic"
        ? "Anthropic Official"
        : provider === "grok"
        ? "Grok Official"
        : provider === "gemini"
        ? "Gemini Official"
        : "Volcano Ark Official",
    provider,
    base_url: defaultProviderBaseURL(provider),
    api_key: "",
    status: "active",
    upstream_cost_discount: "1",
    upstream_integration: "official",
    upstream_account_credential: "",
    upstream_account_user_id: "",
    upstream_account_credential_configured: false,
    clear_upstream_account_credential: false,
    priority: 100,
    weight: 100,
    // Model mappings are explicit. A new channel must never inherit a
    // model from another provider or silently expose a platform default.
    models: [],
  };
}

function channelFormFromSummary(channel: ChannelSummary): ChannelFormState {
  const provider =
    channel.provider === "anthropic" ||
    channel.provider === "grok" ||
    channel.provider === "gemini" ||
    channel.provider === "volcengine"
      ? channel.provider
      : "openai";
  return {
    id: channel.id,
    name: channel.name,
    provider,
    base_url: channel.base_url || defaultProviderBaseURL(provider),
    api_key: "",
    status:
      channel.status === "draining"
        ? "draining"
        : channel.status === "disabled"
        ? "disabled"
        : "active",
    upstream_cost_discount: formatDecimalWithoutTrailingZeros(channel.upstream_cost_discount, "1"),
    upstream_integration:
      channel.upstream_integration === "newapi" ||
      channel.upstream_integration === "sub2api" ||
      channel.upstream_integration === "other"
        ? channel.upstream_integration
        : "official",
    upstream_account_credential: "",
    upstream_account_user_id: channel.upstream_account_user_id || "",
    upstream_account_credential_configured: channel.has_upstream_account_credential === true,
    clear_upstream_account_credential: false,
    priority: channel.priority,
    weight: channel.weight,
    models:
      channel.models.length > 0
        ? channel.models.map((model) => ({
            id: newFormRowID(),
            model: model.model,
            upstream_model: model.upstream_model || model.model,
            enabled: model.enabled,
          }))
        : [],
  };
}

function defaultModelPriceForm(): ModelPriceFormState {
  return {
    model_id: "",
    provider: "",
    model: "",
    currency: "USD",
    source: "unconfigured",
    input_price_per_million_tokens: "",
    output_price_per_million_tokens: "",
    cached_input_price_per_million_tokens: "",
    cache_creation_price_per_million_tokens: "",
    reasoning_price_per_million_tokens: "",
    components: [],
  };
}

function defaultProfileForm(): ProfileFormState {
  return { display_name: "" };
}

function defaultEmailForm(): EmailFormState {
  return { email: "", current_password: "" };
}

function defaultPasswordForm(): PasswordFormState {
  return { current_password: "", new_password: "", confirm_password: "" };
}

function millionPriceToUnit(value: string) {
  const trimmed = value.trim();
  if (!trimmed) return "";
  return decimalShift(trimmed, -6);
}

function defaultCreditForm(): CreditFormState {
  return {
    tenant_id: "",
    currency: "USD",
    amount: "",
    reason: "",
    direction: "credit",
  };
}

function defaultSiteSettings(): SiteSettings {
  return { site_name: "AI Token Gateway", site_logo_url: "", site_favicon_url: "" };
}

function defaultAPIEndpointForm(): APIEndpointFormState {
  return { id: "", name: "", base_url: "", enabled: true };
}

function defaultSMTPSettings(): SMTPSettingsForm {
  return { smtp_host: "", smtp_port: 587, smtp_username: "", smtp_password: "", smtp_password_clear: false, smtp_from_email: "", smtp_from_name: "", smtp_tls: true, public_base_url: "" };
}

function defaultFeatureSettings(): FeatureSettings {
	return {
		email_enabled: false,
		registration_enabled: true,
		model_status_enabled: true,
		totp_enabled: false,
		step_up_channel_model_enabled: false,
		step_up_group_enabled: false,
		step_up_token_enabled: false,
		step_up_user_enabled: false,
		step_up_role_enabled: false,
		step_up_billing_enabled: false,
		step_up_system_enabled: false,
		email_verification_enabled: true,
		email_password_reset_enabled: true,
		email_subscription_enabled: true,
		email_low_balance_alert_enabled: false,
		email_recharge_success_enabled: false,
		email_usage_limit_alert_enabled: false,
		email_content_audit_enabled: false,
		email_account_disabled_enabled: false,
		email_cyber_policy_enabled: false,
		email_operations_enabled: false,
		balance_threshold: "0",
		recharge_url: "",
	};
}

function defaultGroupForm(): GroupFormState {
  return {
    id: "",
    code: "",
    name: "",
    description: "",
    status: "active",
    multiplier: "1",
    rpm_limit: 0,
    billing_type: "prepaid",
    metering_mode: "token",
    metering_price: "",
    priority: 100,
    channel_ids: [],
  };
}

function defaultModelMonitorForm(): ModelMonitorFormState {
  return {
    id: "",
    group_id: "",
    name: "",
    selection_mode: "all",
    model_names: [],
    primary_model: "",
    mode: "passive",
    probe_interval_seconds: 300,
    recent_request_limit: 60,
    enabled: true,
  };
}

function modelMonitorFormFromItem(item: ModelMonitor): ModelMonitorFormState {
  return {
    id: item.id,
    group_id: item.group_id,
    name: item.name,
    selection_mode: item.selection_mode,
    model_names: item.selection_mode === "all" ? [] : [...item.model_names],
    primary_model: item.primary_model || "",
    mode: item.mode,
    probe_interval_seconds: item.probe_interval_seconds,
    recent_request_limit: item.recent_request_limit || 60,
    enabled: item.enabled,
  };
}

function defaultTokenCreateForm(): TokenCreateFormState {
  return {
    tenant_id: "",
    project_id: "",
    name: "",
    expires_at: "",
    group_id: "",
    allowed_ips: "",
    allowed_domains: "",
    spend_limit: "0",
  };
}

function toDateTimeLocal(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function defaultUserAdminForm(): UserAdminFormState {
  return {
    id: "",
    email: "",
    display_name: "",
    password: "",
  };
}

function groupFormFromSummary(group: GroupSummary): GroupFormState {
  return {
    id: group.id,
    code: group.code,
    name: group.name,
    description: group.description,
    status: group.status,
    multiplier: group.multiplier,
    rpm_limit: group.rpm_limit,
    billing_type: group.billing_type,
    metering_mode: group.metering_mode || "token",
    metering_price: group.metering_price || "",
    priority: group.priority,
    channel_ids: group.channels.map((channel) => channel.id),
  };
}

function mergeDiscoveredChannelModels(
  current: ChannelFormModel[],
  discovered: DiscoveredModel[]
) {
  const existing = current.filter((model) => model.model.trim() || model.upstream_model.trim());
  const mapped = new Set(
    existing
      .map((model) => (model.model.trim() || model.upstream_model.trim()).toLowerCase())
      .filter(Boolean)
  );
  const imported = discovered
    .filter((model) => {
      const key = model.id.trim().toLowerCase();
      if (!key || mapped.has(key)) {
        return false;
      }
      mapped.add(key);
      return true;
    })
    .map((model) => ({
      id: newFormRowID(),
      model: model.id.trim(),
      upstream_model: model.id.trim(),
      enabled: true,
    }));

  return [...existing, ...imported];
}

function resolveLoginError(
  status: number,
  error: string | undefined,
  t: (key: TranslationKey) => string
) {
  if ((status === 401 || status === 403) && error?.toLowerCase() === "mfa_required") {
    return t("loginMFARequired");
  }
  if (status === 403 && error === "EMAIL_VERIFICATION_REQUIRED") {
    return t("loginEmailVerificationRequired");
  }
  if (status === 401) {
    return t("loginInvalid");
  }
  if (status === 429) {
    return t("loginThrottled");
  }
  if (status === 503) {
    return t("loginUnavailable");
  }
  return t("loginFailed");
}

function resolveProfileError(
  status: number,
  error: string | undefined,
  t: (key: TranslationKey) => string
) {
  switch (error) {
    case "CURRENT_PASSWORD_INVALID":
      return t("profileCurrentPasswordInvalid");
    case "INVALID_PASSWORD":
      return t("profilePasswordInvalid");
    case "EMAIL_ALREADY_EXISTS":
      return t("profileEmailExists");
    case "MFA_CODE_INVALID":
      return t("profileMFAInvalid");
    case "MFA_ALREADY_ENABLED":
      return t("profileMFAAlreadyEnabled");
    case "MFA_ENROLLMENT_EXPIRED":
      return t("profileMFAEnrollmentExpired");
    case "MFA_NOT_ENABLED":
      return t("profileMFANotEnabled");
    default:
      return status === 503 ? t("profileUnavailable") : t("profileSaveFailed");
  }
}

function resolveSystemSettingsError(
  status: number,
  error: string | undefined,
  t: (key: TranslationKey) => string
) {
  if (error === "INVALID_SYSTEM_SETTINGS") return t("systemSettingsValidation");
  if (error === "ADMIN_MFA_ENFORCED") return t("systemSettingsMFAEnforced");
  return status === 503 ? t("systemSettingsUnavailable") : t("systemSettingsSaveFailed");
}

function resolveAPIEndpointError(
  status: number,
  error: string | undefined,
  t: (key: TranslationKey) => string
) {
  if (error === "INVALID_API_ENDPOINT") return t("systemAPIEndpointValidation");
  if (error === "API_ENDPOINT_EXISTS") return t("systemAPIEndpointExists");
  if (error === "API_ENDPOINT_NOT_FOUND") return t("systemAPIEndpointUnavailable");
  return status === 503 ? t("systemAPIEndpointUnavailable") : t("systemSettingsSaveFailed");
}

function resolveFeatureSettingsError(
  status: number,
  error: string | undefined,
  t: (key: TranslationKey) => string
) {
  switch (error) {
    case "INVALID_BALANCE_THRESHOLD":
      return t("featureBalanceThresholdInvalid");
    case "INVALID_RECHARGE_URL":
      return t("featureRechargeURLInvalid");
    case "EMAIL_SMTP_REQUIRED":
      return t("featureEmailSMTPRequired");
    case "INVALID_EMAIL_SMTP":
      return t("featureEmailSMTPInvalid");
    case "INVALID_FEATURE_SETTINGS":
      return t("featureSettingsValidation");
    default:
      return status === 503 ? t("featureSettingsUnavailable") : t("featureSettingsSaveFailed");
  }
}

function resolveEnterpriseError(status: number, error: string | undefined, t: (key: TranslationKey) => string) {
  switch (error) {
    case "ENTERPRISE_PENDING": return t("enterpriseAlreadyPending");
    case "ENTERPRISE_APPROVED": return t("enterpriseAlreadyApproved");
    case "LICENSE_TOO_LARGE": return t("enterpriseLicenseTooLarge");
    case "LICENSE_INVALID": return t("enterpriseLicenseInvalid");
    case "INVALID_ENTERPRISE_REQUEST": return t("enterpriseFormInvalid");
    default: return status === 503 ? t("enterpriseUnavailable") : t("enterpriseSubmitFailed");
  }
}

function resolvePaymentError(status: number, error: string | undefined, t: (key: TranslationKey) => string, message?: string) {
  switch (error) {
    case "PAYMENT_PROVIDER_DISABLED": return t("rechargeProviderDisabled");
    case "PAYMENT_PROVIDER_UNCONFIGURED": return t("rechargeProviderUnconfigured");
    case "PAYMENT_ORDER_NOT_FOUND": return t("rechargeOrderNotFound");
    case "PAYMENT_ORDER_CLOSED": return t("rechargeOrderClosed");
    case "INVALID_PAYMENT_REQUEST": return t("rechargeInvalid");
    case "PAYMENT_PROVIDER_FAILED": return message?.trim() ? `${t("rechargeProviderFailed")} ${message.trim()}` : t("rechargeFailed");
    default: return status === 503 ? t("rechargeUnavailable") : t("rechargeFailed");
  }
}

function featureSettingsUpdatePayload(settings: FeatureSettings) {
  return {
    email_enabled: settings.email_enabled,
    registration_enabled: settings.registration_enabled,
    model_status_enabled: settings.model_status_enabled,
    totp_enabled: settings.totp_enabled,
    step_up_channel_model_enabled: settings.step_up_channel_model_enabled,
    step_up_group_enabled: settings.step_up_group_enabled,
    step_up_token_enabled: settings.step_up_token_enabled,
    step_up_user_enabled: settings.step_up_user_enabled,
    step_up_role_enabled: settings.step_up_role_enabled,
    step_up_billing_enabled: settings.step_up_billing_enabled,
    step_up_system_enabled: settings.step_up_system_enabled,
    email_verification_enabled: settings.email_verification_enabled,
    email_password_reset_enabled: settings.email_password_reset_enabled,
    email_subscription_enabled: settings.email_subscription_enabled,
    email_low_balance_alert_enabled: settings.email_low_balance_alert_enabled,
    email_recharge_success_enabled: settings.email_recharge_success_enabled,
    email_usage_limit_alert_enabled: settings.email_usage_limit_alert_enabled,
    email_content_audit_enabled: settings.email_content_audit_enabled,
    email_account_disabled_enabled: settings.email_account_disabled_enabled,
    email_cyber_policy_enabled: settings.email_cyber_policy_enabled,
    email_operations_enabled: settings.email_operations_enabled,
    balance_threshold: settings.balance_threshold,
    recharge_url: settings.recharge_url,
  };
}

export default function App() {
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = window.localStorage.getItem("ai-token-theme");
    if (saved === "light" || saved === "dark") return saved;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });
  const [language, setLanguage] = useState<Language>(() =>
    window.localStorage.getItem("ai-token-language") === "en" ? "en" : "zh"
  );
  const [audience, setAudience] = useState<Audience>("console");
  const [signedIn, setSignedIn] = useState(false);
  const [principal, setPrincipal] = useState<Principal | null>(null);
  const [sessionReady, setSessionReady] = useState(false);
  const [route, setRoute] = useState<SectionRoute>(() => parseRoute(window.location.hash));
  const [adminSection, setAdminSection] = useState<AdminSection>("dashboard");
  const [consoleSection, setConsoleSection] = useState<ConsoleSection>("dashboard");
  const [loginMessage, setLoginMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [modelCatalog, setModelCatalog] = useState<PublicModelSummary[]>([]);
  const [modelCatalogBusy, setModelCatalogBusy] = useState(false);
  const [modelCatalogMessage, setModelCatalogMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [usageStatus, setUsageStatus] = useState<ConsoleUsageStatus | null>(null);
		const [consoleUsageReport, setConsoleUsageReport] = useState<UsageReport | null>(null);
		const [consoleDashboardReport, setConsoleDashboardReport] = useState<ConsoleDashboardReport | null>(null);
		const [consoleDashboardBusy, setConsoleDashboardBusy] = useState(false);
		const [consoleDashboardMessage, setConsoleDashboardMessage] = useState<LoginMessage>({ kind: "", text: "" });
	const [consoleUsageOffset, setConsoleUsageOffset] = useState(0);
  const [consoleUsageTokenName, setConsoleUsageTokenName] = useState("");
  const [consoleUsageModel, setConsoleUsageModel] = useState("");
  const [consoleUsageGroup, setConsoleUsageGroup] = useState("");
  const [consoleUsageFrom, setConsoleUsageFrom] = useState("");
  const [consoleUsageTo, setConsoleUsageTo] = useState("");
  const [usageBusy, setUsageBusy] = useState(false);
  const [usageMessage, setUsageMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [modelStatusReport, setModelStatusReport] = useState<ModelStatusReport | null>(null);
  const [modelStatusBusy, setModelStatusBusy] = useState(false);
  const [modelStatusMessage, setModelStatusMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [adminModelStatusReport, setAdminModelStatusReport] = useState<ModelStatusReport | null>(null);
  const [adminModelStatusBusy, setAdminModelStatusBusy] = useState(false);
  const [adminModelStatusMessage, setAdminModelStatusMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [adminModelMonitors, setAdminModelMonitors] = useState<ModelMonitor[]>([]);
  const [adminModelMonitorsBusy, setAdminModelMonitorsBusy] = useState(false);
  const [adminModelMonitorsMessage, setAdminModelMonitorsMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [adminModelMonitorFormOpen, setAdminModelMonitorFormOpen] = useState(false);
  const [adminModelMonitorForm, setAdminModelMonitorForm] = useState<ModelMonitorFormState>(() => defaultModelMonitorForm());
  const [adminModelMonitorActionBusy, setAdminModelMonitorActionBusy] = useState("");
  const [publicFeatures, setPublicFeatures] = useState<PublicFeatureSettings | null>(null);
  const [oauthProviders, setOauthProviders] = useState<Array<{ provider: string; enabled: boolean }>>([]);
  const [consoleProfile, setConsoleProfile] = useState<ConsoleProfile | null>(null);
  const [profileForm, setProfileForm] = useState<ProfileFormState>(() => defaultProfileForm());
  const [emailForm, setEmailForm] = useState<EmailFormState>(() => defaultEmailForm());
  const [passwordForm, setPasswordForm] = useState<PasswordFormState>(() => defaultPasswordForm());
  const [profileMessage, setProfileMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [profileBusy, setProfileBusy] = useState(false);
  const [mfaStatus, setMfaStatus] = useState<MFAStatus>({ enabled: false });
  const [mfaEnrollment, setMfaEnrollment] = useState<MFAEnrollment | null>(null);
  const [mfaCode, setMfaCode] = useState("");
  const [mfaBusy, setMfaBusy] = useState(false);
  const [users, setUsers] = useState<UserSummary[]>([]);
  const [usersBusy, setUsersBusy] = useState(false);
  const [usersMessage, setUsersMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [userActionBusy, setUserActionBusy] = useState("");
  const [userFormOpen, setUserFormOpen] = useState(false);
  const [userForm, setUserForm] = useState<UserAdminFormState>(() => defaultUserAdminForm());
  const [userFormBusy, setUserFormBusy] = useState(false);
  const [userFormMessage, setUserFormMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [platformRoles, setPlatformRoles] = useState<PlatformRole[]>([]);
  const [platformPermissions, setPlatformPermissions] = useState<PlatformPermission[]>([]);
  const [platformRolesBusy, setPlatformRolesBusy] = useState(false);
  const [platformRolesMessage, setPlatformRolesMessage] = useState<LoginMessage>({ kind: "", text: "" });

  const [securitySettings, setSecuritySettings] = useState<SecuritySettings>({
    admin_mfa_enabled: false,
    updated_at: "",
    updated_by: "",
  });
  const [securityMessage, setSecurityMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [securityBusy, setSecurityBusy] = useState(false);
  const [adminProfile, setAdminProfile] = useState<ConsoleProfile | null>(null);
  const [adminProfileForm, setAdminProfileForm] = useState<ProfileFormState>(() => defaultProfileForm());
  const [adminEmailForm, setAdminEmailForm] = useState<EmailFormState>(() => defaultEmailForm());
  const [adminPasswordForm, setAdminPasswordForm] = useState<PasswordFormState>(() => defaultPasswordForm());
  const [adminProfileMessage, setAdminProfileMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [adminProfileBusy, setAdminProfileBusy] = useState(false);
  const [adminMfaStatus, setAdminMfaStatus] = useState<MFAStatus>({ enabled: false });
  const [adminMfaEnrollment, setAdminMfaEnrollment] = useState<MFAEnrollment | null>(null);
  const [adminMfaCode, setAdminMfaCode] = useState("");
  const [adminMfaBusy, setAdminMfaBusy] = useState(false);
  const [siteSettings, setSiteSettings] = useState<SiteSettings>(() => defaultSiteSettings());
  const [adminSiteForm, setAdminSiteForm] = useState<SiteSettings>(() => defaultSiteSettings());
  const [publicAPIEndpoints, setPublicAPIEndpoints] = useState<PublicAPIEndpoint[]>([]);
  const [adminAPIEndpoints, setAdminAPIEndpoints] = useState<APIEndpoint[]>([]);
  const [apiEndpointFormOpen, setAPIEndpointFormOpen] = useState(false);
  const [apiEndpointForm, setAPIEndpointForm] = useState<APIEndpointFormState>(() => defaultAPIEndpointForm());
  const [apiEndpointBusy, setAPIEndpointBusy] = useState(false);
  const [apiEndpointActionBusy, setAPIEndpointActionBusy] = useState("");
  const [apiEndpointMessage, setAPIEndpointMessage] = useState<LoginMessage>({ kind: "", text: "" });
	  const [siteSettingsMessage, setSiteSettingsMessage] = useState<LoginMessage>({ kind: "", text: "" });
	  const [siteSettingsBusy, setSiteSettingsBusy] = useState(false);
	  const [smtpForm, setSmtpForm] = useState<SMTPSettingsForm>(() => defaultSMTPSettings());
	  const [emailSettings, setEmailSettings] = useState<EmailSettings | null>(null);
	  const [emailBusy, setEmailBusy] = useState(false);
	  const [emailMessage, setEmailMessage] = useState<LoginMessage>({ kind: "", text: "" });
	  const [emailTestRecipient, setEmailTestRecipient] = useState("");
	  const [smtpConnectionBusy, setSMTPConnectionBusy] = useState(false);
	  const [smtpMessageBusy, setSMTPMessageBusy] = useState(false);
	  const [featureSettings, setFeatureSettings] = useState<FeatureSettings>(() => defaultFeatureSettings());
	  const [featureBusy, setFeatureBusy] = useState(false);
	  const [featureMessage, setFeatureMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [emailTemplates, setEmailTemplates] = useState<EmailTemplate[]>([]);
  const [emailTemplatesBusy, setEmailTemplatesBusy] = useState(false);
  const [emailTemplatesMessage, setEmailTemplatesMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [adminEnterpriseItems, setAdminEnterpriseItems] = useState<EnterpriseVerification[]>([]);
  const [adminEnterpriseStatus, setAdminEnterpriseStatus] = useState("");
  const [adminEnterpriseBusy, setAdminEnterpriseBusy] = useState(false);
  const [adminEnterpriseMessage, setAdminEnterpriseMessage] = useState<LoginMessage>({ kind: "", text: "" });
	  const [stepUpOpen, setStepUpOpen] = useState(false);
  const [stepUpCode, setStepUpCode] = useState("");
  const [stepUpError, setStepUpError] = useState("");
  const stepUpResolver = useRef<((code: string | null) => void) | null>(null);

  const [channels, setChannels] = useState<ChannelSummary[]>([]);
  const [channelsMessage, setChannelsMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [channelsBusy, setChannelsBusy] = useState(false);
  const [groups, setGroups] = useState<GroupSummary[]>([]);
  const [groupsMessage, setGroupsMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [groupsBusy, setGroupsBusy] = useState(false);
  const [groupFormOpen, setGroupFormOpen] = useState(false);
  const [groupForm, setGroupForm] = useState<GroupFormState>(() => defaultGroupForm());
  const [groupActionBusy, setGroupActionBusy] = useState("");
  const [groupDeleteConfirm, setGroupDeleteConfirm] = useState("");
  const [tokens, setTokens] = useState<TokenSummary[]>([]);
  const [tokensMessage, setTokensMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [tokensBusy, setTokensBusy] = useState(false);
  const [tokenActionBusy, setTokenActionBusy] = useState("");
  const [tokenCreateOpen, setTokenCreateOpen] = useState(false);
  const [tokenCreateMode, setTokenCreateMode] = useState<"console">("console");
  const [tokenCreateForm, setTokenCreateForm] = useState<TokenCreateFormState>(() => defaultTokenCreateForm());
  const [tokenCreateBusy, setTokenCreateBusy] = useState(false);
  const [tokenCreateMessage, setTokenCreateMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [issuedToken, setIssuedToken] = useState<IssuedTokenResponse | null>(null);
  const [tokenEditingID, setTokenEditingID] = useState("");
  const [consoleTokenSecrets, setConsoleTokenSecrets] = useState<Record<string, string>>({});
  const tokenSecretKey = tokenSecretStorageKey(principal);
  const skipTokenSecretSave = useRef(false);
  const [consoleTokenGroups, setConsoleTokenGroups] = useState<TokenGroupOption[]>([]);
  const [consoleTokenGroupsBusy, setConsoleTokenGroupsBusy] = useState(false);
  const [projects, setProjects] = useState<ProjectSummary[]>([]);
  const [projectsBusy, setProjectsBusy] = useState(false);
  const [projectsMessage, setProjectsMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [projectActionBusy, setProjectActionBusy] = useState("");
  const [projectDeleteConfirm, setProjectDeleteConfirm] = useState("");
  const [members, setMembers] = useState<TenantMember[]>([]);
  const [membersBusy, setMembersBusy] = useState(false);
  const [membersMessage, setMembersMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [memberActionBusy, setMemberActionBusy] = useState("");
  const [projectMembers, setProjectMembers] = useState<ProjectMember[]>([]);
  const [projectMembersBusy, setProjectMembersBusy] = useState(false);
  const [projectMembersMessage, setProjectMembersMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [selectedProjectID, setSelectedProjectID] = useState("");
  const [projectMemberActionBusy, setProjectMemberActionBusy] = useState("");

  useEffect(() => {
    skipTokenSecretSave.current = true;
    setConsoleTokenSecrets(readTokenSecrets(tokenSecretKey));
  }, [tokenSecretKey]);

  useEffect(() => {
    if (!tokenSecretKey) return;
    if (skipTokenSecretSave.current) {
      skipTokenSecretSave.current = false;
      return;
    }
    try {
      window.localStorage.setItem(tokenSecretKey, JSON.stringify(consoleTokenSecrets));
    } catch {
      // Clipboard recovery remains available for the current session if storage is blocked.
    }
  }, [consoleTokenSecrets, tokenSecretKey]);
  const [channelFormOpen, setChannelFormOpen] = useState(false);
  const [channelForm, setChannelForm] = useState<ChannelFormState>(() => defaultChannelForm());
  const [channelActionBusy, setChannelActionBusy] = useState("");
  const [channelDeleteConfirm, setChannelDeleteConfirm] = useState("");
  const [discoveredModels, setDiscoveredModels] = useState<DiscoveredModel[]>([]);
  const [modelDiscoveryBusy, setModelDiscoveryBusy] = useState(false);
  const [modelDiscoveryMessage, setModelDiscoveryMessage] = useState<LoginMessage>({
    kind: "",
    text: "",
  });

  const [prices, setPrices] = useState<PriceMatrixSummary[]>([]);
  const [billingAccount, setBillingAccount] = useState<BillingAccount | null>(null);
  const [billingMessage, setBillingMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [billingBusy, setBillingBusy] = useState(false);
  const [enterpriseVerification, setEnterpriseVerification] = useState<EnterpriseVerification | null>(null);
  const [enterpriseBusy, setEnterpriseBusy] = useState(false);
  const [enterpriseMessage, setEnterpriseMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [paymentProviders, setPaymentProviders] = useState<PublicPaymentProvider[]>([]);
  const [paymentBusy, setPaymentBusy] = useState(false);
  const [paymentMessage, setPaymentMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [paymentOrder, setPaymentOrder] = useState<PaymentOrder | null>(null);
  const [paymentOrders, setPaymentOrders] = useState<PaymentOrderList | null>(null);
  const [paymentOrdersBusy, setPaymentOrdersBusy] = useState(false);
  const [paymentOrdersOffset, setPaymentOrdersOffset] = useState(0);
  const [paymentConfigs, setPaymentConfigs] = useState<PaymentProviderConfig[]>([]);
  const [paymentSettingsBusy, setPaymentSettingsBusy] = useState(false);
  const [paymentSettingsMessage, setPaymentSettingsMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [loginSettings, setLoginSettings] = useState<LoginSettings>({ providers: [] });
  const [loginSettingsBusy, setLoginSettingsBusy] = useState(false);
  const [loginSettingsMessage, setLoginSettingsMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [paymentRechargePackages, setPaymentRechargePackages] = useState<PaymentRechargePackage[]>([]);
  const [officialPriceSyncBusy, setOfficialPriceSyncBusy] = useState(false);
  const [modelPriceForm, setModelPriceForm] = useState<ModelPriceFormState>(() => defaultModelPriceForm());
  const [modelPriceFormOpen, setModelPriceFormOpen] = useState(false);
  const [modelPriceFormBusy, setModelPriceFormBusy] = useState(false);
  const [modelPriceFormMessage, setModelPriceFormMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [creditForm, setCreditForm] = useState<CreditFormState>(() => defaultCreditForm());
  const [formBusy, setFormBusy] = useState(false);
  const [usageReport, setUsageReport] = useState<UsageReport | null>(null);
  const [usageReportBusy, setUsageReportBusy] = useState(false);
  const [usageReportMessage, setUsageReportMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [financeReport, setFinanceReport] = useState<FinanceReport | null>(null);
  const [financeReportBusy, setFinanceReportBusy] = useState(false);
  const [financeReportMessage, setFinanceReportMessage] = useState<LoginMessage>({ kind: "", text: "" });
	const [financeSearch, setFinanceSearch] = useState("");
	const [financeCurrency, setFinanceCurrency] = useState("");
	const [financeFrom, setFinanceFrom] = useState("");
	const [financeTo, setFinanceTo] = useState("");
	const [operationsSnapshot, setOperationsSnapshot] = useState<OperationsSnapshot | null>(null);
	const [operationsBusy, setOperationsBusy] = useState(false);
	const [auditReport, setAuditReport] = useState<AuditReport | null>(null);
	const [auditBusy, setAuditBusy] = useState(false);
	const [auditMessage, setAuditMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [reportSearch, setReportSearch] = useState("");
  const [reportStatus, setReportStatus] = useState("");
  const [reportTenant, setReportTenant] = useState("");
  const [reportModel, setReportModel] = useState("");
  const [reportGroup, setReportGroup] = useState("");
  const [reportFrom, setReportFrom] = useState("");
  const [reportTo, setReportTo] = useState("");
  const [reportOffset, setReportOffset] = useState(0);

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [tenantId, setTenantId] = useState("");
  const [profileMfaCode, setProfileMfaCode] = useState("");

  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const hasConsolePermission = (permission: string) => principal?.permissions?.includes(permission) === true;

  function askForAdminStepUp(error = "") {
    setStepUpError(error);
    setStepUpCode("");
    setStepUpOpen(true);
    return new Promise<string | null>((resolve) => {
      stepUpResolver.current = resolve;
    });
  }

  function submitAdminStepUp(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!/^\d{6}$/.test(stepUpCode.trim())) {
      setStepUpError(t("stepUpValidation"));
      return;
    }
    const resolve = stepUpResolver.current;
    stepUpResolver.current = null;
    setStepUpOpen(false);
    resolve?.(stepUpCode.trim());
  }

  function cancelAdminStepUp() {
    const resolve = stepUpResolver.current;
    stepUpResolver.current = null;
    setStepUpOpen(false);
    setStepUpCode("");
    resolve?.(null);
  }

  async function fetchAdminSensitive(input: RequestInfo | URL, init: RequestInit = {}) {
    let response = await fetch(input, init);
    for (let attempt = 0; attempt < 3; attempt += 1) {
      const result = (await response.clone().json().catch(() => ({}))) as { error?: string };
      if (result.error !== "STEP_UP_REQUIRED" && result.error !== "MFA_CODE_INVALID") return response;
      const code = await askForAdminStepUp(result.error === "MFA_CODE_INVALID" ? t("stepUpInvalid") : "");
      if (!code) throw new Error("STEP_UP_CANCELLED");
      const headers = new Headers(init.headers);
      headers.set("X-MFA-Code", code);
      response = await fetch(input, { ...init, headers });
    }
    return response;
  }

  useEffect(() => {
    window.localStorage.setItem("ai-token-theme", theme);
    if (theme === "dark") {
      document.documentElement.classList.add("dark");
    } else {
      document.documentElement.classList.remove("dark");
    }
  }, [theme]);

  useEffect(() => {
    let cancelled = false;
    async function loadSiteSettings() {
      try {
        const response = await fetch("/public/v1/settings", { headers: { Accept: "application/json" } });
        const result = (await response.json().catch(() => ({}))) as Partial<SiteSettings> & { api_endpoints?: PublicAPIEndpoint[] };
        if (!cancelled && response.ok) {
          setSiteSettings({
            site_name: result.site_name?.trim() || "AI Token Gateway",
            site_logo_url: result.site_logo_url?.trim() || "",
            site_favicon_url: result.site_favicon_url?.trim() || "",
          });
          setPublicAPIEndpoints(result.api_endpoints || []);
        }
      } catch {
        // Keep the bundled brand when the public settings endpoint is unavailable.
      }
    }
    loadSiteSettings();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    fetch("/public/v1/auth/providers", { headers: { Accept: "application/json" } })
      .then(async (response) => (response.ok ? (await response.json()) as { providers?: Array<{ provider: string; enabled: boolean }> } : { providers: [] }))
      .then((result) => { if (!cancelled) setOauthProviders(result.providers || []); })
      .catch(() => { if (!cancelled) setOauthProviders([]); });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function loadPublicFeatures() {
      try {
        const response = await fetch("/public/v1/features", { headers: { Accept: "application/json" } });
        const result = (await response.json().catch(() => ({}))) as Partial<PublicFeatureSettings>;
        if (!cancelled && response.ok) {
          setPublicFeatures({
            registration_enabled: result.registration_enabled !== false,
            model_status_enabled: result.model_status_enabled !== false,
            totp_enabled: result.totp_enabled === true,
          });
        }
      } catch {
        // Keep optional customer features visible when the public settings endpoint is unavailable.
      }
    }
    loadPublicFeatures();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const name = siteSettings.site_name || "AI Token Gateway";
    document.title = `${name} - ${language === "zh" ? "企业级大模型统一分发与计费网关" : "Enterprise AI routing and billing gateway"}`;
    let icon = document.querySelector<HTMLLinkElement>("link[data-site-favicon]");
    if (!siteSettings.site_favicon_url) {
      icon?.remove();
      return;
    }
    if (!icon) {
      icon = document.createElement("link");
      icon.rel = "icon";
      icon.dataset.siteFavicon = "true";
      document.head.appendChild(icon);
    }
    const assetPath = siteSettings.site_favicon_url.split(/[?#]/, 1)[0].toLowerCase();
    icon.type = assetPath.endsWith(".svg") ? "image/svg+xml" : "";
    icon.href = siteSettings.site_favicon_url;
  }, [language, siteSettings.site_favicon_url, siteSettings.site_name]);

  useEffect(() => {
    window.localStorage.setItem("ai-token-language", language);
    document.documentElement.lang = language === "zh" ? "zh-CN" : "en";
  }, [language]);

  useEffect(() => {
    if (!channelFormOpen) return;
    const previousOverflow = document.body.style.overflow;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !modelDiscoveryBusy && !channelActionBusy) {
        closeChannelForm();
      }
    };
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [channelFormOpen, modelDiscoveryBusy, channelActionBusy]);

  useEffect(() => {
    const onHashChange = () => setRoute(parseRoute(window.location.hash));
    window.addEventListener("hashchange", onHashChange);
    onHashChange();
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => {
    if (route.view !== "login") return;
    const query = new URLSearchParams(window.location.search);
    const oauthError = query.get("oauth_error");
    if (!oauthError) return;
    const cleanURL = new URL(window.location.href);
    cleanURL.searchParams.delete("oauth_error");
    cleanURL.searchParams.delete("oauth_provider");
    window.history.replaceState({}, document.title, `${cleanURL.pathname}${cleanURL.search}${cleanURL.hash}`);
    setLoginMessage({ kind: "error", text: oauthError === "OAUTH_PROVIDER_UNAVAILABLE" ? t("oauthProviderUnavailable") : t("oauthLoginFailed") });
  }, [route.view, language]);

  useEffect(() => {
    let cancelled = false;
    async function restoreSession() {
      try {
        if (isAdminEntryLocation()) {
          const response = await fetch("/admin/v1/me", {
            headers: { Accept: "application/json" },
            credentials: "same-origin",
          });
          if (!response.ok) throw new Error("unauthorized");
          const profile = (await response.json()) as Principal;
          if (cancelled) return;
          setSignedIn(true);
          setAudience("admin");
          setPrincipal(profile);
          return;
        }

        const consoleResponse = await fetch("/console/v1/me", {
          headers: { Accept: "application/json" },
          credentials: "same-origin",
        });
        if (consoleResponse.ok) {
          const profile = (await consoleResponse.json()) as Principal;
          if (cancelled) return;
          setSignedIn(true);
          setAudience("console");
          setPrincipal(profile);
          return;
        }

        const adminResponse = await fetch("/admin/v1/me", {
          headers: { Accept: "application/json" },
          credentials: "same-origin",
        });
        if (!adminResponse.ok) throw new Error("unauthorized");
        const profile = (await adminResponse.json()) as Principal;
        if (cancelled) return;
        setSignedIn(true);
        setAudience("admin");
        setPrincipal(profile);
      } catch {
        if (!cancelled) {
          setSignedIn(false);
          setPrincipal(null);
        }
      } finally {
        if (!cancelled) {
          setSessionReady(true);
        }
      }
    }
    restoreSession();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!sessionReady) return;
    if (route.view === "admin-login") {
      if (signedIn && audience === "admin") {
        window.location.hash = "#admin/dashboard";
      } else if (signedIn && audience === "console") {
        window.location.replace("/#console/dashboard");
      } else {
        setAudience("admin");
      }
      return;
    }
    if (route.view === "login") {
      setAudience("console");
    }
    if (route.view === "register" && publicFeatures?.registration_enabled === false) {
      window.location.hash = "#login";
      return;
    }
    if (route.view === "admin" && (!signedIn || audience !== "admin")) {
      window.location.hash = signedIn && audience === "console" ? "#console/dashboard" : "#login";
      return;
    }
    if (route.view === "console" && (!signedIn || audience !== "console")) {
      window.location.hash = signedIn && audience === "admin" ? "#admin/dashboard" : "#login";
      return;
    }
    if ((route.view === "login" || route.view === "register" || route.view === "reset") && signedIn) {
      window.location.hash = audience === "console" ? "#console/dashboard" : "#admin/dashboard";
      return;
    }
    if (route.view === "admin") {
      setAdminSection(route.section);
    }
    if (route.view === "console") {
      setConsoleSection(route.console_section || "dashboard");
    }
  }, [route, sessionReady, signedIn, audience, publicFeatures]);

  useEffect(() => {
    if (route.view === "console" && route.console_section === "model-status" && publicFeatures?.model_status_enabled === false) {
      window.location.hash = "#console/dashboard";
    }
  }, [route, publicFeatures]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin") {
      setChannels([]);
      setGroups([]);
      return;
    }
    refreshChannels(adminSection === "channels");
  }, [signedIn, audience, route.view, adminSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || (consoleSection !== "dashboard" && consoleSection !== "tokens" && consoleSection !== "projects" && consoleSection !== "billing-records")) return;
    refreshConsoleTokens(true);
    refreshConsoleTokenGroups();
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || (consoleSection !== "dashboard" && consoleSection !== "tokens" && consoleSection !== "projects")) return;
    refreshConsoleProjects(true);
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (route.view !== "home" && route.view !== "models" && !(route.view === "console" && (consoleSection === "interface-debug-text" || consoleSection === "interface-debug-model" || consoleSection === "interface-debug-image"))) return;
    refreshModelCatalog(true);
  }, [route.view, consoleSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || (adminSection !== "groups" && adminSection !== "tokens" && adminSection !== "model-status")) {
      return;
    }
    refreshGroups(true);
  }, [signedIn, audience, route.view, adminSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "tokens") {
      return;
    }
    refreshTokens(true);
  }, [signedIn, audience, route.view, adminSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "channels") {
      return;
    }
    const interval = window.setInterval(() => void refreshChannels(false), 60_000);
    return () => window.clearInterval(interval);
  }, [signedIn, audience, route.view, adminSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || (adminSection !== "users" && adminSection !== "roles")) {
      return;
    }
    refreshUsers(true);
    refreshPlatformRoles(true);
  }, [signedIn, audience, route.view, adminSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || !["dashboard", "billing", "billing-center", "billing-records", "billing-orders"].includes(consoleSection)) return;
    refreshConsoleBilling();
    if (consoleSection !== "dashboard") refreshPaymentProviders();
    if (consoleSection === "billing-orders") refreshPaymentOrders();
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || !["billing", "billing-center", "billing-records", "billing-orders"].includes(consoleSection)) return;
    const query = new URLSearchParams(window.location.search);
    const orderID = query.get("payment_order_id")?.trim();
    const provider = query.get("provider")?.trim().toLowerCase();
    if (!orderID || (provider !== "stripe" && provider !== "paypal")) return;
    const cancelledByUser = query.get("cancelled") === "1";
    const cleanURL = new URL(window.location.href);
    cleanURL.searchParams.delete("payment_order_id");
    cleanURL.searchParams.delete("provider");
    cleanURL.searchParams.delete("cancelled");
    window.history.replaceState({}, document.title, `${cleanURL.pathname}${cleanURL.search}${cleanURL.hash}`);
    void loadPaymentOrder(orderID, cancelledByUser);
  }, [signedIn, audience, route.view, consoleSection, principal?.tenant_id]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || consoleSection !== "enterprise") return;
    refreshEnterpriseVerification(true);
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || !["billing", "billing-center", "billing-records", "billing-orders"].includes(consoleSection) || paymentOrder?.status !== "pending") return;
    const interval = window.setInterval(() => void refreshPaymentOrder(), 5_000);
    return () => window.clearInterval(interval);
  }, [signedIn, audience, route.view, consoleSection, paymentOrder?.id, paymentOrder?.status]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || consoleSection !== "billing-records") return;
    refreshConsoleUsage();
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || consoleSection !== "dashboard") return;
    void refreshConsoleDashboard();
    const interval = window.setInterval(() => void refreshConsoleDashboard(), 30_000);
    return () => window.clearInterval(interval);
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || consoleSection !== "model-status") return;
    void refreshConsoleModelStatus(true);
    const interval = window.setInterval(() => void refreshConsoleModelStatus(false), 15_000);
    return () => window.clearInterval(interval);
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || consoleSection !== "profile") return;
    refreshConsoleProfile(true);
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin") return;
    let cancelled = false;
    async function loadSecurity() {
      setSecurityBusy(true);
      setSecurityMessage({ kind: "pending", text: t("securityLoading") });
      try {
        const response = await fetch("/admin/v1/security/settings", {
          headers: { Accept: "application/json" },
          credentials: "same-origin",
        });
        const result = (await response.json().catch(() => ({}))) as Partial<SecuritySettings>;
        if (!response.ok) {
          throw new Error("security unavailable");
        }
        if (!cancelled) {
          setSecuritySettings({
            admin_mfa_enabled: Boolean(result.admin_mfa_enabled),
            updated_at: result.updated_at || "",
            updated_by: result.updated_by || "",
          });
          setSecurityMessage({
            kind: "success",
            text: result.admin_mfa_enabled
              ? t("securityMessageEnabled")
              : t("securityMessageDisabled"),
          });
        }
      } catch {
        if (!cancelled) {
          setSecurityMessage({ kind: "error", text: t("securityMessageUnavailable") });
        }
      } finally {
        if (!cancelled) {
          setSecurityBusy(false);
        }
      }
    }
    loadSecurity();
    return () => {
      cancelled = true;
    };
  }, [audience, route.view, signedIn, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "billing") {
      return;
    }
    refreshBilling();
  }, [signedIn, audience, route.view, adminSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "settings") {
      return;
    }
    refreshAdminSettings(true);
  }, [signedIn, audience, route.view, adminSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "enterprise") return;
    refreshAdminEnterprise(true);
  }, [signedIn, audience, route.view, adminSection, adminEnterpriseStatus, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "usage") return;
    refreshUsageReport(true, 0);
  }, [signedIn, audience, route.view, adminSection, language, reportSearch, reportStatus, reportTenant, reportModel, reportGroup, reportFrom, reportTo]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || (adminSection !== "finance" && adminSection !== "account-finance")) return;
    refreshFinanceReport(true, 0);
  }, [signedIn, audience, route.view, adminSection, language, financeSearch, financeCurrency, financeFrom, financeTo]);

		useEffect(() => {
			if (!signedIn || audience !== "admin" || route.view !== "admin" || (adminSection !== "dashboard" && adminSection !== "ops")) return;
			refreshOperations(true);
			const interval = window.setInterval(() => void refreshOperations(false), 30_000);
			return () => window.clearInterval(interval);
		}, [signedIn, audience, route.view, adminSection, language]);

	useEffect(() => {
		if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "model-status") return;
		void refreshAdminModelMonitors(true);
		void refreshAdminModelStatus(true);
		const interval = window.setInterval(() => {
			void refreshAdminModelMonitors(false);
			void refreshAdminModelStatus(false);
		}, 15_000);
		return () => window.clearInterval(interval);
	}, [signedIn, audience, route.view, adminSection, language]);

	useEffect(() => {
		if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "audit") return;
		refreshAudit(true, 0);
	}, [signedIn, audience, route.view, adminSection, language]);

  const routeTo = (target: string) => {
    if (target === "#register" && publicFeatures?.registration_enabled === false) {
      window.location.hash = "#login";
      return;
    }
    if (
      isAdminEntryLocation() &&
      (target === "#home" || target === "#login" || target === "#register" || target === "#models")
    ) {
      window.location.assign(`/${target}`);
      return;
    }
    if ((target === "#admin" || target.startsWith("#admin/")) && !isAdminEntryLocation()) {
      const adminEntryPath = principal?.audience === "admin" && isValidAdminEntryPath(principal.admin_entry_path)
        ? principal.admin_entry_path
        : "";
      if (!adminEntryPath) return;
      window.location.assign(`${adminEntryPath}${target}`);
      return;
    }
    window.location.hash = target;
  };

  async function handleLogin(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoginMessage({ kind: "pending", text: t("signingIn") });
    setFormBusy(true);
    try {
      const loginAudience: Audience = route.view === "admin-login" ? "admin" : "console";
      const payload: Record<string, string> = { email, password, mfa_code: mfaCode };
      if (loginAudience === "console") {
        payload.tenant_id = tenantId;
      }
      const response = await fetch(`/${loginAudience}/v1/auth/login`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(payload),
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) {
        throw new Error(resolveLoginError(response.status, result.error, t));
      }
      setSignedIn(true);
      setAudience(loginAudience);
      if (loginAudience === "admin") {
        const profile = await fetchAdminProfile();
        setPrincipal(profile);
        window.location.hash = "#admin/dashboard";
        setLoginMessage({ kind: "", text: "" });
      } else {
        const profile = await fetchConsoleProfile();
        setPrincipal(profile);
        window.location.hash = "#console/dashboard";
        setLoginMessage({ kind: "success", text: t("loginSuccess") });
      }
    } catch (error) {
      setSignedIn(false);
      setPrincipal(null);
      setLoginMessage({
        kind: "error",
        text: error instanceof Error ? error.message : t("loginFailed"),
      });
    } finally {
      setFormBusy(false);
    }
  }

  async function fetchAdminProfile() {
    const response = await fetch("/admin/v1/me", {
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
    if (!response.ok) {
      throw new Error(t("loginUnavailable"));
    }
    return (await response.json()) as Principal;
  }

  async function fetchConsoleProfile() {
    const response = await fetch("/console/v1/me", {
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
    if (!response.ok) {
      throw new Error(t("loginUnavailable"));
    }
    return (await response.json()) as Principal;
  }

  async function refreshConsoleProfile(showPending = false) {
    if (!signedIn || audience !== "console" || !principal?.id) return;
    setProfileBusy(true);
    if (showPending) setProfileMessage({ kind: "pending", text: t("profileLoading") });
    try {
      const [profileResponse, mfaResponse] = await Promise.all([
        fetch("/console/v1/profile", {
          headers: { Accept: "application/json" },
          credentials: "same-origin",
        }),
        fetch("/console/v1/profile/mfa", {
          headers: { Accept: "application/json" },
          credentials: "same-origin",
        }),
      ]);
      const profileResult = (await profileResponse.json().catch(() => ({}))) as ConsoleProfile & { error?: string };
      const mfaResult = (await mfaResponse.json().catch(() => ({}))) as MFAStatus & { error?: string };
      if (!profileResponse.ok) {
        throw new Error(resolveProfileError(profileResponse.status, profileResult.error, t));
      }
      if (!mfaResponse.ok) {
        throw new Error(resolveProfileError(mfaResponse.status, mfaResult.error, t));
      }
      setConsoleProfile(profileResult);
      setPrincipal((current) => current ? { ...current, email: profileResult.email, display_name: profileResult.display_name } : current);
      setProfileForm({ display_name: profileResult.display_name || "" });
      setEmailForm({ email: profileResult.email || "", current_password: "" });
      setPasswordForm(defaultPasswordForm());
      setMfaStatus({ enabled: Boolean(mfaResult.enabled), enrolled_at: mfaResult.enrolled_at });
      setMfaEnrollment(null);
      setProfileMfaCode("");
      setProfileMessage({ kind: "", text: "" });
    } catch (error) {
      setProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("profileUnavailable") });
    } finally {
      setProfileBusy(false);
    }
  }

  async function handleProfileSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!profileForm.display_name.trim()) {
      setProfileMessage({ kind: "error", text: t("profileValidation") });
      return;
    }
    setProfileBusy(true);
    setProfileMessage({ kind: "pending", text: t("profileSaving") });
    try {
      const response = await fetch("/console/v1/profile", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ display_name: profileForm.display_name }),
      });
      const result = (await response.json().catch(() => ({}))) as ConsoleProfile & { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      setConsoleProfile(result);
      setPrincipal((current) => current ? { ...current, display_name: result.display_name, email: result.email } : current);
      setProfileForm({ display_name: result.display_name || "" });
      setProfileMessage({ kind: "success", text: t("profileSaved") });
    } catch (error) {
      setProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("profileSaveFailed") });
    } finally {
      setProfileBusy(false);
    }
  }

  async function handleEmailSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!emailForm.email.trim() || !emailForm.current_password) {
      setProfileMessage({ kind: "error", text: t("profileEmailValidation") });
      return;
    }
    setProfileBusy(true);
    setProfileMessage({ kind: "pending", text: t("profileSaving") });
    try {
      const response = await fetch("/console/v1/profile/email", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(emailForm),
      });
      const result = (await response.json().catch(() => ({}))) as ConsoleProfile & { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      forceConsoleReauth(t("profileEmailChanged"), result.email);
    } catch (error) {
      setProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("profileSaveFailed") });
    } finally {
      setProfileBusy(false);
    }
  }

  async function handlePasswordSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!passwordForm.current_password || passwordForm.new_password.length < 12 || passwordForm.new_password !== passwordForm.confirm_password) {
      setProfileMessage({ kind: "error", text: t("profilePasswordValidation") });
      return;
    }
    setProfileBusy(true);
    setProfileMessage({ kind: "pending", text: t("profileSaving") });
    try {
      const response = await fetch("/console/v1/profile/password", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ current_password: passwordForm.current_password, new_password: passwordForm.new_password }),
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      forceConsoleReauth(t("profilePasswordChanged"), consoleProfile?.email || principal?.email || "");
    } catch (error) {
      setProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("profileSaveFailed") });
    } finally {
      setProfileBusy(false);
    }
  }

  async function beginConsoleMFA() {
    if (!consoleProfile?.email) return;
    setMfaBusy(true);
    setProfileMessage({ kind: "pending", text: t("profileMFAStarting") });
    try {
      const response = await fetch("/console/v1/profile/mfa/enroll", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ issuer: "AI Token Gateway", account: consoleProfile.email }),
      });
      const result = (await response.json().catch(() => ({}))) as MFAEnrollment & { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      setMfaEnrollment(result);
      setProfileMfaCode("");
      setProfileMessage({ kind: "", text: "" });
    } catch (error) {
      setProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("profileMFAFailed") });
    } finally {
      setMfaBusy(false);
    }
  }

  async function confirmConsoleMFA(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!mfaEnrollment || !/^\d{6}$/.test(profileMfaCode.trim())) {
      setProfileMessage({ kind: "error", text: t("profileMFAValidation") });
      return;
    }
    setMfaBusy(true);
    setProfileMessage({ kind: "pending", text: t("profileMFAVerifying") });
    try {
      const response = await fetch(`/console/v1/profile/mfa/enroll/${encodeURIComponent(mfaEnrollment.enrollment_id)}/confirm`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ code: profileMfaCode.trim() }),
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      setMfaStatus({ enabled: true, enrolled_at: new Date().toISOString() });
      setMfaEnrollment(null);
      setProfileMfaCode("");
      setProfileMessage({ kind: "success", text: t("profileMFAEnabled") });
    } catch (error) {
      setProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("profileMFAFailed") });
    } finally {
      setMfaBusy(false);
    }
  }

  async function disableConsoleMFA(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!/^\d{6}$/.test(profileMfaCode.trim())) {
      setProfileMessage({ kind: "error", text: t("profileMFAValidation") });
      return;
    }
    setMfaBusy(true);
    setProfileMessage({ kind: "pending", text: t("profileMFAVerifying") });
    try {
      const response = await fetch("/console/v1/profile/mfa/disable", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ code: profileMfaCode.trim() }),
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      setMfaStatus({ enabled: false });
      setProfileMfaCode("");
      setProfileMessage({ kind: "success", text: t("profileMFADisabled") });
    } catch (error) {
      setProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("profileMFAFailed") });
    } finally {
      setMfaBusy(false);
    }
  }

  function cancelConsoleMFA() {
    if (mfaBusy) return;
    setMfaEnrollment(null);
    setProfileMfaCode("");
    setProfileMessage({ kind: "", text: "" });
  }

  function forceConsoleReauth(message: string, nextEmail = "") {
    setSignedIn(false);
    setAudience("console");
    setPrincipal(null);
    setConsoleProfile(null);
    setMfaEnrollment(null);
    setProfileMfaCode("");
    setProfileBusy(false);
    setMfaBusy(false);
    setEmail(nextEmail);
    setPassword("");
    setTenantId("");
    setLoginMessage({ kind: "success", text: message });
    window.location.hash = "#login";
  }

  async function refreshAdminSettings(showPending = false) {
    if (!signedIn || audience !== "admin" || !principal?.id) return;
    setAdminProfileBusy(true);
    if (showPending) setAdminProfileMessage({ kind: "pending", text: t("systemSettingsLoading") });
    try {
	      const [profileResponse, mfaResponse, settingsResponse, endpointResponse, emailResponse, featureResponse, templateResponse, paymentResponse, paymentPackagesResponse, loginResponse] = await Promise.all([
	        fetch("/admin/v1/profile", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	        fetch("/admin/v1/auth/mfa/status", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	        fetch("/admin/v1/settings", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	        fetch("/admin/v1/settings/api-endpoints", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	        fetch("/admin/v1/settings/email", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	        fetch("/admin/v1/settings/features", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	        fetch("/admin/v1/settings/email/templates", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	        fetch("/admin/v1/settings/payments", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	        fetch("/admin/v1/settings/payment-packages", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	        fetch("/admin/v1/settings/login", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
	      ]);
      const profileResult = (await profileResponse.json().catch(() => ({}))) as ConsoleProfile & { error?: string };
	      const mfaResult = (await mfaResponse.json().catch(() => ({}))) as MFAStatus & { error?: string };
	      const settingsResult = (await settingsResponse.json().catch(() => ({}))) as Partial<SystemSettings> & { error?: string };
	      const endpointResult = (await endpointResponse.json().catch(() => ({}))) as { api_endpoints?: APIEndpoint[]; error?: string };
	      const emailResult = (await emailResponse.json().catch(() => ({}))) as EmailSettings & { error?: string };
	      const featureResult = (await featureResponse.json().catch(() => ({}))) as FeatureSettings & { error?: string };
	      const templateResult = (await templateResponse.json().catch(() => ({}))) as { templates?: EmailTemplate[]; error?: string };
	      const paymentResult = (await paymentResponse.json().catch(() => ({}))) as { providers?: PaymentProviderConfig[]; error?: string };
	      const paymentPackagesResult = (await paymentPackagesResponse.json().catch(() => ({}))) as PaymentRechargePackages & { error?: string };
	      const loginResult = (await loginResponse.json().catch(() => ({}))) as LoginSettings & { error?: string };
	      if (!profileResponse.ok) throw new Error(resolveProfileError(profileResponse.status, profileResult.error, t));
	      if (!mfaResponse.ok) throw new Error(resolveProfileError(mfaResponse.status, mfaResult.error, t));
	      if (!settingsResponse.ok) throw new Error(resolveSystemSettingsError(settingsResponse.status, settingsResult.error, t));
	      if (!endpointResponse.ok) throw new Error(resolveAPIEndpointError(endpointResponse.status, endpointResult.error, t));
      if (!emailResponse.ok || !featureResponse.ok || !templateResponse.ok) throw new Error(t("emailSettingsUnavailable"));
      setAdminProfile(profileResult);
      setAdminProfileForm({ display_name: profileResult.display_name || "" });
      setAdminEmailForm({ email: profileResult.email || "", current_password: "" });
      setAdminPasswordForm(defaultPasswordForm());
      setAdminMfaStatus({ enabled: Boolean(mfaResult.enabled), enrolled_at: mfaResult.enrolled_at });
      setAdminMfaEnrollment(null);
      setAdminMfaCode("");
      setSecuritySettings({
        admin_mfa_enabled: Boolean(settingsResult.admin_mfa_enabled),
        updated_at: settingsResult.updated_at || "",
        updated_by: settingsResult.updated_by || "",
      });
	      setAdminSiteForm({
	        site_name: settingsResult.site_name?.trim() || "AI Token Gateway",
	        site_logo_url: settingsResult.site_logo_url?.trim() || "",
	        site_favicon_url: settingsResult.site_favicon_url?.trim() || "",
	      });
	      setAdminAPIEndpoints(endpointResult.api_endpoints || []);
	      setEmailSettings(emailResult);
	      setSmtpForm({
	        smtp_host: emailResult.smtp_host || "",
	        smtp_port: Number(emailResult.smtp_port) || 587,
	        smtp_username: emailResult.smtp_username || "",
	        smtp_password: "",
	        smtp_password_clear: false,
	        smtp_from_email: emailResult.smtp_from_email || "",
	        smtp_from_name: emailResult.smtp_from_name || "",
	        smtp_tls: emailResult.smtp_tls !== false,
	        public_base_url: emailResult.public_base_url || "",
	      });
	      setFeatureSettings({ ...defaultFeatureSettings(), ...featureResult });
      setEmailTemplates(templateResult.templates || []);
      setPaymentConfigs(paymentResponse.ok ? paymentResult.providers || [] : []);
      setPaymentSettingsMessage(paymentResponse.ok ? { kind: "", text: "" } : { kind: "error", text: t("paymentSettingsUnavailable") });
	      setPaymentRechargePackages(paymentPackagesResponse.ok ? paymentPackagesResult.packages || [] : []);
      setLoginSettings(loginResponse.ok ? { providers: loginResult.providers || [] } : { providers: [] });
      setLoginSettingsMessage(loginResponse.ok ? { kind: "", text: "" } : { kind: "error", text: t("loginSettingsUnavailable") });
      setSiteSettings({
        site_name: settingsResult.site_name?.trim() || "AI Token Gateway",
        site_logo_url: settingsResult.site_logo_url?.trim() || "",
        site_favicon_url: settingsResult.site_favicon_url?.trim() || "",
      });
      setAdminProfileMessage({ kind: "", text: "" });
    } catch (error) {
      setAdminProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("systemSettingsUnavailable") });
    } finally {
      setAdminProfileBusy(false);
    }
  }

  async function handleAdminProfileSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!adminProfileForm.display_name.trim()) {
      setAdminProfileMessage({ kind: "error", text: t("adminProfileValidation") });
      return;
    }
    setAdminProfileBusy(true);
    setAdminProfileMessage({ kind: "pending", text: t("adminProfileSaving") });
    try {
      const response = await fetch("/admin/v1/profile", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(adminProfileForm),
      });
      const result = (await response.json().catch(() => ({}))) as ConsoleProfile & { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      setAdminProfile(result);
      setPrincipal((current) => current ? { ...current, email: result.email, display_name: result.display_name } : current);
      setAdminProfileForm({ display_name: result.display_name || "" });
      setAdminProfileMessage({ kind: "success", text: t("adminProfileSaved") });
    } catch (error) {
      setAdminProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("systemSettingsSaveFailed") });
    } finally {
      setAdminProfileBusy(false);
    }
  }

  async function handleAdminEmailSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!adminEmailForm.email.trim() || !adminEmailForm.current_password) {
      setAdminProfileMessage({ kind: "error", text: t("adminEmailValidation") });
      return;
    }
    setAdminProfileBusy(true);
    setAdminProfileMessage({ kind: "pending", text: t("adminProfileSaving") });
    try {
      const response = await fetch("/admin/v1/profile/email", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(adminEmailForm),
      });
      const result = (await response.json().catch(() => ({}))) as ConsoleProfile & { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      forceAdminReauth(t("adminEmailChanged"), result.email);
    } catch (error) {
      setAdminProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("systemSettingsSaveFailed") });
    } finally {
      setAdminProfileBusy(false);
    }
  }

  async function handleAdminPasswordSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!adminPasswordForm.current_password || adminPasswordForm.new_password.length < 12 || adminPasswordForm.new_password !== adminPasswordForm.confirm_password) {
      setAdminProfileMessage({ kind: "error", text: t("adminPasswordValidation") });
      return;
    }
    setAdminProfileBusy(true);
    setAdminProfileMessage({ kind: "pending", text: t("adminProfileSaving") });
    try {
      const response = await fetch("/admin/v1/profile/password", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ current_password: adminPasswordForm.current_password, new_password: adminPasswordForm.new_password }),
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      forceAdminReauth(t("adminPasswordChanged"), adminProfile?.email || principal?.email || "");
    } catch (error) {
      setAdminProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("systemSettingsSaveFailed") });
    } finally {
      setAdminProfileBusy(false);
    }
  }

  async function beginAdminMFA() {
    if (!adminProfile?.email) return;
    setAdminMfaBusy(true);
    setAdminProfileMessage({ kind: "pending", text: t("adminMFAStarting") });
    try {
      const response = await fetch("/admin/v1/auth/mfa/enroll", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ issuer: "AI Token Gateway", account: adminProfile.email }),
      });
      const result = (await response.json().catch(() => ({}))) as MFAEnrollment & { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      setAdminMfaEnrollment(result);
      setAdminMfaCode("");
      setAdminProfileMessage({ kind: "", text: "" });
    } catch (error) {
      setAdminProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("adminMFAFailed") });
    } finally {
      setAdminMfaBusy(false);
    }
  }

  async function confirmAdminMFA(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!adminMfaEnrollment || !/^\d{6}$/.test(adminMfaCode.trim())) {
      setAdminProfileMessage({ kind: "error", text: t("adminMFAValidation") });
      return;
    }
    setAdminMfaBusy(true);
    setAdminProfileMessage({ kind: "pending", text: t("adminMFAVerifying") });
    try {
      const response = await fetch(`/admin/v1/auth/mfa/enroll/${encodeURIComponent(adminMfaEnrollment.enrollment_id)}/confirm`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ code: adminMfaCode.trim() }),
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(resolveProfileError(response.status, result.error, t));
      setAdminMfaStatus({ enabled: true, enrolled_at: new Date().toISOString() });
      setAdminMfaEnrollment(null);
      setAdminMfaCode("");
      setAdminProfileMessage({ kind: "success", text: t("adminMFAEnabled") });
    } catch (error) {
      setAdminProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("adminMFAFailed") });
    } finally {
      setAdminMfaBusy(false);
    }
  }

  async function disableAdminMFA(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!/^\d{6}$/.test(adminMfaCode.trim())) {
      setAdminProfileMessage({ kind: "error", text: t("adminMFAValidation") });
      return;
    }
    setAdminMfaBusy(true);
    setAdminProfileMessage({ kind: "pending", text: t("adminMFAVerifying") });
    try {
      const response = await fetch("/admin/v1/auth/mfa/disable", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ code: adminMfaCode.trim() }),
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(resolveSystemSettingsError(response.status, result.error, t));
      setAdminMfaStatus({ enabled: false });
      setAdminMfaCode("");
      setAdminProfileMessage({ kind: "success", text: t("adminMFADisabled") });
    } catch (error) {
      setAdminProfileMessage({ kind: "error", text: error instanceof Error ? error.message : t("adminMFAFailed") });
    } finally {
      setAdminMfaBusy(false);
    }
  }

  function cancelAdminMFA() {
    if (adminMfaBusy) return;
    setAdminMfaEnrollment(null);
    setAdminMfaCode("");
    setAdminProfileMessage({ kind: "", text: "" });
  }

  async function saveSystemSettings(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!adminSiteForm.site_name.trim()) {
      setSiteSettingsMessage({ kind: "error", text: t("systemSettingsValidation") });
      return;
    }
    setSiteSettingsBusy(true);
    setSiteSettingsMessage({ kind: "pending", text: t("systemSettingsSaving") });
    try {
	      const response = await fetchAdminSensitive("/admin/v1/settings/site", {
	        method: "PUT",
	        headers: { "Content-Type": "application/json", Accept: "application/json" },
	        credentials: "same-origin",
	        body: JSON.stringify(adminSiteForm),
      });
      const result = (await response.json().catch(() => ({}))) as Partial<SystemSettings> & { error?: string };
      if (!response.ok) throw new Error(resolveSystemSettingsError(response.status, result.error, t));
      const next = {
        site_name: result.site_name?.trim() || adminSiteForm.site_name.trim(),
        site_logo_url: result.site_logo_url?.trim() || "",
        site_favicon_url: result.site_favicon_url?.trim() || "",
      };
      setAdminSiteForm(next);
      setSiteSettings(next);
      setSecuritySettings((current) => ({ ...current, updated_at: result.updated_at || current.updated_at, updated_by: result.updated_by || current.updated_by }));
      setSiteSettingsMessage({ kind: "success", text: t("systemSettingsSaved") });
    } catch (error) {
      setSiteSettingsMessage({ kind: "error", text: error instanceof Error ? error.message : t("systemSettingsSaveFailed") });
    } finally {
      setSiteSettingsBusy(false);
    }
  }

  function openCreateAPIEndpoint() {
    setAPIEndpointForm(defaultAPIEndpointForm());
    setAPIEndpointFormOpen(true);
    setAPIEndpointMessage({ kind: "", text: "" });
  }

  function openEditAPIEndpoint(endpoint: APIEndpoint) {
    setAPIEndpointForm({
      id: endpoint.id,
      name: endpoint.name,
      base_url: endpoint.base_url,
      enabled: endpoint.enabled,
    });
    setAPIEndpointFormOpen(true);
    setAPIEndpointMessage({ kind: "", text: "" });
  }

  function closeAPIEndpointForm() {
    if (apiEndpointBusy) return;
    setAPIEndpointFormOpen(false);
    setAPIEndpointForm(defaultAPIEndpointForm());
    setAPIEndpointMessage({ kind: "", text: "" });
  }

  function applyAPIEndpointUpdate(endpoint: APIEndpoint, previousBaseURL = "") {
    const endpointURLs = resolveAPIEndpointURLs(endpoint);
    const normalizedEndpoint: APIEndpoint = {
      ...endpoint,
      base_url: endpointURLs.root,
      openai_base_url: endpointURLs.openai,
      anthropic_base_url: endpointURLs.anthropic,
    };
    setAdminAPIEndpoints((current) => {
      const next = normalizedEndpoint.id
        ? [...current.filter((item) => item.id !== normalizedEndpoint.id), normalizedEndpoint]
        : [...current, normalizedEndpoint];
      return next.sort((left, right) => left.sort_order - right.sort_order || left.name.localeCompare(right.name));
    });
    setPublicAPIEndpoints((current) => {
      const next = current.filter((item) => item.base_url !== normalizedEndpoint.base_url && (!previousBaseURL || item.base_url !== previousBaseURL));
      return normalizedEndpoint.enabled
        ? [...next, {
          name: normalizedEndpoint.name,
          base_url: normalizedEndpoint.base_url,
          openai_base_url: normalizedEndpoint.openai_base_url,
          anthropic_base_url: normalizedEndpoint.anthropic_base_url,
        }].sort((left, right) => left.name.localeCompare(right.name))
        : next;
    });
  }

  async function saveAPIEndpoint(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!apiEndpointForm.name.trim() || !apiEndpointForm.base_url.trim()) {
      setAPIEndpointMessage({ kind: "error", text: t("systemAPIEndpointValidation") });
      return;
    }
    setAPIEndpointBusy(true);
    setAPIEndpointMessage({ kind: "pending", text: t("systemAPIEndpointSaving") });
    try {
      const response = await fetchAdminSensitive(
        apiEndpointForm.id
          ? `/admin/v1/settings/api-endpoints/${encodeURIComponent(apiEndpointForm.id)}`
          : "/admin/v1/settings/api-endpoints",
        {
          method: apiEndpointForm.id ? "PUT" : "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          credentials: "same-origin",
          body: JSON.stringify({
            name: apiEndpointForm.name.trim(),
            base_url: apiEndpointForm.base_url.trim(),
            enabled: apiEndpointForm.enabled,
          }),
        }
      );
      const result = (await response.json().catch(() => ({}))) as APIEndpoint & { error?: string };
      if (!response.ok) throw new Error(resolveAPIEndpointError(response.status, result.error, t));
      applyAPIEndpointUpdate(result, apiEndpointForm.id ? adminAPIEndpoints.find((item) => item.id === apiEndpointForm.id)?.base_url : "");
      setAPIEndpointFormOpen(false);
      setAPIEndpointForm(defaultAPIEndpointForm());
      setAPIEndpointMessage({ kind: "success", text: t("systemAPIEndpointSaved") });
    } catch (error) {
      setAPIEndpointMessage({ kind: "error", text: error instanceof Error ? error.message : t("systemAPIEndpointUnavailable") });
    } finally {
      setAPIEndpointBusy(false);
    }
  }

  async function toggleAPIEndpoint(endpoint: APIEndpoint) {
    setAPIEndpointActionBusy(endpoint.id);
    setAPIEndpointMessage({ kind: "pending", text: t("systemAPIEndpointSaving") });
    try {
      const response = await fetchAdminSensitive(`/admin/v1/settings/api-endpoints/${encodeURIComponent(endpoint.id)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ name: endpoint.name, base_url: endpoint.base_url, enabled: !endpoint.enabled }),
      });
      const result = (await response.json().catch(() => ({}))) as APIEndpoint & { error?: string };
      if (!response.ok) throw new Error(resolveAPIEndpointError(response.status, result.error, t));
      applyAPIEndpointUpdate(result);
      setAPIEndpointMessage({ kind: "success", text: result.enabled ? t("systemAPIEndpointEnabled") : t("systemAPIEndpointDisabled") });
    } catch (error) {
      setAPIEndpointMessage({ kind: "error", text: error instanceof Error ? error.message : t("systemAPIEndpointUnavailable") });
    } finally {
      setAPIEndpointActionBusy("");
    }
  }

  async function deleteAPIEndpoint(endpoint: APIEndpoint) {
    if (!window.confirm(t("systemAPIEndpointDeleteConfirm"))) return;
    setAPIEndpointActionBusy(endpoint.id);
    setAPIEndpointMessage({ kind: "pending", text: t("systemAPIEndpointSaving") });
    try {
      const response = await fetchAdminSensitive(`/admin/v1/settings/api-endpoints/${encodeURIComponent(endpoint.id)}`, {
        method: "DELETE",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(resolveAPIEndpointError(response.status, result.error, t));
      setAdminAPIEndpoints((current) => current.filter((item) => item.id !== endpoint.id));
      setPublicAPIEndpoints((current) => current.filter((item) => item.base_url !== endpoint.base_url));
      setAPIEndpointMessage({ kind: "success", text: t("systemAPIEndpointDeleted") });
    } catch (error) {
      setAPIEndpointMessage({ kind: "error", text: error instanceof Error ? error.message : t("systemAPIEndpointUnavailable") });
    } finally {
      setAPIEndpointActionBusy("");
    }
  }

  function forceAdminReauth(message: string, nextEmail = "") {
    setSignedIn(false);
    setAudience("admin");
    setPrincipal(null);
    setAdminProfile(null);
    setAdminMfaEnrollment(null);
    setAdminMfaCode("");
    setAdminProfileBusy(false);
    setAdminMfaBusy(false);
    setEmail(nextEmail);
    setPassword("");
    setMfaCode("");
    setLoginMessage({ kind: "success", text: message });
    window.location.hash = "#login";
  }

  async function refreshUsageReport(showPending = false, offset = reportOffset) {
    if (!signedIn || audience !== "admin") return;
    setUsageReportBusy(true);
    if (showPending) setUsageReportMessage({ kind: "pending", text: t("usageRecordsLoading") });
    try {
      const params = new URLSearchParams({ limit: "50", offset: String(offset) });
      if (reportSearch.trim()) params.set("search", reportSearch.trim());
      if (reportStatus) params.set("status", reportStatus);
      if (reportTenant.trim()) params.set("tenant_id", reportTenant.trim());
      if (reportModel.trim()) params.set("model", reportModel.trim());
      if (reportGroup.trim()) params.set("group_id", reportGroup.trim());
      if (reportFrom) params.set("from", new Date(`${reportFrom}T00:00:00Z`).toISOString());
      if (reportTo) params.set("to", new Date(`${reportTo}T23:59:59.999Z`).toISOString());
      const response = await fetch(`/admin/v1/usage?${params.toString()}`, {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as UsageReport & { error?: string };
      if (!response.ok) throw new Error(result.error || "usage report unavailable");
      setUsageReport(result);
      setReportOffset(offset);
      setUsageReportMessage({ kind: "", text: "" });
    } catch {
      setUsageReportMessage({ kind: "error", text: t("usageRecordsUnavailable") });
    } finally {
      setUsageReportBusy(false);
    }
  }

  async function refreshFinanceReport(showPending = false, offset = 0) {
    if (!signedIn || audience !== "admin") return;
    setFinanceReportBusy(true);
    if (showPending) setFinanceReportMessage({ kind: "pending", text: t("financeLoading") });
    try {
      const params = new URLSearchParams({ limit: "50", offset: String(offset) });
      if (financeSearch.trim()) params.set("search", financeSearch.trim());
      if (financeCurrency.trim()) params.set("currency", financeCurrency.trim().toUpperCase());
      if (financeFrom) params.set("from", new Date(`${financeFrom}T00:00:00Z`).toISOString());
      if (financeTo) params.set("to", new Date(`${financeTo}T23:59:59.999Z`).toISOString());
      const response = await fetch(`/admin/v1/finance?${params.toString()}`, {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as FinanceReport & { error?: string };
      if (!response.ok) throw new Error(result.error || "finance report unavailable");
      setFinanceReport(result);
      setFinanceReportMessage({ kind: "", text: "" });
    } catch {
      setFinanceReportMessage({ kind: "error", text: t("financeUnavailable") });
    } finally {
      setFinanceReportBusy(false);
    }
  }

	async function refreshOperations(showPending = false) {
		if (!signedIn || audience !== "admin") return;
		setOperationsBusy(true);
		try {
			const response = await fetch("/admin/v1/overview", { headers: { Accept: "application/json" }, credentials: "same-origin" });
			const result = (await response.json().catch(() => ({}))) as OperationsSnapshot & { error?: string };
			if (!response.ok) throw new Error(result.error || "operations unavailable");
			setOperationsSnapshot(result);
		} catch {
			if (showPending) setOperationsSnapshot(null);
		}
		setOperationsBusy(false);
	}

	async function saveEmailSettings(event: React.FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setEmailBusy(true);
		setEmailMessage({ kind: "pending", text: t("emailSettingsSaving") });
		try {
			const response = await fetchAdminSensitive("/admin/v1/settings/email", {
				method: "PUT",
				headers: { "Content-Type": "application/json", Accept: "application/json" },
				credentials: "same-origin",
				body: JSON.stringify(smtpForm),
			});
			const result = (await response.json().catch(() => ({}))) as EmailSettings & { error?: string };
			if (!response.ok) throw new Error(response.status === 400 ? t("emailSettingsValidation") : t("emailSettingsSaveFailed"));
			setEmailSettings(result);
			setSmtpForm((current) => ({ ...current, smtp_password: "", smtp_password_clear: false }));
			setEmailMessage({ kind: "success", text: t("emailSettingsSaved") });
		} catch (error) {
			setEmailMessage({ kind: "error", text: error instanceof Error ? error.message : t("emailSettingsSaveFailed") });
		} finally {
			setEmailBusy(false);
		}
	}

	async function testSMTPConnection() {
		setSMTPConnectionBusy(true);
		setEmailMessage({ kind: "pending", text: t("emailSMTPTesting") });
		try {
			const response = await fetchAdminSensitive("/admin/v1/settings/email/test-connection", { headers: { Accept: "application/json" }, credentials: "same-origin" });
			if (!response.ok) throw new Error(t("emailSMTPConnectionFailed"));
			setEmailMessage({ kind: "success", text: t("emailSMTPConnectionSuccess") });
		} catch (error) {
			setEmailMessage({ kind: "error", text: error instanceof Error ? error.message : t("emailSMTPConnectionFailed") });
		} finally {
			setSMTPConnectionBusy(false);
		}
	}

	async function sendTestEmail() {
		if (!emailTestRecipient.trim()) {
			setEmailMessage({ kind: "error", text: t("emailTestRecipientRequired") });
			return;
		}
		setSMTPMessageBusy(true);
		setEmailMessage({ kind: "pending", text: t("emailTestSending") });
		try {
			const response = await fetchAdminSensitive("/admin/v1/settings/email/test-message", {
				method: "POST",
				headers: { "Content-Type": "application/json", Accept: "application/json" },
				credentials: "same-origin",
				body: JSON.stringify({ recipient: emailTestRecipient.trim() }),
			});
			if (!response.ok) throw new Error(t("emailTestFailed"));
			setEmailMessage({ kind: "success", text: t("emailTestSent") });
		} catch (error) {
			setEmailMessage({ kind: "error", text: error instanceof Error ? error.message : t("emailTestFailed") });
		} finally {
			setSMTPMessageBusy(false);
		}
	}

	async function saveFeatureSettings(next: FeatureSettings) {
		setFeatureBusy(true);
		setFeatureMessage({ kind: "pending", text: t("featureSettingsSaving") });
		try {
			const response = await fetchAdminSensitive("/admin/v1/settings/features", {
				method: "PUT",
				headers: { "Content-Type": "application/json", Accept: "application/json" },
				credentials: "same-origin",
				body: JSON.stringify(featureSettingsUpdatePayload(next)),
			});
			const result = (await response.json().catch(() => ({}))) as FeatureSettings & { error?: string };
			if (!response.ok) throw new Error(resolveFeatureSettingsError(response.status, result.error, t));
			setFeatureSettings(result);
			setPublicFeatures({
				registration_enabled: result.registration_enabled !== false,
				model_status_enabled: result.model_status_enabled !== false,
				totp_enabled: result.totp_enabled === true,
			});
			if (result.totp_enabled !== true) {
				setSecuritySettings((current) => ({ ...current, admin_mfa_enabled: false }));
			}
			setEmailSettings((current) => current ? { ...current, email_enabled: result.email_enabled, balance_threshold: result.balance_threshold, recharge_url: result.recharge_url } : current);
			setFeatureMessage({ kind: "success", text: t("featureSettingsSaved") });
		} catch (error) {
			setFeatureMessage({ kind: "error", text: error instanceof Error ? error.message : t("featureSettingsSaveFailed") });
		} finally {
			setFeatureBusy(false);
		}
	}

  async function savePaymentConfig(provider: PaymentProviderConfig["provider"], enabled: boolean, values: Record<string, string>, clear: string[]) {
    setPaymentSettingsBusy(true);
    setPaymentSettingsMessage({ kind: "pending", text: t("paymentSaving") });
    try {
      const response = await fetchAdminSensitive(`/admin/v1/settings/payments/${encodeURIComponent(provider)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ enabled, values, clear }),
      });
      const result = (await response.json().catch(() => ({}))) as PaymentProviderConfig & { error?: string };
      if (!response.ok) throw new Error(resolvePaymentError(response.status, result.error, t));
      setPaymentConfigs((current) => current.map((item) => item.provider === result.provider ? result : item));
      if (!paymentConfigs.some((item) => item.provider === result.provider)) setPaymentConfigs((current) => [...current, result]);
      setPaymentSettingsMessage({ kind: "success", text: t("paymentSaved") });
      setPaymentProviders((current) => {
        const next = current.filter((item) => item.provider !== result.provider);
        return result.enabled ? [...next, { provider: result.provider, enabled: true }] : next;
      });
    } catch (error) {
      setPaymentSettingsMessage({ kind: "error", text: error instanceof Error ? error.message : t("paymentSaveFailed") });
    } finally {
      setPaymentSettingsBusy(false);
    }
  }

  async function savePaymentRechargePackages(packages: PaymentRechargePackage[]) {
    setPaymentSettingsBusy(true);
    setPaymentSettingsMessage({ kind: "pending", text: t("paymentRechargePackagesSaving") });
    try {
      const response = await fetchAdminSensitive("/admin/v1/settings/payment-packages", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ packages }),
      });
      const result = (await response.json().catch(() => ({}))) as PaymentRechargePackages & { error?: string };
      if (!response.ok) throw new Error(resolvePaymentError(response.status, result.error, t));
      setPaymentRechargePackages(result.packages || []);
      setPaymentSettingsMessage({ kind: "success", text: t("paymentRechargePackagesSaved") });
    } catch (error) {
      setPaymentSettingsMessage({ kind: "error", text: error instanceof Error ? error.message : t("paymentRechargePackagesSaveFailed") });
    } finally {
      setPaymentSettingsBusy(false);
    }
  }

  async function saveLoginSettings(settings: LoginSettings) {
    setLoginSettingsBusy(true);
    setLoginSettingsMessage({ kind: "pending", text: t("loginSettingsSaving") });
    try {
      const response = await fetchAdminSensitive("/admin/v1/settings/login", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ providers: settings.providers.map((item) => ({ provider: item.provider, enabled: item.enabled, client_id: item.client_id, client_secret: item.client_secret || "", clear_client_secret: Boolean(item.clear_client_secret), authorization_url: item.authorization_url, token_url: item.token_url, userinfo_url: item.userinfo_url, scopes: item.scopes })) }),
      });
      const result = (await response.json().catch(() => ({}))) as LoginSettings & { error?: string };
      if (!response.ok) throw new Error(result.error || "login settings unavailable");
      setLoginSettings({ providers: result.providers || [] });
      setLoginSettingsMessage({ kind: "success", text: t("loginSettingsSaved") });
    } catch (error) {
      setLoginSettingsMessage({ kind: "error", text: error instanceof Error && error.message === "INVALID_LOGIN_SETTINGS" ? t("loginSettingsValidation") : t("loginSettingsSaveFailed") });
    } finally {
      setLoginSettingsBusy(false);
    }
  }

	async function saveEmailTemplate(form: EmailTemplateFormState): Promise<boolean> {
		setEmailTemplatesBusy(true);
		setEmailTemplatesMessage({ kind: "pending", text: t("emailTemplateSaving") });
		try {
			const response = await fetchAdminSensitive(form.id ? `/admin/v1/settings/email/templates/${encodeURIComponent(form.id)}` : "/admin/v1/settings/email/templates", {
				method: form.id ? "PUT" : "POST",
				headers: { "Content-Type": "application/json", Accept: "application/json" },
				credentials: "same-origin",
				body: JSON.stringify(form),
			});
			const result = (await response.json().catch(() => ({}))) as EmailTemplate & { error?: string };
			if (!response.ok) throw new Error(response.status === 400 ? t("emailTemplateValidation") : t("emailTemplateSaveFailed"));
			setEmailTemplates((current) => form.id ? current.map((item) => item.id === result.id ? result : item) : [result, ...current]);
			setEmailTemplatesMessage({ kind: "success", text: t("emailTemplateSaved") });
			return true;
		} catch (error) {
			setEmailTemplatesMessage({ kind: "error", text: error instanceof Error ? error.message : t("emailTemplateSaveFailed") });
			return false;
		} finally {
			setEmailTemplatesBusy(false);
		}
	}

	async function deleteEmailTemplate(template: EmailTemplate) {
		if (!window.confirm(t("emailTemplateDeleteConfirm"))) return;
		setEmailTemplatesBusy(true);
		try {
			const response = await fetchAdminSensitive(`/admin/v1/settings/email/templates/${encodeURIComponent(template.id)}`, { method: "DELETE", headers: { Accept: "application/json" }, credentials: "same-origin" });
			if (!response.ok) throw new Error(t("emailTemplateSaveFailed"));
			setEmailTemplates((current) => current.filter((item) => item.id !== template.id));
			setEmailTemplatesMessage({ kind: "success", text: t("emailTemplateDeleted") });
		} catch (error) {
			setEmailTemplatesMessage({ kind: "error", text: error instanceof Error ? error.message : t("emailTemplateSaveFailed") });
		} finally {
			setEmailTemplatesBusy(false);
		}
	}

	async function refreshAudit(showPending = false, offset = 0) {
		if (!signedIn || audience !== "admin") return;
		setAuditBusy(true);
		if (showPending) setAuditMessage({ kind: "pending", text: t("auditLoading") });
		try {
			const response = await fetch(`/admin/v1/audit?limit=50&offset=${offset}`, { headers: { Accept: "application/json" }, credentials: "same-origin" });
			const result = (await response.json().catch(() => ({}))) as AuditReport & { error?: string };
			if (!response.ok) throw new Error(result.error || "audit unavailable");
			setAuditReport(result);
			setAuditMessage({ kind: "", text: "" });
		} catch {
			setAuditMessage({ kind: "error", text: t("auditUnavailable") });
		} finally {
			setAuditBusy(false);
		}
	}

  async function refreshChannels(showPending = false) {
    if (!signedIn || audience !== "admin") return;
    setChannelsBusy(true);
    if (showPending) {
      setChannelsMessage({ kind: "pending", text: t("channelsLoading") });
    }
    try {
      const response = await fetch("/admin/v1/channels", {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) {
        throw new Error("channels unavailable");
      }
      const result = (await response.json()) as { channels?: ChannelSummary[] };
      setChannels(result.channels || []);
      setChannelsMessage({ kind: "", text: "" });
    } catch {
      setChannelsMessage({ kind: "error", text: t("channelsUnavailable") });
    } finally {
      setChannelsBusy(false);
    }
  }

  async function syncChannelAccount(channel: ChannelSummary) {
    if (channelActionBusy) return;
    setChannelActionBusy(`account-sync:${channel.id}`);
    setChannelsMessage({ kind: "pending", text: t("channelsAccountSyncing") });
    try {
      const response = await fetch(`/admin/v1/channels/${encodeURIComponent(channel.id)}/sync-account`, {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as ChannelSummary & { error?: string };
      if (!response.ok) {
        throw new Error(result.error || "account sync failed");
      }
      setChannels((current) => current.map((item) => (item.id === result.id ? result : item)));
      setChannelsMessage({
        kind: result.upstream_account_sync_status === "success" ? "success" : "pending",
        text:
          result.upstream_account_sync_status === "success"
            ? t("channelsAccountSyncSuccess")
            : t("channelsAccountSyncCompleted"),
      });
    } catch {
      setChannelsMessage({ kind: "error", text: t("channelsAccountSyncFailed") });
    } finally {
      setChannelActionBusy("");
    }
  }

  async function refreshGroups(showPending = false) {
    if (!signedIn || audience !== "admin") return;
    setGroupsBusy(true);
    if (showPending) {
      setGroupsMessage({ kind: "pending", text: t("groupsLoading") });
    }
    try {
      const response = await fetch("/admin/v1/groups", {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { groups?: GroupSummary[] };
      if (!response.ok) {
        throw new Error("groups unavailable");
      }
      setGroups(result.groups || []);
      setGroupsMessage({ kind: "", text: "" });
    } catch {
      setGroupsMessage({ kind: "error", text: t("groupsUnavailable") });
    } finally {
      setGroupsBusy(false);
    }
  }

  function openCreateGroup() {
    setGroupForm(defaultGroupForm());
    setGroupFormOpen(true);
    setGroupDeleteConfirm("");
    setGroupsMessage({ kind: "", text: "" });
  }

  function openEditGroup(group: GroupSummary) {
    setGroupForm(groupFormFromSummary(group));
    setGroupFormOpen(true);
    setGroupDeleteConfirm("");
    setGroupsMessage({ kind: "", text: "" });
  }

  function closeGroupForm() {
    setGroupFormOpen(false);
    setGroupForm(defaultGroupForm());
    setGroupsMessage({ kind: "", text: "" });
  }

  async function handleGroupSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const multiplier = groupForm.multiplier.trim();
    if (!groupForm.code.trim() || !groupForm.name.trim() || !multiplier || !Number.isFinite(Number(multiplier)) || Number(multiplier) <= 0) {
      setGroupsMessage({ kind: "error", text: t("groupsValidationFailed") });
      return;
    }
    setGroupActionBusy(groupForm.id ? `update:${groupForm.id}` : "create");
    setGroupsMessage({ kind: "pending", text: t("groupsSavePending") });
    try {
      const response = await fetchAdminSensitive(groupForm.id ? `/admin/v1/groups/${encodeURIComponent(groupForm.id)}` : "/admin/v1/groups", {
        method: groupForm.id ? "PUT" : "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({
          code: groupForm.code.trim(),
          name: groupForm.name.trim(),
          description: groupForm.description.trim(),
          status: groupForm.status,
          multiplier,
          rpm_limit: Number(groupForm.rpm_limit),
          billing_type: groupForm.billing_type,
          metering_mode: groupForm.metering_mode,
          metering_price: groupForm.metering_mode === "token" ? "0" : groupForm.metering_price.trim(),
          priority: Number(groupForm.priority),
          channel_ids: groupForm.channel_ids,
        }),
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) {
        throw new Error(result.error || "group save failed");
      }
      await refreshGroups(false);
      setGroupFormOpen(false);
      setGroupForm(defaultGroupForm());
      setGroupsMessage({ kind: "success", text: t("groupsSaveSuccess") });
    } catch (error) {
      setGroupsMessage({ kind: "error", text: error instanceof Error && error.message === "GROUP_IN_USE" ? t("groupsInUse") : error instanceof Error && error.message === "DEFAULT_GROUP_PROTECTED" ? t("groupsDefaultProtected") : t("groupsValidationFailed") });
    } finally {
      setGroupActionBusy("");
    }
  }

  async function deleteGroup(group: GroupSummary) {
    if (groupDeleteConfirm !== group.id) {
      setGroupDeleteConfirm(group.id);
      setGroupsMessage({ kind: "pending", text: t("groupsDeleteConfirm") });
      return;
    }
    setGroupActionBusy(`delete:${group.id}`);
    try {
      const response = await fetchAdminSensitive(`/admin/v1/groups/${encodeURIComponent(group.id)}`, {
        method: "DELETE",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) {
        const result = (await response.json().catch(() => ({}))) as { error?: string };
        throw new Error(result.error || "group delete failed");
      }
      await refreshGroups(false);
      setGroupDeleteConfirm("");
      setGroupsMessage({ kind: "success", text: t("groupsDeleteSuccess") });
    } catch (error) {
      setGroupsMessage({ kind: "error", text: error instanceof Error && error.message === "GROUP_IN_USE" ? t("groupsInUse") : error instanceof Error && error.message === "DEFAULT_GROUP_PROTECTED" ? t("groupsDefaultProtected") : t("groupsValidationFailed") });
    } finally {
      setGroupActionBusy("");
    }
  }

  async function refreshTokens(showPending = false) {
    if (!signedIn || audience !== "admin") return;
    setTokensBusy(true);
    if (showPending) {
      setTokensMessage({ kind: "pending", text: t("tokensLoading") });
    }
    try {
      const response = await fetch("/admin/v1/tokens", {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { tokens?: TokenSummary[] };
      if (!response.ok) {
        throw new Error("tokens unavailable");
      }
      setTokens(result.tokens || []);
      setTokensMessage({ kind: "", text: "" });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensUnavailable") });
    } finally {
      setTokensBusy(false);
    }
  }

  async function refreshConsoleTokens(showPending = false) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("token:read")) return;
    setTokensBusy(true);
    if (showPending) setTokensMessage({ kind: "pending", text: t("tokensLoading") });
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/tokens`, {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { tokens?: TokenSummary[] };
      if (!response.ok) throw new Error("tokens unavailable");
      setTokens(result.tokens || []);
      setTokensMessage({ kind: "", text: "" });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensUnavailable") });
    } finally {
      setTokensBusy(false);
    }
  }

  async function refreshConsoleTokenGroups() {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("token:create")) return;
    setConsoleTokenGroupsBusy(true);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/token-groups`, {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { groups?: TokenGroupOption[] };
      if (!response.ok) throw new Error("token groups unavailable");
      setConsoleTokenGroups(result.groups || []);
    } catch {
      setConsoleTokenGroups([]);
    } finally {
      setConsoleTokenGroupsBusy(false);
    }
  }

  async function refreshConsoleProjects(showPending = false) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("project:read")) return;
    setProjectsBusy(true);
    if (showPending) setProjectsMessage({ kind: "pending", text: t("consoleUsageLoading") });
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/projects`, { headers: { Accept: "application/json" }, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as { projects?: ProjectSummary[]; error?: string };
      if (!response.ok) throw new Error(result.error || "projects unavailable");
      const nextProjects = result.projects || [];
      setProjects(nextProjects);
      setPrincipal((current) => current ? { ...current, project_ids: nextProjects.filter((project) => project.status === "active").map((project) => project.id) } : current);
      setProjectsMessage({ kind: "", text: "" });
    } catch {
      setProjectsMessage({ kind: "error", text: t("consoleProjectActionFailed") });
    } finally {
      setProjectsBusy(false);
    }
  }

  async function refreshConsoleMembers(showPending = false) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("member:invite")) return;
    setMembersBusy(true);
    if (showPending) setMembersMessage({ kind: "pending", text: t("consoleUsageLoading") });
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/members`, { headers: { Accept: "application/json" }, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as { members?: TenantMember[]; error?: string };
      if (!response.ok) throw new Error(result.error || "members unavailable");
      setMembers(result.members || []);
      setMembersMessage({ kind: "", text: "" });
    } catch {
      setMembersMessage({ kind: "error", text: t("consoleMemberFailed") });
    } finally {
      setMembersBusy(false);
    }
  }

  async function refreshConsoleProjectMembers(projectID: string, showPending = false) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !projectID || !hasConsolePermission("project:update")) return;
    setProjectMembersBusy(true);
    if (showPending) setProjectMembersMessage({ kind: "pending", text: t("consoleUsageLoading") });
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/projects/${encodeURIComponent(projectID)}/members`, { headers: { Accept: "application/json" }, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as { members?: ProjectMember[]; error?: string };
      if (!response.ok) throw new Error(result.error || "project members unavailable");
      setProjectMembers(result.members || []);
      setProjectMembersMessage({ kind: "", text: "" });
    } catch {
      setProjectMembers([]);
      setProjectMembersMessage({ kind: "error", text: t("consoleProjectMemberFailed") });
    } finally {
      setProjectMembersBusy(false);
    }
  }

  async function saveConsoleProject(form: ProjectFormState): Promise<boolean> {
    if (!principal?.tenant_id) return false;
    setProjectActionBusy(form.id || "new");
    try {
      const response = await fetch(form.id
        ? `/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/projects/${encodeURIComponent(form.id)}`
        : `/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/projects`, {
        method: form.id ? "PUT" : "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ name: form.name.trim(), slug: form.slug.trim(), status: form.status }),
      });
      const result = (await response.json().catch(() => ({}))) as ProjectSummary & { error?: string };
      if (!response.ok) throw new Error(result.error || "project save failed");
      await refreshConsoleProjects(false);
      setProjectsMessage({ kind: "success", text: form.id ? t("consoleProjectSaveSuccess") : t("consoleProjectCreateSuccess") });
      return true;
    } catch (error) {
      setProjectsMessage({ kind: "error", text: error instanceof Error && error.message === "TENANT_RESOURCE_EXISTS" ? t("consoleProjectActionFailed") : t("consoleProjectActionFailed") });
      return false;
    } finally {
      setProjectActionBusy("");
    }
  }

  async function deleteConsoleProject(project: ProjectSummary) {
    if (!principal?.tenant_id) return;
    if (projectDeleteConfirm !== project.id) { setProjectDeleteConfirm(project.id); setProjectsMessage({ kind: "pending", text: t("consoleProjectDelete") }); return; }
    setProjectActionBusy(project.id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/projects/${encodeURIComponent(project.id)}`, { method: "DELETE", headers: { Accept: "application/json" }, credentials: "same-origin" });
      if (!response.ok) throw new Error("project delete failed");
      setProjectDeleteConfirm("");
      if (selectedProjectID === project.id) { setSelectedProjectID(""); setProjectMembers([]); }
      await refreshConsoleProjects(false);
      setProjectsMessage({ kind: "success", text: t("consoleProjectDeleteSuccess") });
    } catch {
      setProjectsMessage({ kind: "error", text: t("consoleProjectActionFailed") });
    } finally {
      setProjectActionBusy("");
    }
  }

  async function addConsoleMember(email: string, role: TenantMember["role"]) {
    if (!principal?.tenant_id) return;
    setMemberActionBusy("new");
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/members`, { method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, credentials: "same-origin", body: JSON.stringify({ email, role }) });
      if (!response.ok) throw new Error("member add failed");
      await refreshConsoleMembers(false);
      setMembersMessage({ kind: "success", text: t("consoleMemberAddSuccess") });
    } catch { setMembersMessage({ kind: "error", text: t("consoleMemberFailed") }); } finally { setMemberActionBusy(""); }
  }

  async function updateConsoleMember(member: TenantMember, role: TenantMember["role"], status: TenantMember["status"]) {
    if (!principal?.tenant_id) return;
    setMemberActionBusy(member.user_id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/members/${encodeURIComponent(member.user_id)}`, { method: "PUT", headers: { "Content-Type": "application/json", Accept: "application/json" }, credentials: "same-origin", body: JSON.stringify({ role, status }) });
      if (!response.ok) throw new Error("member update failed");
      await refreshConsoleMembers(false);
      setMembersMessage({ kind: "success", text: t("consoleMemberUpdateSuccess") });
    } catch { setMembersMessage({ kind: "error", text: t("consoleMemberFailed") }); } finally { setMemberActionBusy(""); }
  }

  async function removeConsoleMember(member: TenantMember) {
    if (!principal?.tenant_id) return;
    setMemberActionBusy(member.user_id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/members/${encodeURIComponent(member.user_id)}`, { method: "DELETE", headers: { Accept: "application/json" }, credentials: "same-origin" });
      if (!response.ok) throw new Error("member remove failed");
      await refreshConsoleMembers(false);
      setMembersMessage({ kind: "success", text: t("consoleMemberRemoveSuccess") });
    } catch { setMembersMessage({ kind: "error", text: t("consoleMemberFailed") }); } finally { setMemberActionBusy(""); }
  }

  async function addConsoleProjectMember(email: string, role: ProjectMember["role"]) {
    if (!principal?.tenant_id || !selectedProjectID) return;
    setProjectMemberActionBusy("new");
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/projects/${encodeURIComponent(selectedProjectID)}/members`, { method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, credentials: "same-origin", body: JSON.stringify({ email, role }) });
      if (!response.ok) throw new Error("project member add failed");
      await refreshConsoleProjectMembers(selectedProjectID, false);
      await refreshConsoleProjects(false);
      setProjectMembersMessage({ kind: "success", text: t("consoleProjectMemberSuccess") });
    } catch { setProjectMembersMessage({ kind: "error", text: t("consoleProjectMemberFailed") }); } finally { setProjectMemberActionBusy(""); }
  }

  async function updateConsoleProjectMember(member: ProjectMember, role: ProjectMember["role"]) {
    if (!principal?.tenant_id) return;
    setProjectMemberActionBusy(member.user_id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/projects/${encodeURIComponent(member.project_id)}/members/${encodeURIComponent(member.user_id)}`, { method: "PUT", headers: { "Content-Type": "application/json", Accept: "application/json" }, credentials: "same-origin", body: JSON.stringify({ role }) });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(result.error || "project member update failed");
      await refreshConsoleProjectMembers(member.project_id, false);
      setProjectMembersMessage({ kind: "success", text: t("consoleProjectMemberSuccess") });
    } catch (error) { setProjectMembersMessage({ kind: "error", text: error instanceof Error && error.message === "LAST_PROJECT_ADMIN_PROTECTED" ? t("consoleProjectLastAdmin") : t("consoleProjectMemberFailed") }); } finally { setProjectMemberActionBusy(""); }
  }

  async function removeConsoleProjectMember(member: ProjectMember) {
    if (!principal?.tenant_id) return;
    setProjectMemberActionBusy(member.user_id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/projects/${encodeURIComponent(member.project_id)}/members/${encodeURIComponent(member.user_id)}`, { method: "DELETE", headers: { Accept: "application/json" }, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(result.error || "project member remove failed");
      await refreshConsoleProjectMembers(member.project_id, false);
      await refreshConsoleProjects(false);
      setProjectMembersMessage({ kind: "success", text: t("consoleProjectMemberSuccess") });
    } catch (error) { setProjectMembersMessage({ kind: "error", text: error instanceof Error && error.message === "LAST_PROJECT_ADMIN_PROTECTED" ? t("consoleProjectLastAdmin") : t("consoleProjectMemberFailed") }); } finally { setProjectMemberActionBusy(""); }
  }

  async function refreshConsoleBilling() {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("billing:read")) return;
    setBillingBusy(true);
    setBillingMessage({ kind: "pending", text: t("consoleBillingLoading") });
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/billing/account`, {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as BillingAccount & { error?: string };
      if (!response.ok) throw new Error(result.error || "console billing unavailable");
      setBillingAccount(result);
      setBillingMessage({ kind: "", text: "" });
    } catch {
      setBillingAccount(null);
      setBillingMessage({ kind: "error", text: t("consoleBillingUnavailable") });
    } finally {
      setBillingBusy(false);
    }
  }

  async function refreshPaymentProviders() {
    try {
      const response = await fetch("/public/v1/payments/providers", { headers: { Accept: "application/json" } });
      const result = (await response.json().catch(() => ({}))) as { providers?: PublicPaymentProvider[] };
      if (!response.ok) throw new Error("payment providers unavailable");
      setPaymentProviders(result.providers || []);
    } catch {
      setPaymentProviders([]);
    }
  }

  async function refreshPaymentOrders(offset = paymentOrdersOffset) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("billing:read")) return;
    setPaymentOrdersBusy(true);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/billing/recharge?limit=20&offset=${Math.max(0, offset)}`, { headers: { Accept: "application/json" }, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as PaymentOrderList & { error?: string };
      if (!response.ok) throw new Error(result.error || "payment orders unavailable");
      setPaymentOrders(result);
      setPaymentOrdersOffset(result.offset || 0);
    } catch {
      setPaymentOrders({ orders: [], total: 0, limit: 20, offset: 0 });
    } finally {
      setPaymentOrdersBusy(false);
    }
  }

  async function createPaymentOrder(provider: PaymentOrder["provider"], amount: string, currency: string, packageID?: string) {
    if (!principal?.tenant_id) return;
    setPaymentBusy(true);
    setPaymentMessage({ kind: "pending", text: t("rechargeCreating") });
    try {
      const idempotencyKey = `recharge-${crypto.randomUUID()}`;
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/billing/recharge`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json", "Idempotency-Key": idempotencyKey },
        credentials: "same-origin",
        body: JSON.stringify({ provider, amount, currency, package_id: packageID || "", return_url: paymentReturnURL() }),
      });
		const result = (await response.json().catch(() => ({}))) as PaymentOrder & { error?: string; message?: string };
		if (!response.ok) throw new Error(resolvePaymentError(response.status, result.error, t, result.message));
      setPaymentOrder(result);
      setPaymentMessage({ kind: "success", text: t("rechargeCreated") });
      await refreshPaymentOrders(0);
      if (result.checkout_url && !result.checkout_client_secret) {
        window.location.assign(result.checkout_url);
      }
    } catch (error) {
      setPaymentMessage({ kind: "error", text: error instanceof Error ? error.message : t("rechargeFailed") });
    } finally {
      setPaymentBusy(false);
    }
  }

  async function loadPaymentOrder(orderID: string, cancelledByUser = false) {
    if (!principal?.tenant_id || !orderID) return;
    setPaymentBusy(true);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/billing/recharge/${encodeURIComponent(orderID)}`, { headers: { Accept: "application/json" }, credentials: "same-origin" });
		const result = (await response.json().catch(() => ({}))) as PaymentOrder & { error?: string; message?: string };
		if (!response.ok) throw new Error(resolvePaymentError(response.status, result.error, t, result.message));
      setPaymentOrder(result);
      if (result.status === "paid") {
        await refreshConsoleBilling();
        setPaymentMessage({ kind: "success", text: t("rechargePaid") });
      } else if (cancelledByUser) {
        setPaymentMessage({ kind: "pending", text: t("rechargeCancelled") });
      } else {
        setPaymentMessage({ kind: "pending", text: t("rechargeReturnPending") });
      }
    } catch (error) {
      setPaymentMessage({ kind: "error", text: error instanceof Error ? error.message : t("rechargeFailed") });
    } finally {
      setPaymentBusy(false);
    }
  }

  async function refreshPaymentOrder() {
    if (!principal?.tenant_id || !paymentOrder?.id) return;
    await loadPaymentOrder(paymentOrder.id);
  }

  async function capturePayPal() {
    if (!principal?.tenant_id || !paymentOrder?.id) return;
    setPaymentBusy(true);
    setPaymentMessage({ kind: "pending", text: t("rechargeConfirming") });
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/billing/recharge/${encodeURIComponent(paymentOrder.id)}/capture`, { method: "POST", headers: { Accept: "application/json" }, credentials: "same-origin" });
		const result = (await response.json().catch(() => ({}))) as PaymentOrder & { error?: string; message?: string };
		if (!response.ok) throw new Error(resolvePaymentError(response.status, result.error, t, result.message));
      setPaymentOrder(result);
      await refreshConsoleBilling();
      setPaymentMessage({ kind: "success", text: t("rechargePaid") });
    } catch (error) {
      setPaymentMessage({ kind: "error", text: error instanceof Error ? error.message : t("rechargeFailed") });
    } finally {
      setPaymentBusy(false);
    }
  }

  async function refreshEnterpriseVerification(showPending = false) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("enterprise:read")) return;
    setEnterpriseBusy(true);
    if (showPending) setEnterpriseMessage({ kind: "pending", text: t("enterpriseLoading") });
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/enterprise-verification`, { headers: { Accept: "application/json" }, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as EnterpriseVerification & { error?: string };
      if (!response.ok) throw new Error(result.error || "enterprise unavailable");
      setEnterpriseVerification(result.status ? result : null);
      setEnterpriseMessage({ kind: "", text: "" });
    } catch {
      setEnterpriseMessage({ kind: "error", text: t("enterpriseUnavailable") });
    } finally {
      setEnterpriseBusy(false);
    }
  }

  async function submitEnterpriseVerification(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!principal?.tenant_id) return;
    const form = new FormData(event.currentTarget);
    setEnterpriseBusy(true);
    setEnterpriseMessage({ kind: "pending", text: t("enterpriseSubmitting") });
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/enterprise-verification`, { method: "POST", body: form, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as EnterpriseVerification & { error?: string };
      if (!response.ok) throw new Error(resolveEnterpriseError(response.status, result.error, t));
      setEnterpriseVerification(result);
      setEnterpriseMessage({ kind: "success", text: t("enterpriseSubmitted") });
      event.currentTarget.reset();
    } catch (error) {
      setEnterpriseMessage({ kind: "error", text: error instanceof Error ? error.message : t("enterpriseUnavailable") });
    } finally {
      setEnterpriseBusy(false);
    }
  }

  async function refreshAdminEnterprise(showPending = false) {
    if (!signedIn || audience !== "admin" || !principal?.id || !principal.permissions?.includes("enterprise:read")) return;
    setAdminEnterpriseBusy(true);
    if (showPending) setAdminEnterpriseMessage({ kind: "pending", text: t("enterpriseLoading") });
    try {
      const query = adminEnterpriseStatus ? `?status=${encodeURIComponent(adminEnterpriseStatus)}` : "";
      const response = await fetch(`/admin/v1/enterprise-verifications${query}`, { headers: { Accept: "application/json" }, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as { submissions?: EnterpriseVerification[]; error?: string };
      if (!response.ok) throw new Error(result.error || "enterprise unavailable");
      setAdminEnterpriseItems(result.submissions || []);
      setAdminEnterpriseMessage({ kind: "", text: "" });
    } catch {
      setAdminEnterpriseMessage({ kind: "error", text: t("enterpriseUnavailable") });
    } finally {
      setAdminEnterpriseBusy(false);
    }
  }

  async function loadAdminEnterpriseDetails(item: EnterpriseVerification): Promise<EnterpriseVerification> {
    const response = await fetch(`/admin/v1/enterprise-verifications/${encodeURIComponent(item.id)}`, {
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
    const result = (await response.json().catch(() => ({}))) as EnterpriseVerification & { error?: string };
    if (!response.ok) throw new Error(result.error || "enterprise unavailable");
    return result;
  }

  async function reviewEnterprise(item: EnterpriseVerification, status: "approved" | "rejected", reason: string) {
    const response = await fetchAdminSensitive(`/admin/v1/enterprise-verifications/${encodeURIComponent(item.id)}/review`, { method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, credentials: "same-origin", body: JSON.stringify({ status, reason }) });
    const result = (await response.json().catch(() => ({}))) as EnterpriseVerification & { error?: string };
    if (!response.ok) throw new Error(resolveEnterpriseError(response.status, result.error, t));
    setAdminEnterpriseItems((current) => current.map((entry) => entry.id === result.id ? result : entry));
    setAdminEnterpriseMessage({ kind: "success", text: t("enterpriseReviewed") });
  }

	async function refreshConsoleDashboard(from = "", to = "") {
		if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("usage:read")) return;
		setConsoleDashboardBusy(true);
		try {
			const params = new URLSearchParams();
			if (from) params.set("from", from);
			if (to) params.set("to", to);
			const suffix = params.toString() ? `?${params.toString()}` : "";
			const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/dashboard${suffix}`, {
				headers: { Accept: "application/json" },
				credentials: "same-origin",
			});
			const result = (await response.json().catch(() => ({}))) as ConsoleDashboardReport & { error?: string };
			if (!response.ok) throw new Error(result.error || "dashboard unavailable");
			setConsoleDashboardReport(result);
			setConsoleDashboardMessage({ kind: "", text: "" });
		} catch {
			setConsoleDashboardMessage({ kind: "error", text: t("consoleDashboardUnavailable") });
		} finally {
			setConsoleDashboardBusy(false);
		}
	}

	async function refreshConsoleUsage(showPending = false, offset = consoleUsageOffset) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("usage:read")) return;
    setUsageBusy(true);
    if (showPending) setUsageMessage({ kind: "pending", text: t("consoleUsageLoading") });
    try {
      const params = new URLSearchParams({ limit: "50", offset: String(offset) });
      if (consoleUsageTokenName.trim()) params.set("token_name", consoleUsageTokenName.trim());
      if (consoleUsageModel.trim()) params.set("model", consoleUsageModel.trim());
      if (consoleUsageGroup) params.set("group_id", consoleUsageGroup);
      const fromBoundary = usageDateBoundary(consoleUsageFrom, false);
      const toBoundary = usageDateBoundary(consoleUsageTo, true);
      if (fromBoundary) params.set("from", fromBoundary);
      if (toBoundary) params.set("to", toBoundary);
		const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/usage?${params.toString()}`, {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
		const result = (await response.json().catch(() => ({}))) as UsageReport & { error?: string };
		if (!response.ok) throw new Error("usage unavailable");
		setConsoleUsageReport(result);
		setConsoleUsageOffset(offset);
		setUsageStatus({ tenant_id: principal.tenant_id, status: "ready" });
      setUsageMessage({ kind: "", text: "" });
    } catch {
		setUsageStatus(null);
		setConsoleUsageReport(null);
      setUsageMessage({ kind: "error", text: t("consoleUsageUnavailable") });
    } finally {
      setUsageBusy(false);
	}
  }

function usageDateBoundary(value: string, endOfDay: boolean) {
  const parts = value.split("-").map(Number);
  if (parts.length !== 3 || parts.some((part) => !Number.isInteger(part))) return "";
  const [year, month, day] = parts;
  const date = new Date(
    year,
    month - 1,
    day,
    endOfDay ? 23 : 0,
    endOfDay ? 59 : 0,
    endOfDay ? 59 : 0,
    endOfDay ? 999 : 0,
  );
  return Number.isNaN(date.getTime()) ? "" : date.toISOString();
}

  async function refreshConsoleModelStatus(showPending = false) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id || !hasConsolePermission("model:status:read")) return;
    setModelStatusBusy(true);
    if (showPending) setModelStatusMessage({ kind: "pending", text: t("consoleModelStatusLoading") });
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/model-status`, {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as ModelStatusReport & { error?: string };
      if (!response.ok) throw new Error(result.error || "model status unavailable");
      setModelStatusReport(result);
      setModelStatusMessage({ kind: "", text: "" });
    } catch {
      setModelStatusReport(null);
      setModelStatusMessage({ kind: "error", text: t("consoleModelStatusUnavailable") });
    } finally {
      setModelStatusBusy(false);
    }
  }

  async function refreshAdminModelStatus(showPending = false) {
    if (!signedIn || audience !== "admin") return;
    setAdminModelStatusBusy(true);
    if (showPending) setAdminModelStatusMessage({ kind: "pending", text: t("adminModelStatusLoading") });
    try {
      const response = await fetch("/admin/v1/model-status", {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as ModelStatusReport & { error?: string };
      if (!response.ok) throw new Error(result.error || "model status unavailable");
      setAdminModelStatusReport(result);
      setAdminModelStatusMessage({ kind: "", text: "" });
    } catch (error) {
      setAdminModelStatusReport(null);
      const code = error instanceof Error ? error.message : "";
      setAdminModelStatusMessage({ kind: "error", text: code === "PERMISSION_DENIED" ? t("adminModelMonitorPermissionDenied") : t("adminModelStatusUnavailable") });
    } finally {
      setAdminModelStatusBusy(false);
    }
  }

  async function refreshAdminModelMonitors(showPending = false) {
    if (!signedIn || audience !== "admin") return;
    setAdminModelMonitorsBusy(true);
    if (showPending) setAdminModelMonitorsMessage({ kind: "pending", text: t("adminModelMonitorLoading") });
    try {
      const response = await fetch("/admin/v1/model-monitors", {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { monitors?: ModelMonitor[]; error?: string };
      if (!response.ok) throw new Error(result.error || "model monitors unavailable");
      setAdminModelMonitors(result.monitors || []);
      setAdminModelMonitorsMessage({ kind: "", text: "" });
    } catch (error) {
      const code = error instanceof Error ? error.message : "";
      setAdminModelMonitorsMessage({ kind: "error", text: code === "PERMISSION_DENIED" ? t("adminModelMonitorPermissionDenied") : t("adminModelMonitorUnavailable") });
    } finally {
      setAdminModelMonitorsBusy(false);
    }
  }

  function openCreateAdminModelMonitor() {
    const firstGroup = groups.find((group) => group.status === "active") || groups[0];
    setAdminModelMonitorForm({
      ...defaultModelMonitorForm(),
      group_id: firstGroup?.id || "",
      name: firstGroup ? `${firstGroup.name} ${t("adminModelMonitorDefaultName")}` : "",
      // Primary model is optional; the server resolves the first current model.
      primary_model: "",
    });
    setAdminModelMonitorFormOpen(true);
    setAdminModelMonitorsMessage({ kind: "", text: "" });
  }

  function openEditAdminModelMonitor(item: ModelMonitor) {
    setAdminModelMonitorForm(modelMonitorFormFromItem(item));
    setAdminModelMonitorFormOpen(true);
    setAdminModelMonitorsMessage({ kind: "", text: "" });
  }

  function closeAdminModelMonitorForm() {
    setAdminModelMonitorFormOpen(false);
    setAdminModelMonitorForm(defaultModelMonitorForm());
  }

  async function saveAdminModelMonitor(form: ModelMonitorFormState) {
    if (!form.group_id || !form.name.trim()) {
      setAdminModelMonitorsMessage({ kind: "error", text: t("adminModelMonitorValidation") });
      return false;
    }
    if (form.selection_mode === "selected" && form.model_names.length === 0) {
      setAdminModelMonitorsMessage({ kind: "error", text: t("adminModelMonitorSelectModel") });
      return false;
    }
    setAdminModelMonitorActionBusy(form.id ? `update:${form.id}` : "create");
    setAdminModelMonitorsMessage({ kind: "pending", text: t("adminModelMonitorSaving") });
    try {
      const response = await fetchAdminSensitive(
        form.id ? `/admin/v1/model-monitors/${encodeURIComponent(form.id)}` : "/admin/v1/model-monitors",
        {
          method: form.id ? "PUT" : "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          credentials: "same-origin",
          body: JSON.stringify({
            group_id: form.group_id,
            name: form.name.trim(),
            selection_mode: form.selection_mode,
            model_names: form.selection_mode === "all" ? [] : form.model_names,
            primary_model: form.primary_model.trim(),
            mode: form.mode,
            probe_interval_seconds: Math.max(60, Number(form.probe_interval_seconds) || 300),
            recent_request_limit: [30, 60, 120].includes(Number(form.recent_request_limit)) ? Number(form.recent_request_limit) : 60,
            enabled: form.enabled,
          }),
        },
      );
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(result.error || "model monitor save failed");
      await refreshAdminModelMonitors(false);
      await refreshAdminModelStatus(false);
      closeAdminModelMonitorForm();
      setAdminModelMonitorsMessage({ kind: "success", text: t("adminModelMonitorSaved") });
      return true;
    } catch (error) {
      const code = error instanceof Error ? error.message : "";
      setAdminModelMonitorsMessage({
        kind: "error",
        text: code === "MODEL_MONITOR_GROUP_ALREADY_CONFIGURED"
          ? t("adminModelMonitorGroupAlreadyConfigured")
          : code === "MODEL_MONITOR_GROUP_NOT_FOUND"
          ? t("adminModelMonitorGroupNotFound")
          : code === "MODEL_MONITOR_GROUP_INACTIVE"
          ? t("adminModelMonitorGroupInactive")
          : code === "MODEL_MONITORS_UNAVAILABLE"
          ? t("adminModelMonitorUnavailable")
          : code === "PERMISSION_DENIED"
          ? t("adminModelMonitorPermissionDenied")
          : t("adminModelMonitorValidation"),
      });
      return false;
    } finally {
      setAdminModelMonitorActionBusy("");
    }
  }

  async function deleteAdminModelMonitor(item: ModelMonitor) {
    if (!window.confirm(t("adminModelMonitorDeleteConfirm"))) return;
    setAdminModelMonitorActionBusy(`delete:${item.id}`);
    setAdminModelMonitorsMessage({ kind: "pending", text: t("adminModelMonitorDeleting") });
    try {
      const response = await fetchAdminSensitive(`/admin/v1/model-monitors/${encodeURIComponent(item.id)}`, {
        method: "DELETE",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(result.error || "model monitor delete failed");
      await refreshAdminModelMonitors(false);
      await refreshAdminModelStatus(false);
      setAdminModelMonitorsMessage({ kind: "success", text: t("adminModelMonitorDeleted") });
    } catch {
      setAdminModelMonitorsMessage({ kind: "error", text: t("adminModelMonitorUnavailable") });
    } finally {
      setAdminModelMonitorActionBusy("");
    }
  }

  async function probeAdminModelMonitor(item: ModelMonitor) {
    setAdminModelMonitorActionBusy(`probe:${item.id}`);
    setAdminModelMonitorsMessage({ kind: "pending", text: t("adminModelMonitorProbing") });
    try {
      const response = await fetchAdminSensitive(`/admin/v1/model-monitors/${encodeURIComponent(item.id)}/probe`, {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(result.error || "model monitor probe failed");
      await refreshAdminModelMonitors(false);
      await refreshAdminModelStatus(false);
      setAdminModelMonitorsMessage({ kind: "success", text: t("adminModelMonitorProbeCompleted") });
    } catch (error) {
      const code = error instanceof Error ? error.message : "";
      setAdminModelMonitorsMessage({
        kind: "error",
        text: code === "MODEL_MONITOR_ACTIVE_PROBE_REQUIRED"
          ? t("adminModelMonitorActiveRequired")
          : code === "MODEL_MONITOR_BUSY"
          ? t("adminModelMonitorBusy")
          : code === "MODEL_MONITOR_GROUP_INACTIVE"
          ? t("adminModelMonitorGroupInactive")
          : t("adminModelMonitorProbeFailed"),
      });
    } finally {
      setAdminModelMonitorActionBusy("");
    }
  }

  async function refreshModelCatalog(showPending = false) {
    setModelCatalogBusy(true);
    if (showPending) setModelCatalogMessage({ kind: "pending", text: t("modelsLoading") });
    try {
      const response = await fetch("/public/v1/models", {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { models?: PublicModelSummary[] };
      if (!response.ok) throw new Error("model catalog unavailable");
      setModelCatalog(result.models || []);
      setModelCatalogMessage({ kind: "", text: "" });
    } catch {
      setModelCatalogMessage({ kind: "error", text: t("modelsUnavailable") });
    } finally {
      setModelCatalogBusy(false);
    }
  }

  async function refreshUsers(showPending = false) {
    if (!signedIn || audience !== "admin") return;
    setUsersBusy(true);
    if (showPending) setUsersMessage({ kind: "pending", text: t("usersLoading") });
    try {
      const response = await fetch("/admin/v1/users", { headers: { Accept: "application/json" }, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as { users?: UserSummary[] };
      if (!response.ok) throw new Error("users unavailable");
      setUsers(result.users || []);
      setUsersMessage({ kind: "", text: "" });
    } catch {
      setUsersMessage({ kind: "error", text: t("usersUnavailable") });
    } finally {
      setUsersBusy(false);
    }
  }

  async function refreshPlatformRoles(showPending = false) {
    if (!signedIn || audience !== "admin") return;
    setPlatformRolesBusy(true);
    if (showPending) setPlatformRolesMessage({ kind: "pending", text: t("rolesLoading") });
    try {
      const [rolesResponse, permissionsResponse] = await Promise.all([
        fetch("/admin/v1/roles", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
        fetch("/admin/v1/permissions", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
      ]);
      const rolesResult = (await rolesResponse.json().catch(() => ({}))) as { roles?: PlatformRole[]; error?: string };
      const permissionsResult = (await permissionsResponse.json().catch(() => ({}))) as { permissions?: PlatformPermission[]; error?: string };
      if (!rolesResponse.ok || !permissionsResponse.ok) throw new Error(rolesResult.error || permissionsResult.error || "roles unavailable");
      setPlatformRoles(rolesResult.roles || []);
      setPlatformPermissions(permissionsResult.permissions || []);
      setPlatformRolesMessage({ kind: "", text: "" });
    } catch {
      setPlatformRolesMessage({ kind: "error", text: t("rolesUnavailable") });
    } finally {
      setPlatformRolesBusy(false);
    }
  }

  async function savePlatformRole(form: PlatformRoleFormState): Promise<boolean> {
    setPlatformRolesBusy(true);
    setPlatformRolesMessage({ kind: "pending", text: t("rolesSaving") });
    try {
      const response = await fetchAdminSensitive(form.id ? `/admin/v1/roles/${encodeURIComponent(form.id)}` : "/admin/v1/roles", {
        method: form.id ? "PUT" : "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ code: form.code.trim(), name: form.name.trim(), status: form.status, permissions: form.permissions }),
      });
      const result = (await response.json().catch(() => ({}))) as PlatformRole & { error?: string };
      if (!response.ok) throw new Error(result.error || "role save failed");
      await refreshPlatformRoles(false);
      setPlatformRolesMessage({ kind: "success", text: form.id ? t("rolesUpdated") : t("rolesCreated") });
      return true;
    } catch (error) {
      const code = error instanceof Error ? error.message : "";
      setPlatformRolesMessage({ kind: "error", text: code === "PLATFORM_ROLE_EXISTS" ? t("rolesExists") : code === "PLATFORM_OWNER_ROLE_PROTECTED" ? t("rolesProtected") : t("rolesSaveFailed") });
      return false;
    } finally {
      setPlatformRolesBusy(false);
    }
  }

  async function disablePlatformRole(role: PlatformRole) {
    setPlatformRolesBusy(true);
    setPlatformRolesMessage({ kind: "pending", text: t("rolesSaving") });
    try {
      const response = await fetchAdminSensitive(`/admin/v1/roles/${encodeURIComponent(role.id)}/disable`, {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(result.error || "role disable failed");
      await refreshPlatformRoles(false);
      setPlatformRolesMessage({ kind: "success", text: t("rolesDisabledSuccess") });
    } catch (error) {
      const code = error instanceof Error ? error.message : "";
      setPlatformRolesMessage({ kind: "error", text: code === "LAST_PLATFORM_ADMIN_PROTECTED" ? t("usersLastAdminProtected") : code === "PLATFORM_OWNER_ROLE_PROTECTED" ? t("rolesProtected") : t("rolesSaveFailed") });
    } finally {
      setPlatformRolesBusy(false);
    }
  }

  async function loadPlatformUserRoles(userID: string) {
    const response = await fetch(`/admin/v1/users/${encodeURIComponent(userID)}/roles`, { headers: { Accept: "application/json" }, credentials: "same-origin" });
    const result = (await response.json().catch(() => ({}))) as { roles?: PlatformRole[]; error?: string };
    if (!response.ok) throw new Error(result.error || "roles unavailable");
    return result.roles || [];
  }

  async function savePlatformUserRoles(user: UserSummary, roleIDs: string[]): Promise<boolean> {
    setPlatformRolesBusy(true);
    setPlatformRolesMessage({ kind: "pending", text: t("rolesSaving") });
    try {
      const response = await fetchAdminSensitive(`/admin/v1/users/${encodeURIComponent(user.id)}/roles`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ role_ids: roleIDs }),
      });
      const result = (await response.json().catch(() => ({}))) as { roles?: PlatformRole[]; error?: string };
      if (!response.ok) throw new Error(result.error || "role binding failed");
      setUsers((current) => current.map((item) => item.id === user.id ? { ...item, platform_roles: (result.roles || []).map((role) => role.code) } : item));
      await refreshPlatformRoles(false);
      setPlatformRolesMessage({ kind: "success", text: t("rolesBindingSaved") });
      return true;
    } catch (error) {
      const code = error instanceof Error ? error.message : "";
      setPlatformRolesMessage({ kind: "error", text: code === "LAST_PLATFORM_ADMIN_PROTECTED" ? t("usersLastAdminProtected") : code === "PLATFORM_ADMIN_MFA_REQUIRED" ? t("rolesAdminMFARequired") : t("rolesBindingFailed") });
      return false;
    } finally {
      setPlatformRolesBusy(false);
    }
  }

  async function updateUserStatus(user: UserSummary, status: "active" | "locked" | "disabled") {
    if (user.id === principal?.id || userActionBusy) return;
    setUserActionBusy(user.id);
    setUsersMessage({ kind: "pending", text: t("usersUpdating") });
    try {
      const response = await fetchAdminSensitive(`/admin/v1/users/${encodeURIComponent(user.id)}/status`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ status }),
      });
      const result = (await response.json().catch(() => ({}))) as UserSummary & { error?: string };
      if (!response.ok) throw new Error(result.error || "user status update failed");
      setUsers((current) => current.map((item) => item.id === user.id ? result : item));
      setUsersMessage({ kind: "success", text: t("usersUpdateSuccess") });
    } catch (error) {
      setUsersMessage({ kind: "error", text: error instanceof Error && error.message === "LAST_PLATFORM_ADMIN_PROTECTED" ? t("usersLastAdminProtected") : error instanceof Error && error.message === "PLATFORM_OWNER_PROTECTED" ? t("usersPlatformOwnerProtected") : error instanceof Error && error.message === "USER_EMAIL_NOT_VERIFIED" ? t("usersEmailVerificationRequired") : t("usersUpdateFailed") });
    } finally {
      setUserActionBusy("");
    }
  }

  function openEditUser(user: UserSummary) {
    if (user.id === principal?.id) return;
    setUserForm({
      id: user.id,
      email: user.email,
      display_name: user.display_name,
      password: "",
    });
    setUserFormMessage({ kind: "", text: "" });
    setUserFormOpen(true);
  }

  function closeUserForm() {
    setUserFormOpen(false);
    setUserForm(defaultUserAdminForm());
    setUserFormMessage({ kind: "", text: "" });
  }

  async function handleUserSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const emailValue = userForm.email.trim();
    const displayName = userForm.display_name.trim();
    // Passwords are opaque credentials; whitespace at either edge is valid
    // input and must not be silently removed before hashing.
    const passwordValue = userForm.password;
    if (!emailValue || !displayName || (passwordValue && passwordValue.length < 12)) {
      setUserFormMessage({ kind: "error", text: t("usersFormValidation") });
      return;
    }
    setUserFormBusy(true);
    setUserFormMessage({ kind: "pending", text: t("usersSaving") });
    try {
      const payload: Record<string, string> = {
        email: emailValue,
        display_name: displayName,
        password: passwordValue,
      };
      const response = await fetchAdminSensitive(`/admin/v1/users/${encodeURIComponent(userForm.id)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(payload),
      });
      const result = (await response.json().catch(() => ({}))) as UserSummary & { error?: string };
      if (!response.ok) {
        if (result.error === "EMAIL_ALREADY_EXISTS") throw new Error(t("usersEmailExists"));
        if (result.error === "PLATFORM_OWNER_PROTECTED") throw new Error(t("usersPlatformOwnerProtected"));
        if (result.error === "USER_EMAIL_NOT_VERIFIED") throw new Error(t("usersEmailVerificationRequired"));
        if (result.error === "TENANT_NOT_FOUND") throw new Error(t("usersTenantUnavailable"));
        throw new Error(t("usersSaveFailed"));
      }
      setUsers((current) => current.map((item) => item.id === result.id ? result : item));
      setUsersMessage({ kind: "success", text: t("usersEditSuccess") });
      closeUserForm();
      } catch (error) {
      setUserFormMessage({ kind: "error", text: error instanceof Error ? error.message : t("usersSaveFailed") });
    } finally {
      setUserFormBusy(false);
    }
  }

  function handleRegistered(registeredEmail: string, registeredTenantID: string, verificationRequired: boolean) {
    setEmail(registeredEmail);
    setTenantId(registeredTenantID);
    setPassword("");
    setMfaCode("");
    setAudience("console");
    setLoginMessage({ kind: "success", text: verificationRequired ? t("registerVerificationSent") : t("registerSuccessGoLogin") });
    window.location.hash = "#login";
  }

  function openCreateToken() {
    if (!hasConsolePermission("token:create")) return;
    const projectID = projects.find((project) => project.status === "active")?.id || principal?.project_ids?.[0] || "";
    const availableGroups: TokenGroupOption[] = consoleTokenGroups;
    const defaultGroup = availableGroups.find((group) => group.code === "default") || availableGroups.find((group) => group.status === "active");
    setTokenCreateMode("console");
    setTokenCreateForm({
      ...defaultTokenCreateForm(),
      tenant_id: principal?.tenant_id || "",
      project_id: projectID,
      group_id: defaultGroup?.id || "",
    });
    setTokenEditingID("");
    setTokenCreateMessage({ kind: "", text: "" });
    setIssuedToken(null);
    setTokenCreateOpen(true);
  }

  function openEditToken(token: TokenSummary) {
    if (!hasConsolePermission("token:update") || token.status === "revoked" || token.status === "expired") return;
    setTokenEditingID(token.id);
    setTokenCreateMode("console");
    setTokenCreateForm({
      tenant_id: token.tenant_id,
      project_id: token.project_id,
      name: token.name,
      expires_at: token.expires_at ? toDateTimeLocal(token.expires_at) : "",
      group_id: token.group_id || "",
      allowed_ips: (token.allowed_ips || []).join("\n"),
      allowed_domains: (token.allowed_domains || []).join("\n"),
      spend_limit: token.spend_limit || "0",
    });
    setTokenCreateMessage({ kind: "", text: "" });
    setIssuedToken(null);
    setTokenCreateOpen(true);
  }

  function closeTokenCreate() {
    if (tokenCreateBusy) return;
    setTokenCreateOpen(false);
    setTokenEditingID("");
    setTokenCreateForm(defaultTokenCreateForm());
    setTokenCreateMessage({ kind: "", text: "" });
    setIssuedToken(null);
  }

  async function handleTokenCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = tokenCreateForm.name.trim();
    const tenantID = tokenCreateForm.tenant_id.trim();
    const projectID = tokenCreateForm.project_id.trim();
    const groupID = tokenCreateForm.group_id.trim();
    const editingToken = tokenEditingID ? tokens.find((token) => token.id === tokenEditingID) : undefined;
    if (!tenantID) {
      setTokenCreateMessage({ kind: "error", text: t("tokensUnavailable") });
      return;
    }
    if (!projectID) {
      setTokenCreateMessage({ kind: "error", text: t("tokensProjectRequired") });
      return;
    }
    if (!groupID) {
      setTokenCreateMessage({ kind: "error", text: t("tokensGroupRequired") });
      return;
    }
    if (editingToken && !name) {
      setTokenCreateMessage({ kind: "error", text: t("tokensNameRequired") });
      return;
    }
    setTokenCreateBusy(true);
    setTokenCreateMessage({ kind: "pending", text: editingToken ? t("tokensSaving") : t("tokensCreating") });
    try {
      const payload: Record<string, unknown> = {
        name,
        project_id: projectID,
        group_id: groupID,
        allowed_models: editingToken?.allowed_models || [],
        rate_limit: editingToken?.rate_limit || {},
        allowed_ips: parseTokenList(tokenCreateForm.allowed_ips),
        allowed_domains: parseTokenList(tokenCreateForm.allowed_domains),
        spend_limit: tokenCreateForm.spend_limit.trim() || "0",
      };
      if (tokenCreateForm.expires_at) {
        payload.expires_at = new Date(tokenCreateForm.expires_at).toISOString();
      }
      const baseEndpoint = `/console/v1/tenants/${encodeURIComponent(principal?.tenant_id || "")}/tokens`;
      const endpoint = editingToken ? `${baseEndpoint}/${encodeURIComponent(editingToken.id)}` : baseEndpoint;
      const response = await fetch(endpoint, {
        method: editingToken ? "PUT" : "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(payload),
      });
      const result = (await response.json().catch(() => ({}))) as (IssuedTokenResponse | TokenSummary) & { error?: string };
      if (!response.ok) throw new Error(result.error || "token creation failed");
      if (editingToken) {
        setTokenCreateOpen(false);
        setTokenEditingID("");
        setTokenCreateForm(defaultTokenCreateForm());
        setTokenCreateMessage({ kind: "", text: "" });
        await refreshConsoleTokens(false);
        setTokensMessage({ kind: "success", text: t("tokensSaveSuccess") });
      } else {
        const issued = result as IssuedTokenResponse;
        setIssuedToken(issued);
        setConsoleTokenSecrets((current) => ({ ...current, [issued.id]: issued.token }));
        setTokenCreateMessage({ kind: "success", text: t("tokensCreateSuccess") });
        await refreshConsoleTokens(false);
      }
    } catch (error) {
      const code = error instanceof Error ? error.message : "";
      const message = code === "INVALID_TOKEN_SPEND_LIMIT"
        ? t("tokensSpendLimitInvalid")
        : editingToken
          ? t("tokensSaveFailed")
          : t("tokensCreateValidation");
      setTokenCreateMessage({ kind: "error", text: message });
    } finally {
      setTokenCreateBusy(false);
    }
  }

  async function pauseConsoleToken(token: TokenSummary) {
    if (!principal?.tenant_id) return;
    if (token.status !== "active") return;
    setTokenActionBusy(token.id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/tokens/${encodeURIComponent(token.id)}/pause`, {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) throw new Error("token pause failed");
      await refreshConsoleTokens(false);
      setTokensMessage({ kind: "success", text: t("tokensPauseSuccess") });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensUnavailable") });
    } finally {
      setTokenActionBusy("");
    }
  }

  async function resumeConsoleToken(token: TokenSummary) {
    if (!principal?.tenant_id) return;
    if (token.status !== "disabled") return;
    setTokenActionBusy(token.id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/tokens/${encodeURIComponent(token.id)}/resume`, {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) throw new Error("token resume failed");
      await refreshConsoleTokens(false);
      setTokensMessage({ kind: "success", text: t("tokensResumeSuccess") });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensUnavailable") });
    } finally {
      setTokenActionBusy("");
    }
  }

  async function terminateConsoleToken(token: TokenSummary) {
    if (!principal?.tenant_id || (token.status !== "active" && token.status !== "disabled")) return;
    if (!window.confirm(t("tokensTerminateConfirm"))) return;
    setTokenActionBusy(token.id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/tokens/${encodeURIComponent(token.id)}/terminate`, {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) throw new Error("token terminate failed");
      await refreshConsoleTokens(false);
      setTokensMessage({ kind: "success", text: t("tokensTerminateSuccess") });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensUnavailable") });
    } finally {
      setTokenActionBusy("");
    }
  }

  async function deleteConsoleToken(token: TokenSummary) {
    if (!principal?.tenant_id || !window.confirm(t("tokensDeleteConfirm"))) return;
    setTokenActionBusy(token.id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/tokens/${encodeURIComponent(token.id)}`, {
        method: "DELETE",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) throw new Error("token delete failed");
      setConsoleTokenSecrets((current) => {
        const next = { ...current };
        delete next[token.id];
        return next;
      });
      await refreshConsoleTokens(false);
      setTokensMessage({ kind: "success", text: t("tokensDeleteSuccess") });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensUnavailable") });
    } finally {
      setTokenActionBusy("");
    }
  }

  async function copyConsoleToken(token: TokenSummary) {
    const secret = consoleTokenSecrets[token.id];
    if (!secret) {
      setTokensMessage({ kind: "error", text: t("tokensSecretUnavailable") });
      return;
    }
    try {
      await navigator.clipboard.writeText(secret);
      setTokensMessage({ kind: "success", text: t("tokensSecretCopied") });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensSecretCopyFailed") });
    }
  }

  function tokenSecretAvailable(token: TokenSummary) {
    return Boolean(consoleTokenSecrets[token.id]);
  }

  async function pauseAdminToken(token: TokenSummary) {
    if (token.status !== "active") return;
    setTokenActionBusy(token.id);
    try {
      const response = await fetchAdminSensitive(`/admin/v1/tokens/${encodeURIComponent(token.id)}/pause`, {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) throw new Error("token pause failed");
      await refreshTokens(false);
      setTokensMessage({ kind: "success", text: t("tokensPauseSuccess") });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensUnavailable") });
    } finally {
      setTokenActionBusy("");
    }
  }

  async function refreshBilling() {
    if (!signedIn || audience !== "admin") return;
    setBillingBusy(true);
    setBillingMessage({ kind: "pending", text: t("billingLoad") });
    try {
      const response = await fetch("/admin/v1/prices", {
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as {
        prices?: PriceMatrixSummary[];
      };
      if (!response.ok) {
        throw new Error("billing unavailable");
      }
      setPrices(result.prices || []);
      setBillingMessage({ kind: "", text: "" });
    } catch {
      setBillingMessage({ kind: "error", text: t("billingLoadFailed") });
    } finally {
      setBillingBusy(false);
    }
  }

  async function loadBillingAccount() {
    const tenant = creditForm.tenant_id.trim();
    if (!tenant) {
      setBillingMessage({ kind: "error", text: t("billingInvalid") });
      return;
    }
    setBillingBusy(true);
    setBillingMessage({ kind: "pending", text: t("billingAccountLoading") });
    try {
      const query = creditForm.currency.trim()
        ? `?currency=${encodeURIComponent(creditForm.currency.trim().toUpperCase())}`
        : "";
      const response = await fetch(
        `/admin/v1/tenants/${encodeURIComponent(tenant)}/billing/account${query}`,
        {
          headers: { Accept: "application/json" },
          credentials: "same-origin",
        }
      );
      const result = (await response.json().catch(() => ({}))) as BillingAccount & {
        error?: string;
      };
      if (!response.ok) {
        throw new Error(result.error || "account unavailable");
      }
      setBillingAccount(result);
      setBillingMessage({ kind: "success", text: "" });
    } catch {
      setBillingAccount(null);
      setBillingMessage({ kind: "error", text: t("billingAccountNotFound") });
    } finally {
      setBillingBusy(false);
    }
  }

  async function syncOfficialPrices() {
    if (!signedIn || audience !== "admin") return;
    setOfficialPriceSyncBusy(true);
    setBillingMessage({ kind: "pending", text: t("billingOfficialSyncing") });
    try {
      const response = await fetchAdminSensitive("/admin/v1/prices/sync-official", {
        method: "POST",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      const result = (await response.json().catch(() => ({}))) as {
        models_matched?: number;
        models_updated?: number;
        models_unchanged?: number;
        unmatched?: string[];
      };
      if (!response.ok) throw new Error("official price sync failed");
      await refreshBilling();
      await refreshModelCatalog(false);
      setBillingMessage({
        kind: "success",
        text: `${t("billingOfficialSyncSuccess")} ${result.models_matched || 0} ${t("billingOfficialMatched")}，${result.models_updated || 0} ${t("billingOfficialUpdated")}，${result.unmatched?.length || 0} ${t("billingOfficialUnmatched")}。`,
      });
    } catch {
      setBillingMessage({ kind: "error", text: t("billingOfficialSyncFailed") });
    } finally {
      setOfficialPriceSyncBusy(false);
    }
  }

  function openEditPrice(price: PriceMatrixSummary) {
    setModelPriceForm({
      model_id: price.model_id,
      provider: price.provider,
      model: price.model,
      currency: price.currency || "USD",
      source: price.source,
      input_price_per_million_tokens: price.input_price_per_million_tokens,
      output_price_per_million_tokens: price.output_price_per_million_tokens,
      cached_input_price_per_million_tokens: price.cached_input_price_per_million_tokens,
      cache_creation_price_per_million_tokens: tokenPriceToDisplay((price.components || []).find((component) => component.component_code === "cache_creation_tokens")?.price_per_unit || "", "token"),
      reasoning_price_per_million_tokens: price.reasoning_price_per_million_tokens,
      components: (price.components || []).map((component) => ({
        component_code: component.component_code,
        unit: component.unit,
        price_per_unit: isTokenPriceUnit(component.unit) ? tokenPriceToDisplay(component.price_per_unit, component.unit) : component.price_per_unit,
        tiers: component.tiers,
        metadata: component.metadata,
      })),
    });
    setModelPriceFormMessage({ kind: "", text: "" });
    setModelPriceFormOpen(true);
  }

  function closeModelPriceForm() {
    if (modelPriceFormBusy) return;
    setModelPriceFormOpen(false);
    setModelPriceForm(defaultModelPriceForm());
    setModelPriceFormMessage({ kind: "", text: "" });
  }

  async function handleModelPriceSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const input = millionPriceToUnit(modelPriceForm.input_price_per_million_tokens);
    const output = millionPriceToUnit(modelPriceForm.output_price_per_million_tokens);
    const cached = modelPriceForm.cached_input_price_per_million_tokens.trim() ? millionPriceToUnit(modelPriceForm.cached_input_price_per_million_tokens) : "0";
    const cacheCreation = modelPriceForm.cache_creation_price_per_million_tokens.trim() ? millionPriceToUnit(modelPriceForm.cache_creation_price_per_million_tokens) : "0";
    const reasoning = modelPriceForm.reasoning_price_per_million_tokens.trim() ? millionPriceToUnit(modelPriceForm.reasoning_price_per_million_tokens) : "0";
    const components = modelPriceForm.components.map((component) => ({
      component_code: component.component_code,
      unit: component.unit,
      price_per_unit: isTokenPriceUnit(component.unit) ? tokenDisplayToPrice(component.price_per_unit, component.unit) : component.price_per_unit.trim(),
      tiers: component.tiers,
      metadata: component.metadata,
    })).filter((component) => component.component_code && component.price_per_unit && component.component_code !== "cache_creation_tokens");
    if (cacheCreation !== "0") components.push({ component_code: "cache_creation_tokens", unit: "token", price_per_unit: cacheCreation, tiers: undefined, metadata: undefined });
    if (!modelPriceForm.model_id || modelPriceForm.currency.trim().toUpperCase() !== "USD" || ![input, output, cached, reasoning, ...components.map((component) => component.price_per_unit)].some((value) => value && value !== "0")) {
      setModelPriceFormMessage({ kind: "error", text: t("billingPriceFormInvalid") });
      return;
    }
    setModelPriceFormBusy(true);
    setModelPriceFormMessage({ kind: "pending", text: t("billingPriceSaving") });
    try {
      const response = await fetchAdminSensitive("/admin/v1/prices/publish", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({
          scope_type: "platform_default",
          model_id: modelPriceForm.model_id,
          currency: "USD",
          input_price_per_unit: input,
          output_price_per_unit: output,
          cached_input_price_per_unit: cached,
          cache_creation_price_per_unit: cacheCreation,
          reasoning_price_per_unit: reasoning,
          minimum_charge: "0",
          components: components.length > 0 ? components : undefined,
        }),
      });
      const result = (await response.json().catch(() => ({}))) as { error?: string };
      if (!response.ok) throw new Error(result.error || "price save failed");
      closeModelPriceForm();
      await refreshBilling();
      setBillingMessage({ kind: "success", text: t("billingPriceSaveSuccess") });
      await refreshModelCatalog(false);
    } catch {
      setModelPriceFormMessage({ kind: "error", text: t("billingPriceSaveFailed") });
    } finally {
      setModelPriceFormBusy(false);
    }
  }

  async function creditBillingAccount(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const tenant = creditForm.tenant_id.trim();
    if (!tenant || !creditForm.amount.trim() || Number(creditForm.amount) <= 0) {
      setBillingMessage({ kind: "error", text: t("billingInvalid") });
      return;
    }
    setBillingBusy(true);
    setBillingMessage({ kind: "pending", text: t("billingCrediting") });
    try {
      const idempotencyKey = `admin-credit-${crypto.randomUUID()}`;
      const response = await fetchAdminSensitive(
        `/admin/v1/tenants/${encodeURIComponent(tenant)}/billing/credit`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Accept: "application/json",
            "Idempotency-Key": idempotencyKey,
          },
          credentials: "same-origin",
          body: JSON.stringify({
            currency: creditForm.currency.trim().toUpperCase(),
            amount: creditForm.amount.trim(),
            reason: creditForm.reason.trim(),
            direction: creditForm.direction || "credit",
            idempotency_key: idempotencyKey,
          }),
        }
      );
      const result = (await response.json().catch(() => ({}))) as BillingAccount & {
        error?: string;
      };
      if (!response.ok) {
        throw new Error(result.error || "credit failed");
      }
      setBillingAccount(result);
      setBillingMessage({ kind: "success", text: creditForm.direction === "debit" ? t("accountFinanceDebitSuccess") : t("billingCreditSuccess") });
      setCreditForm((current) => ({ ...current, amount: "", reason: "" }));
    } catch {
      setBillingMessage({ kind: "error", text: t("billingInvalid") });
    } finally {
      setBillingBusy(false);
    }
  }

  function openCreateChannel(provider: "openai" | "anthropic" | "grok" | "gemini" | "volcengine" = "openai") {
    setChannelForm(defaultChannelForm(provider));
    setChannelFormOpen(true);
    setChannelDeleteConfirm("");
    setDiscoveredModels([]);
    setModelDiscoveryMessage({ kind: "", text: "" });
    setChannelsMessage({ kind: "", text: "" });
  }

  function openEditChannel(channel: ChannelSummary) {
    setChannelForm(channelFormFromSummary(channel));
    setChannelFormOpen(true);
    setChannelDeleteConfirm("");
    setDiscoveredModels([]);
    setModelDiscoveryMessage({ kind: "", text: "" });
    setChannelsMessage({ kind: "", text: "" });
  }

  function closeChannelForm() {
    setChannelFormOpen(false);
    setChannelForm(defaultChannelForm());
    setDiscoveredModels([]);
    setModelDiscoveryMessage({ kind: "", text: "" });
  }

  function setChannelProvider(provider: "openai" | "anthropic" | "grok" | "gemini" | "volcengine") {
    setDiscoveredModels([]);
    setModelDiscoveryMessage({ kind: "", text: "" });
    setChannelForm((current) => {
      const currentDefault = defaultProviderBaseURL(current.provider);
      return {
        ...current,
        provider,
        base_url:
          !current.base_url || current.base_url === currentDefault
            ? defaultProviderBaseURL(provider)
            : current.base_url,
        // A model mapping belongs to one provider. Clear it when the
        // provider changes instead of carrying an OpenAI model into an
        // Anthropic, Gemini, Grok, or Volcano channel.
        models: current.provider === provider ? current.models : [],
      };
    });
  }

  function updateChannelModel(rowID: string, patch: Partial<Omit<ChannelFormModel, "id">>) {
    setChannelForm((current) => ({
      ...current,
      models: current.models.map((model) =>
        model.id === rowID ? { ...model, ...patch } : model
      ),
    }));
  }

  function addChannelModel() {
    setChannelForm((current) => ({
      ...current,
      models: [
        ...current.models,
        { id: newFormRowID(), model: "", upstream_model: "", enabled: true },
      ],
    }));
  }

  function removeChannelModel(rowID: string) {
    setChannelForm((current) => ({
      ...current,
      models: current.models.filter((model) => model.id !== rowID),
    }));
  }

  function isChannelModelMapped(modelID: string) {
    return channelForm.models.some(
      (model) => model.model.trim().toLowerCase() === modelID.trim().toLowerCase()
    );
  }

  function addDiscoveredModel(model: DiscoveredModel) {
    if (isChannelModelMapped(model.id)) return;
    setChannelForm((current) => ({
      ...current,
      models: [
        ...current.models,
        {
          id: newFormRowID(),
          model: model.id,
          upstream_model: model.id,
          enabled: true,
        },
      ],
    }));
  }

  async function discoverChannelModels() {
    if (!channelForm.base_url.trim() || (!channelForm.id && !channelForm.api_key.trim())) {
      setModelDiscoveryMessage({ kind: "error", text: t("channelsFormDiscoverRequiresKey") });
      return;
    }
    setModelDiscoveryBusy(true);
    setModelDiscoveryMessage({ kind: "pending", text: t("channelsFormDiscovering") });
    try {
      const response = await fetchAdminSensitive("/admin/v1/channels/discover-models", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({
          channel_id: channelForm.id || undefined,
          provider: channelForm.provider,
          base_url: channelForm.base_url.trim(),
          api_key: channelForm.api_key.trim(),
        }),
      });
      if (!response.ok) {
        const result = (await response.json().catch(() => ({}))) as { error?: string };
        throw new Error(result.error || "MODEL_DISCOVERY_FAILED");
      }
      const result = (await response.json()) as { models?: DiscoveredModel[] };
      const models = result.models || [];
      setDiscoveredModels(models);
      setChannelForm((current) => ({
        ...current,
        models: mergeDiscoveredChannelModels(current.models, models),
      }));
      setModelDiscoveryMessage({
        kind: models.length > 0 ? "success" : "pending",
        text:
          models.length > 0
            ? `${models.length} ${t("channelsFormDiscoveredTitle")} ${t(
                "channelsFormDiscoverImported"
              )}`
            : t("channelsFormDiscoverEmpty"),
      });
    } catch (error) {
      setDiscoveredModels([]);
      const errorCode = error instanceof Error ? error.message : "";
      const message =
        errorCode === "CHANNEL_CREDENTIAL_REQUIRED"
          ? t("channelsFormDiscoverCredentialRequired")
          : errorCode === "CHANNEL_CREDENTIAL_INVALID"
            ? t("channelsCredentialInvalid")
            : errorCode === "INVALID_REQUEST"
              ? t("channelsSaveInvalid")
              : t("channelsFormDiscoverFailed");
      setModelDiscoveryMessage({ kind: "error", text: message });
    } finally {
      setModelDiscoveryBusy(false);
    }
  }

  async function handleChannelSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const models = channelForm.models
      .map((model) => ({
        model: model.model.trim(),
        upstream_model: (model.upstream_model || model.model).trim(),
        enabled: model.enabled,
      }))
      .filter((model) => model.model);

    if (
      !channelForm.name.trim() ||
      !channelForm.base_url.trim() ||
      (!channelForm.id && !channelForm.api_key.trim())
    ) {
      setChannelsMessage({ kind: "error", text: t("channelsValidationFailed") });
      return;
    }

    setChannelActionBusy(channelForm.id ? `update:${channelForm.id}` : "create");
    setChannelsMessage({ kind: "pending", text: t("channelsSavePending") });
    try {
      const payload = {
        name: channelForm.name.trim(),
        provider: channelForm.provider,
        base_url: channelForm.base_url.trim(),
        api_key: channelForm.api_key.trim(),
        status: channelForm.status,
        upstream_cost_discount: channelForm.upstream_cost_discount.trim(),
        upstream_integration: channelForm.upstream_integration,
        upstream_account_credential: channelForm.upstream_account_credential.trim(),
        upstream_account_user_id:
          channelForm.upstream_integration === "newapi"
            ? channelForm.upstream_account_user_id.trim()
            : "",
        clear_upstream_account_credential: channelForm.clear_upstream_account_credential,
        priority: Number(channelForm.priority),
        weight: Number(channelForm.weight),
        models,
      };
      const response = await fetchAdminSensitive(
        channelForm.id ? `/admin/v1/channels/${channelForm.id}` : "/admin/v1/channels",
        {
          method: channelForm.id ? "PUT" : "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          credentials: "same-origin",
          body: JSON.stringify(payload),
        }
      );
      if (!response.ok) {
        const result = (await response.json().catch(() => ({}))) as { error?: string };
        if (response.status === 400) {
          throw new Error(
            result.error === "CHANNEL_CREDENTIAL_REQUIRED"
              ? t("channelsCredentialRequired")
              : result.error === "CHANNEL_CREDENTIAL_INVALID"
                ? t("channelsCredentialInvalid")
                : t("channelsSaveInvalid")
          );
        }
        throw new Error(result.error || "save failed");
      }
      closeChannelForm();
      await refreshChannels(false);
      setChannelsMessage({ kind: "success", text: t("channelsSaveSuccess") });
    } catch (error) {
      setChannelsMessage({
        kind: "error",
        text:
          error instanceof Error &&
          ([
            t("channelsSaveInvalid"),
            t("channelsCredentialRequired"),
            t("channelsCredentialInvalid"),
          ] as string[]).includes(error.message)
            ? error.message
            : t("channelsUnavailable"),
      });
    } finally {
      setChannelActionBusy("");
    }
  }

  async function changeChannelStatus(
    channel: ChannelSummary,
    nextStatus: "active" | "disabled"
  ) {
    setChannelDeleteConfirm("");
    setChannelActionBusy(`status:${channel.id}`);
    try {
      const response = await fetchAdminSensitive(
        `/admin/v1/channels/${channel.id}/${nextStatus === "active" ? "enable" : "pause"}`,
        {
          method: "POST",
          headers: { Accept: "application/json" },
          credentials: "same-origin",
        }
      );
      if (!response.ok) {
        throw new Error("status failed");
      }
      await refreshChannels(false);
      setChannelsMessage({ kind: "success", text: t("channelsStatusSuccess") });
    } catch {
      setChannelsMessage({ kind: "error", text: t("channelsUnavailable") });
    } finally {
      setChannelActionBusy("");
    }
  }

  async function deleteChannel(channel: ChannelSummary) {
    if (channelDeleteConfirm !== channel.id) {
      setChannelDeleteConfirm(channel.id);
      setChannelsMessage({ kind: "pending", text: t("channelsDeleteConfirm") });
      return;
    }
    setChannelActionBusy(`delete:${channel.id}`);
    try {
      const response = await fetchAdminSensitive(`/admin/v1/channels/${channel.id}`, {
        method: "DELETE",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) {
        throw new Error("delete failed");
      }
      await refreshChannels(false);
      setChannelDeleteConfirm("");
      setChannelsMessage({ kind: "success", text: t("channelsDeleteSuccess") });
    } catch {
      setChannelsMessage({ kind: "error", text: t("channelsUnavailable") });
    } finally {
      setChannelActionBusy("");
    }
  }

  async function handleSignOut() {
    const logoutAudience = principal?.audience || audience;
    await fetch(`/${logoutAudience}/v1/auth/logout`, {
      method: "POST",
      credentials: "same-origin",
    }).catch(() => undefined);
    setSignedIn(false);
    setPrincipal(null);
    setSecuritySettings({ admin_mfa_enabled: false, updated_at: "", updated_by: "" });
    setSecurityMessage({ kind: "", text: "" });
    setChannels([]);
    setChannelsMessage({ kind: "", text: "" });
    setGroups([]);
    setGroupsMessage({ kind: "", text: "" });
    setGroupFormOpen(false);
    setGroupForm(defaultGroupForm());
    setGroupActionBusy("");
    setGroupDeleteConfirm("");
    setTokens([]);
    setTokensMessage({ kind: "", text: "" });
    setTokensBusy(false);
    setTokenActionBusy("");
    setTokenCreateOpen(false);
    setTokenCreateForm(defaultTokenCreateForm());
    setTokenCreateMessage({ kind: "", text: "" });
    setIssuedToken(null);
    setTokenEditingID("");
    setConsoleTokenGroups([]);
    setConsoleTokenGroupsBusy(false);
    setProjects([]);
    setProjectsBusy(false);
    setProjectsMessage({ kind: "", text: "" });
    setMembers([]);
    setMembersBusy(false);
    setMembersMessage({ kind: "", text: "" });
    setProjectMembers([]);
    setProjectMembersBusy(false);
    setProjectMembersMessage({ kind: "", text: "" });
    setSelectedProjectID("");
    setProjectActionBusy("");
    setProjectDeleteConfirm("");
    setMemberActionBusy("");
    setProjectMemberActionBusy("");
    setUsers([]);
    setUsersMessage({ kind: "", text: "" });
    setUsersBusy(false);
    setUserActionBusy("");
    setPlatformRoles([]);
    setPlatformPermissions([]);
    setPlatformRolesBusy(false);
    setPlatformRolesMessage({ kind: "", text: "" });
    setConsoleSection("dashboard");
    setUsageStatus(null);
	setConsoleUsageReport(null);
	setConsoleUsageOffset(0);
    setConsoleUsageTokenName("");
    setConsoleUsageModel("");
    setConsoleUsageGroup("");
    setConsoleUsageFrom("");
    setConsoleUsageTo("");
    setUsageMessage({ kind: "", text: "" });
    setUsageBusy(false);
    setConsoleProfile(null);
    setProfileForm(defaultProfileForm());
    setEmailForm(defaultEmailForm());
    setPasswordForm(defaultPasswordForm());
    setProfileMessage({ kind: "", text: "" });
    setProfileBusy(false);
    setMfaStatus({ enabled: false });
    setMfaEnrollment(null);
    setProfileMfaCode("");
    setMfaBusy(false);
    setAdminProfile(null);
    setAdminProfileForm(defaultProfileForm());
    setAdminEmailForm(defaultEmailForm());
    setAdminPasswordForm(defaultPasswordForm());
    setAdminProfileMessage({ kind: "", text: "" });
    setAdminProfileBusy(false);
    setAdminMfaStatus({ enabled: false });
    setAdminMfaEnrollment(null);
    setAdminMfaCode("");
    setAdminMfaBusy(false);
	    setAdminSiteForm(defaultSiteSettings());
	    setSmtpForm(defaultSMTPSettings());
	    setEmailSettings(null);
	    setEmailMessage({ kind: "", text: "" });
	    setEmailTestRecipient("");
	    setEmailBusy(false);
	    setSMTPConnectionBusy(false);
	    setSMTPMessageBusy(false);
	    setFeatureSettings(defaultFeatureSettings());
	    setFeatureMessage({ kind: "", text: "" });
	    setFeatureBusy(false);
	    setEmailTemplates([]);
	    setEmailTemplatesBusy(false);
	    setEmailTemplatesMessage({ kind: "", text: "" });
    setSiteSettingsMessage({ kind: "", text: "" });
    setSiteSettingsBusy(false);
    setModelCatalog([]);
    setModelCatalogMessage({ kind: "", text: "" });
    setModelCatalogBusy(false);
    setAdminModelStatusReport(null);
    setAdminModelStatusMessage({ kind: "", text: "" });
    setAdminModelStatusBusy(false);
    setAdminModelMonitors([]);
    setAdminModelMonitorsMessage({ kind: "", text: "" });
    setAdminModelMonitorsBusy(false);
    setAdminModelMonitorFormOpen(false);
    setAdminModelMonitorForm(defaultModelMonitorForm());
    setAdminModelMonitorActionBusy("");
    setOfficialPriceSyncBusy(false);
    cancelAdminStepUp();
    setChannelFormOpen(false);
    setChannelForm(defaultChannelForm());
    setChannelActionBusy("");
    setChannelDeleteConfirm("");
    setDiscoveredModels([]);
    setModelDiscoveryMessage({ kind: "", text: "" });
    setPrices([]);
    setBillingAccount(null);
    setBillingMessage({ kind: "", text: "" });
    setCreditForm(defaultCreditForm());
    setUsageReport(null);
    setUsageReportMessage({ kind: "", text: "" });
    setUsageReportBusy(false);
    setFinanceReport(null);
    setFinanceReportMessage({ kind: "", text: "" });
    setFinanceReportBusy(false);
	setOperationsSnapshot(null);
	setOperationsBusy(false);
	setAuditReport(null);
	setAuditBusy(false);
	setAuditMessage({ kind: "", text: "" });
    setReportSearch("");
    setReportStatus("");
    setReportTenant("");
    setReportModel("");
    setReportGroup("");
    setReportFrom("");
    setReportTo("");
	setFinanceSearch("");
	setFinanceCurrency("");
	setFinanceFrom("");
	setFinanceTo("");
    setReportOffset(0);
    setLoginMessage({ kind: "success", text: t("signedOut") });
    if (logoutAudience === "admin" && isAdminEntryLocation()) {
      window.location.replace("/#home");
      return;
    }
    window.location.hash = "#home";
  }

  async function persistSecurity(nextEnabled: boolean) {
    if (!signedIn || audience !== "admin") return;
    setSecurityBusy(true);
    setSecurityMessage({ kind: "pending", text: t("securitySaving") });
    try {
      const response = await fetchAdminSensitive("/admin/v1/security/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ admin_mfa_enabled: nextEnabled }),
      });
      const result = (await response.json().catch(() => ({}))) as Partial<SecuritySettings> & {
        error?: string;
      };
      if (!response.ok) {
        if (response.status === 409) {
          setSecuritySettings((current) => ({ ...current, admin_mfa_enabled: false }));
          setSecurityMessage({ kind: "error", text: t("securityMessageNeedsMfa") });
          return;
        }
        throw new Error(result.error || t("securityMessageUnavailable"));
      }
      setSecuritySettings({
        admin_mfa_enabled: Boolean(result.admin_mfa_enabled),
        updated_at: result.updated_at || "",
        updated_by: result.updated_by || "",
      });
      setSecurityMessage({
        kind: "success",
        text: result.admin_mfa_enabled
          ? t("securityMessageEnabled")
          : t("securityMessageDisabled"),
      });
    } catch {
      setSecurityMessage({ kind: "error", text: t("securityMessageUnavailable") });
    } finally {
      setSecurityBusy(false);
    }
  }

  const currentView = route.view;

  return (
    <div className="min-h-screen flex flex-col bg-slate-50 dark:bg-slate-950 text-slate-900 dark:text-slate-100 transition-colors duration-200">
      {/* Universal Top Navigation */}
      <Navbar
        theme={theme}
        setTheme={setTheme}
        language={language}
        setLanguage={setLanguage}
        signedIn={signedIn}
        currentView={currentView}
        routeTo={routeTo}
        onSignOut={handleSignOut}
        principalName={principal?.id}
        principalAudience={principal?.audience || audience}
        siteName={siteSettings.site_name}
        siteLogoURL={siteSettings.site_logo_url}
        registrationEnabled={publicFeatures?.registration_enabled !== false}
      />

      {/* Main View Router */}
      <main className={currentView === "admin" || currentView === "console" || currentView === "models" ? "flex-1 w-full flex flex-col min-w-0" : "flex-1 w-full max-w-[1720px] mx-auto px-4 sm:px-6 lg:px-8"}>
        <Suspense fallback={
          <div className="flex min-h-[50vh] items-center justify-center gap-2 text-sm text-slate-500 dark:text-slate-400">
            <span className="h-4 w-4 animate-spin rounded-full border-2 border-indigo-500/30 border-t-indigo-500" />
            <span>{t("restoreChecking")}</span>
          </div>
        }>
          {!sessionReady ? (
            <div className="flex min-h-[50vh] items-center justify-center gap-2 text-sm text-slate-500 dark:text-slate-400">
              <span className="h-4 w-4 animate-spin rounded-full border-2 border-indigo-500/30 border-t-indigo-500" />
              <span>{t("restoreChecking")}</span>
            </div>
          ) : currentView === "home" ? (
            <HomeView
            language={language}
            signedIn={signedIn}
            workspaceRoute={(principal?.audience || audience) === "console" ? "#console/dashboard" : "#admin/dashboard"}
            routeTo={routeTo}
            handleSignOut={handleSignOut}
            models={modelCatalog}
            registrationEnabled={publicFeatures?.registration_enabled !== false}
            apiEndpoints={publicAPIEndpoints}
            />
          ) : currentView === "models" ? (
            <ModelPlazaView
            language={language}
            models={modelCatalog}
            busy={modelCatalogBusy}
            message={modelCatalogMessage}
            refresh={refreshModelCatalog}
            />
          ) : currentView === "login" || currentView === "admin-login" ? (
            <LoginView
            language={language}
            loginAudience={currentView === "admin-login" ? "admin" : "console"}
            totpEnabled={publicFeatures?.totp_enabled === true}
            registrationEnabled={publicFeatures?.registration_enabled !== false}
            email={email}
            setEmail={setEmail}
            password={password}
            setPassword={setPassword}
            tenantId={tenantId}
            setTenantId={setTenantId}
            mfaCode={mfaCode}
            setMfaCode={setMfaCode}
            handleLogin={handleLogin}
            loginMessage={loginMessage}
            formBusy={formBusy}
            signedIn={signedIn}
            principal={principal}
            handleSignOut={handleSignOut}
            routeTo={routeTo}
            oauthProviders={oauthProviders}
            />
          ) : currentView === "register" ? (
            <RegisterView language={language} routeTo={routeTo} onRegistered={handleRegistered} registrationEnabled={publicFeatures?.registration_enabled !== false} oauthProviders={oauthProviders} />
          ) : currentView === "reset" ? (
            <ResetPasswordView language={language} token={route.reset_token} routeTo={routeTo} />
          ) : currentView === "verify-email" ? (
            <EmailVerificationView language={language} token={route.verification_token} routeTo={routeTo} />
          ) : currentView === "not-found" ? (
            <NotFoundView language={language} routeTo={routeTo} />
          ) : currentView === "console" ? (
            <ConsoleView
            language={language}
            principal={principal}
            section={consoleSection}
            setSection={setConsoleSection}
            routeTo={routeTo}
            handleSignOut={handleSignOut}
            tokens={tokens}
            tokensBusy={tokensBusy}
            tokensMessage={tokensMessage}
            refreshTokens={refreshConsoleTokens}
            editToken={openEditToken}
            pauseToken={pauseConsoleToken}
            resumeToken={resumeConsoleToken}
            terminateToken={terminateConsoleToken}
            deleteToken={deleteConsoleToken}
            copyToken={copyConsoleToken}
            tokenSecretAvailable={tokenSecretAvailable}
            tokenActionBusy={tokenActionBusy}
            openCreateToken={openCreateToken}
            projects={projects}
            projectsBusy={projectsBusy}
            projectsMessage={projectsMessage}
            refreshProjects={refreshConsoleProjects}
            saveProject={saveConsoleProject}
            deleteProject={deleteConsoleProject}
            projectActionBusy={projectActionBusy}
            projectDeleteConfirm={projectDeleteConfirm}
            members={members}
            membersBusy={membersBusy}
            membersMessage={membersMessage}
            refreshMembers={refreshConsoleMembers}
            addMember={addConsoleMember}
            updateMember={updateConsoleMember}
            removeMember={removeConsoleMember}
            memberActionBusy={memberActionBusy}
            projectMembers={projectMembers}
            projectMembersBusy={projectMembersBusy}
            projectMembersMessage={projectMembersMessage}
            selectedProjectID={selectedProjectID}
            selectProject={setSelectedProjectID}
            refreshProjectMembers={refreshConsoleProjectMembers}
            addProjectMember={addConsoleProjectMember}
            updateProjectMember={updateConsoleProjectMember}
            removeProjectMember={removeConsoleProjectMember}
            projectMemberActionBusy={projectMemberActionBusy}
            billingAccount={billingAccount}
            billingBusy={billingBusy}
            billingMessage={billingMessage}
            refreshBilling={refreshConsoleBilling}
            enterpriseVerification={enterpriseVerification}
            enterpriseBusy={enterpriseBusy}
            enterpriseMessage={enterpriseMessage}
            refreshEnterprise={() => refreshEnterpriseVerification(true)}
            submitEnterprise={submitEnterpriseVerification}
            paymentProviders={paymentProviders}
            paymentBusy={paymentBusy}
            paymentMessage={paymentMessage}
            paymentOrder={paymentOrder}
            createPaymentOrder={createPaymentOrder}
            refreshPaymentOrder={refreshPaymentOrder}
            capturePayPal={capturePayPal}
            paymentOrders={paymentOrders}
            paymentOrdersBusy={paymentOrdersBusy}
            refreshPaymentOrders={refreshPaymentOrders}
            usageStatus={usageStatus}
            usageBusy={usageBusy}
            usageMessage={usageMessage}
            dashboardReport={consoleDashboardReport}
            dashboardBusy={consoleDashboardBusy}
            dashboardMessage={consoleDashboardMessage}
            refreshDashboard={refreshConsoleDashboard}
            refreshUsage={refreshConsoleUsage}
			 consoleUsageReport={consoleUsageReport}
            consoleUsageTokens={tokens}
            consoleUsageGroups={consoleTokenGroups}
            consoleUsageTokenName={consoleUsageTokenName}
            setConsoleUsageTokenName={setConsoleUsageTokenName}
            consoleUsageModel={consoleUsageModel}
            setConsoleUsageModel={setConsoleUsageModel}
            consoleUsageGroup={consoleUsageGroup}
            setConsoleUsageGroup={setConsoleUsageGroup}
            consoleUsageFrom={consoleUsageFrom}
            setConsoleUsageFrom={setConsoleUsageFrom}
            consoleUsageTo={consoleUsageTo}
            setConsoleUsageTo={setConsoleUsageTo}
            apiEndpoints={publicAPIEndpoints}
            models={modelCatalog}
            modelStatusEnabled={publicFeatures?.model_status_enabled !== false}
            modelStatusReport={modelStatusReport}
            modelStatusBusy={modelStatusBusy}
            modelStatusMessage={modelStatusMessage}
            refreshModelStatus={refreshConsoleModelStatus}
            consoleProfile={consoleProfile}
            profileForm={profileForm}
            setProfileForm={setProfileForm}
            emailForm={emailForm}
            setEmailForm={setEmailForm}
            passwordForm={passwordForm}
            setPasswordForm={setPasswordForm}
            profileBusy={profileBusy}
            profileMessage={profileMessage}
            refreshProfile={refreshConsoleProfile}
            saveProfile={handleProfileSubmit}
            saveEmail={handleEmailSubmit}
            savePassword={handlePasswordSubmit}
            mfaStatus={mfaStatus}
            totpEnabled={publicFeatures?.totp_enabled === true}
            mfaEnrollment={mfaEnrollment}
            profileMfaCode={profileMfaCode}
            setProfileMfaCode={setProfileMfaCode}
            mfaBusy={mfaBusy}
            beginMFA={beginConsoleMFA}
            confirmMFA={confirmConsoleMFA}
            cancelMFA={cancelConsoleMFA}
            disableMFA={disableConsoleMFA}
            />
          ) : (
            <AdminConsole
            language={language}
            adminSection={adminSection}
            setAdminSection={setAdminSection}
            routeTo={routeTo}
            principal={principal}
            siteName={siteSettings.site_name}
            siteLogoURL={siteSettings.site_logo_url}
            handleSignOut={handleSignOut}
            channels={channels}
            channelsBusy={channelsBusy}
            channelsMessage={channelsMessage}
            refreshChannels={refreshChannels}
            openCreateChannel={openCreateChannel}
            openEditChannel={openEditChannel}
            syncChannelAccount={syncChannelAccount}
            changeChannelStatus={changeChannelStatus}
            deleteChannel={deleteChannel}
            channelDeleteConfirm={channelDeleteConfirm}
            channelActionBusy={channelActionBusy}
            groups={groups}
            groupsBusy={groupsBusy}
            groupsMessage={groupsMessage}
            refreshGroups={refreshGroups}
            openCreateGroup={openCreateGroup}
            openEditGroup={openEditGroup}
            deleteGroup={deleteGroup}
            groupDeleteConfirm={groupDeleteConfirm}
            groupActionBusy={groupActionBusy}
            tokens={tokens}
            tokensBusy={tokensBusy}
            tokensMessage={tokensMessage}
            refreshTokens={refreshTokens}
            pauseToken={pauseAdminToken}
            tokenActionBusy={tokenActionBusy}
            users={users}
            usersBusy={usersBusy}
            usersMessage={usersMessage}
            refreshUsers={refreshUsers}
            openEditUser={openEditUser}
            updateUserStatus={updateUserStatus}
            userActionBusy={userActionBusy}
            platformRoles={platformRoles}
            platformPermissions={platformPermissions}
            platformRolesBusy={platformRolesBusy}
            platformRolesMessage={platformRolesMessage}
            refreshPlatformRoles={refreshPlatformRoles}
            savePlatformRole={savePlatformRole}
            disablePlatformRole={disablePlatformRole}
            loadPlatformUserRoles={loadPlatformUserRoles}
            savePlatformUserRoles={savePlatformUserRoles}
            prices={prices}
            billingAccount={billingAccount}
            billingMessage={billingMessage}
            billingBusy={billingBusy}
            officialPriceSyncBusy={officialPriceSyncBusy}
            syncOfficialPrices={syncOfficialPrices}
            openEditPrice={openEditPrice}
            creditForm={creditForm}
            setCreditForm={setCreditForm}
            creditBillingAccount={creditBillingAccount}
            loadBillingAccount={loadBillingAccount}
            securitySettings={securitySettings}
            securityMessage={securityMessage}
            securityBusy={securityBusy}
            persistSecurity={persistSecurity}
            adminProfile={adminProfile}
            adminProfileForm={adminProfileForm}
            setAdminProfileForm={setAdminProfileForm}
            adminEmailForm={adminEmailForm}
            setAdminEmailForm={setAdminEmailForm}
            adminPasswordForm={adminPasswordForm}
            setAdminPasswordForm={setAdminPasswordForm}
            adminProfileBusy={adminProfileBusy}
            adminProfileMessage={adminProfileMessage}
            saveAdminProfile={handleAdminProfileSubmit}
            saveAdminEmail={handleAdminEmailSubmit}
            saveAdminPassword={handleAdminPasswordSubmit}
            adminMfaStatus={adminMfaStatus}
            adminMfaEnrollment={adminMfaEnrollment}
            adminMfaCode={adminMfaCode}
            setAdminMfaCode={setAdminMfaCode}
            adminMfaBusy={adminMfaBusy}
            beginAdminMFA={beginAdminMFA}
            confirmAdminMFA={confirmAdminMFA}
            cancelAdminMFA={cancelAdminMFA}
            disableAdminMFA={disableAdminMFA}
            siteForm={adminSiteForm}
            setSiteForm={setAdminSiteForm}
            siteBusy={siteSettingsBusy}
            siteMessage={siteSettingsMessage}
            saveSiteSettings={saveSystemSettings}
            apiEndpoints={adminAPIEndpoints}
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
            loginSettings={loginSettings}
            loginSettingsBusy={loginSettingsBusy}
            loginSettingsMessage={loginSettingsMessage}
            saveLoginSettings={saveLoginSettings}
            canUpdatePaymentSettings={principal?.permissions?.includes("payment:update") === true}
            usageReport={usageReport}
            usageReportBusy={usageReportBusy}
            usageReportMessage={usageReportMessage}
            refreshUsageReport={refreshUsageReport}
            financeReport={financeReport}
            financeReportBusy={financeReportBusy}
            financeReportMessage={financeReportMessage}
            refreshFinanceReport={refreshFinanceReport}
            financeSearch={financeSearch}
            setFinanceSearch={(value) => { setFinanceSearch(value); }}
            financeCurrency={financeCurrency}
            setFinanceCurrency={(value) => { setFinanceCurrency(value); }}
            financeFrom={financeFrom}
            setFinanceFrom={setFinanceFrom}
            financeTo={financeTo}
            setFinanceTo={setFinanceTo}
            reportSearch={reportSearch}
            setReportSearch={(value) => { setReportSearch(value); setReportOffset(0); }}
            reportStatus={reportStatus}
            setReportStatus={(value) => { setReportStatus(value); setReportOffset(0); }}
            reportTenant={reportTenant}
            setReportTenant={(value: string) => { setReportTenant(value); setReportOffset(0); }}
            reportModel={reportModel}
            setReportModel={(value: string) => { setReportModel(value); setReportOffset(0); }}
            reportGroup={reportGroup}
            setReportGroup={(value: string) => { setReportGroup(value); setReportOffset(0); }}
            reportFrom={reportFrom}
            setReportFrom={(value: string) => { setReportFrom(value); setReportOffset(0); }}
            reportTo={reportTo}
            setReportTo={(value: string) => { setReportTo(value); setReportOffset(0); }}
             reportOffset={reportOffset}
			 adminModelStatusReport={adminModelStatusReport}
			 adminModelStatusBusy={adminModelStatusBusy}
			 adminModelStatusMessage={adminModelStatusMessage}
			 refreshAdminModelStatus={refreshAdminModelStatus}
			 adminModelMonitors={adminModelMonitors}
			 adminModelMonitorsBusy={adminModelMonitorsBusy}
			 adminModelMonitorsMessage={adminModelMonitorsMessage}
			 adminModelMonitorFormOpen={adminModelMonitorFormOpen}
			 adminModelMonitorForm={adminModelMonitorForm}
			 setAdminModelMonitorForm={setAdminModelMonitorForm}
			 adminModelMonitorActionBusy={adminModelMonitorActionBusy}
			 openCreateAdminModelMonitor={openCreateAdminModelMonitor}
			 openEditAdminModelMonitor={openEditAdminModelMonitor}
			 closeAdminModelMonitorForm={closeAdminModelMonitorForm}
			 saveAdminModelMonitor={saveAdminModelMonitor}
			 deleteAdminModelMonitor={deleteAdminModelMonitor}
			 probeAdminModelMonitor={probeAdminModelMonitor}
			 operationsSnapshot={operationsSnapshot}
			 operationsBusy={operationsBusy}
             refreshOperations={refreshOperations}
			 auditReport={auditReport}
			 auditBusy={auditBusy}
			 auditMessage={auditMessage}
			 refreshAudit={refreshAudit}
            enterpriseItems={adminEnterpriseItems}
            enterpriseBusy={adminEnterpriseBusy}
            enterpriseMessage={adminEnterpriseMessage}
            enterpriseStatus={adminEnterpriseStatus}
            setEnterpriseStatus={setAdminEnterpriseStatus}
            refreshEnterprise={() => refreshAdminEnterprise(true)}
            loadEnterprise={loadAdminEnterpriseDetails}
            reviewEnterprise={reviewEnterprise}
            />
          )}
        </Suspense>
      </main>

      <StepUpDialog
        language={language}
        open={stepUpOpen}
        code={stepUpCode}
        error={stepUpError}
        busy={false}
        setCode={setStepUpCode}
        onSubmit={submitAdminStepUp}
        onCancel={cancelAdminStepUp}
      />

      {/* Global Channel Modal */}
      <ChannelModal
        channelFormOpen={channelFormOpen}
        channelForm={channelForm}
        setChannelForm={setChannelForm}
        closeChannelForm={closeChannelForm}
        handleChannelSubmit={handleChannelSubmit}
        channelActionBusy={channelActionBusy}
        modelDiscoveryBusy={modelDiscoveryBusy}
        discoverChannelModels={discoverChannelModels}
        discoveredModels={discoveredModels}
        modelDiscoveryMessage={modelDiscoveryMessage}
        addDiscoveredModel={addDiscoveredModel}
        isChannelModelMapped={isChannelModelMapped}
        setChannelProvider={setChannelProvider}
        updateChannelModel={updateChannelModel}
        addChannelModel={addChannelModel}
        removeChannelModel={removeChannelModel}
        language={language}
      />

      <GroupModal
        open={groupFormOpen}
        form={groupForm}
        setForm={setGroupForm}
        channels={channels}
        language={language}
        busy={Boolean(groupActionBusy)}
        message={groupsMessage}
        onClose={closeGroupForm}
        onSubmit={handleGroupSubmit}
      />

      <TokenCreateModal
        open={tokenCreateOpen}
        mode={tokenCreateMode}
        language={language}
        form={tokenCreateForm}
        setForm={setTokenCreateForm}
        projectIDs={projects.filter((project) => project.status === "active").map((project) => project.id)}
        groups={consoleTokenGroups}
        groupsBusy={consoleTokenGroupsBusy}
        busy={tokenCreateBusy}
        message={tokenCreateMessage}
        issuedToken={issuedToken}
        editing={Boolean(tokenEditingID)}
        onClose={closeTokenCreate}
        onSubmit={handleTokenCreate}
      />

      <UserModal
        open={userFormOpen}
        form={userForm}
        setForm={setUserForm}
        language={language}
        busy={userFormBusy}
        message={userFormMessage}
        onClose={closeUserForm}
        onSubmit={handleUserSubmit}
      />

      <ModelPriceModal
        open={modelPriceFormOpen}
        language={language}
        form={modelPriceForm}
        setForm={setModelPriceForm}
        busy={modelPriceFormBusy}
        message={modelPriceFormMessage}
        onClose={closeModelPriceForm}
        onSubmit={handleModelPriceSubmit}
      />

      {/* Commercial Footer (Visible on Home, Login, Register) */}
      {currentView !== "admin" && currentView !== "console" && <Footer language={language} routeTo={routeTo} siteName={siteSettings.site_name} siteLogoURL={siteSettings.site_logo_url} />}
    </div>
  );
}

function isTokenPriceUnit(unit: string) {
  const normalized = unit.trim().toLowerCase();
  return normalized === "token" || normalized.endsWith("_token");
}

function tokenPriceToDisplay(value: string, unit: string) {
  return decimalShift(value.trim(), tokenUnitPlaces(unit));
}

function tokenDisplayToPrice(value: string, unit: string) {
  return decimalShift(value.trim(), -tokenUnitPlaces(unit));
}

function tokenUnitPlaces(unit: string) {
  return unit.toLowerCase().includes("1k") ? 3 : 6;
}

function decimalShift(value: string, places: number) {
  const trimmed = value.trim();
  if (!/^(?:\d+(?:\.\d*)?|\.\d+)$/.test(trimmed)) return "";
  const parts = trimmed.split(".");
  const integer = parts[0] || "0";
  const fraction = parts[1] || "";
  const digits = integer + fraction;
  const point = integer.length + places;
  let result: string;
  if (point <= 0) {
    result = "0." + "0".repeat(-point) + digits;
  } else if (point >= digits.length) {
    result = digits + "0".repeat(point - digits.length);
  } else {
    result = digits.slice(0, point) + "." + digits.slice(point);
  }
  if (result.includes(".")) {
    const resultParts = result.split(".");
    result = (resultParts[0].replace(/^0+(?=\d)/, "") || "0") + "." + resultParts[1].replace(/0+$/, "");
    result = result.replace(/\.$/, "");
  } else {
    result = result.replace(/^0+(?=\d)/, "") || "0";
  }
  return result || "0";
}
