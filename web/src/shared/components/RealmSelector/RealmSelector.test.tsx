import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { RealmSelector } from "./RealmSelector";

describe("RealmSelector", () => {
  const mockRealms = [
    { realm_name: "master" },
    { realm_name: "test-realm" },
    { realm_name: "dev-realm" },
  ];

  const defaultProps = {
    selectedRealm: "all",
    onRealmChange: vi.fn(),
    realms: mockRealms,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render with 'All Realms' when selectedRealm is 'all'", () => {
    render(<RealmSelector {...defaultProps} />);

    expect(screen.getByRole("button")).toHaveTextContent("All Realms");
  });

  it("should render with realm name when a specific realm is selected", () => {
    render(<RealmSelector {...defaultProps} selectedRealm="master" />);

    expect(screen.getByRole("button")).toHaveTextContent("master");
  });

  it("should open menu when button is clicked", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    const button = screen.getByRole("button");
    await user.click(button);

    await waitFor(() => {
      expect(screen.getByRole("menu")).toBeInTheDocument();
    });
  });

  it("should display all realm options in the menu", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    await waitFor(() => {
      const menuItems = screen.getAllByRole("menuitem");
      expect(menuItems).toHaveLength(4); // All Realms + 3 realms
    });

    // Find menu items by text content
    const menuItems = screen.getAllByRole("menuitem");
    const realmNames = menuItems.map((item) => item.textContent);
    expect(realmNames).toContain("All Realms");
    expect(realmNames.some((name) => name?.includes("master"))).toBe(true);
    expect(realmNames.some((name) => name?.includes("test-realm"))).toBe(true);
    expect(realmNames.some((name) => name?.includes("dev-realm"))).toBe(true);
  });

  it("should call onRealmChange when a realm is selected", async () => {
    const user = userEvent.setup();
    const onRealmChange = vi.fn();
    render(<RealmSelector {...defaultProps} onRealmChange={onRealmChange} />);

    await user.click(screen.getByRole("button"));

    await waitFor(() => {
      expect(screen.getByRole("menu")).toBeInTheDocument();
    });

    const menuItems = screen.getAllByRole("menuitem");
    const masterItem = menuItems.find((item) =>
      item.textContent?.includes("master"),
    );
    if (masterItem) {
      await user.click(masterItem);
    }

    expect(onRealmChange).toHaveBeenCalledWith("master");
  });

  it("should call onRealmChange with 'all' when All Realms is selected", async () => {
    const user = userEvent.setup();
    const onRealmChange = vi.fn();
    render(
      <RealmSelector
        {...defaultProps}
        selectedRealm="master"
        onRealmChange={onRealmChange}
      />,
    );

    await user.click(screen.getByRole("button"));

    await waitFor(() => {
      expect(screen.getByRole("menu")).toBeInTheDocument();
    });

    // Find the "All Realms" menu item
    const menuItems = screen.getAllByRole("menuitem");
    const allRealmsItem = menuItems.find(
      (item) => item.textContent === "All Realms",
    );

    if (allRealmsItem) {
      await user.click(allRealmsItem);
    }

    expect(onRealmChange).toHaveBeenCalledWith("all");
  });

  it("should close menu after selecting a realm", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    await waitFor(() => {
      expect(screen.getByRole("menu")).toBeInTheDocument();
    });

    const menuItems = screen.getAllByRole("menuitem");
    const masterItem = menuItems.find((item) =>
      item.textContent?.includes("master"),
    );
    if (masterItem) {
      await user.click(masterItem);
    }

    await waitFor(() => {
      expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    });
  });

  it("should show 'Default' chip for the default realm", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} defaultRealm="master" />);

    await user.click(screen.getByRole("button"));

    await waitFor(() => {
      expect(screen.getByText("Default")).toBeInTheDocument();
    });
  });

  it("should not show 'Default' chip when no defaultRealm is provided", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    await waitFor(() => {
      expect(screen.getByRole("menu")).toBeInTheDocument();
    });

    expect(screen.queryByText("Default")).not.toBeInTheDocument();
  });

  it("should apply custom className", () => {
    const { container } = render(
      <RealmSelector {...defaultProps} className="custom-class" />,
    );

    expect(container.querySelector(".custom-class")).toBeInTheDocument();
  });

  it("should highlight selected realm in menu", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} selectedRealm="test-realm" />);

    await user.click(screen.getByRole("button"));

    await waitFor(() => {
      const menuItems = screen.getAllByRole("menuitem");
      const selectedItem = menuItems.find((item) =>
        item.textContent?.includes("test-realm"),
      );
      expect(selectedItem).toHaveClass("Mui-selected");
    });
  });
});
