import { ReactNode } from "react";
import { useAuth, TenantProvider } from "@/shared/context";
import { LoginPage } from "@/features/auth";
import { Layout } from "@/shared/components/Layout";
import { ServerUnavailable } from "./ServerUnavailable";

interface ProtectedRouteProps {
  children: ReactNode;
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const {
    isAuthenticated,
    isLoading,
    serverUnavailable,
    nextRetryAt,
    retryNow,
  } = useAuth();

  if (isLoading) {
    return (
      <div className="min-h-screen bg-defense-bg-primary flex items-center justify-center">
        <div className="text-center">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-defense-border-primary border-t-defense-red-primary mb-4"></div>
          <p className="text-defense-text-secondary uppercase tracking-wide text-sm">
            Loading...
          </p>
        </div>
      </div>
    );
  }

  // Not "signed out": the server could not be asked. The sign-in card would
  // have no way to sign in, and would stay after the server came back.
  if (serverUnavailable) {
    return (
      <ServerUnavailable nextRetryAt={nextRetryAt} onRetryNow={retryNow} />
    );
  }

  if (!isAuthenticated) {
    return <LoginPage />;
  }

  return (
    <TenantProvider>
      <Layout>{children}</Layout>
    </TenantProvider>
  );
}
