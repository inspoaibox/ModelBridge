import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  ArrowLeft,
  Check,
  CircleAlert,
  Code2,
  Copy,
  Download,
  Image,
  KeyRound,
  LoaderCircle,
  Play,
  Video,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { resolveAPIEndpointURLs } from "@/lib/api-endpoint";
import { cn } from "@/lib/utils";
import { Language, PublicAPIEndpoint, PublicModelSummary, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";

type LabKind = "image" | "video";

export interface MediaLabProps {
  language: Language;
  models: PublicModelSummary[];
  apiEndpoints: PublicAPIEndpoint[];
  routeTo: (target: string) => void;
  embedded?: boolean;
}

interface EndpointOption {
  label: string;
  root: string;
}

interface ResponseSnapshot {
  status: number;
  latency: number;
  requestID: string;
  raw: string;
}

interface VideoJob {
  id: string;
  status: string;
  progress?: string;
  baseURL: string;
  apiKey: string;
  createdAt: number;
}

interface GeneratedImage {
  src: string;
  label: string;
}

type JSONRecord = Record<string, unknown>;

interface SeedanceSpec {
  version: string;
  defaultDuration: number;
  maxDuration: number;
  maxReferenceImages: number;
  maxReferenceVideos: number;
  maxReferenceAudios: number;
  audioOnlyReference: boolean;
  resolutions: string[];
  supportsOutputFormat: boolean;
  supportsOmniTaskType: boolean;
  supportsReturnLastFrame: boolean;
}

export function ImageLabView(props: MediaLabProps) {
  return <MediaLab {...props} kind="image" />;
}

export function VideoLabView(props: MediaLabProps) {
  return <MediaLab {...props} kind="video" />;
}

export function MediaLab({ language, models, apiEndpoints, routeTo, embedded = false, kind }: MediaLabProps & { kind: LabKind }) {
  const t = useCallback((key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key, [language]);
  const endpointOptions = useMemo(() => endpointChoices(apiEndpoints, t("mediaLabCurrentGateway")), [apiEndpoints, t]);
  const availableModels = useMemo(
    () => models.filter((model) => model.category === kind && model.available).sort((left, right) => left.name.localeCompare(right.name)),
    [kind, models],
  );
  const [endpointRoot, setEndpointRoot] = useState(() => endpointOptions[0]?.root || window.location.origin);
  const [apiKey, setAPIKey] = useState("");
  const [model, setModel] = useState("");
  const [prompt, setPrompt] = useState("");
  const [imageCount, setImageCount] = useState("1");
  const [imageSize, setImageSize] = useState("1024x1024");
  const [imageQuality, setImageQuality] = useState("standard");
  const [imageFormat, setImageFormat] = useState("url");
  const [videoSeconds, setVideoSeconds] = useState("8");
  const [videoResolution, setVideoResolution] = useState("");
  const [videoRatio, setVideoRatio] = useState("");
  const [videoTaskType, setVideoTaskType] = useState("");
  const [videoGenerateAudio, setVideoGenerateAudio] = useState(false);
  const [videoReturnLastFrame, setVideoReturnLastFrame] = useState(false);
  const [videoOutputFormat, setVideoOutputFormat] = useState("");
  const [videoExecutionExpiry, setVideoExecutionExpiry] = useState("");
  const [videoPriority, setVideoPriority] = useState("");
  const [videoWebSearch, setVideoWebSearch] = useState(false);
  const [videoFirstFrameURL, setVideoFirstFrameURL] = useState("");
  const [videoLastFrameURL, setVideoLastFrameURL] = useState("");
  const [videoReferenceImages, setVideoReferenceImages] = useState("");
  const [videoReferenceVideos, setVideoReferenceVideos] = useState("");
  const [videoReferenceAudios, setVideoReferenceAudios] = useState("");
  const [busy, setBusy] = useState(false);
  const [copyState, setCopyState] = useState("");
  const [error, setError] = useState("");
  const [response, setResponse] = useState<ResponseSnapshot | null>(null);
  const [images, setImages] = useState<GeneratedImage[]>([]);
  const [videoJob, setVideoJob] = useState<VideoJob | null>(null);
  const [videoBlobURL, setVideoBlobURL] = useState("");
  const [videoLoading, setVideoLoading] = useState(false);
  const blobURLRef = useRef("");
  const selectedModel = availableModels.find((item) => item.name === model);
  const seedanceSpec = selectedModel ? seedanceCapability(selectedModel) : null;

  useEffect(() => {
    if (endpointOptions.some((item) => item.root === endpointRoot)) return;
    setEndpointRoot(endpointOptions[0]?.root || window.location.origin);
  }, [endpointOptions, endpointRoot]);

  useEffect(() => {
    if (availableModels.some((item) => item.name === model)) return;
    setModel(availableModels[0]?.name || "");
  }, [availableModels, model]);

  useEffect(() => {
    if (kind !== "video") return;
    setVideoSeconds(seedanceSpec ? String(seedanceSpec.defaultDuration) : "8");
    if (!seedanceSpec?.supportsOmniTaskType) setVideoTaskType("");
    if (!seedanceSpec?.supportsOutputFormat) setVideoOutputFormat("");
  }, [kind, seedanceSpec?.supportsOmniTaskType, seedanceSpec?.supportsOutputFormat, seedanceSpec?.defaultDuration, seedanceSpec?.version]);

  useEffect(() => () => {
    if (blobURLRef.current) URL.revokeObjectURL(blobURLRef.current);
  }, []);

  useEffect(() => {
    if (!videoJob || !shouldPoll(videoJob.status)) return;
    let cancelled = false;
    const timer = window.setTimeout(async () => {
      setVideoLoading(true);
      try {
        const next = await requestJSON(`${videoJob.baseURL}/v1/videos/${encodeURIComponent(videoJob.id)}`, videoJob.apiKey);
        if (cancelled) return;
        setResponse(next.snapshot);
        if (!next.ok) {
          setError(errorFromResponse(next.value) || t("mediaLabRequestFailed"));
          setVideoJob((current) => current ? { ...current, status: "failed" } : current);
          return;
        }
        const video = asRecord(next.value);
        const status = videoStatus(video?.status);
        const progress = video?.progress === undefined ? undefined : String(video.progress);
        setVideoJob((current) => current ? { ...current, status, progress } : current);
      } catch {
        if (!cancelled) setError(t("mediaLabRequestFailed"));
      } finally {
        if (!cancelled) setVideoLoading(false);
      }
    }, 3000);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [t, videoJob]);

  useEffect(() => {
    if (!videoJob || videoJob.status !== "completed" || videoBlobURL || videoLoading) return;
    let cancelled = false;
    const load = async () => {
      setVideoLoading(true);
      try {
        const started = performance.now();
        const result = await fetch(`${videoJob.baseURL}/v1/videos/${encodeURIComponent(videoJob.id)}/content`, {
          headers: { Authorization: `Bearer ${videoJob.apiKey}` },
        });
        const body = result.ok ? await result.blob() : null;
        const raw = result.ok ? "{\n  \"content\": \"video binary loaded\"\n}" : await result.text();
        if (cancelled) return;
        setResponse({
          status: result.status,
          latency: Math.round(performance.now() - started),
          requestID: result.headers.get("x-request-id") || "",
          raw,
        });
        if (!result.ok) {
          setError(t("mediaLabRequestFailed"));
          return;
        }
        if (blobURLRef.current) URL.revokeObjectURL(blobURLRef.current);
        const nextURL = URL.createObjectURL(body!);
        blobURLRef.current = nextURL;
        setVideoBlobURL(nextURL);
      } catch {
        if (!cancelled) setError(t("mediaLabRequestFailed"));
      } finally {
        if (!cancelled) setVideoLoading(false);
      }
    };
    void load();
    return () => { cancelled = true; };
  }, [t, videoBlobURL, videoJob, videoLoading]);

  const seedanceContent = [
    ...(prompt.trim() ? [{ type: "text", text: prompt.trim() }] : []),
    ...(videoFirstFrameURL.trim() ? [{ type: "image_url", role: "first_frame", image_url: { url: videoFirstFrameURL.trim() } }] : []),
    ...(videoLastFrameURL.trim() ? [{ type: "image_url", role: "last_frame", image_url: { url: videoLastFrameURL.trim() } }] : []),
    ...videoReferenceImages.split(/\r?\n/).map((url) => url.trim()).filter(Boolean).map((url) => ({ type: "image_url", role: "reference_image", image_url: { url } })),
    ...videoReferenceVideos.split(/\r?\n/).map((url) => url.trim()).filter(Boolean).map((url) => ({ type: "video_url", role: "reference_video", video_url: { url } })),
    ...videoReferenceAudios.split(/\r?\n/).map((url) => url.trim()).filter(Boolean).map((url) => ({ type: "audio_url", role: "reference_audio", audio_url: { url } })),
  ];
  const videoPayload = cleanPayload({
    model,
    prompt: seedanceSpec ? undefined : prompt,
    seconds: Number(videoSeconds),
    content: seedanceSpec && seedanceContent.length > 0 ? seedanceContent : undefined,
    resolution: seedanceSpec ? videoResolution : undefined,
    ratio: seedanceSpec ? videoRatio : undefined,
    omni_reference_task_type: seedanceSpec ? videoTaskType : undefined,
    generate_audio: seedanceSpec && videoGenerateAudio ? true : undefined,
    return_last_frame: seedanceSpec && videoReturnLastFrame ? true : undefined,
    output_format: seedanceSpec ? videoOutputFormat : undefined,
    execution_expires_after: seedanceSpec && videoExecutionExpiry ? Number(videoExecutionExpiry) : undefined,
    priority: seedanceSpec && videoPriority ? Number(videoPriority) : undefined,
    tools: seedanceSpec && videoWebSearch ? [{ type: "web_search" }] : undefined,
  });
  const videoHasReferences = seedanceContent.some((item) => item.type !== "text");
  const payload = kind === "image"
    ? cleanPayload({ model, prompt, n: Number(imageCount), size: imageSize, quality: imageQuality, response_format: imageFormat })
    : videoPayload;
  const endpoint = `${endpointRoot.replace(/\/+$/, "")}/v1/${kind === "image" ? "images/generations" : "videos"}`;
  const curl = curlExample(endpoint, payload);

  async function copy(value: string, target: string) {
    try {
      await navigator.clipboard.writeText(value);
      setCopyState(target);
      window.setTimeout(() => setCopyState(""), 1500);
    } catch {
      setError(t("mediaLabRequestFailed"));
    }
  }

  async function submit() {
    setError("");
    setImages([]);
    setResponse(null);
    if (!apiKey.trim()) {
      setError(t("mediaLabKeyRequired"));
      return;
    }
    if (!prompt.trim() && !(seedanceSpec && videoHasReferences)) {
      setError(t("mediaLabPromptRequired"));
      return;
    }
    if (!model) {
      setError(t("mediaLabNoModels"));
      return;
    }
    if (kind === "video") {
      if (blobURLRef.current) URL.revokeObjectURL(blobURLRef.current);
      blobURLRef.current = "";
      setVideoBlobURL("");
      setVideoJob(null);
    }
    setBusy(true);
    try {
      const result = await requestJSON(endpoint, apiKey, payload);
      setResponse(result.snapshot);
      if (!result.ok) {
        setError(errorFromResponse(result.value) || t("mediaLabRequestFailed"));
        return;
      }
      if (kind === "image") {
        const nextImages = extractImages(result.value);
        setImages(nextImages);
        if (nextImages.length === 0) setError(t("mediaLabNoPreview"));
        return;
      }
      const video = asRecord(result.value);
      const jobID = typeof video?.id === "string" ? video.id.trim() : "";
      if (!jobID) {
        setError(t("mediaLabRequestFailed"));
        return;
      }
      setVideoJob({
        id: jobID,
        status: videoStatus(video?.status),
        progress: video?.progress === undefined ? undefined : String(video.progress),
        baseURL: endpointRoot.replace(/\/+$/, ""),
        apiKey,
        createdAt: Date.now(),
      });
    } catch {
      setError(t("mediaLabRequestFailed"));
    } finally {
      setBusy(false);
    }
  }

  const isImage = kind === "image";
  const Icon = isImage ? Image : Video;
  const title = isImage ? t("mediaLabImageTitle") : t("mediaLabVideoTitle");
  const description = isImage ? t("mediaLabImageDescription") : t("mediaLabVideoDescription");
  const action = isImage ? t("mediaLabGenerateImage") : t("mediaLabCreateVideo");
  const actionBusy = isImage ? t("mediaLabGeneratingImage") : t("mediaLabCreatingVideo");

  return (
    <div className={embedded ? "space-y-5" : "min-h-[calc(100vh-72px)] bg-slate-50 px-4 py-6 dark:bg-slate-950 sm:px-6 lg:px-10"}>
      <div className="mx-auto max-w-[1480px] space-y-5">
        <section className={cn("overflow-hidden border px-5 py-6 shadow-sm sm:px-7", isImage ? "border-fuchsia-500/20 bg-fuchsia-500/[0.045] dark:border-fuchsia-400/20" : "border-cyan-500/20 bg-cyan-500/[0.045] dark:border-cyan-400/20")}>
          {!embedded ? <Button type="button" variant="ghost" size="sm" onClick={() => routeTo("#models")} className="-ml-2 mb-5 gap-1.5 text-slate-600 dark:text-slate-300"><ArrowLeft className="h-4 w-4" />{t("mediaLabBackToModels")}</Button> : null}
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div className="max-w-3xl"><div className={cn("flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em]", isImage ? "text-fuchsia-700 dark:text-fuchsia-300" : "text-cyan-700 dark:text-cyan-300")}><Icon className="h-4 w-4" />{isImage ? t("mediaLabImageEyebrow") : t("mediaLabVideoEyebrow")}</div><h1 className="mt-3 text-3xl font-bold text-slate-950 dark:text-white">{title}</h1><p className="mt-3 text-sm leading-6 text-slate-600 dark:text-slate-400">{description}</p></div>
            <div className="max-w-sm rounded-lg border border-slate-200/80 bg-white/80 px-4 py-3 text-xs leading-5 text-slate-600 dark:border-slate-700 dark:bg-slate-900/70 dark:text-slate-300"><KeyRound className="mr-2 inline h-4 w-4 text-emerald-600 dark:text-emerald-300" />{t("mediaLabKeyNotice")}</div>
          </div>
        </section>

        <div className="grid gap-5 xl:grid-cols-[minmax(0,1.08fr)_minmax(420px,0.92fr)]">
          <section className="space-y-5">
            <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/70">
              <CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Icon className={cn("h-5 w-5", isImage ? "text-fuchsia-600 dark:text-fuchsia-300" : "text-cyan-600 dark:text-cyan-300")} />{title}</CardTitle><CardDescription>{t("mediaLabKeyNotice")}</CardDescription></CardHeader>
              <CardContent className="space-y-5">
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label={t("mediaLabEndpoint")}><select value={endpointRoot} onChange={(event) => setEndpointRoot(event.target.value)} className={selectClass}>{endpointOptions.map((item) => <option key={item.root} value={item.root}>{item.label}</option>)}</select></Field>
                  <Field label={t("mediaLabModel")}><select value={model} onChange={(event) => setModel(event.target.value)} className={selectClass} disabled={availableModels.length === 0}>{availableModels.length === 0 ? <option value="">{t("mediaLabNoModels")}</option> : availableModels.map((item) => <option key={item.id} value={item.name}>{item.display_name} ({item.provider})</option>)}</select></Field>
                </div>
                <Field label={t("mediaLabApiKey")}><Input type="password" value={apiKey} onChange={(event) => setAPIKey(event.target.value)} placeholder={t("mediaLabApiKeyPlaceholder")} autoComplete="off" /></Field>
                <Field label={t("mediaLabPrompt")}><textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder={t("mediaLabPromptPlaceholder")} className="min-h-32 w-full resize-y rounded-md border border-slate-200 bg-white px-3 py-2.5 text-sm leading-6 text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/15 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100" /></Field>
                {isImage ? <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4"><Field label={t("mediaLabImageCount")}><select value={imageCount} onChange={(event) => setImageCount(event.target.value)} className={selectClass}><option value="1">1</option><option value="2">2</option><option value="4">4</option></select></Field><Field label={t("mediaLabImageSize")}><select value={imageSize} onChange={(event) => setImageSize(event.target.value)} className={selectClass}><option value="1024x1024">1024 x 1024</option><option value="1536x1024">1536 x 1024</option><option value="1024x1536">1024 x 1536</option></select></Field><Field label={t("mediaLabImageQuality")}><select value={imageQuality} onChange={(event) => setImageQuality(event.target.value)} className={selectClass}><option value="standard">standard</option><option value="hd">hd</option></select></Field><Field label={t("mediaLabResponseFormat")}><select value={imageFormat} onChange={(event) => setImageFormat(event.target.value)} className={selectClass}><option value="url">url</option><option value="b64_json">b64_json</option></select></Field></div> : <VideoControls t={t} seedance={seedanceSpec} videoSeconds={videoSeconds} setVideoSeconds={setVideoSeconds} videoResolution={videoResolution} setVideoResolution={setVideoResolution} videoRatio={videoRatio} setVideoRatio={setVideoRatio} videoTaskType={videoTaskType} setVideoTaskType={setVideoTaskType} videoGenerateAudio={videoGenerateAudio} setVideoGenerateAudio={setVideoGenerateAudio} videoReturnLastFrame={videoReturnLastFrame} setVideoReturnLastFrame={setVideoReturnLastFrame} videoOutputFormat={videoOutputFormat} setVideoOutputFormat={setVideoOutputFormat} videoExecutionExpiry={videoExecutionExpiry} setVideoExecutionExpiry={setVideoExecutionExpiry} videoPriority={videoPriority} setVideoPriority={setVideoPriority} videoWebSearch={videoWebSearch} setVideoWebSearch={setVideoWebSearch} videoFirstFrameURL={videoFirstFrameURL} setVideoFirstFrameURL={setVideoFirstFrameURL} videoLastFrameURL={videoLastFrameURL} setVideoLastFrameURL={setVideoLastFrameURL} videoReferenceImages={videoReferenceImages} setVideoReferenceImages={setVideoReferenceImages} videoReferenceVideos={videoReferenceVideos} setVideoReferenceVideos={setVideoReferenceVideos} videoReferenceAudios={videoReferenceAudios} setVideoReferenceAudios={setVideoReferenceAudios} />}
                {!isImage && seedanceSpec ? <SeedanceGuide t={t} model={selectedModel} spec={seedanceSpec} /> : null}
                {error ? <div className="flex items-start gap-2 rounded-md border border-rose-500/30 bg-rose-50 p-3 text-sm text-rose-700 dark:bg-rose-500/10 dark:text-rose-300"><CircleAlert className="mt-0.5 h-4 w-4 shrink-0" />{error}</div> : null}
                <Button type="button" onClick={() => void submit()} disabled={busy || availableModels.length === 0} className={cn("w-full gap-2 sm:w-auto", isImage ? "bg-fuchsia-600 hover:bg-fuchsia-700" : "bg-cyan-600 hover:bg-cyan-700")}><Play className="h-4 w-4" />{busy ? <><LoaderCircle className="h-4 w-4 animate-spin" />{actionBusy}</> : action}</Button>
              </CardContent>
            </Card>
            <RequestDetails t={t} payload={payload} curl={curl} copy={copy} copyState={copyState} />
          </section>
          <section className="space-y-5">
            {isImage ? <ImagePreview t={t} images={images} response={response} /> : <VideoPreview t={t} job={videoJob} blobURL={videoBlobURL} loading={videoLoading} response={response} />}
            <ResponsePanel t={t} response={response} />
          </section>
        </div>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block space-y-2"><span className="text-xs font-semibold text-slate-700 dark:text-slate-300">{label}</span>{children}</label>;
}

function VideoControls({
  t, seedance, videoSeconds, setVideoSeconds, videoResolution, setVideoResolution, videoRatio, setVideoRatio,
  videoTaskType, setVideoTaskType, videoGenerateAudio, setVideoGenerateAudio, videoReturnLastFrame, setVideoReturnLastFrame,
  videoOutputFormat, setVideoOutputFormat, videoExecutionExpiry, setVideoExecutionExpiry, videoPriority, setVideoPriority,
  videoWebSearch, setVideoWebSearch, videoFirstFrameURL, setVideoFirstFrameURL, videoLastFrameURL, setVideoLastFrameURL,
  videoReferenceImages, setVideoReferenceImages, videoReferenceVideos, setVideoReferenceVideos, videoReferenceAudios, setVideoReferenceAudios,
}: {
  t: (key: TranslationKey) => string;
  seedance: SeedanceSpec | null;
  videoSeconds: string; setVideoSeconds: (value: string) => void;
  videoResolution: string; setVideoResolution: (value: string) => void;
  videoRatio: string; setVideoRatio: (value: string) => void;
  videoTaskType: string; setVideoTaskType: (value: string) => void;
  videoGenerateAudio: boolean; setVideoGenerateAudio: (value: boolean) => void;
  videoReturnLastFrame: boolean; setVideoReturnLastFrame: (value: boolean) => void;
  videoOutputFormat: string; setVideoOutputFormat: (value: string) => void;
  videoExecutionExpiry: string; setVideoExecutionExpiry: (value: string) => void;
  videoPriority: string; setVideoPriority: (value: string) => void;
  videoWebSearch: boolean; setVideoWebSearch: (value: boolean) => void;
  videoFirstFrameURL: string; setVideoFirstFrameURL: (value: string) => void;
  videoLastFrameURL: string; setVideoLastFrameURL: (value: string) => void;
  videoReferenceImages: string; setVideoReferenceImages: (value: string) => void;
  videoReferenceVideos: string; setVideoReferenceVideos: (value: string) => void;
  videoReferenceAudios: string; setVideoReferenceAudios: (value: string) => void;
}) {
  const durations = seedance ? [...(seedance.version === "2.5" ? ["-1"] : []), ...Array.from({ length: seedance.maxDuration - 3 }, (_, index) => String(index + 4))] : ["4", "8", "12"];
  return <div className="space-y-4">
    <div className="max-w-xs"><Field label={t("mediaLabVideoDuration")}><select value={videoSeconds} onChange={(event) => setVideoSeconds(event.target.value)} className={selectClass}>{durations.map((value) => <option key={value} value={value}>{value === "-1" ? t("mediaLabVideoAutoDuration") : `${value} s`}</option>)}</select></Field></div>
    {seedance ? <section className="space-y-4 rounded-xl border border-cyan-500/20 bg-cyan-500/[0.04] p-4 dark:border-cyan-400/20 dark:bg-cyan-500/[0.06]">
      <div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("mediaLabSeedanceAdvancedTitle")}</div><p className="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-400">{t("mediaLabSeedanceAdvancedHint")}</p></div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("mediaLabSeedanceResolution")}><select value={videoResolution} onChange={(event) => setVideoResolution(event.target.value)} className={selectClass}><option value="">{t("mediaLabDefaultOption")}</option>{seedance.resolutions.map((value) => <option key={value} value={value}>{value}</option>)}</select></Field>
        <Field label={t("mediaLabSeedanceRatio")}><select value={videoRatio} onChange={(event) => setVideoRatio(event.target.value)} className={selectClass}><option value="">{t("mediaLabDefaultOption")}</option>{["adaptive", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9"].map((value) => <option key={value} value={value}>{value}</option>)}</select></Field>
        {seedance.supportsOmniTaskType ? <Field label={t("mediaLabSeedanceTaskType")}><select value={videoTaskType} onChange={(event) => setVideoTaskType(event.target.value)} className={selectClass}><option value="">{t("mediaLabDefaultOption")}</option><option value="auto">auto</option><option value="reference">reference</option><option value="edit">edit</option><option value="extend">extend</option></select></Field> : null}
        {seedance.supportsOutputFormat ? <Field label={t("mediaLabSeedanceOutputFormat")}><select value={videoOutputFormat} onChange={(event) => setVideoOutputFormat(event.target.value)} className={selectClass}><option value="">mp4</option><option value="mp4">mp4</option><option value="mov">mov</option></select></Field> : null}
        <Field label={t("mediaLabSeedanceExecutionExpiry")}><Input type="number" min={3600} max={259200} value={videoExecutionExpiry} onChange={(event) => setVideoExecutionExpiry(event.target.value)} placeholder="3600" /></Field>
        <Field label={t("mediaLabSeedancePriority")}><select value={videoPriority} onChange={(event) => setVideoPriority(event.target.value)} className={selectClass}><option value="">{t("mediaLabDefaultOption")}</option>{[0, 1, 2, 3, 4, 5, 6, 7, 8, 9].map((value) => <option key={value} value={String(value)}>{value}</option>)}</select></Field>
      </div>
      <div className="grid gap-2 sm:grid-cols-3"><Toggle label={t("mediaLabSeedanceGenerateAudio")} checked={videoGenerateAudio} onChange={setVideoGenerateAudio} /><Toggle label={t("mediaLabSeedanceReturnLastFrame")} checked={videoReturnLastFrame} onChange={setVideoReturnLastFrame} disabled={!seedance.supportsReturnLastFrame} /><Toggle label={t("mediaLabSeedanceWebSearch")} checked={videoWebSearch} onChange={setVideoWebSearch} /></div>
      <div className="grid gap-4 sm:grid-cols-2"><URLField label={t("mediaLabSeedanceFirstFrame")} value={videoFirstFrameURL} onChange={setVideoFirstFrameURL} placeholder={t("mediaLabSeedanceURLPlaceholder")} /><URLField label={t("mediaLabSeedanceLastFrame")} value={videoLastFrameURL} onChange={setVideoLastFrameURL} placeholder={t("mediaLabSeedanceURLPlaceholder")} /><URLField label={`${t("mediaLabSeedanceReferenceImages")} (${seedance.maxReferenceImages})`} value={videoReferenceImages} onChange={setVideoReferenceImages} placeholder={t("mediaLabSeedanceURLsPlaceholder")} /><URLField label={`${t("mediaLabSeedanceReferenceVideos")} (${seedance.maxReferenceVideos})`} value={videoReferenceVideos} onChange={setVideoReferenceVideos} placeholder={t("mediaLabSeedanceURLsPlaceholder")} /><URLField label={`${t("mediaLabSeedanceReferenceAudios")} (${seedance.maxReferenceAudios})`} value={videoReferenceAudios} onChange={setVideoReferenceAudios} placeholder={t("mediaLabSeedanceURLsPlaceholder")} /></div>
    </section> : null}
  </div>;
}

function Toggle({ label, checked, onChange, disabled = false }: { label: string; checked: boolean; onChange: (checked: boolean) => void; disabled?: boolean }) {
  return <label className={cn("flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-700 dark:border-slate-700 dark:bg-slate-950/40 dark:text-slate-200", disabled && "opacity-50")}><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} disabled={disabled} className="h-4 w-4 accent-cyan-600" />{label}</label>;
}

function URLField({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (value: string) => void; placeholder: string }) {
  return <label className="block space-y-2 sm:col-span-1"><span className="text-xs font-semibold text-slate-700 dark:text-slate-300">{label}</span><textarea value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} className="min-h-20 w-full resize-y rounded-md border border-slate-200 bg-white px-3 py-2 text-xs leading-5 text-slate-900 outline-none focus:border-cyan-500 focus:ring-2 focus:ring-cyan-500/15 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100" /></label>;
}

function SeedanceGuide({ t, model, spec }: { t: (key: TranslationKey) => string; model?: PublicModelSummary; spec: SeedanceSpec }) {
  return <div className="space-y-3 rounded-xl border border-indigo-500/20 bg-indigo-500/[0.04] p-4 dark:border-indigo-400/20 dark:bg-indigo-500/[0.06]"><div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm font-semibold text-indigo-900 dark:text-indigo-100"><span>{t("mediaLabSeedanceGuideTitle")}</span><code className="rounded bg-white/70 px-2 py-1 font-mono text-xs dark:bg-slate-950/50">{model?.name || "-"}</code><span>Seedance {spec.version}</span></div><div className="grid gap-2 text-xs leading-5 text-slate-600 dark:text-slate-300 sm:grid-cols-2"><span>{t("mediaLabSeedanceDurationRule")} 4-{spec.maxDuration}s{spec.version === "2.5" ? ` / ${t("mediaLabVideoAutoDuration")}` : ""}</span><span>{t("mediaLabSeedanceResolutionRule")} {spec.resolutions.join(", ")}</span><span>{t("mediaLabSeedanceReferenceRule")} {spec.maxReferenceImages} {t("mediaLabSeedanceImageUnit")} · {spec.maxReferenceVideos} {t("mediaLabSeedanceVideoUnit")} · {spec.maxReferenceAudios} {t("mediaLabSeedanceAudioUnit")}</span><span>{spec.audioOnlyReference ? t("mediaLabSeedanceAudioOnly") : t("mediaLabSeedanceVisualRequired")}</span></div><p className="text-xs leading-5 text-slate-600 dark:text-slate-300">{t("mediaLabSeedanceModelIDRule")}</p><p className="text-xs leading-5 text-slate-600 dark:text-slate-300">{t("mediaLabSeedanceContentRule")}</p><p className="text-xs leading-5 text-slate-600 dark:text-slate-300">{spec.version === "2.5" ? t("mediaLabSeedance25TaskRule") : t("mediaLabSeedance20TaskRule")}</p></div>;
}

function RequestDetails({ t, payload, curl, copy, copyState }: { t: (key: TranslationKey) => string; payload: Record<string, unknown>; curl: string; copy: (value: string, target: string) => Promise<void>; copyState: string }) {
  const body = JSON.stringify(payload, null, 2);
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/70"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Code2 className="h-5 w-5 text-indigo-600 dark:text-indigo-300" />{t("mediaLabRequestDetails")}</CardTitle></CardHeader><CardContent className="space-y-4"><Snippet title={t("mediaLabRequestBody")} value={body} target="body" t={t} copy={copy} copied={copyState === "body"} /><Snippet title={t("mediaLabCurlExample")} value={curl} target="curl" t={t} copy={copy} copied={copyState === "curl"} /></CardContent></Card>;
}

function Snippet({ title, value, target, t, copy, copied }: { title: string; value: string; target: string; t: (key: TranslationKey) => string; copy: (value: string, target: string) => Promise<void>; copied: boolean }) {
  return <div className="overflow-hidden border border-slate-200 dark:border-slate-800"><div className="flex items-center justify-between border-b border-slate-200 bg-slate-50 px-3 py-2 dark:border-slate-800 dark:bg-slate-950/60"><span className="text-xs font-semibold text-slate-600 dark:text-slate-300">{title}</span><Button type="button" variant="ghost" size="sm" onClick={() => void copy(value, target)} className="h-7 gap-1 px-2 text-xs">{copied ? <Check className="h-3.5 w-3.5 text-emerald-600" /> : <Copy className="h-3.5 w-3.5" />}{copied ? t("mediaLabCopied") : t("mediaLabCopy")}</Button></div><pre className="max-h-64 overflow-auto bg-slate-950 p-3 text-xs leading-5 text-slate-100"><code>{value}</code></pre></div>;
}

function ImagePreview({ t, images, response }: { t: (key: TranslationKey) => string; images: GeneratedImage[]; response: ResponseSnapshot | null }) {
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/70"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Image className="h-5 w-5 text-fuchsia-600 dark:text-fuchsia-300" />{t("mediaLabPreview")}</CardTitle>{response ? <ResponseMeta t={t} response={response} /> : null}</CardHeader><CardContent>{images.length === 0 ? <EmptyPreview t={t} /> : <div className={cn("grid gap-3", images.length === 1 ? "grid-cols-1" : "grid-cols-2")}>{images.map((item, index) => <a key={`${item.src.slice(0, 64)}:${index}`} href={item.src} target="_blank" rel="noreferrer" className="group overflow-hidden border border-slate-200 bg-slate-100 dark:border-slate-800 dark:bg-slate-950"><img src={item.src} alt={item.label} className="aspect-square w-full object-cover transition-transform duration-300 group-hover:scale-[1.02]" /><div className="truncate border-t border-slate-200 bg-white px-3 py-2 text-xs text-slate-500 dark:border-slate-800 dark:bg-slate-900">{item.label}</div></a>)}</div>}</CardContent></Card>;
}

function VideoPreview({ t, job, blobURL, loading, response }: { t: (key: TranslationKey) => string; job: VideoJob | null; blobURL: string; loading: boolean; response: ResponseSnapshot | null }) {
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/70"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Video className="h-5 w-5 text-cyan-600 dark:text-cyan-300" />{t("mediaLabPreview")}</CardTitle>{response ? <ResponseMeta t={t} response={response} /> : null}</CardHeader><CardContent>{!job ? <EmptyPreview t={t} /> : <div className="space-y-4"><div className="grid gap-3 border border-slate-200 bg-slate-50 p-3 text-xs dark:border-slate-800 dark:bg-slate-950/40 sm:grid-cols-3"><Meta label={t("mediaLabJob")} value={job.id} mono /><Meta label={t("mediaLabStatus")} value={videoStatusLabel(job.status, t)} /><Meta label={t("mediaLabElapsed")} value={formatElapsed(Date.now() - job.createdAt)} /></div>{shouldPoll(job.status) || loading ? <div className="flex min-h-64 flex-col items-center justify-center gap-3 border border-dashed border-cyan-500/35 bg-cyan-500/[0.04] text-center text-sm text-cyan-800 dark:text-cyan-200"><LoaderCircle className="h-7 w-7 animate-spin" /><div className="font-semibold">{t("mediaLabPolling")}</div><div className="text-xs text-cyan-700/70 dark:text-cyan-200/70">{job.progress ? `${job.progress}%` : videoStatusLabel(job.status, t)}</div></div> : job.status === "completed" && blobURL ? <div className="space-y-3"><video controls className="aspect-video w-full bg-black" src={blobURL} /><a href={blobURL} download={`${job.id}.mp4`} className="inline-flex h-9 items-center gap-2 border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200"><Download className="h-4 w-4" />{t("mediaLabDownload")}</a></div> : <div className="flex min-h-64 flex-col items-center justify-center gap-2 border border-dashed border-rose-500/35 bg-rose-500/[0.04] text-center text-sm text-rose-700 dark:text-rose-300"><CircleAlert className="h-6 w-6" />{videoStatusLabel(job.status, t)}</div>}</div>}</CardContent></Card>;
}

function EmptyPreview({ t }: { t: (key: TranslationKey) => string }) {
  return <div className="flex min-h-72 flex-col items-center justify-center gap-3 border border-dashed border-slate-300 bg-slate-50/70 p-6 text-center text-sm text-slate-500 dark:border-slate-700 dark:bg-slate-950/30 dark:text-slate-400"><Image className="h-7 w-7 text-slate-400" />{t("mediaLabNoPreview")}</div>;
}

function ResponsePanel({ t, response }: { t: (key: TranslationKey) => string; response: ResponseSnapshot | null }) {
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/70"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Code2 className="h-5 w-5 text-slate-600 dark:text-slate-300" />{t("mediaLabResponse")}</CardTitle>{response ? <ResponseMeta t={t} response={response} /> : null}</CardHeader><CardContent><pre className="max-h-96 overflow-auto border border-slate-800 bg-slate-950 p-3 text-xs leading-5 text-slate-100"><code>{response?.raw || "{}"}</code></pre></CardContent></Card>;
}

function ResponseMeta({ t, response }: { t: (key: TranslationKey) => string; response: ResponseSnapshot }) {
  return <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500 dark:text-slate-400"><span>{t("mediaLabStatus")} <strong className={response.status >= 200 && response.status < 300 ? "text-emerald-600 dark:text-emerald-300" : "text-rose-600 dark:text-rose-300"}>{response.status}</strong></span><span>{t("mediaLabLatency")} <strong className="text-slate-700 dark:text-slate-200">{response.latency} ms</strong></span>{response.requestID ? <span className="font-mono text-[10px]">{response.requestID}</span> : null}</div>;
}

function Meta({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div className="min-w-0"><div className="text-[10px] text-slate-500 dark:text-slate-400">{label}</div><div className={cn("mt-1 truncate text-sm font-semibold text-slate-800 dark:text-slate-200", mono && "font-mono text-xs")}>{value}</div></div>;
}

async function requestJSON(url: string, apiKey: string, body?: Record<string, unknown>) {
  const started = performance.now();
  const response = await fetch(url, {
    method: body ? "POST" : "GET",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${apiKey.trim()}`,
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
  const raw = await response.text();
  let value: unknown;
  try { value = raw ? JSON.parse(raw) : {}; } catch { value = { raw }; }
  return {
    ok: response.ok,
    value,
    snapshot: {
      status: response.status,
      latency: Math.round(performance.now() - started),
      requestID: response.headers.get("x-request-id") || "",
      raw: prettyJSON(value, raw),
    } satisfies ResponseSnapshot,
  };
}

function endpointChoices(apiEndpoints: PublicAPIEndpoint[], currentLabel: string): EndpointOption[] {
  const values = [{ label: currentLabel, root: window.location.origin }];
  for (const endpoint of apiEndpoints) {
    const root = resolveAPIEndpointURLs(endpoint).root;
    if (!root || values.some((item) => item.root === root)) continue;
    values.push({ label: endpoint.name || root, root });
  }
  return values;
}

function cleanPayload(value: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== "" && item !== undefined));
}

function curlExample(endpoint: string, body: Record<string, unknown>) {
  const encoded = JSON.stringify(body, null, 2).replace(/'/g, `'"'"'`);
  return `curl ${shellQuote(endpoint)} \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '${encoded}'`;
}

function shellQuote(value: string) {
  return `'${value.replace(/'/g, `'"'"'`)}'`;
}

function prettyJSON(value: unknown, fallback: string) {
  try { return JSON.stringify(value, null, 2); } catch { return fallback || "{}"; }
}

function errorFromResponse(value: unknown) {
  const response = asRecord(value);
  if (!response) return "";
  if (typeof response.error === "string") return response.error;
  const error = asRecord(response.error);
  if (typeof error?.message === "string") return error.message;
  if (typeof response.message === "string") return response.message;
  return "";
}

function extractImages(value: unknown): GeneratedImage[] {
  const response = asRecord(value);
  if (!Array.isArray(response?.data)) return [];
  return response.data.flatMap((value, index) => {
    const item = asRecord(value);
    if (typeof item?.url === "string" && item.url.trim()) return [{ src: item.url, label: `image-${index + 1}` }];
    if (typeof item?.b64_json === "string" && item.b64_json.trim()) return [{ src: `data:image/png;base64,${item.b64_json}`, label: `image-${index + 1}` }];
    return [];
  });
}

function asRecord(value: unknown): JSONRecord | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as JSONRecord : undefined;
}

function videoStatus(value: unknown) {
  const status = typeof value === "string" ? value.toLowerCase().trim() : "queued";
  if (["succeeded", "success", "completed", "done"].includes(status)) return "completed";
  if (["failed", "error", "expired"].includes(status)) return "failed";
  if (["cancelled", "canceled"].includes(status)) return "cancelled";
  if (["running", "in_progress", "processing"].includes(status)) return "processing";
  return "queued";
}

function shouldPoll(status: string) {
  return status === "queued" || status === "processing";
}

function videoStatusLabel(status: string, t: (key: TranslationKey) => string) {
  if (status === "completed") return t("mediaLabCompleted");
  if (status === "failed") return t("mediaLabFailed");
  if (status === "cancelled") return t("mediaLabCancelled");
  if (status === "processing") return t("mediaLabProcessing");
  return t("mediaLabQueued");
}

function formatElapsed(value: number) {
  const seconds = Math.max(0, Math.floor(value / 1000));
  return seconds >= 60 ? `${Math.floor(seconds / 60)}m ${seconds % 60}s` : `${seconds}s`;
}

function seedanceCapability(model: PublicModelSummary): SeedanceSpec | null {
  const capabilities = model.capabilities || {};
  const version = typeof capabilities.seedance_version === "string" ? capabilities.seedance_version.trim() : "";
  if (!version) return null;
  const numberValue = (value: unknown, fallback: number) => typeof value === "number" && Number.isFinite(value) ? value : fallback;
  const stringValues = (value: unknown) => Array.isArray(value) ? value.filter((item): item is string => typeof item === "string" && item.trim() !== "").map((item) => item.trim()) : [];
  return {
    version,
    defaultDuration: numberValue(capabilities.default_duration_seconds, version === "2.5" ? -1 : 5),
    maxDuration: numberValue(capabilities.max_duration_seconds, version === "2.5" ? 30 : 15),
    maxReferenceImages: numberValue(capabilities.max_reference_images, version === "2.5" ? 30 : 9),
    maxReferenceVideos: numberValue(capabilities.max_reference_videos, version === "2.5" ? 10 : 3),
    maxReferenceAudios: numberValue(capabilities.max_reference_audios, version === "2.5" ? 10 : 3),
    audioOnlyReference: capabilities.audio_only_reference === true,
    resolutions: stringValues(capabilities.supported_resolutions).length > 0 ? stringValues(capabilities.supported_resolutions) : version === "2.5" ? ["480p", "720p", "1080p"] : ["480p", "720p", "1080p", "4k"],
    supportsOutputFormat: capabilities.supports_output_format === true,
    supportsOmniTaskType: capabilities.supports_omni_task_type === true,
    supportsReturnLastFrame: capabilities.supports_return_last_frame === true,
  };
}

const selectClass = "h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/15 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100";
