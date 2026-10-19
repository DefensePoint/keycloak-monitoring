import { useState, useCallback } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { Box } from "@mui/material";
import { useTenant } from "@/shared/context";
import { AlertBanner } from "@/shared/components";
import {
  UserDetailsHeader,
  UserInfoCard,
  UserEventsFilters,
  UserEventsTable,
} from "../components";
import { useUserDetails, useUserEvents, USER_EVENTS_LIMIT } from "../hooks";
import type { UserEvent } from "../types";

export function UserDetailsPage() {
  const { realmName, userId } = useParams<{
    realmName: string;
    userId: string;
  }>();
  const navigate = useNavigate();
  const { selectedTenant } = useTenant();

  // State for filters
  const [limit, setLimit] = useState(50);
  const [severityFilter, setSeverityFilter] = useState<string>("");
  const [sourceFilter, setSourceFilter] = useState<string>("");
  const [searchTerm, setSearchTerm] = useState<string>("");
  const [startTime, setStartTime] = useState<string | undefined>(undefined);
  const [endTime, setEndTime] = useState<string | undefined>(undefined);

  const handleTimeRangeChange = useCallback(
    (start: Date | null, end: Date | null) => {
      setStartTime(start ? start.toISOString() : undefined);
      setEndTime(end ? end.toISOString() : undefined);
    },
    [],
  );

  // Fetch user details from Keycloak
  const { data: userDetailsResponse, isLoading: userDetailsLoading } =
    useUserDetails({
      tenantId: selectedTenant?.tenant_id,
      realmName,
      userId,
    });

  const userDetails = userDetailsResponse?.user;
  const userGroups = userDetailsResponse?.groups || [];
  const roleMappings = userDetailsResponse?.roleMappings;

  // Fetch this user's events in the realm named in the URL
  const { data: eventsResponse, isLoading: eventsLoading } = useUserEvents({
    tenantId: selectedTenant?.tenant_id,
    realmName,
    userId,
    startTime,
    endTime,
    severity: severityFilter || undefined,
  });

  const events = eventsResponse?.events || [];
  // An API that predates the user_id filter ignores it and answers with the
  // tenant's events; any other user's row gives that away. Its total is then
  // the tenant's, so no count may be claimed for this user.
  const serverScoped = events.every(
    (event: UserEvent) => event.user_id === userId,
  );
  // The server caps a page at 1000 rows; say so rather than let a longer
  // history look complete.
  const totalForUser = eventsResponse?.total ?? 0;
  const truncated = serverScoped && totalForUser > events.length;
  // A total below the rows returned cannot be true: /events answers 0 when
  // its count query fails. If the list also hit the cap it may be partial,
  // so warn without claiming a number.
  const maybeTruncated =
    serverScoped &&
    totalForUser < events.length &&
    events.length >= USER_EVENTS_LIMIT;

  // The server already scopes by user_id. This check stays so the page is
  // no worse than before against an API that predates the user_id filter.
  const userEvents = events.filter(
    (event: UserEvent) => event.user_id === userId,
  );

  // Apply additional client-side filtering. Severity is already applied by
  // the server.
  const filteredEvents = userEvents.filter((event: UserEvent) => {
    if (
      sourceFilter &&
      !event.source.toLowerCase().includes(sourceFilter.toLowerCase())
    )
      return false;
    if (searchTerm) {
      const term = searchTerm.toLowerCase();
      return (
        event.description.toLowerCase().includes(term) ||
        event.type.toLowerCase().includes(term)
      );
    }
    return true;
  });

  const handleResetFilters = useCallback(() => {
    setSeverityFilter("");
    setSourceFilter("");
    setSearchTerm("");
  }, []);

  const handleEventClick = useCallback(
    (event: UserEvent) => {
      navigate(`/${selectedTenant?.tenant_id}/events?event=${event.event_id}`);
    },
    [navigate, selectedTenant],
  );

  const isLoading = eventsLoading || userDetailsLoading;

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      {/* Header */}
      <UserDetailsHeader />

      {/* User Information Card */}
      <Box sx={{ mb: 3 }}>
        <UserInfoCard
          userDetails={userDetails}
          userGroups={userGroups}
          roleMappings={roleMappings}
          loading={userDetailsLoading}
        />
      </Box>

      {/* Filters */}
      <UserEventsFilters
        searchTerm={searchTerm}
        severityFilter={severityFilter}
        sourceFilter={sourceFilter}
        limit={limit}
        onSearchChange={setSearchTerm}
        onSeverityChange={setSeverityFilter}
        onSourceChange={setSourceFilter}
        onLimitChange={setLimit}
        onReset={handleResetFilters}
        onTimeRangeChange={handleTimeRangeChange}
      />

      {truncated && (
        <AlertBanner
          severity="warning"
          message={`Showing the newest ${events.length} of ${totalForUser} events for this user. Narrow the time range to see older ones.`}
          sx={{ mb: 2 }}
        />
      )}
      {maybeTruncated && (
        <AlertBanner
          severity="warning"
          message={`Showing the newest ${events.length} events for this user; there may be more. Narrow the time range to see older ones.`}
          sx={{ mb: 2 }}
        />
      )}
      {!serverScoped && (
        <AlertBanner
          severity="warning"
          message="The server could not filter by user, so this list only covers the tenant's most recent events and may be incomplete."
          sx={{ mb: 2 }}
        />
      )}

      {/* Events Table */}
      <UserEventsTable
        events={filteredEvents}
        loading={isLoading}
        onEventClick={handleEventClick}
        pageSize={limit}
      />
    </Box>
  );
}
