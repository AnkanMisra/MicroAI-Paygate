"use client";

import { useTheme } from "next-themes";
import { useEffect, useState } from "react";

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => setMounted(true), []);

  if (!mounted) return null;

  return (
    <button
      onClick={() =>
        setTheme(theme === "light" ? "dark" : "light")
      }
      className="border border-ink bg-paper px-3 py-1.5 font-mono text-[10px] uppercase tracking-[0.12em] text-ink transition-all duration-300 hover:bg-ink hover:text-paper"
    >
      {theme === "light" ? "🌙 Dark" : "☀ Light"}
    </button>
  );
}