import React from "react";
import { Box, IconButton, Typography } from "@mui/material";
import { ArrowBack } from "@mui/icons-material";
import { useNavigate } from "react-router-dom";

export const UserDetailsHeader: React.FC = () => {
  const navigate = useNavigate();

  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 2, mb: 3 }}>
      <IconButton
        onClick={() => navigate(-1)}
        sx={{
          color: "text.secondary",
          "&:hover": {
            color: "text.primary",
          },
        }}
      >
        <ArrowBack />
      </IconButton>
      <Box>
        <Typography variant="h4">User Details</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          View user information and related events
        </Typography>
      </Box>
    </Box>
  );
};
