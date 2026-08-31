import { useEffect, useState } from "react";
import { Navbar } from "@/components/layout/Navbar";
import { Footer } from "@/components/layout/Footer";
import { HomeView } from "@/components/HomeView";
import { ModelPlazaView } from "@/components/ModelPlazaView";
import { LoginView } from "@/components/LoginView";
import { RegisterView } from "@/components/RegisterView";
import { ConsoleView } from "@/components/ConsoleView";
import { AdminConsole } from "@/components/admin/AdminConsole";
import { ChannelModal } from "@/components/ChannelModal";
import { GroupModal } from "@/components/GroupModal";
import { TokenCreateModal } from "@/components/TokenCreateModal";
import { UserModal } from "@/components/UserModal";
import { ModelPriceModal } from "@/components/ModelPriceModal";
import { NotFoundView } from "@/components/NotFoundView";
import { ResetPasswordView } from "@/components/ResetPasswordView";
import {
  AdminSection,
  AuditReport,
  Audience,
  BillingAccount,
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
  ConsoleProfile,
  EmailFormState,
  	FinanceReport,
	OperationsSnapshot,
  MFAEnrollment,
  MFAStatus,
  PasswordFormState,
  ProfileFormState,
  PriceMatrixSummary,
  ModelPriceFormState,
  Principal,
  PublicModelSummary,
  SiteSettings,
  SectionRoute,
  SecuritySettings,
  SystemSettings,
  Theme,
  TokenSummary,
  TokenCreateFormState,
  TokenGroupOption,
  TranslationKey,
  TenantSummary,
  UsageReport,
  UserAdminFormState,
  UserSummary,
} from "@/types";
import { translations } from "@/locales/translations";

function parseRoute(hash: string): SectionRoute {
  const raw = hash.replace(/^#/, "");
  if (raw === "") {
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
  if (raw === "models") {
    return { view: "models", section: "dashboard" };
  }
  if (raw === "admin" || raw.startsWith("admin/")) {
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
  return value === "usage" || value === "projects" || value === "tokens" || value === "billing" || value === "profile" || value === "docs"
    ? value
    : "dashboard";
}

function normalizeSection(value: string): AdminSection {
  if (value === "security") return "settings";
  return value === "ops" ||
    value === "users" ||
    value === "groups" ||
    value === "tokens" ||
    value === "channels" ||
    value === "billing" ||
    value === "finance" ||
    value === "usage" ||
    value === "audit" ||
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

function defaultProviderBaseURL(provider: "openai" | "anthropic" | "grok" | "gemini") {
  switch (provider) {
    case "anthropic":
      return "https://api.anthropic.com";
    case "grok":
      return "https://api.x.ai/v1";
    case "gemini":
      return "https://generativelanguage.googleapis.com";
    default:
      return "https://api.openai.com/v1";
  }
}

function defaultChannelForm(provider: "openai" | "anthropic" | "grok" | "gemini" = "openai"): ChannelFormState {
  const model = provider === "openai" ? "gpt-5" : provider === "anthropic" ? "claude-sonnet-5" : provider === "grok" ? "grok-4.6" : "gemini-3.7-flash";
  return {
    id: "",
    name: provider === "openai" ? "OpenAI Official" : provider === "anthropic" ? "Anthropic Official" : provider === "grok" ? "Grok Official" : "Gemini Official",
    provider,
    base_url: defaultProviderBaseURL(provider),
    api_key: "",
    status: "active",
    priority: 100,
    weight: 100,
    models: [{ id: newFormRowID(), model, upstream_model: model, enabled: true }],
  };
}

function channelFormFromSummary(channel: ChannelSummary): ChannelFormState {
  const provider = channel.provider === "anthropic" || channel.provider === "grok" || channel.provider === "gemini" ? channel.provider : "openai";
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
        : [{ id: newFormRowID(), model: "", upstream_model: "", enabled: true }],
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
  };
}

function defaultSiteSettings(): SiteSettings {
  return { site_name: "AI Token Gateway", site_logo_url: "", site_favicon_url: "" };
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
    priority: 100,
    channel_ids: [],
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
  };
}

function defaultUserAdminForm(): UserAdminFormState {
  return {
    id: "",
    email: "",
    display_name: "",
    password: "",
    tenant_id: "",
    tenant_role: "developer",
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

export default function App() {
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = window.localStorage.getItem("ai-token-theme");
    if (saved === "light" || saved === "dark") return saved;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });
  const [language, setLanguage] = useState<Language>(() =>
    window.localStorage.getItem("ai-token-language") === "en" ? "en" : "zh"
  );
  const [audience, setAudience] = useState<Audience>("admin");
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
	const [consoleUsageOffset, setConsoleUsageOffset] = useState(0);
  const [usageBusy, setUsageBusy] = useState(false);
  const [usageMessage, setUsageMessage] = useState<LoginMessage>({ kind: "", text: "" });
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
  const [userTenants, setUserTenants] = useState<TenantSummary[]>([]);
  const [userFormOpen, setUserFormOpen] = useState(false);
  const [userFormMode, setUserFormMode] = useState<"create" | "edit">("create");
  const [userForm, setUserForm] = useState<UserAdminFormState>(() => defaultUserAdminForm());
  const [userFormBusy, setUserFormBusy] = useState(false);
  const [userFormMessage, setUserFormMessage] = useState<LoginMessage>({ kind: "", text: "" });

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
  const [siteSettingsMessage, setSiteSettingsMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [siteSettingsBusy, setSiteSettingsBusy] = useState(false);

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
  const [tokenCreateMode, setTokenCreateMode] = useState<"admin" | "console">("console");
  const [tokenCreateForm, setTokenCreateForm] = useState<TokenCreateFormState>(() => defaultTokenCreateForm());
  const [tokenCreateBusy, setTokenCreateBusy] = useState(false);
  const [tokenCreateMessage, setTokenCreateMessage] = useState<LoginMessage>({ kind: "", text: "" });
  const [issuedToken, setIssuedToken] = useState<IssuedTokenResponse | null>(null);
  const [tokenRevokeConfirm, setTokenRevokeConfirm] = useState("");
  const [consoleTokenGroups, setConsoleTokenGroups] = useState<TokenGroupOption[]>([]);
  const [consoleTokenGroupsBusy, setConsoleTokenGroupsBusy] = useState(false);
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
        const result = (await response.json().catch(() => ({}))) as Partial<SiteSettings>;
        if (!cancelled && response.ok) {
          setSiteSettings({
            site_name: result.site_name?.trim() || "AI Token Gateway",
            site_logo_url: result.site_logo_url?.trim() || "",
            site_favicon_url: result.site_favicon_url?.trim() || "",
          });
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
    let cancelled = false;
    async function restoreSession() {
      try {
        const response = await fetch("/admin/v1/me", {
          headers: { Accept: "application/json" },
          credentials: "same-origin",
        });
        if (!response.ok) {
          throw new Error("unauthorized");
        }
        const profile = (await response.json()) as Principal;
        if (cancelled) return;
        setSignedIn(true);
        setAudience("admin");
        setPrincipal(profile);
      } catch {
        try {
          const response = await fetch("/console/v1/me", {
            headers: { Accept: "application/json" },
            credentials: "same-origin",
          });
          if (!response.ok) throw new Error("unauthorized");
          const profile = (await response.json()) as Principal;
          if (cancelled) return;
          setSignedIn(true);
          setAudience("console");
          setPrincipal(profile);
        } catch {
          if (!cancelled) {
            setSignedIn(false);
            setPrincipal(null);
          }
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
  }, [route, sessionReady, signedIn, audience]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin") {
      setChannels([]);
      setGroups([]);
      return;
    }
    refreshChannels(adminSection === "channels");
  }, [signedIn, audience, route.view, adminSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || (consoleSection !== "dashboard" && consoleSection !== "tokens")) return;
    refreshConsoleTokens(true);
    refreshConsoleTokenGroups();
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (route.view !== "home" && route.view !== "models") return;
    refreshModelCatalog(true);
  }, [route.view, language]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || (adminSection !== "groups" && adminSection !== "tokens")) {
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
    if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "users") {
      return;
    }
    refreshUsers(true);
    refreshUserTenants();
  }, [signedIn, audience, route.view, adminSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || consoleSection !== "billing") return;
    refreshConsoleBilling();
  }, [signedIn, audience, route.view, consoleSection, language]);

  useEffect(() => {
    if (!signedIn || audience !== "console" || route.view !== "console" || (consoleSection !== "dashboard" && consoleSection !== "usage")) return;
    refreshConsoleUsage();
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
    if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "usage") return;
    refreshUsageReport(true, 0);
  }, [signedIn, audience, route.view, adminSection, language, reportSearch, reportStatus, reportTenant, reportModel, reportGroup, reportFrom, reportTo]);

  useEffect(() => {
    if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "finance") return;
    refreshFinanceReport(true, 0);
  }, [signedIn, audience, route.view, adminSection, language, financeSearch, financeCurrency, financeFrom, financeTo]);

	useEffect(() => {
		if (!signedIn || audience !== "admin" || route.view !== "admin" || (adminSection !== "dashboard" && adminSection !== "ops")) return;
		refreshOperations(true);
	}, [signedIn, audience, route.view, adminSection, language]);

	useEffect(() => {
		if (!signedIn || audience !== "admin" || route.view !== "admin" || adminSection !== "audit") return;
		refreshAudit(true, 0);
	}, [signedIn, audience, route.view, adminSection, language]);

  const routeTo = (target: string) => {
    window.location.hash = target;
  };

  async function handleLogin(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoginMessage({ kind: "pending", text: t("signingIn") });
    setFormBusy(true);
    try {
      const payload: Record<string, string> = { email, password, mfa_code: mfaCode };
      if (audience === "console") {
        payload.tenant_id = tenantId;
      }
      const response = await fetch(`/${audience}/v1/auth/login`, {
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
      if (audience === "admin") {
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
      const [profileResponse, mfaResponse, settingsResponse] = await Promise.all([
        fetch("/admin/v1/profile", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
        fetch("/admin/v1/auth/mfa/status", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
        fetch("/admin/v1/settings", { headers: { Accept: "application/json" }, credentials: "same-origin" }),
      ]);
      const profileResult = (await profileResponse.json().catch(() => ({}))) as ConsoleProfile & { error?: string };
      const mfaResult = (await mfaResponse.json().catch(() => ({}))) as MFAStatus & { error?: string };
      const settingsResult = (await settingsResponse.json().catch(() => ({}))) as Partial<SystemSettings> & { error?: string };
      if (!profileResponse.ok) throw new Error(resolveProfileError(profileResponse.status, profileResult.error, t));
      if (!mfaResponse.ok) throw new Error(resolveProfileError(mfaResponse.status, mfaResult.error, t));
      if (!settingsResponse.ok) throw new Error(resolveSystemSettingsError(settingsResponse.status, settingsResult.error, t));
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
      const response = await fetch("/admin/v1/settings", {
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
      const response = await fetch(groupForm.id ? `/admin/v1/groups/${encodeURIComponent(groupForm.id)}` : "/admin/v1/groups", {
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
      const response = await fetch(`/admin/v1/groups/${encodeURIComponent(group.id)}`, {
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

  async function updateTokenGroup(token: TokenSummary, groupID: string) {
    if (!groupID || token.group_id === groupID) return;
    setTokenActionBusy(token.id);
    setTokensMessage({ kind: "pending", text: t("tokensLoading") });
    try {
      const response = await fetch(`/admin/v1/tokens/${encodeURIComponent(token.id)}/group`, {
        method: "PUT",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ group_id: groupID }),
      });
      const result = (await response.json().catch(() => ({}))) as TokenSummary & { error?: string };
      if (!response.ok) {
        throw new Error(result.error || "token group update failed");
      }
      setTokens((current) => current.map((item) => (item.id === token.id ? result : item)));
      setTokensMessage({ kind: "success", text: t("tokensUpdateSuccess") });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensUnavailable") });
    } finally {
      setTokenActionBusy("");
    }
  }

  async function refreshConsoleTokens(showPending = false) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id) return;
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
    if (!signedIn || audience !== "console" || !principal?.tenant_id) return;
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

  async function refreshConsoleBilling() {
    if (!signedIn || audience !== "console" || !principal?.tenant_id) return;
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

	async function refreshConsoleUsage(showPending = false, offset = consoleUsageOffset) {
    if (!signedIn || audience !== "console" || !principal?.tenant_id) return;
    setUsageBusy(true);
    if (showPending) setUsageMessage({ kind: "pending", text: t("consoleUsageLoading") });
    try {
		const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/usage?limit=50&offset=${offset}`, {
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

  async function refreshUserTenants() {
    if (!signedIn || audience !== "admin") return;
    try {
      const response = await fetch("/admin/v1/tenants", { headers: { Accept: "application/json" }, credentials: "same-origin" });
      const result = (await response.json().catch(() => ({}))) as { tenants?: TenantSummary[] };
      if (!response.ok) throw new Error("tenants unavailable");
      setUserTenants(result.tenants || []);
    } catch {
      setUserTenants([]);
    }
  }

  async function updateUserStatus(user: UserSummary, status: "active" | "locked" | "disabled") {
    if (user.id === principal?.id || userActionBusy) return;
    setUserActionBusy(user.id);
    setUsersMessage({ kind: "pending", text: t("usersUpdating") });
    try {
      const response = await fetch(`/admin/v1/users/${encodeURIComponent(user.id)}/status`, {
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
      setUsersMessage({ kind: "error", text: error instanceof Error && error.message === "LAST_PLATFORM_ADMIN_PROTECTED" ? t("usersLastAdminProtected") : t("usersUpdateFailed") });
    } finally {
      setUserActionBusy("");
    }
  }

  function openCreateUser() {
    setUserFormMode("create");
    setUserForm(defaultUserAdminForm());
    setUserFormMessage({ kind: "", text: "" });
    setUserFormOpen(true);
  }

  function openEditUser(user: UserSummary) {
    if (user.id === principal?.id) return;
    setUserFormMode("edit");
    setUserForm({
      id: user.id,
      email: user.email,
      display_name: user.display_name,
      password: "",
      tenant_id: "",
      tenant_role: "developer",
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
    const passwordValue = userForm.password.trim();
    if (!emailValue || !displayName || (userFormMode === "create" && (!passwordValue || !userForm.tenant_id)) || (passwordValue && passwordValue.length < 12)) {
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
      let endpoint = "/admin/v1/users";
      let method = "POST";
      if (userFormMode === "create") {
        payload.tenant_id = userForm.tenant_id;
        payload.tenant_role = userForm.tenant_role;
      } else {
        endpoint = `/admin/v1/users/${encodeURIComponent(userForm.id)}`;
        method = "PUT";
      }
      const response = await fetch(endpoint, {
        method,
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(payload),
      });
      const result = (await response.json().catch(() => ({}))) as UserSummary & { error?: string };
      if (!response.ok) {
        if (result.error === "EMAIL_ALREADY_EXISTS") throw new Error(t("usersEmailExists"));
        if (result.error === "TENANT_NOT_FOUND") throw new Error(t("usersTenantUnavailable"));
        throw new Error(t("usersSaveFailed"));
      }
      setUsers((current) => userFormMode === "create" ? [result, ...current] : current.map((item) => item.id === result.id ? result : item));
      setUsersMessage({ kind: "success", text: userFormMode === "create" ? t("usersCreateSuccess") : t("usersEditSuccess") });
      closeUserForm();
    } catch (error) {
      setUserFormMessage({ kind: "error", text: error instanceof Error ? error.message : t("usersSaveFailed") });
    } finally {
      setUserFormBusy(false);
    }
  }

  function handleRegistered(registeredEmail: string, registeredTenantID: string) {
    setEmail(registeredEmail);
    setTenantId(registeredTenantID);
    setPassword("");
    setMfaCode("");
    setAudience("console");
    setLoginMessage({ kind: "success", text: t("registerSuccessGoLogin") });
    window.location.hash = "#login";
  }

  function openCreateToken(mode: "admin" | "console") {
    const projectID = mode === "console" ? principal?.project_ids?.[0] || "" : "";
    const availableGroups: TokenGroupOption[] = mode === "console"
      ? consoleTokenGroups
      : groups.map((group) => ({
          id: group.id,
          code: group.code,
          name: group.name,
          multiplier: group.multiplier,
          billing_type: group.billing_type,
          status: group.status,
          models: group.models || [],
        }));
    const defaultGroup = availableGroups.find((group) => group.code === "default") || availableGroups.find((group) => group.status === "active");
    setTokenCreateMode(mode);
    setTokenCreateForm({
      ...defaultTokenCreateForm(),
      tenant_id: mode === "console" ? principal?.tenant_id || "" : "",
      project_id: projectID,
      group_id: defaultGroup?.id || "",
    });
    setTokenCreateMessage({ kind: "", text: "" });
    setIssuedToken(null);
    setTokenCreateOpen(true);
  }

  function closeTokenCreate() {
    if (tokenCreateBusy) return;
    setTokenCreateOpen(false);
    setTokenCreateForm(defaultTokenCreateForm());
    setTokenCreateMessage({ kind: "", text: "" });
    setIssuedToken(null);
  }

  async function handleTokenCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = tokenCreateForm.name.trim();
    const tenantID = tokenCreateForm.tenant_id.trim();
    const projectID = tokenCreateForm.project_id.trim();
    if (!name || !projectID || !tokenCreateForm.group_id || (tokenCreateMode === "admin" && !tenantID)) {
      setTokenCreateMessage({ kind: "error", text: t("tokensCreateValidation") });
      return;
    }
    setTokenCreateBusy(true);
    setTokenCreateMessage({ kind: "pending", text: t("tokensCreating") });
    try {
      const payload: Record<string, unknown> = {
        name,
        project_id: projectID,
        group_id: tokenCreateForm.group_id,
        allowed_models: [],
        allowed_ips: parseTokenList(tokenCreateForm.allowed_ips),
        allowed_domains: parseTokenList(tokenCreateForm.allowed_domains),
      };
      if (tokenCreateMode === "admin") payload.tenant_id = tenantID;
      if (tokenCreateForm.expires_at) {
        payload.expires_at = new Date(tokenCreateForm.expires_at).toISOString();
      }
      const endpoint = tokenCreateMode === "admin"
        ? "/admin/v1/tokens"
        : `/console/v1/tenants/${encodeURIComponent(principal?.tenant_id || "")}/tokens`;
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(payload),
      });
      const result = (await response.json().catch(() => ({}))) as IssuedTokenResponse & { error?: string };
      if (!response.ok) throw new Error(result.error || "token creation failed");
      setIssuedToken(result);
      setTokenCreateMessage({ kind: "success", text: t("tokensCreateSuccess") });
      if (tokenCreateMode === "admin") await refreshTokens(false);
      else await refreshConsoleTokens(false);
    } catch {
      setTokenCreateMessage({ kind: "error", text: t("tokensCreateValidation") });
    } finally {
      setTokenCreateBusy(false);
    }
  }

  async function revokeConsoleToken(token: TokenSummary) {
    if (token.status !== "active" || !principal?.tenant_id) return;
    if (tokenRevokeConfirm !== token.id) {
      setTokenRevokeConfirm(token.id);
      setTokensMessage({ kind: "pending", text: t("tokensRevokeConfirm") });
      return;
    }
    setTokenActionBusy(token.id);
    try {
      const response = await fetch(`/console/v1/tenants/${encodeURIComponent(principal.tenant_id)}/tokens/${encodeURIComponent(token.id)}`, {
        method: "DELETE",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) throw new Error("token revoke failed");
      setTokenRevokeConfirm("");
      await refreshConsoleTokens(false);
      setTokensMessage({ kind: "success", text: t("tokensRevokeSuccess") });
    } catch {
      setTokensMessage({ kind: "error", text: t("tokensUnavailable") });
    } finally {
      setTokenActionBusy("");
    }
  }

  async function revokeAdminToken(token: TokenSummary) {
    if (token.status !== "active") return;
    if (tokenRevokeConfirm !== token.id) {
      setTokenRevokeConfirm(token.id);
      setTokensMessage({ kind: "pending", text: t("tokensRevokeConfirm") });
      return;
    }
    setTokenActionBusy(token.id);
    try {
      const response = await fetch(`/admin/v1/tokens/${encodeURIComponent(token.id)}`, {
        method: "DELETE",
        headers: { Accept: "application/json" },
        credentials: "same-origin",
      });
      if (!response.ok) throw new Error("token revoke failed");
      setTokenRevokeConfirm("");
      await refreshTokens(false);
      setTokensMessage({ kind: "success", text: t("tokensRevokeSuccess") });
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
      const response = await fetch("/admin/v1/prices/sync-official", {
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
    const reasoning = modelPriceForm.reasoning_price_per_million_tokens.trim() ? millionPriceToUnit(modelPriceForm.reasoning_price_per_million_tokens) : "0";
    const components = modelPriceForm.components.map((component) => ({
      component_code: component.component_code,
      unit: component.unit,
      price_per_unit: isTokenPriceUnit(component.unit) ? tokenDisplayToPrice(component.price_per_unit, component.unit) : component.price_per_unit.trim(),
      tiers: component.tiers,
      metadata: component.metadata,
    })).filter((component) => component.component_code && component.price_per_unit);
    if (!modelPriceForm.model_id || modelPriceForm.currency.trim().toUpperCase() !== "USD" || ![input, output, cached, reasoning, ...components.map((component) => component.price_per_unit)].some((value) => value && value !== "0")) {
      setModelPriceFormMessage({ kind: "error", text: t("billingPriceFormInvalid") });
      return;
    }
    setModelPriceFormBusy(true);
    setModelPriceFormMessage({ kind: "pending", text: t("billingPriceSaving") });
    try {
      const response = await fetch("/admin/v1/prices/publish", {
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
      const response = await fetch(
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
      setBillingMessage({ kind: "success", text: t("billingCreditSuccess") });
      setCreditForm((current) => ({ ...current, amount: "", reason: "" }));
    } catch {
      setBillingMessage({ kind: "error", text: t("billingInvalid") });
    } finally {
      setBillingBusy(false);
    }
  }

  function openCreateChannel(provider: "openai" | "anthropic" | "grok" | "gemini" = "openai") {
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

  function setChannelProvider(provider: "openai" | "anthropic" | "grok" | "gemini") {
    setDiscoveredModels([]);
    setModelDiscoveryMessage({ kind: "", text: "" });
    setChannelForm((current) => {
      const currentDefault = defaultProviderBaseURL(current.provider);
      const nextModel = provider === "openai" ? "gpt-5" : provider === "anthropic" ? "claude-sonnet-5" : provider === "grok" ? "grok-4.6" : "gemini-3.7-flash";
      return {
        ...current,
        provider,
        base_url:
          !current.base_url || current.base_url === currentDefault
            ? defaultProviderBaseURL(provider)
            : current.base_url,
        models:
          current.models.length === 0 || current.models.every((model) => !model.model.trim())
            ? [{ id: newFormRowID(), model: nextModel, upstream_model: nextModel, enabled: true }]
            : current.models,
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
      models:
        current.models.length <= 1
          ? current.models
          : current.models.filter((model) => model.id !== rowID),
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
      const response = await fetch("/admin/v1/channels/discover-models", {
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
        throw new Error("model discovery failed");
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
    } catch {
      setDiscoveredModels([]);
      setModelDiscoveryMessage({ kind: "error", text: t("channelsUnavailable") });
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
      (!channelForm.id && !channelForm.api_key.trim()) ||
      models.length === 0
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
        priority: Number(channelForm.priority),
        weight: Number(channelForm.weight),
        models,
      };
      const response = await fetch(
        channelForm.id ? `/admin/v1/channels/${channelForm.id}` : "/admin/v1/channels",
        {
          method: channelForm.id ? "PUT" : "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          credentials: "same-origin",
          body: JSON.stringify(payload),
        }
      );
      if (!response.ok) {
        throw new Error("save failed");
      }
      closeChannelForm();
      await refreshChannels(false);
      setChannelsMessage({ kind: "success", text: t("channelsSaveSuccess") });
    } catch {
      setChannelsMessage({ kind: "error", text: t("channelsUnavailable") });
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
      const response = await fetch(
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
      const response = await fetch(`/admin/v1/channels/${channel.id}`, {
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
    setTokenRevokeConfirm("");
    setConsoleTokenGroups([]);
    setConsoleTokenGroupsBusy(false);
    setUsers([]);
    setUsersMessage({ kind: "", text: "" });
    setUsersBusy(false);
    setUserActionBusy("");
    setConsoleSection("dashboard");
    setUsageStatus(null);
	setConsoleUsageReport(null);
	setConsoleUsageOffset(0);
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
    setSiteSettingsMessage({ kind: "", text: "" });
    setSiteSettingsBusy(false);
    setModelCatalog([]);
    setModelCatalogMessage({ kind: "", text: "" });
    setModelCatalogBusy(false);
    setOfficialPriceSyncBusy(false);
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
    window.location.hash = "#home";
  }

  async function persistSecurity(nextEnabled: boolean) {
    if (!signedIn || audience !== "admin") return;
    setSecurityBusy(true);
    setSecurityMessage({ kind: "pending", text: t("securitySaving") });
    try {
      const response = await fetch("/admin/v1/security/settings", {
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
    <div className="min-h-screen flex flex-col bg-slate-50 dark:bg-slate-950 text-slate-900 dark:text-slate-100 selection:bg-indigo-500/30 selection:text-indigo-200 transition-colors duration-200">
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
      />

      {/* Main View Router */}
      <main className={currentView === "admin" || currentView === "console" || currentView === "models" ? "flex-1 w-full flex flex-col min-w-0" : "flex-1 w-full max-w-[1720px] mx-auto px-4 sm:px-6 lg:px-8"}>
        {!sessionReady ? (
          <div className="flex items-center justify-center min-h-[50vh] text-sm text-slate-500 dark:text-slate-400 gap-2">
            <span className="h-4 w-4 rounded-full border-2 border-indigo-500/30 border-t-indigo-500 animate-spin" />
            <span>{t("restoreChecking")}</span>
          </div>
        ) : currentView === "home" ? (
          <HomeView
            language={language}
            signedIn={signedIn}
            workspaceRoute={audience === "console" ? "#console/dashboard" : "#admin/dashboard"}
            routeTo={routeTo}
            handleSignOut={handleSignOut}
            models={modelCatalog}
          />
        ) : currentView === "models" ? (
          <ModelPlazaView
            language={language}
            models={modelCatalog}
            busy={modelCatalogBusy}
            message={modelCatalogMessage}
            refresh={refreshModelCatalog}
          />
        ) : currentView === "login" ? (
          <LoginView
            language={language}
            audience={audience}
            setAudience={setAudience}
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
          />
        ) : currentView === "register" ? (
          <RegisterView language={language} routeTo={routeTo} onRegistered={handleRegistered} />
        ) : currentView === "reset" ? (
          <ResetPasswordView language={language} token={route.reset_token} routeTo={routeTo} />
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
            revokeToken={revokeConsoleToken}
            revokeConfirm={tokenRevokeConfirm}
            openCreateToken={() => openCreateToken("console")}
            billingAccount={billingAccount}
            billingBusy={billingBusy}
            billingMessage={billingMessage}
            refreshBilling={refreshConsoleBilling}
            usageStatus={usageStatus}
            usageBusy={usageBusy}
            usageMessage={usageMessage}
            refreshUsage={refreshConsoleUsage}
			 consoleUsageReport={consoleUsageReport}
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
            updateTokenGroup={updateTokenGroup}
            openCreateToken={() => openCreateToken("admin")}
            revokeToken={revokeAdminToken}
            tokenRevokeConfirm={tokenRevokeConfirm}
            tokenActionBusy={tokenActionBusy}
            users={users}
            usersBusy={usersBusy}
            usersMessage={usersMessage}
            refreshUsers={refreshUsers}
            openCreateUser={openCreateUser}
            openEditUser={openEditUser}
            updateUserStatus={updateUserStatus}
            userActionBusy={userActionBusy}
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
			 operationsSnapshot={operationsSnapshot}
			 operationsBusy={operationsBusy}
             refreshOperations={refreshOperations}
			 auditReport={auditReport}
			 auditBusy={auditBusy}
			 auditMessage={auditMessage}
			 refreshAudit={refreshAudit}
           />
        )}
      </main>

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
        projectIDs={tokenCreateMode === "console" ? principal?.project_ids || [] : []}
        groups={tokenCreateMode === "console" ? consoleTokenGroups : groups.map((group) => ({ id: group.id, code: group.code, name: group.name, multiplier: group.multiplier, billing_type: group.billing_type, status: group.status, models: group.models || [] }))}
        groupsBusy={tokenCreateMode === "console" ? consoleTokenGroupsBusy : groupsBusy}
        busy={tokenCreateBusy}
        message={tokenCreateMessage}
        issuedToken={issuedToken}
        onClose={closeTokenCreate}
        onSubmit={handleTokenCreate}
      />

      <UserModal
        open={userFormOpen}
        mode={userFormMode}
        form={userForm}
        setForm={setUserForm}
        tenants={userTenants}
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
