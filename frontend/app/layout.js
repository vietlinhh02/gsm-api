import "@/app/layers.css";
import "@astryxdesign/core/reset.css";
import "@astryxdesign/core/astryx.css";
import "maplibre-gl/dist/maplibre-gl.css";
import "@/themes/greensm.css";
import "@/themes/fonts.css";
import "@/app/globals.css";
import DocsShell from "@/components/docs-shell";

export const metadata = {
  title: "API đặt xe · GreenSM",
  description: "Tài liệu API, báo giá và quản lý chuyến xe GreenSM.",
};

export default function RootLayout({ children }) {
  return (
    <html lang="vi" data-scroll-behavior="smooth">
      <body>
        <DocsShell>{children}</DocsShell>
      </body>
    </html>
  );
}
