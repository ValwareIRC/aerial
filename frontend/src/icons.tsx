const PATHS: Record<string, string> = {
  overview: "M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z",
  people: "M16 20v-1a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v1M12 7a3.5 3.5 0 1 1-7 0 3.5 3.5 0 0 1 7 0M17 11a3 3 0 1 0 0-6M21 20v-1a4 4 0 0 0-3-3.8",
  home: "M3 10.5 12 3l9 7.5M5.5 9.5V20h13V9.5M10 20v-6h4v6",
  licence: "M3 6h18v13H3zM3 10h18M7 14h5M7 17h3M16 14h2",
  money: "M12 2v20M17 6.5c0-2-2.2-3-5-3s-5 1-5 3.5S9.5 11 12 11.5s5 1.2 5 3.5-2.2 3.5-5 3.5-5-1-5-3",
  chart: "M4 20V10M10 20V4M16 20v-7M22 20H2",
  log: "M4 5h16M4 10h16M4 15h11M4 20h7",
  pin: "M12 21s7-6.2 7-11a7 7 0 1 0-14 0c0 4.8 7 11 7 11ZM12 12.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5Z",
  phone: "M5 3h4l2 5-2.5 1.5a12 12 0 0 0 6 6L16 13l5 2v4a2 2 0 0 1-2.2 2A17 17 0 0 1 3 5.2 2 2 0 0 1 5 3Z",
  mail: "M3 6h18v12H3zM3 7l9 6 9-6",
  calendar: "M3 6h18v15H3zM3 11h18M8 3v5M16 3v5",
  note: "M5 3h10l4 4v14H5zM15 3v4h4",
  card: "M3 7h18v11H3zM3 11h18M6 15h4",
  check: "m4 12.5 5 5L20 6.5",
  alert: "M12 3 1.5 21h21L12 3ZM12 9v5M12 17.5v.5",
  clock: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18ZM12 7v5.5l3.5 2",
  search: "M10.5 18a7.5 7.5 0 1 0 0-15 7.5 7.5 0 0 0 0 15ZM21 21l-5.2-5.2",
  shield: "M12 3 4.5 6v6c0 4.5 3.2 8 7.5 9 4.3-1 7.5-4.5 7.5-9V6L12 3Z",
  tag: "M3 12V4h8l10 10-8 8L3 12ZM7.5 8h.01",
  undo: "M4 9h11a5 5 0 0 1 0 10h-6M4 9l4-4M4 9l4 4",
};

export type IconName = keyof typeof PATHS | string;

export function Icon({ name, size = 18 }: { name: IconName; size?: number }) {
  const d = PATHS[name] ?? PATHS.note;
  return (
    <svg
      className="icon"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d={d} />
    </svg>
  );
}

export type Hue = "mint" | "peach" | "sky" | "lilac" | "teal" | "rose" | "sun" | "sand";

const HUES: Hue[] = ["mint", "peach", "sky", "lilac", "teal", "rose", "sun", "sand"];

/** A stable colour for a name, so the same person is always the same colour. */
export function hueFor(seed: string): Hue {
  let h = 0;
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0;
  return HUES[h % HUES.length] as Hue;
}

export function initials(name: string): string {
  const words = name
    .replace(/^(Mr|Mrs|Ms|Miss|Dr|Prof)\.?\s+/i, "")
    .split(/\s+/)
    .filter(Boolean);
  if (words.length === 0) return "?";
  if (words.length === 1) return words[0]!.slice(0, 2).toUpperCase();
  return (words[0]![0]! + words[words.length - 1]![0]!).toUpperCase();
}

export function Avatar({ name, size = 44 }: { name: string; size?: number }) {
  return (
    <span
      className={`avatar hue-${hueFor(name)}`}
      style={{ width: size, height: size, fontSize: Math.round(size * 0.36) }}
      aria-hidden="true"
    >
      {initials(name)}
    </span>
  );
}

export function IconChip({ name, hue, size = 34 }: { name: IconName; hue: Hue; size?: number }) {
  return (
    <span className={`icon-chip hue-${hue}`} style={{ width: size, height: size }}>
      <Icon name={name} size={Math.round(size * 0.55)} />
    </span>
  );
}
