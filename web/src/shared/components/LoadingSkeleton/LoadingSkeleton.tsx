import {
  Box,
  Skeleton,
  CircularProgress,
  Typography,
  SxProps,
  Theme,
} from "@mui/material";

type SkeletonVariant = "table" | "card" | "list" | "text" | "spinner";

export interface LoadingSkeletonProps {
  /** The type of skeleton to display */
  variant: SkeletonVariant;
  /** Number of rows for table variant */
  rows?: number;
  /** Number of cards for card variant */
  count?: number;
  /** Number of items for list variant */
  items?: number;
  /** Number of lines for text variant */
  lines?: number;
  /** Message for spinner variant */
  message?: string;
  /** Size for spinner variant */
  size?: "small" | "medium" | "large";
  /** Minimum height for spinner variant */
  minHeight?: number | string;
  /** Additional sx props */
  sx?: SxProps<Theme>;
}

/**
 * TableSkeleton - Skeleton for data tables
 */
function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <Box sx={{ width: "100%" }}>
      {/* Header row */}
      <Box
        sx={{
          display: "flex",
          gap: 2,
          p: 2,
          borderBottom: "1px solid",
          borderColor: "divider",
        }}
      >
        {[1, 2, 3, 4, 5].map((col) => (
          <Skeleton
            key={col}
            variant="text"
            width={`${100 / 5}%`}
            height={24}
            sx={{ bgcolor: "rgba(255, 255, 255, 0.1)" }}
          />
        ))}
      </Box>
      {/* Data rows */}
      {Array.from({ length: rows }).map((_, rowIndex) => (
        <Box
          key={rowIndex}
          sx={{
            display: "flex",
            gap: 2,
            p: 2,
            borderBottom: "1px solid",
            borderColor: "divider",
          }}
        >
          {[1, 2, 3, 4, 5].map((col) => (
            <Skeleton
              key={col}
              variant="text"
              width={`${100 / 5}%`}
              height={20}
              sx={{ bgcolor: "rgba(255, 255, 255, 0.05)" }}
            />
          ))}
        </Box>
      ))}
    </Box>
  );
}

/**
 * CardSkeleton - Skeleton for card grids
 */
function CardSkeleton({ count = 3 }: { count?: number }) {
  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: {
          xs: "1fr",
          sm: "repeat(2, 1fr)",
          lg: "repeat(3, 1fr)",
        },
        gap: 3,
      }}
    >
      {Array.from({ length: count }).map((_, index) => (
        <Box
          key={index}
          sx={{
            bgcolor: "background.paper",
            border: "1px solid",
            borderColor: "divider",
            borderRadius: 1,
            p: 3,
          }}
        >
          {/* Card header */}
          <Box sx={{ display: "flex", alignItems: "center", gap: 2, mb: 2 }}>
            <Skeleton
              variant="circular"
              width={40}
              height={40}
              sx={{ bgcolor: "rgba(255, 255, 255, 0.1)" }}
            />
            <Box sx={{ flex: 1 }}>
              <Skeleton
                variant="text"
                width="60%"
                height={24}
                sx={{ bgcolor: "rgba(255, 255, 255, 0.1)" }}
              />
              <Skeleton
                variant="text"
                width="40%"
                height={16}
                sx={{ bgcolor: "rgba(255, 255, 255, 0.05)" }}
              />
            </Box>
          </Box>
          {/* Card content */}
          <Skeleton
            variant="text"
            width="100%"
            height={16}
            sx={{ mb: 1, bgcolor: "rgba(255, 255, 255, 0.05)" }}
          />
          <Skeleton
            variant="text"
            width="80%"
            height={16}
            sx={{ bgcolor: "rgba(255, 255, 255, 0.05)" }}
          />
        </Box>
      ))}
    </Box>
  );
}

/**
 * ListSkeleton - Skeleton for list items
 */
function ListSkeleton({ items = 5 }: { items?: number }) {
  return (
    <Box sx={{ width: "100%" }}>
      {Array.from({ length: items }).map((_, index) => (
        <Box
          key={index}
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 2,
            p: 2,
            borderBottom: "1px solid",
            borderColor: "divider",
          }}
        >
          <Skeleton
            variant="circular"
            width={32}
            height={32}
            sx={{ bgcolor: "rgba(255, 255, 255, 0.1)" }}
          />
          <Box sx={{ flex: 1 }}>
            <Skeleton
              variant="text"
              width="50%"
              height={20}
              sx={{ mb: 0.5, bgcolor: "rgba(255, 255, 255, 0.1)" }}
            />
            <Skeleton
              variant="text"
              width="30%"
              height={16}
              sx={{ bgcolor: "rgba(255, 255, 255, 0.05)" }}
            />
          </Box>
          <Skeleton
            variant="rounded"
            width={60}
            height={24}
            sx={{ bgcolor: "rgba(255, 255, 255, 0.1)" }}
          />
        </Box>
      ))}
    </Box>
  );
}

/**
 * TextSkeleton - Skeleton for text blocks
 */
function TextSkeleton({ lines = 4 }: { lines?: number }) {
  return (
    <Box sx={{ width: "100%" }}>
      {Array.from({ length: lines }).map((_, index) => (
        <Skeleton
          key={index}
          variant="text"
          width={index === lines - 1 ? "60%" : "100%"}
          height={20}
          sx={{ mb: 1, bgcolor: "rgba(255, 255, 255, 0.05)" }}
        />
      ))}
    </Box>
  );
}

/**
 * SpinnerSkeleton - Centered loading spinner with optional message
 */
function SpinnerSkeleton({
  message = "Loading...",
  size = "medium",
  minHeight = 200,
}: {
  message?: string;
  size?: "small" | "medium" | "large";
  minHeight?: number | string;
}) {
  const spinnerSize = {
    small: 32,
    medium: 40,
    large: 56,
  }[size];

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight,
        py: 4,
      }}
    >
      <CircularProgress size={spinnerSize} sx={{ mb: 2 }} />
      {message && (
        <Typography variant="body2" color="text.secondary">
          {message}
        </Typography>
      )}
    </Box>
  );
}

/**
 * LoadingSkeleton - Skeleton loading states for different content types
 *
 * Provides layout-stable loading states that prevent content shift
 * when data loads.
 *
 * @example
 * ```tsx
 * <LoadingSkeleton variant="table" rows={10} />
 * <LoadingSkeleton variant="card" count={6} />
 * <LoadingSkeleton variant="list" items={8} />
 * <LoadingSkeleton variant="text" lines={4} />
 * <LoadingSkeleton variant="spinner" message="Loading data..." />
 * ```
 */
export function LoadingSkeleton({
  variant,
  rows,
  count,
  items,
  lines,
  message,
  size,
  minHeight,
  sx,
}: LoadingSkeletonProps) {
  const skeletonContent = () => {
    switch (variant) {
      case "table":
        return <TableSkeleton rows={rows} />;
      case "card":
        return <CardSkeleton count={count} />;
      case "list":
        return <ListSkeleton items={items} />;
      case "text":
        return <TextSkeleton lines={lines} />;
      case "spinner":
        return (
          <SpinnerSkeleton
            message={message}
            size={size}
            minHeight={minHeight}
          />
        );
      default:
        return null;
    }
  };

  return <Box sx={sx}>{skeletonContent()}</Box>;
}
