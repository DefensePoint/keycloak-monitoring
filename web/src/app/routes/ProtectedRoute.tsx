import { ReactNode } from "react";
import { useAuth, TenantProvider } from "@/shared/context";
import { LoginPage } from "@/features/auth";
import { Layout } from "@/shared/components/Layout";

interface ProtectedRouteProps {
  children: ReactNode;
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const { isAuthenticated, isLoading } = useAuth();

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

  if (!isAuthenticated) {
    return <LoginPage />;
  }

  return (
    <TenantProvider>
      <Layout>{children}</Layout>
    </TenantProvider>
  );
}
