import { endpoints } from "#lib/catalog.mjs";

function allowedMethod(pathname) {
  if (/^v1\/sandbox\/ride\/orders\/[^/]+\/events$/.test(pathname)) return "GET";
  if (pathname === "healthz") return "GET";
  if (!pathname.startsWith("v1/sandbox/")) return undefined;
  if (pathname === "v1/sandbox/token") return "POST";
  const normalized = "/" + pathname;
  if (/^\/v1\/sandbox\/ride\/orders\/[^/]+$/.test(normalized)) return "GET";
  return endpoints.find((endpoint) => endpoint.path.split("?")[0] === normalized)?.method;
}

function errorResponse(status, message) {
  return Response.json(
    { code: status, message },
    {
      status,
      headers: { "cache-control": "no-store" },
    },
  );
}

function validateRequest(request, segments) {
  if (segments.some((segment) => /[\\/]/.test(segment) || [".", ".."].includes(segment))) {
    return errorResponse(400, "Đường dẫn không hợp lệ.");
  }
  const method = allowedMethod(segments.join("/"));
  if (!method) return errorResponse(404, "Endpoint này chưa được hỗ trợ.");
  if (request.method !== method) return errorResponse(405, "HTTP method không phù hợp.");
  return null;
}

function forwardedHeaders(request) {
  const headers = new Headers();
  for (const name of ["authorization", "content-type", "accept-language", "accept"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  return headers;
}

function backendSignal(request, pathname, options) {
  const streaming = /^v1\/sandbox\/ride\/orders\/[^/]+\/events$/.test(pathname);
  const controller = new AbortController();
  const timeoutMs = options.timeoutMs ?? 10000;
  const timeout = streaming
    ? setTimeout(
        () => controller.abort(new DOMException("Upstream timeout", "TimeoutError")),
        timeoutMs,
      )
    : undefined;
  return {
    signal: AbortSignal.any([
      request.signal,
      streaming ? controller.signal : AbortSignal.timeout(timeoutMs),
    ]),
    clearTimeout: () => clearTimeout(timeout),
  };
}

function serverCredentials(options) {
  return {
    clientId: options.clientId ?? process.env.CLIENT_ID,
    clientSecret: options.clientSecret ?? process.env.CLIENT_SECRET,
  };
}

async function upstreamBody(request, pathname, options) {
  if (request.method === "GET") return undefined;
  const body = await request.arrayBuffer();
  const tokenRequest = pathname === "v1/sandbox/token";
  if (!tokenRequest) return body;
  const form = new URLSearchParams(new TextDecoder().decode(body));
  const { clientId, clientSecret } = serverCredentials(options);
  const hasCredentials = Boolean(form.get("client_id") || form.get("client_secret"));
  const configured = Boolean(clientId && clientSecret);
  if (hasCredentials || !configured) {
    return body;
  }
  form.set("client_id", clientId);
  form.set("client_secret", clientSecret);
  return new TextEncoder().encode(form.toString());
}

async function fetchBackend(request, segments, options) {
  const origin =
    options.origin ??
    process.env.API_ORIGIN ??
    (process.env.NODE_ENV === "production" ? undefined : "http://localhost:8081");
  if (!origin) throw new Error("API_ORIGIN is required in production");
  const fetchUpstream = options.fetchImpl ?? fetch;
  const pathname = segments.join("/");
  const { signal, clearTimeout: clearRequestTimeout } = backendSignal(request, pathname, options);
  try {
    return await fetchUpstream(
      origin.replace(/\/$/, "") + "/" + pathname + new URL(request.url).search,
      {
        method: request.method,
        headers: forwardedHeaders(request),
        body: await upstreamBody(request, pathname, options),
        cache: "no-store",
        signal,
      },
    );
  } finally {
    clearRequestTimeout();
  }
}

function forwardedResponse(response) {
  return new Response(response.body, {
    status: response.status,
    headers: {
      "content-type": response.headers.get("content-type") ?? "application/json",
      "cache-control": "no-store",
      "x-accel-buffering": "no",
    },
  });
}

function connectionError(error) {
  if (["TimeoutError", "AbortError"].includes(error.name)) {
    return errorResponse(504, "Máy chủ API phản hồi quá lâu. Hãy thử lại.");
  }
  return errorResponse(502, "Không kết nối được máy chủ API. Kiểm tra kết nối và API_ORIGIN.");
}

/** Forward only documented API routes, with bounded waiting and JSON failures. */
export async function forwardRequest(request, segments, options = {}) {
  const invalid = validateRequest(request, segments);
  if (invalid) return invalid;
  try {
    return forwardedResponse(await fetchBackend(request, segments, options));
  } catch (error) {
    return connectionError(error);
  }
}
