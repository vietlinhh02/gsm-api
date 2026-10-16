import { forwardRequest } from "#lib/proxy.mjs";

async function proxy(request, context) {
  const { path } = await context.params;
  return forwardRequest(request, path);
}

export const maxDuration = 300;

export const GET = proxy;
export const POST = proxy;
