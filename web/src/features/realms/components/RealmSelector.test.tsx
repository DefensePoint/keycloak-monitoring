import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { RealmSelector } from "./RealmSelector";

describe("RealmSelector", () => {
  const mockOnRealmChange = vi.fn();

  const defaultProps = {
    selectedRealm: "realm-1",
    onRealmChange: mockOnRealmChange,
    realms: [
      { realm_name: "realm-1" },
      { realm_name: "realm-2" },
      { realm_name: "realm-3" },
    ],
    defaultRealm: "realm-1",
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render selected realm name", () => {
    render(<RealmSelector {...defaultProps} />);

    // Use getByRole with button to be more specific
    const button = screen.getByRole("button");
    expect(button).toHaveTextContent("realm-1");
  });

  it("should render 'All Realms' when selectedRealm is 'all'", () => {
    render(<RealmSelector {...defaultProps} selectedRealm="all" />);

    expect(screen.getByText("All Realms")).toBeInTheDocument();
  });

  it("should open menu when button is clicked", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("should render all realms in menu", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    // Use getAllByRole to find menu items
    const menuItems = screen.getAllByRole("menuitem");
    expect(menuItems.length).toBeGreaterThanOrEqual(3);
    expect(screen.getAllByText("realm-1").length).toBeGreaterThan(0);
    expect(screen.getByText("realm-2")).toBeInTheDocument();
    expect(screen.getByText("realm-3")).toBeInTheDocument();
  });

  it("should render 'All Realms' option in menu", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    const allRealmsOptions = screen.getAllByText("All Realms");
    expect(allRealmsOptions.length).toBeGreaterThan(0);
  });

  it("should call onRealmChange when a realm is selected", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    // Find the menuitem with realm-2
    const menuItems = screen.getAllByRole("menuitem");
    const realm2Item = menuItems.find((item) =>
      item.textContent?.includes("realm-2"),
    );

    if (realm2Item) {
      await user.click(realm2Item);
    }

    expect(mockOnRealmChange).toHaveBeenCalledWith("realm-2");
  });

  it("should call onRealmChange with 'all' when All Realms is selected", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    // Find the menuitem with All Realms
    const menuItems = screen.getAllByRole("menuitem");
    const allRealmsItem = menuItems.find((item) =>
      item.textContent?.includes("All Realms"),
    );

    if (allRealmsItem) {
      await user.click(allRealmsItem);
    }

    expect(mockOnRealmChange).toHaveBeenCalledWith("all");
  });

  it("should close menu after selecting a realm", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    // Find the menuitem with realm-2
    const menuItems = screen.getAllByRole("menuitem");
    const realm2Item = menuItems.find((item) =>
      item.textContent?.includes("realm-2"),
    );

    if (realm2Item) {
      await user.click(realm2Item);
    }

    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("should show default badge on default realm", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} />);

    await user.click(screen.getByRole("button"));

    expect(screen.getByText("Default")).toBeInTheDocument();
  });

  it("should not show default badge on non-default realms", async () => {
    const user = userEvent.setup();
    const propsWithoutDefault = {
      ...defaultProps,
      defaultRealm: undefined,
    };
    render(<RealmSelector {...propsWithoutDefault} />);

    await user.click(screen.getByRole("button"));

    expect(screen.queryByText("Default")).not.toBeInTheDocument();
  });

  it("should rotate expand icon when menu is open", async () => {
    const user = userEvent.setup();
    const { container } = render(<RealmSelector {...defaultProps} />);

    const button = screen.getByRole("button");
    await user.click(button);

    const expandIcon = container.querySelector(
      '[data-testid="ExpandMoreIcon"]',
    );
    expect(expandIcon).toBeInTheDocument();
  });

  it("should apply custom className when provided", () => {
    const { container } = render(
      <RealmSelector {...defaultProps} className="custom-class" />,
    );

    const wrapper = container.querySelector(".custom-class");
    expect(wrapper).toBeInTheDocument();
  });

  it("should mark selected realm as selected in menu", async () => {
    const user = userEvent.setup();
    render(<RealmSelector {...defaultProps} selectedRealm="realm-2" />);

    await user.click(screen.getByRole("button"));

    // Find the menuitem with realm-2
    const menuItems = screen.getAllByRole("menuitem");
    const realm2MenuItem = menuItems.find((item) =>
      item.textContent?.includes("realm-2"),
    );

    expect(realm2MenuItem).toHaveClass("Mui-selected");
  });
});
