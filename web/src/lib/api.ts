import { initData } from "./telegram";
import type { Profile } from "./targets";
import type { Treats } from "./treats";

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
  /** A preview is at /api/meals/{id}/photo. */
  hasPhoto: boolean;
}

export interface MealInput {
  eatenAt: string;
  title: string;
  note: string;
  items: Item[];
  /** Base64 JPEG preview, on create only; an update keeps the saved one. */
  photo?: string;
  /** The same photo bigger, for the meal's own screen; same rules. */
  photoLarge?: string;
}

/** A meal worth repeating: its latest instance and how often it was eaten. */
export interface Regular {
  meal: Meal;
  times: number;
}

/** One weigh-in on a local day. */
export interface Weight {
  day: string; // YYYY-MM-DD
  weightKg: number;
}

/** Water drunk on a local day and the day's goal from the current weight. */
export interface Water {
  day: string; // YYYY-MM-DD
  ml: number;
  goalMl: number;
}

/** Bot messages a person opted into, on their own clock. */
export interface Reminders {
  timeZone: string;
  summaryAt: string; // "HH:MM", or "" for off
  nudge: boolean;
}

export interface Me {
  id: number;
  firstName: string;
  estimator: string;
  analysesLeft: number | null;
  /** Null until the person fills in the form. */
  profile: (Profile & { updatedAt: string }) | null;
  /** Compliments and cats are on for this person. */
  treats: boolean;
  /** The owner: the look-and-feel lab opens on a long press of the title. */
  lab: boolean;
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

async function blob(path: string): Promise<Blob> {
  let response: Response;
  try {
    response = await fetch(path, { headers: { "X-Telegram-Init-Data": initData() } });
  } catch {
    throw new ApiError(0, "Нет связи с сервером");
  }
  if (!response.ok) throw new ApiError(response.status, `Ошибка ${response.status}`);
  return response.blob();
}

function json(method: string, body: unknown): RequestInit {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
}

export const api = {
  me: () => request<Me>("/api/me"),
  saveProfile: (profile: Profile) => request<Profile>("/api/profile", json("PUT", profile)),

  analyze(photos: Blob[], note: string, clarification?: Clarification): Promise<Draft> {
    const form = new FormData();
    photos.forEach((photo, i) => form.append("photo", photo, `meal-${i + 1}.jpg`));
    if (note.trim()) form.append("note", note.trim());
    if (clarification) {
      form.append("question", clarification.question);
      form.append("answer", clarification.answer.trim());
    }
    return request<Draft>("/api/analyze", { method: "POST", body: form });
  },

  regularMeals: () => request<Regular[]>("/api/meals/regular"),
  barcode: (code: string) => request<Draft>(`/api/barcode/${code}`),

  water: (day: string) => request<Water>(`/api/water/${day}`),
  saveWater: (day: string, ml: number) => request<Water>(`/api/water/${day}`, json("PUT", { ml })),

  reminders: () => request<Reminders>("/api/reminders"),
  saveReminders: (r: Reminders) => request<Reminders>("/api/reminders", json("PUT", r)),

  weights: (from: string, to: string) => request<Weight[]>(`/api/weights?from=${from}&to=${to}`),
  saveWeight: (day: string, weightKg: number) => request<Weight>(`/api/weights/${day}`, json("PUT", { weightKg })),
  deleteWeight: (day: string) => request<void>(`/api/weights/${day}`, { method: "DELETE" }),

  treats: () => request<Treats>("/api/treats"),
  cat: (name: string) => blob(`/api/treats/cats/${encodeURIComponent(name)}`),

  meals: (from: Date, to: Date) =>
    request<Meal[]>(
      `/api/meals?from=${Math.floor(from.getTime() / 1000)}&to=${Math.floor(to.getTime() / 1000)}`,
    ),

  createMeal: (meal: MealInput) => request<Meal>("/api/meals", json("POST", meal)),
  updateMeal: (id: number, meal: MealInput) => request<Meal>(`/api/meals/${id}`, json("PUT", meal)),
  deleteMeal: (id: number) => request<void>(`/api/meals/${id}`, { method: "DELETE" }),
  mealPhoto: (id: number, large = false) => blob(`/api/meals/${id}/photo${large ? "?size=large" : ""}`),
};
