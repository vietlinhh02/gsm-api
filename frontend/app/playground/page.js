import Playground from "@/components/playground";

export const metadata = { title: "API playground · GreenSM" };

export default async function PlaygroundPage({ searchParams }) {
  const { endpoint } = await searchParams;
  return <Playground endpointId={endpoint} />;
}
