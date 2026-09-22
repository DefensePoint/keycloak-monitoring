import { ReactNode } from "react";
import { useIsAdmin } from "@/shared/hooks";
import { AccessDeniedPage, LoadingSkeleton } from "@/shared/components";

interface AdminRouteProps {
  children: ReactNode;
}

export function AdminRoute({ children }: AdminRouteProps) {
  const { isAdmin, isLoading } = useIsAdmin();

  if (isLoading) {
    return (
      <LoadingSkeleton variant="spinner" message="Checking permissions..." />
    );
  }

  if (!isAdmin) {
    return (
      <AccessDeniedPage
        title="Admin Access Required"
        message="You need administrator privileges to access this page."
      />
    );
  }

  return <>{children}</>;
}
