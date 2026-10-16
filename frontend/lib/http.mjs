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
  413: "Nội dung webhook vượt quá giới hạn 256 KiB.",
  500: "Hệ thống chưa xử lý được yêu cầu. Hãy thử lại.",
  502: "Chưa kết nối được dịch vụ. Hãy thử lại sau.",
  503: "Dịch vụ tạm thời không phản hồi. Hãy thử lại sau.",
  504: "Yêu cầu quá thời gian chờ. Hãy thử lại.",
};

const bookingErrors = {
  "webhook inbox not found":
    "Không tìm thấy hộp thư cho tài khoản/môi trường này. Hãy tạo link mới.",
  "webhook receipt not found": "Request không còn trong hộp thư. Hãy tải lại danh sách.",
  "fee_id is invalid or expired": "Báo giá không hợp lệ hoặc đã hết hạn. Hãy so sánh giá lại.",
  "fee_id already booked with different fields; obtain a new estimate":
    "Báo giá này đã tạo chuyến với thông tin khác. Hãy lấy báo giá mới.",
  "no available vehicle within 10 km; retry later":
    "Chưa có xe rảnh trong bán kính 10 km. Hãy thử lại sau.",
  "payment_failed requires payment_method partner_pay":
    "Kịch bản thanh toán thất bại cần chọn Đối tác thanh toán rồi lấy báo giá lại.",
};

/** Obtain a token for the current environment, using the playground's configured credentials. */
export async function getAccessToken(state) {
  if (state.token) return state.token;
  const result = await requestApi({
    path: "/v1/sandbox/token",
    method: "POST",
    contentType: "application/x-www-form-urlencoded",
    body: new URLSearchParams({
      client_id: state.clientId,
      client_secret: state.clientSecret,
      grant_type: "client_credentials",
      scope: "sandbox.ride.trips",
    }),
  });
  if (result.status !== 200 || !result.data?.access_token) {
    throw new Error(apiErrorMessage(result));
  }
  return result.data.access_token;
}

/** Explain a failed API request while leaving the original JSON available for inspection. */
export function apiErrorMessage(result) {
  if (result.data?.message === "invalid client credentials") {
    return "Client ID hoặc Client secret không đúng.";
  }
  if (bookingErrors[result.data?.message]) return bookingErrors[result.data.message];
  return apiErrors[result.status] ?? "Chưa thực hiện được yêu cầu. Xem dữ liệu trả về.";
}

async function readResult(response, started) {
  const text = await response.text();
  const contentType = response.headers.get("content-type") ?? "";
  let data = null;
  if (contentType.includes("application/json")) {
    try {
      data = JSON.parse(text);
    } catch (cause) {
      throw new Error("Máy chủ trả dữ liệu không hợp lệ. Hãy thử lại yêu cầu.", { cause });
    }
  }
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
  const response = await fetchApi(path, options);
  const result = await readResult(response, started);
  return { ...result, requestPath: path, requestMethod: method };
}

async function fetchApi(path, options) {
  try {
    return await fetch("/api/backend" + path, options);
  } catch (cause) {
    const message =
      cause.name === "TimeoutError"
        ? "Yêu cầu quá thời gian chờ. Bạn có thể thử lại; cùng báo giá sẽ không tạo chuyến trùng."
        : "Không kết nối được máy chủ. Kiểm tra kết nối rồi thử lại.";
    throw new Error(message, { cause });
  }
}
