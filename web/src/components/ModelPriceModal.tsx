import React from "react";
import { Database, Save, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Language, LoginMessage, ModelPriceFormState, PriceComponentFormState, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface ModelPriceModalProps {
  open: boolean;
  language: Language;
  form: ModelPriceFormState;
  setForm: React.Dispatch<React.SetStateAction<ModelPriceFormState>>;
  busy: boolean;
  message: LoginMessage;
  onClose: () => void;
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
}

export function ModelPriceModal({ open, language, form, setForm, busy, message, onClose, onSubmit }: ModelPriceModalProps) {
  if (!open) return null;
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;

  return (
    <div className="fixed inset-0 z-[75] flex items-center justify-center overflow-y-auto bg-slate-950/60 p-3 backdrop-blur-sm sm:p-6">
      <button type="button" aria-label={t("billingPriceCancel")} className="absolute inset-0 h-full w-full cursor-default" onClick={onClose} disabled={busy} />
      <div role="dialog" aria-modal="true" aria-labelledby="model-price-dialog-title" className="relative z-10 flex max-h-[92vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white text-slate-900 shadow-2xl dark:border-slate-700/80 dark:bg-slate-900 dark:text-slate-100">
        <div className="flex items-center justify-between gap-4 border-b border-slate-200 bg-slate-50/80 px-5 py-4 dark:border-slate-800/80 dark:bg-slate-950/60 sm:px-6">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-cyan-500 text-white shadow-md shadow-indigo-500/20"><Database className="h-5 w-5" /></div>
            <div>
              <h2 id="model-price-dialog-title" className="text-lg font-bold tracking-tight text-slate-900 dark:text-white">{t("billingPriceEditTitle")}</h2>
              <p className="text-xs text-slate-500 dark:text-slate-400">{t("billingPriceEditHint")}</p>
            </div>
          </div>
          <Button type="button" variant="ghost" size="icon" onClick={onClose} disabled={busy} title={t("billingPriceCancel")}><X className="h-4 w-4" /></Button>
        </div>

        <div className="min-h-0 overflow-y-auto px-5 py-5 sm:px-6">
          <form id="model-price-form" className="space-y-5" onSubmit={onSubmit}>
            <div className="rounded-xl border border-slate-200 bg-slate-50/80 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/40">
              <div className="font-semibold text-slate-900 dark:text-white">{form.model}</div>
              <div className="mt-1 font-mono text-[11px] uppercase text-slate-500 dark:text-slate-400">{form.provider}</div>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="model-price-currency">{t("billingPriceCurrency")}</Label>
                <Input id="model-price-currency" maxLength={3} value={form.currency} onChange={(event) => setForm((current) => ({ ...current, currency: event.target.value.toUpperCase() }))} placeholder="USD" disabled={busy} required />
              </div>
              <div className="flex items-end pb-1 text-xs text-slate-500 dark:text-slate-400">{t("billingPricePerMillionHint")}</div>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <PriceInput id="model-price-input" label={t("modelsInputPrice")} value={form.input_price_per_million_tokens} onChange={(value) => setForm((current) => ({ ...current, input_price_per_million_tokens: value }))} disabled={busy} />
              <PriceInput id="model-price-output" label={t("modelsOutputPrice")} value={form.output_price_per_million_tokens} onChange={(value) => setForm((current) => ({ ...current, output_price_per_million_tokens: value }))} disabled={busy} />
              <PriceInput id="model-price-cached" label={t("modelsCachedInputPrice")} value={form.cached_input_price_per_million_tokens} onChange={(value) => setForm((current) => ({ ...current, cached_input_price_per_million_tokens: value }))} disabled={busy} />
              <PriceInput id="model-price-cache-creation" label={t("modelsCacheWritesPrice")} value={form.cache_creation_price_per_million_tokens} onChange={(value) => setForm((current) => ({ ...current, cache_creation_price_per_million_tokens: value }))} disabled={busy} />
              <PriceInput id="model-price-reasoning" label={t("modelsReasoningPrice")} value={form.reasoning_price_per_million_tokens} onChange={(value) => setForm((current) => ({ ...current, reasoning_price_per_million_tokens: value }))} disabled={busy} />
            </div>
            <div className="space-y-3 rounded-xl border border-slate-200 p-4 dark:border-slate-800">
              <div className="flex items-center justify-between gap-3"><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{language === "zh" ? "其他计费组件" : "Other billing components"}</div><div className="text-xs text-slate-500 dark:text-slate-400">{language === "zh" ? "按组件实际单位填写，Token 组件使用每 1M Token。" : "Use the component unit; token components use per 1M tokens."}</div></div><Button type="button" variant="outline" size="sm" onClick={() => setForm((current) => ({ ...current, components: [...current.components, { component_code: "", unit: "", price_per_unit: "" }] }))} disabled={busy}>+ {language === "zh" ? "添加" : "Add"}</Button></div>
              {form.components.filter((component) => !["input_tokens", "output_tokens", "cached_input_tokens", "cache_creation_tokens", "reasoning_tokens"].includes(component.component_code)).length === 0 ? <div className="text-xs text-slate-500 dark:text-slate-400">{language === "zh" ? "暂无额外组件。" : "No additional components."}</div> : form.components.map((component, index) => !["input_tokens", "output_tokens", "cached_input_tokens", "cache_creation_tokens", "reasoning_tokens"].includes(component.component_code) ? <ComponentInput key={String(component.component_code) + "-" + index} component={component} index={index} busy={busy} setForm={setForm} language={language} /> : null)}
            </div>
            {message.text ? <p className={cn("text-sm", message.kind === "error" ? "text-rose-600 dark:text-rose-400" : message.kind === "success" ? "text-emerald-600 dark:text-emerald-400" : "text-amber-600 dark:text-amber-400")}>{message.text}</p> : null}
          </form>
        </div>

        <div className="flex flex-col-reverse gap-2 border-t border-slate-200 px-5 py-4 dark:border-slate-800/80 sm:flex-row sm:justify-end sm:px-6">
          <Button type="button" variant="outline" onClick={onClose} disabled={busy}>{t("billingPriceCancel")}</Button>
          <Button type="submit" form="model-price-form" disabled={busy}><Save className="h-4 w-4" />{busy ? t("billingPriceSaving") : t("billingPriceSave")}</Button>
        </div>
      </div>
    </div>
  );
}

function PriceInput({ id, label, value, onChange, disabled }: { id: string; label: string; value: string; onChange: (value: string) => void; disabled: boolean }) {
  return <div className="space-y-2"><Label htmlFor={id}>{label}</Label><Input id={id} inputMode="decimal" value={value} onChange={(event) => onChange(event.target.value)} placeholder="0" disabled={disabled} /></div>;
}

function ComponentInput({ component, index, busy, setForm, language }: { component: PriceComponentFormState; index: number; busy: boolean; setForm: React.Dispatch<React.SetStateAction<ModelPriceFormState>>; language: Language }) {
  const update = (patch: Partial<PriceComponentFormState>) => setForm((current) => ({ ...current, components: current.components.map((item, itemIndex) => itemIndex === index ? { ...item, ...patch } : item) }));
  return <div className="grid gap-2 sm:grid-cols-[1.2fr_0.8fr_1fr_auto]"><Input value={component.component_code} onChange={(event) => update({ component_code: event.target.value })} placeholder={language === "zh" ? "组件编码，如 input_images" : "Code, e.g. input_images"} disabled={busy} /><Input value={component.unit} onChange={(event) => update({ unit: event.target.value })} placeholder={language === "zh" ? "单位" : "Unit"} disabled={busy} /><Input inputMode="decimal" value={component.price_per_unit} onChange={(event) => update({ price_per_unit: event.target.value })} placeholder="0" disabled={busy} /><Button type="button" variant="ghost" size="icon" onClick={() => setForm((current) => ({ ...current, components: current.components.filter((_, itemIndex) => itemIndex !== index) }))} disabled={busy} title={language === "zh" ? "删除组件" : "Remove component"}>x</Button></div>;
}
