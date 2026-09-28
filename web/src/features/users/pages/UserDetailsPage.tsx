import { useState, useCallback } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { Box } from "@mui/material";
import { useTenant } from "@/shared/context";
import {
  UserDetailsHeader,
  UserInfoCard,
  UserEventsFilters,
  UserEventsTable,
} from "../components";
import { useUserDetails, useUserEvents } from "../hooks";
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
  const [offset, setOffset] = useState(0);
  const [severityFilter, setSeverityFilter] = useState<string>("");
  const [sourceFilter, setSourceFilter] = useState<string>("");
  const [searchTerm, setSearchTerm] = useState<string>("");
  const [startTime, setStartTime] = useState<string | undefined>(undefined);
  const [endTime, setEndTime] = useState<string | undefined>(undefined);

  const handleTimeRangeChange = useCallback(
    (start: Date | null, end: Date | null) => {
      setStartTime(start ? start.toISOString() : undefined);
      setEndTime(end ? end.toISOString() : undefined);
      setOffset(0);
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

  // Fetch all events and filter by user_id on the client side
  const { data: eventsResponse, isLoading: eventsLoading } = useUserEvents({
    tenantId: selectedTenant?.tenant_id,
    userId,
    startTime,
    endTime,
  });

  const events = eventsResponse?.events || [];

  // Filter events by user_id
  const userEvents = events.filter(
    (event: UserEvent) => event.user_id === userId,
  );

  // Apply additional client-side filtering
  const filteredEvents = userEvents.filter((event: UserEvent) => {
    if (severityFilter && event.severity !== severityFilter) return false;
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

  // Paginate the filtered results
  const paginatedEvents = filteredEvents.slice(offset, offset + limit);

  const handleResetFilters = useCallback(() => {
    setSeverityFilter("");
    setSourceFilter("");
    setSearchTerm("");
    setOffset(0);
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

      {/* Events Table */}
      <UserEventsTable
        events={paginatedEvents}
        loading={isLoading}
        onEventClick={handleEventClick}
      />
    </Box>
  );
}
