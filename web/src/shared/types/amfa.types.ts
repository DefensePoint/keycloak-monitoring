/** AMFA KPI counters for a realm (backend: GET /amfa/stats). */
export interface AmfaStats {
  total: number;
  risky: number;
  unique_users: number;
  flagged_ips: number;
  /**
   * Set only when the backend answered an "all realms" request for a narrower
   * realm. The counters then describe that realm, not the tenant.
   */
  applied_realm_id?: string;
}

/** One aggregated lat/long cell for the AMFA geo map (backend: GET /amfa/geo). */
export interface AmfaGeoBucket {
  country: string | null;
  lat: number;
  long: number;
  count: number;
  risky_count: number;
}
