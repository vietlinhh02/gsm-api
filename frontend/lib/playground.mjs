import { endpoints } from "#lib/catalog.mjs";

export const findEndpoint = (id) =>
  endpoints.find((endpoint) => endpoint.id === id) ?? endpoints[0];
export const stringify = (value) => JSON.stringify(value, null, 2);

export function endpointPath(endpoint, environment) {
  const path = endpoint.path.replace("/v1/sandbox/", "/v1/");
  return environment === "sandbox" ? path.replace("/v1/", "/v1/sandbox/") : path;
}

export function initialState(endpointId = "services") {
  const endpoint = findEndpoint(endpointId);
  const environment = endpoint.sandboxOnly ? "sandbox" : "production";
  return {
    endpointId: endpoint.id,
    environment,
    token: "",
    clientId: "",
    clientSecret: "",
    body: stringify(endpoint.sample ?? {}),
    path: endpointPath(endpoint, environment),
    drafts: {},
    orderId: "",
    booking: null,
    result: null,
    error: "",
    busy: false,
  };
}

/** Discard tokens and quotes that belong to the previous environment. */
export function changeEnvironment(state, environment) {
  const endpoint = findEndpoint(state.endpointId);
  const id = endpoint.sandboxOnly && environment === "production" ? "services" : endpoint.id;
  return {
    ...initialState(id),
    environment,
    clientId: state.clientId,
    clientSecret: state.clientSecret,
    path: endpointPath(findEndpoint(id), environment),
  };
}

export function chooseEndpoint(state, endpointId) {
  const endpoint = findEndpoint(endpointId);
  const base =
    endpoint.sandboxOnly && state.environment !== "sandbox"
      ? changeEnvironment(state, "sandbox")
      : state;
  let path = endpointPath(endpoint, base.environment);
  if (base.orderId) path = path.replace("{id}", encodeURIComponent(base.orderId));
  return {
    ...base,
    endpointId: endpoint.id,
    path,
    body: base.drafts[endpoint.id] ?? stringify(endpoint.sample ?? {}),
    result: null,
    error: "",
  };
}

function availableEstimate(response) {
  const estimates = response?.data?.service_estimates;
  if (!Array.isArray(estimates)) return null;
  return estimates.find((item) => !item.fee_unavailable);
}

function quoteDetails(endpointId, response) {
  const estimate = availableEstimate(response);
  if (!estimate) return null;
  const info = endpointId === "hour" ? estimate.estimate_infos?.[0] : estimate.estimate_info;
  if (!info) return null;
  if (!info.fee_id) return null;
  const service = estimate.service;
  if (!service?.id) return null;
  return { feeId: info.fee_id, serviceId: service.id };
}

function estimateSource(endpointId, request) {
  return endpointId === "hour" ? request.data?.[0] : request;
}

function attachAddons(booking, request) {
  const selected = request.service_ids?.find((service) => service.id === booking.service_id);
  if (selected?.addons?.length) booking.addons = selected.addons;
  return booking;
}

/** Preserve the estimate's service, time, points and add-ons in the booking draft. */
export function bookingFromEstimate(endpointId, request, response) {
  if (!["trip", "hour"].includes(endpointId)) return null;
  const quote = quoteDetails(endpointId, response);
  const source = estimateSource(endpointId, request);
  if (!quote || !source) return null;
  const orderTime = endpointId === "hour" ? source.order_time : Number(response.data.order_time);
  if (!Number.isSafeInteger(orderTime) || orderTime < 0) return null;
  return attachAddons(
    {
      service_id: quote.serviceId,
      service_type: endpointId === "hour" ? "RIDE-HOUR" : "RIDE-TRIP",
      fee_id: quote.feeId,
      order_time: orderTime,
      payment_method: source.payment_method,
      points: source.points,
    },
    request,
  );
}

function recordCreatedOrder(state, result) {
  if (state.endpointId !== "create") return state;
  const orderId = result.data?.data?.id;
  if (!orderId) return state;
  const drafts = { ...state.drafts };
  for (const id of ["cancel", "status"]) {
    drafts[id] = stringify({ ...findEndpoint(id).sample, order_id: orderId });
  }
  return { ...state, orderId, drafts };
}

export function recordResult(state, result, request) {
  const next = { ...state, result, error: "", busy: false };
  if (result.status < 200 || result.status >= 300) return next;
  const booking = bookingFromEstimate(state.endpointId, request, result.data);
  if (booking) return { ...next, booking, drafts: { ...state.drafts, create: stringify(booking) } };
  return recordCreatedOrder(next, result);
}

/** Validate editable request JSON before making a network call. */
export function parseRequestBody(method, body) {
  if (method === "GET") return undefined;
  try {
    const parsed = JSON.parse(body);
    if (parsed === null || Array.isArray(parsed) || typeof parsed !== "object") {
      throw new Error("Request body phải là một JSON object.");
    }
    return parsed;
  } catch (error) {
    if (error instanceof SyntaxError) {
      throw new Error("JSON chưa hợp lệ. Kiểm tra dấu phẩy, dấu ngoặc và dấu nháy.");
    }
    throw error;
  }
}

export function validatePath(path) {
  if (!path.startsWith("/v1/") || path.includes("{") || path.includes("<")) {
    throw new Error("Điền request URL /v1/ hợp lệ và thay ID còn thiếu trước khi gửi.");
  }
}
