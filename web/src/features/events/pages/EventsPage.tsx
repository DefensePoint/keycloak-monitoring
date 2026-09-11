import { useState, useEffect, useMemo, useCallback } from "react";
import { useSearchParams, Link } from "react-router-dom";
import {
  Box,
  Typography,
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  Chip,
  Tooltip,
  Button,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from "@mui/material";
import {
  Close as CloseIcon,
  OpenInNew as OpenInNewIcon,
  Visibility as VisibilityIcon,
  ExpandMore as ExpandMoreIcon,
} from "@mui/icons-material";
import {
  MaterialReactTable,
  type MRT_ColumnDef,
  type MRT_Row,
  type MRT_Cell,
} from "material-react-table";
import { useTenant } from "@/shared/context";
import {
  useKeycloakDashboard,
  useRealmSelector,
  useEvents,
  useAmfaStats,
  useAmfaGeo,
  useAmfaRealms,
  useSessionStorage,
  isAllRealmsUnsupportedError,
  isRealmScopeRequiresRealmError,
} from "@/shared/hooks";
import {
  RealmSelector,
  TimeSelector,
  PageSizeSelector,
  PageHeader,
  FeatureErrorBoundary,
  RiskBadge,
  AmfaKpiRow,
  AmfaGeoMap,
  LocationMap,
  AlertBanner,
} from "@/shared/components";
import { Refresh } from "@mui/icons-material";
import { MRT_OPTIONS_WITH_TOOLBAR } from "@/shared/constants";
import { hasEventLocation } from "@/shared/utils";
import type { Event as EventType } from "../types";

type SeverityColor = "error" | "warning" | "info";

type EventWithFormattedDate = EventType & {
  formattedTimestamp: string;
};

function getSeverityColor(severity: string): SeverityColor {
  switch (severity) {
    case "error":
      return "error";
    case "warning":
      return "warning";
    default:
      return "info";
  }
}

/**
 * Height of the location map embedded in the details dialog, in pixels.
 *
 * At the dialog's ~830px content width this is roughly 2.3:1. Shorter and the
 * marker plus its 20km accuracy circle are squeezed into a letterboxed band
 * with too little land around them to place the point; taller and the map
 * pushes the AMFA fields below it out of the dialog's first screenful.
 */
const EMBEDDED_MAP_HEIGHT = 368;

export function EventsPage() {
  const { selectedTenant } = useTenant();
  const [selectedEvent, setSelectedEvent] = useState<EventType | null>(null);
  // Leaflet measures its container once, on init. Mounting the map before the
  // dialog's scale transform finishes makes it measure the mid-animation size
  // and render a partial map, so hold the map back until onEntered fires.
  const [detailsDialogEntered, setDetailsDialogEntered] = useState(false);
  const [pageSize, setPageSize] = useSessionStorage<number>("pageSize", 25);
  const [limit, setLimit] = useState(pageSize);
  const [offset, setOffset] = useState(0);
  const [startTime, setStartTime] = useState<string | undefined>(undefined);
  const [endTime, setEndTime] = useState<string | undefined>(undefined);
  const [searchParams, setSearchParams] = useSearchParams();

  // Read initial time range from URL
  const initialStart = searchParams.get("start")
    ? new Date(searchParams.get("start")!)
    : undefined;
  const initialEnd = searchParams.get("end")
    ? new Date(searchParams.get("end")!)
    : undefined;

  const handleTimeRangeChange = useCallback(
    (start: Date | null, end: Date | null) => {
      setStartTime(start ? start.toISOString() : undefined);
      setEndTime(end ? end.toISOString() : undefined);
      setOffset(0);
      setSearchParams((prev) => {
        const newParams = new URLSearchParams(prev);
        if (start) {
          newParams.set("start", start.toISOString());
        } else {
          newParams.delete("start");
        }
        if (end) {
          newParams.set("end", end.toISOString());
        } else {
          newParams.delete("end");
        }
        return newParams;
      });
    },
    [setSearchParams],
  );

  const { data: keycloakDashboard } = useKeycloakDashboard({
    tenantId: selectedTenant?.tenant_id,
  });

  const realms = keycloakDashboard?.realms ?? [];

  const { selectedRealm, handleRealmChange: handleRealmChangeFromHook } =
    useRealmSelector({
      tenantId: selectedTenant?.tenant_id,
      defaultRealm: selectedTenant?.default_realm,
      availableRealms: realms,
    });

  const handleRealmChange = useCallback(
    (realm: string) => {
      handleRealmChangeFromHook(realm);
      setOffset(0);
    },
    [handleRealmChangeFromHook],
  );

  // Sent as a realm, not as source="keycloak:<realm>". That older encoding
  // matched only the realm's Keycloak rows, so selecting a realm emptied the
  // table of its AMFA-mirrored events (source "amfa:<realm>") - the very
  // events the AMFA sections above the table are about.
  const realmFilter = selectedRealm !== "all" ? selectedRealm : undefined;

  const {
    data: eventsResponse,
    isLoading,
    refetch,
  } = useEvents({
    tenantId: selectedTenant?.tenant_id,
    limit,
    offset,
    startTime,
    endTime,
    realm: realmFilter,
    refetchInterval: 30000, // 30 seconds instead of default 10 seconds
  });

  // Realms whose Keycloak browser flow uses the AMFA authenticator. Drives
  // whether the AMFA-only sections (KPI cards, geo map, Risk column) show:
  // for a specific realm, only if it is AMFA-enabled; for "all realms", if any
  // realm in the tenant is AMFA-enabled.
  const { data: amfaRealms } = useAmfaRealms(selectedTenant?.tenant_id);
  const amfaRealmSet = amfaRealms ?? [];
  const currentRealmHasAmfa =
    selectedRealm !== "all" && amfaRealmSet.includes(selectedRealm);
  const showAmfaSections =
    selectedRealm === "all" ? amfaRealmSet.length > 0 : currentRealmHasAmfa;

  // AMFA KPI counters for the selected realm. AMFA stats are per-realm, so the
  // strip only populates once a specific realm is chosen (dashes for "all").
  // An "all realms" request can instead be refused (501 or 403) or answered for
  // a narrower realm, named in applied_realm_id.
  const { data: amfaStats, error: amfaStatsError } = useAmfaStats({
    tenantId: selectedTenant?.tenant_id,
    realm: selectedRealm !== "all" ? selectedRealm : undefined,
    startTime,
    endTime,
  });
  const amfaAllRealmsUnsupported =
    selectedRealm === "all" && isAllRealmsUnsupportedError(amfaStatsError);
  const amfaRealmScopeRequiresRealm =
    selectedRealm === "all" && isRealmScopeRequiresRealmError(amfaStatsError);
  const amfaAppliedRealm =
    selectedRealm === "all" ? amfaStats?.applied_realm_id : undefined;

  // AMFA geo buckets for the login map. The /amfa/geo endpoint is per-realm, and
  // the map only renders for a specific AMFA-enabled realm, so gate the query on
  // that to avoid pointless calls for non-AMFA realms.
  const realmForAmfa = currentRealmHasAmfa ? selectedRealm : undefined;
  const { data: amfaGeo } = useAmfaGeo({
    tenantId: selectedTenant?.tenant_id,
    realm: realmForAmfa,
    startTime,
    endTime,
  });

  const events = useMemo<EventWithFormattedDate[]>(() => {
    const eventsList = eventsResponse?.events || [];
    // Pre-format dates to avoid repeated toLocaleString() calls during rendering
    return eventsList.map((event) => ({
      ...event,
      formattedTimestamp: new Date(event.timestamp).toLocaleString(),
    }));
  }, [eventsResponse]);

  const currentPage = Math.floor(offset / limit) + 1;
  const totalPages = eventsResponse
    ? Math.ceil(eventsResponse.total / limit)
    : 1;

  const handlePrevPage = useCallback(() => {
    setOffset((prev) => (prev >= limit ? prev - limit : prev));
  }, [limit]);

  const handleNextPage = useCallback(() => {
    if (offset + limit < (eventsResponse?.total || 0)) {
      setOffset(offset + limit);
    }
  }, [offset, limit, eventsResponse?.total]);

  // Memoize event search from URL to avoid repeated linear searches
  const eventFromUrl = useMemo(() => {
    const eventId = searchParams.get("event");
    if (eventId && events.length > 0) {
      return events.find((e) => e.event_id === eventId);
    }
    return undefined;
  }, [searchParams, events]);

  // Handle URL-based event selection
  useEffect(() => {
    if (eventFromUrl) {
      setSelectedEvent(eventFromUrl);
    }
  }, [eventFromUrl]);

  const handleEventClick = useCallback(
    (event: EventType) => {
      setSelectedEvent(event);
      setSearchParams((prev) => {
        const newParams = new URLSearchParams(prev);
        newParams.set("event", event.event_id);
        return newParams;
      });
    },
    [setSearchParams],
  );

  const handleCloseEvent = useCallback(() => {
    setSelectedEvent(null);
    setDetailsDialogEntered(false);
    setSearchParams((prev) => {
      const newParams = new URLSearchParams(prev);
      newParams.delete("event");
      return newParams;
    });
  }, [setSearchParams]);

  // Define columns for Material React Table
  const columns = useMemo<MRT_ColumnDef<EventWithFormattedDate>[]>(
    () => [
      {
        accessorKey: "formattedTimestamp",
        header: "Timestamp",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<EventWithFormattedDate, unknown>;
        }) => (
          <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
            {cell.getValue() as string}
          </Typography>
        ),
      },
      {
        accessorKey: "type",
        header: "Type",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<EventWithFormattedDate, unknown>;
        }) => (
          <Typography variant="body2" sx={{ fontWeight: 500 }}>
            {cell.getValue() as string}
          </Typography>
        ),
      },
      {
        accessorKey: "username",
        header: "User",
        grow: true,
        Cell: ({ row }: { row: MRT_Row<EventWithFormattedDate> }) => {
          if (!row.original.user_id) {
            return (
              <Typography variant="body2" color="text.secondary">
                -
              </Typography>
            );
          }
          return (
            <Box>
              <Typography variant="body2">
                {row.original.username ||
                  row.original.email ||
                  row.original.user_id ||
                  "Unknown User"}
              </Typography>
              {row.original.username && row.original.email && (
                <Typography variant="caption" color="text.secondary">
                  {row.original.email}
                </Typography>
              )}
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ fontFamily: "monospace", display: "block" }}
              >
                {row.original.user_id}
              </Typography>
            </Box>
          );
        },
      },
      {
        accessorKey: "client_id",
        header: "Client",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<EventWithFormattedDate, unknown>;
        }) => (
          <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
            {(cell.getValue() as string) || (
              <Typography component="span" color="text.secondary">
                -
              </Typography>
            )}
          </Typography>
        ),
      },
      {
        accessorKey: "source",
        header: "Source",
        grow: true,
      },
      {
        accessorKey: "description",
        header: "Description",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<EventWithFormattedDate, unknown>;
        }) => (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {cell.getValue() as string}
          </Typography>
        ),
      },
      {
        accessorKey: "severity",
        header: "Severity",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<EventWithFormattedDate, unknown>;
        }) => (
          <Chip
            label={cell.getValue() as string}
            color={getSeverityColor(cell.getValue() as string)}
            size="small"
          />
        ),
      },
      // Risk column only for AMFA-enabled realms (see showAmfaSections).
      ...(showAmfaSections
        ? [
            {
              accessorKey: "risk_level",
              header: "Risk",
              size: 80,
              Cell: ({ row }: { row: MRT_Row<EventWithFormattedDate> }) => (
                <RiskBadge risk={row.original.risk_level} />
              ),
            } as MRT_ColumnDef<EventWithFormattedDate>,
          ]
        : []),
      {
        id: "actions",
        header: "Actions",
        size: 80,
        Cell: ({ row }: { row: MRT_Row<EventWithFormattedDate> }) => (
          <Box sx={{ display: "flex", gap: 0.5 }}>
            <Tooltip title="View details">
              <IconButton
                size="small"
                aria-label="View event details"
                onClick={(e) => {
                  e.stopPropagation();
                  handleEventClick(row.original);
                }}
                sx={{ color: "primary.main" }}
              >
                <VisibilityIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Box>
        ),
      },
    ],
    [handleEventClick, showAmfaSections],
  );

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      {/* Header with filters */}
      <PageHeader title="Events" subtitle="View and analyze security events">
        <RealmSelector
          selectedRealm={selectedRealm}
          onRealmChange={handleRealmChange}
          realms={realms}
          defaultRealm={selectedTenant?.default_realm}
        />
        <TimeSelector
          onTimeRangeChange={handleTimeRangeChange}
          initialStart={initialStart}
          initialEnd={initialEnd}
        />
        <PageSizeSelector
          value={limit}
          onChange={(value) => {
            setPageSize(value);
            setLimit(value);
            setOffset(0);
          }}
          options={[25, 50, 100, 200]}
          label="events"
        />
        <Button
          variant="contained"
          size="small"
          startIcon={<Refresh />}
          onClick={() => refetch()}
        >
          Refresh
        </Button>
      </PageHeader>

      {/* AMFA KPI strip: only for AMFA-enabled realms (or "all" when any realm
          uses AMFA). Empty/dashes when a specific AMFA realm has no data yet.
          When "All Realms" is selected and the numbers can't cover every realm,
          say which of the two reasons applies, or which realm they do cover. */}
      {showAmfaSections &&
        (amfaAllRealmsUnsupported ? (
          <AlertBanner
            severity="info"
            message="This tenant's AMFA integration doesn't support aggregating stats across all realms. Select a specific realm to see AMFA metrics."
            sx={{ mb: 3 }}
          />
        ) : amfaRealmScopeRequiresRealm ? (
          <AlertBanner
            severity="info"
            message="Your access is limited to specific realms, so AMFA metrics can't be shown across all of them. Select one of your realms."
            sx={{ mb: 3 }}
          />
        ) : (
          <>
            {amfaAppliedRealm && (
              <AlertBanner
                severity="info"
                message={`Your access is limited to ${amfaAppliedRealm}, so these AMFA metrics cover that realm rather than all realms.`}
                sx={{ mb: 2 }}
              />
            )}
            <AmfaKpiRow stats={amfaStats} />
          </>
        ))}

      {/* Material React Table */}
      <FeatureErrorBoundary
        featureId="EventsTable"
        title="Events Table"
        resetKeys={[selectedTenant?.tenant_id, selectedRealm]}
      >
        <MaterialReactTable
          {...(MRT_OPTIONS_WITH_TOOLBAR as object)}
          columns={columns}
          data={events}
          state={{
            isLoading,
            pagination: { pageIndex: currentPage - 1, pageSize: limit },
          }}
          enablePagination={false}
          enableSorting
          enableColumnFilters
          enableGlobalFilter
          muiPaginationProps={{
            rowsPerPageOptions: [25, 50, 100, 200],
            showFirstButton: false,
            showLastButton: false,
          }}
          renderTopToolbarCustomActions={() => (
            <Box sx={{ px: 1, py: 0.5 }}>
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 700,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                }}
              >
                Security Events
              </Typography>
            </Box>
          )}
          renderBottomToolbarCustomActions={() => (
            <Box
              sx={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                width: "100%",
                px: 2,
                py: 1,
              }}
            >
              <Typography variant="body2" color="text.secondary">
                Page {currentPage} of {totalPages} ({eventsResponse?.total || 0}{" "}
                total events)
              </Typography>
              <Box sx={{ display: "flex", gap: 1 }}>
                <IconButton
                  onClick={handlePrevPage}
                  disabled={offset === 0}
                  size="small"
                >
                  ←
                </IconButton>
                <IconButton
                  onClick={handleNextPage}
                  disabled={offset + limit >= (eventsResponse?.total || 0)}
                  size="small"
                >
                  →
                </IconButton>
              </Box>
            </Box>
          )}
        />
      </FeatureErrorBoundary>

      {/* AMFA geolocation map: per-realm, so it only shows once a specific
          realm is selected. Collapsible, expanded by default, and placed below
          the events table so the list stays the first thing on the page. */}
      {realmForAmfa && (
        <Accordion defaultExpanded sx={{ mt: 2 }}>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 700,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              Geolocation Map
            </Typography>
          </AccordionSummary>
          <AccordionDetails>
            <FeatureErrorBoundary
              featureId="AmfaGeoMap"
              title="AMFA Geo Map"
              resetKeys={[selectedTenant?.tenant_id, selectedRealm]}
            >
              <AmfaGeoMap buckets={amfaGeo ?? []} />
            </FeatureErrorBoundary>
          </AccordionDetails>
        </Accordion>
      )}

      {/* Event Details Modal */}
      <Dialog
        open={!!selectedEvent}
        onClose={handleCloseEvent}
        maxWidth="md"
        fullWidth
        // Leaflet measures its container once and a scale transform
        // mid-animation makes it measure wrong, so the map inside AMFA Details
        // waits for this. See LocationMap's sizing note.
        TransitionProps={{ onEntered: () => setDetailsDialogEntered(true) }}
      >
        {selectedEvent && (
          <>
            <DialogTitle
              sx={{
                bgcolor: "background.paper",
                borderBottom: 1,
                borderColor: "divider",
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
              }}
            >
              <Typography
                variant="h6"
                // DialogTitle already renders an <h2>, so an <h6> inside it is
                // invalid nesting and React warns. Same look, valid DOM.
                component="span"
                sx={{
                  fontWeight: 700,
                  textTransform: "uppercase",
                  letterSpacing: "0.02em",
                }}
              >
                Event Details
              </Typography>
              <IconButton
                onClick={handleCloseEvent}
                size="small"
                sx={{ color: "text.secondary" }}
              >
                <CloseIcon />
              </IconButton>
            </DialogTitle>
            <DialogContent sx={{ p: 3 }}>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                    }}
                  >
                    Event ID
                  </Typography>
                  <Typography
                    variant="body2"
                    sx={{ fontFamily: "monospace", mt: 0.5 }}
                  >
                    {selectedEvent.event_id}
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                    }}
                  >
                    Timestamp
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    {new Date(selectedEvent.timestamp).toLocaleString()}
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                    }}
                  >
                    Type
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    {selectedEvent.type}
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                    }}
                  >
                    Category
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    {selectedEvent.category}
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                    }}
                  >
                    Severity
                  </Typography>
                  <Box sx={{ mt: 0.5 }}>
                    <Chip
                      label={selectedEvent.severity}
                      color={getSeverityColor(selectedEvent.severity)}
                      size="small"
                    />
                  </Box>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                    }}
                  >
                    Description
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    {selectedEvent.description}
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                    }}
                  >
                    Source
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    {selectedEvent.source}
                  </Typography>
                </Box>
                {selectedEvent.source_ip && (
                  <Box>
                    <Typography
                      variant="caption"
                      sx={{
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        color: "text.secondary",
                      }}
                    >
                      Source IP
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{ fontFamily: "monospace", mt: 0.5 }}
                    >
                      {selectedEvent.source_ip}
                    </Typography>
                  </Box>
                )}
                {selectedEvent.user_id && (
                  <Box>
                    <Typography
                      variant="caption"
                      sx={{
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        color: "text.secondary",
                      }}
                    >
                      User
                    </Typography>
                    <Box sx={{ mt: 0.5 }}>
                      <Link
                        to={`/${selectedTenant?.tenant_id}/realm/${selectedEvent.source.replace("keycloak:", "")}/user/${selectedEvent.user_id}`}
                        style={{
                          color: "inherit",
                          textDecoration: "none",
                        }}
                      >
                        <Typography
                          variant="body2"
                          sx={{
                            color: "primary.main",
                            fontWeight: 500,
                            display: "flex",
                            alignItems: "center",
                            gap: 0.5,
                            "&:hover": {
                              textDecoration: "underline",
                            },
                          }}
                        >
                          {selectedEvent.username ||
                            selectedEvent.email ||
                            selectedEvent.user_id ||
                            "Unknown User"}
                          <OpenInNewIcon sx={{ fontSize: 16 }} />
                        </Typography>
                      </Link>
                      {selectedEvent.email && selectedEvent.username && (
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          sx={{ display: "block", mt: 0.5 }}
                        >
                          {selectedEvent.email}
                        </Typography>
                      )}
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{
                          fontFamily: "monospace",
                          display: "block",
                          mt: 0.5,
                        }}
                      >
                        {selectedEvent.user_id}
                      </Typography>
                    </Box>
                  </Box>
                )}
                {selectedEvent.client_id && (
                  <Box>
                    <Typography
                      variant="caption"
                      sx={{
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        color: "text.secondary",
                      }}
                    >
                      Client
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{ fontFamily: "monospace", mt: 0.5 }}
                    >
                      {selectedEvent.client_id}
                    </Typography>
                  </Box>
                )}
                {(selectedEvent.amfa_event_id ||
                  selectedEvent.risk_level != null) && (
                  <Box
                    sx={{
                      borderTop: 1,
                      borderColor: "divider",
                      pt: 2,
                      display: "flex",
                      flexDirection: "column",
                      gap: 2,
                    }}
                  >
                    <Typography
                      variant="subtitle2"
                      sx={{
                        fontWeight: 700,
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                      }}
                    >
                      AMFA Details
                    </Typography>
                    <Box>
                      <Typography
                        variant="caption"
                        sx={{
                          textTransform: "uppercase",
                          letterSpacing: "0.05em",
                          color: "text.secondary",
                        }}
                      >
                        Risk Level
                      </Typography>
                      <Box sx={{ mt: 0.5 }}>
                        <RiskBadge risk={selectedEvent.risk_level} />
                      </Box>
                    </Box>
                    {selectedEvent.final_status && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{
                            textTransform: "uppercase",
                            letterSpacing: "0.05em",
                            color: "text.secondary",
                          }}
                        >
                          Final Status
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 0.5 }}>
                          {selectedEvent.final_status}
                        </Typography>
                      </Box>
                    )}
                    {selectedEvent.is_vpn != null && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{
                            textTransform: "uppercase",
                            letterSpacing: "0.05em",
                            color: "text.secondary",
                          }}
                        >
                          VPN
                        </Typography>
                        <Box sx={{ mt: 0.5 }}>
                          <Chip
                            label={
                              selectedEvent.is_vpn ? "VPN detected" : "No VPN"
                            }
                            color={selectedEvent.is_vpn ? "warning" : "default"}
                            size="small"
                          />
                        </Box>
                      </Box>
                    )}
                    {selectedEvent.country && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{
                            textTransform: "uppercase",
                            letterSpacing: "0.05em",
                            color: "text.secondary",
                          }}
                        >
                          Country
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 0.5 }}>
                          {selectedEvent.country}
                        </Typography>
                      </Box>
                    )}
                    {hasEventLocation(selectedEvent) && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{
                            textTransform: "uppercase",
                            letterSpacing: "0.05em",
                            color: "text.secondary",
                          }}
                        >
                          Coordinates
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{ fontFamily: "monospace", mt: 0.5 }}
                        >
                          {selectedEvent.lat}, {selectedEvent.long}
                        </Typography>
                        <Box sx={{ mt: 1 }}>
                          <FeatureErrorBoundary
                            featureId="EventLocationMap"
                            title="Event Location Map"
                            resetKeys={[selectedEvent.event_id]}
                          >
                            {detailsDialogEntered ? (
                              <LocationMap
                                lat={selectedEvent.lat!}
                                long={selectedEvent.long!}
                                label={
                                  [selectedEvent.city, selectedEvent.country]
                                    .filter(Boolean)
                                    .join(", ") || undefined
                                }
                                detail={selectedEvent.source_ip}
                                isProxied={selectedEvent.is_vpn}
                                timestamp={new Date(
                                  selectedEvent.timestamp,
                                ).toLocaleString()}
                                height={EMBEDDED_MAP_HEIGHT}
                              />
                            ) : (
                              // Reserves the map's height so the dialog does
                              // not resize when the map mounts a frame later.
                              <Box sx={{ height: EMBEDDED_MAP_HEIGHT }} />
                            )}
                          </FeatureErrorBoundary>
                        </Box>
                      </Box>
                    )}
                    {(
                      [
                        { label: "City", value: selectedEvent.city },
                        {
                          label: "Operating System",
                          value: selectedEvent.operating_system,
                        },
                        { label: "Browser", value: selectedEvent.browser },
                        { label: "Device", value: selectedEvent.device },
                        {
                          label: "System Language",
                          value: selectedEvent.system_language,
                        },
                        {
                          label: "Screen Resolution",
                          value: selectedEvent.screen_resolution,
                        },
                      ] as const
                    )
                      .filter((f) => f.value)
                      .map((f) => (
                        <Box key={f.label}>
                          <Typography
                            variant="caption"
                            sx={{
                              textTransform: "uppercase",
                              letterSpacing: "0.05em",
                              color: "text.secondary",
                            }}
                          >
                            {f.label}
                          </Typography>
                          <Typography variant="body2" sx={{ mt: 0.5 }}>
                            {f.value}
                          </Typography>
                        </Box>
                      ))}
                  </Box>
                )}
              </Box>
            </DialogContent>
          </>
        )}
      </Dialog>
    </Box>
  );
}
