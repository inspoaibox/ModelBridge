import { useCallback, useMemo, useState, type ReactNode } from "react";
import { Check, CircleAlert, Code2, Copy, LoaderCircle, MessageSquare, Play, Server } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { resolveAPIEndpointURLs } from "@/lib/api-endpoint";
import { Language, PublicAPIEndpoint, PublicModelSummary, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";

interface TextDebugPanelProps {
  language: Language;
  models: PublicModelSummary[];
  apiEndpoints: PublicAPIEndpoint[];
}

interface ResponseSnapshot {
  status: number;
  latency: number;
  requestID: string;
  raw: string;
}

type JSONRecord = Record<string, unknown>;

export function TextDebugPanel({ language, models, apiEndpoints }: TextDebugPanelProps) {
  const t = useCallback((key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key, [language]);
  const endpointOptions = useMemo(() => {
    const options: Array<{ label: string; url: string }> = [{ label: String(t("consoleDebugCurrentGateway")), url: `${window.location.origin}/v1` }];
    for (const item of apiEndpoints) {
      const url = resolveAPIEndpointURLs(item).openai;
      if (url && !options.some((option) => option.url === url)) options.push({ label: item.name || url, url });
    }
    return options;
  }, [apiEndpoints, t]);
  const textModels = useMemo(
    () => models.filter((model) => model.category === "text" && model.available).sort((left, right) => left.name.localeCompare(right.name)),
    [models],
  );
  const [endpoint, setEndpoint] = useState(() => endpointOptions[0]?.url || `${window.location.origin}/v1`);
  const [model, setModel] = useState("");
  const [apiKey, setAPIKey] = useState("");
  const [system, setSystem] = useState("");
  const [prompt, setPrompt] = useState("");
  const [maxTokens, setMaxTokens] = useState("512");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [response, setResponse] = useState<ResponseSnapshot | null>(null);
  const [copyState, setCopyState] = useState("");

  const activeModel = textModels.some((item) => item.name === model) ? model : textModels[0]?.name || "";
  const payload = useMemo(() => ({
    model: activeModel,
    messages: [
      ...(system.trim() ? [{ role: "system", content: system.trim() }] : []),
      { role: "user", content: prompt },
    ],
    max_tokens: Number(maxTokens) || 512,
  }), [activeModel, maxTokens, prompt, system]);
  const body = JSON.stringify(payload, null, 2);
  const curl = [
    `curl ${shellQuote(`${endpoint.replace(/\/+$/, "")}/chat/completions`)} \\`,
    '  -H "Authorization: Bearer YOUR_API_KEY" \\',
    '  -H "Content-Type: application/json" \\',
    `  -d ${shellQuote(body)}`,
  ].join("\n");

  async function submit() {
    setError("");
    setResponse(null);
    if (!apiKey.trim()) return setError(t("consoleDebugApiKeyRequired"));
    if (!prompt.trim()) return setError(t("consoleDebugPromptRequired"));
    if (!activeModel) return setError(t("consoleDebugNoModels"));
    setBusy(true);
    const started = performance.now();
    try {
      const result = await fetch(`${endpoint.replace(/\/+$/, "")}/chat/completions`, {
        method: "POST",
        headers: { Accept: "application/json", "Content-Type": "application/json", Authorization: `Bearer ${apiKey.trim()}` },
        body: JSON.stringify(payload),
      });
      const raw = await result.text();
      let value: unknown;
      try { value = raw ? JSON.parse(raw) : {}; } catch { value = { raw }; }
      setResponse({ status: result.status, latency: Math.round(performance.now() - started), requestID: result.headers.get("x-request-id") || "", raw: prettyJSON(value, raw) });
      if (!result.ok) setError(errorMessage(value) || t("consoleDebugRequestFailed"));
    } catch {
      setError(t("consoleDebugRequestFailed"));
    } finally {
      setBusy(false);
    }
  }

  async function copy(value: string, key: string) {
    try {
      await navigator.clipboard.writeText(value);
      setCopyState(key);
      window.setTimeout(() => setCopyState((current) => current === key ? "" : current), 1600);
    } catch {
      setError(t("consoleDebugRequestFailed"));
    }
  }

  return (
    <div className="space-y-5">
      <section className="overflow-hidden border border-indigo-500/20 bg-indigo-500/[0.045] px-5 py-6 shadow-sm dark:border-indigo-400/20 sm:px-7">
        <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-indigo-700 dark:text-indigo-300"><MessageSquare className="h-4 w-4" />{t("consoleDebugTextEyebrow")}</div>
        <h2 className="mt-3 text-2xl font-bold text-slate-950 dark:text-white">{t("consoleDebugTextTitle")}</h2>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600 dark:text-slate-400">{t("consoleDebugTextDescription")}</p>
        <p className="mt-3 text-xs text-slate-500 dark:text-slate-400">{t("consoleDebugKeyNotice")}</p>
      </section>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.08fr)_minmax(420px,0.92fr)]">
        <section className="space-y-5">
          <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/70">
            <CardHeader><CardTitle className="flex items-center gap-2 text-lg"><MessageSquare className="h-5 w-5 text-indigo-600 dark:text-indigo-300" />{t("consoleDebugTextTitle")}</CardTitle><CardDescription>{t("consoleDebugRequestDescription")}</CardDescription></CardHeader>
            <CardContent className="space-y-5">
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label={t("consoleDebugEndpoint")}><select value={endpoint} onChange={(event) => setEndpoint(event.target.value)} className={selectClass}>{endpointOptions.map((item) => <option key={item.url} value={item.url}>{item.label}</option>)}</select></Field>
                <Field label={t("consoleDebugModel")}><select value={activeModel} onChange={(event) => setModel(event.target.value)} disabled={textModels.length === 0} className={selectClass}>{textModels.length === 0 ? <option value="">{t("consoleDebugNoModels")}</option> : textModels.map((item) => <option key={item.id} value={item.name}>{item.display_name} ({item.provider})</option>)}</select></Field>
              </div>
              <Field label={t("consoleDebugApiKey")}><Input type="password" value={apiKey} onChange={(event) => setAPIKey(event.target.value)} placeholder={t("consoleDebugApiKeyPlaceholder")} autoComplete="off" /></Field>
              <Field label={t("consoleDebugSystem")}><Input value={system} onChange={(event) => setSystem(event.target.value)} placeholder={t("consoleDebugSystemPlaceholder")} /></Field>
              <Field label={t("consoleDebugPrompt")}><textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder={t("consoleDebugPromptPlaceholder")} className="min-h-36 w-full resize-y rounded-md border border-slate-200 bg-white px-3 py-2.5 text-sm leading-6 text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/15 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100" /></Field>
              <Field label={t("consoleDebugMaxTokens")}><Input type="number" min={1} max={8192} value={maxTokens} onChange={(event) => setMaxTokens(event.target.value)} className="max-w-xs" /></Field>
              {error ? <div className="flex items-start gap-2 rounded-md border border-rose-500/30 bg-rose-50 p-3 text-sm text-rose-700 dark:bg-rose-500/10 dark:text-rose-300"><CircleAlert className="mt-0.5 h-4 w-4 shrink-0" />{error}</div> : null}
              <Button type="button" onClick={() => void submit()} disabled={busy || textModels.length === 0} className="gap-2 bg-indigo-600 hover:bg-indigo-700"><Play className="h-4 w-4" />{busy ? <><LoaderCircle className="h-4 w-4 animate-spin" />{t("consoleDebugSending")}</> : t("consoleDebugSend")}</Button>
            </CardContent>
          </Card>
          <CodePanel title={t("consoleDebugRequestBody")} value={body} copied={copyState === "body"} onCopy={() => void copy(body, "body")} t={t} />
          <CodePanel title={t("consoleDebugCurl")} value={curl} copied={copyState === "curl"} onCopy={() => void copy(curl, "curl")} t={t} />
        </section>
        <section className="space-y-5">
          <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/70"><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><Server className="h-5 w-5 text-indigo-600 dark:text-indigo-300" />{t("consoleDebugResponse")}</CardTitle>{response ? <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500 dark:text-slate-400"><span>{t("consoleDebugStatus")} <strong className={response.status >= 200 && response.status < 300 ? "text-emerald-600" : "text-rose-600"}>{response.status}</strong></span><span>{t("consoleDebugLatency")} <strong className="text-slate-700 dark:text-slate-200">{response.latency} ms</strong></span>{response.requestID ? <span className="font-mono text-[10px]">{response.requestID}</span> : null}</div> : null}</CardHeader><CardContent><pre className="max-h-[560px] overflow-auto border border-slate-800 bg-slate-950 p-3 text-xs leading-5 text-slate-100"><code>{response?.raw || "{}"}</code></pre></CardContent></Card>
          <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/70"><CardContent className="flex min-h-48 flex-col items-center justify-center gap-2 text-center text-sm text-slate-500 dark:text-slate-400"><Code2 className="h-7 w-7 text-slate-400" />{t("consoleDebugResponseHint")}</CardContent></Card>
        </section>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block space-y-2"><span className="text-xs font-semibold text-slate-700 dark:text-slate-300">{label}</span>{children}</label>;
}

function CodePanel({ title, value, copied, onCopy, t }: { title: string; value: string; copied: boolean; onCopy: () => void; t: (key: TranslationKey) => string }) {
  return <Card className="overflow-hidden border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/70"><CardHeader className="flex flex-row items-center justify-between space-y-0 border-b border-slate-200 py-3 dark:border-slate-800"><CardTitle className="text-sm">{title}</CardTitle><Button type="button" variant="ghost" size="sm" onClick={onCopy} className="h-7 gap-1 px-2 text-xs">{copied ? <Check className="h-3.5 w-3.5 text-emerald-600" /> : <Copy className="h-3.5 w-3.5" />}{copied ? t("consoleDebugCopied") : t("consoleDebugCopy")}</Button></CardHeader><CardContent className="p-0"><pre className="max-h-64 overflow-auto bg-slate-950 p-3 text-xs leading-5 text-slate-100"><code>{value}</code></pre></CardContent></Card>;
}

function prettyJSON(value: unknown, fallback: string) {
  try { return JSON.stringify(value, null, 2); } catch { return fallback || "{}"; }
}

function errorMessage(value: unknown) {
  const record = value !== null && typeof value === "object" && !Array.isArray(value) ? value as JSONRecord : undefined;
  if (typeof record?.error === "string") return record.error;
  const error = record?.error !== null && typeof record?.error === "object" && !Array.isArray(record.error) ? record.error as JSONRecord : undefined;
  if (typeof error?.message === "string") return error.message;
  if (typeof record?.message === "string") return record.message;
  return "";
}

function shellQuote(value: string) {
  return `'${value.replace(/'/g, "'\"'\"'")}'`;
}

const selectClass = "h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/15 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100";
