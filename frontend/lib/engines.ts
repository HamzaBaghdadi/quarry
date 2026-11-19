// Visual identity per engine: a short monogram and a hue. We deliberately use
// neutral monograms instead of vendor logos.
const marks: Record<string, { mark: string; hue: string }> = {
  postgres: { mark: "Pg", hue: "#5b8def" },
  cockroachdb: { mark: "Cr", hue: "#7c6cf0" },
  mysql: { mark: "My", hue: "#3fa7c9" },
  mariadb: { mark: "Ma", hue: "#c9883f" },
  tidb: { mark: "Ti", hue: "#e5484d" },
  sqlite: { mark: "Sl", hue: "#4fa3d9" },
  mssql: { mark: "Ms", hue: "#d9534f" },
  clickhouse: { mark: "Ch", hue: "#e8c547" },
  mongodb: { mark: "Mg", hue: "#3fb950" },
  redis: { mark: "Rd", hue: "#e5534b" },
  valkey: { mark: "Vk", hue: "#6d8cff" },
};

export const engineMark = (id?: string) => marks[id ?? ""] ?? { mark: (id ?? "?").slice(0, 2), hue: "#94a3b8" };

export const swatches = ["#3B82F6", "#22C55E", "#EAB308", "#F97316", "#EF4444", "#EC4899", "#A855F7", "#14B8A6", "#94A3B8"];
