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

function hasGroupPrice(model: PublicModelSummary, groupID: string) {
  const normalizedGroupID = groupID.trim();
  return Boolean(normalizedGroupID) && (model.pricing?.platform_prices || []).some((price) => price.group_id.trim() === normalizedGroupID);
}

function matchesModelSearch(model: PublicModelSummary, query: string) {
  return !query || `${model.name} ${model.display_name} ${model.provider} ${model.protocol_family}`.toLowerCase().includes(query);
}

export function ModelPlazaView({ language, models, busy, message, refresh }: ModelPlazaViewProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [search, setSearch] = useState("");
  const [provider, setProvider] = useState("all");
  const [category, setCategory] = useState<ModelCategory>("all");
  const [groupID, setGroupID] = useState("");

  const providers = useMemo(() => Array.from(new Set(models.map((model) => model.provider.trim()).filter(Boolean))).sort(), [models]);
  const platformGroups = useMemo(() => {
    const unique = new Map<string, PublicPlatformModelPrice>();
    for (const model of models) {
      for (const group of model.pricing?.platform_prices || []) {
        const groupID = group.group_id.trim();
        const groupName = group.group_name.trim();
        if (!groupID || !groupName || unique.has(groupID)) continue;
        unique.set(groupID, { ...group, group_id: groupID, group_name: groupName });
      }
    }
    return Array.from(unique.values());
  }, [models]);

  const query = search.trim().toLowerCase();
  const modelsForGroupFilter = useMemo(() => models.filter((model) => {
    const matchesProvider = provider === "all" || model.provider.trim() === provider;
    const matchesCategory = category === "all" || model.category === category;
    return matchesProvider && matchesCategory && matchesModelSearch(model, query);
  }), [category, models, provider, query]);

  const availableGroupIDs = useMemo(() => {
    const available = new Set<string>();
    for (const group of platformGroups) {
      if (modelsForGroupFilter.some((model) => hasGroupPrice(model, group.group_id))) available.add(group.group_id);
    }
    return available;
  }, [modelsForGroupFilter, platformGroups]);

  useEffect(() => {
    if (groupID && !availableGroupIDs.has(groupID)) setGroupID("");
  }, [availableGroupIDs, groupID]);

  const categoryCounts = useMemo(() => ({
    all: models.length,
    text: models.filter((model) => model.category === "text").length,
    image: models.filter((model) => model.category === "image").length,
    video: models.filter((model) => model.category === "video").length,
    audio: models.filter((model) => model.category === "audio").length,
    embedding: models.filter((model) => model.category === "embedding").length,
  }), [models]);

  const filteredModels = useMemo(() => {
    return models.filter((model) => {
      const matchesProvider = provider === "all" || model.provider.trim() === provider;
      const matchesCategory = category === "all" || model.category === category;
      const matchesSearch = matchesModelSearch(model, query);
      const matchesGroup = !groupID || hasGroupPrice(model, groupID);
      return matchesProvider && matchesCategory && matchesSearch && matchesGroup;
    });
  }, [category, groupID, models, provider, search]);

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

        <div className="space-y-3 rounded-2xl border border-slate-200/80 bg-white/70 p-3 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
          <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div className="flex min-w-0 flex-1 flex-wrap gap-1.5">
              {categories.map((item) => {
                const Icon = item.icon;
                const active = category === item.id;
                return <button key={item.id} type="button" aria-pressed={active} onClick={() => setCategory(item.id)} className={cn("inline-flex items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-semibold transition-colors", active ? "bg-indigo-600 text-white shadow-sm" : "text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800")}><Icon className="h-3.5 w-3.5" />{item.label}<span className={cn("rounded-md px-1.5 py-0.5 text-[10px]", active ? "bg-white/15 text-white" : "bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400")}>{categoryCounts[item.id]}</span></button>;
              })}
            </div>
            <div className="flex w-full flex-col gap-2 sm:flex-row xl:w-auto">
              <div className="relative w-full sm:w-64"><Search className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" /><Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("modelsSearchPlaceholder")} className="h-9 pl-9" /></div>
              <Button variant="outline" size="icon" onClick={() => void refresh(true)} disabled={busy} title={t("modelsRefresh")}><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} /></Button>
            </div>
          </div>

          <div className="space-y-2 border-t border-slate-200/80 pt-3 dark:border-slate-800">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
              <div className="w-16 shrink-0 pt-2 text-xs font-semibold text-slate-500 dark:text-slate-400">{t("modelsProviders")}</div>
              <div className="flex min-w-0 flex-1 flex-wrap gap-1.5">
                <button type="button" aria-pressed={provider === "all"} onClick={() => setProvider("all")} className={cn("rounded-lg border px-3 py-1.5 text-xs font-semibold transition-colors", provider === "all" ? "border-indigo-500 bg-indigo-600 text-white shadow-sm" : "border-slate-200 bg-white/70 text-slate-600 hover:border-indigo-300 hover:bg-indigo-50 dark:border-slate-700 dark:bg-slate-900/70 dark:text-slate-300 dark:hover:border-indigo-500/60 dark:hover:bg-indigo-500/10")}>{t("modelsAllProviders")}</button>
                {providers.map((item) => {
                  const active = provider === item;
                  return <button key={item} type="button" aria-pressed={active} onClick={() => setProvider(item)} className={cn("rounded-lg border px-3 py-1.5 text-xs font-semibold transition-colors", active ? "border-indigo-500 bg-indigo-600 text-white shadow-sm" : "border-slate-200 bg-white/70 text-slate-600 hover:border-indigo-300 hover:bg-indigo-50 dark:border-slate-700 dark:bg-slate-900/70 dark:text-slate-300 dark:hover:border-indigo-500/60 dark:hover:bg-indigo-500/10")}>{item}</button>;
                })}
              </div>
            </div>

            {platformGroups.length > 0 ? <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
              <div className="w-16 shrink-0 pt-2 text-xs font-semibold text-slate-500 dark:text-slate-400">{t("modelsGroupFilter")}</div>
              <div className="flex min-w-0 flex-1 flex-wrap gap-1.5">
                <button type="button" aria-pressed={!groupID} onClick={() => setGroupID("")} className={cn("rounded-lg border px-3 py-1.5 text-xs font-semibold transition-colors", !groupID ? "border-indigo-500 bg-indigo-600 text-white shadow-sm" : "border-slate-200 bg-white/70 text-slate-600 hover:border-indigo-300 hover:bg-indigo-50 dark:border-slate-700 dark:bg-slate-900/70 dark:text-slate-300 dark:hover:border-indigo-500/60 dark:hover:bg-indigo-500/10")}>{t("modelsGroupAll")}</button>
                {platformGroups.map((group) => {
                  const active = groupID === group.group_id;
                  const available = availableGroupIDs.has(group.group_id);
                  return <button key={group.group_id} type="button" aria-pressed={active} disabled={!available} title={!available ? t("modelsNoMatch") : undefined} onClick={() => setGroupID(group.group_id)} className={cn("rounded-lg border px-3 py-1.5 text-xs font-semibold transition-colors", active ? "border-indigo-500 bg-indigo-600 text-white shadow-sm" : available ? "border-slate-200 bg-white/70 text-slate-600 hover:border-indigo-300 hover:bg-indigo-50 dark:border-slate-700 dark:bg-slate-900/70 dark:text-slate-300 dark:hover:border-indigo-500/60 dark:hover:bg-indigo-500/10" : "cursor-not-allowed border-slate-200/70 bg-slate-100/60 text-slate-400 opacity-60 dark:border-slate-800 dark:bg-slate-950/50 dark:text-slate-600")}>{group.group_name} <span className={cn("ml-1 font-mono text-[10px]", active ? "text-indigo-100" : "text-slate-400 dark:text-slate-500")}>x{formatMultiplier(group.multiplier)}</span></button>;
                })}
              </div>
            </div> : null}
          </div>
        </div>

        {message.text ? <div className="rounded-xl border border-rose-500/30 bg-rose-50 p-3 text-sm text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{message.text}</div> : null}

        {busy && models.length === 0 ? <div className="py-20 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-3 h-6 w-6 animate-spin text-indigo-600" />{t("modelsLoading")}</div> : filteredModels.length === 0 ? <div className="rounded-2xl border border-dashed border-slate-300 py-20 text-center text-sm text-slate-500 dark:border-slate-700">{models.length === 0 ? t("modelsEmpty") : t("modelsNoMatch")}</div> : <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredModels.map((model) => {
            const capabilities = capabilityItems(model, t);
            const platformPrice = findPlatformPrice(model, groupID);
            return <Card key={model.id} className="group border-slate-200/80 bg-white shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-indigo-300 hover:shadow-lg hover:shadow-indigo-500/10 dark:border-slate-800 dark:bg-slate-900/70 dark:hover:border-indigo-500/50">
              <CardHeader className="space-y-3 pb-3"><div className="flex items-start justify-between gap-3"><div className={cn("inline-flex items-center gap-2 rounded-full border px-2.5 py-1 text-xs font-semibold", providerClass(model.provider))}><Cpu className="h-3.5 w-3.5" />{model.provider}</div><Badge variant={model.available ? "success" : "muted"}>{model.available ? <><CheckCircle2 className="mr-1 h-3.5 w-3.5" />{t("modelsStatusAvailable")}</> : <><CircleOff className="mr-1 h-3.5 w-3.5" />{t("modelsStatusUnavailable")}</>}</Badge></div><div className="flex items-start gap-2"><div className="rounded-lg bg-slate-100 p-2 text-slate-500 dark:bg-slate-800 dark:text-slate-300">{(() => { const Icon = categoryIcon(model.category); return <Icon className="h-4 w-4" />; })()}</div><div><CardTitle className="text-xl text-slate-950 dark:text-white">{model.display_name}</CardTitle><CardDescription className="mt-1 font-mono text-xs">{model.name}</CardDescription></div></div></CardHeader>
              <CardContent className="space-y-4"><div className="flex flex-wrap gap-1.5">{capabilities.length > 0 ? capabilities.map((item) => <span key={item} className="rounded-md bg-slate-100 px-2 py-1 text-[11px] font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-300">{item}</span>) : <span className="text-xs text-slate-500">{t("modelsCapabilitiesPending")}</span>}</div>{model.capabilities.seedance_version ? <SeedanceCapabilitySummary capabilities={model.capabilities} t={t} /> : null}<div className="grid grid-cols-2 gap-3 border-y border-slate-100 py-3 dark:border-slate-800"><div className="flex items-start gap-2"><Server className="mt-0.5 h-4 w-4 text-cyan-600 dark:text-cyan-400" /><div><div className="text-[11px] text-slate-500">{t("modelsRoutes")}</div><div className="mt-1 text-sm font-semibold text-slate-800 dark:text-slate-200">{model.active_channel_count} / {model.channel_count}</div></div></div><div className="flex items-start gap-2"><Code2 className="mt-0.5 h-4 w-4 text-indigo-600 dark:text-indigo-400" /><div><div className="text-[11px] text-slate-500">{t("modelsProtocol")}</div><div className="mt-1 truncate text-xs font-semibold text-slate-700 dark:text-slate-300">{model.protocol_family}</div></div></div></div>{model.pricing ? <PriceComparison official={model.pricing} platform={platformPrice} language={language} t={t} /> : <div className="flex items-center gap-2 text-xs text-slate-500"><Layers3 className="h-4 w-4" />{t("modelsPricingPending")}</div>}</CardContent>
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

function SeedanceCapabilitySummary({ capabilities, t }: { capabilities: Record<string, unknown>; t: (key: TranslationKey) => string }) {
  const version = typeof capabilities.seedance_version === "string" ? capabilities.seedance_version : "-";
  const defaultDuration = typeof capabilities.default_duration_seconds === "number"
    ? capabilities.default_duration_seconds === -1 ? "-1" : `${capabilities.default_duration_seconds}s`
    : "-";
  const maxDuration = typeof capabilities.max_duration_seconds === "number" ? `${capabilities.max_duration_seconds}s` : "-";
  const references = [
    { label: t("modelSeedanceReferenceImages"), value: capabilities.max_reference_images },
    { label: t("modelSeedanceReferenceVideos"), value: capabilities.max_reference_videos },
    { label: t("modelSeedanceReferenceAudios"), value: capabilities.max_reference_audios },
  ].filter((item) => typeof item.value === "number");
  const flags = [
    capabilities.supports_4k === true ? t("modelSeedance4K") : "",
    capabilities.supports_output_format === true ? t("modelSeedanceOutputFormat") : "",
    capabilities.audio_only_reference === true ? t("modelSeedanceAudioOnly") : t("modelSeedanceRequiresVisual"),
    capabilities.supports_omni_task_type === true ? t("modelSeedanceTaskTypes") : "",
  ].filter(Boolean);
  return <div className="space-y-2 rounded-xl border border-cyan-500/20 bg-cyan-500/[0.045] p-3 dark:border-cyan-400/20 dark:bg-cyan-400/[0.06]">
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs font-semibold text-cyan-800 dark:text-cyan-200">
      <span>{t("modelSeedanceVersion")} {version}</span>
      <span>{t("modelSeedanceDefaultDuration")} {defaultDuration}</span>
      <span>{t("modelSeedanceMaxDuration")} {maxDuration}</span>
    </div>
    <div className="grid grid-cols-3 gap-2 text-[10px] text-cyan-900/70 dark:text-cyan-100/70">
      {references.map((item) => <div key={item.label} className="min-w-0"><span className="block truncate">{item.label}</span><strong className="mt-0.5 block text-xs text-cyan-900 dark:text-cyan-100">{String(item.value)}</strong></div>)}
    </div>
    {flags.length > 0 ? <div className="flex flex-wrap gap-1.5">{flags.map((flag) => <span key={flag} className="rounded-md border border-cyan-500/20 bg-white/60 px-1.5 py-1 text-[10px] font-medium text-cyan-800 dark:border-cyan-400/20 dark:bg-slate-900/40 dark:text-cyan-200">{flag}</span>)}</div> : null}
  </div>;
}

function findPlatformPrice(model: PublicModelSummary, groupID: string) {
  const prices = model.pricing?.platform_prices || [];
  if (prices.length === 0) return undefined;
  if (!groupID) return prices.find((price) => price.group_code === "default") || prices[0];
  return prices.find((price) => price.group_id.trim() === groupID.trim());
}

type PriceComponents = NonNullable<PublicModelSummary["pricing"]>["components"];

interface PriceSummary {
  input: string;
  cachedInput: string;
  cacheWrites: string;
  output: string;
}

function componentPrice(components: PriceComponents, codes: string[]) {
  const component = (components || []).find((item) => codes.includes(item.component_code));
  if (!component) return "-";
  const parsed = Number(component.price_per_unit);
  if (!Number.isFinite(parsed)) return "-";
  const unit = component.unit.toLowerCase();
  const scale = unit === "token" || unit.endsWith("_token") ? 1_000_000 : 1;
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 6 }).format(parsed * scale);
}

function primaryOrComponent(primary: string | undefined, fallback: string | undefined, components: PriceComponents, codes: string[]) {
  if (primary) return formatPrice(primary);
  if (fallback) return formatPrice(fallback, true);
  return componentPrice(components, codes);
}

function officialPriceSummary(official: NonNullable<PublicModelSummary["pricing"]>): PriceSummary {
  return {
    input: primaryOrComponent(official.input_price_per_million_tokens, official.input_price_per_unit, official.components, ["input_tokens"]),
    cachedInput: primaryOrComponent(official.cached_input_price_per_million_tokens, official.cached_input_price_per_unit, official.components, ["cached_input_tokens"]),
    cacheWrites: primaryOrComponent(official.cache_creation_price_per_million_tokens, undefined, official.components, ["cache_creation_tokens", "cache_creation_1h_tokens"]),
    output: primaryOrComponent(official.output_price_per_million_tokens, official.output_price_per_unit, official.components, ["output_tokens"]),
  };
}

function platformPriceSummary(platform: PublicPlatformModelPrice): PriceSummary {
  return {
    input: primaryOrComponent(platform.input_price_per_million_tokens, undefined, platform.components, ["input_tokens"]),
    cachedInput: primaryOrComponent(platform.cached_input_price_per_million_tokens, undefined, platform.components, ["cached_input_tokens"]),
    cacheWrites: primaryOrComponent(platform.cache_creation_price_per_million_tokens, undefined, platform.components, ["cache_creation_tokens", "cache_creation_1h_tokens"]),
    output: primaryOrComponent(platform.output_price_per_million_tokens, undefined, platform.components, ["output_tokens"]),
  };
}

function CompactPriceRow({ pricing, tone = "official", t }: { pricing: PriceSummary; tone?: "official" | "platform"; t: (key: TranslationKey) => string }) {
  const valueClass = tone === "platform" ? "text-indigo-700 dark:text-indigo-300" : "text-slate-800 dark:text-slate-200";
  const items = [
    { label: t("modelsInputPrice"), value: pricing.input },
    { label: t("modelsCachedInputPrice"), value: pricing.cachedInput },
    { label: t("modelsCacheWritesPrice"), value: pricing.cacheWrites },
    { label: t("modelsOutputPrice"), value: pricing.output },
  ];
  return <div className="grid grid-cols-2 gap-x-3 gap-y-1.5 sm:grid-cols-4">{items.map((item) => <div key={item.label} className="min-w-0 text-[11px]"><span className="block truncate text-slate-500 dark:text-slate-400">{item.label}</span><strong className={cn("mt-0.5 block truncate font-mono text-xs", valueClass)}>{item.value}</strong></div>)}</div>;
}

function PriceComparison({ official, platform, language, t }: { official: NonNullable<PublicModelSummary["pricing"]>; platform?: PublicPlatformModelPrice; language: Language; t: (key: TranslationKey) => string }) {
  const officialPricing = officialPriceSummary(official);
  const platformPricing = platform ? platformPriceSummary(platform) : undefined;
  return <div className="space-y-2.5 rounded-xl border border-slate-200/80 bg-slate-50/70 p-3 dark:border-slate-800 dark:bg-slate-950/40">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <span className="text-xs font-semibold text-slate-700 dark:text-slate-200">{t("modelsOfficialPrice")} · {official.currency} / 1M</span>
      <span className="rounded-md bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:text-amber-300">{official.source === "litellm" ? t("modelsPricingReference") : t("modelsPricingManual")}</span>
    </div>
    <CompactPriceRow pricing={officialPricing} t={t} />
    {platform ? <div className="border-t border-slate-200 pt-2.5 dark:border-slate-800">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs font-semibold text-slate-700 dark:text-slate-200">{t("modelsPlatformPricing")}</span>
        <span className="font-mono text-[10px] text-indigo-600 dark:text-indigo-300">{platform.group_name} · x{formatMultiplier(platform.multiplier)}</span>
      </div>
      {platform.billing_type === "free" ? <div className="mt-1 text-[11px] font-medium text-emerald-700 dark:text-emerald-300">{t("modelsPlatformFree")}</div> : <div className="mt-2"><CompactPriceRow pricing={platformPricing!} tone="platform" t={t} /></div>}
    </div> : null}
    {official.updated_at ? <div className="text-[10px] text-slate-400">{t("modelsPricingUpdated")} {new Date(official.updated_at).toLocaleDateString(language === "zh" ? "zh-CN" : "en-US")}</div> : null}
  </div>;
}
