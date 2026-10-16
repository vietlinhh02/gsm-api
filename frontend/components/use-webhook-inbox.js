"use client";

import { useEffect, useState } from "react";
import { apiErrorMessage, getAccessToken, requestApi } from "#lib/http.mjs";
import { inboxURL } from "#lib/webhook-inbox.mjs";

async function inboxRequest(path, token, body) {
  const result = await requestApi({
    path,
    token,
    method: body === undefined ? "GET" : "POST",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (result.status < 200 || result.status >= 300) throw new Error(apiErrorMessage(result));
  return result.data;
}

export function useWebhookInbox(model, active) {
  const { state, patch } = model;
  const inbox = state.webhookInbox;
  const prefix = "/v1/sandbox";
  const storageKey = `gsm-webhook-inbox:sandbox:${state.clientId}`;
  const [receipts, setReceipts] = useState([]);
  const [selected, setSelected] = useState(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  useInboxPolling({ state, patch, active, prefix, setReceipts, setError });

  async function perform(task) {
    setBusy(true);
    patch({ busy: true });
    setError("");
    setNotice("");
    try {
      const token = await getAccessToken(state);
      if (token !== state.token) patch({ token });
      await task(token);
    } catch (failure) {
      setError(failure.message);
    } finally {
      setBusy(false);
      patch({ busy: false });
    }
  }

  const refresh = async (token) => {
    const result = await inboxRequest(`${prefix}/webhook-inboxes/${inbox.id}`, token);
    setReceipts(result.receipts);
  };
  return {
    inbox,
    receipts,
    selected,
    error: error || state.webhookInboxError,
    notice,
    busy,
    ...inboxActions({
      model,
      inbox,
      prefix,
      storageKey,
      perform,
      refresh,
      setSelected,
      setReceipts,
      setNotice,
      setError,
    }),
  };
}

function inboxActions(context) {
  const { inbox, prefix, perform, refresh, setSelected, setNotice, setError } = context;
  return {
    create: () => perform((token) => createInbox(context, token)),
    refresh: () => perform(refresh),
    test: () =>
      perform(async (token) => {
        await inboxRequest(inbox.receive_path, "", {
          event_id: crypto.randomUUID(),
          event_type: "webhook.test",
          data: { message: "Webhook đã hoạt động" },
        });
        await refresh(token);
        setNotice("Đã gửi event thử vào hộp thư.");
      }),
    select: (id) =>
      perform(async (token) => {
        const { data } = await inboxRequest(
          `${prefix}/webhook-inboxes/${inbox.id}/receipts/${id}`,
          token,
        );
        setSelected(data);
      }),
    copy: () => copyInboxURL(inbox.receive_url, setNotice, setError),
  };
}

async function createInbox(context, token) {
  const { model, prefix, storageKey, setReceipts, setSelected, setNotice } = context;
  const { data } = await inboxRequest(`${prefix}/webhook-inboxes`, token, {});
  const next = { ...data, receive_url: inboxURL(data, window.location.origin) };
  model.patch({ webhookInbox: next });
  setReceipts([]);
  setSelected(null);
  try {
    sessionStorage.setItem(storageKey, JSON.stringify(next));
    window.dispatchEvent(new Event("gsm-webhook-inbox"));
  } catch {
    setNotice("Link đã tạo. Trình duyệt không cho lưu hộp thư để khôi phục sau khi tải lại.");
  }
}

async function copyInboxURL(url, setNotice, setError) {
  try {
    await navigator.clipboard.writeText(url);
    setNotice("Đã sao chép link webhook.");
  } catch {
    setError("Không sao chép tự động được. Hãy chọn và sao chép URL trong ô Link nhận webhook.");
  }
}

function useInboxPolling({ state, patch, active, prefix, setReceipts, setError }) {
  const { token, clientId, clientSecret } = state;
  const id = state.webhookInbox?.id;
  useEffect(() => {
    if (!active || !id) return;
    let cancelled = false;
    let loading = false;
    async function refresh() {
      if (loading) return;
      loading = true;
      try {
        const access = await getAccessToken({ token, clientId, clientSecret });
        if (cancelled) return;
        if (access !== token) patch({ token: access });
        const result = await inboxRequest(`${prefix}/webhook-inboxes/${id}`, access);
        if (!cancelled) {
          setReceipts(result.receipts);
          setError("");
        }
      } catch (failure) {
        if (!cancelled) setError(failure.message);
      } finally {
        loading = false;
      }
    }
    void refresh();
    const timer = setInterval(() => void refresh(), 3000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [active, id, token, clientId, clientSecret, prefix, patch, setReceipts, setError]);
}
