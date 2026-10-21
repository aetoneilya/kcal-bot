import { describe, expect, it } from "vitest";
import { complimentOfDay, pick } from "./treats";

describe("complimentOfDay", () => {
  const list = ["a", "b", "c"];

  it("keeps one compliment through the day and moves on the next", () => {
    const morning = complimentOfDay(list, new Date(2026, 9, 21, 8));
    expect(complimentOfDay(list, new Date(2026, 9, 21, 23, 59))).toBe(morning);
    const days = [21, 22, 23].map((d) => complimentOfDay(list, new Date(2026, 9, d)));
    expect(new Set(days).size).toBe(3);
  });

  it("is empty without compliments", () => {
    expect(complimentOfDay([], new Date())).toBe("");
  });
});

describe("pick", () => {
  it("avoids the last one when there is a choice", () => {
    for (let i = 0; i < 20; i++) expect(pick(["a", "b"], "a")).toBe("b");
    expect(pick(["a"], "a")).toBe("a");
    expect(pick([], "a")).toBeUndefined();
  });
});
