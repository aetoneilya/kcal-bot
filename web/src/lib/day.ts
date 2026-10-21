/** Local-day arithmetic. The server stores instants; "today" is the phone's. */

export function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

/** Adds calendar days, so a DST change never shifts the day boundary. */
export function addDays(d: Date, days: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + days);
}

export function sameDay(a: Date, b: Date): boolean {
  return startOfDay(a).getTime() === startOfDay(b).getTime();
}

export function dayLabel(day: Date, now = new Date()): string {
  if (sameDay(day, now)) return "Сегодня";
  if (sameDay(day, addDays(now, -1))) return "Вчера";
  return day.toLocaleDateString("ru-RU", { weekday: "short", day: "numeric", month: "long" });
}

/** "вторник, 21 октября" — the date under a relative title like "Сегодня". */
export function fullDate(day: Date): string {
  return day.toLocaleDateString("ru-RU", { weekday: "long", day: "numeric", month: "long" });
}

export function timeLabel(d: Date): string {
  return d.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
}

/** "HH:MM" for <input type="time">. */
export function timeValue(d: Date): string {
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/** The given day at the time from an <input type="time"> value. */
export function atTime(day: Date, value: string): Date {
  const [h, m] = value.split(":").map(Number);
  return new Date(day.getFullYear(), day.getMonth(), day.getDate(), h || 0, m || 0);
}
