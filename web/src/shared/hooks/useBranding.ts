import { useContext } from "react";
import { AuthContext } from "@/shared/context/AuthContext";
import { DEFAULT_BRANDING } from "@/shared/lib/branding";
import type { Branding } from "@/shared/types/auth.types";

/**
 * The product names this deployment shows. Outside an AuthProvider it gives
 * the default names rather than failing, since naming the product never
 * needs a session.
 */
export function useBranding(): Branding {
  return useContext(AuthContext)?.branding ?? DEFAULT_BRANDING;
}

/**
 * Whether useBranding gives names the server sent. False only before this
 * browser has ever heard from the server; outside an AuthProvider the
 * default names are the ones meant.
 */
export function useBrandingKnown(): boolean {
  return useContext(AuthContext)?.brandingKnown ?? true;
}
