import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AlertActionsMenu } from "./AlertActionsMenu";
import type { ConfigurationAlert } from "../types";

describe("AlertActionsMenu", () => {
  const mockOnClose = vi.fn();
  const mockOnAcknowledge = vi.fn();
  const mockOnIgnore = vi.fn();
  const mockOnResolve = vi.fn();

  const mockAlert: ConfigurationAlert = {
    alert_id: "alert-1",
    tenant_id: "tenant-1",
    realm_name: "test-realm",
    source: "keycloak",
    type: "configuration",
    severity: "critical",
    status: "active",
    title: "Test Alert",
    description: "Test description",
    recommendation: "Fix it",
    check_type: "password-policy",
    resource_type: "realm",
    resource_id: "res-1",
    resource_name: "Test Resource",
    first_detected: "2024-01-01",
    last_seen: "2024-01-02",
    metadata: "{}",
  };

  const defaultProps = {
    anchorEl: null as HTMLElement | null,
    selectedAlert: mockAlert,
    onClose: mockOnClose,
    onAcknowledge: mockOnAcknowledge,
    onIgnore: mockOnIgnore,
    onResolve: mockOnResolve,
    canAcknowledge: true,
    canResolve: true,
    isActionPending: false,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should not render menu when anchorEl is null", () => {
    render(<AlertActionsMenu {...defaultProps} anchorEl={null} />);

    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("should render menu when anchorEl is provided", () => {
    const anchorEl = document.createElement("div");
    render(<AlertActionsMenu {...defaultProps} anchorEl={anchorEl} />);

    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  describe("when alert status is active", () => {
    it("should render acknowledge, ignore, and resolve menu items", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "active" }}
        />,
      );

      expect(screen.getByText("Acknowledge")).toBeInTheDocument();
      expect(screen.getByText("Ignore")).toBeInTheDocument();
      expect(screen.getByText("Resolve")).toBeInTheDocument();
    });

    it("should call onAcknowledge when acknowledge is clicked", async () => {
      const user = userEvent.setup();
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "active" }}
        />,
      );

      await user.click(screen.getByText("Acknowledge"));
      expect(mockOnAcknowledge).toHaveBeenCalledTimes(1);
    });
  });

  describe("when alert status is acknowledged", () => {
    it("should not render acknowledge menu item", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "acknowledged" }}
        />,
      );

      expect(screen.queryByText("Acknowledge")).not.toBeInTheDocument();
    });

    it("should render ignore and resolve menu items", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "acknowledged" }}
        />,
      );

      expect(screen.getByText("Ignore")).toBeInTheDocument();
      expect(screen.getByText("Resolve")).toBeInTheDocument();
    });
  });

  describe("when alert status is resolved", () => {
    it("should not render resolve menu item", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "resolved" }}
        />,
      );

      expect(screen.queryByText("Resolve")).not.toBeInTheDocument();
    });

    it("should render ignore menu item", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "resolved" }}
        />,
      );

      expect(screen.getByText("Ignore")).toBeInTheDocument();
    });
  });

  it("should call onIgnore when ignore is clicked", async () => {
    const user = userEvent.setup();
    const anchorEl = document.createElement("div");
    render(<AlertActionsMenu {...defaultProps} anchorEl={anchorEl} />);

    await user.click(screen.getByText("Ignore"));
    expect(mockOnIgnore).toHaveBeenCalledTimes(1);
  });

  it("should call onResolve when resolve is clicked", async () => {
    const user = userEvent.setup();
    const anchorEl = document.createElement("div");
    render(<AlertActionsMenu {...defaultProps} anchorEl={anchorEl} />);

    await user.click(screen.getByText("Resolve"));
    expect(mockOnResolve).toHaveBeenCalledTimes(1);
  });

  it("should call onClose when menu is closed", async () => {
    const user = userEvent.setup();
    const anchorEl = document.createElement("div");
    render(<AlertActionsMenu {...defaultProps} anchorEl={anchorEl} />);

    // Click outside to close menu
    await user.keyboard("{Escape}");
    expect(mockOnClose).toHaveBeenCalled();
  });

  describe("permission handling", () => {
    it("should show no permission message when user has no permissions", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          canAcknowledge={false}
          canResolve={false}
        />,
      );

      expect(
        screen.getByText("No permission to manage alerts"),
      ).toBeInTheDocument();
      expect(screen.queryByText("Acknowledge")).not.toBeInTheDocument();
      expect(screen.queryByText("Ignore")).not.toBeInTheDocument();
      expect(screen.queryByText("Resolve")).not.toBeInTheDocument();
    });

    it("should show only acknowledge/ignore when user has only canAcknowledge", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "active" }}
          canAcknowledge={true}
          canResolve={false}
        />,
      );

      expect(screen.getByText("Acknowledge")).toBeInTheDocument();
      expect(screen.getByText("Ignore")).toBeInTheDocument();
      expect(screen.queryByText("Resolve")).not.toBeInTheDocument();
    });

    it("should show only resolve when user has only canResolve", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "active" }}
          canAcknowledge={false}
          canResolve={true}
        />,
      );

      expect(screen.queryByText("Acknowledge")).not.toBeInTheDocument();
      expect(screen.queryByText("Ignore")).not.toBeInTheDocument();
      expect(screen.getByText("Resolve")).toBeInTheDocument();
    });

    it("should disable menu items when isActionPending is true", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "active" }}
          isActionPending={true}
        />,
      );

      const processingItems = screen.getAllByText("Processing...");
      // All menu items should be disabled
      processingItems.forEach((item) => {
        expect(item.closest("li")).toHaveAttribute("aria-disabled", "true");
      });
    });

    it("should show Processing... text when action is pending", () => {
      const anchorEl = document.createElement("div");
      render(
        <AlertActionsMenu
          {...defaultProps}
          anchorEl={anchorEl}
          selectedAlert={{ ...mockAlert, status: "active" }}
          isActionPending={true}
        />,
      );

      expect(screen.getAllByText("Processing...").length).toBeGreaterThan(0);
    });
  });
});
