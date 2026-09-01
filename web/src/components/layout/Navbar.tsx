import React from "react";
import {
	Boxes,
  Globe,
  LayoutDashboard,
  LogIn,
  LogOut,
  Moon,
  Sparkles,
  Sun,
  UserPlus,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Audience, Language, Theme, TranslationKey, View } from "@/types";
import { translations } from "@/locales/translations";

interface NavbarProps {
  theme: Theme;
  setTheme: (theme: Theme | ((prev: Theme) => Theme)) => void;
  language: Language;
  setLanguage: (lang: Language | ((prev: Language) => Language)) => void;
  signedIn: boolean;
  currentView: View;
  routeTo: (target: string) => void;
  onSignOut: () => void;
  principalName?: string;
  principalAudience?: Audience;
  siteName?: string;
  siteLogoURL?: string;
}

export function Navbar({
  theme,
  setTheme,
  language,
  setLanguage,
  signedIn,
  currentView,
  routeTo,
  onSignOut,
  principalName,
  principalAudience,
  siteName,
  siteLogoURL,
}: NavbarProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const brandName = siteName?.trim() || t("brandName");

  return (
    <header className="sticky top-0 z-50 border-b border-slate-200/80 dark:border-slate-800/80 bg-white/85 dark:bg-slate-950/85 backdrop-blur-xl transition-colors duration-200">
      <div className="flex w-full items-center justify-between gap-2 px-3 py-2.5 sm:px-6 lg:px-8">
        {/* Brand Logo */}
        <div className="flex items-center gap-4 lg:gap-6 min-w-0">
          <a
            className="group flex shrink-0 cursor-pointer select-none items-center gap-3"
            href="#home"
            aria-label={brandName}
            onClick={(e) => {
              e.preventDefault();
              routeTo("#home");
            }}
          >
            <div className="relative flex h-9 w-9 sm:h-10 sm:w-10 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br from-indigo-500 via-indigo-600 to-cyan-500 shadow-lg shadow-indigo-500/25 ring-1 ring-white/20 transition-transform duration-300 group-hover:scale-105">
              <Sparkles className="h-4 w-4 sm:h-5 sm:w-5 text-white" />
              {siteLogoURL ? <img src={siteLogoURL} alt="" className="absolute inset-0 h-full w-full bg-white object-contain p-1" onError={(event) => { event.currentTarget.style.display = "none"; }} /> : null}
              <div className="absolute inset-0 rounded-xl bg-white/20 opacity-0 transition-opacity group-hover:opacity-100" />
            </div>
            <div className="hidden min-w-0 flex-col sm:flex">
              <div className="flex items-center gap-1.5 sm:gap-2">
                <span className="text-sm sm:text-base font-bold tracking-tight text-slate-900 dark:text-white group-hover:text-indigo-600 dark:group-hover:text-indigo-300 transition-colors truncate">
                  {brandName}
                </span>
                <span className="rounded-md bg-indigo-500/10 dark:bg-indigo-500/20 px-1.5 py-0.5 text-[10px] font-bold text-indigo-600 dark:text-indigo-300 ring-1 ring-indigo-500/30">
                  PRO
                </span>
              </div>
              <span className="text-xs text-slate-500 dark:text-slate-400 font-medium hidden md:inline-block truncate">
                {t("brandSubtitle")}
              </span>
            </div>
          </a>

        </div>

        {/* Navigation & Controls */}
        <nav className="flex shrink-0 items-center gap-1 sm:gap-2.5" aria-label="Primary navigation">
          <Button
            variant={currentView === "home" ? "secondary" : "ghost"}
            size="sm"
            onClick={() => routeTo("#home")}
            className="hidden h-8.5 px-2.5 text-xs font-medium sm:inline-flex sm:px-3 sm:text-sm"
          >
            {t("navHome")}
          </Button>

          <Button
            variant={currentView === "models" ? "secondary" : "ghost"}
            size="sm"
            onClick={() => routeTo("#models")}
            className="h-8.5 gap-1.5 px-2 text-xs font-medium sm:px-3 sm:text-sm"
            title={t("navModels")}
            aria-label={t("navModels")}
          >
            <Boxes className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
            <span className="hidden sm:inline">{t("navModels")}</span>
          </Button>

          {signedIn ? (
            <Button
            variant={currentView === "admin" || currentView === "console" ? "default" : "secondary"}
              size="sm"
              onClick={() => routeTo(principalAudience === "console" ? "#console/dashboard" : "#admin/dashboard")}
              className="h-8.5 gap-1.5 px-2 text-xs font-medium sm:px-3 sm:text-sm"
              title={t("navConsole")}
              aria-label={t("navConsole")}
            >
              <LayoutDashboard className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
              <span className="hidden sm:inline">{t("navConsole")}</span>
            </Button>
          ) : (
            <>
              <Button
                variant={currentView === "login" ? "secondary" : "ghost"}
                size="sm"
                onClick={() => routeTo("#login")}
                className="h-8.5 px-2 text-xs font-medium sm:px-3 sm:text-sm"
                title={t("navLogin")}
                aria-label={t("navLogin")}
              >
                <LogIn className="h-3.5 w-3.5" />
                <span className="hidden sm:inline">{t("navLogin")}</span>
              </Button>
              <Button
                variant="default"
                size="sm"
                onClick={() => routeTo("#register")}
                className="h-8.5 gap-1.5 px-2 text-xs font-medium sm:px-3 sm:text-sm"
                title={t("navRegister")}
                aria-label={t("navRegister")}
              >
                <UserPlus className="h-3.5 w-3.5" />
                <span className="hidden sm:inline">{t("navRegister")}</span>
              </Button>
            </>
          )}

          {/* Theme Toggle (Light / Dark) */}
          <Button
            variant="outline"
            size="sm"
            onClick={() => setTheme((prev) => (prev === "dark" ? "light" : "dark"))}
            className="h-8.5 w-8.5 p-0 text-slate-700 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white"
            title={theme === "dark" ? (language === "zh" ? "切换为明亮模式" : "Switch to Light Mode") : (language === "zh" ? "切换为暗黑模式" : "Switch to Dark Mode")}
          >
            {theme === "dark" ? (
              <Sun className="h-4 w-4 text-amber-400 transition-transform duration-200 rotate-0 hover:rotate-45" />
            ) : (
              <Moon className="h-4 w-4 text-indigo-600 transition-transform duration-200 rotate-0 hover:-rotate-12" />
            )}
          </Button>

          {/* Language Switcher */}
          <Button
            variant="outline"
            size="sm"
            onClick={() => setLanguage((current) => (current === "zh" ? "en" : "zh"))}
            className="h-8.5 w-8.5 px-0 text-xs text-slate-700 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white sm:w-auto sm:px-2"
            title="Switch Language"
          >
            <Globe className="h-3.5 w-3.5 text-indigo-500 dark:text-indigo-400 sm:mr-1" />
            <span className="font-semibold">{language === "zh" ? "EN" : "中"}</span>
          </Button>

          {/* Auth State Chip */}
          {signedIn ? (
            <div className="flex items-center gap-1.5 sm:gap-2 pl-0.5 sm:pl-1">
              <div className="hidden lg:flex flex-col text-right">
                <span className="text-xs font-semibold text-slate-800 dark:text-slate-200 truncate max-w-[100px] xl:max-w-[140px] font-mono">
                  {principalName || "Admin"}
                </span>
                <span className="text-[9px] text-emerald-600 dark:text-emerald-400 font-mono font-bold tracking-wider">
                  AUTHORIZED
                </span>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={onSignOut}
                className="h-8.5 px-2 text-xs text-slate-500 dark:text-slate-400 hover:text-rose-600 dark:hover:text-rose-300 hover:bg-rose-50 dark:hover:bg-rose-500/10"
                title={t("signOut")}
              >
                <LogOut className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                <span className="hidden md:inline ml-1">{t("signOut")}</span>
              </Button>
            </div>
          ) : (
            <Badge variant="secondary" className="hidden md:inline-flex text-[11px]">
              {t("sessionSignedOut")}
            </Badge>
          )}
        </nav>
      </div>
    </header>
  );
}

