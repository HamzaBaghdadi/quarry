// Visual identity per engine. Logos come from Simple Icons (CC0); trademarks
// belong to their owners and are used only to identify each database.
// SQL Server and Valkey have no Simple Icons entry, so they get neutral marks.
import {
  siClickhouse,
  siCockroachlabs,
  siMariadb,
  siMongodb,
  siMysql,
  siPostgresql,
  siRedis,
  siSqlite,
  siTidb,
} from "simple-icons";

export interface EngineMark {
  /** 24×24 SVG path, or null to fall back to the monogram. */
  path: string | null;
  /** Brand colour used for the tile. */
  hue: string;
  mark: string;
}

const custom = {
  // Stylised database cylinder — not Microsoft's logo.
  mssql:
    "M12 2C7 2 3 3.6 3 5.5v13C3 20.4 7 22 12 22s9-1.6 9-3.5v-13C21 3.6 17 2 12 2Zm0 2c4.4 0 7 1.3 7 1.5S16.4 7 12 7 5 5.7 5 5.5 7.6 4 12 4ZM5 8.3C6.7 9.1 9.2 9.5 12 9.5s5.3-.4 7-1.2v3.2c0 .2-2.6 1.5-7 1.5s-7-1.3-7-1.5Zm0 6c1.7.8 4.2 1.2 7 1.2s5.3-.4 7-1.2v4.2c0 .2-2.6 1.5-7 1.5s-7-1.3-7-1.5Z",
  // Simple key glyph for Valkey.
  valkey:
    "M14.5 2a7.5 7.5 0 0 0-7.1 9.9L2 17.3V22h4.7v-2.4h2.4v-2.4h2.4l1.4-1.4A7.5 7.5 0 1 0 14.5 2Zm2.3 3.4a1.8 1.8 0 1 1 0 3.6 1.8 1.8 0 0 1 0-3.6Z",
};

const marks: Record<string, EngineMark> = {
  postgres: { path: siPostgresql.path, hue: `#${siPostgresql.hex}`, mark: "Pg" },
  cockroachdb: { path: siCockroachlabs.path, hue: `#${siCockroachlabs.hex}`, mark: "Cr" },
  mysql: { path: siMysql.path, hue: `#${siMysql.hex}`, mark: "My" },
  mariadb: { path: siMariadb.path, hue: "#1f6f8b", mark: "Ma" },
  tidb: { path: siTidb.path, hue: `#${siTidb.hex}`, mark: "Ti" },
  sqlite: { path: siSqlite.path, hue: "#0f80cc", mark: "Sl" },
  mssql: { path: custom.mssql, hue: "#cc2927", mark: "Ms" },
  clickhouse: { path: siClickhouse.path, hue: `#${siClickhouse.hex}`, mark: "Ch" },
  mongodb: { path: siMongodb.path, hue: `#${siMongodb.hex}`, mark: "Mg" },
  redis: { path: siRedis.path, hue: `#${siRedis.hex}`, mark: "Rd" },
  valkey: { path: custom.valkey, hue: "#6983ff", mark: "Vk" },
};

export const engineMark = (id?: string): EngineMark => marks[id ?? ""] ?? { path: null, mark: (id ?? "?").slice(0, 2), hue: "#94a3b8" };

/** Pick black or white glyph for legibility on a brand-coloured tile. */
export function onColor(hex: string) {
  const n = parseInt(hex.replace("#", ""), 16);
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b > 0.45 ? "#111827" : "#ffffff";
}

export const swatches = ["#3B82F6", "#22C55E", "#EAB308", "#F97316", "#EF4444", "#EC4899", "#A855F7", "#14B8A6", "#94A3B8"];
