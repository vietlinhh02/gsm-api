/** Build a same-origin receive URL; stored paths must identify a generated receiver. */
export function inboxURL(inbox, origin) {
  if (!/^\/webhook-inbox\/[a-f0-9]{64}$/.test(inbox?.receive_path ?? "")) {
    throw new Error("Link webhook không hợp lệ. Hãy tạo link mới.");
  }
  return new URL(`/api/backend${inbox.receive_path}`, origin).href;
}

/** Restore this tab's inbox without trusting arbitrary URLs in browser storage. */
export function restoreInbox(value, origin) {
  if (!value) return null;
  const inbox = JSON.parse(value);
  if (typeof inbox?.id !== "string" || !/^[a-f0-9-]{36}$/.test(inbox.id)) {
    throw new Error("Hộp thư đã lưu không hợp lệ. Hãy tạo link mới.");
  }
  return { ...inbox, receive_url: inboxURL(inbox, origin) };
}

/** Display JSON when possible and preserve plain text or malformed JSON verbatim. */
export function receiptContent(body) {
  try {
    return { code: JSON.stringify(JSON.parse(body), null, 2), language: "json" };
  } catch (error) {
    if (error instanceof SyntaxError) return { code: body, language: "plaintext" };
    throw error;
  }
}
