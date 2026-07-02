import { createContext, useContext, useEffect, useState } from "react";

type Theme = "light" | "dark";
const KEY = "tp-theme";

function initialTheme(): Theme {
  const stored = localStorage.getItem(KEY);
  if (stored === "light" || stored === "dark") return stored;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function apply(theme: Theme) {
  document.documentElement.classList.toggle("dark", theme === "dark");
}

interface Ctx {
  theme: Theme;
  toggle: () => void;
}
const ThemeContext = createContext<Ctx>({ theme: "light", toggle: () => {} });

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => {
    const t = initialTheme();
    apply(t);
    return t;
  });
  useEffect(() => {
    apply(theme);
    localStorage.setItem(KEY, theme);
  }, [theme]);
  return (
    <ThemeContext.Provider value={{ theme, toggle: () => setTheme((t) => (t === "dark" ? "light" : "dark")) }}>
      {children}
    </ThemeContext.Provider>
  );
}

export const useTheme = () => useContext(ThemeContext);

export function ThemeToggle({ className = "" }: { className?: string }) {
  const { theme, toggle } = useTheme();
  return (
    <button
      onClick={toggle}
      title={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
      aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
      className={`text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100 ${className}`}
    >
      {theme === "dark" ? "☀︎" : "☾"}
    </button>
  );
}
