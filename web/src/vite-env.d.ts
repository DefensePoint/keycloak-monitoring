/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly DEV: boolean;
  readonly PROD: boolean;
  readonly MODE: string;
  /**
   * Free CARTO basemaps API key for the AMFA geolocation map. Baked in at
   * build time. Without it CARTO serves tiles watermarked "API KEY REQUIRED".
   * Request one at https://carto.com/basemaps/apikey/
   */
  readonly VITE_CARTO_BASEMAP_KEY?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
