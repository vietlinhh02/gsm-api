import { notFound } from "next/navigation";
import { EndpointDocument, GuideDocument } from "@/components/doc-content";
import { endpoints, guides } from "#lib/catalog.mjs";

export function generateStaticParams() {
  return [
    ...endpoints.map((endpoint) => ({ slug: endpoint.id })),
    ...Object.keys(guides).map((slug) => ({ slug })),
  ];
}

export async function generateMetadata({ params }) {
  const { slug } = await params;
  const document = endpoints.find((endpoint) => endpoint.id === slug) ?? guides[slug];
  return { title: (document?.title ?? "Tài liệu") + " · GreenSM Developers" };
}

export default async function DocumentationPage({ params }) {
  const { slug } = await params;
  const endpoint = endpoints.find((item) => item.id === slug);
  if (endpoint) return <EndpointDocument endpoint={endpoint} />;
  if (guides[slug]) return <GuideDocument guide={guides[slug]} guideId={slug} />;
  notFound();
}
