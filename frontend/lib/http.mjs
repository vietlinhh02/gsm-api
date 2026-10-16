function requestHeaders({ token, body, contentType }) {
  const headers = { "Accept-Language": "vi" };
  if (token) headers.Authorization = "Bearer " + token.trim().replace(/^Bearer\s+/i, "");
  if (body !== undefined) headers["Content-Type"] = contentType ?? "application/json";
  return headers;
}

const apiErrors = {
  400: "Thông tin gửi lên chưa hợp lệ. Xem chi tiết trong dữ liệu trả về.",
  401: "Access token không hợp lệ hoặc đã hết hạn. Lấy token mới ở mục Xác thực.",
  404: "Không tìm thấy địa chỉ hoặc chuyến đi.",
  409: "Chuyến đi không thể thực hiện thao tác này ở trạng thái hiện tại.",
  500: "Hệ thống chưa xử lý được yêu cầu. Hãy thử lại.",
  502: "Chưa kết nối được dịch vụ. Hãy thử lại sau.",
  503: "Dịch vụ tạm thời không phản hồi. Hãy thử lại sau.",
  504: "Yêu cầu quá thời gian chờ. Hãy thử lại.",
};

/** Explain a failed API request while leaving the original JSON available for inspection. */
export function apiErrorMessage(result) {
  if (result.data?.message === "invalid client credentials") {
    return "Client ID hoặc Client secret không đúng.";
  }
  return apiErrors[result.status] ?? "Chưa thực hiện được yêu cầu. Xem dữ liệu trả về.";
}

async function readResult(response, started) {
  const text = await response.text();
  const contentType = response.headers.get("content-type") ?? "";
  const data = contentType.includes("application/json") ? JSON.parse(text) : null;
  return {
    status: response.status,
    data,
    text: data === null ? text : JSON.stringify(data, null, 2),
    duration: Math.round(performance.now() - started),
  };
}

/** Send same-origin API calls, preserving upstream HTTP errors for inspection. */
export async function requestApi({ path, method = "GET", token, body, contentType }) {
  const headers = requestHeaders({ token, body, contentType });
  const started = performance.now();
  const options = { method, headers, signal: AbortSignal.timeout(15000) };
  if (method !== "GET") options.body = body;
  const result = await readResult(await fetch("/api/backend" + path, options), started);
  return { ...result, requestPath: path, requestMethod: method };
}
