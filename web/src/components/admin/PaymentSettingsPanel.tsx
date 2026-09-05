import React, { useEffect, useMemo, useState } from "react";
import { BookOpen, CheckCircle2, Copy, CreditCard, ExternalLink, Info, Plus, Save, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Language, LoginMessage, PaymentProviderConfig, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";

interface Props {
  language: Language;
  configs: PaymentProviderConfig[];
  busy: boolean;
  message: LoginMessage;
  save: (provider: PaymentProviderConfig["provider"], enabled: boolean, values: Record<string, string>, clear: string[]) => Promise<void>;
  canUpdate: boolean;
}

type Provider = PaymentProviderConfig["provider"];
type Field = { key: string; label: TranslationKey; secret?: boolean; multiline?: boolean; hint?: TranslationKey };

const fields: Record<Provider, Field[]> = {
  wechat: [
    { key: "recharge_rate", label: "paymentRechargeRate", hint: "paymentRechargeRateHint" },
    { key: "app_id", label: "paymentWechatAppID" },
    { key: "mch_id", label: "paymentWechatMchID" },
    { key: "serial_no", label: "paymentWechatSerial" },
    { key: "platform_certificate_serial_no", label: "paymentWechatPlatformSerial" },
    { key: "private_key_pem", label: "paymentPrivateKey", secret: true, multiline: true },
    { key: "api_v3_key", label: "paymentWechatAPIv3Key", secret: true },
    { key: "platform_certificate_pem", label: "paymentWechatPlatformCertificate", secret: true, multiline: true },
    { key: "notify_url", label: "paymentNotifyURL" },
    { key: "api_base_url", label: "paymentAPIBaseURL" },
  ],
  alipay: [
    { key: "recharge_rate", label: "paymentRechargeRate", hint: "paymentRechargeRateHint" },
    { key: "app_id", label: "paymentAlipayAppID" },
    { key: "seller_id", label: "paymentAlipaySellerID" },
    { key: "private_key_pem", label: "paymentPrivateKey", secret: true, multiline: true },
    { key: "alipay_public_key_pem", label: "paymentAlipayPublicKey", secret: true, multiline: true },
    { key: "notify_url", label: "paymentNotifyURL" },
    { key: "gateway", label: "paymentGateway" },
  ],
  stripe: [
    { key: "recharge_rate", label: "paymentRechargeRate", hint: "paymentRechargeRateHint" },
    { key: "secret_key", label: "paymentStripeSecretKey", secret: true },
    { key: "publishable_key", label: "paymentStripePublishableKey" },
    { key: "webhook_secret", label: "paymentStripeWebhookSecret", secret: true },
    { key: "api_base_url", label: "paymentAPIBaseURL", hint: "paymentStripeAPIBaseURLHint" },
  ],
  paypal: [
    { key: "recharge_rate", label: "paymentRechargeRate", hint: "paymentRechargeRateHint" },
    { key: "client_id", label: "paymentPayPalClientID" },
    { key: "client_secret", label: "paymentPayPalClientSecret", secret: true },
    { key: "webhook_id", label: "paymentPayPalWebhookID" },
    { key: "environment", label: "paymentPayPalEnvironment" },
  ],
};

const providerLabels: Record<Provider, TranslationKey> = {
  wechat: "paymentWechat",
  alipay: "paymentAlipay",
  stripe: "paymentStripe",
  paypal: "paymentPayPal",
};

const stripeMethodOptions: Array<{ value: string; label: TranslationKey }> = [
  { value: "card", label: "paymentStripeCard" },
  { value: "alipay", label: "paymentStripeAlipay" },
  { value: "wechat_pay", label: "paymentStripeWechatPay" },
];

export function PaymentSettingsPanel({ language, configs, busy, message, save, canUpdate }: Props) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [provider, setProvider] = useState<Provider>("wechat");
  const [enabled, setEnabled] = useState(false);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [clear, setClear] = useState<string[]>([]);
  const current = useMemo(() => configs.find((item) => item.provider === provider), [configs, provider]);

  useEffect(() => {
    setEnabled(Boolean(current?.enabled));
    setDraft(Object.fromEntries(Object.entries(current?.values || {}).filter(([key]) => !key.endsWith("_configured"))));
    setClear([]);
  }, [current, provider]);

  const providerFields = fields[provider];
  const webhookURL = current?.webhook_url ? absoluteURL(current.webhook_url) : "";
  const stripeSuccessURL = provider === "stripe" ? `${window.location.origin}/?payment_order_id={order_id}&provider=stripe#console/billing` : "";
  const stripeCancelURL = provider === "stripe" ? `${window.location.origin}/?payment_order_id={order_id}&provider=stripe&cancelled=1#console/billing` : "";

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    await save(provider, enabled, draft, clear);
  }

  function updateStripeMethod(method: string, checked: boolean) {
    const selected = parseStripeMethods(draft.payment_method_types);
    const next = checked ? [...new Set([...selected, method])] : selected.filter((item) => item !== method);
    setDraft((value) => ({ ...value, payment_method_types: next.join(",") }));
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap gap-2 border-b border-slate-200 pb-3 dark:border-slate-800">
        {(Object.keys(fields) as Provider[]).map((item) => (
          <Button key={item} type="button" size="sm" variant={provider === item ? "default" : "outline"} onClick={() => setProvider(item)}>
            {t(providerLabels[item])}
          </Button>
        ))}
      </div>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.45fr)_minmax(320px,0.75fr)]">
      <Card className="border-cyan-500/20">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg"><CreditCard className="h-5 w-5 text-cyan-600" />{t("paymentSettingsTitle")}</CardTitle>
          <CardDescription>{t("paymentSettingsDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <PaymentSetupGuide provider={provider} t={t} />
          <form className="space-y-5" onSubmit={(event) => void submit(event)}>
            <div className="flex items-center justify-between rounded-xl border border-cyan-500/20 bg-cyan-50/60 p-4 dark:bg-cyan-500/10">
              <div>
                <div className="text-sm font-semibold text-slate-900 dark:text-white">{t(providerLabels[provider])}</div>
                <div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{current?.configured ? t("paymentConfigured") : t("paymentNotConfigured")}</div>
              </div>
              <Switch checked={enabled} onCheckedChange={setEnabled} disabled={busy || !canUpdate} />
            </div>

            {webhookURL ? <EndpointRow label={t("paymentWebhookURL")} value={webhookURL} copyLabel={t("paymentCopyURL")} /> : null}
            {provider === "stripe" ? <StripeEndpointInfo t={t} successURL={stripeSuccessURL} cancelURL={stripeCancelURL} /> : null}

            <div className="grid gap-4 sm:grid-cols-2">
              {providerFields.map((field) => {
                const configured = current?.values?.[field.key + "_configured"] === "true";
                const isCleared = clear.includes(field.key);
                return (
                  <div key={field.key} className={field.multiline ? "space-y-2 sm:col-span-2" : "space-y-2"}>
                    <Label htmlFor={`payment-${provider}-${field.key}`}>{t(field.label)}</Label>
                    {field.hint ? <p className="text-xs text-slate-500 dark:text-slate-400">{t(field.hint)}</p> : null}
                    {field.key === "recharge_presets" ? (
                      <RechargePresetsEditor id={`payment-${provider}-${field.key}`} value={draft[field.key] || ""} onChange={(value) => setDraft((current) => ({ ...current, [field.key]: value }))} disabled={busy || !canUpdate} t={t} />
                    ) : field.multiline ? (
                      <textarea id={`payment-${provider}-${field.key}`} value={draft[field.key] || ""} onChange={(event) => setDraft((value) => ({ ...value, [field.key]: event.target.value }))} rows={6} className="w-full rounded-xl border border-slate-200 bg-white p-3 font-mono text-xs text-slate-900 outline-none dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100" placeholder={configured ? t("paymentSecretKeep") : ""} disabled={busy || !canUpdate || isCleared} />
                    ) : (
                      <Input id={`payment-${provider}-${field.key}`} type={field.secret ? "password" : "text"} value={draft[field.key] || ""} onChange={(event) => setDraft((value) => ({ ...value, [field.key]: event.target.value }))} placeholder={field.secret && configured ? t("paymentSecretKeep") : ""} disabled={busy || !canUpdate || isCleared} />
                    )}
                    {field.secret && configured ? <label className="flex items-center gap-2 text-xs text-slate-500"><input type="checkbox" checked={isCleared} onChange={(event) => setClear((value) => event.target.checked ? [...value, field.key] : value.filter((item) => item !== field.key))} disabled={busy || !canUpdate} />{t("paymentSecretClear")}</label> : null}
                  </div>
                );
              })}
            </div>

            {provider === "stripe" ? <StripePaymentMethods t={t} value={draft.payment_method_types || ""} update={updateStripeMethod} disabled={busy || !canUpdate} /> : null}
            {message.text ? <div className="rounded-xl border border-amber-500/30 bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-500/10 dark:text-amber-200">{message.text}</div> : null}
            <div className="flex justify-end"><Button type="submit" disabled={busy || !canUpdate} className="gap-2"><Save className="h-4 w-4" />{busy ? t("paymentSaving") : t("paymentSave")}</Button></div>
          </form>
          <div className="mt-5 flex flex-wrap gap-2 text-xs text-slate-500 dark:text-slate-400"><Badge variant="muted">{t("paymentCallbackOnly")}</Badge><span>{t("paymentSecurityHint")}</span></div>
        </CardContent>
      </Card>
      <RechargePackagePanel provider={provider} value={draft.recharge_presets || ""} onChange={(value) => setDraft((current) => ({ ...current, recharge_presets: value }))} disabled={busy || !canUpdate} t={t} />
      </div>
    </div>
  );
}

function RechargePackagePanel({ provider, value, onChange, disabled, t }: { provider: Provider; value: string; onChange: (value: string) => void; disabled: boolean; t: (key: TranslationKey) => string }) {
  return <Card className="h-fit border-emerald-500/25 bg-emerald-50/35 shadow-sm dark:border-emerald-400/20 dark:bg-emerald-500/[0.06]"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><CreditCard className="h-5 w-5 text-emerald-600 dark:text-emerald-300" />{t("paymentRechargePackagesTitle")}</CardTitle><CardDescription>{t("paymentRechargePackagesDescription")}</CardDescription></CardHeader><CardContent className="space-y-4"><div className="flex items-center justify-between rounded-lg border border-emerald-500/20 bg-white/75 px-3 py-2 text-xs dark:bg-slate-950/40"><span className="text-slate-500">{t("paymentRechargePackagesProvider")}</span><strong className="text-slate-900 dark:text-white">{t(providerLabels[provider])}</strong></div><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("paymentRechargePresets")}</div><p className="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">{t("paymentRechargePresetsHint")}</p></div><RechargePresetsEditor id={`payment-${provider}-recharge-presets`} value={value} onChange={onChange} disabled={disabled} t={t} /><p className="text-xs leading-5 text-slate-500 dark:text-slate-400">{t("paymentRechargePackagesSaveHint")}</p></CardContent></Card>;
}

function RechargePresetsEditor({ id, value, onChange, disabled, t }: { id: string; value: string; onChange: (value: string) => void; disabled: boolean; t: (key: TranslationKey) => string }) {
  const [input, setInput] = useState("");
  const presets = value.split(",").map((item) => item.trim()).filter(Boolean);
  function add() {
    const next = input.trim();
    if (!next || presets.includes(next)) return;
    onChange([...presets, next].join(","));
    setInput("");
  }
  function remove(preset: string) {
    onChange(presets.filter((item) => item !== preset).join(","));
  }
  return <div className="space-y-2"><div className="flex gap-2"><Input id={id} value={input} onChange={(event) => setInput(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); add(); } }} placeholder={t("paymentRechargePresetPlaceholder")} inputMode="decimal" disabled={disabled} /><Button type="button" variant="outline" onClick={add} disabled={disabled || !input.trim()} className="shrink-0 gap-1.5"><Plus className="h-3.5 w-3.5" />{t("paymentRechargePresetAdd")}</Button></div>{presets.length > 0 ? <div className="flex flex-wrap gap-2">{presets.map((preset) => <span key={preset} className="inline-flex items-center gap-1 rounded-full border border-emerald-500/25 bg-emerald-500/10 px-2.5 py-1 text-xs font-semibold text-emerald-700 dark:text-emerald-300">{preset}<button type="button" onClick={() => remove(preset)} disabled={disabled} title={t("paymentRechargePresetRemove")} aria-label={t("paymentRechargePresetRemove")} className="rounded-full p-0.5 hover:bg-emerald-500/20 disabled:opacity-50"><X className="h-3 w-3" /></button></span>)}</div> : null}</div>;
}

function PaymentSetupGuide({ provider, t }: { provider: Provider; t: (key: TranslationKey) => string }) {
  const steps: Record<Provider, TranslationKey[]> = {
    wechat: ["paymentGuideWechatStep1", "paymentGuideWechatStep2"],
    alipay: ["paymentGuideAlipayStep1", "paymentGuideAlipayStep2"],
    stripe: ["paymentGuideStripeStep1", "paymentGuideStripeStep2"],
    paypal: ["paymentGuidePayPalStep1", "paymentGuidePayPalStep2"],
  };
  return (
    <section className="mb-5 border-b border-slate-200 pb-5 dark:border-slate-800" aria-labelledby="payment-setup-guide-title">
      <div className="flex items-start gap-2">
        <BookOpen className="mt-0.5 h-4 w-4 shrink-0 text-indigo-600 dark:text-indigo-300" />
        <div>
          <h3 id="payment-setup-guide-title" className="text-sm font-semibold text-slate-900 dark:text-white">{t("paymentGuideTitle")}</h3>
          <p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">{t("paymentGuideDescription")}</p>
        </div>
      </div>
      <div className="mt-3 grid gap-2 sm:grid-cols-2">
        {steps[provider].map((step) => (
          <div key={step} className="flex items-start gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs leading-5 text-slate-700 dark:border-slate-700 dark:bg-slate-950/40 dark:text-slate-300">
            <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-600 dark:text-emerald-400" />
            <span>{t(step)}</span>
          </div>
        ))}
      </div>
      {provider === "stripe" ? (
        <div className="mt-3 rounded-lg border border-indigo-500/20 bg-indigo-50/60 p-3 dark:bg-indigo-500/10">
          <div className="text-xs font-semibold text-indigo-900 dark:text-indigo-100">{t("paymentGuideStripeEventsTitle")}</div>
          <p className="mt-1 text-xs leading-5 text-indigo-900/75 dark:text-indigo-100/75">{t("paymentGuideStripeEventsDescription")}</p>
          <div className="mt-2 grid gap-1 text-xs text-indigo-950 dark:text-indigo-50">
            <code>checkout.session.completed</code>
            <code>checkout.session.async_payment_succeeded</code>
          </div>
          <p className="mt-2 text-xs leading-5 text-indigo-900/75 dark:text-indigo-100/75">{t("paymentGuideStripeEventsNote")}</p>
        </div>
      ) : null}
    </section>
  );
}

function StripeEndpointInfo({ t, successURL, cancelURL }: { t: (key: TranslationKey) => string; successURL: string; cancelURL: string }) {
  return (
    <div className="space-y-3 rounded-xl border border-indigo-500/20 bg-indigo-50/60 p-4 dark:bg-indigo-500/10">
      <div className="flex items-start gap-2"><Info className="mt-0.5 h-4 w-4 shrink-0 text-indigo-600 dark:text-indigo-300" /><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("paymentStripeEndpointTitle")}</div><p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">{t("paymentStripeEndpointHint")}</p></div></div>
      <div className="space-y-2"><EndpointRow label={t("paymentStripeSuccessURL")} value={successURL} copyLabel={t("paymentCopyURL")} /><EndpointRow label={t("paymentStripeCancelURL")} value={cancelURL} copyLabel={t("paymentCopyURL")} /></div>
      <div className="flex items-start gap-2 text-xs leading-5 text-slate-600 dark:text-slate-300"><ExternalLink className="mt-0.5 h-3.5 w-3.5 shrink-0" />{t("paymentStripeDashboardHint")}</div>
    </div>
  );
}

function StripePaymentMethods({ t, value, update, disabled }: { t: (key: TranslationKey) => string; value: string; update: (method: string, checked: boolean) => void; disabled: boolean }) {
  const selected = parseStripeMethods(value);
  return (
    <div className="space-y-3 rounded-xl border border-emerald-500/20 bg-emerald-50/50 p-4 dark:bg-emerald-500/10">
      <div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("paymentStripePaymentMethods")}</div><p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">{t("paymentStripePaymentMethodsHint")}</p></div>
      <div className="grid gap-2 sm:grid-cols-3">
        {stripeMethodOptions.map((method) => <label key={method.value} className="flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-700 dark:border-slate-700 dark:bg-slate-950/50 dark:text-slate-200"><input type="checkbox" checked={selected.includes(method.value)} onChange={(event) => update(method.value, event.target.checked)} disabled={disabled} />{t(method.label)}</label>)}
      </div>
      <p className="text-xs leading-5 text-slate-500 dark:text-slate-400">{selected.length === 0 ? t("paymentStripeAutomatic") : t("paymentStripeWallets")}</p>
    </div>
  );
}

function EndpointRow({ label, value, copyLabel }: { label: string; value: string; copyLabel: string }) {
  return <div className="flex min-w-0 items-center gap-2"><span className="w-28 shrink-0 text-xs font-semibold text-slate-600 dark:text-slate-300">{label}</span><code className="min-w-0 flex-1 break-all rounded-lg border border-slate-200 bg-white px-3 py-2 text-[11px] text-slate-700 dark:border-slate-700 dark:bg-slate-950/50 dark:text-slate-200">{value}</code><Button type="button" variant="outline" size="icon" onClick={() => void navigator.clipboard?.writeText(value)} title={copyLabel} aria-label={copyLabel}><Copy className="h-4 w-4" /></Button></div>;
}

function parseStripeMethods(value: string | undefined) {
  return (value || "").split(",").map((item) => item.trim().toLowerCase()).filter(Boolean);
}

function absoluteURL(value: string) {
  try {
    return new URL(value, window.location.origin).toString();
  } catch {
    return value;
  }
}
