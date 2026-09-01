import { useEffect, useMemo, useState } from "react";
import { AudioLines, CheckCircle2, CircleOff, Code2, Cpu, Image, Layers3, RefreshCw, Search, Server, Sparkles, Video, Boxes } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Language, LoginMessage, PublicModelSummary, PublicPlatformModelPrice, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface ModelPlazaViewProps {
  language: Language;
  models: PublicModelSummary[];
  busy: boolean;
  message: LoginMessage;
  refresh: (showPending?: boolean) => Promise<void>;
}

type ModelCategory = "all" | "text" | "image" | "video" | "audio" | "embedding";

function providerClass(provider: string) {
  switch (provider.toLowerCase()) {
    case "openai":
      return "border-indigo-500/25 bg-indigo-500/10 text-indigo-700 dark:text-indigo-300";
    case "anthropic":
      return "border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
    default:
      return "border-cyan-500/25 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300";
  }
}

function formatPrice(value: string | undefined, rawPerToken = false) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "-";
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 6 }).format(rawPerToken ? parsed * 1_000_000 : parsed);
}

function formatMultiplier(value: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 6 }).format(parsed);
}

function categoryIcon(category: ModelCategory) {
  switch (category) {
    case "image":
      return Image;
    case "video":
      return Video;
    case "audio":
      return AudioLines;
    case "embedding":
      return Boxes;
    default:
      return Code2;
  }
}

export function ModelPlazaView({ language, models, busy, message, refresh }: ModelPlazaViewProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [search, setSearch] = useState("");
  const [provider, setProvider] = useState("all");
  const [category, setCategory] = useState<ModelCategory>("all");
  const [groupID, setGroupID] = useState("default");

  const providers = useMemo(() => Array.from(new Set(models.map((model) => model.provider))).sort(), [models]);
  const platformGroups = useMemo(() => {
    const unique = new Map<string, PublicPlatformModelPrice>();
    for (const model of models) {
      for (const group of model.pricing?.platform_prices || []) {
        if (!unique.has(group.group_id)) unique.set(group.group_id, group);
      }
    }
    return Array.from(unique.values());
  }, [models]);

  useEffect(() => {
    if (platformGroups.length === 0) return;
    if (!platformGroups.some((group) => group.group_id === groupID)) {
      setGroupID(platformGroups.find((group) => group.group_code === "default")?.group_id || platformGroups[0].group_id);
    }
  }, [groupID, platformGroups]);

  const categoryCounts = useMemo(() => ({
    all: models.length,
    text: models.filter((model) => model.category === "text").length,
    image: models.filter((model) => model.category === "image").length,
    video: models.filter((model) => model.category === "video").length,
    audio: models.filter((model) => model.category === "audio").length,
    embedding: models.filter((model) => model.category === "embedding").length,
  }), [models]);

  const filteredModels = useMemo(() => {
    const query = search.trim().toLowerCase();
    return models.filter((model) => {
      const matchesProvider = provider === "all" || model.provider === provider;
      const matchesCategory = category === "all" || model.category === category;
      const matchesSearch = !query || `${model.name} ${model.display_name} ${model.provider} ${model.protocol_family}`.toLowerCase().includes(query);
      return matchesProvider && matchesCategory && matchesSearch;
    });
  }, [category, models, provider, search]);

  const categories: Array<{ id: ModelCategory; label: string; icon: typeof Code2 }> = [
    { id: "all", label: t("modelsCategoryAll"), icon: Layers3 },
    { id: "text", label: t("modelsCategoryText"), icon: Code2 },
    { id: "image", label: t("modelsCategoryImage"), icon: Image },
    { id: "video", label: t("modelsCategoryVideo"), icon: Video },
    { id: "audio", label: t("modelsCategoryAudio"), icon: AudioLines },
    { id: "embedding", label: t("modelsCategoryEmbedding"), icon: Boxes },
  ];

  return (
    <div className="min-h-[calc(100vh-72px)] bg-slate-50 px-4 py-8 dark:bg-slate-950 sm:px-6 lg:px-10">
      <div className="mx-auto max-w-[1480px] space-y-6">
        <section className="relative overflow-hidden rounded-[28px] border border-slate-200/80 bg-white px-6 py-8 shadow-sm dark:border-slate-800 dark:bg-slate-900/70 lg:px-10 lg:py-10">
          <div className="pointer-events-none absolute right-0 top-0 h-full w-1/3 bg-gradient-to-bl from-indigo-500/10 via-cyan-500/5 to-transparent" />
          <div className="relative flex flex-col gap-8 lg:flex-row lg:items-end lg:justify-between">
            <div className="max-w-3xl">
              <div className="mb-3 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-indigo-600 dark:text-indigo-400"><Sparkles className="h-4 w-4" />{t("modelsEyebrow")}</div>
              <h1 className="text-3xl font-bold tracking-tight text-slate-950 dark:text-white sm:text-4xl">{t("modelsTitle")}</h1>
              <p className="mt-3 text-sm leading-6 text-slate-600 dark:text-slate-400">{t("modelsDescription")}</p>
            </div>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
              <div className="rounded-2xl border border-slate-200 bg-slate-50 px-5 py-4 dark:border-slate-800 dark:bg-slate-950/60"><div className="text-2xl font-bold text-slate-950 dark:text-white">{models.length}</div><div className="mt-1 text-xs text-slate-500">{t("modelsTotal")}</div></div>
              <div className="rounded-2xl border border-emerald-500/20 bg-emerald-500/5 px-5 py-4"><div className="text-2xl font-bold text-emerald-700 dark:text-emerald-300">{models.filter((model) => model.available).length}</div><div className="mt-1 text-xs text-emerald-700/70 dark:text-emerald-300/70">{t("modelsAvailable")}</div></div>
              <div className="hidden rounded-2xl border border-indigo-500/20 bg-indigo-500/5 px-5 py-4 sm:block"><div className="text-2xl font-bold text-indigo-700 dark:text-indigo-300">{providers.length}</div><div className="mt-1 text-xs text-indigo-700/70 dark:text-indigo-300/70">{t("modelsProviders")}</div></div>
            </div>
          </div>
        </section>

        <div className="flex flex-col gap-3 rounded-2xl border border-slate-200/80 bg-white/70 p-3 shadow-sm dark:border-slate-800 dark:bg-slate-900/60 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex min-w-0 flex-1 flex-wrap gap-1.5">
            {categories.map((item) => {
              const Icon = item.icon;
              const active = category === item.id;
              return <button key={item.id} type="button" onClick={() => setCategory(item.id)} className={cn("inline-flex items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-semibold transition-colors", active ? "bg-indigo-600 text-white shadow-sm" : "text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800")}><Icon className="h-3.5 w-3.5" />{item.label}<span className={cn("rounded-md px-1.5 py-0.5 text-[10px]", active ? "bg-white/15 text-white" : "bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400")}>{categoryCounts[item.id]}</span></button>;
            })}
          </div>
          <div className="flex flex-col gap-2 sm:flex-row">
            <div className="relative w-full sm:w-64"><Search className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" /><Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("modelsSearchPlaceholder")} className="h-9 pl-9" /></div>
            <select value={provider} onChange={(event) => setProvider(event.target.value)} className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200"><option value="all">{t("modelsAllProviders")}</option>{providers.map((item) => <option key={item} value={item}>{item}</option>)}</select>
            {platformGroups.length > 0 ? <select value={groupID} onChange={(event) => setGroupID(event.target.value)} className="h-9 max-w-full rounded-lg border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200"><option value="">{t("modelsGroupAll")}</option>{platformGroups.map((group) => <option key={group.group_id} value={group.group_id}>{group.group_name} · x{formatMultiplier(group.multiplier)}</option>)}</select> : null}
            <Button variant="outline" size="icon" onClick={() => void refresh(true)} disabled={busy} title={t("modelsRefresh")}><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} /></Button>
          </div>
        </div>

        {message.text ? <div className="rounded-xl border border-rose-500/30 bg-rose-50 p-3 text-sm text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{message.text}</div> : null}

        {busy && models.length === 0 ? <div className="py-20 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-3 h-6 w-6 animate-spin text-indigo-600" />{t("modelsLoading")}</div> : filteredModels.length === 0 ? <div className="rounded-2xl border border-dashed border-slate-300 py-20 text-center text-sm text-slate-500 dark:border-slate-700">{models.length === 0 ? t("modelsEmpty") : t("modelsNoMatch")}</div> : <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredModels.map((model) => {
            const capabilities = capabilityItems(model, t);
            const platformPrice = findPlatformPrice(model, groupID);
            return <Card key={model.id} className="group border-slate-200/80 bg-white shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-indigo-300 hover:shadow-lg hover:shadow-indigo-500/10 dark:border-slate-800 dark:bg-slate-900/70 dark:hover:border-indigo-500/50">
              <CardHeader className="space-y-3 pb-3"><div className="flex items-start justify-between gap-3"><div className={cn("inline-flex items-center gap-2 rounded-full border px-2.5 py-1 text-xs font-semibold", providerClass(model.provider))}><Cpu className="h-3.5 w-3.5" />{model.provider}</div><Badge variant={model.available ? "success" : "muted"}>{model.available ? <><CheckCircle2 className="mr-1 h-3.5 w-3.5" />{t("modelsStatusAvailable")}</> : <><CircleOff className="mr-1 h-3.5 w-3.5" />{t("modelsStatusUnavailable")}</>}</Badge></div><div className="flex items-start gap-2"><div className="rounded-lg bg-slate-100 p-2 text-slate-500 dark:bg-slate-800 dark:text-slate-300">{(() => { const Icon = categoryIcon(model.category); return <Icon className="h-4 w-4" />; })()}</div><div><CardTitle className="text-xl text-slate-950 dark:text-white">{model.display_name}</CardTitle><CardDescription className="mt-1 font-mono text-xs">{model.name}</CardDescription></div></div></CardHeader>
              <CardContent className="space-y-4"><div className="flex flex-wrap gap-1.5">{capabilities.length > 0 ? capabilities.map((item) => <span key={item} className="rounded-md bg-slate-100 px-2 py-1 text-[11px] font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-300">{item}</span>) : <span className="text-xs text-slate-500">{t("modelsCapabilitiesPending")}</span>}</div><div className="grid grid-cols-2 gap-3 border-y border-slate-100 py-3 dark:border-slate-800"><div className="flex items-start gap-2"><Server className="mt-0.5 h-4 w-4 text-cyan-600 dark:text-cyan-400" /><div><div className="text-[11px] text-slate-500">{t("modelsRoutes")}</div><div className="mt-1 text-sm font-semibold text-slate-800 dark:text-slate-200">{model.active_channel_count} / {model.channel_count}</div></div></div><div className="flex items-start gap-2"><Code2 className="mt-0.5 h-4 w-4 text-indigo-600 dark:text-indigo-400" /><div><div className="text-[11px] text-slate-500">{t("modelsProtocol")}</div><div className="mt-1 truncate text-xs font-semibold text-slate-700 dark:text-slate-300">{model.protocol_family}</div></div></div></div>{model.pricing ? <PriceComparison official={model.pricing} platform={platformPrice} language={language} t={t} /> : <div className="flex items-center gap-2 text-xs text-slate-500"><Layers3 className="h-4 w-4" />{t("modelsPricingPending")}</div>}</CardContent>
            </Card>;
          })}
        </div>}
      </div>
    </div>
  );
}

function capabilityItems(model: PublicModelSummary, t: (key: TranslationKey) => string) {
  const items: string[] = [];
  const modalities = model.capabilities.modalities;
  if (Array.isArray(modalities)) {
    for (const modality of modalities) if (typeof modality === "string") items.push(modality);
  }
  if (model.capabilities.official_sdk === true) items.push(t("modelOfficialSDK"));
  if (model.capabilities.tool_calling === true) items.push(t("modelToolCalling"));
  if (model.capabilities.streaming === true) items.push(t("modelStreaming"));
  if (model.capabilities.multimodal_input === true) items.push(t("modelMultimodal"));
  if (model.capabilities.reasoning === true) items.push(t("modelReasoning"));
  return items.slice(0, 4);
}

function findPlatformPrice(model: PublicModelSummary, groupID: string) {
  const prices = model.pricing?.platform_prices || [];
  if (prices.length === 0) return undefined;
  return prices.find((price) => price.group_id === groupID) || prices.find((price) => price.group_code === "default") || prices[0];
}

function componentLabel(code: string) {
  return code.replace(/_/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function componentPrice(value: string, unit: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "-";
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 8 }).format(parsed * componentUnitScale(unit));
}

function componentUnit(unit: string) {
  const normalizedUnit = unit.toLowerCase();
  if (normalizedUnit.includes("1k") && normalizedUnit.includes("call")) return "USD / 1K calls";
  if (normalizedUnit.includes("1k") && normalizedUnit.includes("token")) return "USD / 1K Token";
  if (normalizedUnit.includes("token")) {
    const kind = normalizedUnit.includes("audio") ? "Audio" : normalizedUnit.includes("image") ? "Image" : normalizedUnit.includes("video") ? "Video" : normalizedUnit.includes("dbu") ? "DBU" : "";
    return kind ? "USD / 1M " + kind + " Token" : "USD / 1M Token";
  }
  if (normalizedUnit === "second") return "USD / second";
  if (normalizedUnit === "image") return "USD / image";
  if (normalizedUnit === "pixel") return "USD / pixel";
  if (normalizedUnit === "request") return "USD / request";
  if (normalizedUnit === "query") return "USD / query";
  if (normalizedUnit === "session") return "USD / session";
  if (normalizedUnit === "page") return "USD / page";
  if (normalizedUnit === "gb_day") return "USD / GB-day";
  return "USD / " + (unit || "unit");
}

function componentUnitScale(unit: string) {
  const normalizedUnit = unit.toLowerCase();
  if (normalizedUnit.includes("1k") || normalizedUnit.includes("gb_day")) return 1;
  if (normalizedUnit === "token" || normalizedUnit.endsWith("_token")) return 1_000_000;
  return 1;
}

function ComponentPriceList({ components, tone = "official" }: { components?: NonNullable<PublicModelSummary["pricing"]>["components"]; tone?: "official" | "platform" }) {
  const visible = (components || []).filter((component) => !["input_tokens", "output_tokens", "cached_input_tokens", "reasoning_tokens"].includes(component.component_code));
  if (visible.length === 0) return null;
  return <div className="mt-2 space-y-1.5 border-t border-slate-200 pt-2 dark:border-slate-800">{visible.map((component) => <div key={component.component_code} className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-[11px]"><span className="text-slate-500 dark:text-slate-400">{componentLabel(component.component_code)}</span><span className={cn("font-mono", tone === "platform" ? "text-indigo-700 dark:text-indigo-300" : "text-slate-800 dark:text-slate-200")}>{componentPrice(component.price_per_unit, component.unit)} <span className="font-sans text-[10px] text-slate-400">{componentUnit(component.unit)}</span></span></div>)}</div>;
}

function PriceComparison({ official, platform, language, t }: { official: NonNullable<PublicModelSummary["pricing"]>; platform?: PublicPlatformModelPrice; language: Language; t: (key: TranslationKey) => string }) {
  const input = formatPrice(official.input_price_per_million_tokens || official.input_price_per_unit, !official.input_price_per_million_tokens);
  const output = formatPrice(official.output_price_per_million_tokens || official.output_price_per_unit, !official.output_price_per_million_tokens);
  const officialReasoning = Number(official.reasoning_price_per_million_tokens || official.reasoning_price_per_unit);
  const platformReasoning = Number(platform?.reasoning_price_per_million_tokens || "");
  return <div className="space-y-2 rounded-xl border border-slate-200/80 bg-slate-50/70 p-3 dark:border-slate-800 dark:bg-slate-950/40"><div className="flex flex-wrap items-center justify-between gap-2"><span className="text-xs font-semibold text-slate-700 dark:text-slate-200">{t("modelsOfficialPrice")} · {official.currency} / 1M</span><span className="rounded-md bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:text-amber-300">{official.source === "litellm" ? t("modelsPricingReference") : t("modelsPricingManual")}</span></div><div className="flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-slate-600 dark:text-slate-400"><span>{t("modelsInputPrice")} <strong className="text-slate-800 dark:text-slate-200">{input}</strong></span><span>{t("modelsOutputPrice")} <strong className="text-slate-800 dark:text-slate-200">{output}</strong></span>{Number(official.cached_input_price_per_million_tokens || official.cached_input_price_per_unit) > 0 ? <span>{t("modelsCachedInputPrice")} <strong className="text-slate-800 dark:text-slate-200">{formatPrice(official.cached_input_price_per_million_tokens || official.cached_input_price_per_unit, !official.cached_input_price_per_million_tokens)}</strong></span> : null}{Number.isFinite(officialReasoning) && officialReasoning > 0 ? <span>{t("modelsReasoningPrice")} <strong className="text-slate-800 dark:text-slate-200">{formatPrice(official.reasoning_price_per_million_tokens || official.reasoning_price_per_unit, !official.reasoning_price_per_million_tokens)}</strong></span> : null}</div><ComponentPriceList components={official.components} />{platform ? <div className="border-t border-slate-200 pt-2 dark:border-slate-800"><div className="flex flex-wrap items-center justify-between gap-2"><span className="text-xs font-semibold text-slate-700 dark:text-slate-200">{t("modelsPlatformPricing")}</span><span className="font-mono text-[10px] text-indigo-600 dark:text-indigo-300">{platform.group_name} · x{formatMultiplier(platform.multiplier)}</span></div>{platform.billing_type === "free" ? <div className="mt-1 text-[11px] font-medium text-emerald-700 dark:text-emerald-300">{t("modelsPlatformFree")}</div> : <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-slate-600 dark:text-slate-400"><span>{t("modelsInputPrice")} <strong className="text-indigo-700 dark:text-indigo-300">{platform.input_price_per_million_tokens}</strong></span><span>{t("modelsOutputPrice")} <strong className="text-indigo-700 dark:text-indigo-300">{platform.output_price_per_million_tokens}</strong></span>{Number(platform.cached_input_price_per_million_tokens) > 0 ? <span>{t("modelsCachedInputPrice")} <strong className="text-indigo-700 dark:text-indigo-300">{platform.cached_input_price_per_million_tokens}</strong></span> : null}{Number.isFinite(platformReasoning) && platformReasoning > 0 ? <span>{t("modelsReasoningPrice")} <strong className="text-indigo-700 dark:text-indigo-300">{platform.reasoning_price_per_million_tokens}</strong></span> : null}<ComponentPriceList components={platform.components} tone="platform" /></div>}</div> : null}{official.updated_at ? <div className="text-[10px] text-slate-400">{t("modelsPricingUpdated")} {new Date(official.updated_at).toLocaleDateString(language === "zh" ? "zh-CN" : "en-US")}</div> : null}</div>;
}
