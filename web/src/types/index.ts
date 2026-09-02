import type { translations } from "../locales/translations";

export type Theme = "light" | "dark";
export type Language = "zh" | "en";
export type Audience = "admin" | "console";
export type TranslationKey = keyof (typeof translations)["zh"];
export type AdminSection = "dashboard" | "ops" | "model-status" | "users" | "roles" | "groups" | "tokens" | "channels" | "billing" | "finance" | "usage" | "audit" | "settings";
export type ConsoleSection = "dashboard" | "model-status" | "usage" | "projects" | "tokens" | "billing" | "profile" | "docs";
export type View = "home" | "models" | "login" | "admin-login" | "register" | "reset" | "verify-email" | "admin" | "console" | "not-found";

export interface Principal {
  id: string;
  type?: string;
  audience?: Audience;
  email?: string;
  display_name?: string;
  roles?: string[];
  permissions?: string[];
  tenant_id?: string;
  project_ids?: string[];
  project_roles?: Record<string, string>;
}

export interface UserSummary {
  id: string;
  email: string;
  display_name: string;
  status: "active" | "locked" | "disabled" | "pending";
  platform_roles: string[];
  tenant_count: number;
  tenant_names: string[];
  tenant_ids: string[];
  created_at: string;
  last_login_at?: string;
}

export interface PlatformPermission {
  id: string;
  resource: string;
  action: string;
  name: string;
}

export interface PlatformRole {
  id: string;
  code: string;
  name: string;
  status: "active" | "disabled";
  member_count: number;
  permissions: string[];
  created_at: string;
}

export interface PlatformRoleFormState {
  id: string;
  code: string;
  name: string;
  status: "active" | "disabled";
  permissions: string[];
}

export interface TenantSummary {
  id: string;
  name: string;
  slug: string;
}

export interface TenantMember {
  tenant_id: string;
  user_id: string;
  email: string;
  display_name: string;
  role: "tenant_owner" | "tenant_admin" | "developer" | "viewer";
  status: "active" | "suspended";
  project_count: number;
  created_at: string;
}

export interface ProjectSummary {
  id: string;
  tenant_id: string;
  name: string;
  slug: string;
  status: "active" | "disabled";
  created_by: string;
  member_count: number;
  created_at: string;
  updated_at: string;
}

export interface ProjectMember {
  project_id: string;
  user_id: string;
  email: string;
  display_name: string;
  role: "project_admin" | "developer" | "viewer";
  created_at: string;
}

export interface ProjectFormState {
  id: string;
  name: string;
  slug: string;
  status: "active" | "disabled";
}

export interface UserAdminFormState {
  id: string;
  email: string;
  display_name: string;
  password: string;
}

export interface SecuritySettings {
  admin_mfa_enabled: boolean;
  updated_at: string;
  updated_by: string;
}

export interface SiteSettings {
  site_name: string;
  site_logo_url: string;
  site_favicon_url: string;
}

export interface APIEndpoint {
  id: string;
  name: string;
  base_url: string;
  openai_base_url: string;
  anthropic_base_url: string;
  enabled: boolean;
  sort_order: number;
  created_at?: string;
  updated_at?: string;
}

export interface PublicAPIEndpoint {
  name: string;
  base_url: string;
  openai_base_url: string;
  anthropic_base_url: string;
}

export interface APIEndpointFormState {
  id: string;
  name: string;
  base_url: string;
  enabled: boolean;
}

export interface SystemSettings extends SecuritySettings, SiteSettings {
  smtp_addr: string;
  smtp_from: string;
  smtp_username: string;
  smtp_password_configured: boolean;
  public_base_url: string;
}

export interface SMTPSettingsForm {
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password: string;
  smtp_password_clear: boolean;
  smtp_from_email: string;
  smtp_from_name: string;
  smtp_tls: boolean;
  public_base_url: string;
}

export interface EmailSettings {
  email_enabled: boolean;
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password_configured: boolean;
  smtp_from_email: string;
  smtp_from_name: string;
  smtp_tls: boolean;
  smtp_configured: boolean;
  public_base_url: string;
  balance_threshold: string;
  recharge_url: string;
  updated_at?: string;
  updated_by?: string;
}

export interface FeatureSettings {
  email_enabled: boolean;
  registration_enabled: boolean;
  model_status_enabled: boolean;
  totp_enabled: boolean;
  step_up_channel_model_enabled: boolean;
  step_up_group_enabled: boolean;
  step_up_token_enabled: boolean;
  step_up_user_enabled: boolean;
  step_up_role_enabled: boolean;
  step_up_billing_enabled: boolean;
  step_up_system_enabled: boolean;
  email_verification_enabled: boolean;
  email_password_reset_enabled: boolean;
  email_subscription_enabled: boolean;
  email_low_balance_alert_enabled: boolean;
  email_recharge_success_enabled: boolean;
  email_usage_limit_alert_enabled: boolean;
  email_content_audit_enabled: boolean;
  email_account_disabled_enabled: boolean;
  email_cyber_policy_enabled: boolean;
  email_operations_enabled: boolean;
  balance_threshold: string;
  recharge_url: string;
  updated_at?: string;
  updated_by?: string;
}

export interface PublicFeatureSettings {
  registration_enabled: boolean;
  model_status_enabled: boolean;
  totp_enabled: boolean;
}

export interface EmailTemplate {
  id: string;
  event_code: string;
  language: "zh" | "en";
  subject: string;
  html_body: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface EmailTemplateFormState {
  id: string;
  event_code: string;
  language: "zh" | "en";
  subject: string;
  html_body: string;
  enabled: boolean;
}

export interface ChannelModel {
  model: string;
  provider: string;
  upstream_model: string;
  enabled: boolean;
  health_status: string;
}

export interface DiscoveredModel {
  id: string;
  display_name: string;
  provider: string;
}

export interface PublicModelPricing {
  currency: string;
  input_price_per_unit: string;
  output_price_per_unit: string;
  cached_input_price_per_unit: string;
  reasoning_price_per_unit: string;
  minimum_charge: string;
  unit: "per_1m_tokens";
  input_price_per_million_tokens: string;
  output_price_per_million_tokens: string;
  cached_input_price_per_million_tokens: string;
  reasoning_price_per_million_tokens: string;
  source: string;
  source_url?: string;
  updated_at?: string;
  components?: PriceComponent[];
  platform_prices?: PublicPlatformModelPrice[];
}

export interface PublicPlatformModelPrice {
  group_id: string;
  group_code: string;
  group_name: string;
  multiplier: string;
  billing_type: "prepaid" | "free";
  input_price_per_million_tokens: string;
  output_price_per_million_tokens: string;
  cached_input_price_per_million_tokens: string;
  reasoning_price_per_million_tokens: string;
  components?: PriceComponent[];
}

export interface PublicModelSummary {
  id: string;
  provider: string;
  name: string;
  display_name: string;
  protocol_family: string;
  category: "text" | "image" | "video" | "audio" | "embedding";
  capabilities: Record<string, unknown>;
  channel_count: number;
  active_channel_count: number;
  available: boolean;
  pricing?: PublicModelPricing;
}

export interface ChannelSummary {
  id: string;
  name: string;
  provider: string;
  base_url: string;
  credential_ref: string;
  credential_mode: string;
  credential_preview: string;
  has_credential: boolean;
  status: string;
  priority: number;
  weight: number;
  consecutive_failures?: number;
  auto_disabled_until?: string;
  last_failure_status?: number;
  models: ChannelModel[];
}

export interface GroupChannelSummary {
  id: string;
  name: string;
  provider: string;
  status: string;
}

export interface GroupSummary {
  id: string;
  code: string;
  name: string;
  description: string;
  status: "active" | "disabled";
  multiplier: string;
  rpm_limit: number;
  billing_type: "prepaid" | "free";
  priority: number;
  channels: GroupChannelSummary[];
  models: string[];
}

export type ModelRouteStatus = "normal" | "pending" | "degraded" | "unavailable" | "disabled";

export interface ModelStatus {
  model: string;
  provider: string;
  status: ModelRouteStatus;
  total_routes: number;
  available_routes: number;
  observed_routes: number;
  consecutive_failures: number;
  last_success_at?: string;
  last_failure_at?: string;
  last_latency_ms: number;
  availability_7d: number;
  request_count_7d: number;
  recent_statuses?: string[];
  last_request_at?: string;
  last_request_status?: string;
  last_failure_reason?: string;
}

export interface ModelStatusGroup {
  group_id: string;
  group_code: string;
  group_name: string;
  status: ModelRouteStatus;
  group_status: "active" | "disabled";
  multiplier: string;
  rpm_limit: number;
  billing_type: "prepaid" | "free";
  monitor_id?: string;
  monitor_name?: string;
  monitor_mode?: "passive" | "active";
  selection_mode?: "all" | "selected";
  probe_interval_seconds?: number;
  last_probe_started_at?: string;
  last_probe_finished_at?: string;
  last_probe_status?: "success" | "failed" | "skipped" | "";
  last_probe_error?: string;
  models: ModelStatus[];
  updated_at: string;
}

export interface ModelStatusReport {
  updated_at: string;
  groups: ModelStatusGroup[];
}

export type ModelMonitorSelectionMode = "all" | "selected";
export type ModelMonitorMode = "passive" | "active";

export interface ModelMonitor {
  id: string;
  group_id: string;
  group_code: string;
  group_name: string;
  name: string;
  selection_mode: ModelMonitorSelectionMode;
  mode: ModelMonitorMode;
  probe_interval_seconds: number;
  enabled: boolean;
  model_names: string[];
  available_models: string[];
  last_probe_started_at?: string;
  last_probe_finished_at?: string;
  last_probe_status?: "success" | "failed" | "skipped" | "";
  last_probe_error?: string;
  created_at: string;
  updated_at: string;
}

export interface ModelMonitorFormState {
  id: string;
  group_id: string;
  name: string;
  selection_mode: ModelMonitorSelectionMode;
  model_names: string[];
  mode: ModelMonitorMode;
  probe_interval_seconds: number;
  enabled: boolean;
}

export interface GroupFormState {
  id: string;
  code: string;
  name: string;
  description: string;
  status: "active" | "disabled";
  multiplier: string;
  rpm_limit: number;
  billing_type: "prepaid" | "free";
  priority: number;
  channel_ids: string[];
}

export interface TokenSummary {
  id: string;
  name: string;
  token_prefix: string;
  tenant_id: string;
  project_id: string;
  group_id?: string;
  group_code?: string;
  network_allowlist_enabled?: boolean;
  allowed_ip_count?: number;
  allowed_domain_count?: number;
  status: "active" | "revoked" | "expired";
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
}

export interface TokenCreateFormState {
  tenant_id: string;
  project_id: string;
  name: string;
  expires_at: string;
  group_id: string;
  allowed_ips: string;
  allowed_domains: string;
}

export interface TokenGroupOption {
  id: string;
  code: string;
  name: string;
  multiplier: string;
  billing_type: "prepaid" | "free";
  status: "active" | "disabled";
  models: string[];
}

export interface IssuedTokenResponse {
  id: string;
  token: string;
  token_prefix: string;
  expires_at?: string;
  warning: string;
}

export interface ChannelFormModel {
  id: string;
  model: string;
  upstream_model: string;
  enabled: boolean;
}

export interface ChannelFormState {
  id: string;
  name: string;
  provider: "openai" | "anthropic" | "grok" | "gemini" | "volcengine";
  base_url: string;
  api_key: string;
  status: "active" | "disabled" | "draining";
  priority: number;
  weight: number;
  models: ChannelFormModel[];
}

export interface PriceVersionSummary {
  id: string;
  scope_type: string;
  scope_id?: string;
  model_id: string;
  provider: string;
  model: string;
  currency: string;
  input_price_per_unit: string;
  output_price_per_unit: string;
  cached_input_price_per_unit: string;
  reasoning_price_per_unit: string;
  minimum_charge: string;
  version: number;
  effective_from: string;
  effective_to?: string;
  status: string;
  components?: PriceComponent[];
}

export interface PriceComponent {
  component_code: string;
  unit: string;
  price_per_unit: string;
  tiers?: unknown;
  metadata?: unknown;
}

export interface PriceComponentFormState {
  component_code: string;
  unit: string;
  price_per_unit: string;
  tiers?: unknown;
  metadata?: unknown;
}

export interface PriceMatrixSummary {
  model_id: string;
  provider: string;
  model: string;
  currency: string;
  input_price_per_million_tokens: string;
  output_price_per_million_tokens: string;
  cached_input_price_per_million_tokens: string;
  reasoning_price_per_million_tokens: string;
  source: "manual" | "litellm" | "unconfigured";
  source_url?: string;
  updated_at?: string;
  components?: PriceComponent[];
}

export interface BillingAccount {
  id: string;
  tenant_id: string;
  currency: string;
  balance: string;
  status: string;
}

export interface OperationsSnapshot {
  users: number;
  tenants: number;
  channels: number;
  active_channels: number;
  groups: number;
  active_groups: number;
  tokens: number;
  active_tokens: number;
  requests_24h: number;
  failed_requests_24h: number;
  spend_24h: string;
  average_latency_ms: number;
  collected_at: string;
}

export interface AuditRecord {
  id: string;
  request_id: string;
  actor_type: string;
  actor_id?: string;
  tenant_id?: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  result: "success" | "denied" | "failed";
  reason?: string;
  created_at: string;
}

export interface AuditReport {
  records: AuditRecord[];
  total: number;
  limit: number;
  offset: number;
}

export interface UsageRecord {
  id: string;
  request_id: string;
  token_id: string;
  token_name: string;
  token_prefix: string;
  tenant_id: string;
  tenant_name: string;
  model_id: string;
  provider: string;
  model: string;
  reasoning_effort: string;
  endpoint: string;
  client_ip: string;
  group_id: string;
  group_code: string;
  group_name: string;
  request_type: string;
  billing_type: string;
  status: string;
  input_tokens: number;
  output_tokens: number;
  cached_input_tokens: number;
  reasoning_tokens: number;
  total_tokens: number;
  cost: string;
  estimated_cost: string;
  currency: string;
  latency_ms: number;
  started_at: string;
  finished_at?: string;
  created_at: string;
  usage_metrics?: Record<string, string>;
  charge_breakdown?: Array<{ component_code: string; unit: string; quantity: string; price_per_unit: string; amount: string }>;
  price_snapshot?: Record<string, unknown>;
}

export interface UsageSummary {
  total_records: number;
  input_tokens: number;
  output_tokens: number;
  cached_input_tokens: number;
  reasoning_tokens: number;
  total_tokens: number;
  total_cost: string;
}

export interface UsageReport {
  records: UsageRecord[];
  summary: UsageSummary;
  limit: number;
  offset: number;
}

export interface FinanceCurrencySummary {
  currency: string;
  customer_count: number;
  remaining_balance: string;
  total_consumed: string;
  total_topups: string;
  request_count: number;
}

export interface FinanceAccount {
  tenant_id: string;
  tenant_name: string;
  tenant_slug: string;
  currency: string;
  balance: string;
  total_consumed: string;
  total_topups: string;
  request_count: number;
  last_usage_at?: string;
}

export interface FinanceTransaction {
  id: string;
  transaction_type: string;
  direction: string;
  amount: string;
  currency: string;
  tenant_id: string;
  tenant_name: string;
  reference_type: string;
  reference_id: string;
  model: string;
  token_name: string;
  description: string;
  created_at: string;
  usage_metrics?: Record<string, string>;
  charge_breakdown?: Array<{ component_code: string; unit: string; quantity: string; price_per_unit: string; amount: string }>;
  price_snapshot?: Record<string, unknown>;
}

export interface FinanceReport {
  summaries: FinanceCurrencySummary[];
  accounts: FinanceAccount[];
  transactions: FinanceTransaction[];
  total_accounts: number;
  total_transactions: number;
  limit: number;
  offset: number;
}

export interface ConsoleProfile {
  id: string;
  email: string;
  display_name: string;
  status: string;
  created_at: string;
  last_login_at?: string;
  tenant_id?: string;
  roles?: string[];
  project_ids?: string[];
  project_roles?: Record<string, string>;
}

export interface ProfileFormState {
  display_name: string;
}

export interface EmailFormState {
  email: string;
  current_password: string;
}

export interface PasswordFormState {
  current_password: string;
  new_password: string;
  confirm_password: string;
}

export interface MFAStatus {
  enabled: boolean;
  enrolled_at?: string;
}

export interface MFAEnrollment {
  enrollment_id: string;
  secret: string;
  otpauth_url: string;
  expires_at: string;
}

export interface ModelPriceFormState {
  model_id: string;
  provider: string;
  model: string;
  currency: string;
  source: "manual" | "litellm" | "unconfigured";
  input_price_per_million_tokens: string;
  output_price_per_million_tokens: string;
  cached_input_price_per_million_tokens: string;
  reasoning_price_per_million_tokens: string;
  components: PriceComponentFormState[];
}

export interface CreditFormState {
  tenant_id: string;
  currency: string;
  amount: string;
  reason: string;
}

export interface LoginMessage {
  kind: "pending" | "success" | "error" | "";
  text: string;
}

export interface ConsoleUsageStatus {
  tenant_id: string;
  status: string;
}

export interface SectionRoute {
  view: View;
  section: AdminSection;
  console_section?: ConsoleSection;
  reset_token?: string;
  verification_token?: string;
}
