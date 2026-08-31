import { ArrowLeft, Compass } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Language } from "@/types";

interface NotFoundViewProps {
  language: Language;
  routeTo: (target: string) => void;
}

export function NotFoundView({ language, routeTo }: NotFoundViewProps) {
  const zh = language === "zh";
  return (
    <div className="flex min-h-[58vh] items-center justify-center py-10">
      <section className="w-full max-w-xl text-center">
        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl border border-indigo-500/20 bg-indigo-500/10 text-indigo-600 dark:text-indigo-300"><Compass className="h-6 w-6" /></div>
        <div className="mt-6 font-mono text-6xl font-extrabold text-slate-900 dark:text-white">404</div>
        <h1 className="mt-4 text-2xl font-bold text-slate-900 dark:text-white">{zh ? "页面不存在" : "Page not found"}</h1>
        <p className="mx-auto mt-3 max-w-md text-sm leading-6 text-slate-600 dark:text-slate-400">{zh ? "链接可能已失效，或该页面尚未对当前账号开放。" : "The link may be outdated, or this page is not available to your account."}</p>
        <Button type="button" className="mt-6 gap-2" onClick={() => routeTo("#home")}><ArrowLeft className="h-4 w-4" />{zh ? "返回首页" : "Back to home"}</Button>
      </section>
    </div>
  );
}
