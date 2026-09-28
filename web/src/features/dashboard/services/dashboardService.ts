import { apiClient } from "@/shared/lib/apiClient";

class DashboardService {
  async generateMonthlyReport(
    tenantId: string,
    startDate: string,
    endDate: string,
  ): Promise<Blob> {
    const params = new URLSearchParams({
      start_date: startDate,
      end_date: endDate,
    });
    return apiClient.getBlob(
      `/tenants/${tenantId}/reports/generate?${params.toString()}`,
    );
  }
}

export const dashboardService = new DashboardService();
