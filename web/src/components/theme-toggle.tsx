"use client";

import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useEffect, useState } from "react";

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  const isLightTheme = theme === "light";

  useEffect(() => {
    const frame = requestAnimationFrame(() => setMounted(true));

    return () => cancelAnimationFrame(frame);
  }, []);

  if (!mounted) return null;

  return (
    <button
      onClick={() => setTheme(isLightTheme ? "dark" : "light")}
      aria-pressed={!isLightTheme}
      aria-label={
        isLightTheme ? "Switch to dark mode" : "Switch to light mode"
      }
      className="inline-flex items-center gap-1 border border-ink bg-paper px-2.5 py-1.5 font-mono text-[10px] uppercase tracking-[0.12em] text-ink transition-all duration-300 hover:bg-ink hover:text-paper sm:px-3"
    >
      {isLightTheme ? (
        <Moon aria-hidden="true" className="size-3.5" strokeWidth={2} />
      ) : (
        <Sun aria-hidden="true" className="size-3.5" strokeWidth={2} />
      )}
      <span className="sr-only sm:not-sr-only">
        {isLightTheme ? "Dark" : "Light"}
      </span>
    </button>
  );
}
