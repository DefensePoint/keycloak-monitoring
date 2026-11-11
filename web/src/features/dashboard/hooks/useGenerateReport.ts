import { useMutation } from "@tanstack/react-query";
import { dashboardService } from "../services";
import { useBranding } from "@/shared/hooks/useBranding";
import { filePrefix } from "@/shared/lib/branding";

interface GenerateReportParams {
  tenantId: string;
  tenantName: string;
  startDate: string;
  endDate: string;
}

export function useGenerateReport() {
  const branding = useBranding();
  return useMutation({
    mutationFn: async ({
      tenantId,
      tenantName,
      startDate,
      endDate,
    }: GenerateReportParams) => {
      const blob = await dashboardService.generateMonthlyReport(
        tenantId,
        startDate,
        endDate,
      );

      const url = window.URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `${filePrefix(branding)}-report-${tenantName}-${startDate}-to-${endDate}.pdf`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    },
  });
}
