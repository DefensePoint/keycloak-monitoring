import { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import { AuthProvider, ToastProvider } from "@/shared/context";
import { DefensePointThemeProvider } from "@/theme/ThemeProvider";
import { ToastContainer } from "@/shared/components";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

interface AppProvidersProps {
  children: ReactNode;
}

export function AppProviders({ children }: AppProvidersProps) {
  return (
    <DefensePointThemeProvider>
      <QueryClientProvider client={queryClient}>
        <ToastProvider maxToasts={3}>
          <AuthProvider>
            <BrowserRouter>{children}</BrowserRouter>
          </AuthProvider>
          <ToastContainer />
        </ToastProvider>
      </QueryClientProvider>
    </DefensePointThemeProvider>
  );
}
