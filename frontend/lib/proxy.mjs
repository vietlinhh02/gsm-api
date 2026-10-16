import { endpoints } from "#lib/catalog.mjs";

function allowedMethod(pathname) {
  if (/^v1\/sandbox\/ride\/orders\/[^/]+\/events$/.test(pathname)) return "GET";
  if (pathname === "healthz") return "GET";
  if (pathname === "v1/ride/update-status") return undefined;
  if (["v1/public/token", "v1/sandbox/token"].includes(pathname)) return "POST";
  const normalized = "/" + pathname.replace("v1/sandbox/", "v1/");
  if (/^\/v1\/ride\/orders\/[^/]+$/.test(normalized)) return "GET";
  return endpoints.find(
    (endpoint) => endpoint.path.replace("/v1/sandbox/", "/v1/").split("?")[0] === normalized,
  )?.method;
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

async function fetchBackend(request, segments, options) {
  const origin = options.origin ?? process.env.API_ORIGIN ?? "http://localhost:8081";
  const fetchUpstream = options.fetchImpl ?? fetch;
  const pathname = segments.join("/");
  const { signal, clearTimeout: clearRequestTimeout } = backendSignal(request, pathname, options);
  try {
    return await fetchUpstream(
      origin.replace(/\/$/, "") + "/" + pathname + new URL(request.url).search,
      {
        method: request.method,
        headers: forwardedHeaders(request),
        body: request.method === "GET" ? undefined : await request.arrayBuffer(),
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
