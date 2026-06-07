import { initBrowserAnalytics } from "@/lib/browser-analytics";

try {
  initBrowserAnalytics();
} catch {
  // Analytics must never block the client app from booting.
}
