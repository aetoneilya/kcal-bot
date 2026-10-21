/**
 * Compliments and dancing cats for the people the server picked (see
 * api/internal/treats). The lists come from the server; this only chooses.
 */

export interface Treats {
  compliments: string[];
  /** Video file names, fetched one at a time through the API. */
  cats: string[];
}

/** The same compliment all day, the next one tomorrow, through the whole list. */
export function complimentOfDay(list: string[], day: Date): string {
  if (list.length === 0) return "";
  const n = Math.floor(Date.UTC(day.getFullYear(), day.getMonth(), day.getDate()) / 86_400_000);
  return list[n % list.length];
}

/** A random element, other than `avoid` when there is a choice. */
export function pick<T>(list: T[], avoid?: T): T | undefined {
  const choices = list.length > 1 ? list.filter((x) => x !== avoid) : list;
  return choices[Math.floor(Math.random() * choices.length)];
}

/**
 * Hands out cat videos as object URLs, never the same one twice in a row.
 * One can be fetched ahead, so the cat after a save starts at once; the
 * caller revokes each URL when done with it.
 */
export class Cats {
  private ahead: Promise<string> | null = null;
  private last = "";

  constructor(
    private readonly names: string[],
    private readonly load: (name: string) => Promise<Blob>,
  ) {}

  get any(): boolean {
    return this.names.length > 0;
  }

  prefetch(): void {
    if (this.ahead || !this.any) return;
    const ahead = this.fetchOne();
    this.ahead = ahead;
    ahead.catch(() => {
      if (this.ahead === ahead) this.ahead = null;
    });
  }

  next(): Promise<string> {
    const next = this.ahead ?? this.fetchOne();
    this.ahead = null;
    return next;
  }

  private async fetchOne(): Promise<string> {
    const name = pick(this.names, this.last)!;
    this.last = name;
    return URL.createObjectURL(await this.load(name));
  }
}
