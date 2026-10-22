/**
 * Thin wrapper over Telegram's WebApp global.
 *
 * Deliberately not a dependency — the SDK is one script tag and we use a
 * handful of its methods. Everything degrades to a no-op in a plain browser
 * so `npm run dev` works on the desktop.
 */

type HapticStyle = "light" | "medium" | "heavy" | "rigid" | "soft";

interface TelegramWebApp {
  initData: string;
  colorScheme?: "light" | "dark";
  onEvent?(event: "themeChanged", cb: () => void): void;
  ready(): void;
  expand(): void;
  showConfirm?(message: string, callback: (ok: boolean) => void): void;
  HapticFeedback?: {
    impactOccurred(style: HapticStyle): void;
    notificationOccurred(type: "error" | "success" | "warning"): void;
  };
  BackButton?: {
    show(): void;
    hide(): void;
    onClick(cb: () => void): void;
    offClick(cb: () => void): void;
  };
}

const tg: TelegramWebApp | undefined = (
  window as unknown as { Telegram?: { WebApp?: TelegramWebApp } }
).Telegram?.WebApp;

/** False when running in a normal browser tab during development. */
export const inTelegram = Boolean(tg?.initData);

export function ready(): void {
  tg?.ready();
  tg?.expand();
  // Telegram's own theme may differ from the phone's; the CSS follows
  // data-theme when Telegram says, and the system setting otherwise.
  const apply = () => {
    if (tg?.colorScheme) document.documentElement.dataset.theme = tg.colorScheme;
  };
  apply();
  tg?.onEvent?.("themeChanged", apply);
}

/**
 * Signed payload identifying the user, sent with every API request. The
 * backend validates its HMAC against the bot token; never trust a user id
 * that didn't arrive inside this string.
 */
export function initData(): string {
  return tg?.initData ?? "";
}

export function haptic(style: HapticStyle = "light"): void {
  tg?.HapticFeedback?.impactOccurred(style);
}

export function hapticSuccess(): void {
  tg?.HapticFeedback?.notificationOccurred("success");
}

/** Telegram's own confirm sheet; window.confirm in a browser. */
export function confirm(message: string): Promise<boolean> {
  if (tg?.showConfirm) {
    return new Promise((resolve) => tg.showConfirm!(message, resolve));
  }
  return Promise.resolve(window.confirm(message));
}

/** Wire Telegram's native back button to a handler while a screen is open. */
export function onBack(handler: () => void): () => void {
  if (!tg?.BackButton) return () => {};
  tg.BackButton.show();
  tg.BackButton.onClick(handler);
  return () => {
    tg.BackButton?.offClick(handler);
    tg.BackButton?.hide();
  };
}
