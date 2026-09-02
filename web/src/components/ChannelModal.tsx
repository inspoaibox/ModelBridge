import React from "react";
import {
  ArrowRight,
  Check,
  Cpu,
  Key,
  Network,
  Plus,
  RefreshCw,
  Save,
  Server,
  Sparkles,
  Trash2,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  ChannelFormModel,
  ChannelFormState,
  DiscoveredModel,
  Language,
  LoginMessage,
  TranslationKey,
} from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface ChannelModalProps {
  channelFormOpen: boolean;
  channelForm: ChannelFormState;
  setChannelForm: React.Dispatch<React.SetStateAction<ChannelFormState>>;
  closeChannelForm: () => void;
  handleChannelSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  channelActionBusy: string;
  modelDiscoveryBusy: boolean;
  discoverChannelModels: () => Promise<void>;
  discoveredModels: DiscoveredModel[];
  modelDiscoveryMessage: LoginMessage;
  addDiscoveredModel: (model: DiscoveredModel) => void;
  isChannelModelMapped: (modelID: string) => boolean;
  setChannelProvider: (provider: "openai" | "anthropic" | "grok" | "gemini" | "volcengine") => void;
  updateChannelModel: (rowID: string, patch: Partial<Omit<ChannelFormModel, "id">>) => void;
  addChannelModel: () => void;
  removeChannelModel: (rowID: string) => void;
  language: Language;
}

export function ChannelModal({
  channelFormOpen,
  channelForm,
  setChannelForm,
  closeChannelForm,
  handleChannelSubmit,
  channelActionBusy,
  modelDiscoveryBusy,
  discoverChannelModels,
  discoveredModels,
  modelDiscoveryMessage,
  addDiscoveredModel,
  isChannelModelMapped,
  setChannelProvider,
  updateChannelModel,
  addChannelModel,
  removeChannelModel,
  language,
}: ChannelModalProps) {
  if (!channelFormOpen) return null;

  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-6 overflow-y-auto bg-slate-950/60 backdrop-blur-md animate-in fade-in duration-200">
      {/* Background click dismiss */}
      <div
        className="fixed inset-0 cursor-default"
        onClick={() => {
          if (!channelActionBusy && !modelDiscoveryBusy) closeChannelForm();
        }}
      />

      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="channel-dialog-title"
        className="relative z-10 flex max-h-[92vh] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-slate-200 dark:border-slate-700/80 bg-white dark:bg-slate-900 shadow-2xl shadow-slate-900/30 dark:shadow-black/80 text-slate-900 dark:text-slate-100"
      >
        {/* Modal Header */}
        <div className="flex items-center justify-between border-b border-slate-200 dark:border-slate-800/80 bg-slate-50/80 dark:bg-slate-950/60 px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-cyan-500 text-white shadow-md shadow-indigo-500/20">
              <Network className="h-5 w-5" />
            </div>
            <div>
              <h2 id="channel-dialog-title" className="text-lg font-bold text-slate-900 dark:text-white tracking-tight">
                {channelForm.id ? t("channelsEditTitle") : t("channelsCreateTitle")}
              </h2>
              <p className="text-xs text-slate-500 dark:text-slate-400">{t("channelsFormModalHint")}</p>
            </div>
          </div>

          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={closeChannelForm}
            disabled={Boolean(channelActionBusy) || modelDiscoveryBusy}
            className="h-8.5 w-8.5 rounded-xl p-0 text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>

        {/* Modal Body / Scroll Area */}
        <div className="min-h-0 overflow-y-auto px-6 py-6 space-y-6">
          <form id="channel-form" className="space-y-6" onSubmit={handleChannelSubmit}>
            {/* Section 1: Basic Config */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-800/80 bg-slate-50/80 dark:bg-slate-950/40 p-5 space-y-4">
              <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-indigo-600 dark:text-indigo-400">
                <Server className="h-3.5 w-3.5" />
                <span>1. {language === "zh" ? "基础协议与状态" : "Basic Protocol & Status"}</span>
              </div>

              <div className="grid gap-4 sm:grid-cols-3">
                <div className="space-y-1.5 sm:col-span-1">
                  <Label htmlFor="channel-name" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                    {t("channelsFormName")} <span className="text-rose-500">*</span>
                  </Label>
                  <Input
                    id="channel-name"
                    value={channelForm.name}
                    onChange={(e) => setChannelForm((curr) => ({ ...curr, name: e.target.value }))}
                    placeholder="e.g. OpenAI Official US-East"
                    required
                  />
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="channel-provider" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                    {t("channelsFormProvider")} <span className="text-rose-500">*</span>
                  </Label>
                  <select
                    id="channel-provider"
                    className="flex h-10 w-full rounded-xl border border-slate-200 dark:border-slate-700/80 bg-white dark:bg-slate-900/90 px-3 py-2 text-sm text-slate-900 dark:text-slate-100 focus-visible:outline-none focus-visible:border-indigo-500 focus-visible:ring-2 focus-visible:ring-indigo-500/20"
                    value={channelForm.provider}
                    onChange={(e) => {
                      const provider = e.target.value;
                      setChannelProvider(
                        provider === "anthropic" ||
                          provider === "grok" ||
                          provider === "gemini" ||
                          provider === "volcengine"
                          ? provider
                          : "openai"
                      );
                    }}
                  >
                    <option value="openai">{t("channelsProviderOpenAI")}</option>
                    <option value="anthropic">{t("channelsProviderAnthropic")}</option>
                    <option value="grok">{t("channelsProviderGrok")}</option>
                    <option value="gemini">{t("channelsProviderGemini")}</option>
                    <option value="volcengine">{t("channelsProviderVolcengine")}</option>
                  </select>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="channel-status" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                    {t("channelsFormStatus")}
                  </Label>
                  <select
                    id="channel-status"
                    className="flex h-10 w-full rounded-xl border border-slate-200 dark:border-slate-700/80 bg-white dark:bg-slate-900/90 px-3 py-2 text-sm text-slate-900 dark:text-slate-100 focus-visible:outline-none focus-visible:border-indigo-500 focus-visible:ring-2 focus-visible:ring-indigo-500/20"
                    value={channelForm.status}
                    onChange={(e) =>
                      setChannelForm((curr) => ({
                        ...curr,
                        status:
                          e.target.value === "draining"
                            ? "draining"
                            : e.target.value === "disabled"
                            ? "disabled"
                            : "active",
                      }))
                    }
                  >
                    <option value="active">🟢 {t("channelsStatusActive")}</option>
                    <option value="draining">🟡 {t("channelsStatusDraining")}</option>
                    <option value="disabled">🔴 {t("channelsStatusDisabled")}</option>
                  </select>
                </div>
              </div>
            </div>

            {/* Section 2: Endpoint, API Key, Priority */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-800/80 bg-slate-50/80 dark:bg-slate-950/40 p-5 space-y-4">
              <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-cyan-600 dark:text-cyan-400">
                <Key className="h-3.5 w-3.5" />
                <span>2. {language === "zh" ? "接入凭据与调度权重" : "Credentials & Dispatch Weights"}</span>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="channel-base-url" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                    {t("channelsFormBaseURL")} <span className="text-rose-500">*</span>
                  </Label>
                  <Input
                    id="channel-base-url"
                    value={channelForm.base_url}
                    onChange={(e) => {
                      setChannelForm((curr) => ({ ...curr, base_url: e.target.value }));
                    }}
                    placeholder={
                      channelForm.provider === "grok"
                        ? "https://api.x.ai/v1"
                        : channelForm.provider === "gemini"
                        ? "https://generativelanguage.googleapis.com"
                        : channelForm.provider === "anthropic"
                        ? "https://api.anthropic.com"
                        : channelForm.provider === "volcengine"
                        ? "https://ark.cn-beijing.volces.com/api/v3"
                        : "https://api.openai.com/v1"
                    }
                    required
                  />
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="channel-api-key" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                    {t("channelsFormAPIKey")}{" "}
                    {!channelForm.id && <span className="text-rose-500">*</span>}
                  </Label>
                  <Input
                    id="channel-api-key"
                    type="password"
                    value={channelForm.api_key}
                    onChange={(e) => {
                      setChannelForm((curr) => ({ ...curr, api_key: e.target.value }));
                    }}
                    placeholder={channelForm.id ? t("channelsFormAPIKeyOptional") : "sk-..."}
                    autoComplete="new-password"
                  />
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="channel-priority" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                    {t("channelsFormPriority")}
                  </Label>
                  <Input
                    id="channel-priority"
                    type="number"
                    min={0}
                    max={10000}
                    value={channelForm.priority}
                    onChange={(e) => setChannelForm((curr) => ({ ...curr, priority: Number(e.target.value) }))}
                  />
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="channel-weight" className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                    {t("channelsFormWeight")}
                  </Label>
                  <Input
                    id="channel-weight"
                    type="number"
                    min={0}
                    max={10000}
                    value={channelForm.weight}
                    onChange={(e) => setChannelForm((curr) => ({ ...curr, weight: Number(e.target.value) }))}
                  />
                </div>
              </div>

              {channelForm.provider === "volcengine" && (
                <p className="rounded-lg border border-sky-500/20 bg-sky-50/70 px-3 py-2 text-xs text-sky-700 dark:border-sky-400/20 dark:bg-sky-500/10 dark:text-sky-300">
                  {language === "zh"
                    ? "火山方舟官方 Content Generation：支持 Seedance 2.0/2.5 视频异步任务。可填写根地址，系统会自动补全 /api/v3。"
                    : "Volcano Ark Content Generation: supports asynchronous Seedance 2.0/2.5 video tasks. The /api/v3 path is added automatically when needed."}
                </p>
              )}
            </div>

            {/* Section 3: Model Mapping & Upstream Discovery */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-800/80 bg-slate-50/80 dark:bg-slate-950/40 p-5 space-y-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-purple-600 dark:text-purple-400">
                  <Cpu className="h-3.5 w-3.5" />
                  <span>3. {t("channelsFormModels")}</span>
                </div>

                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={discoverChannelModels}
                    disabled={Boolean(channelActionBusy) || modelDiscoveryBusy}
                    className="h-8 text-xs border-indigo-500/40 text-indigo-600 dark:text-indigo-300 hover:bg-indigo-50 dark:hover:bg-indigo-500/10"
                  >
                    <RefreshCw className={cn("h-3.5 w-3.5 mr-1", modelDiscoveryBusy ? "animate-spin" : "")} />
                    <span>{modelDiscoveryBusy ? t("channelsFormDiscovering") : t("channelsFormDiscover")}</span>
                  </Button>

                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    onClick={addChannelModel}
                    disabled={Boolean(channelActionBusy)}
                    className="h-8 text-xs"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    <span>{t("channelsFormAddModel")}</span>
                  </Button>
                </div>
              </div>

              {/* Discovery Message / Notification */}
              {modelDiscoveryMessage.text && (
                <div
                  className={cn("rounded-lg border px-3.5 py-2 text-xs flex items-center gap-2", {
                    "border-emerald-500/30 bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300": modelDiscoveryMessage.kind === "success",
                    "border-amber-500/30 bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300": modelDiscoveryMessage.kind === "pending",
                    "border-rose-500/30 bg-rose-50 dark:bg-rose-500/10 text-rose-700 dark:text-rose-300": modelDiscoveryMessage.kind === "error",
                  })}
                >
                  <Sparkles className="h-3.5 w-3.5 shrink-0" />
                  <span>{modelDiscoveryMessage.text}</span>
                </div>
              )}

              {/* Discovered Models Chips */}
              {discoveredModels.length > 0 && (
                <div className="rounded-xl border border-indigo-500/20 bg-indigo-50/80 dark:bg-indigo-950/20 p-3 space-y-2">
                  <div className="text-[11px] font-bold uppercase tracking-wider text-indigo-700 dark:text-indigo-300 flex items-center justify-between">
                    <span>{t("channelsFormDiscoveredTitle")} ({discoveredModels.length})</span>
                    <span className="text-[10px] text-slate-500 dark:text-slate-400">{language === "zh" ? "点击快速加入映射" : "Click to map"}</span>
                  </div>
                  <div className="flex flex-wrap gap-1.5 max-h-36 overflow-y-auto pr-1">
                    {discoveredModels.map((model) => {
                      const added = isChannelModelMapped(model.id);
                      return (
                        <button
                          key={model.id}
                          type="button"
                          disabled={added || Boolean(channelActionBusy)}
                          onClick={() => addDiscoveredModel(model)}
                          className={cn(
                            "inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-mono transition-all",
                            added
                              ? "bg-slate-200/80 dark:bg-slate-800/80 text-slate-500 dark:text-slate-400 border border-slate-300 dark:border-slate-700/60 cursor-default"
                              : "bg-indigo-50 dark:bg-indigo-500/15 text-indigo-700 dark:text-indigo-200 border border-indigo-500/30 hover:bg-indigo-100 dark:hover:bg-indigo-500/25 hover:border-indigo-400 cursor-pointer shadow-sm"
                          )}
                        >
                          <span>{model.id}</span>
                          {added ? <Check className="h-3 w-3 text-emerald-600 dark:text-emerald-400" /> : <Plus className="h-3 w-3" />}
                        </button>
                      );
                    })}
                  </div>
                </div>
              )}

              {/* Model Mapping Rows */}
              <div className="space-y-2.5">
                {channelForm.models.length === 0 && (
                  <div className="rounded-xl border border-dashed border-slate-300 bg-white/60 px-4 py-8 text-center text-sm text-slate-500 dark:border-slate-700 dark:bg-slate-900/40 dark:text-slate-400">
                    {t("channelsFormNoModelsHint")}
                  </div>
                )}
                {channelForm.models.map((model) => (
                  <div
                    key={model.id}
                    className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2.5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/80 p-3 shadow-sm"
                  >
                    <div className="flex-1 space-y-1">
                      <div className="text-[10px] uppercase font-semibold text-slate-500 dark:text-slate-400">{t("channelsFormModel")} (Alias)</div>
                      <Input
                        value={model.model}
                        onChange={(e) => updateChannelModel(model.id, { model: e.target.value })}
                        placeholder="e.g. gpt-4o"
                        className="h-8 text-xs font-mono"
                      />
                    </div>

                    <div className="hidden sm:flex items-center justify-center pt-4 text-slate-400">
                      <ArrowRight className="h-4 w-4" />
                    </div>

                    <div className="flex-1 space-y-1">
                      <div className="text-[10px] uppercase font-semibold text-slate-500 dark:text-slate-400">{t("channelsFormUpstream")} (Target)</div>
                      <Input
                        value={model.upstream_model}
                        onChange={(e) => updateChannelModel(model.id, { upstream_model: e.target.value })}
                        placeholder="e.g. gpt-4o-2024-08-06"
                        className="h-8 text-xs font-mono"
                      />
                    </div>

                    <div className="flex items-center justify-between sm:justify-end gap-3 pt-2 sm:pt-4 sm:pl-2">
                      <label className="flex items-center gap-1.5 text-xs text-slate-700 dark:text-slate-300 cursor-pointer">
                        <Switch
                          checked={model.enabled}
                          onCheckedChange={(checked) => updateChannelModel(model.id, { enabled: checked })}
                        />
                        <span className="text-[11px] font-medium">{model.enabled ? (language === "zh" ? "开放" : "Visible") : (language === "zh" ? "隐藏" : "Hidden")}</span>
                      </label>

                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => removeChannelModel(model.id)}
                        disabled={Boolean(channelActionBusy)}
                        className="h-8 w-8 p-0 text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-500/10"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </form>
        </div>

        {/* Modal Footer */}
        <div className="flex items-center justify-between border-t border-slate-200 dark:border-slate-800/80 bg-slate-50/80 dark:bg-slate-950/60 px-6 py-4">
          <div className="text-xs text-slate-500">
            {language === "zh" ? "渠道可以先保存，模型映射可随后手动添加或自动获取" : "Save the channel first, then add or discover model mappings"}
          </div>

          <div className="flex items-center gap-3">
            <Button
              type="button"
              variant="outline"
              onClick={closeChannelForm}
              disabled={Boolean(channelActionBusy) || modelDiscoveryBusy}
            >
              {t("channelsCancel")}
            </Button>
            <Button
              type="submit"
              form="channel-form"
              disabled={Boolean(channelActionBusy) || modelDiscoveryBusy}
              className="gap-2 shadow-md shadow-indigo-500/25"
            >
              <Save className="h-4 w-4" />
              <span>{t("channelsSave")}</span>
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
