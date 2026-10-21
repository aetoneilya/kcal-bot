import { initData } from "./telegram";
import type { Profile } from "./targets";

export type Confidence = "high" | "medium" | "low";

/** One component of a meal. Nutrition is per 100 g; the line total is derived. */
export interface Item {
  name: string;
  grams: number;
  kcal100: number;
  protein100: number;
  fat100: number;
  carbs100: number;
  confidence: Confidence;
}

export interface Draft {
  title: string;
  items: Item[];
  question: string;
  /** Ready answers to `question`, shown as buttons; empty without a question. */
  options: string[];
}

/** The model's earlier question and the person's answer, for a re-estimate. */
export interface Clarification {
  question: string;
  answer: string;
}

export interface Meal {
  id: number;
  eatenAt: string;
  title: string;
  note: string;
  items: Item[];
}

export interface MealInput {
  eatenAt: string;
  title: string;
  note: string;
  items: Item[];
}

export interface Me {
  id: number;
  firstName: string;
  estimator: string;
  analysesLeft: number | null;
  /** Null until the person fills in the form. */
  profile: (Profile & { updatedAt: string }) | null;
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("X-Telegram-Init-Data", initData());
  let response: Response;
  try {
    response = await fetch(path, { ...init, headers });
  } catch {
    throw new ApiError(0, "Нет связи с сервером");
  }
  if (response.status === 204) return undefined as T;
  const body = await response.json().catch(() => null);
  if (!response.ok) {
    throw new ApiError(response.status, body?.error ?? `Ошибка ${response.status}`);
  }
  return body as T;
}

function json(method: string, body: unknown): RequestInit {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
}

export const api = {
  me: () => request<Me>("/api/me"),
  saveProfile: (profile: Profile) => request<Profile>("/api/profile", json("PUT", profile)),

  analyze(photo: Blob | null, note: string, clarification?: Clarification): Promise<Draft> {
    const form = new FormData();
    if (photo) form.append("photo", photo, "meal.jpg");
    if (note.trim()) form.append("note", note.trim());
    if (clarification) {
      form.append("question", clarification.question);
      form.append("answer", clarification.answer.trim());
    }
    return request<Draft>("/api/analyze", { method: "POST", body: form });
  },

  meals: (from: Date, to: Date) =>
    request<Meal[]>(
      `/api/meals?from=${Math.floor(from.getTime() / 1000)}&to=${Math.floor(to.getTime() / 1000)}`,
    ),

  createMeal: (meal: MealInput) => request<Meal>("/api/meals", json("POST", meal)),
  updateMeal: (id: number, meal: MealInput) => request<Meal>(`/api/meals/${id}`, json("PUT", meal)),
  deleteMeal: (id: number) => request<void>(`/api/meals/${id}`, { method: "DELETE" }),
};
