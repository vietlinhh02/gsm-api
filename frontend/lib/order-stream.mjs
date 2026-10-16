/** Decode SSE frames across UTF-8 chunks; heartbeat comments carry no order data. */
export async function readOrderEvents(response, onOrder) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let pending = "";
  try {
    while (true) {
      const { value, done } = await reader.read();
      pending += decoder.decode(value, { stream: !done });
      const frames = pending.split(/\r?\n\r?\n/);
      pending = frames.pop();
      for (const frame of frames) {
        if (applyOrderFrame(frame, onOrder)) return true;
      }
      if (done) return false;
    }
  } finally {
    try {
      await reader.cancel();
    } finally {
      reader.releaseLock();
    }
  }
}

function applyOrderFrame(frame, onOrder) {
  const lines = frame.split(/\r?\n/);
  if (!lines.includes("event: order")) return false;
  const json = lines
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).replace(/^ /, ""))
    .join("\n");
  const { data } = JSON.parse(json);
  if (!data?.id || !data?.status) throw new Error("Dữ liệu chuyến đi không hợp lệ.");
  onOrder(data);
  return ["COMPLETED", "CANCELLED"].includes(data.status);
}

async function openOrderEvents(id, token, signal) {
  const response = await fetch(
    `/api/backend/v1/sandbox/ride/orders/${encodeURIComponent(id)}/events`,
    {
      headers: {
        Authorization: `Bearer ${token.trim().replace(/^Bearer\s+/i, "")}`,
        Accept: "text/event-stream",
      },
      cache: "no-store",
      signal,
    },
  );
  if ([400, 401, 403, 404].includes(response.status)) {
    await response.body?.cancel();
    return {
      failure: `Không mở được luồng chuyến đi (HTTP ${response.status}). Lấy token hoặc tạo chuyến lại.`,
    };
  }
  if (!response.ok || !response.headers.get("content-type")?.includes("text/event-stream")) {
    await response.body?.cancel();
    throw new Error("Máy chủ chưa mở được luồng cập nhật chuyến đi.");
  }
  return { response };
}

function retryDelay(ms, signal) {
  return new Promise((resolve) => {
    if (signal.aborted) return resolve();
    const finish = () => {
      clearTimeout(timer);
      signal.removeEventListener("abort", finish);
      resolve();
    };
    const timer = setTimeout(finish, ms);
    signal.addEventListener("abort", finish, { once: true });
  });
}

/** Follow one authenticated SSE connection, reconnecting to a fresh snapshot after disconnect. */
export async function followOrder({ id, token, signal, onOrder, onError }) {
  let delay = 1000;
  while (!signal.aborted) {
    try {
      const { response, failure } = await openOrderEvents(id, token, signal);
      if (failure) {
        if (!signal.aborted) onError(failure);
        return;
      }
      const terminal = await readOrderEvents(response, (order) => {
        if (!signal.aborted) onOrder(order);
        delay = 1000;
      });
      if (terminal || signal.aborted) return;
      throw new Error("Kết nối cập nhật chuyến đi bị ngắt.");
    } catch (error) {
      if (signal.aborted) return;
      onError(`${error.message} Đang kết nối lại…`);
    }
    await retryDelay(delay, signal);
    delay = Math.min(delay * 2, 15000);
  }
}
