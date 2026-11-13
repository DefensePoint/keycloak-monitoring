import type { Branding } from "@/shared/types/auth.types";

export const DEFAULT_BRANDING: Branding = {
  name: "Keycloak Monitoring Tool",
  short_name: "KMT",
  logo_text: "Keycloak",
};

const STORAGE_KEY = "kmt:branding";

/**
 * The words under the logo: the name without the logo text it starts with,
 * so "Keycloak Monitoring Tool" next to "Keycloak" reads "Monitoring Tool".
 * A name that does not start with the logo text is shown whole.
 */
export function brandingSubtitle(branding: Branding): string {
  const { name, logo_text: logo } = branding;
  if (name.toLowerCase().startsWith(logo.toLowerCase() + " ")) {
    return name.slice(logo.length + 1);
  }
  return name;
}

/** Letters that do not break down into an ASCII letter and accents. */
const FOLDED_LETTERS: Record<string, string> = {
  ß: "ss",
  æ: "ae",
  ø: "o",
  œ: "oe",
  đ: "d",
  ł: "l",
  þ: "th",
  ı: "i",
};

/**
 * The start of a downloaded file's name: the short name in lower case
 * without accents, its letters and digits joined by dashes, as the server
 * does ("kmt", "Über" gives "uber"). A name with no letter that can be
 * written in ASCII gives "monitoring" rather than a name the deployment did
 * not choose.
 */
export function filePrefix(branding: Branding): string {
  const words = branding.short_name
    .toLowerCase()
    .normalize("NFD")
    .replace(/\p{M}/gu, "")
    .replace(/[ßæøœđłþı]/g, (c) => FOLDED_LETTERS[c])
    .match(/[a-z0-9]+/g);
  return words ? words.join("-") : "monitoring";
}

/**
 * The branding the server sent last time, so a page shown while the server
 * cannot be reached still carries the deployment's name. Null when this
 * browser has none: the default might name the wrong product.
 */
export function rememberedBranding(): Branding | null {
  try {
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "null");
    if (
      typeof stored?.name === "string" &&
      typeof stored?.short_name === "string" &&
      typeof stored?.logo_text === "string"
    ) {
      return stored;
    }
  } catch {
    // Unreadable storage counts as none.
  }
  return null;
}

export function rememberBranding(branding: Branding): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(branding));
  } catch {
    // Storage may be full or blocked; the default is shown next time.
  }
}
