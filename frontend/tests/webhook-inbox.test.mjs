import assert from "node:assert/strict";
import { test } from "node:test";
import { inboxURL, restoreInbox, receiptContent } from "#lib/webhook-inbox.mjs";

const inbox = {
  id: "b3ef9f4e-0000-4000-8000-111111111111",
  receive_path: `/webhook-inbox/${"a".repeat(64)}`,
};

test("receiver URLs use the current app origin and reject arbitrary destinations", () => {
  assert.equal(
    inboxURL(inbox, "https://docs.example"),
    `https://docs.example/api/backend${inbox.receive_path}`,
  );
  for (const receive_path of ["https://other.example", "//other.example", "/admin", "", null]) {
    assert.throws(
      () => inboxURL({ ...inbox, receive_path }, "https://docs.example"),
      /không hợp lệ/,
    );
  }
});

test("restoring an inbox rebuilds its link and handles missing or malformed storage", () => {
  assert.equal(restoreInbox(null, "https://docs.example"), null);
  const restored = restoreInbox(
    JSON.stringify({ ...inbox, receive_url: "https://other.example" }),
    "https://docs.example",
  );
  assert.equal(restored.receive_url, inboxURL(inbox, "https://docs.example"));
  for (const value of ["{", "null", "{}", JSON.stringify({ ...inbox, id: "unknown" })]) {
    assert.throws(() => restoreInbox(value, "https://docs.example"));
  }
});

test("receipt previews format JSON and keep arbitrary text intact", () => {
  assert.deepEqual(receiptContent('{"status":"CANCELLED"}'), {
    code: '{\n  "status": "CANCELLED"\n}',
    language: "json",
  });
  for (const body of ["", "{", "Xin chào", "<script>alert(1)</script>"]) {
    assert.deepEqual(receiptContent(body), { code: body, language: "plaintext" });
  }
});
