import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AuthorAvatar } from "./Author";

describe("author avatar", () => {
  it("shows the saved picture, and initials when there is none or it fails to load", () => {
    const { container, rerender } = render(<AuthorAvatar name="Ada Lovelace" username="ada" src="/api/v1/avatars/0123456789abcdef0123.png" />);
    const picture = container.querySelector("img");
    expect(picture).toHaveAttribute("src", "/api/v1/avatars/0123456789abcdef0123.png");
    expect(container).not.toHaveTextContent("AL");

    // The file is gone: the initials take its place.
    fireEvent.error(picture!);
    expect(container.querySelector("img")).toBeNull();
    expect(container).toHaveTextContent("AL");

    // A new picture gets its own chance.
    rerender(<AuthorAvatar name="Ada Lovelace" username="ada" src="/api/v1/avatars/ffffffffffffffffffff.png" />);
    expect(container.querySelector("img")).toHaveAttribute("src", "/api/v1/avatars/ffffffffffffffffffff.png");

    rerender(<AuthorAvatar name="Ada Lovelace" username="ada" />);
    expect(container.querySelector("img")).toBeNull();
    expect(container).toHaveTextContent("AL");
  });
});
