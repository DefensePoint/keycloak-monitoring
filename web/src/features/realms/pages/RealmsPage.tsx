import { useState, useCallback, useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Box, Typography, Pagination } from "@mui/material";
import { useTenant } from "@/shared/context";
import { useKeycloakDashboard } from "@/shared/hooks";
import {
  isTenantConnectionBroken,
  tenantConnectionError,
} from "@/shared/utils";
import { RealmsPageHeader, RealmsGrid } from "../components";
import type { RealmGridItem } from "../types";

export function RealmsPage() {
  const navigate = useNavigate();
  const { tenantId } = useParams<{ tenantId: string }>();
  const { selectedTenant } = useTenant();
  const [currentPage, setCurrentPage] = useState(1);
  const itemsPerPage = 9;

  // Reset to first page when navigating to this page
  useEffect(() => {
    setCurrentPage(1);
  }, []);

  const {
    data: keycloakDashboard,
    isLoading,
    isError,
  } = useKeycloakDashboard({
    tenantId: selectedTenant?.tenant_id,
  });

  const allRealms: RealmGridItem[] =
    keycloakDashboard && "realms" in keycloakDashboard
      ? keycloakDashboard.realms
      : [];

  const connectionBroken =
    isTenantConnectionBroken(selectedTenant) ||
    (isError && allRealms.length === 0);
  const connectionError = tenantConnectionError(selectedTenant);

  // Pagination
  const totalPages = Math.ceil(allRealms.length / itemsPerPage);
  const startIndex = (currentPage - 1) * itemsPerPage;
  const paginatedRealms = allRealms.slice(
    startIndex,
    startIndex + itemsPerPage,
  );

  const handleRealmClick = useCallback(
    (realmName: string) => {
      navigate(`/${tenantId}/realm/${realmName}`);
    },
    [navigate, tenantId],
  );

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      {/* Header */}
      <RealmsPageHeader />

      {/* Results Count */}
      {allRealms.length > 0 && (
        <Box sx={{ mb: 2 }}>
          <Typography variant="body2" color="text.secondary">
            {allRealms.length} realm{allRealms.length !== 1 ? "s" : ""}
            {totalPages > 1 && ` - Page ${currentPage} of ${totalPages}`}
          </Typography>
        </Box>
      )}

      {/* Grid */}
      <RealmsGrid
        realms={paginatedRealms}
        onRealmClick={handleRealmClick}
        loading={isLoading}
        connectionBroken={connectionBroken}
        connectionError={connectionError}
      />

      {/* Pagination */}
      {totalPages > 1 && (
        <Box
          sx={{
            mt: 3,
            display: "flex",
            justifyContent: "center",
          }}
        >
          <Pagination
            count={totalPages}
            page={currentPage}
            onChange={(_, page) => setCurrentPage(page)}
            color="primary"
            size="large"
            showFirstButton
            showLastButton
          />
        </Box>
      )}
    </Box>
  );
}
