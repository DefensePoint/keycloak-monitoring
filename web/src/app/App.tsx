import { AppProviders } from "./providers";
import { AppRoutes } from "./routes";
import "@/App.css";

export function App() {
  return (
    <AppProviders>
      <AppRoutes />
    </AppProviders>
  );
}
