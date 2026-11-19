// Window chrome helpers backed by the Wails runtime (no-ops in a browser).

export type Platform = "windows" | "darwin" | "linux" | "browser";

interface WailsRuntime {
  Environment(): Promise<{ platform: string; arch: string; buildType: string }>;
  WindowMinimise(): void;
  WindowToggleMaximise(): void;
  WindowIsMaximised(): Promise<boolean>;
  Quit(): void;
  BrowserOpenURL(url: string): void;
  EventsOn(name: string, cb: (...data: unknown[]) => void): () => void;
}

const rt = (): Partial<WailsRuntime> | undefined =>
  typeof window === "undefined" ? undefined : (window as unknown as { runtime?: Partial<WailsRuntime> }).runtime;

export async function getPlatform(): Promise<Platform> {
  const env = await rt()?.Environment?.().catch(() => undefined);
  const p = env?.platform;
  return p === "windows" || p === "darwin" || p === "linux" ? p : "browser";
}

export const win = {
  minimise: () => rt()?.WindowMinimise?.(),
  toggleMaximise: () => rt()?.WindowToggleMaximise?.(),
  isMaximised: async () => (await rt()?.WindowIsMaximised?.()) ?? false,
  close: () => rt()?.Quit?.(),
  openURL: (url: string) => {
    const r = rt();
    if (r?.BrowserOpenURL) r.BrowserOpenURL(url);
    else window.open(url, "_blank", "noopener");
  },
};
