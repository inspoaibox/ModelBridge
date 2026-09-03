import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDecimalWithoutTrailingZeros(
  value: string | number | null | undefined,
  fallback = "-",
) {
  const raw = String(value ?? "").trim();
  if (!raw) return fallback;
  if (!/^[+-]?(?:\d+\.?\d*|\.\d+)$/.test(raw)) return raw;

  const [integerPart, fractionPart] = raw.split(".");
  if (fractionPart === undefined) return raw;

  const trimmedFraction = fractionPart.replace(/0+$/, "");
  return trimmedFraction ? `${integerPart}.${trimmedFraction}` : integerPart;
}
