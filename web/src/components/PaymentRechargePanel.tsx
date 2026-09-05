import React, { useEffect, useRef, useState } from "react";
import { BadgeDollarSign, CheckCircle2, Copy, ExternalLink, QrCode, RefreshCw } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { loadStripe } from "@stripe/stripe-js";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { BillingAccount, Language, LoginMessage, PaymentOrder, PaymentRechargePackage, PublicPaymentProvider, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn, formatDecimalWithoutTrailingZeros } from "@/lib/utils";

interface Props {
  language: Language;
  account: BillingAccount | null;
  providers: PublicPaymentProvider[];
  busy: boolean;
  message: LoginMessage;
  order: PaymentOrder | null;
  createOrder: (provider: PaymentOrder["provider"], amount: string, currency: string, packageID?: string) => Promise<void>;
  refreshOrder: () => Promise<void>;
  capturePayPal: () => Promise<void>;
}

const providerLabels: Record<PaymentOrder["provider"], TranslationKey> = {
  wechat: "paymentWechat",
  alipay: "paymentAlipay",
  stripe: "paymentStripe",
  paypal: "paymentPayPal",
};

export function PaymentRechargePanel({ language, account, providers, busy, message, order, createOrder, refreshOrder, capturePayPal }: Props) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [provider, setProvider] = useState<PaymentOrder["provider"]>("stripe");
  const [amount, setAmount] = useState("");
  const [packageID, setPackageID] = useState("");
  const currency = account?.currency || "USD";

  const activeProviders = providers.filter((item) => item.enabled && (item.provider !== "wechat" && item.provider !== "alipay" || currency === "CNY"));

  useEffect(() => {
    const first = activeProviders[0]?.provider;
    if (!first) return;
    setProvider((current) => activeProviders.some((item) => item.provider === current) ? current : first);
    const firstPackage = firstRechargePackage(activeProviders[0], currency);
    setPackageID((current) => current || firstPackage?.id || "");
    setAmount((current) => current || firstPackage?.amount || activeProviders[0]?.recharge_presets?.[0] || "");
  }, [providers, currency]);

  const selectedProvider = activeProviders.find((item) => item.provider === provider) || activeProviders[0];
  const legacyPresets = selectedProvider?.recharge_presets || [];
  const packages = (selectedProvider?.recharge_packages || []).filter((item) => item.kind === "recharge" && item.enabled && item.currency.toUpperCase() === currency.toUpperCase());
  const rate = selectedProvider?.recharge_rate || "1";

  function selectProvider(next: PaymentOrder["provider"]) {
    setProvider(next);
    const nextProvider = activeProviders.find((item) => item.provider === next);
    const firstPackage = firstRechargePackage(nextProvider, currency);
    setPackageID(firstPackage?.id || "");
    setAmount(firstPackage?.amount || nextProvider?.recharge_presets?.[0] || "");
  }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const selected = selectedProvider?.provider;
    if (!selected) return;
    await createOrder(selected, amount, currency, packageID || undefined);
  }

  const canCreate = activeProviders.length > 0 && Boolean(account) && !busy;
  const qrValue = order?.qr_code || "";

  return (
    <Card className="border-emerald-500/20 shadow-sm dark:bg-slate-900/60">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg"><BadgeDollarSign className="h-5 w-5 text-emerald-600" />{t("rechargeTitle")}</CardTitle>
        <CardDescription>{t("rechargeDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {message.text ? <div className={cn("rounded-xl border p-3 text-sm", message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : message.kind === "pending" ? "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300" : "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300")}>{message.text}</div> : null}
        <div className="grid gap-4 sm:grid-cols-3">
          <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-4"><div className="text-xs text-slate-500">{t("consoleBillingBalance")}</div><div className="mt-2 text-2xl font-bold text-emerald-700 dark:text-emerald-300">{account ? `${account.currency} ${formatMoney(account.balance)}` : "-"}</div></div>
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800"><div className="text-xs text-slate-500">{t("rechargeProviders")}</div><div className="mt-2 text-2xl font-bold text-slate-900 dark:text-white">{activeProviders.length}</div></div>
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800"><div className="text-xs text-slate-500">{t("rechargeCurrency")}</div><div className="mt-2 font-mono text-2xl font-bold text-slate-900 dark:text-white">{currency}</div></div>
        </div>
        {activeProviders.length === 0 ? <div className="rounded-xl border border-dashed border-slate-300 px-4 py-10 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">{t("rechargeUnavailable")}</div> : <form className="space-y-5 rounded-2xl border border-emerald-500/20 bg-emerald-50/40 p-4 dark:bg-emerald-500/5 sm:p-5" onSubmit={(event) => void submit(event)}>
          <div className="space-y-3"><Label>{t("rechargeProvider")}</Label><div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">{activeProviders.map((item) => { const active = selectedProvider?.provider === item.provider; return <button key={item.provider} type="button" onClick={() => selectProvider(item.provider)} className={cn("flex min-h-20 flex-col items-start justify-between rounded-xl border px-4 py-3 text-left transition-all", active ? "border-indigo-500 bg-white shadow-md shadow-indigo-500/10 dark:border-indigo-400 dark:bg-slate-900" : "border-slate-200 bg-white/70 hover:border-indigo-300 dark:border-slate-700 dark:bg-slate-950/40")}><span className="text-sm font-semibold text-slate-900 dark:text-white">{t(providerLabels[item.provider])}</span><span className={cn("text-xs", active ? "text-indigo-600 dark:text-indigo-300" : "text-slate-500")}>{t("rechargeRate")} x{item.recharge_rate || "1"}</span></button>; })}</div></div>
          <div className="space-y-3"><Label htmlFor="recharge-amount">{t("rechargeAmount")}</Label>{packages.length > 0 ? <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{packages.map((item) => <PackageOption key={item.id} item={item} currency={currency} selected={packageID === item.id} t={t} onSelect={() => { setPackageID(item.id); setAmount(item.amount); }} />)}</div> : <p className="rounded-lg border border-dashed border-slate-300 px-3 py-3 text-xs text-slate-500 dark:border-slate-700">{t("rechargeCustomOnly")}</p>}<Input id="recharge-amount" inputMode="decimal" min="0.01" value={amount} onChange={(event) => { setAmount(event.target.value); setPackageID(""); }} placeholder={legacyPresets[0] || t("rechargeCustomAmountPlaceholder")} required /><p className="text-xs text-slate-500">{t("rechargeRateHint")} {rate}</p></div>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between"><p className="max-w-xl text-xs leading-5 text-slate-500">{t("rechargeCustomAmountHint")}</p><Button type="submit" disabled={!canCreate} className="gap-2 sm:min-w-44"><BadgeDollarSign className="h-4 w-4" />{busy ? t("rechargeCreating") : t("rechargeCreate")}</Button></div>
        </form>}
        {order ? <OrderStatus t={t} order={order} qrValue={qrValue} stripePublishableKey={activeProviders.find((item) => item.provider === "stripe")?.publishable_key || ""} busy={busy} refreshOrder={refreshOrder} capturePayPal={capturePayPal} /> : null}
      </CardContent>
    </Card>
  );
}

function formatMoney(value?: string) {
  return formatDecimalWithoutTrailingZeros(value);
}

function firstRechargePackage(provider: PublicPaymentProvider | undefined, currency: string) {
  return provider?.recharge_packages?.find((item) => item.kind === "recharge" && item.enabled && item.currency.toUpperCase() === currency.toUpperCase());
}

function PackageOption({ item, currency, selected, t, onSelect }: { item: PaymentRechargePackage; currency: string; selected: boolean; t: (key: TranslationKey) => string; onSelect: () => void }) {
  const validity = item.validity_days > 0 ? `${item.validity_days} ${t("rechargeValidityDays")}` : t("rechargeValidityPermanent");
  return <button type="button" onClick={onSelect} className={cn("rounded-xl border bg-white px-4 py-3 text-left transition-all dark:bg-slate-950/60", selected ? "border-emerald-600 bg-emerald-50 shadow-md shadow-emerald-500/10 dark:border-emerald-400 dark:bg-emerald-500/10" : "border-slate-200 hover:border-emerald-400 dark:border-slate-700")}><span className="block text-base font-bold text-slate-900 dark:text-white">{item.name}</span><span className="mt-1 block text-lg font-bold text-emerald-700 dark:text-emerald-300">{currency} {formatMoney(item.amount)}</span><span className="mt-1 block text-xs text-slate-500">{t("rechargeCreditedAmount")} {currency} {formatMoney(item.credited_amount)}{Number(item.bonus_amount) > 0 ? ` (+${formatMoney(item.bonus_amount)} ${t("rechargeBonus")})` : ""}</span><span className="mt-1 block text-xs text-slate-500">{validity}</span>{item.description ? <span className="mt-1 block text-xs text-slate-500">{item.description}</span> : null}</button>;
}

function OrderStatus({ t, order, qrValue, stripePublishableKey, busy, refreshOrder, capturePayPal }: { t: (key: TranslationKey) => string; order: PaymentOrder; qrValue: string; stripePublishableKey: string; busy: boolean; refreshOrder: () => Promise<void>; capturePayPal: () => Promise<void> }) {
  const statusLabel = order.status === "paid" ? t("rechargeStatusPaid") : order.status === "failed" ? t("rechargeStatusFailed") : order.status === "expired" ? t("rechargeStatusExpired") : order.status === "cancelled" ? t("rechargeStatusCancelled") : t("rechargeStatusPending");
  const canOpen = Boolean(order.checkout_url);
  return <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800"><div className="flex flex-wrap items-center justify-between gap-3"><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("rechargeOrderTitle")}</div><div className="mt-1 font-mono text-[11px] text-slate-500">{order.merchant_order_no}</div></div><Badge variant={order.status === "paid" ? "success" : order.status === "failed" || order.status === "expired" ? "destructive" : "warning"}>{statusLabel}</Badge></div><div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1fr)_220px]">{order.status === "pending" && order.checkout_client_secret && stripePublishableKey ? <StripeEmbeddedCheckout clientSecret={order.checkout_client_secret} publishableKey={stripePublishableKey} /> : order.status === "pending" && qrValue ? <div className="flex flex-col items-center justify-center gap-3 rounded-xl bg-white p-4"><QrCode className="h-5 w-5 text-emerald-600" /><QRCodeSVG value={qrValue} size={170} includeMargin /><p className="text-center text-xs text-slate-500">{t("rechargeScanHint")}</p></div> : null}<div className="space-y-3"><div className="grid grid-cols-2 gap-3 text-sm"><Info label={t("rechargeAmount")} value={`${order.currency} ${formatMoney(order.amount)}`} /><Info label={t("rechargeCreditedAmount")} value={`${order.currency} ${formatMoney(order.credited_amount || order.amount)}`} /><Info label={t("rechargeProvider")} value={t(providerLabels[order.provider])} /><Info label={t("rechargeRate")} value={order.recharge_rate || "1"} /></div>{order.bonus_amount && Number(order.bonus_amount) > 0 ? <Info label={t("rechargeBonus")} value={`${order.currency} ${formatMoney(order.bonus_amount)}`} /> : null}{order.valid_until ? <Info label={t("rechargeValidity")} value={new Date(order.valid_until).toLocaleDateString()} /> : null}{order.failure_reason ? <div className="rounded-lg bg-rose-50 p-3 text-xs text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{order.failure_reason}</div> : null}<div className="flex flex-wrap gap-2">{canOpen ? <Button type="button" variant="outline" size="sm" className="gap-2" onClick={() => window.location.assign(order.checkout_url || "")}><ExternalLink className="h-4 w-4" />{t("rechargeOpenPayment")}</Button> : null}{order.provider === "paypal" && order.status === "pending" ? <Button type="button" size="sm" onClick={() => void capturePayPal()} disabled={busy} className="gap-2"><CheckCircle2 className="h-4 w-4" />{t("rechargeConfirmPayPal")}</Button> : null}{order.status === "pending" ? <Button type="button" variant="ghost" size="icon" onClick={() => void refreshOrder()} disabled={busy} title={t("rechargeRefresh")} aria-label={t("rechargeRefresh")}><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} /></Button> : null}</div>{order.checkout_url ? <div className="flex items-center gap-2"><code className="min-w-0 flex-1 break-all text-[11px] text-slate-500">{order.checkout_url}</code><Button type="button" variant="outline" size="icon" onClick={() => void navigator.clipboard?.writeText(order.checkout_url || "")} title={t("rechargeCopyLink")} aria-label={t("rechargeCopyLink")}><Copy className="h-4 w-4" /></Button></div> : null}</div></div></div>;
}

function StripeEmbeddedCheckout({ clientSecret, publishableKey }: { clientSecret: string; publishableKey: string }) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const checkoutRef = useRef<{ mount: (element: HTMLElement) => void; destroy: () => void } | null>(null);
  const [error, setError] = useState(false);
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const stripe = await loadStripe(publishableKey);
      if (!stripe || !containerRef.current || cancelled) {
        if (!cancelled) setError(true);
        return;
      }
      try {
        const checkout = await stripe.createEmbeddedCheckoutPage({ clientSecret });
        if (cancelled || !containerRef.current) {
          checkout.destroy();
          return;
        }
        checkoutRef.current = checkout;
        checkout.mount(containerRef.current);
      } catch {
        if (!cancelled) setError(true);
      }
    })();
    return () => {
      cancelled = true;
      checkoutRef.current?.destroy();
      checkoutRef.current = null;
    };
  }, [clientSecret, publishableKey]);
  return <div className="min-h-[420px] rounded-xl border border-indigo-500/20 bg-white p-3 dark:bg-slate-950">{error ? <div className="flex min-h-[390px] items-center justify-center text-sm text-rose-600">Stripe Checkout unavailable. Please reopen the payment page.</div> : <div ref={containerRef} />}</div>;
}

function Info({ label, value }: { label: string; value: string }) { return <div className="rounded-lg border border-slate-200 bg-slate-50 p-3 dark:border-slate-800 dark:bg-slate-950/40"><div className="text-[11px] text-slate-500">{label}</div><div className="mt-1 text-sm font-semibold text-slate-900 dark:text-white">{value}</div></div>; }
