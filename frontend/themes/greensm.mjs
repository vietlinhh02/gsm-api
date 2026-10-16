import { defineTheme } from "@astryxdesign/core/theme";
import { neutralTheme } from "@astryxdesign/theme-neutral/built";

export const greensmTheme = defineTheme({
  name: "greensm",
  extends: neutralTheme,
  typography: {
    scale: { base: 14, ratio: 1.2 },
    body: { family: "Be Vietnam Pro", fallbacks: 'system-ui, "Segoe UI", sans-serif' },
    heading: { family: "Be Vietnam Pro", fallbacks: 'system-ui, "Segoe UI", sans-serif' },
    code: { family: "ui-monospace", fallbacks: 'Consolas, "Liberation Mono", monospace' },
  },
  tokens: {
    "--text-body-leading": "1.65",
    "--text-large-leading": "1.6",
    "--text-supporting-leading": "1.6",
  },
  adaptations: {
    rules: [
      {
        when: { pointer: "coarse", width: { below: "lg" } },
        value: { typography: { scale: { base: 16, ratio: 1.1736 } } },
      },
    ],
  },
  localTokens: {
    "--map-route-color": "#167a60",
    "--map-route-outline": "#ffffff",
    "--map-dropoff-color": "#26364a",
  },
});
