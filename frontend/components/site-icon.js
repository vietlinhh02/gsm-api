"use client";

import { Icon } from "@astryxdesign/core/Icon";
import paths from "@/components/phosphor-paths.json";

export const iconTypes = Object.fromEntries(
  Object.entries(paths).map(([name, path]) => {
    function Glyph(props) {
      return (
        <svg
          {...props}
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 256 256"
          fill="currentColor"
        >
          <path d={path} />
        </svg>
      );
    }
    return [name, Glyph];
  }),
);

export const pageIcons = {
  overview: "book-open",
  auth: "key",
  services: "car",
  trip: "currency-circle-dollar",
  hour: "clock",
  create: "navigation-arrow",
  details: "list-bullets",
  list: "table",
  cancel: "x-circle",
  geocode: "map-pin",
  status: "flask",
  webhooks: "plugs",
  "green-connect": "bus",
  playground: "code",
};

export default function SiteIcon({ name, ...props }) {
  return <Icon icon={iconTypes[name]} {...props} />;
}
